package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestSpeedTargetDefaultAndValidation(t *testing.T) {
	for _, provider := range []string{"", "cloudflare"} {
		got, err := resolveSpeedTarget(SpeedTestTarget{Provider: provider, DownloadURL: "https://ignored.example"})
		if err != nil || got.Provider != "cloudflare" || got.DownloadURL != speedTestURL || got.UploadURL != speedUploadURL || got.PingURL != speedPingURL {
			t.Fatalf("default %+v %v", got, err)
		}
	}
	want := SpeedTestTarget{Provider: "custom", DownloadURL: "https://test.example/file?signature=keep%2Fme", UploadURL: "https://test.example/up", PingURL: "http://[::1]:8080/ping"}
	got, err := resolveSpeedTarget(want)
	if err != nil || got != want {
		t.Fatalf("custom %+v %v", got, err)
	}
	for _, value := range []string{"", "/relative", "ftp://test.example/file", "file:///tmp/file", "https://user:pass@test.example/file", "https://test.example/file#fragment", "https://test.example:99999/file", "https://test.example/bad path", "https://test.example/" + strings.Repeat("a", 2048)} {
		if _, err := resolveSpeedTarget(SpeedTestTarget{Provider: "custom", DownloadURL: value}); err == nil {
			t.Fatalf("accepted %q", value)
		}
	}
	for _, target := range []SpeedTestTarget{
		{Provider: "other"}, {Provider: "custom", DownloadURL: want.DownloadURL, UploadURL: "file:///tmp"},
		{Provider: "custom", DownloadURL: want.DownloadURL, PingURL: "javascript:bad"},
	} {
		if _, err := resolveSpeedTarget(target); err == nil {
			t.Fatalf("accepted %+v", target)
		}
	}
	got, err = resolveSpeedTarget(SpeedTestTarget{Provider: "custom", DownloadURL: "  https://test.example/file  "})
	if err != nil || got.UploadURL != "" || got.PingURL != "" || got.DownloadURL != "https://test.example/file" {
		t.Fatalf("optional URLs %+v %v", got, err)
	}
}
func TestSpeedTargetOptionalRequestsAndFailureState(t *testing.T) {
	for _, status := range []int{200, 403} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var count, wrong atomic.Int64
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				count.Add(1)
				if r.Method != "GET" || r.URL.RequestURI() != "/down?signature=keep%2Fme" {
					wrong.Add(1)
				}
				w.WriteHeader(status)
				_, _ = w.Write(make([]byte, 8192))
			}))
			defer srv.Close()
			row := measureSpeedSuite(context.Background(), srv.Client(), srv.URL+"/down?signature=keep%2Fme", "", "", "a", 1, time.Second, 8192, func(SpeedTestRow) {})
			want := "done"
			if status == 403 {
				want = "failed"
			}
			if row.State != want || row.Upload.State != "skipped" || row.Ping.State != "skipped" || count.Load() != 1 || wrong.Load() != 0 {
				t.Fatalf("row=%+v count=%d wrong=%d", row, count.Load(), wrong.Load())
			}
			if status == 200 && (row.Progress != 100 || row.Download.Bytes != 8192) {
				t.Fatalf("download-only progress %+v", row)
			}
		})
	}
}
func TestSpeedTargetPingPreservesQueryAndAccepts204(t *testing.T) {
	var wrong atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.RequestURI() != "/ping?signature=keep%2Fme" {
			wrong.Store(true)
		}
		w.WriteHeader(204)
	}))
	defer srv.Close()
	_, ok := measureSpeedPing(context.Background(), srv.Client(), srv.URL+"/ping?signature=keep%2Fme")
	if !ok || wrong.Load() {
		t.Fatal("custom ping changed URL or rejected 204")
	}
}
