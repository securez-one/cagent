// +build windows

package main

import (
	"github.com/kardianos/service"

	"github.com/securez-one/cagent"
)

func updateServiceConfig(ca *cagent.Cagent, username string) {
	// nothing to do
}

func configureServiceEnabledState(s service.Service) {
	// nothing to do
}
