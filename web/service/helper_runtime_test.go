package service

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
	"x-ui/database/model"
	"x-ui/internal/amneziawg"
	"x-ui/xray"
)

func TestHelperActiveUsersRespectAllDisablePaths(t *testing.T) {
	clients := []model.Client{{ID: "1", Email: "active", Enable: true}, {ID: "2", Email: "disabled", Enable: false}, {ID: "3", Email: "expired", Enable: true, ExpiryTime: time.Now().Add(-time.Hour).UnixMilli()}, {ID: "4", Email: "quota", Enable: true}, {ID: "5", Email: "blocked", Enable: true}}
	cfg, _ := json.Marshal(map[string]any{"clients": clients})
	ib := &model.Inbound{Enable: true, Settings: string(cfg), ClientStats: []xray.ClientTraffic{{Email: "quota", Enable: true, Total: 100, Up: 100}, {Email: "blocked", Enable: false}}}
	active := helperActiveInbound(ib)
	var got struct {
		Clients []model.Client `json:"clients"`
	}
	_ = json.Unmarshal([]byte(active.Settings), &got)
	if len(got.Clients) != 1 || got.Clients[0].Email != "active" {
		t.Fatalf("invalid active list: %+v", got)
	}
	if active.Settings == ib.Settings {
		t.Fatal("active filter missing")
	}
	if !strings.Contains(ib.Settings, "disabled") {
		t.Fatal("source mutated")
	}
	ib.Total = 10
	ib.Down = 10
	if helperActiveInbound(ib).Enable {
		t.Fatal("exhausted parent active")
	}
}

func TestAWGDesiredPreservesNegotiatedOptions(t *testing.T) {
	ob := amneziawg.Obfuscation31{HeaderProtectionKey: "key", ContentPaddingAddition: "20-40", RekeyAfterTime: "120", RekeyTimeout: "5", RejectAfterTime: "180", KeepaliveTimeout: "10", MaxHandshakeAttempts: "18", RandomTrailers: true, DisableCookies: true}
	got := awgDesired(amneziawg.Instance{Obfuscation: ob}).Options
	if got.HeaderProtectionKey != ob.HeaderProtectionKey || got.ContentPaddingAddition != ob.ContentPaddingAddition || got.RekeyAfterTime != ob.RekeyAfterTime || got.RekeyTimeout != ob.RekeyTimeout || got.RejectAfterTime != ob.RejectAfterTime || got.KeepaliveTimeout != ob.KeepaliveTimeout || got.MaxHandshakeAttempts != ob.MaxHandshakeAttempts || !got.RandomTrailers || !got.DisableCookies {
		t.Fatal("AWG client/server options diverged")
	}
}

func TestUnsupportedHelperLimitsAreRejected(t *testing.T) {
	rate := 300.0
	for _, protocol := range []model.Protocol{model.TUIC, model.MTProto, model.AmneziaWG} {
		for _, c := range []model.Client{{Email: "user", SpeedLimitMbps: &rate}, {Email: "user", LimitIP: 2}} {
			raw, _ := json.Marshal(map[string]any{"clients": []model.Client{c}})
			ib := &model.Inbound{Protocol: protocol, Port: 40000, Settings: string(raw)}
			if err := validateHelperInbound(ib); err == nil || !strings.Contains(err.Error(), "不支持") {
				t.Fatalf("unsupported control silently accepted: %s, %v", protocol, err)
			}
		}
	}
}
