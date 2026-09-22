package service

import (
	"encoding/json"
	"testing"
)

func TestXrayConnectionPolicyRoundTrip(t *testing.T) {
	input := `{
	  "policy": {
	    "levels": {
	      "0": {
	        "handshake": 4,
	        "connIdle": 300,
	        "uplinkOnly": 0,
	        "downlinkOnly": 0,
	        "statsUserUplink": true
	      }
	    },
	    "system": {"statsInboundUplink": true}
	  },
	  "outbounds": []
	}`

	want := XrayConnectionPolicy{Handshake: 8, ConnIdle: 600, UplinkOnly: 2, DownlinkOnly: 3}
	updated, err := UpdateXrayConnectionPolicyTemplate(input, want)
	if err != nil {
		t.Fatal(err)
	}
	got, err := GetXrayConnectionPolicy(updated)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("got %#v, want %#v", got, want)
	}
	var root map[string]any
	if err := json.Unmarshal([]byte(updated), &root); err != nil {
		t.Fatal(err)
	}
	policy := root["policy"].(map[string]any)
	system := policy["system"].(map[string]any)
	if system["statsInboundUplink"] != true {
		t.Fatalf("unrelated policy field was lost: %#v", system)
	}
	level0 := policy["levels"].(map[string]any)["0"].(map[string]any)
	if level0["statsUserUplink"] != true || level0["statsUserDownlink"] != true || level0["statsUserOnline"] != true {
		t.Fatalf("required stats flags missing: %#v", level0)
	}
}

func TestXrayConnectionPolicyValidation(t *testing.T) {
	_, err := UpdateXrayConnectionPolicyTemplate(`{"policy":{}}`, XrayConnectionPolicy{
		Handshake: 0, ConnIdle: 300, UplinkOnly: 0, DownlinkOnly: 0,
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
}
