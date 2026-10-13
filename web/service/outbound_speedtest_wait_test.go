package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestSpeedWaitSlowHeadersAndBody(t *testing.T) {
	for _, flush := range []bool{false, true} {
		name := "headers"
		if flush {
			name = "body"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Length", "4096")
				if flush {
					w.WriteHeader(200)
					w.(http.Flusher).Flush()
				}
				select {
				case <-r.Context().Done():
					return
				case <-time.After(13 * time.Second):
				}
				w.Write(make([]byte, 4096))
			}))
			defer srv.Close()
			waiting := false
			row := measureSpeed(context.Background(), srv.Client(), srv.URL, "test", 1, 50*time.Millisecond, 4096, func(r SpeedTestRow) {
				if r.State == "connecting" && r.WaitSeconds >= 12 && r.Seconds == 0 && r.Progress == 0 {
					waiting = true
				}
			})
			if row.State != "done" || row.Bytes != 4096 || row.WaitSeconds < 12 || row.Seconds >= 1 || !waiting {
				t.Fatalf("slow start not measured correctly: %+v waiting=%v", row, waiting)
			}
		})
	}
}
func TestSpeedWaitTransientRetryAndRejection(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch calls.Add(1) {
		case 1:
			conn, _, _ := w.(http.Hijacker).Hijack()
			conn.Close()
		case 2:
			w.WriteHeader(503)
		default:
			io.WriteString(w, "payload")
		}
	}))
	defer srv.Close()
	row := measureSpeedWithWait(context.Background(), srv.Client(), srv.URL, "test", 1, time.Second, 7, time.Second, 5*time.Millisecond, func(SpeedTestRow) {})
	if row.State != "done" || row.Bytes != 7 || row.Retries != 2 || calls.Load() != 3 {
		t.Fatalf("retry recovery: %+v calls=%d", row, calls.Load())
	}
	var rejects atomic.Int32
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { rejects.Add(1); w.WriteHeader(403) }))
	defer bad.Close()
	row = measureSpeedWithWait(context.Background(), bad.Client(), bad.URL, "test", 1, time.Second, 7, time.Second, 5*time.Millisecond, func(SpeedTestRow) {})
	if row.Error != "download_rejected" || row.HTTPStatus != 403 || row.State != "failed" || rejects.Load() != 1 || row.Bytes != 0 {
		t.Fatalf("permanent failure should not retry: %+v", row)
	}
}
func TestSpeedWaitBoundedAndCancelable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer srv.Close()
	start := time.Now()
	row := measureSpeedWithWait(context.Background(), srv.Client(), srv.URL, "test", 2, time.Second, 1024, 300*time.Millisecond, time.Millisecond, func(SpeedTestRow) {})
	if row.Error != "download_timeout" || row.Seconds != 0 || row.Progress != 0 || time.Since(start) > time.Second || row.WaitSeconds < .25 {
		t.Fatalf("waiting budget: %+v", row)
	}
	var attempts atomic.Int32
	fail := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { attempts.Add(1); w.WriteHeader(503) }))
	defer fail.Close()
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(30*time.Millisecond, cancel)
	start = time.Now()
	row = measureSpeedWithWait(ctx, fail.Client(), fail.URL, "test", 1, time.Second, 1024, time.Second, 200*time.Millisecond, func(SpeedTestRow) {})
	if row.State != "cancelled" || time.Since(start) > 200*time.Millisecond || attempts.Load() != 1 {
		t.Fatalf("cancel retry backoff: %+v calls=%d", row, attempts.Load())
	}
}
