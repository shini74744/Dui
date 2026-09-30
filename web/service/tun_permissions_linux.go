//go:build linux

package service

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

func checkTunPermissions() error {
	if _, err := os.Stat("/dev/net/tun"); err != nil {
		return fmt.Errorf("系统未提供 /dev/net/tun，暂不能启用 TUN")
	}
	status, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return fmt.Errorf("无法确认 TUN 所需的网络管理权限")
	}
	for _, line := range strings.Split(string(status), "\n") {
		if strings.HasPrefix(line, "CapEff:") {
			bits, err := strconv.ParseUint(strings.TrimSpace(strings.TrimPrefix(line, "CapEff:")), 16, 64)
			if err == nil && bits&(1<<12) != 0 {
				return nil
			}
		}
	}
	return fmt.Errorf("启用 TUN 需要 CAP_NET_ADMIN 网络管理权限；容器还需映射 /dev/net/tun")
}
