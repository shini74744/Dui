package mtproto

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"x-ui/database/model"
	"x-ui/xray"
)

func TestRestartRestoresPersistedQuotaWithoutReloadChurn(t *testing.T) {
	t.Setenv("XUI_BIN_FOLDER", t.TempDir())
	if err := os.MkdirAll(configDir(), 0700); err != nil {
		t.Fatal(err)
	}
	ib := &model.Inbound{Id: 71, Protocol: model.MTProto, Settings: `{"clients":[{"email":"one","secret":"eetest","enable":true,"totalGB":1000}]}`, ClientStats: []xray.ClientTraffic{{Email: "one", Up: 300, Down: 200}}}
	inst, ok := InstanceFromInbound(ib)
	if !ok || inst.Secrets[0].UsedBytes != 500 {
		t.Fatal("persisted usage missing")
	}
	if err := writeUsageState(inst); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(usagePathForID(ib.Id))
	if err != nil {
		t.Fatal(err)
	}
	var usage map[string]map[string]int64
	if err := json.Unmarshal(data, &usage); err != nil {
		t.Fatal(err)
	}
	if usage["one"]["quota_used"] != 500 {
		t.Fatal("restart reset quota")
	}
	fp := inst.secretsFingerprint()
	inst.Secrets[0].UsedBytes += 100
	if inst.secretsFingerprint() != fp {
		t.Fatal("traffic tick must not reload secrets")
	}
	inst.Secrets[0].UsedBytes = 0
	if err := writeUsageState(inst); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(usagePathForID(ib.Id))
	_ = json.Unmarshal(data, &usage)
	if usage["one"]["quota_used"] != 0 {
		t.Fatal("offline reset lost")
	}
	if filepath.Dir(usagePathForID(ib.Id)) != configDir() {
		t.Fatal("usage outside helper directory")
	}
}
