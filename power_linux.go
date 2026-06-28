//go:build linux

package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func getPowerStatus() (bool, int, bool) {
	acOnline := true
	files, err := os.ReadDir("/sys/class/power_supply")
	if err != nil {
		return false, 0, true
	}

	hasBattery := false
	batteryPct := 0

	for _, f := range files {
		name := f.Name()
		if strings.HasPrefix(name, "AC") || strings.HasPrefix(name, "ADP") {
			if data, err := os.ReadFile(filepath.Join("/sys/class/power_supply", name, "online")); err == nil {
				val := strings.TrimSpace(string(data))
				if val == "0" {
					acOnline = false
				}
			}
		} else if strings.HasPrefix(name, "BAT") {
			if data, err := os.ReadFile(filepath.Join("/sys/class/power_supply", name, "capacity")); err == nil {
				val := strings.TrimSpace(string(data))
				if pct, err := strconv.Atoi(val); err == nil {
					hasBattery = true
					batteryPct = pct
				}
			}
		}
	}
	return hasBattery, batteryPct, acOnline
}
