package xray

import (
	"context"
	command "github.com/xtls/xray-core/app/proxyman/command"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/encoding/protowire"
	"testing"
)

type captureUserClient struct {
	command.HandlerServiceClient
	request *command.AlterInboundRequest
}

func (c *captureUserClient) AlterInbound(ctx context.Context, in *command.AlterInboundRequest, opts ...grpc.CallOption) (*command.AlterInboundResponse, error) {
	c.request = in
	return &command.AlterInboundResponse{}, nil
}
func TestDUIUserAPILevelAndSS2022Account(t *testing.T) {
	c := &captureUserClient{}
	var client command.HandlerServiceClient = c
	a := &XrayAPI{HandlerServiceClient: &client}
	level := uint32(1<<31) | 37500000
	err := a.AddUser("shadowsocks", "test", map[string]any{"email": "test", "password": "test-key", "cipher": "2022-blake3-aes-128-gcm", "level": level})
	if err != nil {
		t.Fatal(err)
	}
	m, err := c.request.Operation.GetInstance()
	if err != nil {
		t.Fatal(err)
	}
	u := m.(*command.AddUserOperation).User
	if u.Level != level || u.Account.Type != "xray.proxy.shadowsocks_2022.Account" {
		t.Fatal("incorrect wire user")
	}
	field, kind, n := protowire.ConsumeTag(u.Account.Value)
	key, _ := protowire.ConsumeString(u.Account.Value[n:])
	if field != 1 || kind != protowire.BytesType || key != "test-key" {
		t.Fatal("wrong SS2022 Account field")
	}
}
