//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

type systemPowerStatus struct {
	ACLineStatus        byte
	BatteryFlag         byte
	BatteryLifePercent  byte
	SystemStatusFlag    byte
	BatteryLifeTime     uint32
	BatteryFullLifeTime uint32
}

func getPowerStatus() (bool, int, bool) {
	var status systemPowerStatus
	mod := syscall.NewLazyDLL("kernel32.dll")
	proc := mod.NewProc("GetSystemPowerStatus")
	ret, _, _ := proc.Call(uintptr(unsafe.Pointer(&status)))
	if ret == 0 {
		return false, 0, true
	}
	acOnline := status.ACLineStatus == 1
	if status.BatteryLifePercent != 255 && status.BatteryLifePercent <= 100 {
		hasBattery := status.BatteryFlag != 128
		return hasBattery, int(status.BatteryLifePercent), acOnline
	}
	return false, 0, acOnline
}
