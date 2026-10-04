//go:build linux

package update

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	_ "github.com/mattn/go-sqlite3"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var command = func(name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	b, e := exec.CommandContext(ctx, name, args...).Output()
	return strings.TrimSpace(string(b)), e
}

func Supported(panel string) bool {
	if os.Geteuid() != 0 {
		return false
	}
	pid, e := command("systemctl", "show", "x-ui.service", "-p", "MainPID", "--value")
	if e != nil || pid != strconv.Itoa(os.Getpid()) {
		return false
	}
	installed, e := filepath.EvalSymlinks("/proc/" + pid + "/exe")
	return e == nil && installed == panel
}
func lockRoot(root string) (*os.File, error) {
	if e := os.MkdirAll(root, 0700); e != nil {
		return nil, e
	}
	f, e := os.OpenFile(filepath.Join(root, "lock"), os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return nil, e
	}
	if e = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); e != nil {
		f.Close()
		return nil, errors.New("update_busy")
	}
	return f, nil
}
func unlock(f *os.File) { syscall.Flock(int(f.Fd()), syscall.LOCK_UN); f.Close() }
func unitActive(unit string) bool {
	s, e := command("systemctl", "is-active", unit)
	return e == nil && (s == "active" || s == "activating")
}
func Start(j Job) (Job, error) {
	if !ValidVersion(j.Kind, j.Version) {
		return j, errors.New("invalid_release")
	}
	if !Supported(j.Panel) {
		return j, errors.New("unsupported_installation")
	}
	f, e := lockRoot(j.Root)
	if e != nil {
		return j, e
	}
	defer unlock(f)
	if old, e := Load(j.Root); e == nil && old.Busy() {
		if unitActive(old.Unit) || time.Since(old.UpdatedAt) < 30*time.Second {
			return j, errors.New("update_busy")
		}
		if old.Applied || old.BackupReady {
			return j, errors.New("recovery_required")
		}
	}
	j.ID = fmt.Sprintf("%d-%d", time.Now().UnixNano(), os.Getpid())
	j.Unit = "dui-update-" + j.ID
	j.Dir, e = os.MkdirTemp(filepath.Dir(j.Target), ".dui-update-")
	if e != nil {
		return j, errors.New("disk_write_failed")
	}
	j.Worker = filepath.Join(j.Dir, "worker")
	// The worker belongs to a separate systemd service, so restarting x-ui cannot kill it.
	if e = Copy(j.Panel, j.Worker, 0700); e != nil {
		os.RemoveAll(j.Dir)
		return j, e
	}
	j.Phase = "queued"
	j.StartedAt = time.Now().UTC()
	if e = j.Save(); e != nil {
		return j, e
	}
	_, e = command("systemd-run", "--quiet", "--collect", "--unit="+j.Unit,
		"--property=Type=exec", "--property=UMask=0077", "--property=WorkingDirectory="+j.WorkDir,
		j.Worker, "update-worker", j.Root, j.ID)
	if e != nil {
		j.Phase = "failed"
		j.Error = "worker_start_failed"
		j.Save()
		os.RemoveAll(j.Dir)
		return j, errors.New(j.Error)
	}
	return j, nil
}

// Ready is written only after the web server and its initial Xray startup completed.
func Ready(root string, coreRunning bool) {
	j, e := Load(root)
	if e != nil || !j.Busy() {
		return
	}
	_ = WriteJSON(filepath.Join(root, "ready-"+j.ID+".json"), map[string]any{"pid": os.Getpid(), "coreRunning": coreRunning})
}
func healthy(j *Job, expected string) bool {
	pid, e := command("systemctl", "show", "x-ui.service", "-p", "MainPID", "--value")
	if e != nil || pid == "" || pid == "0" {
		return false
	}
	var r struct {
		PID         int  `json:"pid"`
		CoreRunning bool `json:"coreRunning"`
	}
	b, e := os.ReadFile(filepath.Join(j.Root, "ready-"+j.ID+".json"))
	if e != nil || json.Unmarshal(b, &r) != nil || strconv.Itoa(r.PID) != pid {
		return false
	}
	if j.CoreRunning && !r.CoreRunning {
		return false
	}
	target := j.Target
	if j.Kind == "panel" {
		target = "/proc/" + pid + "/exe"
	}
	h, e := Hash(target)
	if e != nil || h != expected {
		return false
	}
	if j.Kind == "core" && j.CoreRunning {
		// Verify the running child executable, rather than merely the on-disk file.
		// /proc/PID/task/PID/children only covers children spawned by the main
		// thread. Go may start Xray on any OS thread, so inspect every task.
		tasks, e := filepath.Glob("/proc/" + pid + "/task/*/children")
		if e != nil {
			return false
		}
		match := false
		for _, task := range tasks {
			children, e := os.ReadFile(task)
			if e != nil {
				continue
			}
			for _, c := range strings.Fields(string(children)) {
				if h, e := Hash("/proc/" + c + "/exe"); e == nil && h == expected {
					match = true
					break
				}
			}
			if match {
				break
			}
		}
		if !match {
			return false
		}
	}
	return true
}

var restartAndWait = func(j *Job, expected string) error {
	os.Remove(filepath.Join(j.Root, "ready-"+j.ID+".json"))
	if _, e := command("systemctl", "restart", "x-ui.service"); e != nil {
		return errors.New("restart_failed")
	}
	deadline := time.Now().Add(90 * time.Second)
	stable := 0
	for time.Now().Before(deadline) {
		if healthy(j, expected) {
			stable++
			if stable >= 3 {
				return nil
			}
		} else {
			stable = 0
		}
		time.Sleep(2 * time.Second)
	}
	return errors.New("health_check_failed")
}

func backupDB(j *Job) error {
	db, e := sql.Open("sqlite3", j.DB+"?_busy_timeout=5000")
	if e != nil {
		return e
	}
	defer db.Close()
	_, e = db.Exec("VACUUM INTO ?", filepath.Join(j.Dir, "database.backup"))
	return e
}
func restoreFile(src, dst string, mode os.FileMode) error {
	temp, e := os.CreateTemp(filepath.Dir(dst), ".dui-restore-")
	if e != nil {
		return e
	}
	name := temp.Name()
	temp.Close()
	os.Remove(name)
	defer os.Remove(name)
	if e = Copy(src, name, mode); e != nil {
		return e
	}
	if e = os.Rename(name, dst); e != nil {
		return e
	}
	return syncDir(filepath.Dir(dst))
}
func rollback(j *Job) error {
	j.Phase = "rolling_back"
	if e := j.Save(); e != nil {
		return e
	}
	if _, e := command("systemctl", "stop", "x-ui.service"); e != nil {
		return errors.New("rollback_failed")
	}
	if j.Applied {
		if e := restoreFile(filepath.Join(j.Dir, "previous"), j.Target, 0755); e != nil {
			return errors.New("rollback_failed")
		}
	}
	if j.BackupReady {
		// The service is stopped; stale SQLite sidecars must not be replayed onto the restored DB.
		for _, suffix := range []string{"-wal", "-shm"} {
			if e := os.Remove(j.DB + suffix); e != nil && !os.IsNotExist(e) {
				return errors.New("rollback_failed")
			}
		}
		if e := restoreFile(filepath.Join(j.Dir, "database.backup"), j.DB, 0600); e != nil {
			return errors.New("rollback_failed")
		}
		if e := restoreFile(filepath.Join(j.Dir, "config.backup"), j.Config, 0600); e != nil {
			return errors.New("rollback_failed")
		}
	}
	if e := restartAndWait(j, j.OldHash); e != nil {
		return errors.New("rollback_failed")
	}
	j.Applied = false
	j.Phase = "rolled_back"
	return j.Save()
}
func install(j *Job) error {
	// Validate again immediately before stopping anything.
	if e := ValidateBinary(j); e != nil {
		return e
	}
	before, e := Hash(j.Target)
	if e != nil {
		return e
	}
	if before != j.OldHash {
		return errors.New("installed_version_changed")
	}
	if e = j.phase("preparing"); e != nil {
		return e
	}
	if e = Copy(j.Target, filepath.Join(j.Dir, "previous"), 0755); e != nil {
		return errors.New("backup_failed")
	}
	if _, e = command("systemctl", "stop", "x-ui.service"); e != nil {
		return errors.New("stop_failed")
	}
	recoverBeforeApply := func(err error) error { _, _ = command("systemctl", "start", "x-ui.service"); return err }
	if e = backupDB(j); e != nil {
		return recoverBeforeApply(errors.New("backup_failed"))
	}
	if e = Copy(j.Config, filepath.Join(j.Dir, "config.backup"), 0600); e != nil {
		return recoverBeforeApply(errors.New("backup_failed"))
	}
	j.BackupReady = true
	// Persist intent first; crash recovery can always restore the previous binary.
	j.Applied = true
	if e = j.phase("installing"); e != nil {
		return recoverBeforeApply(e)
	}
	if e = os.Chmod(filepath.Join(j.Dir, "new"), 0755); e != nil {
		return e
	}
	if e = os.Rename(filepath.Join(j.Dir, "new"), j.Target); e != nil {
		return e
	}
	if e = syncDir(filepath.Dir(j.Target)); e != nil {
		return e
	}
	if e = j.phase("restarting"); e != nil {
		return e
	}
	return restartAndWait(j, j.NewHash)
}
func Run(root, id string) int {
	// Start holds the lock until systemd acknowledges launching this process.
	// Wait briefly for that handoff, while still rejecting a second real worker.
	var f *os.File
	var e error
	for attempt := 0; attempt < 100; attempt++ {
		f, e = lockRoot(root)
		if e == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if e != nil {
		return 1
	}
	defer unlock(f)
	j, e := Load(root)
	if e != nil || j.ID != id || j.Root != root || os.Geteuid() != 0 {
		return 1
	}
	fail := func(e error) int {
		j.Error = e.Error()
		if j.Applied || j.BackupReady {
			if re := rollback(&j); re != nil {
				j.Error = "rollback_failed"
				j.Phase = "failed"
				j.Save()
				return 1
			}
		} else {
			j.Phase = "failed"
			j.Save()
		}
		// Keep verified backups, but remove large failed/partial downloads.
		os.Remove(filepath.Join(j.Dir, "download.part"))
		os.Remove(filepath.Join(j.Dir, "package"))
		os.Remove(filepath.Join(j.Dir, "new"))
		return 1
	}
	if j.Applied || j.BackupReady {
		return fail(errors.New("interrupted_update"))
	}
	defer os.Remove(j.Worker)
	if e = j.phase("connecting"); e != nil {
		return fail(e)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	offer, e := Resolve(ctx, Client(), j.Kind, j.Version)
	cancel()
	if e != nil {
		return fail(e)
	}
	j.Offer = offer
	j.Total = offer.Size
	j.Save()
	j.OldHash, e = Hash(j.Target)
	if e != nil {
		return fail(errors.New("installed_binary_missing"))
	}
	if e = Download(context.Background(), Client(), &j); e != nil {
		return fail(e)
	}
	if e = Extract(&j); e != nil {
		return fail(e)
	}
	if e = ValidateBinary(&j); e != nil {
		return fail(e)
	}
	if e = install(&j); e != nil {
		return fail(e)
	}
	j.Phase = "complete"
	j.Error = ""
	if e = j.Save(); e != nil {
		return 1
	}
	os.Remove(filepath.Join(j.Dir, "package"))
	return 0
}

func Status(root string) (Job, error) {
	j, e := Load(root)
	if e != nil {
		return j, e
	}
	if j.Busy() && time.Since(j.UpdatedAt) > 30*time.Second && !unitActive(j.Unit) {
		j.Phase = "failed"
		j.Error = "interrupted_update"
		if j.Applied || j.BackupReady {
			j.Error = "recovery_required"
		}
	}
	return j, nil
}
