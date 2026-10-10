package service

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestSpeedConfigIsolationAndChains(t *testing.T) {
	raw := []byte(`{"inbounds":[{"port":744}],"api":{},"log":{"access":"secret.log"},"observatory":{},"strategyObservatory":{},"outbounds":[{"tag":"default","protocol":"freedom"},{"tag":"picked","protocol":"socks","proxySettings":{"tag":"chain"}},{"tag":"chain","protocol":"freedom"},{"tag":"wg","protocol":"wireguard"},{"tag":"blocked","protocol":"blackhole"}]}`)
	config, err := speedWorkerConfig(raw, "picked", 1234, "user", "pass")
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]json.RawMessage
	_ = json.Unmarshal(config, &got)
	for _, k := range []string{"api", "observatory", "strategyObservatory"} {
		if _, ok := got[k]; ok {
			t.Fatal("production feature copied", k)
		}
	}
	var out []struct{ Tag string }
	_ = json.Unmarshal(got["outbounds"], &out)
	if len(out) != 2 || out[0].Tag != "picked" || out[1].Tag != "chain" {
		t.Fatalf("wrong chain: %+v", out)
	}
	if strings.Contains(string(config), "secret.log") || strings.Contains(string(config), "\"port\":744") {
		t.Fatal("production resources copied")
	}
	for _, tag := range []string{"missing", "wg", "blocked"} {
		if _, e := speedWorkerConfig(raw, tag, 1, "a", "b"); e == nil {
			t.Fatal("accepted", tag)
		}
	}
	cycle := []byte(`{"outbounds":[{"tag":"a","protocol":"socks","streamSettings":{"sockopt":{"dialerProxy":"b"}}},{"tag":"b","protocol":"freedom","proxySettings":{"tag":"a"}}]}`)
	if _, e := speedWorkerConfig(cycle, "a", 1, "a", "b"); e == nil {
		t.Fatal("accepted cycle")
	}
	if len(SpeedTestCatalog(raw)) != 3 {
		t.Fatal("catalog should exclude unsupported outbounds")
	}
}

func TestSpeedDownloadConcurrencyQuotaAndCancellation(t *testing.T) {
	for _, threads := range []int{1, 8, 16} {
		var active, maxActive atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			n := active.Add(1)
			defer active.Add(-1)
			for {
				old := maxActive.Load()
				if n <= old || maxActive.CompareAndSwap(old, n) {
					break
				}
			}
			w.Header().Set("Content-Type", "application/octet-stream")
			for i := 0; i < 256; i++ {
				select {
				case <-r.Context().Done():
					return
				case <-time.After(time.Millisecond):
				}
				if _, e := w.Write(make([]byte, 1024)); e != nil {
					return
				}
				w.(http.Flusher).Flush()
			}
		}))
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		row := measureSpeed(ctx, srv.Client(), srv.URL, "picked", threads, 2*time.Second, 64*1024, func(SpeedTestRow) {})
		cancel()
		srv.Close()
		if row.State != "done" || row.Bytes != 64*1024 || !row.LimitReached || row.Mbps <= 0 {
			t.Fatalf("invalid result: %+v", row)
		}
		if maxActive.Load() > int32(threads) || maxActive.Load() < int32(threads) {
			t.Fatalf("connections got %d, want %d", maxActive.Load(), threads)
		}
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer srv.Close()
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(30*time.Millisecond, cancel)
	before := time.Now()
	row := measureSpeed(ctx, srv.Client(), srv.URL, "picked", 4, time.Second, 1024, func(SpeedTestRow) {})
	if row.State != "cancelled" || time.Since(before) > time.Second {
		t.Fatal("cancel did not interrupt requests")
	}
}
func TestSpeedHTTPFailureIsNotReportedAsSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "blocked", 403) }))
	defer srv.Close()
	row := measureSpeed(context.Background(), srv.Client(), srv.URL, "test", 1, time.Second, 1024, func(SpeedTestRow) {})
	if row.State != "failed" || row.Bytes != 0 || row.Error != "download_failed" {
		t.Fatalf("%+v", row)
	}
}
func TestSpeedRequestValidationAndStaleCancel(t *testing.T) {
	raw := []byte(`{"outbounds":[{"tag":"a","protocol":"freedom"}]}`)
	m := &OutboundSpeedTestManager{}
	for _, r := range []SpeedTestRequest{{Tags: []string{"a"}, Threads: 17}, {Tags: []string{"a"}, Threads: 0}, {Tags: []string{"a", "a"}, Threads: 1}, {Threads: 1}} {
		if _, e := m.Start(raw, r); e == nil {
			t.Fatal("invalid request accepted")
		}
	}
	if m.Cancel("other") == nil {
		t.Fatal("stale cancel accepted")
	}
	j := m.Snapshot()
	if j.State != "idle" || j.Rows == nil {
		t.Fatal("invalid empty snapshot")
	}
}
func TestSpeedRealCoreSelectedOutboundAndNoFallback(t *testing.T) {
	binary := os.Getenv("DUI_TEST_XRAY_BINARY")
	if binary == "" {
		t.Skip("set DUI_TEST_XRAY_BINARY for real core routing test")
	}
	var countA, countB atomic.Int32
	serve := func(count *atomic.Int32) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { count.Add(1); _, _ = io.WriteString(w, "selected-exit") }))
	}
	a, b := serve(&countA), serve(&countB)
	defer a.Close()
	defer b.Close()
	listener, _ := net.Listen("tcp", "127.0.0.1:0")
	bad := listener.Addr().String()
	listener.Close()
	raw, _ := json.Marshal(map[string]any{"outbounds": []any{
		map[string]any{"tag": "a", "protocol": "freedom", "settings": map[string]any{"redirect": a.Listener.Addr().String()}},
		map[string]any{"tag": "b", "protocol": "freedom", "settings": map[string]any{"redirect": b.Listener.Addr().String()}},
		map[string]any{"tag": "bad", "protocol": "freedom", "settings": map[string]any{"redirect": bad}},
	}})
	for _, tag := range []string{"b", "bad"} {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		client, cleanup, err := startSpeedWorker(ctx, binary, raw, tag)
		if err != nil {
			cancel()
			t.Fatal(err)
		}
		resp, err := client.Get(a.URL)
		if tag == "b" {
			if err != nil {
				t.Fatal(err)
			}
			data, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if string(data) != "selected-exit" {
				t.Fatal("bad response")
			}
		}
		if tag == "bad" && err == nil {
			resp.Body.Close()
			if resp.StatusCode == 200 {
				t.Fatal("failed outbound fell back")
			}
		}
		cleanup()
		cancel()
	}
	if countA.Load() != 0 || countB.Load() != 1 {
		t.Fatalf("wrong exit counts: a=%d b=%d", countA.Load(), countB.Load())
	}
}

func TestSpeedEmptyResponseIsFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	defer srv.Close()
	row := measureSpeed(context.Background(), srv.Client(), srv.URL, "test", 1, 30*time.Millisecond, 1024, func(SpeedTestRow) {})
	if row.State != "failed" || row.Bytes != 0 {
		t.Fatalf("empty response reported as success: %+v", row)
	}
}

func TestSpeedLegacyTLSCompatibility(t *testing.T) {
	raw := []byte(`{"outbounds":[{"tag":"tls","protocol":"trojan","streamSettings":{"security":"tls","tlsSettings":{"verifyPeerCertInNames":["one.example","two.example"],"echForceQuery":"full"}}}]}`)
	before := string(raw)
	data, err := speedWorkerConfig(raw, "tls", 1234, "u", "p")
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != before {
		t.Fatal("saved input changed")
	}
	if strings.Contains(string(data), "verifyPeerCertInNames") || strings.Contains(string(data), "echForceQuery") {
		t.Fatal("retired TLS fields were not normalized")
	}
	if !strings.Contains(string(data), `"verifyPeerCertByName":"one.example,two.example"`) {
		t.Fatal("TLS verification names were lost")
	}
}
