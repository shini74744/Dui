package update

import "testing"

func TestWireGuardValidationCompatibility(t *testing.T) {
	for _, tc := range []struct {
		name, data, version string
		want                bool
	}{
		{"old implicit kernel", "{\"outbounds\":[{\"protocol\":\"wireguard\",\"settings\":{}}]}", "vx-26.8", true},
		{"old explicit kernel", "{\"outbounds\":[{\"protocol\":\"wireguard\",\"settings\":{\"noKernelTun\":false}}]}", "vx-26.4", true},
		{"upstream", "{\"outbounds\":[{\"protocol\":\"wireguard\"}]}", "26.9.9", true},
		{"old userspace", "{\"outbounds\":[{\"protocol\":\"wireguard\",\"settings\":{\"noKernelTun\":true}}]}", "vx-26.8", false},
		{"new compatibility", "{\"outbounds\":[{\"protocol\":\"wireguard\"}]}", "vx-26.9", false},
		{"new major", "{\"outbounds\":[{\"protocol\":\"wireguard\"}]}", "vx-27.0", false},
		{"ordinary", "{\"outbounds\":[{\"protocol\":\"freedom\"}]}", "vx-26.4", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := needsWireGuardIsolation([]byte(tc.data), tc.version); got != tc.want {
				t.Fatalf("isolation = %v, want %v", got, tc.want)
			}
		})
	}
}
