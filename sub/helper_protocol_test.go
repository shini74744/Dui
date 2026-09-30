package sub

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"
	"x-ui/database/model"
)

func TestHelperSubscriptionExportsOnlySelectedCredential(t *testing.T) {
	cfg := map[string]any{"clients": []model.Client{{ID: "uuid-one", Password: "a:/?#@中文", Email: "one", Enable: true}, {ID: "uuid-two", Password: "NEVER_EXPORT", Email: "two", Enable: true}}, "certificate": "/private/cert.pem", "private_key": "/private/key.pem", "sni": "test.example", "allowInsecure": true}
	b, _ := json.Marshal(cfg)
	ib := &model.Inbound{Protocol: model.TUIC, Listen: "2001:db8::1", Port: 443, Settings: string(b)}
	s := &SubService{remarkModel: "-ieo"}
	link := s.genHelperLink(ib, "one")
	u, err := url.Parse(link)
	if err != nil {
		t.Fatal(err)
	}
	pass, _ := u.User.Password()
	if u.Scheme != "tuic" || u.Host != "[2001:db8::1]:443" || pass != "a:/?#@中文" || u.Query().Get("sni") != "test.example" {
		t.Fatalf("bad URI: %s", link)
	}
	if strings.Contains(link, "NEVER_EXPORT") || strings.Contains(link, "private/") {
		t.Fatal("leaked another user's or server configuration")
	}
	ib.Protocol = model.MTProto
	ib.Settings = `{"clients":[{"email":"one","secret":"eetest"},{"email":"two","secret":"NEVER_EXPORT"}]}`
	link = s.genHelperLink(ib, "one")
	u, err = url.Parse(link)
	if err != nil || u.Scheme != "tg" || u.Query().Get("secret") != "eetest" || u.Query().Get("server") != "2001:db8::1" {
		t.Fatalf("bad MTProto URI")
	}
}
