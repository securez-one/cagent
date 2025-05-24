// +build windows

package hyperv

import (
	"fmt"
	"time"

	"github.com/securez-one/cagent/perfcounters"
	"github.com/securez-one/cagent/pkg/monitoring"
	"github.com/securez-one/cagent/pkg/monitoring/vmstat/types"
	"github.com/securez-one/cagent/pkg/wmi"
)

type impl struct {
	watcher *perfcounters.WinPerfCountersWatcher
}

var _ types.Provider = (*impl)(nil)

func New() types.Provider {
	return &impl{
		watcher: monitoring.GetWatcher(),
	}
}

func (im *impl) Run() error {
	interval := time.Second * 1

	err := im.watcher.StartContinuousQuery(hypervPath, interval)
	if err != nil {
		return fmt.Errorf("vmstat run failure: %s", err.Error())
	}

	return nil
}

func (im *impl) Shutdown() error {
	return nil
}

func (im *impl) Name() string {
	return "hyper-v"
}

func (im *impl) IsAvailable() error {
	st, err := wmiutil.CheckOptionalFeatureStatus(wmiutil.FeatureMicrosoftHyperV)
	if err != nil {
		return fmt.Errorf("%s %s", types.ErrCheck.Error(), err.Error())
	}

	if st != wmiutil.FeatureInstallStateEnabled {
		return types.ErrNotAvailable
	}

	return nil
}
