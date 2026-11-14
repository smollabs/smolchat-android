package app

import (
	"bufio"
	"bytes"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

// DeviceInfo is a best-effort hardware snapshot used for recommendations.
type DeviceInfo struct {
	TotalRAM    int64
	AvailRAM    int64
	FreeStorage int64
	Arch        string
	CPUCores    int
	DotProd     bool   // ARMv8.2 dot-product (required by the Android build)
	OS          string // e.g. "Android 15 (API 35)"
	Model       string // device model, when known
	Thermal     ThermalStatus
	SystemDark  bool // the OS is in dark mode
}

// ThermalStatus mirrors Android's PowerManager.THERMAL_STATUS_* levels.
type ThermalStatus int

const (
	ThermalUnknown ThermalStatus = iota - 1
	ThermalNone
	ThermalLight
	ThermalModerate
	ThermalSevere
	ThermalCritical
	ThermalEmergency
	ThermalShutdown
)

func (t ThermalStatus) String() string {
	switch t {
	case ThermalNone:
		return "Normal"
	case ThermalLight:
		return "Warm"
	case ThermalModerate:
		return "Hot"
	case ThermalSevere, ThermalCritical, ThermalEmergency, ThermalShutdown:
		return "Throttling"
	}
	return "Unknown"
}

// DetectDevice reads RAM, storage and CPU details. dir is a path on the
// storage volume models are written to.
func DetectDevice(dir string) DeviceInfo {
	d := DeviceInfo{Arch: runtime.GOARCH, CPUCores: runtime.NumCPU(), Thermal: ThermalUnknown}
	d.TotalRAM, d.AvailRAM = memInfo()
	d.FreeStorage = FreeSpace(dir)
	d.DotProd = cpuHasDotProd()
	d.OS, d.Model = osInfo()
	d.Thermal = thermalStatus()
	d.SystemDark = systemDark()
	return d
}

// RefreshDynamic updates the fields that change while running.
func (d *DeviceInfo) RefreshDynamic(dir string) {
	if t, a := memInfo(); t > 0 {
		d.TotalRAM, d.AvailRAM = t, a
	}
	d.FreeStorage = FreeSpace(dir)
	d.Thermal = thermalStatus()
	d.SystemDark = systemDark()
}

func memInfo() (total, avail int64) {
	if runtime.GOOS == "linux" || runtime.GOOS == "android" {
		f, err := os.Open("/proc/meminfo")
		if err != nil {
			return 0, 0
		}
		defer f.Close()
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			fields := strings.Fields(sc.Text())
			if len(fields) < 2 {
				continue
			}
			kb, _ := strconv.ParseInt(fields[1], 10, 64)
			switch fields[0] {
			case "MemTotal:":
				total = kb << 10
			case "MemAvailable:":
				avail = kb << 10
			}
		}
		return total, avail
	}
	total = sysctlMem()
	return total, total / 2 // desktop dev builds: rough guess
}

// sysctlMem reports physical memory on desktop dev builds (set on darwin).
var sysctlMem = func() int64 { return 0 }

func cpuHasDotProd() bool {
	if runtime.GOARCH != "arm64" {
		return false
	}
	if runtime.GOOS == "darwin" {
		return true // all Apple silicon
	}
	b, err := os.ReadFile("/proc/cpuinfo")
	return err == nil && bytes.Contains(b, []byte("asimddp"))
}

func osInfo() (osName, model string) {
	if runtime.GOOS != "android" {
		return runtime.GOOS, ""
	}
	rel, sdk := getprop("ro.build.version.release"), getprop("ro.build.version.sdk")
	osName = "Android"
	if rel != "" {
		osName += " " + rel
	}
	if sdk != "" {
		osName += " (API " + sdk + ")"
	}
	model = strings.TrimSpace(getprop("ro.product.manufacturer") + " " + getprop("ro.product.model"))
	return osName, model
}

func getprop(name string) string {
	out, err := exec.Command("/system/bin/getprop", name).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// Fit describes how well a model suits the device.
type Fit int

const (
	FitGreat Fit = iota // comfortably fits
	FitOK               // fits, may be slower
	FitTight            // likely to be killed or very slow
	FitNo               // will not fit
)

func (f Fit) Label() string {
	switch f {
	case FitGreat:
		return "Runs well"
	case FitOK:
		return "May be slow"
	case FitTight:
		return "Tight on memory"
	}
	return "Not enough RAM"
}

// AssessFit compares a model's estimated runtime memory to device RAM.
// Available RAM is volatile on Android (other apps get killed to make room),
// so total RAM is the main signal, with available RAM as a tie breaker.
func AssessFit(spec ModelSpec, d DeviceInfo, nCtx int) Fit {
	need := spec.EstimateRuntimeBytes(nCtx)
	if d.TotalRAM <= 0 {
		return FitOK
	}
	// Android keeps ~40% of RAM for the system and foreground services.
	budget := d.TotalRAM * 55 / 100
	switch {
	case need > budget:
		return FitNo
	case need > budget*3/4 || (d.AvailRAM > 0 && need > d.AvailRAM):
		return FitTight
	case need > budget/2 || spec.ParamsM >= 1500:
		return FitOK
	}
	return FitGreat
}
