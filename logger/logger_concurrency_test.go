package logger

import (
	"sync"
	"testing"
)

func TestConcurrentBufferReadsAndWrites(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 1500; j++ {
				addToBuffer("INFO", "concurrent buffer")
				_ = GetLogs(10, "DEBUG")
			}
		}()
	}
	wg.Wait()
	bufferMu.RLock()
	defer bufferMu.RUnlock()
	if len(logBuffer) > 10240 {
		t.Fatalf("unbounded buffer: %d", len(logBuffer))
	}
}
