// +build windows

package processes

import (
	"runtime"
	"time"

	"github.com/pkg/errors"

	"github.com/securez-one/cagent/pkg/common"
	"github.com/securez-one/cagent/pkg/winapi"
)

var monitoredProcessCache = make(map[uint32]*winapi.SystemProcessInformation)
var lastProcessQueryTime time.Time
var windowsEnumerator *winapi.WindowsEnumerator

func init() {
	windowsEnumerator = winapi.NewWindowsEnumerator()
}

func processes(systemMemorySize uint64) ([]*ProcStat, error) {
	procByPid, threadsByProcPid, err := winapi.GetSystemProcessInformation(false)
	if err != nil {
		return nil, errors.Wrap(err, "can't get system processes")
	}

	now := time.Now()
	timeElapsedReal := 0.0
	if !lastProcessQueryTime.IsZero() {
		timeElapsedReal = now.Sub(lastProcessQueryTime).Seconds()
	}

	var result []*ProcStat
	var updatedProcessCache = make(map[uint32]*winapi.SystemProcessInformation)
	cmdLineRetrievalFailuresCount := 0
	logicalCPUCount := uint8(runtime.NumCPU())
	windowByProcessId, err := windowsEnumerator.Enumerate()
	if err != nil {
		log.Errorf("failed to list all windows by processId")
	}

	for pid, proc := range procByPid {
		if pid == 0 {
			continue
		}

		cmdLine, err := winapi.GetProcessCommandLine(pid)
		if err != nil {
			// there are some edge-cases when we can't get cmdLine in reliable way.
			// it includes system processes, which are not accessible in user-mode and processes from outside of WOW64 when running as a 32-bit process
			cmdLineRetrievalFailuresCount++
		}

		oldProcessInfo, oldProcessInfoExists := monitoredProcessCache[pid]
		cpuUsagePercent := 0.0
		if oldProcessInfoExists && timeElapsedReal > 0 {
			cpuUsagePercent = winapi.CalculateProcessCPUUsagePercent(oldProcessInfo, proc, timeElapsedReal, logicalCPUCount)
		}

		allSuspended := true
		for _, thread := range threadsByProcPid[pid] {
			if thread.ThreadState != winapi.SystemThreadStateWait {
				allSuspended = false
			} else {
				if thread.WaitReason != winapi.SystemThreadWaitReasonSuspended {
					allSuspended = false
				}
			}
		}

		// default state is running
		var state = "running"

		if allSuspended {
			// all threads suspended so mark the process as suspended
			state = "suspended"
		} else if windowByProcessId != nil {
			if window, exists := windowByProcessId[pid]; exists {
				isHanging, err := winapi.IsHangWindow(window)
				if err != nil {
					log.WithError(err).Error("can't query hang window")
				} else if isHanging {
					state = "not responding"
				}
			}
		}

		memoryUsagePercent := 0.0
		if systemMemorySize > 0 {
			memoryUsagePercent = (float64(proc.WorkingSetSize) / float64(systemMemorySize)) * 100
		}

		ps := &ProcStat{
			PID:                    int(pid),
			ParentPID:              int(proc.InheritedFromUniqueProcessID),
			ProcessGID:             -1,
			State:                  state,
			Name:                   proc.ImageName.String(),
			Cmdline:                cmdLine,
			CPUAverageUsagePercent: float32(common.RoundToTwoDecimalPlaces(cpuUsagePercent)),
			RSS:                    uint64(proc.WorkingSetPrivateSize),
			VMS:                    uint64(proc.VirtualSize),
			MemoryUsagePercent:     float32(common.RoundToTwoDecimalPlaces(memoryUsagePercent)),
		}

		updatedProcessCache[pid] = proc
		result = append(result, ps)
	}
	lastProcessQueryTime = now
	monitoredProcessCache = updatedProcessCache

	if cmdLineRetrievalFailuresCount > 0 {
		log.Debugf("could not get command line for %d processes", cmdLineRetrievalFailuresCount)
	}

	return result, nil
}

func isKernelTask(p *ProcStat) bool {
	// For Windows we can't distinct if the process is a system process without additional API calls.
	// For performance reasons this feature will be ignored for Windows:
	return false

	// if needed it can be implemented using next pseudocode:
	// return p.PID < 1 || RtlEqualSid(p.SessionId, LocalSystemSid)
	// Where LocalSystemSID is { SID_REVISION, 1, SECURITY_NT_AUTHORITY, { SECURITY_LOCAL_SYSTEM_RID } };
	// See https://github.com/processhacker/processhacker/blob/master/phlib/data.c for more details
}
