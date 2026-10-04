package xray

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

func hasStrategyObservatory(cfg *Config) (bool, error) {
	if cfg == nil || len(cfg.StrategyObservatory) == 0 {
		return false, nil
	}
	var settings map[string]json.RawMessage
	if err := json.Unmarshal(cfg.StrategyObservatory, &settings); err != nil {
		return false, fmt.Errorf("invalid strategyObservatory: %w", err)
	}
	return len(settings) > 0, nil
}
func supportsStrategyObservatoryVersion(output string) bool {
	fields := strings.Fields(output)
	if len(fields) < 2 || fields[0] != "Xray" || !strings.HasPrefix(fields[1], "vx-") {
		return false
	}
	parts := strings.Split(strings.TrimPrefix(fields[1], "vx-"), ".")
	if len(parts) != 2 {
		return false
	}
	major, e1 := strconv.Atoi(parts[0])
	minor, e2 := strconv.Atoi(parts[1])
	return e1 == nil && e2 == nil && minor >= 0 && (major > 26 || (major == 26 && minor >= 5))
}
func ValidateStrategyObservatoryTarget(cfg *Config, tag string) error {
	enabled, err := hasStrategyObservatory(cfg)
	if err != nil || !enabled {
		return err
	}
	if !supportsStrategyObservatoryVersion("Xray " + tag) {
		return fmt.Errorf("独立策略探测需要 DUI 核心 vx-26.5 或更新版本，不能切换到不支持的核心")
	}
	return nil
}
func ValidateStrategyObservatorySupport(cfg *Config) error {
	enabled, err := hasStrategyObservatory(cfg)
	if err != nil || !enabled {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, GetBinaryPath(), "-version").Output()
	if err != nil || !supportsStrategyObservatoryVersion(string(output)) {
		return fmt.Errorf("独立策略探测需要 DUI 核心 vx-26.5 或更新版本，请先升级核心再保存")
	}
	return nil
}
