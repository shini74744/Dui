//go:build linux

package update

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestInstallRollbackKeepsCompletePreviousVersion(t *testing.T) {
	for _, failRestart := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "rollback"}[failRestart], func(t *testing.T) {
			j := job(t, "panel")
			j.Version = "v26.9.47"
			j.Panel = j.Target
			j.DB = filepath.Join(j.Root, "db.sqlite")
			j.Config = filepath.Join(j.Root, "config.json")
			j.WorkDir = j.Root
			os.WriteFile(filepath.Join(j.Dir, "new"), []byte("#!/bin/sh\necho 26.9.47\n"), 0755)
			os.WriteFile(j.Config, []byte("{}"), 0600)
			db, e := sql.Open("sqlite3", j.DB)
			if e != nil {
				t.Fatal(e)
			}
			db.Exec("CREATE TABLE data(value TEXT)")
			db.Exec("INSERT INTO data VALUES('before')")
			db.Close()
			j.OldHash, _ = Hash(j.Target)
			prevCmd, prevRestart := command, restartAndWait
			defer func() { command = prevCmd; restartAndWait = prevRestart }()
			stopped := false
			command = func(name string, args ...string) (string, error) {
				if name != "systemctl" {
					t.Fatal(name)
				}
				stopped = args[0] == "stop"
				return "", nil
			}
			restartAndWait = func(j *Job, expected string) error {
				if !stopped {
					t.Fatal("replacement without stopped service")
				}
				h, _ := Hash(j.Target)
				if h != expected {
					t.Fatal("wrong installed hash")
				}
				if failRestart && expected == j.NewHash {
					return errors.New("health_check_failed")
				}
				return nil
			}
			e = install(j)
			if failRestart {
				if e == nil {
					t.Fatal("restart failure ignored")
				}
				if e = rollback(j); e != nil {
					t.Fatal(e)
				}
				h, _ := Hash(j.Target)
				if h != j.OldHash || j.Phase != "rolled_back" {
					t.Fatal("previous executable not restored")
				}
			} else {
				if e != nil {
					t.Fatal(e)
				}
				h, _ := Hash(j.Target)
				if h != j.NewHash {
					t.Fatal("new executable missing")
				}
			}
			backup, _ := os.ReadFile(filepath.Join(j.Dir, "previous"))
			if string(backup) != "original" {
				t.Fatal("backup corrupt")
			}
		})
	}
}
func TestRejectBadBinaryBeforeServiceStop(t *testing.T) {
	j := job(t, "panel")
	j.Version = "v26.9.47"
	os.WriteFile(filepath.Join(j.Dir, "new"), []byte("#!/bin/sh\necho 26.9.46\n"), 0755)
	prev := command
	defer func() { command = prev }()
	command = func(string, ...string) (string, error) {
		t.Fatal("service touched before verification")
		return "", nil
	}
	if e := install(j); e == nil {
		t.Fatal("wrong binary accepted")
	}
	original, _ := os.ReadFile(j.Target)
	if string(original) != "original" {
		t.Fatal("live changed")
	}
}
func TestConcurrentWorkerGuard(t *testing.T) {
	root := t.TempDir()
	f, e := lockRoot(root)
	if e != nil {
		t.Fatal(e)
	}
	defer unlock(f)
	if other, e := lockRoot(root); e == nil {
		unlock(other)
		t.Fatal("concurrent update accepted")
	}
}

func TestWorkerWaitsForStarterLock(t *testing.T) {
	root := t.TempDir()
	f, e := lockRoot(root)
	if e != nil {
		t.Fatal(e)
	}
	done := make(chan int, 1)
	go func() { done <- Run(root, "missing-test-job") }()
	select {
	case <-done:
		unlock(f)
		t.Fatal("worker raced starter lock")
	case <-time.After(150 * time.Millisecond):
	}
	unlock(f)
	select {
	case code := <-done:
		if code != 1 {
			t.Fatal(code)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not acquire released lock")
	}
}
