package model

import (
	"encoding/json"
	"strings"
	"testing"
)

func nativeHysteria() Inbound {
	return Inbound{Protocol: Hysteria, Port: 14443, Settings: `{"version":2,"clients":[{"auth":"test-auth","email":"test","enable":true,"speedLimitMbps":300}]}`, StreamSettings: `{"network":"hysteria","security":"tls","hysteriaSettings":{"version":2},"tlsSettings":{"certificates":[{"certificateFile":"test.crt","keyFile":"test.key"}],"alpn":["h3"]}}`}
}
func TestNativeHysteriaRateAndValidation(t *testing.T) {
	i := nativeHysteria()
	if err := i.ValidateNativeProtocol(); err != nil {
		t.Fatal(err)
	}
	var settings struct {
		Clients []struct {
			Auth  string `json:"auth"`
			Level uint32 `json:"level"`
		} `json:"clients"`
	}
	if err := json.Unmarshal(i.GenXrayInboundConfig().Settings, &settings); err != nil {
		t.Fatal(err)
	}
	if len(settings.Clients) != 1 || settings.Clients[0].Auth != "test-auth" || settings.Clients[0].Level != DUIRateLevelFlag|37500000 {
		t.Fatal("hot load lost auth/rate")
	}
	for name, mutate := range map[string]func(*Inbound){
		"wrong version": func(i *Inbound) { i.Settings = strings.Replace(i.Settings, `"version":2`, `"version":1`, 1) },
		"no TLS":        func(i *Inbound) { i.StreamSettings = strings.Replace(i.StreamSettings, `"tls"`, `"none"`, 1) },
		"wrong transport": func(i *Inbound) {
			i.StreamSettings = strings.Replace(i.StreamSettings, `"network":"hysteria"`, `"network":"tcp"`, 1)
		},
		"missing auth":  func(i *Inbound) { i.Settings = strings.Replace(i.Settings, "test-auth", "", 1) },
		"missing email": func(i *Inbound) { i.Settings = strings.Replace(i.Settings, `"email":"test"`, `"email":""`, 1) },
		"global bypass": func(i *Inbound) {
			i.StreamSettings = strings.Replace(i.StreamSettings, `"version":2`, `"version":2,"auth":"global"`, 1)
		},
		"wrong ALPN": func(i *Inbound) { i.StreamSettings = strings.Replace(i.StreamSettings, `"h3"`, `"h2"`, 1) },
	} {
		t.Run(name, func(t *testing.T) {
			i := nativeHysteria()
			mutate(&i)
			if i.ValidateNativeProtocol() == nil {
				t.Fatal("invalid config accepted")
			}
		})
	}
}
func TestMixedAuthenticationBoundaries(t *testing.T) {
	for _, test := range []struct {
		listen, settings string
		valid            bool
	}{
		{"127.0.0.1", `{"auth":"noauth"}`, true}, {"::1", `{"auth":"noauth"}`, true},
		{"", `{"auth":"noauth"}`, false}, {"0.0.0.0", `{"auth":"noauth"}`, false},
		{"0.0.0.0", `{"auth":"password","accounts":[{"user":"a","pass":"b"}]}`, true},
		{"127.0.0.1", `{"auth":"password","accounts":[]}`, false},
		{"127.0.0.1", `{"auth":"password","accounts":[{"user":"a","pass":"b"},{"user":"a","pass":"c"}]}`, false},
	} {
		i := Inbound{Protocol: Mixed, Listen: test.listen, Port: 1080, Settings: test.settings}
		if (i.ValidateNativeProtocol() == nil) != test.valid {
			t.Errorf("unexpected validation for listen=%s", test.listen)
		}
	}
}
func TestTunInterfaceAndRouteValidation(t *testing.T) {
	good := `{"name":"dui0","mtu":1500,"gateway":["172.31.255.1/30"],"autoSystemRoutingTable":[],"autoOutboundsInterface":""}`
	i := Inbound{Protocol: TUN, Port: 1234, Listen: "0.0.0.0", Settings: good}
	if err := i.ValidateNativeProtocol(); err != nil {
		t.Fatal(err)
	}
	if i.Port != 0 || i.Listen != "" || i.Tag != "inbound-tun-dui0" {
		t.Fatal("TUN retained socket identity")
	}
	for _, bad := range []string{strings.Replace(good, "dui0", "../../eth0", 1), strings.Replace(good, "172.31.255.1/30", "bad", 1), strings.Replace(good, "1500", "0", 1), strings.Replace(good, `"autoSystemRoutingTable":[]`, `"autoSystemRoutingTable":["0.0.0.0/0"]`, 1)} {
		i.Settings = bad
		if i.ValidateNativeProtocol() == nil {
			t.Fatal("invalid TUN accepted")
		}
	}
}
