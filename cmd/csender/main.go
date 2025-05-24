package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/securez-one/cagent"
	"github.com/securez-one/cagent/pkg/csender"
)

type boolFlag struct {
	set   bool
	value bool
}

func (sf *boolFlag) Set(value string) error {
	i, err := strconv.Atoi(value)
	if err != nil {
		return err
	}

	switch i {
	case 0:
		sf.value = false
	case 1:
		sf.value = true
	default:
		return fmt.Errorf("arg can be either 0 or 1")
	}

	sf.set = true
	return nil
}

func (sf *boolFlag) String() string {
	var s string
	if sf.set {
		if sf.value {
			s = "1"
		} else {
			s = "0"
		}
	}

	return s
}

func (sf *boolFlag) BoolPtr() *bool {
	if !sf.set {
		return nil
	}

	return &sf.value
}

func fatal(msg string) {
	_, _ = fmt.Fprintln(os.Stderr, msg)
	os.Exit(1)
}

func main() {
	var successFlag boolFlag

	checkNamePtr := flag.String("n", "", "check name (*required)")
	tokenPtr := flag.String("t", "", "custom check token (*required)")
	hubURLPtr := flag.String("u", "https://hub.cloudradar.io/cct/", "hub URL to use")
	flag.Var(&successFlag, "s", "set success [0,1]")
	alertMessagePtr := flag.String("a", "", "alert message")
	warningMessagePtr := flag.String("w", "", "warning message")
	retriesPtr := flag.String("r", "5", "number of retries")
	maxTimePtr := flag.String("m", "15", "hub connection timeout in seconds")
	verbosePtr := flag.Bool("v", false, "verbose")

	versionPtr := flag.Bool("version", false, "show the csender version")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of %s:\n", os.Args[0])
		flag.PrintDefaults()
		fmt.Fprintln(flag.CommandLine.Output(), "  key=value\n"+
			"        Arbitrary data to send. Use multiple times.")
		fmt.Fprintln(flag.CommandLine.Output(), "See https://docs.cloudradar.io/configuring-hosts/managing-checks/custom-checks#sending-data-using-csender")

		fmt.Fprintln(flag.CommandLine.Output(), "")
		fmt.Fprintf(flag.CommandLine.Output(), `Example:
%s -t <TOKEN> -n <CHECK_NAME> -s 1 -a "This text triggers an alert. Optional" -w "This text triggers a warning. Optional" any_number=1 any_float=0.1245 any_string="Put your check result here"`+"\n", os.Args[0])
	}
	flag.Parse()

	if *versionPtr {
		printVersion()
		return
	}

	if tokenPtr == nil || *tokenPtr == "" {
		fatal("-t token arg is required")
	}

	if checkNamePtr == nil || *checkNamePtr == "" {
		fatal("-n check name arg is required")
	}

	if hubURLPtr == nil || *hubURLPtr == "" {
		fatal("-u hub url arg can't be empty")
	}

	cs := csender.Csender{
		HubURL:     *hubURLPtr,
		HubToken:   *tokenPtr,
		CheckName:  *checkNamePtr,
		Verbose:    *verbosePtr,
		Timeout:    15 * time.Second,
		HubGzip:    true,
		RetryLimit: 5,
	}

	var kvParams []string
	var skipNext bool
	for _, arg := range os.Args[1:] {
		if skipNext {
			skipNext = false
			continue
		}

		if strings.HasPrefix("-", arg) {
			skipNext = true
			continue
		}

		if !strings.Contains(arg, "=") {
			continue
		}

		kvParams = append(kvParams, arg)
	}

	err := cs.AddMultipleKeyValue(kvParams)
	if err != nil {
		fatal(err.Error())
	}

	if successFlag.set {
		err := cs.SetSuccess(successFlag.value)
		if err != nil {
			fatal(err.Error())
		}
	} else {
		err := cs.SetSuccess(true)
		if err != nil {
			fatal(err.Error())
		}
	}

	if alertMessagePtr != nil && *alertMessagePtr != "" {
		err := cs.SetAlert(*alertMessagePtr)
		if err != nil {
			fatal(err.Error())
		}
	}

	if warningMessagePtr != nil && *warningMessagePtr != "" {
		err := cs.SetWarning(*warningMessagePtr)
		if err != nil {
			fatal(err.Error())
		}
	}

	if retriesPtr != nil {
		retries, err := strconv.ParseInt(*retriesPtr, 10, 64)
		if err != nil {
			fatal(err.Error())
		}
		cs.RetryLimit = int(retries)
	}

	if maxTimePtr != nil {
		maxTime, err := strconv.ParseInt(*maxTimePtr, 10, 64)
		if err != nil {
			fatal(err.Error())
		}
		cs.Timeout = time.Duration(maxTime) * time.Second
	}

	if err := cs.GracefulSend(); err != nil {
		fatal(err.Error())
	}
}

func printVersion() {
	fmt.Printf("csender - tool for sending custom check results to Hub.\nPart of cagent package v%s %s\n", cagent.Version, cagent.LicenseInfo)
}
