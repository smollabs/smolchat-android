//go:build !android

package app

import (
	"os/exec"
	"runtime"
	"strings"
)

func thermalStatus() ThermalStatus { return ThermalUnknown }

// systemDark reports the desktop appearance (dark unless macOS says light).
func systemDark() bool {
	if runtime.GOOS != "darwin" {
		return true
	}
	out, _ := exec.Command("defaults", "read", "-g", "AppleInterfaceStyle").Output()
	return strings.Contains(string(out), "Dark")
}
