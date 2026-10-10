//go:build linux

package update

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestWireGuardValidationNetworkNamespace(t *testing.T) {
	probe := exec.Command("true")
	isolateValidation(probe)
	if err := probe.Run(); err != nil {
		if os.Getenv("DUI_TEST_NETNS") == "1" {
			t.Fatal(err)
		}
		t.Skip("network namespace permission unavailable")
	}
	parent, err := os.Readlink("/proc/self/ns/net")
	if err != nil {
		t.Fatal(err)
	}
	j := job(t, "core")
	j.Version = "vx-26.8"
	j.WorkDir = j.Root
	j.Core = j.Target
	j.Config = filepath.Join(j.Root, "config.json")
	config := []byte(`{"outbounds":[{"protocol":"wireguard","settings":{"noKernelTun":false}}]}`)
	if err := os.WriteFile(j.Config, config, 0600); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\nif [ \"$1\" = '-version' ]; then echo 'Xray vx-26.8'; exit 0; fi\ncat > received\nreadlink /proc/self/ns/net > namespace\n"
	if err := os.WriteFile(filepath.Join(j.Dir, "new"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	if err := ValidateBinary(j); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(j.Root, "namespace"))
	if string(got) == parent+"\n" || len(got) == 0 {
		t.Fatal("validation used the business network")
	}
	received, _ := os.ReadFile(filepath.Join(j.Root, "received"))
	saved, _ := os.ReadFile(j.Config)
	if string(received) != string(config) || string(saved) != string(config) {
		t.Fatal("configuration changed")
	}
	if j.NewHash == "" {
		t.Fatal("validated hash missing")
	}
	// Invalid configurations still fail, and the installed executable stays intact.
	script += "exit 23\n"
	if err := os.WriteFile(filepath.Join(j.Dir, "new"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	if err := ValidateBinary(j); err == nil || err.Error() != "config_rejected" {
		t.Fatal(err)
	}
	original, _ := os.ReadFile(j.Target)
	if string(original) != "original" {
		t.Fatal("installed core changed during validation")
	}
}
