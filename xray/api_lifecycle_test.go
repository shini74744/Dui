package xray

import (
	"context"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/xtls/xray-core/app/proxyman/command"
	stats "github.com/xtls/xray-core/app/stats/command"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
)

func TestXrayAPILifecycleReinitAndClose(t *testing.T) {
	var api XrayAPI
	defer api.Close()
	for i := 0; i < 100; i++ {
		if err := api.Init(1); err != nil {
			t.Fatal(err)
		}
		old := api.grpcClient
		if err := api.Init(2); err != nil {
			t.Fatal(err)
		}
		if old.GetState() != connectivity.Shutdown {
			t.Fatal("reinitialization leaked previous client")
		}
		current := api.grpcClient
		if err := api.Init(0); err == nil {
			t.Fatal("invalid port accepted")
		}
		if api.grpcClient != current || current.GetState() == connectivity.Shutdown {
			t.Fatal("invalid initialization destroyed active client")
		}
		api.Close()
		api.Close()
		if current.GetState() != connectivity.Shutdown || api.grpcClient != nil {
			t.Fatal("close retained current client")
		}
	}
	if err := api.DelInbound("closed"); err == nil {
		t.Fatal("closed API accepted delete")
	}
	if err := api.RemoveUser("closed", "user"); err == nil {
		t.Fatal("closed API accepted remove")
	}
	if _, _, err := api.GetTraffic(false); err == nil {
		t.Fatal("closed API accepted stats")
	}
}

type apiLifecycleStats struct {
	stats.UnimplementedStatsServiceServer
}

func (*apiLifecycleStats) QueryStats(context.Context, *stats.QueryStatsRequest) (*stats.QueryStatsResponse, error) {
	return &stats.QueryStatsResponse{}, nil
}

type apiLifecycleHandler struct {
	command.UnimplementedHandlerServiceServer
}

func (*apiLifecycleHandler) RemoveInbound(ctx context.Context, _ *command.RemoveInboundRequest) (*command.RemoveInboundResponse, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestXrayAPIIndependentConcurrentOperations(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer()
	stats.RegisterStatsServiceServer(server, &apiLifecycleStats{})
	command.RegisterHandlerServiceServer(server, &apiLifecycleHandler{})
	go server.Serve(listener)
	t.Cleanup(server.Stop)
	port := listener.Addr().(*net.TCPAddr).Port
	const count = 64
	clients := make([]*XrayAPI, count)
	for i := range clients {
		clients[i] = &XrayAPI{}
		if err := clients[i].Init(port); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(clients[i].Close)
	}
	var wg sync.WaitGroup
	errors := make(chan error, count)
	for i, api := range clients {
		wg.Add(1)
		go func(i int, api *XrayAPI) {
			defer wg.Done()
			defer api.Close()
			if i%2 == 0 {
				return
			}
			for attempt := 0; attempt < 5; attempt++ {
				if _, _, err := api.GetTraffic(false); err != nil {
					errors <- fmt.Errorf("client %d: %w", i, err)
					return
				}
			}
		}(i, api)
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		t.Error(err)
	}
}

func TestXrayAPIDeleteHasDeadline(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer()
	command.RegisterHandlerServiceServer(server, &apiLifecycleHandler{})
	go server.Serve(listener)
	t.Cleanup(server.Stop)
	var api XrayAPI
	if err := api.Init(listener.Addr().(*net.TCPAddr).Port); err != nil {
		t.Fatal(err)
	}
	defer api.Close()
	start := time.Now()
	if err := api.DelInbound("stall"); err == nil {
		t.Fatal("stalled request succeeded")
	}
	if elapsed := time.Since(start); elapsed < 4*time.Second || elapsed > 7*time.Second {
		t.Fatalf("delete did not honor deadline: %v", elapsed)
	}
}
