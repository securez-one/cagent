package csender

import (
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/securez-one/cagent/pkg/common"
)

type Csender struct {
	HubURL     string
	HubToken   string
	HubGzip    bool
	CheckName  string
	Verbose    bool
	RetryLimit int
	Timeout    time.Duration

	version string
	result  common.MeasurementsMap
}

func (cs *Csender) SetVersion(version string) {
	cs.version = version
}

func (cs *Csender) userAgent() string {
	if cs.version == "" {
		cs.version = "{undefined}"
	}
	parts := strings.Split(cs.version, "-")

	return fmt.Sprintf("Csender v%s %s %s", parts[0], runtime.GOOS, runtime.GOARCH)
}
