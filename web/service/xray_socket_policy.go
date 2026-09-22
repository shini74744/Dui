package service

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"

	"x-ui/xray"
)

var (
	xraySocketPolicyPath = "/etc/x-ui/xray_socket_policy.json"
	xraySocketPolicyMu   sync.Mutex
)

type XraySocketPolicy struct {
	Enabled              bool   `json:"enabled" form:"enabled"`
	ApplyInbound         bool   `json:"applyInbound" form:"applyInbound"`
	ApplyOutbound        bool   `json:"applyOutbound" form:"applyOutbound"`
	TCPKeepAliveIdle     int    `json:"tcpKeepAliveIdle" form:"tcpKeepAliveIdle"`
	TCPKeepAliveInterval int    `json:"tcpKeepAliveInterval" form:"tcpKeepAliveInterval"`
	TCPUserTimeout       int    `json:"tcpUserTimeout" form:"tcpUserTimeout"`
	TCPFastOpenMode      string `json:"tcpFastOpenMode" form:"tcpFastOpenMode"`
	TCPMaxSeg            int    `json:"tcpMaxSeg" form:"tcpMaxSeg"`
	TCPWindowClamp       int    `json:"tcpWindowClamp" form:"tcpWindowClamp"`
	TCPCongestion        string `json:"tcpCongestion" form:"tcpCongestion"`
	TCPMptcpMode         string `json:"tcpMptcpMode" form:"tcpMptcpMode"`
	Interface            string `json:"interface" form:"interface"`
	Mark                 int    `json:"mark" form:"mark"`
	V6OnlyMode           string `json:"v6OnlyMode" form:"v6OnlyMode"`
}
type XraySocketPolicyStatus struct {
	XraySocketPolicy
	AvailableCongestion []string `json:"availableCongestion"`
	Interfaces          []string `json:"interfaces"`
	TCPFastOpenKernel   int      `json:"tcpFastOpenKernel"`
	MPTCPAvailable      bool     `json:"mptcpAvailable"`
	Path                string   `json:"path"`
}

func defaultXraySocketPolicy() XraySocketPolicy {
	return XraySocketPolicy{
		Enabled:         false,
		ApplyInbound:    true,
		ApplyOutbound:   true,
		TCPFastOpenMode: "inherit",
		TCPMptcpMode:    "inherit",
		V6OnlyMode:      "inherit",
	}
}

func normalizeTriState(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "on":
		return "on"
	case "off":
		return "off"
	default:
		return "inherit"
	}
}
func readXraySocketPolicyUnlocked() XraySocketPolicy {
	p := defaultXraySocketPolicy()
	data, err := os.ReadFile(xraySocketPolicyPath)
	if err != nil {
		return p
	}
	if err := json.Unmarshal(data, &p); err != nil {
		return defaultXraySocketPolicy()
	}
	p.TCPFastOpenMode = normalizeTriState(p.TCPFastOpenMode)
	p.TCPMptcpMode = normalizeTriState(p.TCPMptcpMode)
	p.V6OnlyMode = normalizeTriState(p.V6OnlyMode)
	if !p.ApplyInbound && !p.ApplyOutbound {
		p.ApplyInbound = true
		p.ApplyOutbound = true
	}
	return p
}

func writeXraySocketPolicyUnlocked(p XraySocketPolicy) error {
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return writeAtomicFile(xraySocketPolicyPath, data, 0600)
}
func availableCongestionAlgorithms() []string {
	data, err := os.ReadFile("/proc/sys/net/ipv4/tcp_available_congestion_control")
	if err != nil {
		return nil
	}
	items := strings.Fields(string(data))
	sort.Strings(items)
	return items
}

func hostInterfaceNames() []string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(ifaces))
	for _, iface := range ifaces {
		names = append(names, iface.Name)
	}
	sort.Strings(names)
	return names
}

func readIntFile(path string) int {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	n, _ := strconv.Atoi(strings.TrimSpace(string(data)))
	return n
}

func xraySocketPolicyStatus(p XraySocketPolicy) XraySocketPolicyStatus {
	return XraySocketPolicyStatus{
		XraySocketPolicy:    p,
		AvailableCongestion: availableCongestionAlgorithms(),
		Interfaces:          hostInterfaceNames(),
		TCPFastOpenKernel:   readIntFile("/proc/sys/net/ipv4/tcp_fastopen"),
		MPTCPAvailable:      readIntFile("/proc/sys/net/mptcp/enabled") > 0,
		Path:                xraySocketPolicyPath,
	}
}
func validateXraySocketPolicy(p XraySocketPolicy) error {
	if !p.ApplyInbound && !p.ApplyOutbound {
		return fmt.Errorf("入站和出站至少选择一个应用范围")
	}
	checkRange := func(name string, value, min, max int) error {
		if value == 0 {
			return nil
		}
		if value < min || value > max {
			return fmt.Errorf("%s 必须为 0（继承）或 %d-%d", name, min, max)
		}
		return nil
	}
	if err := checkRange("tcpKeepAliveIdle", p.TCPKeepAliveIdle, 30, 86400); err != nil {
		return err
	}
	if err := checkRange("tcpKeepAliveInterval", p.TCPKeepAliveInterval, 5, 3600); err != nil {
		return err
	}
	if err := checkRange("tcpUserTimeout", p.TCPUserTimeout, 1000, 3600000); err != nil {
		return err
	}
	if err := checkRange("tcpMaxSeg", p.TCPMaxSeg, 536, 65535); err != nil {
		return err
	}
	if err := checkRange("tcpWindowClamp", p.TCPWindowClamp, 536, 16777216); err != nil {
		return err
	}
	if p.Mark < 0 || p.Mark > 2147483647 {
		return fmt.Errorf("mark 必须为 0-2147483647")
	}
	for _, mode := range []string{p.TCPFastOpenMode, p.TCPMptcpMode, p.V6OnlyMode} {
		if normalizeTriState(mode) != strings.ToLower(strings.TrimSpace(mode)) {
			return fmt.Errorf("布尔模式必须是 inherit、on 或 off")
		}
	}
	if p.TCPCongestion != "" {
		ok := false
		for _, item := range availableCongestionAlgorithms() {
			if p.TCPCongestion == item {
				ok = true
				break
			}
		}
		if !ok {
			return fmt.Errorf("当前系统不支持拥塞算法 %s", p.TCPCongestion)
		}
	}
	if p.Interface != "" {
		ok := false
		for _, name := range hostInterfaceNames() {
			if p.Interface == name {
				ok = true
				break
			}
		}
		if !ok {
			return fmt.Errorf("网卡 %s 不存在", p.Interface)
		}
	}
	if p.TCPMptcpMode == "on" && readIntFile("/proc/sys/net/mptcp/enabled") <= 0 {
		return fmt.Errorf("当前系统未启用 MPTCP；DUI 不会自动修改系统内核开关")
	}
	return nil
}

func GetXraySocketPolicyStatus() XraySocketPolicyStatus {
	xraySocketPolicyMu.Lock()
	p := readXraySocketPolicyUnlocked()
	xraySocketPolicyMu.Unlock()
	return xraySocketPolicyStatus(p)
}
func SaveXraySocketPolicy(p XraySocketPolicy) error {
	p.TCPFastOpenMode = normalizeTriState(p.TCPFastOpenMode)
	p.TCPMptcpMode = normalizeTriState(p.TCPMptcpMode)
	p.V6OnlyMode = normalizeTriState(p.V6OnlyMode)
	if err := validateXraySocketPolicy(p); err != nil {
		return err
	}
	xraySocketPolicyMu.Lock()
	defer xraySocketPolicyMu.Unlock()
	return writeXraySocketPolicyUnlocked(p)
}

func getXraySocketPolicy() XraySocketPolicy {
	xraySocketPolicyMu.Lock()
	defer xraySocketPolicyMu.Unlock()
	return readXraySocketPolicyUnlocked()
}

func setTriState(sockopt map[string]any, key, mode string) {
	switch normalizeTriState(mode) {
	case "on":
		sockopt[key] = true
	case "off":
		sockopt[key] = false
	}
}
func applySocketPolicyToStream(stream map[string]any, p XraySocketPolicy, inbound bool) {
	sockopt, _ := stream["sockopt"].(map[string]any)
	if sockopt == nil {
		sockopt = map[string]any{}
	}
	if p.TCPKeepAliveIdle > 0 {
		sockopt["tcpKeepAliveIdle"] = p.TCPKeepAliveIdle
	}
	if p.TCPKeepAliveInterval > 0 {
		sockopt["tcpKeepAliveInterval"] = p.TCPKeepAliveInterval
	}
	if p.TCPUserTimeout > 0 {
		sockopt["tcpUserTimeout"] = p.TCPUserTimeout
	}
	if p.TCPMaxSeg > 0 {
		sockopt["tcpMaxSeg"] = p.TCPMaxSeg
	}
	if p.TCPWindowClamp > 0 {
		sockopt["tcpWindowClamp"] = p.TCPWindowClamp
	}
	if p.TCPCongestion != "" {
		sockopt["tcpCongestion"] = p.TCPCongestion
	}
	if !inbound && p.Interface != "" {
		sockopt["interface"] = p.Interface
	}
	if p.Mark > 0 {
		sockopt["mark"] = p.Mark
	}
	setTriState(sockopt, "tcpFastOpen", p.TCPFastOpenMode)
	setTriState(sockopt, "tcpMptcp", p.TCPMptcpMode)
	if inbound {
		setTriState(sockopt, "v6only", p.V6OnlyMode)
	}
	if len(sockopt) > 0 {
		stream["sockopt"] = sockopt
	}
}

func applyXraySocketPolicy(config *xray.Config) error {
	p := getXraySocketPolicy()
	if !p.Enabled {
		return nil
	}
	if err := validateXraySocketPolicy(p); err != nil {
		return fmt.Errorf("Xray Socket 策略无效: %w", err)
	}
	if p.ApplyInbound {
		for i := range config.InboundConfigs {
			inbound := &config.InboundConfigs[i]
			if inbound.Tag == "api" {
				continue
			}
			stream := map[string]any{}
			if len(inbound.StreamSettings) > 0 {
				if err := json.Unmarshal(inbound.StreamSettings, &stream); err != nil {
					return fmt.Errorf("解析入站 %s StreamSettings 失败: %w", inbound.Tag, err)
				}
			}
			applySocketPolicyToStream(stream, p, true)
			data, err := json.Marshal(stream)
			if err != nil {
				return err
			}
			inbound.StreamSettings = data
		}
	}
	if p.ApplyOutbound && len(config.OutboundConfigs) > 0 {
		var outbounds []map[string]any
		if err := json.Unmarshal(config.OutboundConfigs, &outbounds); err != nil {
			return fmt.Errorf("解析 Xray 出站失败: %w", err)
		}
		for _, outbound := range outbounds {
			protocol, _ := outbound["protocol"].(string)
			switch protocol {
			case "blackhole", "dns", "loopback":
				continue
			}
			stream, _ := outbound["streamSettings"].(map[string]any)
			if stream == nil {
				stream = map[string]any{}
			}
			applySocketPolicyToStream(stream, p, false)
			outbound["streamSettings"] = stream
		}
		data, err := json.Marshal(outbounds)
		if err != nil {
			return err
		}
		config.OutboundConfigs = data
	}
	return nil
}
