package xray

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/xtls/xray-core/app/proxyman"
	"github.com/xtls/xray-core/app/proxyman/command"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
)

type duiInboundCapture struct {
	command.UnimplementedHandlerServiceServer
	requests chan *command.AddInboundRequest
}

func (s *duiInboundCapture) AddInbound(_ context.Context, r *command.AddInboundRequest) (*command.AddInboundResponse, error) {
	s.requests <- r
	return &command.AddInboundResponse{}, nil
}

// Set DUI_TEST_CORE to an actual prepared DUI core. Verify the new server-only
// protobuf field survives the CLI and gRPC path even with the panel's older Go SDK.
func TestDUIHotAddUsesInstalledCore(t *testing.T) {
	binary := os.Getenv("DUI_TEST_CORE")
	if binary == "" {
		t.Skip("DUI_TEST_CORE not supplied")
	}
	data, err := os.ReadFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	t.Setenv("XUI_BIN_FOLDER", dir)
	if err := os.WriteFile(filepath.Join(dir, GetBinaryName()), data, 0700); err != nil {
		t.Fatal(err)
	}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := grpc.NewServer()
	capture := &duiInboundCapture{requests: make(chan *command.AddInboundRequest, 2)}
	command.RegisterHandlerServiceServer(s, capture)
	go s.Serve(l)
	t.Cleanup(s.Stop)
	var api XrayAPI
	if err := api.Init(l.Addr().(*net.TCPAddr).Port); err != nil {
		t.Fatal(err)
	}
	defer api.Close()
	for _, strict := range []bool{true, false} {
		cfg := fmt.Sprintf(`{"tag":"dui-test","listen":"127.0.0.1","port":23456,"protocol":"vless","settings":{"clients":[{"id":"82de13e6-2df5-4dd4-b2b0-4c8878b8dc09"}],"decryption":"none"},"streamSettings":{"network":"tcp","security":"reality","realitySettings":{"target":"example.com:443","serverNames":["example.com"],"privateKey":"MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY","shortIds":["12345678"],"duiRequireHybridKeyShare":%t}}}`, strict)
		if err := api.AddInbound([]byte(cfg)); err != nil {
			t.Fatal(err)
		}
		req := <-capture.requests
		var receiver proxyman.ReceiverConfig
		if err := proto.Unmarshal(req.Inbound.ReceiverSettings.Value, &receiver); err != nil {
			t.Fatal(err)
		}
		var actual bool
		for _, security := range receiver.StreamSettings.SecuritySettings {
			if security.Type != "xray.transport.internet.reality.Config" {
				continue
			}
			wire := security.Value
			for len(wire) > 0 {
				field, kind, n := protowire.ConsumeTag(wire)
				if n < 0 {
					t.Fatal("bad tag")
				}
				wire = wire[n:]
				if field == 14 {
					v, n := protowire.ConsumeVarint(wire)
					if n < 0 {
						t.Fatal("bad mode")
					}
					actual = v == 1
				}
				n = protowire.ConsumeFieldValue(field, kind, wire)
				if n < 0 {
					t.Fatal("bad field")
				}
				wire = wire[n:]
			}
		}
		if actual != strict {
			t.Fatalf("hot-added mode=%v want=%v", actual, strict)
		}
	}
	if err := api.AddInbound([]byte(`[]`)); err == nil {
		t.Fatal("accepted non-object")
	}
	if err := api.AddInbound([]byte(`{"protocol":"invalid"}`)); err == nil {
		t.Fatal("ignored core error")
	}
	api.Close()
	if err := api.AddInbound([]byte(`{}`)); err == nil {
		t.Fatal("accepted closed API")
	}
}
