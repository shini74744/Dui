package model

import (
	"encoding/json"
	"math"
	"testing"
)

func TestDUIUserRate(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		want uint32
	}{{`{"email":"a","speedLimitMbps":300}`, DUIRateLevelFlag | 37500000}, {`{"email":"a","speedLimit":1024}`, DUIRateLevelFlag | 1048576}, {`{"email":"a","speedLimit":1024,"speedLimitMbps":0}`, 0}} {
		var c Client
		if err := json.Unmarshal([]byte(tc.raw), &c); err != nil {
			t.Fatal(err)
		}
		if err := c.ValidateUserRate(); err != nil {
			t.Fatal(err)
		}
		if c.UserRateLevel() != tc.want {
			t.Fatalf("%s: %d != %d", tc.raw, c.UserRateLevel(), tc.want)
		}
	}
	for _, v := range []float64{-1, 10001, math.Inf(1), math.NaN()} {
		c := Client{Email: "a", SpeedLimitMbps: &v}
		if c.ValidateUserRate() == nil {
			t.Fatal("accepted invalid speed", v)
		}
	}
}

func TestDUIHotInboundRateLevelsPreserveKeys(t *testing.T) {
	for _, protocol := range []Protocol{VMESS, VLESS, Trojan, Shadowsocks} {
		original := `{"decryption":"existing-decryption","encryption":"existing-encryption","clients":[{"id":"same-user","email":"a","password":"same-key","flow":"xtls-rprx-vision","speedLimitMbps":300},{"email":"b","speedLimit":1024},{"email":"c","speedLimit":1024,"speedLimitMbps":0}]}`
		in := Inbound{Protocol: protocol, Settings: original}
		var settings struct {
			Decryption, Encryption string
			Clients                []struct {
				ID, Password, Flow string
				Level              uint32
			}
		}
		if err := json.Unmarshal(in.GenXrayInboundConfig().Settings, &settings); err != nil {
			t.Fatal(err)
		}
		if settings.Clients[0].Level != DUIRateLevelFlag|37500000 || settings.Clients[1].Level != DUIRateLevelFlag|1048576 || settings.Clients[2].Level != 0 {
			t.Fatal("hot inbound rate conversion failed")
		}
		if settings.Decryption != "existing-decryption" || settings.Encryption != "existing-encryption" || settings.Clients[0].ID != "same-user" || settings.Clients[0].Password != "same-key" || settings.Clients[0].Flow != "xtls-rprx-vision" || in.Settings != original {
			t.Fatal("hot rate conversion changed saved authentication")
		}
	}
	for _, raw := range []string{`{"clients":[null]}`, `{"clients":"invalid"}`, `not-json`} {
		in := Inbound{Protocol: VLESS, Settings: raw}
		if in.xraySettingsWithRates() != raw {
			t.Fatal("invalid configuration was silently rewritten")
		}
	}
}
