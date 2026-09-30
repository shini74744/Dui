package sub

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"
	"x-ui/database/model"
)

func TestHysteriaSubscriptionIdentityAndPrivacy(t *testing.T) {
	i := &model.Inbound{Protocol: model.Hysteria, Port: 14443, Remark: "测试", Settings: `{"version":2,"clients":[{"email":"first","auth":"a:/?#@中文"},{"email":"second","auth":"other-secret"}]}`, StreamSettings: `{"network":"hysteria","security":"tls","hysteriaSettings":{"version":2,"masquerade":{"type":"file","dir":"private-directory"}},"tlsSettings":{"serverName":"test.example","certificates":[{"keyFile":"private.key"}],"settings":{"allowInsecure":true,"fingerprint":"chrome"}}}`}
	s := SubService{address: "2001:db8::1", remarkModel: "-ie"}
	u, err := url.Parse(s.genHysteriaLink(i, "first"))
	if err != nil {
		t.Fatal(err)
	}
	if u.Scheme != "hysteria2" || u.Host != "[2001:db8::1]:14443" || u.User.Username() != "a:/?#@中文" || u.Query().Get("sni") != "test.example" || u.Query().Get("insecure") != "1" {
		t.Fatal("link fields changed")
	}
	if strings.Contains(u.String(), "other-secret") {
		t.Fatal("other user's auth exported")
	}
	js := SubJsonService{}
	stream := js.streamData(i.StreamSettings)
	i.Listen = "test.example"
	data := js.genHysteria(i, stream, model.Client{Auth: "a:/?#@中文"})
	for _, private := range []string{"private-directory", "private.key", "other-secret", "fingerprint", "certificates"} {
		if strings.Contains(string(data), private) {
			t.Fatal("server-only field exported:", private)
		}
	}
	var result map[string]any
	if json.Unmarshal(data, &result) != nil || result["protocol"] != "hysteria" {
		t.Fatal("wrong outbound")
	}
}
