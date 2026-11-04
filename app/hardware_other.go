//go:build !android

package app

func thermalStatus() ThermalStatus { return ThermalUnknown }
