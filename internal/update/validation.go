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
			Protocol string          `json:"protocol"`
			Settings json.RawMessage `json:"settings"`
		} `json:"outbounds"`
	}
	if json.Unmarshal(data, &config) != nil {
		// Let the candidate reject malformed config rather than guessing settings.
		return false
	}
	for _, outbound := range config.Outbounds {
		if outbound.Protocol != "wireguard" {
			continue
		}
		var settings struct {
			NoKernelTun bool `json:"noKernelTun"`
		}
		// Unrecognized settings on other protocols are irrelevant to WireGuard.
		// Missing or malformed WireGuard settings must never disable isolation.
		if json.Unmarshal(outbound.Settings, &settings) != nil || !settings.NoKernelTun {
			return true
		}
	}
	return false
}
