package xray

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"
)

// ValidateConfig does not start listeners or alter network routes. Never return
// the core's raw diagnostics: they can echo node credentials or private keys.
func ValidateConfig(cfg *Config) error {
	if err := ValidateStrategyObservatorySupport(cfg); err != nil {
		return err
	}
	if err := ValidateCloseWaitSupport(cfg); err != nil {
		return err
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, GetBinaryPath(), "run", "-test", "-format", "json", "-config", "stdin:")
	cmd.Stdin = bytes.NewReader(data)
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("核心配置校验超时")
		}
		return fmt.Errorf("核心配置校验未通过，请检查协议、证书及传输设置 (%w)", err)
	}
	return nil
}
