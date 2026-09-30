package tuic

import (
	"strings"
	"testing"
)

func TestActivityRequiresAuthenticatedUserField(t *testing.T) {
	const a = "11111111-2222-3333-4444-555555555555"
	const b = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	w := &procLogWriter{uuidToEmail: map[string]string{a: "alice", b: "bob"}}
	w.Write([]byte("[0x12345678] [127.0.0.1:1234] [unauthenticated] authentication failed: " + a + "\n"))
	if len(w.lastActive) != 0 {
		t.Fatal("failed authentication marked user online")
	}
	w.Write([]byte("[0x12345678] [[::1]:1234] [" + a + "] [connect] [" + b + "]:443\n"))
	if len(w.lastActive) != 1 || w.lastActive["alice"] == 0 {
		t.Fatal("wrong authenticated user tracked")
	}
	p := &Process{logWriter: w}
	p.UpdateClients(map[string]string{b: "bob"})
	if len(w.lastActive) != 0 {
		t.Fatal("removed user remains in activity cache")
	}
	w.Write([]byte(strings.Repeat("x", 128*1024)))
	if len(w.buf) > 64*1024 {
		t.Fatal("unterminated log line is unbounded")
	}
}
