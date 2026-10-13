package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"x-ui/xray"
)

const speedTestDuration = 10 * time.Second
const speedTestWait = 45 * time.Second
const speedTestWorkerTimeout = 2*(speedTestWait+speedTestDuration) + 15*time.Second
const speedTestLimit int64 = 512 * 1024 * 1024
const speedTestURL = "https://speed.cloudflare.com/__down?bytes=25000000"

var OutboundSpeedTests = &OutboundSpeedTestManager{}

type SpeedTestRequest struct {
	Target  SpeedTestTarget `json:"target"`
	Tags    []string        `json:"tags"`
	Threads int             `json:"threads"`
}
type SpeedTestRow struct {
	WaitSeconds      float64          `json:"waitSeconds"`
	WaitLimitSeconds float64          `json:"waitLimitSeconds"`
	Retries          int              `json:"retries"`
	HTTPStatus       int              `json:"httpStatus,omitempty"`
	Phase            string           `json:"phase,omitempty"`
	Download         SpeedMeasurement `json:"download"`
	Upload           SpeedMeasurement `json:"upload"`
	Ping             SpeedLatency     `json:"ping"`
	Tag              string           `json:"tag"`
	State            string           `json:"state"`
	Error            string           `json:"error,omitempty"`
	Bytes            int64            `json:"bytes"`
	Seconds          float64          `json:"seconds"`
	Mbps             float64          `json:"mbps"`
	CurrentMbps      float64          `json:"currentMbps"`
	Progress         int              `json:"progress"`
	LimitReached     bool             `json:"limitReached"`
}
type SpeedTestJob struct {
	Target    SpeedTestTarget `json:"target"`
	ID        string          `json:"id"`
	State     string          `json:"state"`
	Threads   int             `json:"threads"`
	StartedAt int64           `json:"startedAt"`
	Rows      []SpeedTestRow  `json:"rows"`
}
type OutboundSpeedTestManager struct {
	mu     sync.Mutex
	job    SpeedTestJob
	cancel context.CancelFunc
}
type speedConfig map[string]json.RawMessage

func speedToken() string {
	var data [24]byte
	if _, err := rand.Read(data[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(data[:])
}

// Only the selected outbound and its explicit dialer/proxy chain are copied.
// No production inbounds, routing, probes, API or logs are started in the worker.
func speedWorkerConfig(raw []byte, tag string, port int, user, password string) ([]byte, error) {
	var cfg speedConfig
	if json.Unmarshal(raw, &cfg) != nil {
		return nil, errors.New("invalid_config")
	}
	var outbounds []speedConfig
	if json.Unmarshal(cfg["outbounds"], &outbounds) != nil {
		return nil, errors.New("invalid_config")
	}
	byTag := map[string]speedConfig{}
	str := func(m speedConfig, k string) string { var v string; _ = json.Unmarshal(m[k], &v); return v }
	for _, o := range outbounds {
		name := str(o, "tag")
		if name == "" {
			continue
		}
		if byTag[name] != nil {
			return nil, errors.New("duplicate_tag")
		}
		byTag[name] = o
	}
	selected := []speedConfig{}
	visiting, done := map[string]bool{}, map[string]bool{}
	var visit func(string) error
	visit = func(name string) error {
		if visiting[name] {
			return errors.New("invalid_chain")
		}
		if done[name] {
			return nil
		}
		o := byTag[name]
		if o == nil {
			return errors.New("missing_outbound")
		}
		switch str(o, "protocol") {
		case "freedom", "socks", "http", "shadowsocks", "vmess", "vless", "trojan", "hysteria", "hysteria2":
		default:
			return errors.New("unsupported_outbound")
		}
		var settings map[string]json.RawMessage
		_ = json.Unmarshal(o["settings"], &settings)
		// Reverse outbounds may register listeners; never start them in a test worker.
		if len(settings["reverse"]) > 0 && string(settings["reverse"]) != "null" {
			return errors.New("unsupported_outbound")
		}
		visiting[name] = true
		selected = append(selected, o)
		var proxy struct {
			Tag string `json:"tag"`
		}
		_ = json.Unmarshal(o["proxySettings"], &proxy)
		var stream struct {
			Sockopt struct {
				DialerProxy string `json:"dialerProxy"`
			} `json:"sockopt"`
		}
		_ = json.Unmarshal(o["streamSettings"], &stream)
		for _, dep := range []string{proxy.Tag, stream.Sockopt.DialerProxy} {
			if dep != "" {
				if err := visit(dep); err != nil {
					return err
				}
			}
		}
		visiting[name] = false
		done[name] = true
		return nil
	}
	if err := visit(tag); err != nil {
		return nil, err
	}
	worker := map[string]any{
		"log":       map[string]any{"loglevel": "none"},
		"inbounds":  []any{map[string]any{"tag": "dui-speed-in", "listen": "127.0.0.1", "port": port, "protocol": "http", "settings": map[string]any{"accounts": []any{map[string]string{"user": user, "pass": password}}}}},
		"outbounds": selected,
		"routing":   map[string]any{"domainStrategy": "AsIs", "rules": []any{map[string]any{"type": "field", "inboundTag": []string{"dui-speed-in"}, "outboundTag": tag}}},
	}
	if dns := cfg["dns"]; len(dns) > 0 {
		worker["dns"] = dns
	}
	// Keep the same retired TLS-key normalization as the production config builder.
	// Normalize the isolated copy only; saved configuration remains unchanged.
	data, err := json.Marshal(worker)
	if err != nil {
		return nil, err
	}
	var normalized any
	if err := json.Unmarshal(data, &normalized); err != nil {
		return nil, err
	}
	normalizeDUITLS(normalized)
	return json.Marshal(normalized)
}

func (m *OutboundSpeedTestManager) Snapshot() SpeedTestJob {
	m.mu.Lock()
	defer m.mu.Unlock()
	j := m.job
	j.Rows = append([]SpeedTestRow{}, j.Rows...)
	if j.State == "" {
		j.State = "idle"
	}
	return j
}
func (m *OutboundSpeedTestManager) Cancel(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if id == "" || id != m.job.ID {
		return errors.New("stale_job")
	}
	if m.cancel != nil {
		m.job.State = "stopping"
		m.cancel()
	}
	return nil
}
func (m *OutboundSpeedTestManager) Start(raw []byte, req SpeedTestRequest) (SpeedTestJob, error) {
	if len(req.Tags) < 1 || len(req.Tags) > 10 || req.Threads < 1 || req.Threads > 16 {
		return SpeedTestJob{}, errors.New("invalid_request")
	}
	target, err := resolveSpeedTarget(req.Target)
	if err != nil {
		return SpeedTestJob{}, err
	}
	req.Target = target
	seen := map[string]bool{}
	for _, tag := range req.Tags {
		if tag == "" || seen[tag] {
			return SpeedTestJob{}, errors.New("invalid_request")
		}
		seen[tag] = true
		if _, err := speedWorkerConfig(raw, tag, 12345, "test", "test"); err != nil {
			return SpeedTestJob{}, err
		}
	}
	// Resolve now, before acquiring the single-job slot.
	binary, err := filepath.Abs(xray.GetBinaryPath())
	if err != nil {
		return SpeedTestJob{}, errors.New("core_unavailable")
	}
	if stat, e := os.Stat(binary); e != nil || stat.IsDir() {
		return SpeedTestJob{}, errors.New("core_unavailable")
	}
	m.mu.Lock()
	if m.cancel != nil {
		m.mu.Unlock()
		return SpeedTestJob{}, errors.New("busy")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(len(req.Tags))*speedTestWorkerTimeout+15*time.Second)
	m.cancel = cancel
	m.job = SpeedTestJob{Target: target, ID: speedToken(), State: "running", Threads: req.Threads, StartedAt: time.Now().UnixMilli(), Rows: []SpeedTestRow{}}
	for _, tag := range req.Tags {
		m.job.Rows = append(m.job.Rows, SpeedTestRow{Tag: tag, State: "queued"})
	}
	job := m.job
	job.Rows = append([]SpeedTestRow{}, job.Rows...)
	m.mu.Unlock()
	go m.run(ctx, cancel, append([]byte{}, raw...), binary, req)
	return job, nil
}
func (m *OutboundSpeedTestManager) setRow(i int, row SpeedTestRow) {
	m.mu.Lock()
	m.job.Rows[i] = row
	m.mu.Unlock()
}
func (m *OutboundSpeedTestManager) run(ctx context.Context, cancel context.CancelFunc, raw []byte, binary string, req SpeedTestRequest) {
	defer func() {
		wasCancelled := ctx.Err() != nil
		cancel()
		m.mu.Lock()
		defer m.mu.Unlock()
		m.job.State = "done"
		if wasCancelled {
			m.job.State = "cancelled"
		}
		for i := range m.job.Rows {
			if m.job.Rows[i].State == "queued" {
				m.job.Rows[i].State = "cancelled"
			}
		}
		m.cancel = nil
	}()
	for i, tag := range req.Tags {
		if ctx.Err() != nil {
			return
		}
		row := SpeedTestRow{Tag: tag, State: "preparing"}
		m.setRow(i, row)
		func() {
			workerCtx, workerCancel := context.WithTimeout(ctx, speedTestWorkerTimeout)
			defer workerCancel()
			client, cleanup, err := startSpeedWorker(workerCtx, binary, raw, tag)
			if err != nil {
				row.State = "failed"
				row.Error = err.Error()
				if ctx.Err() != nil {
					row.State = "cancelled"
					row.Error = ""
				}
				m.setRow(i, row)
				return
			}
			defer cleanup()
			row = measureSpeedSuite(workerCtx, client, req.Target.DownloadURL, req.Target.UploadURL, req.Target.PingURL, tag, req.Threads, speedTestDuration, speedTestLimit, func(r SpeedTestRow) { m.setRow(i, r) })
			if ctx.Err() != nil {
				row.State = "cancelled"
				row.Error = ""
			} else if row.State == "cancelled" {
				row.State = "failed"
				row.Error = "test_timeout"
			}
			m.setRow(i, row)
		}()
	}
}

func startSpeedWorker(ctx context.Context, binary string, raw []byte, tag string) (*http.Client, func(), error) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, nil, errors.New("worker_start")
	}
	port := listener.Addr().(*net.TCPAddr).Port
	user, password := speedToken(), speedToken()
	data, err := speedWorkerConfig(raw, tag, port, user, password)
	if err != nil {
		listener.Close()
		return nil, nil, err
	}
	dir, err := os.MkdirTemp("", "dui-speed-")
	if err != nil {
		listener.Close()
		return nil, nil, errors.New("worker_start")
	}
	path := filepath.Join(dir, "config.json")
	if err = os.WriteFile(path, data, 0600); err != nil {
		listener.Close()
		os.RemoveAll(dir)
		return nil, nil, errors.New("worker_start")
	}
	// CommandContext bounds the lifetime; Linux also kills the worker if its panel parent exits.
	cmd := exec.CommandContext(ctx, binary, "run", "-config", path)
	speedWorkerParentDeath(cmd)
	cmd.Env = append(os.Environ(), "XRAY_LOCATION_ASSET="+filepath.Dir(binary))
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	listener.Close()
	if err = cmd.Start(); err != nil {
		os.RemoveAll(dir)
		return nil, nil, errors.New("worker_start")
	}
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()
	proxyURL := &url.URL{Scheme: "http", Host: net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), User: url.UserPassword(user, password)}
	transport := &http.Transport{Proxy: http.ProxyURL(proxyURL), DisableCompression: true, ForceAttemptHTTP2: false, MaxIdleConnsPerHost: 16, MaxConnsPerHost: 16, TLSHandshakeTimeout: 30 * time.Second, ResponseHeaderTimeout: speedTestWait}
	client := &http.Client{Transport: transport, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return errors.New("redirect_disabled") }}
	cleanup := func() { transport.CloseIdleConnections(); _ = cmd.Process.Kill(); <-done; _ = os.RemoveAll(dir) }
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			cleanup()
			return nil, nil, errors.New("worker_start")
		case <-done:
			cleanup()
			return nil, nil, errors.New("worker_start")
		case <-timer.C:
			cleanup()
			return nil, nil, errors.New("worker_start")
		case <-ticker.C:
			conn, e := net.DialTimeout("tcp", proxyURL.Host, 100*time.Millisecond)
			if e == nil {
				conn.Close()
				return client, cleanup, nil
			}
		}
	}
}

// A slow connection gets a separate budget; sampling begins with the first body byte.
func measureSpeed(parent context.Context, client *http.Client, target, tag string, threads int, duration time.Duration, limit int64, update func(SpeedTestRow)) SpeedTestRow {
	return measureSpeedWithWait(parent, client, target, tag, threads, duration, limit, speedTestWait, 2*time.Second, update)
}
func measureSpeedWithWait(parent context.Context, client *http.Client, target, tag string, threads int, duration time.Duration, limit int64, wait, retryDelay time.Duration, update func(SpeedTestRow)) SpeedTestRow {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	waitingSince := time.Now()
	var total, reserved atomic.Int64
	var failures, retries atomic.Int32
	type failure struct {
		code   string
		status int
	}
	var lastFailure atomic.Pointer[failure]
	var first sync.Once
	started := make(chan time.Time, 1)
	var wg sync.WaitGroup
	for n := 0; n < threads; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			buf := make([]byte, 32*1024)
			attempt := int32(0)
			retry := func() bool {
				if ctx.Err() != nil || total.Load() > 0 {
					return false
				}
				attempt++
				delay := retryDelay
				for i := int32(1); i < attempt && i < 3; i++ {
					delay *= 2
				}
				timer := time.NewTimer(delay)
				defer timer.Stop()
				select {
				case <-ctx.Done():
					return false
				case <-timer.C:
					if total.Load() != 0 || ctx.Err() != nil {
						return false
					}
					for old := retries.Load(); attempt > old; old = retries.Load() {
						if retries.CompareAndSwap(old, attempt) {
							break
						}
					}

					return true
				}
			}
			record := func(code string, status int) { lastFailure.Store(&failure{code, status}) }
			for ctx.Err() == nil {
				req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
				if err != nil {
					record("download_failed", 0)
					failures.Add(1)
					return
				}
				req.Header.Set("Accept-Encoding", "identity")
				req.Header.Set("Cache-Control", "no-cache")
				req.Header.Set("User-Agent", "DUI-SpeedTest/2")
				resp, err := client.Do(req)
				if err != nil {
					if ctx.Err() == nil {
						record("download_failed", 0)
						if retry() {
							continue
						}
						failures.Add(1)
					}
					return
				}
				if resp.StatusCode != http.StatusOK {
					resp.Body.Close()
					record("download_rejected", resp.StatusCode)
					if (resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500) && retry() {
						continue
					}
					if ctx.Err() == nil {
						failures.Add(1)
					}
					return
				}
				var readErr error
				exhausted := false
				received := false
				for ctx.Err() == nil {
					quota := int64(len(buf))
					for {
						old := reserved.Load()
						if old >= limit {
							quota = 0
							break
						}
						quota = int64(len(buf))
						if limit-old < quota {
							quota = limit - old
						}
						if reserved.CompareAndSwap(old, old+quota) {
							break
						}
					}
					if quota == 0 {
						exhausted = true
						break
					}
					count, e := resp.Body.Read(buf[:quota])
					if count > 0 {
						first.Do(func() { started <- time.Now() })
						received = true
					}
					total.Add(int64(count))
					reserved.Add(-(quota - int64(count)))
					if e != nil {
						readErr = e
						break
					}
				}
				resp.Body.Close()
				if total.Load() >= limit {
					return
				}
				if readErr != nil && readErr != io.EOF {
					if ctx.Err() == nil {
						record("download_failed", 0)
						if !received && retry() {
							continue
						}
						failures.Add(1)
					}
					return
				}
				if exhausted {
					return
				}
				if !received {
					// A completed empty HTTP 200 response cannot become a download by waiting.
					if ctx.Err() == nil {
						record("download_empty", 200)
						failures.Add(1)
					}
					return
				}
				if reserved.Load() >= limit {
					return
				}
			}
		}()
	}
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	row := SpeedTestRow{Tag: tag, State: "connecting", WaitLimitSeconds: wait.Seconds()}
	update(row)
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	connectTimer := time.NewTimer(wait)
	defer connectTimer.Stop()
	var begin, lastTime time.Time
	var lastBytes int64
	var deadline <-chan time.Time
	finish := func() {
		row.Bytes = total.Load()
		row.Retries = int(retries.Load())
		row.WaitSeconds = time.Since(waitingSince).Seconds()
		if !begin.IsZero() {
			row.WaitSeconds = begin.Sub(waitingSince).Seconds()
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
	result := func(state string, timedOut bool) SpeedTestRow {
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
			row.Error = "download_failed"
			if f := lastFailure.Load(); f != nil {
				row.Error = f.code
				row.HTTPStatus = f.status
			}
			if timedOut && row.HTTPStatus == 0 {
				row.Error = "download_timeout"
			}
		} else if failures.Load() > 0 {
			row.State = "partial"
			row.Error = "stream_failed"
		}
		if row.LimitReached || row.Seconds >= duration.Seconds() {
			row.Progress = 100
		}
		return row
	}
	for {
		select {
		case begin = <-started:
			lastTime = begin
			timer := time.NewTimer(duration)
			defer timer.Stop()
			deadline = timer.C
			connectTimer.Stop()
			row.State = "testing"
			finish()
			update(row)
		case <-ticker.C:
			finish()
			if !begin.IsZero() {
				now := time.Now()
				row.CurrentMbps = float64(row.Bytes-lastBytes) * 8 / now.Sub(lastTime).Seconds() / 1e6
				lastBytes = row.Bytes
				lastTime = now
			}
			update(row)
		case <-deadline:
			return result("done", false)
		case <-done:
			return result("done", false)
		case <-connectTimer.C:
			return result("failed", true)
		case <-parent.Done():
			return result("cancelled", false)
		}
	}
}

func SpeedTestCatalog(raw []byte) []map[string]string {
	var cfg struct {
		Outbounds []struct {
			Tag      string `json:"tag"`
			Protocol string `json:"protocol"`
		} `json:"outbounds"`
	}
	if json.Unmarshal(raw, &cfg) != nil {
		return nil
	}
	result := []map[string]string{}
	for _, o := range cfg.Outbounds {
		if _, err := speedWorkerConfig(raw, o.Tag, 12345, "test", "test"); err == nil {
			result = append(result, map[string]string{"tag": o.Tag, "protocol": o.Protocol})
		}
	}
	return result
}
