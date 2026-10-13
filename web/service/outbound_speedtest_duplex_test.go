package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

func TestSpeedUploadQuotaConcurrencyAndFailures(t *testing.T) {
	for _, threads := range []int{1, 8, 16} {
		var active, max, total atomic.Int64
		gate := make(chan struct{})
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "POST" {
				t.Error("upload was not POST")
			}
			current := active.Add(1)
			defer active.Add(-1)
			for {
				old := max.Load()
				if current <= old || max.CompareAndSwap(old, current) {
					break
				}
			}
			if current == int64(threads) {
				close(gate)
			}
			select {
			case <-gate:
			case <-r.Context().Done():
				return
			}
			n, e := io.Copy(io.Discard, r.Body)
			if e == nil {
				total.Add(n)
			}
			w.WriteHeader(200)
		}))
		limit := int64(threads) * 2 * 1024 * 1024
		row := measureUpload(context.Background(), srv.Client(), srv.URL, "a", threads, 2*time.Second, limit, func(SpeedTestRow) {})
		srv.Close()
		if row.State != "done" || row.Bytes != limit || total.Load() != limit || !row.LimitReached || row.Mbps <= 0 || max.Load() != int64(threads) {
			t.Fatalf("quota/concurrency result: %+v count=%d max=%d", row, total.Load(), max.Load())
		}
	}
	for _, status := range []int{403, 500} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.Copy(io.Discard, r.Body); w.WriteHeader(status) }))
		row := measureUpload(context.Background(), srv.Client(), srv.URL, "a", 4, time.Second, 1024*1024, func(SpeedTestRow) {})
		srv.Close()
		if row.State != "failed" || row.Bytes != 0 || row.Error != "upload_failed" {
			t.Fatalf("rejected upload counted as success: %+v", row)
		}
	}
}
func TestSpeedUploadCancellationAndLive(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b := make([]byte, 4096)
		for {
			_, e := r.Body.Read(b)
			if e != nil {
				break
			}
			time.Sleep(time.Millisecond)
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(350*time.Millisecond, cancel)
	live := false
	begin := time.Now()
	row := measureUpload(ctx, srv.Client(), srv.URL, "a", 2, time.Second, 16*1024*1024, func(r SpeedTestRow) {
		if r.CurrentMbps > 0 {
			live = true
		}
	})
	if row.State != "cancelled" || !live || time.Since(begin) > time.Second {
		t.Fatalf("cancellation/live: %+v live=%v", row, live)
	}
}
func TestSpeedDuplexPhasesAndRealtimePing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ping":
			time.Sleep(2 * time.Millisecond)
			w.WriteHeader(200)
		case "/down":
			w.Header().Set("Content-Type", "application/octet-stream")
			for i := 0; i < 400; i++ {
				select {
				case <-r.Context().Done():
					return
				case <-time.After(3 * time.Millisecond):
				}
				if _, e := w.Write(make([]byte, 8192)); e != nil {
					return
				}
				w.(http.Flusher).Flush()
			}
		case "/up":
			io.Copy(io.Discard, r.Body)
			time.Sleep(30 * time.Millisecond)
			w.WriteHeader(200)
		default:
			w.WriteHeader(403)
		}
	}))
	defer srv.Close()
	seen := map[string]bool{}
	row := measureSpeedSuite(context.Background(), srv.Client(), srv.URL+"/down", srv.URL+"/up", srv.URL+"/ping", "a", 2, 650*time.Millisecond, 1<<30, func(r SpeedTestRow) { seen[r.Phase] = true })
	if row.State != "done" || row.Download.Bytes <= 0 || row.Upload.Bytes <= 0 || row.Ping.Samples < 2 || row.Ping.CurrentMS <= 0 || row.Download.CurrentMbps != 0 || row.Upload.CurrentMbps != 0 || !seen["download"] || !seen["upload"] {
		t.Fatalf("duplex result: %+v phases=%v", row, seen)
	}
	row = measureSpeedSuite(context.Background(), srv.Client(), srv.URL+"/bad", srv.URL+"/up", srv.URL+"/bad", "a", 1, 50*time.Millisecond, 64*1024, func(SpeedTestRow) {})
	if row.State != "partial" || row.Download.State != "failed" || row.Upload.State != "done" || row.Ping.State != "failed" || row.Ping.CurrentMS != 0 || row.Ping.Failures != 1 {
		t.Fatalf("independent failures: %+v", row)
	}
}
func TestSpeedDuplexRealCoreAllRequestsUseSelectedOutbound(t *testing.T) {
	binary := os.Getenv("DUI_TEST_XRAY_BINARY")
	if binary == "" {
		t.Skip("DUI_TEST_XRAY_BINARY required")
	}
	var wrong, downs, ups, pings atomic.Int64
	a := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { wrong.Add(1); w.WriteHeader(500) }))
	defer a.Close()
	b := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/up":
			n, _ := io.Copy(io.Discard, r.Body)
			ups.Add(n)
			w.WriteHeader(200)
		case "/ping":
			pings.Add(1)
			w.WriteHeader(200)
		default:
			downs.Add(1)
			w.Write(make([]byte, 65536))
		}
	}))
	defer b.Close()
	raw, _ := json.Marshal(map[string]any{"outbounds": []any{
		map[string]any{"tag": "default", "protocol": "freedom", "settings": map[string]any{"redirect": a.Listener.Addr().String()}},
		map[string]any{"tag": "chosen", "protocol": "freedom", "settings": map[string]any{"redirect": b.Listener.Addr().String()}},
	}})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	client, cleanup, err := startSpeedWorker(ctx, binary, raw, "chosen")
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	row := measureSpeedSuite(ctx, client, a.URL+"/down", a.URL+"/up", a.URL+"/ping", "chosen", 1, time.Second, 65536, func(SpeedTestRow) {})
	if row.State != "done" || wrong.Load() != 0 || downs.Load() == 0 || ups.Load() != 65536 || pings.Load() != 1 {
		t.Fatalf("incorrect route/result: %+v wrong=%d down=%d up=%d ping=%d", row, wrong.Load(), downs.Load(), ups.Load(), pings.Load())
	}
}

func TestSpeedCloudflareSmoke(t *testing.T) {
	if os.Getenv("DUI_SPEED_CLOUDFLARE_SMOKE") != "1" {
		t.Skip("opt-in 64 KiB external API smoke test")
	}
	binary := os.Getenv("DUI_TEST_XRAY_BINARY")
	if binary == "" {
		t.Fatal("DUI_TEST_XRAY_BINARY required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	raw := []byte(`{"outbounds":[{"tag":"cloudflare-smoke","protocol":"freedom"}]}`)
	client, cleanup, err := startSpeedWorker(ctx, binary, raw, "cloudflare-smoke")
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	row := measureSpeedSuite(ctx, client, speedTestURL, speedUploadURL, speedPingURL, "cloudflare-smoke", 1, time.Second, 65536, func(SpeedTestRow) {})
	t.Logf("state=%s download=%s/%d upload=%s/%d ping=%s/%.2fms", row.State, row.Download.State, row.Download.Bytes, row.Upload.State, row.Upload.Bytes, row.Ping.State, row.Ping.CurrentMS)
	if row.State != "done" || row.Download.Bytes != 65536 || row.Upload.Bytes != 65536 || row.Ping.Samples < 1 {
		t.Fatal("Cloudflare API smoke failed")
	}
}
