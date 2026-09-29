package job

import (
	"encoding/base64"
	"testing"
	"x-ui/database/model"
)

func TestDeviceLimitRestorePreservesRateAndSS2022Keys(t *testing.T) {
	mbps := 300.0
	for _, tc := range []struct {
		cipher string
		size   int
	}{
		{"2022-blake3-aes-128-gcm", 16}, {"2022-blake3-aes-256-gcm", 32}, {"2022-blake3-chacha20-poly1305", 32},
	} {
		client := model.Client{Email: "test", Password: base64.StdEncoding.EncodeToString(make([]byte, tc.size)), SpeedLimitMbps: &mbps, Flow: "xtls-rprx-vision"}
		for _, banned := range []bool{true, false} {
			m, err := deviceLimitAPIUser(client, tc.cipher, banned)
			if err != nil {
				t.Fatal(err)
			}
			if m["level"] != model.DUIRateLevelFlag|uint32(37500000) || m["cipher"] != tc.cipher || m["flow"] != client.Flow {
				t.Fatal("restored user lost rate, cipher or flow")
			}
			key, err := base64.StdEncoding.DecodeString(m["password"].(string))
			if err != nil || len(key) != tc.size {
				t.Fatal("invalid temporary SS2022 key")
			}
			if (m["password"] == client.Password) == banned {
				t.Fatal("incorrect temporary/restored credential")
			}
		}
	}
	client := model.Client{ID: "original-user", Email: "test", Flow: "xtls-rprx-vision", SpeedLimitMbps: &mbps}
	restored, err := deviceLimitAPIUser(client, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if restored["id"] != client.ID || restored["level"] != client.UserRateLevel() {
		t.Fatal("VLESS restore changed identity or rate")
	}
}
