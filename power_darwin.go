//go:build darwin

package main

import (
	"bytes"
	"os/exec"
	"regexp"
	"strconv"
)

func getPowerStatus() (bool, int, bool) {
	cmd := exec.Command("pmset", "-g", "batt")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return false, 0, true
	}
	output := out.String()
	acOnline := bytes.Contains(out.Bytes(), []byte("AC Power"))

	re := regexp.MustCompile(`(\d+)%`)
	match := re.FindStringSubmatch(output)
	if len(match) > 1 {
		if pct, err := strconv.Atoi(match[1]); err == nil {
			return true, pct, acOnline
		}
	}
	return false, 0, acOnline
}
