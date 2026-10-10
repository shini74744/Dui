package update

import (
	"encoding/json"
	"strconv"
	"strings"
)

// DUI vx-26.9+ checks WireGuard in userspace without changing NoKernelTun in
// the saved config. Older binaries need an isolated namespace during -test.
func needsWireGuardIsolation(data []byte, version string) bool {
	if strings.HasPrefix(version, "vx-") {
		parts := strings.Split(strings.TrimPrefix(version, "vx-"), ".")
		if len(parts) == 2 {
			major, e1 := strconv.Atoi(parts[0])
			minor, e2 := strconv.Atoi(parts[1])
			if e1 == nil && e2 == nil && (major > 26 || major == 26 && minor >= 9) {
				return false
			}
		}
	}
	var config struct {
		Outbounds []struct {
			Protocol string `json:"protocol"`
			Settings struct {
				NoKernelTun bool `json:"noKernelTun"`
			} `json:"settings"`
		} `json:"outbounds"`
	}
	if json.Unmarshal(data, &config) != nil {
		// Let the candidate reject malformed config rather than guessing settings.
		return false
	}
	for _, outbound := range config.Outbounds {
		if outbound.Protocol == "wireguard" && !outbound.Settings.NoKernelTun {
			return true
		}
	}
	return false
}
