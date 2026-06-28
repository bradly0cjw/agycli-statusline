//go:build !linux && !darwin && !windows

package main

func getPowerStatus() (bool, int, bool) {
	return false, 0, true
}
