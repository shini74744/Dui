package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"x-ui/config"
	"x-ui/database"
	"x-ui/web/global"
	"x-ui/xray"
)

var databaseRestoreMu sync.Mutex

// Runs in a disposable process. Migration and config generation must never
// publish the uploaded database as the live process's database connection.
func ValidateStoredDatabase(path string) error {
	if err := database.InitDB(path); err != nil {
		return err
	}
	defer database.CloseDB()
	var integrity string
	if err := database.GetDB().Raw("PRAGMA quick_check").Scan(&integrity).Error; err != nil {
		return err
	}
	if integrity != "ok" {
		return errors.New("数据库完整性检查未通过")
	}
	if err := (&InboundService{}).MigrationRequirements(); err != nil {
		return err
	}
	cfg, err := (&XrayService{}).buildXrayConfig(false)
	if err != nil {
		return err
	}
	return xray.ValidateConfig(cfg)
}

func validateDatabaseCandidate(path string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, "validate-db", path)
	// Config diagnostics may contain credentials. Keep them out of HTTP replies.
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return errors.New("备份校验超时，当前配置未更改")
		}
		return errors.New("备份数据库或核心配置校验失败，当前配置未更改")
	}
	return nil
}

func copyDatabaseFile(from, to string) error {
	src, err := os.Open(from)
	if err != nil {
		return err
	}
	defer src.Close()
	dst, err := os.OpenFile(to, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(dst, src)
	syncErr := dst.Sync()
	closeErr := dst.Close()
	return errors.Join(copyErr, syncErr, closeErr)
}

// Wait for the restored core to remain alive and expose its API. A successful
// process spawn alone does not prove listeners were opened successfully.
func (s *ServerService) verifyRestoredCore() error {
	deadline := time.Now().Add(10 * time.Second)
	stableSince := time.Now()
	for time.Now().Before(deadline) {
		if !s.xrayService.IsXrayRunning() {
			return errors.New("恢复后的核心启动失败")
		}
		ready := true
		if port := s.xrayService.GetApiPort(); port > 0 {
			conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 200*time.Millisecond)
			ready = err == nil
			if conn != nil {
				conn.Close()
			}
		}
		if !ready {
			stableSince = time.Now()
		}
		if ready && time.Since(stableSince) >= time.Second {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return errors.New("恢复后的核心未就绪")
}

func (s *ServerService) ImportDB(file multipart.File) error {
	databaseRestoreMu.Lock()
	defer databaseRestoreMu.Unlock()
	valid, err := database.IsSQLiteDB(file)
	if err != nil || !valid {
		return errors.New("备份不是有效的 SQLite 数据库")
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	dbPath := config.GetDBPath()
	stage, err := os.MkdirTemp(filepath.Dir(dbPath), ".dui-restore-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	candidate := filepath.Join(stage, "candidate.db")
	f, err := os.OpenFile(candidate, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(f, file)
	syncErr := f.Sync()
	closeErr := f.Close()
	if err := errors.Join(copyErr, syncErr, closeErr); err != nil {
		return err
	}
	if err := validateDatabaseCandidate(candidate); err != nil {
		return err
	}

	// Drain scheduled work and regular HTTP requests before closing SQLite.
	if server := global.GetWebServer(); server != nil {
		cron := server.GetCron()
		if cron != nil {
			stopped := cron.Stop()
			defer cron.Start()
			select {
			case <-stopped.Done():
			case <-time.After(60 * time.Second):
				return errors.New("后台任务繁忙，当前配置未更改，请稍后重试")
			}
		}
	}
	deadline := time.Now().Add(60 * time.Second)
	for !database.MaintenanceMu.TryLock() {
		if time.Now().After(deadline) {
			return errors.New("面板请求繁忙，当前配置未更改，请稍后重试")
		}
		time.Sleep(20 * time.Millisecond)
	}
	defer database.MaintenanceMu.Unlock()
	wasRunning := s.xrayService.IsXrayRunning()
	if wasRunning {
		if err := s.StopXrayService(); err != nil {
			return err
		}
	}
	backup := fmt.Sprintf("%s.backup-%d", dbPath, time.Now().UnixNano())
	backedUp := false
	restore := func(cause error) error {
		if s.xrayService.IsXrayRunning() {
			_ = s.StopXrayService()
		}
		_ = database.CloseDB()
		if backedUp {
			recovery := filepath.Join(stage, "recovery.db")
			if err := copyDatabaseFile(backup, recovery); err != nil {
				return errors.Join(cause, fmt.Errorf("恢复备份失败，备份已保留：%w", err))
			}
			// SQLite was closed, so no connection can retain a stale WAL for this path.
			_ = os.Remove(dbPath + "-wal")
			_ = os.Remove(dbPath + "-shm")
			if err := os.Rename(recovery, dbPath); err != nil {
				return errors.Join(cause, fmt.Errorf("恢复数据库失败，备份已保留：%w", err))
			}
		}
		if err := database.InitDB(dbPath); err != nil {
			return errors.Join(cause, err)
		}
		if wasRunning {
			if err := s.RestartXrayService(); err != nil {
				return errors.Join(cause, fmt.Errorf("恢复原核心失败：%w", err))
			}
		}
		return fmt.Errorf("导入失败，已恢复原配置：%w", cause)
	}
	if err := database.CloseDB(); err != nil {
		return restore(err)
	}
	if err := os.Rename(dbPath, backup); err != nil {
		return restore(err)
	}
	backedUp = true
	if err := os.Chmod(backup, 0600); err != nil {
		return restore(err)
	}
	if err := os.Rename(candidate, dbPath); err != nil {
		return restore(err)
	}
	if err := database.InitDB(dbPath); err != nil {
		return restore(err)
	}
	if wasRunning {
		if err := s.RestartXrayService(); err != nil {
			return restore(err)
		}
		if err := s.verifyRestoredCore(); err != nil {
			return restore(err)
		}
	}
	// Retain the uniquely named backup, including after successful activation.
	return nil
}
