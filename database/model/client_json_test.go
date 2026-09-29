package model

import (
	"encoding/json"
	"testing"
)

func TestLegacyClientTelegramID(t *testing.T) {
	for _, raw := range []string{`{"tgId":"","email":"a","speedLimitMbps":300}`, `{"tgId":"123","email":"a","speedLimitMbps":300}`, `{"tgId":123,"email":"a","speedLimitMbps":300}`} {
		var c Client
		if e := json.Unmarshal([]byte(raw), &c); e != nil {
			t.Fatal(e)
		}
		if c.UserRateLevel() != uint32(1<<31)|37500000 {
			t.Fatal("rate lost")
		}
	}
}
