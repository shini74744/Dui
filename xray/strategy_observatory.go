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
func supportsStrategyObservatoryVersion(output string) bool { return supportsDUIVersion(output, 5) }
func supportsDUIVersion(output string, minimum int) bool {
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
	return e1 == nil && e2 == nil && minor >= 0 && (major > 26 || (major == 26 && minor >= minimum))
}

// Older cores silently ignore route fallback and priority fields. Gate both
// saving and core downgrades before an unsupported configuration can be installed.
func requiredProbeVersion(cfg *Config) (int, error) {
	enabled, err := hasStrategyObservatory(cfg)
	if err != nil {
		return 0, err
	}
	minimum := 0
	if enabled {
		minimum = 5
	}
	if cfg == nil {
		return minimum, nil
	}
	var scopes map[string]json.RawMessage
	if len(cfg.StrategyObservatory) > 0 {
		if err := json.Unmarshal(cfg.StrategyObservatory, &scopes); err != nil {
			return 0, err
		}
		if _, ok := scopes["outbound"]; ok {
			minimum = 10
		}
	}
	var routing struct {
		Rules     []struct{ FallbackTag string }
		Balancers []struct{ Strategy struct{ Type string } }
	}
	if len(cfg.RouterConfig) > 0 {
		if err := json.Unmarshal(cfg.RouterConfig, &routing); err != nil {
			return 0, err
		}
		for _, rule := range routing.Rules {
			if rule.FallbackTag != "" {
				minimum = 10
			}
		}
		for _, b := range routing.Balancers {
			if strings.EqualFold(b.Strategy.Type, "priority") {
				minimum = 10
			}
		}
	}
	return minimum, nil
}
func ValidateStrategyObservatoryTarget(cfg *Config, tag string) error {
	minimum, err := requiredProbeVersion(cfg)
	if err != nil || minimum == 0 {
		return err
	}
	if !supportsDUIVersion("Xray "+tag, minimum) {
		return fmt.Errorf("当前探测与故障切换配置需要 DUI 核心 vx-26.%d 或更新版本，不能切换到不支持的核心", minimum)
	}
	return nil
}
func ValidateStrategyObservatorySupport(cfg *Config) error {
	minimum, err := requiredProbeVersion(cfg)
	if err != nil || minimum == 0 {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, GetBinaryPath(), "-version").Output()
	if err != nil || !supportsDUIVersion(string(output), minimum) {
		return fmt.Errorf("当前探测与故障切换配置需要 DUI 核心 vx-26.%d 或更新版本，请先升级核心再保存", minimum)
	}
	return nil
}
