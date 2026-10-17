package service

import (
	"encoding/json"
	"errors"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"path/filepath"
	"reflect"
	"testing"
	"x-ui/database/model"
)

func TestUpdateInboundRemarkPreservesAllOtherColumns(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "metadata.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err = db.AutoMigrate(&model.Inbound{}); err != nil {
		t.Fatal(err)
	}
	for i, protocol := range []model.Protocol{"vless", "shadowsocks", "tuic", "amneziawg"} {
		old := model.Inbound{Id: i + 1, UserId: 7, Remark: "old", Protocol: protocol, Tag: string(protocol), Port: 42000 + i,
			Up: 1234, Down: 5678, Total: 9000, AllTime: 10000, Enable: true, ExpiryTime: 123456, DeviceLimit: 3, TrafficReset: "monthly",
			LastTrafficResetTime: 456, Settings: `{"clients":[{"id":"keep","email":"alpha","password":"keep","subId":"keep"}],"extension":true}`, StreamSettings: `{"network":"tcp"}`, Sniffing: `{"enabled":true}`}
		if err = db.Create(&old).Error; err != nil {
			t.Fatal(err)
		}
		if err = updateInboundRemark(db, old.Id, 99, "unauthorized"); !errors.Is(err, gorm.ErrRecordNotFound) {
			t.Fatal(err)
		}
		if err = updateInboundRemark(db, old.Id, 7, "new name"); err != nil {
			t.Fatal(err)
		}
		var got model.Inbound
		if err = db.First(&got, old.Id).Error; err != nil {
			t.Fatal(err)
		}
		old.Remark = "new name"
		if !reflect.DeepEqual(got, old) {
			t.Fatalf("changed non-remark fields for %s", protocol)
		}
		if err = updateInboundRemark(db, old.Id, 7, ""); err != nil {
			t.Fatal(err)
		}
		db.First(&got, old.Id)
		if got.Remark != "" {
			t.Fatal("empty remark was not saved")
		}
	}
	if err = updateInboundRemark(db, 999, 7, "missing"); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatal(err)
	}
}
func TestClientRouteReferencesMatchCoreSemantics(t *testing.T) {
	inbounds := []model.Inbound{{Id: 1, Tag: "inbound-one", Settings: `{"clients":[{"email":"Alpha"},{"email":"alpha"},{"email":""}]}`},
		{Id: 2, Tag: "inbound-two", Settings: `{"clients":[{"email":"Alpha"}]}`}}
	raw := `{"routing":{"rules":[
 {"outboundTag":"direct"},
 {"user":["Alpha","Alpha"],"outboundTag":"primary","fallbackTag":"backup"},
 {"user":["regexp:^Al.*"],"balancerTag":"group"},
 {"user":["Alpha"],"inboundTag":["inbound-two"],"outboundTag":"scoped"},
 {"user":["regexp:["],"outboundTag":"bad"},
 {"user":["alpha"],"outboundTag":"case-sensitive"},
 {"user":["regexp:^no$","regexp:Alpha$"],"ruleTag":"named","outboundTag":"regex"},
 {"user":["regexp:"] ,"outboundTag":"literal"}
 ]}}`
	got, err := clientRouteReferences(raw, inbounds)
	if err != nil {
		t.Fatal(err)
	}
	expected := [][]int{{2, 3, 7}, {6}, {2, 3, 4, 7}}
	if len(got) != len(expected) {
		t.Fatal(got)
	}
	for i, ref := range got {
		var indexes []int
		for _, rule := range ref.Rules {
			indexes = append(indexes, rule.Index)
		}
		if !reflect.DeepEqual(indexes, expected[i]) {
			t.Fatalf("%+v", ref)
		}
	}
	if got[0].Rules[0].Regexp || !got[0].Rules[1].Regexp || got[0].Rules[0].FallbackTag != "backup" || got[0].Rules[1].BalancerTag != "group" {
		t.Fatal(got)
	}
	before, _ := json.Marshal(inbounds)
	if _, err = clientRouteReferences(raw, inbounds); err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(inbounds)
	if string(before) != string(after) {
		t.Fatal("reference lookup changed inbound data")
	}
	for _, invalid := range []string{"", `{"routing":{"rules":[{"user":123}]}}`} {
		if _, err = clientRouteReferences(invalid, inbounds); err == nil {
			t.Fatal("invalid config reported as no references")
		}
	}
}
