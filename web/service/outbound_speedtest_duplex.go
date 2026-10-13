package service

import (
	"context"
	"crypto/rand"
	"io"
	"math"
	"net/http"
	"net/http/httptrace"
	"sync"
	"sync/atomic"
	"time"
)

const speedUploadURL = "https://speed.cloudflare.com/__up"
const speedPingURL = "https://speed.cloudflare.com/__down?bytes=0"

type SpeedMeasurement struct {
	WaitSeconds      float64 `json:"waitSeconds"`
	WaitLimitSeconds float64 `json:"waitLimitSeconds"`
	Retries          int     `json:"retries"`
	HTTPStatus       int     `json:"httpStatus,omitempty"`
	State            string  `json:"state"`
	Error            string  `json:"error,omitempty"`
	Bytes            int64   `json:"bytes"`
	Seconds          float64 `json:"seconds"`
	Mbps             float64 `json:"mbps"`
	CurrentMbps      float64 `json:"currentMbps"`
	Progress         int     `json:"progress"`
	LimitReached     bool    `json:"limitReached"`
}
type SpeedLatency struct {
	State     string  `json:"state"`
	CurrentMS float64 `json:"currentMs"`
	AverageMS float64 `json:"averageMs"`
	MinMS     float64 `json:"minMs"`
	MaxMS     float64 `json:"maxMs"`
	JitterMS  float64 `json:"jitterMs"`
	Samples   int     `json:"samples"`
	Failures  int     `json:"failures"`
	UpdatedAt int64   `json:"updatedAt"`
}

func speedMeasurement(r SpeedTestRow) SpeedMeasurement {
	return SpeedMeasurement{State: r.State, Error: r.Error, Bytes: r.Bytes, Seconds: r.Seconds, Mbps: r.Mbps, CurrentMbps: r.CurrentMbps, Progress: r.Progress, LimitReached: r.LimitReached, WaitSeconds: r.WaitSeconds, WaitLimitSeconds: r.WaitLimitSeconds, Retries: r.Retries, HTTPStatus: r.HTTPStatus}
}

// HTTP round-trip time after the request is written, excluding DNS/TLS setup.
// All requests use the same authenticated local proxy and selected outbound.
func measureSpeedPing(ctx context.Context, client *http.Client, target string) (float64, bool) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return 0, false
	}
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Accept-Encoding", "identity")
	var written, first atomic.Int64
	trace := &httptrace.ClientTrace{
		WroteRequest: func(info httptrace.WroteRequestInfo) {
			if info.Err == nil {
				written.Store(time.Now().UnixNano())
			}
		},
		GotFirstResponseByte: func() { first.Store(time.Now().UnixNano()) },
	}
	req = req.WithContext(httptrace.WithClientTrace(req.Context(), trace))
	resp, err := client.Do(req)
	if err != nil {
		return 0, false
	}
	defer resp.Body.Close()
	_, err = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	elapsed := first.Load() - written.Load()
	return float64(elapsed) / 1e6, err == nil && resp.StatusCode >= 200 && resp.StatusCode < 300 && written.Load() > 0 && elapsed > 0
}

func measureSpeedSuite(ctx context.Context, client *http.Client, downURL, upURL, pingURL, tag string, threads int, duration time.Duration, limit int64, update func(SpeedTestRow)) SpeedTestRow {
	// Dedicated transport prevents the ping queue from being blocked by all 16 upload/download connections.
	pingClient := *client
	if transport, ok := client.Transport.(*http.Transport); ok {
		pt := transport.Clone()
		pt.MaxConnsPerHost = 1
		pt.MaxIdleConnsPerHost = 1
		pingClient.Transport = pt
		defer pt.CloseIdleConnections()
	}
	var mu sync.Mutex
	row := SpeedTestRow{Tag: tag, State: "testing", Phase: "latency", Download: SpeedMeasurement{State: "queued"}, Upload: SpeedMeasurement{State: "queued"}, Ping: SpeedLatency{State: "preparing"}}
	if upURL == "" {
		row.Upload.State = "skipped"
	}
	if pingURL == "" {
		row.Ping.State = "skipped"
	}
	update(row)
	var previous, jitterSum float64
	sample := func(probeCtx context.Context) {
		ms, ok := measureSpeedPing(probeCtx, &pingClient, pingURL)
		if probeCtx.Err() != nil {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		p := &row.Ping
		p.UpdatedAt = time.Now().UnixMilli()
		if ok {
			p.State = "done"
			p.CurrentMS = ms
			p.Samples++
			p.AverageMS += (ms - p.AverageMS) / float64(p.Samples)
			if p.Samples == 1 {
				p.MinMS = ms
				p.MaxMS = ms
			} else {
				p.MinMS = math.Min(p.MinMS, ms)
				p.MaxMS = math.Max(p.MaxMS, ms)
				jitterSum += math.Abs(ms - previous)
				p.JitterMS = jitterSum / float64(p.Samples-1)
			}
			previous = ms
		} else {
			p.State = "failed"
			p.CurrentMS = 0
			p.Failures++
		}
		update(row)
	}
	if pingURL != "" {
		sample(ctx)
	}
	pingCtx, pingCancel := context.WithCancel(ctx)
	pingDone := make(chan struct{})
	go func() {
		defer close(pingDone)
		if pingURL == "" {
			return
		}
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-pingCtx.Done():
				return
			case <-ticker.C:
				sample(pingCtx)
			}
		}
	}()
	apply := func(phase string, r SpeedTestRow) {
		mu.Lock()
		defer mu.Unlock()
		row.Phase = phase
		if phase == "download" {
			row.Download = speedMeasurement(r)
			row.Bytes = r.Bytes
			row.Mbps = r.Mbps
			row.CurrentMbps = r.CurrentMbps
			row.LimitReached = r.LimitReached
		} else {
			row.Upload = speedMeasurement(r)
		}
		row.Seconds = row.Download.Seconds + row.Upload.Seconds
		row.Progress = row.Download.Progress
		if upURL != "" {
			row.Progress = (row.Download.Progress + row.Upload.Progress) / 2
		}
		update(row)
	}
	down := measureSpeed(ctx, client, downURL, tag, threads, duration, limit, func(r SpeedTestRow) { apply("download", r) })
	apply("download", down)
	if ctx.Err() == nil && upURL != "" {
		up := measureUpload(ctx, client, upURL, tag, threads, duration, limit, func(r SpeedTestRow) { apply("upload", r) })
		apply("upload", up)
	}
	pingCancel()
	<-pingDone
	mu.Lock()
	defer mu.Unlock()
	row.Phase = ""
	row.CurrentMbps = 0
	row.Download.CurrentMbps = 0
	row.Upload.CurrentMbps = 0
	switch {
	case ctx.Err() != nil:
		row.State = "cancelled"
		if row.Upload.State == "queued" {
			row.Upload.State = "cancelled"
		}
	case row.Download.State == "failed" && (row.Upload.State == "failed" || row.Upload.State == "skipped"):
		row.State = "failed"
	case row.Download.State != "done" || (row.Upload.State != "done" && row.Upload.State != "skipped") || (pingURL != "" && row.Ping.Samples == 0):
		row.State = "partial"
	default:
		row.State = "done"
	}
	return row
}

// A small reusable buffer is streamed into bounded POST requests. Only fully
// acknowledged successful requests contribute to the final upload average.
type speedUploadReader struct {
	pattern   []byte
	remaining atomic.Int64
	sent      *atomic.Int64
	start     func()
}

func (r *speedUploadReader) Read(p []byte) (int, error) {
	remaining := r.remaining.Load()
	if remaining == 0 {
		return 0, io.EOF
	}
	n := len(p)
	if n > len(r.pattern) {
		n = len(r.pattern)
	}
	if int64(n) > remaining {
		n = int(remaining)
	}
	copy(p[:n], r.pattern[:n])
	r.start()
	r.remaining.Add(-int64(n))
	r.sent.Add(int64(n))
	return n, nil
}
func measureUpload(parent context.Context, client *http.Client, target, tag string, threads int, duration time.Duration, limit int64, update func(SpeedTestRow)) SpeedTestRow {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	waitingSince := time.Now()
	var allocated, sent, confirmed atomic.Int64
	var failures atomic.Int32
	started := make(chan time.Time, 1)
	var first sync.Once
	pattern := make([]byte, 32*1024)
	if _, err := rand.Read(pattern); err != nil {
		return SpeedTestRow{Tag: tag, State: "failed", Error: "upload_failed"}
	}
	var wg sync.WaitGroup
	for n := 0; n < threads; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ctx.Err() == nil {
				var size int64
				for {
					old := allocated.Load()
					size = 2 * 1024 * 1024
					if old >= limit {
						return
					}
					if size > limit-old {
						size = limit - old
					}
					if allocated.CompareAndSwap(old, old+size) {
						break
					}
				}
				body := &speedUploadReader{pattern: pattern, sent: &sent, start: func() { first.Do(func() { started <- time.Now() }) }}
				body.remaining.Store(size)
				req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, body)
				if err != nil {
					failures.Add(1)
					return
				}
				req.ContentLength = size
				req.Header.Set("Content-Type", "application/octet-stream")
				req.Header.Set("Cache-Control", "no-cache")
				req.Header.Set("User-Agent", "DUI-SpeedTest/2")
				resp, err := client.Do(req)
				if err != nil {
					if ctx.Err() == nil {
						failures.Add(1)
					}
					return
				}
				_, readErr := io.Copy(io.Discard, io.LimitReader(resp.Body, 64*1024))
				resp.Body.Close()
				// Reject an early response if the complete request body was not consumed.
				if resp.StatusCode < 200 || resp.StatusCode >= 300 || readErr != nil {
					if ctx.Err() == nil {
						failures.Add(1)
					}
					return
				}
				if body.remaining.Load() != 0 {
					if ctx.Err() == nil {
						failures.Add(1)
					}
					return
				}
				confirmed.Add(size)
			}
		}()
	}
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	row := SpeedTestRow{Tag: tag, State: "connecting", WaitLimitSeconds: speedTestWait.Seconds()}
	update(row)
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	connect := time.NewTimer(speedTestWait)
	defer connect.Stop()
	var begin, lastTime time.Time
	var lastBytes int64
	var deadline <-chan time.Time
	var measurementTimer *time.Timer
	finish := func() {
		row.Bytes = confirmed.Load()
		row.WaitSeconds = time.Since(waitingSince).Seconds()
		if !begin.IsZero() {
			row.WaitSeconds = begin.Sub(waitingSince).Seconds()
		}
		if !begin.IsZero() {
			row.Seconds = time.Since(begin).Seconds()
			if row.Seconds > 0 {
				row.Mbps = float64(row.Bytes) * 8 / row.Seconds / 1e6
			}
		}
		row.CurrentMbps = 0
		row.LimitReached = row.Bytes >= limit
		row.Progress = int(row.Seconds / duration.Seconds() * 100)
		if row.Progress > 99 {
			row.Progress = 99
		}
		if row.LimitReached {
			row.Progress = 100
		}
	}
	result := func(state string) SpeedTestRow {
		cancel()
		<-done
		if begin.IsZero() {
			select {
			case begin = <-started:
			default:
			}
		}
		finish()
		row.State = state
		if state == "cancelled" {
			return row
		}
		if row.Bytes == 0 {
			row.State = "failed"
			row.Error = "upload_failed"
		} else if failures.Load() > 0 {
			row.State = "partial"
			row.Error = "upload_stream_failed"
		}
		if row.Seconds >= duration.Seconds() || row.LimitReached {
			row.Progress = 100
		}
		return row
	}
	for {
		select {
		case begin = <-started:
			lastTime = begin
			measurementTimer = time.NewTimer(duration)
			defer measurementTimer.Stop()
			deadline = measurementTimer.C
			connect.Stop()
			row.State = "testing"
		case <-ticker.C:
			if begin.IsZero() {
				finish()
				update(row)
				continue
			}
			finish()
			now := time.Now()
			current := sent.Load()
			row.CurrentMbps = float64(current-lastBytes) * 8 / now.Sub(lastTime).Seconds() / 1e6
			lastBytes = current
			lastTime = now
			update(row)
		case <-deadline:
			return result("done")
		case <-done:
			return result("done")
		case <-connect.C:
			return result("failed")
		case <-parent.Done():
			return result("cancelled")
		}
	}
}
