package service

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"x-ui/xray"
)

func TestApplyXraySocketPolicyToConfig(t *testing.T) {
	oldPath := xraySocketPolicyPath
	defer func() { xraySocketPolicyPath = oldPath }()
	xraySocketPolicyPath = filepath.Join(t.TempDir(), "xray_socket_policy.json")

	p := XraySocketPolicy{
		Enabled: true, ApplyInbound: true, ApplyOutbound: true,
		TCPKeepAliveIdle: 300, TCPKeepAliveInterval: 30, TCPUserTimeout: 15000,
		TCPFastOpenMode: "on", TCPMaxSeg: 1400, TCPWindowClamp: 65535,
		TCPMptcpMode: "inherit", V6OnlyMode: "inherit",
	}
	if err := SaveXraySocketPolicy(p); err != nil {
		t.Fatal(err)
	}
	cfg := &xray.Config{
		InboundConfigs: []xray.InboundConfig{
			{Tag: "api"},
			{Tag: "in-1", StreamSettings: []byte(`{"network":"tcp","sockopt":{"mark":7}}`)},
		},
		OutboundConfigs: []byte(`[
			{"tag":"direct","protocol":"freedom","streamSettings":{"sockopt":{"domainStrategy":"UseIPv4"}}},
			{"tag":"blocked","protocol":"blackhole","settings":{}}
		]`),
	}
	if err := applyXraySocketPolicy(cfg); err != nil {
		t.Fatal(err)
	}

	if len(cfg.InboundConfigs[0].StreamSettings) != 0 {
		t.Fatalf("api inbound must not be modified: %s", cfg.InboundConfigs[0].StreamSettings)
	}
	var inboundStream map[string]any
	if err := json.Unmarshal(cfg.InboundConfigs[1].StreamSettings, &inboundStream); err != nil {
		t.Fatal(err)
	}
	inSock := inboundStream["sockopt"].(map[string]any)
	if int(inSock["tcpKeepAliveIdle"].(float64)) != 300 || inSock["tcpFastOpen"] != true {
		t.Fatalf("inbound socket policy missing: %#v", inSock)
	}
	if int(inSock["mark"].(float64)) != 7 {
		t.Fatalf("existing inbound sockopt was overwritten: %#v", inSock)
	}
	var outbounds []map[string]any
	if err := json.Unmarshal(cfg.OutboundConfigs, &outbounds); err != nil {
		t.Fatal(err)
	}
	outStream := outbounds[0]["streamSettings"].(map[string]any)
	outSock := outStream["sockopt"].(map[string]any)
	if int(outSock["tcpUserTimeout"].(float64)) != 15000 {
		t.Fatalf("outbound socket policy missing: %#v", outSock)
	}
	if outSock["domainStrategy"] != "UseIPv4" {
		t.Fatalf("existing outbound sockopt was overwritten: %#v", outSock)
	}
	if _, ok := outbounds[1]["streamSettings"]; ok {
		t.Fatalf("blackhole outbound must not be modified: %#v", outbounds[1])
	}
}

func TestXraySocketPolicyDisabledDoesNothing(t *testing.T) {
	oldPath := xraySocketPolicyPath
	defer func() { xraySocketPolicyPath = oldPath }()
	xraySocketPolicyPath = filepath.Join(t.TempDir(), "xray_socket_policy.json")
	if err := SaveXraySocketPolicy(XraySocketPolicy{
		Enabled: false, ApplyInbound: true, ApplyOutbound: true,
		TCPKeepAliveIdle: 300, TCPFastOpenMode: "inherit", TCPMptcpMode: "inherit", V6OnlyMode: "inherit",
	}); err != nil {
		t.Fatal(err)
	}
	original := []byte(`{"network":"tcp"}`)
	cfg := &xray.Config{InboundConfigs: []xray.InboundConfig{{Tag: "in-1", StreamSettings: original}}}
	if err := applyXraySocketPolicy(cfg); err != nil {
		t.Fatal(err)
	}
	if string(cfg.InboundConfigs[0].StreamSettings) != string(original) {
		t.Fatalf("disabled policy changed config: %s", cfg.InboundConfigs[0].StreamSettings)
	}
}

func TestXraySocketPolicyRangeValidation(t *testing.T) {
	err := validateXraySocketPolicy(XraySocketPolicy{
		Enabled: true, ApplyInbound: true, ApplyOutbound: true,
		TCPKeepAliveIdle: 1,
		TCPFastOpenMode:  "inherit", TCPMptcpMode: "inherit", V6OnlyMode: "inherit",
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestXraySocketDirectionSpecificFields(t *testing.T) {
	p := XraySocketPolicy{
		Interface:       "eth-test",
		V6OnlyMode:      "on",
		TCPFastOpenMode: "inherit",
		TCPMptcpMode:    "inherit",
	}
	inbound := map[string]any{}
	applySocketPolicyToStream(inbound, p, true)
	inSock := inbound["sockopt"].(map[string]any)
	if _, ok := inSock["interface"]; ok {
		t.Fatalf("interface must not be injected into inbound: %#v", inSock)
	}
	if inSock["v6only"] != true {
		t.Fatalf("v6only must be injected into inbound: %#v", inSock)
	}

	outbound := map[string]any{}
	applySocketPolicyToStream(outbound, p, false)
	outSock := outbound["sockopt"].(map[string]any)
	if outSock["interface"] != "eth-test" {
		t.Fatalf("interface must be injected into outbound: %#v", outSock)
	}
	if _, ok := outSock["v6only"]; ok {
		t.Fatalf("v6only must not be injected into outbound: %#v", outSock)
	}
}
