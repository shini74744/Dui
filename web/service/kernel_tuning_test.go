package service

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func writeFakeSysctl(t *testing.T, root, key string, value int) {
	t.Helper()
	path := filepath.Join(root, strings.ReplaceAll(key, ".", "/"))
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strconv.Itoa(value)), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestKernelTuningApplyAndStatus(t *testing.T) {
	oldRoot, oldConfig, oldBackup, oldGeteuid := kernelProcRoot, kernelSysctlConfig, kernelOriginalBackup, kernelGeteuid
	defer func() {
		kernelProcRoot = oldRoot
		kernelSysctlConfig = oldConfig
		kernelOriginalBackup = oldBackup
		kernelGeteuid = oldGeteuid
	}()
	kernelGeteuid = func() int { return 0 }
	dir := t.TempDir()
	kernelProcRoot = filepath.Join(dir, "proc")
	kernelSysctlConfig = filepath.Join(dir, "99-dui-network.conf")
	kernelOriginalBackup = filepath.Join(dir, "kernel-tuning-original.json")

	initial := KernelTuningValues{
		TCPKeepaliveTime: 7200, TCPKeepaliveIntvl: 75, TCPKeepaliveProbes: 9,
		TCPFinTimeout: 60, TCPMaxSynBacklog: 4096, Somaxconn: 4096, NetdevMaxBacklog: 1000,
	}
	for _, p := range kernelTuningParams {
		writeFakeSysctl(t, kernelProcRoot, p.Key, kernelValue(initial, p.Field))
	}

	svc := &KernelTuningService{}
	status, err := svc.Status()
	if err != nil {
		t.Fatal(err)
	}
	if status.Persistent || status.Configured {
		t.Fatalf("unexpected initial status: %#v", status)
	}

	target := KernelTuningValues{
		TCPKeepaliveTime: 600, TCPKeepaliveIntvl: 30, TCPKeepaliveProbes: 5,
		TCPFinTimeout: 30, TCPMaxSynBacklog: 8192, Somaxconn: 8192, NetdevMaxBacklog: 8192,
	}
	status, err = svc.Apply(target)
	if err != nil {
		t.Fatal(err)
	}
	if !status.Configured || !status.Persistent {
		t.Fatalf("expected persisted status: %#v", status)
	}
	for _, p := range kernelTuningParams {
		got, err := readKernelInt(p.Key)
		if err != nil {
			t.Fatal(err)
		}
		if got != kernelValue(target, p.Field) {
			t.Fatalf("%s=%d, want %d", p.Key, got, kernelValue(target, p.Field))
		}
	}
	cfg, err := os.ReadFile(kernelSysctlConfig)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(cfg), "net.ipv4.tcp_keepalive_time = 600") {
		t.Fatalf("unexpected config: %s", cfg)
	}
	if !status.OriginalBackup || status.OriginalValues == nil {
		t.Fatalf("original snapshot was not exposed: %#v", status)
	}
	backupBefore, err := os.ReadFile(kernelOriginalBackup)
	if err != nil {
		t.Fatal(err)
	}

	target2 := KernelTuningValues{
		TCPKeepaliveTime: 900, TCPKeepaliveIntvl: 45, TCPKeepaliveProbes: 6,
		TCPFinTimeout: 45, TCPMaxSynBacklog: 16384, Somaxconn: 16384, NetdevMaxBacklog: 16384,
	}
	if _, err := svc.Apply(target2); err != nil {
		t.Fatal(err)
	}
	backupAfter, err := os.ReadFile(kernelOriginalBackup)
	if err != nil {
		t.Fatal(err)
	}
	if string(backupBefore) != string(backupAfter) {
		t.Fatal("original snapshot was overwritten by a later apply")
	}

	status, err = svc.RestoreOriginal()
	if err != nil {
		t.Fatal(err)
	}
	if !status.AtOriginal {
		t.Fatalf("expected original state after restore: %#v", status)
	}
	if status.Configured {
		t.Fatalf("initial config did not exist, restore should remove DUI config: %#v", status)
	}
	for _, p := range kernelTuningParams {
		got, err := readKernelInt(p.Key)
		if err != nil {
			t.Fatal(err)
		}
		if got != kernelValue(initial, p.Field) {
			t.Fatalf("restored %s=%d, want %d", p.Key, got, kernelValue(initial, p.Field))
		}
	}
}

func TestValidateKernelTuningRejectsUnsafeRange(t *testing.T) {
	supported := map[string]bool{"tcpFinTimeout": true}
	err := validateKernelTuning(KernelTuningValues{TCPFinTimeout: 1}, supported)
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestKernelTuningRestorePreservesPreexistingConfig(t *testing.T) {
	oldRoot, oldConfig, oldBackup, oldGeteuid := kernelProcRoot, kernelSysctlConfig, kernelOriginalBackup, kernelGeteuid
	defer func() {
		kernelProcRoot = oldRoot
		kernelSysctlConfig = oldConfig
		kernelOriginalBackup = oldBackup
		kernelGeteuid = oldGeteuid
	}()
	kernelGeteuid = func() int { return 0 }
	dir := t.TempDir()
	kernelProcRoot = filepath.Join(dir, "proc")
	kernelSysctlConfig = filepath.Join(dir, "99-dui-network.conf")
	kernelOriginalBackup = filepath.Join(dir, "kernel-tuning-original.json")
	initial := KernelTuningValues{
		TCPKeepaliveTime: 7200, TCPKeepaliveIntvl: 75, TCPKeepaliveProbes: 9,
		TCPFinTimeout: 60, TCPMaxSynBacklog: 4096, Somaxconn: 4096, NetdevMaxBacklog: 1000,
	}
	for _, p := range kernelTuningParams {
		writeFakeSysctl(t, kernelProcRoot, p.Key, kernelValue(initial, p.Field))
	}
	originalConfig := "# preexisting config\nnet.ipv4.ip_forward = 1\n"
	if err := os.WriteFile(kernelSysctlConfig, []byte(originalConfig), 0644); err != nil {
		t.Fatal(err)
	}

	target := KernelTuningValues{
		TCPKeepaliveTime: 600, TCPKeepaliveIntvl: 30, TCPKeepaliveProbes: 5,
		TCPFinTimeout: 30, TCPMaxSynBacklog: 8192, Somaxconn: 8192, NetdevMaxBacklog: 8192,
	}
	svc := &KernelTuningService{}
	if _, err := svc.Apply(target); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RestoreOriginal(); err != nil {
		t.Fatal(err)
	}
	gotConfig, err := os.ReadFile(kernelSysctlConfig)
	if err != nil {
		t.Fatal(err)
	}
	if string(gotConfig) != originalConfig {
		t.Fatalf("preexisting config not restored exactly: %q", gotConfig)
	}
	for _, p := range kernelTuningParams {
		got, err := readKernelInt(p.Key)
		if err != nil {
			t.Fatal(err)
		}
		if got != kernelValue(initial, p.Field) {
			t.Fatalf("restored %s=%d, want %d", p.Key, got, kernelValue(initial, p.Field))
		}
	}
}

func TestKernelTuningRejectsNonRootMutation(t *testing.T) {
	oldGeteuid := kernelGeteuid
	defer func() { kernelGeteuid = oldGeteuid }()
	kernelGeteuid = func() int { return 1000 }
	svc := &KernelTuningService{}
	if _, err := svc.Apply(KernelTuningValues{}); err == nil {
		t.Fatal("expected apply to reject non-root process")
	}
	if _, err := svc.RestoreOriginal(); err == nil {
		t.Fatal("expected restore to reject non-root process")
	}
}
