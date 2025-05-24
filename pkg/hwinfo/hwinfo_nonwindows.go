// +build !windows

package hwinfo

import (
	"bufio"
	"bytes"
	"fmt"
	"io/ioutil"
	"os/exec"
	"strings"

	"github.com/cloudradar-monitoring/dmidecode"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/securez-one/cagent/pkg/common"
)

func isDmidecodeAvailable() bool {
	cmd := exec.Command("/bin/sh", "-c", "command -v dmidecode")
	if err := cmd.Run(); err != nil {
		return false
	}

	return true
}

func fetchInventory() (map[string]interface{}, error) {
	res := make(map[string]interface{})
	errorCollector := common.ErrorCollector{}

	pciDevices, err := listPCIDevices()
	errorCollector.Add(err)
	if len(pciDevices) > 0 {
		res["pci.list"] = pciDevices
	}

	usbDevices, err := listUSBDevices()
	errorCollector.Add(err)
	if len(usbDevices) > 0 {
		res["usb.list"] = usbDevices
	}

	displays, err := listDisplays()
	errorCollector.Add(err)
	if len(displays) > 0 {
		res["displays.list"] = displays
	}

	cpus, err := listCPUs()
	errorCollector.Add(err)
	if cpus != nil {
		res = common.MergeStringMaps(res, cpus)
	}

	dmiDecodeResults, err := retrieveInfoUsingDmiDecode()
	errorCollector.Add(err)
	if len(dmiDecodeResults) > 0 {
		res = common.MergeStringMaps(res, dmiDecodeResults)
	}

	return res, errorCollector.Combine()
}

func retrieveInfoUsingDmiDecode() (map[string]interface{}, error) {
	if !isDmidecodeAvailable() {
		common.LogOncef(log.InfoLevel, "[HWINFO] dmidecode is not present. Skipping retrieval of baseboard, CPU and RAM info...")
		return nil, nil
	}

	cmd := exec.Command("/bin/sh", "-c", dmidecodeCommand())

	stdoutBuffer := bytes.Buffer{}
	cmd.Stdout = bufio.NewWriter(&stdoutBuffer)

	stderrBuffer := bytes.Buffer{}
	cmd.Stderr = bufio.NewWriter(&stderrBuffer)

	if err := cmd.Run(); err != nil {
		stderrBytes, _ := ioutil.ReadAll(bufio.NewReader(&stderrBuffer))
		stderr := string(stderrBytes)
		if strings.Contains(stderr, "/dev/mem: Operation not permitted") {
			log.Infof("[HWINFO] there was an error while executing '%s': %s\nProbably 'CONFIG_STRICT_DEVMEM' kernel configuration option is enabled. Please refer to kernel configuration manual.", dmidecodeCommand(), stderr)
			return nil, nil
		}
		return nil, errors.Wrap(err, "execute dmidecode")
	}

	dmi, err := dmidecode.Unmarshal(bufio.NewReader(&stdoutBuffer))
	if err != nil {
		return nil, errors.Wrap(err, "unmarshal dmi")
	}

	res := make(map[string]interface{})

	// all below requests are based on parsed data returned by dmidecode.Unmarshal
	// refer to doc dmidecode.Get to get description of function behavior
	var reqSys []dmidecode.ReqBaseBoard
	if err = dmi.Get(&reqSys); err == nil {
		res["baseboard.manufacturer"] = reqSys[0].Manufacturer
		res["baseboard.model"] = reqSys[0].Version
		res["baseboard.serial_number"] = reqSys[0].SerialNumber
	} else if err != dmidecode.ErrNotFound {
		log.WithError(err).Info("[HWINFO] failed fetching baseboard info")
	}

	var reqMem []dmidecode.ReqPhysicalMemoryArray
	if err = dmi.Get(&reqMem); err == nil {
		res["ram.number_of_modules"] = reqMem[0].NumberOfDevices
	} else if err != dmidecode.ErrNotFound {
		log.WithError(err).Info("[HWINFO] failed fetching memory array info")
	}

	var reqMemDevs []dmidecode.ReqMemoryDevice
	if err = dmi.Get(&reqMemDevs); err == nil {
		for i := range reqMemDevs {
			if reqMemDevs[i].Size == -1 {
				continue
			}
			res[fmt.Sprintf("ram.%d.size_B", i)] = reqMemDevs[i].Size
			res[fmt.Sprintf("ram.%d.type", i)] = reqMemDevs[i].Type
		}
	} else if err != dmidecode.ErrNotFound {
		log.WithError(err).Info("[HWINFO] failed fetching memory device info")
	}

	return res, nil
}
