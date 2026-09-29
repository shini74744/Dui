package service

import (
	"encoding/json"
	"testing"
	"x-ui/database/model"
)

func TestUserRateParsingAndTLSCompatibility(t *testing.T) {
	c, e := (&InboundService{}).GetClients(&model.Inbound{Settings: `{"method":"2022-blake3-aes-128-gcm","clients":[{"email":"test","speedLimitMbps":300}]}`})
	if e != nil || len(c) != 1 || c[0].UserRateLevel() != uint32(1<<31)|37500000 {
		t.Fatalf("rate parsing: %v %v", c, e)
	}
	var value any
	json.Unmarshal([]byte(`{"outbounds":[{"streamSettings":{"tlsSettings":{"allowInsecure":true,"verifyPeerCertInNames":["example.com"],"echForceQuery":"none","pinnedPeerCertSha256":"pin"}}}]}`), &value)
	normalizeDUITLS(value)
	b, _ := json.Marshal(value)
	var again map[string]any
	json.Unmarshal(b, &again)
	v := again["outbounds"].([]any)[0].(map[string]any)["streamSettings"].(map[string]any)["tlsSettings"].(map[string]any)
	if v["verifyPeerCertByName"] != "example.com" || v["allowInsecure"] != true || v["pinnedPeerCertSha256"] != "pin" {
		t.Fatal(v)
	}
	if _, ok := v["echForceQuery"]; ok {
		t.Fatal("retired field retained")
	}
}
