package xray

import (
	"github.com/xtls/xray-core/app/proxyman/command"
	"google.golang.org/protobuf/encoding/protowire"
	"testing"
)

func TestHysteriaHotUserWire(t *testing.T) {
	c := &captureUserClient{}
	var client command.HandlerServiceClient = c
	api := &XrayAPI{HandlerServiceClient: &client}
	for _, auth := range []string{"test:password", "第二个用户"} {
		if err := api.AddUser("hysteria", "test", map[string]any{"auth": auth, "email": "a", "level": uint32(1<<31) | 37500000}); err != nil {
			t.Fatal(err)
		}
		op, err := c.request.Operation.GetInstance()
		if err != nil {
			t.Fatal(err)
		}
		user := op.(*command.AddUserOperation).User
		field, kind, n := protowire.ConsumeTag(user.Account.Value)
		value, _ := protowire.ConsumeString(user.Account.Value[n:])
		if user.Account.Type != "xray.proxy.hysteria.account.Account" || user.Level != uint32(1<<31)|37500000 || field != 1 || kind != protowire.BytesType || value != auth {
			t.Fatal("incorrect Hysteria account encoding")
		}
	}
	if api.AddUser("hysteria", "test", map[string]any{"auth": ""}) == nil {
		t.Fatal("empty auth accepted")
	}
}
