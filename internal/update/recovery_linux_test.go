//go:build linux

package update

import (
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestInstalledVersionWithoutCoreProcess(t *testing.T) {
	for _, tc := range []struct{ output, want string }{
		{"Xray vx-26.6 (Xray) build", "vx-26.6"},
		{"Xray 26.9.9 (Xray) build", "26.9.9"},
		{"Unknown", ""},
		{"Unexpected vx-26.7", ""},
		{"Xray invalid", ""},
	} {
		path := filepath.Join(t.TempDir(), "core")
		os.WriteFile(path, []byte("#!/bin/sh\nprintf '%s\\n' '"+tc.output+"'\n"), 0755)
		got, err := BinaryVersion(path, "core")
		if got != tc.want || (err != nil) != (tc.want == "") {
			t.Fatalf("%q: %q, %v", tc.output, got, err)
		}
	}
}

func TestMissingConfigSnapshotValidation(t *testing.T) {
	for _, mode := range []string{"snapshot", "none", "invalid_snapshot", "live_rejected"} {
		t.Run(mode, func(t *testing.T) {
			j := job(t, "core")
			j.Config = filepath.Join(j.Root, "config.json")
			j.Core = j.Target
			j.WorkDir = j.Root
			body := "#!/bin/sh\nif [ \"$1\" = -version ]; then echo 'Xray vx-26.7'; exit 0; fi\nread data\n[ \"$data\" = '{\"outbounds\":[{\"protocol\":\"freedom\"}]}' ]\n"
			os.WriteFile(filepath.Join(j.Dir, "new"), []byte(body), 0755)
			j.ValidationConfig = json.RawMessage(`{"outbounds":[{"protocol":"freedom"}]}`)
			if mode == "none" {
				j.ValidationConfig = nil
			}
			if mode == "invalid_snapshot" {
				j.ValidationConfig = json.RawMessage("{")
			}
			if mode == "live_rejected" {
				os.WriteFile(j.Config, []byte(`{"changed":true}`), 0600)
			}
			err := ValidateBinary(j)
			if (err == nil) != (mode == "snapshot") {
				t.Fatalf("%v", err)
			}
			if mode != "live_rejected" {
				if _, err := os.Stat(j.Config); !os.IsNotExist(err) {
					t.Fatal("validation wrote live config")
				}
			}
			if _, ok := j.Public()["ValidationConfig"]; ok {
				t.Fatal("private snapshot exposed")
			}
			if _, ok := j.Public()["validationConfig"]; ok {
				t.Fatal("private snapshot exposed")
			}
		})
	}
}

func TestMissingConfigInstallAndRollback(t *testing.T) {
	for _, kind := range []string{"core", "panel"} {
		for _, fail := range []bool{false, true} {
			name := kind + "-success"
			if fail {
				name = kind + "-rollback"
			}
			t.Run(name, func(t *testing.T) {
				j := job(t, kind)
				j.Panel = j.Target
				j.Core = j.Target
				j.WorkDir = j.Root
				j.DB = filepath.Join(j.Root, "database")
				j.Config = filepath.Join(j.Root, "config.json")
				wasRunning := false
				j.PreviousCoreRunning = &wasRunning
				j.CoreRunning = kind == "core"
				j.ValidationConfig = json.RawMessage(`{"outbounds":[{"protocol":"freedom"}]}`)
				output := "Xray vx-26.7"
				if kind == "panel" {
					j.Version = "v26.9.50"
					output = "26.9.50"
				}
				os.WriteFile(filepath.Join(j.Dir, "new"), []byte("#!/bin/sh\ncase \"$1\" in -v|-version) echo '"+output+"';; *) cat >/dev/null;; esac\n"), 0755)
				db, err := sql.Open("sqlite3", j.DB)
				if err != nil {
					t.Fatal(err)
				}
				if _, err = db.Exec("CREATE TABLE data(value TEXT); INSERT INTO data VALUES('before')"); err != nil {
					t.Fatal(err)
				}
				db.Close()
				sentinel := filepath.Join(j.Root, "unrelated")
				os.WriteFile(sentinel, []byte("untouched"), 0600)
				j.OldHash, _ = Hash(j.Target)
				prevCommand, prevRestart := command, restartAndWait
				defer func() { command = prevCommand; restartAndWait = prevRestart }()
				command = func(name string, args ...string) (string, error) {
					if name != "systemctl" {
						t.Fatal(name)
					}
					return "", nil
				}
				restartAndWait = func(check *Job, expected string) error {
					h, _ := Hash(check.Target)
					if h != expected {
						t.Fatal("incorrect executable")
					}
					if expected == j.NewHash {
						if kind == "core" && !check.CoreRunning {
							t.Fatal("new core must become healthy")
						}
						os.WriteFile(j.Config, []byte("generated-during-start"), 0600)
						if fail {
							return errors.New("health_check_failed")
						}
					} else {
						if check.CoreRunning {
							t.Fatal("rollback wrongly requires previously stopped core")
						}
						if _, err := os.Stat(j.Config); !os.IsNotExist(err) {
							t.Fatal("missing configuration was not restored")
						}
					}
					return nil
				}
				err = install(j)
				if fail {
					if err == nil {
						t.Fatal("expected failed health check")
					}
					if err = rollback(j); err != nil {
						t.Fatal(err)
					}
					h, _ := Hash(j.Target)
					if h != j.OldHash || j.Phase != "rolled_back" {
						t.Fatal("rollback lost original")
					}
				} else if err != nil {
					t.Fatal(err)
				}
				if !j.ConfigAbsent || !j.BackupReady {
					t.Fatal("original absence not recorded")
				}
				b, _ := os.ReadFile(sentinel)
				if string(b) != "untouched" {
					t.Fatal("unrelated file changed")
				}
			})
		}
	}
}
