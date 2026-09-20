package job

import "testing"

func TestActivityDetailLineRegexCompatibility(t *testing.T) {
	tests := []struct {
		name       string
		line       string
		sourceIP   string
		sourcePort string
		network    string
		destHost   string
		destPort   string
		route      string
		email      string
	}{
		{
			name:       "legacy shadowsocks without network prefix",
			line:       "2026/09/20 17:01:03.619549 from 177.0.143.8:59906 accepted 23.193.119.157:443 [inbound-44569 >> direct] email: TG@@Clot88",
			sourceIP:   "177.0.143.8",
			sourcePort: "59906",
			network:    "",
			destHost:   "23.193.119.157",
			destPort:   "443",
			route:      "inbound-44569 >> direct",
			email:      "TG@@Clot88",
		},
		{
			name:       "current format with tcp destination prefix",
			line:       "2026/09/19 19:08:01.123456 from 39.171.179.76:54321 accepted tcp:www.youtube.com:443 [inbound-46038 -> direct] email: user@example.com",
			sourceIP:   "39.171.179.76",
			sourcePort: "54321",
			network:    "tcp",
			destHost:   "www.youtube.com",
			destPort:   "443",
			route:      "inbound-46038 -> direct",
			email:      "user@example.com",
		},
		{
			name:       "explicit udp source and destination prefix",
			line:       "2026/09/20 17:01:03 from udp:[2001:db8::1]:53000 accepted udp:1.1.1.1:53 [inbound-53000 >> direct] email: dns-user",
			sourceIP:   "[2001:db8::1]",
			sourcePort: "53000",
			network:    "udp",
			destHost:   "1.1.1.1",
			destPort:   "53",
			route:      "inbound-53000 >> direct",
			email:      "dns-user",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := activityDetailLineRegex.FindStringSubmatch(tt.line)
			if len(m) < 10 {
				t.Fatalf("line did not match: %q", tt.line)
			}
			if got := m[3]; got != tt.sourceIP {
				t.Fatalf("source IP = %q, want %q", got, tt.sourceIP)
			}
			if got := m[4]; got != tt.sourcePort {
				t.Fatalf("source port = %q, want %q", got, tt.sourcePort)
			}
			if got := m[5]; got != tt.network {
				t.Fatalf("network = %q, want %q", got, tt.network)
			}
			if got := m[6]; got != tt.destHost {
				t.Fatalf("destination host = %q, want %q", got, tt.destHost)
			}
			if got := m[7]; got != tt.destPort {
				t.Fatalf("destination port = %q, want %q", got, tt.destPort)
			}
			if got := m[8]; got != tt.route {
				t.Fatalf("route = %q, want %q", got, tt.route)
			}
			if got := m[9]; got != tt.email {
				t.Fatalf("email = %q, want %q", got, tt.email)
			}
		})
	}
}
