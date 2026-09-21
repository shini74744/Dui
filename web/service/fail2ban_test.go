package service

import (
	"os"
	"testing"
	"time"
)

func TestParseFail2banDuration(t *testing.T) {
	tests := []struct {
		input string
		want  time.Duration
		ok    bool
	}{
		{"600", 600 * time.Second, true},
		{"30m", 30 * time.Minute, true},
		{"1h", time.Hour, true},
		{"1d", 24 * time.Hour, true},
		{"7d", 7 * 24 * time.Hour, true},
		{"1w", 7 * 24 * time.Hour, true},
		{" 2H ", 2 * time.Hour, true},
		{"", 0, false},
		{"1x", 0, false},
		{"-1", 0, false},
	}

	for _, tt := range tests {
		got, ok := parseFail2banDuration(tt.input)
		if ok != tt.ok {
			t.Fatalf("parseFail2banDuration(%q) ok=%v, want %v", tt.input, ok, tt.ok)
		}
		if ok && got != tt.want {
			t.Fatalf("parseFail2banDuration(%q)=%v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestReadFail2banLogsRespectsFindTime(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/fail2ban.log"
	now := time.Date(2026, 9, 21, 12, 0, 0, 0, time.Local)
	content := "" +
		"2026-09-21 08:00:00,000 fail2ban.filter [123]: INFO [sshd] Found 203.0.113.10\n" +
		"2026-09-21 11:30:00,000 fail2ban.actions [123]: NOTICE [sshd] Ban 203.0.113.11\n" +
		"2026-09-21 11:40:00,000 fail2ban.filter [123]: INFO [3xui-tls] Found 203.0.113.12\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	rows := readFail2banLogsFromPaths("sshd", 100, "1h", now, []string{path})
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1: %#v", len(rows), rows)
	}
	if rows[0].IP != "203.0.113.11" || rows[0].Event != "Ban" {
		t.Fatalf("unexpected row: %#v", rows[0])
	}

	rows = readFail2banLogsFromPaths("sshd", 100, "7d", now, []string{path})
	if len(rows) != 2 {
		t.Fatalf("7d window got %d rows, want 2: %#v", len(rows), rows)
	}
}
