package tuic

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"x-ui/config"
	"x-ui/logger"
)

func GetBinaryName() string {
	name := fmt.Sprintf("tuic-server-%s-%s", runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return name
}

func GetBinaryPath() string {
	return filepath.Join(config.GetBinFolderPath(), GetBinaryName())
}

var (
	gracefulStopTimeout = 5 * time.Second
	forceStopTimeout    = 2 * time.Second
)

type procLogWriter struct {
	mu          sync.Mutex
	label       string
	buf         string
	lastLine    string
	uuidToEmail map[string]string
	lastActive  map[string]int64
}

// Pinned tuic-server emits [connection] [address] [authenticated user].
// Auth failures also mention the supplied UUID, so a substring match must not
// start a user's first-use expiry or report that user as online.
var authenticatedUserLog = regexp.MustCompile(`\[0x[0-9a-f]+\] \[(?:\[[0-9a-f:.%]+\]|[0-9.]+):[0-9]+\] \[([0-9a-f-]{36})\]`)

func (w *procLogWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf += string(p)
	for {
		i := strings.IndexByte(w.buf, '\n')
		if i < 0 {
			break
		}
		line := w.buf[:i]
		w.buf = w.buf[i+1:]
		w.emitLocked(line)
	}
	if len(w.buf) > 64*1024 {
		w.buf = w.buf[len(w.buf)-64*1024:]
	}
	return len(p), nil
}

func (w *procLogWriter) Flush() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.buf != "" {
		line := w.buf
		w.buf = ""
		w.emitLocked(line)
	}
}

func (w *procLogWriter) emitLocked(line string) {
	trimmed := strings.TrimSpace(strings.TrimRight(line, "\r"))
	if trimmed == "" {
		return
	}
	w.lastLine = trimmed
	logger.Infof("tuic: tuic-server %s | %s", w.label, trimmed)

	now := time.Now().UnixMilli()
	lowerLine := strings.ToLower(line)
	match := authenticatedUserLog.FindStringSubmatch(lowerLine)
	if len(match) == 2 {
		if email, ok := w.uuidToEmail[match[1]]; ok {
			if w.lastActive == nil {
				w.lastActive = make(map[string]int64)
			}
			w.lastActive[email] = now
		}
	}
}

func (w *procLogWriter) LastLine() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.lastLine
}

type Process struct {
	mu              sync.RWMutex
	cmd             *exec.Cmd
	done            chan struct{}
	configPath      string
	logWriter       *procLogWriter
	exitErr         error
	intentionalStop atomic.Bool
}

func newProcess(configPath, label string, uuidToEmail map[string]string) *Process {
	return &Process{
		configPath: configPath,
		logWriter: &procLogWriter{
			label:       label,
			uuidToEmail: uuidToEmail,
			lastActive:  make(map[string]int64),
		},
	}
}

func (p *Process) GetActiveEmails(window time.Duration) []string {
	if p == nil || p.logWriter == nil {
		return nil
	}
	p.logWriter.mu.Lock()
	defer p.logWriter.mu.Unlock()
	cutoff := time.Now().Add(-window).UnixMilli()
	var active []string
	for email, last := range p.logWriter.lastActive {
		if last >= cutoff {
			active = append(active, email)
		}
	}
	return active
}

func (p *Process) UpdateClients(uuidToEmail map[string]string) {
	if p == nil || p.logWriter == nil {
		return
	}
	p.logWriter.mu.Lock()
	defer p.logWriter.mu.Unlock()
	p.logWriter.uuidToEmail = uuidToEmail
	keep := make(map[string]bool, len(uuidToEmail))
	for _, email := range uuidToEmail {
		keep[email] = true
	}
	for email := range p.logWriter.lastActive {
		if !keep[email] {
			delete(p.logWriter.lastActive, email)
		}
	}
}

func (p *Process) IsRunning() bool {
	p.mu.RLock()
	cmd, done := p.cmd, p.done
	p.mu.RUnlock()
	if cmd == nil || cmd.Process == nil {
		return false
	}
	if done != nil {
		select {
		case <-done:
			return false
		default:
		}
	}
	return true
}

func (p *Process) GetResult() string {
	if line := p.logWriter.LastLine(); line != "" {
		return line
	}
	p.mu.RLock()
	exitErr := p.exitErr
	p.mu.RUnlock()
	if exitErr != nil {
		return exitErr.Error()
	}
	return ""
}

func (p *Process) Start() error {
	if p.IsRunning() {
		return errors.New("tuic-server is already running")
	}
	cmd := exec.CommandContext(context.Background(), GetBinaryPath(), "-c", p.configPath)
	cmd.Stdout = p.logWriter
	cmd.Stderr = p.logWriter
	done := make(chan struct{})
	p.mu.Lock()
	p.cmd = cmd
	p.done = done
	p.exitErr = nil
	p.mu.Unlock()
	p.intentionalStop.Store(false)
	if err := cmd.Start(); err != nil {
		close(done)
		p.mu.Lock()
		p.cmd = nil
		p.mu.Unlock()
		return err
	}
	attachChildLifetime(cmd)
	go p.wait(cmd, done)
	return nil
}

func (p *Process) wait(cmd *exec.Cmd, done chan struct{}) {
	defer close(done)
	err := cmd.Wait()
	p.logWriter.Flush()
	if err == nil || p.intentionalStop.Load() {
		return
	}
	if runtime.GOOS == "windows" {
		if strings.Contains(strings.ToLower(err.Error()), "exit status 1") {
			p.setExitErr(err)
			return
		}
	}
	logger.Errorf("tuic: tuic-server process exited: %v", err)
	p.setExitErr(err)
}

func (p *Process) setExitErr(err error) {
	p.mu.Lock()
	p.exitErr = err
	p.mu.Unlock()
}

func (p *Process) Stop() error {
	if !p.IsRunning() {
		return errors.New("tuic-server is not running")
	}
	p.intentionalStop.Store(true)
	p.mu.RLock()
	cmd, done := p.cmd, p.done
	p.mu.RUnlock()
	if cmd == nil || cmd.Process == nil {
		return errors.New("tuic-server is not running")
	}

	if runtime.GOOS == "windows" {
		if err := cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			return err
		}
		return waitForExit(done, forceStopTimeout)
	}

	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		if errors.Is(err, os.ErrProcessDone) {
			return waitForExit(done, forceStopTimeout)
		}
		return err
	}

	if err := waitForExit(done, gracefulStopTimeout); err == nil {
		return nil
	}

	logger.Warning("tuic: tuic-server did not stop after SIGTERM, killing process")
	if err := cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return err
	}
	return waitForExit(done, forceStopTimeout)
}

func waitForExit(done <-chan struct{}, timeout time.Duration) error {
	if done == nil {
		return nil
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-done:
		return nil
	case <-timer.C:
		return fmt.Errorf("timed out waiting for tuic-server process to stop after %s", timeout)
	}
}
