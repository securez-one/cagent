// +build !windows

package main

import (
	"github.com/securez-one/cagent"
)

// this dumb func exists only for cross-platform compiling, because it was mentioned in the main.go(which is compiling for all platforms)
func windowsShowSettingsUI(_ *cagent.Cagent, _ bool) {

}
