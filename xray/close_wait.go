package xray

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

func closeWaitSeconds(cfg *Config) (int, error) {
	if cfg == nil || len(cfg.Policy) == 0 {
		return 0, nil
	}
	var p struct {
		System struct {
			Seconds int `json:"duiCloseWaitTimeout"`
		} `json:"system"`
	}
	if err := json.Unmarshal(cfg.Policy, &p); err != nil {
		return 0, fmt.Errorf("CLOSE-WAIT 清理超时必须为整数")
	}
	if p.System.Seconds < 0 || p.System.Seconds > 600 {
		return 0, fmt.Errorf("CLOSE-WAIT 清理超时必须为 0-600 秒")
	}
	return p.System.Seconds, nil
}

func supportsCloseWaitVersion(output string) bool {
	fields := strings.Fields(output)
	if len(fields) < 2 || fields[0] != "Xray" || !strings.HasPrefix(fields[1], "vx-") {
		return false
	}
	parts := strings.Split(strings.TrimPrefix(fields[1], "vx-"), ".")
	if len(parts) != 2 {
		return false
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return false
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil || minor < 0 {
		return false
	}
	return major > 26 || (major == 26 && minor >= 4)
}

// ValidateCloseWaitTarget prevents a downgrade from stopping a core whose saved
// policy needs the new feature. The caller checks this before replacing binaries.
func ValidateCloseWaitTarget(cfg *Config, tag string) error {
	seconds, err := closeWaitSeconds(cfg)
	if err != nil || seconds == 0 {
		return err
	}
	if !supportsCloseWaitVersion("Xray " + tag) {
		return fmt.Errorf("CLOSE-WAIT 清理已开启，不能切换到不支持它的核心；请先将清理超时设为 0 并保存，或选择 vx-26.4 及更新版本")
	}
	return nil
}

// Older cores silently ignore unknown policy fields. Reject them before saving
// or starting instead of reporting success for a cleanup option that cannot run.
func ValidateCloseWaitSupport(cfg *Config) error {
	seconds, err := closeWaitSeconds(cfg)
	if err != nil || seconds == 0 {
		return err
	}
	if runtime.GOOS != "linux" {
		return fmt.Errorf("CLOSE-WAIT 清理目前仅支持 Linux")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, GetBinaryPath(), "-version").Output()
	if err != nil {
		return fmt.Errorf("无法检查核心的 CLOSE-WAIT 支持，请确认 DUI 核心已安装")
	}
	if !supportsCloseWaitVersion(string(output)) {
		return fmt.Errorf("CLOSE-WAIT 清理需要 DUI 核心 vx-26.4 或更新版本；请先升级核心，或将该项设为 0")
	}
	return nil
}
