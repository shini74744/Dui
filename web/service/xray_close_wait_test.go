package service

import (
	"encoding/json"
	"testing"
	"x-ui/xray"
)

func TestDUICloseWaitPolicyRoundTripAndDisable(t *testing.T) {
	template := `{"policy":{"levels":{"0":{"connIdle":300,"bufferSize":64}},"system":{"statsInboundUplink":true}},"routing":{"domainStrategy":"AsIs"}}`
	settings := defaultXrayConnectionPolicy()
	settings.CloseWaitTimeout = 30
	updated, err := UpdateXrayConnectionPolicyTemplate(template, settings)
	if err != nil {
		t.Fatal(err)
	}
	got, err := GetXrayConnectionPolicy(updated)
	if err != nil || got != settings {
		t.Fatalf("roundtrip %#v %v", got, err)
	}
	var runtime xray.Config
	if err := json.Unmarshal([]byte(updated), &runtime); err != nil {
		t.Fatal(err)
	}
	var policy map[string]any
	if err := json.Unmarshal(runtime.Policy, &policy); err != nil {
		t.Fatal(err)
	}
	system := policy["system"].(map[string]any)
	if system["duiCloseWaitTimeout"] != float64(30) || system["statsInboundUplink"] != true {
		t.Fatalf("runtime policy lost fields: %#v", system)
	}
	if policy["levels"].(map[string]any)["0"].(map[string]any)["bufferSize"] != float64(64) {
		t.Fatal("unrelated buffer size changed")
	}
	settings.CloseWaitTimeout = 0
	disabled, err := UpdateXrayConnectionPolicyTemplate(updated, settings)
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]any
	_ = json.Unmarshal([]byte(disabled), &root)
	system = root["policy"].(map[string]any)["system"].(map[string]any)
	if _, exists := system["duiCloseWaitTimeout"]; exists {
		t.Fatal("disabled field must be omitted for old core compatibility")
	}
	if system["statsInboundUplink"] != true {
		t.Fatal("disable erased statistics")
	}
	// Global setting must survive a template without level 0.
	got, err = GetXrayConnectionPolicy(`{"policy":{"system":{"duiCloseWaitTimeout":45}}}`)
	if err != nil || got.CloseWaitTimeout != 45 {
		t.Fatalf("global-only template: %#v %v", got, err)
	}
}

func TestDUICloseWaitPolicyInvalidAndDefault(t *testing.T) {
	got, err := GetXrayConnectionPolicy(`{}`)
	if err != nil || got.CloseWaitTimeout != 0 {
		t.Fatal("old configuration enabled cleanup")
	}
	for _, value := range []int{-1, 601} {
		v := defaultXrayConnectionPolicy()
		v.CloseWaitTimeout = value
		if _, err := UpdateXrayConnectionPolicyTemplate(`{}`, v); err == nil {
			t.Fatalf("accepted %d", value)
		}
	}
	if _, err := GetXrayConnectionPolicy(`{"policy":{"system":{"duiCloseWaitTimeout":0.5}}}`); err == nil {
		t.Fatal("fractional timeout silently rounded")
	}
}
