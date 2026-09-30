//go:build !windows

package xray

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func fakeCore(t *testing.T, body string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XUI_BIN_FOLDER", dir)
	t.Setenv("XUI_LOG_FOLDER", filepath.Join(dir, "logs"))
	script := "#!/bin/sh\nif [ \"$1\" = '-version' ]; then echo 'Xray test'; exit 0; fi\n" + body
	if err := os.WriteFile(GetBinaryPath(), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
}

func TestProcessRejectsMissingBinary(t *testing.T) {
	t.Setenv("XUI_BIN_FOLDER", t.TempDir())
	t.Setenv("XUI_LOG_FOLDER", t.TempDir())
	p := newProcess(&Config{})
	if p.Start() == nil || p.IsRunning() || p.GetErr() == nil {
		t.Fatal("missing executable reported running")
	}
}

func TestProcessRejectsEarlyExit(t *testing.T) {
	for _, body := range []string{"echo failed >&2\nexit 7\n", "exit 0\n"} {
		t.Run(body, func(t *testing.T) {
			fakeCore(t, body)
			p := newProcess(&Config{})
			if p.Start() == nil || p.IsRunning() {
				t.Fatal("early exit reported running")
			}
		})
	}
}

func TestProcessStopWaitsAndCanRestart(t *testing.T) {
	fakeCore(t, "trap 'exit 0' TERM\nwhile :; do sleep 0.1; done\n")
	p := newProcess(&Config{})
	t.Cleanup(func() { _ = p.Stop() })
	for i := 0; i < 2; i++ {
		if err := p.Start(); err != nil {
			t.Fatal(err)
		}
		if !p.IsRunning() {
			t.Fatal("not running")
		}
		if p.Start() == nil {
			t.Fatal("duplicate start allowed")
		}
		var wg sync.WaitGroup
		for n := 0; n < 3; n++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for k := 0; k < 30; k++ {
					_ = p.IsRunning()
					_ = p.GetErr()
					_ = p.GetResult()
					time.Sleep(time.Millisecond)
				}
			}()
		}
		if err := p.Stop(); err != nil {
			t.Fatal(err)
		}
		wg.Wait()
		if p.IsRunning() {
			t.Fatal("stop returned before process exit")
		}
		select {
		case <-p.done:
		default:
			t.Fatal("wait has not completed")
		}
	}
}
