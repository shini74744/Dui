package service

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"x-ui/database"
	"x-ui/database/model"
	"x-ui/xray"
)

type DailyTrafficRankItem struct {
	Remark  string `json:"remark"`
	Port    int    `json:"port"`
	User    string `json:"user"`
	Up      int64  `json:"up"`
	Down    int64  `json:"down"`
	Traffic int64  `json:"traffic"`
}

func (s *ServerService) GetDailyTrafficRanking(limit int) ([]DailyTrafficRankItem, error) {
	if limit < 1 {
		limit = 5
	}
	if limit > 20 {
		limit = 20
	}
	db := database.GetDB()
	today := time.Now().Format("2006-01-02")
	cutoff := time.Now().AddDate(0, 0, -31).Format("2006-01-02")
	_ = db.Where("date < ?", cutoff).Delete(&model.DailyClientTraffic{}).Error

	var rows []DailyTrafficRankItem
	err := db.Table("daily_client_traffics AS d").
		Select("COALESCE(i.remark, '') AS remark, COALESCE(i.port, 0) AS port, d.email AS user, d.up AS up, d.down AS down, (d.up + d.down) AS traffic").
		Joins("LEFT JOIN inbounds AS i ON i.id = d.inbound_id").
		Where("d.date = ?", today).
		Order("(d.up + d.down) DESC, d.email ASC").
		Limit(limit).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for i := range rows {
		if strings.HasPrefix(rows[i].User, model.DailyTrafficEmptyEmailPrefix) {
			rows[i].User = "未填写Email"
		}
	}
	return rows, nil
}

type NetworkExitPolicy struct {
	IPv4Enabled bool   `json:"ipv4Enabled"`
	IPv6Enabled bool   `json:"ipv6Enabled"`
	Preferred   string `json:"preferred"`
}

type NetworkExitPolicyStatus struct {
	NetworkExitPolicy
	IPv4Available bool `json:"ipv4Available"`
	IPv6Available bool `json:"ipv6Available"`
	DualStack     bool `json:"dualStack"`
}

type networkFamilyAvailability struct {
	IPv4 bool
	IPv6 bool
}

const networkExitPolicyPath = "/etc/x-ui/network_exit_policy.json"

var networkExitPolicyMu sync.Mutex

func defaultNetworkExitPolicy() NetworkExitPolicy {
	return NetworkExitPolicy{IPv4Enabled: true, IPv6Enabled: true}
}

func canRouteUDP(network string, remote *net.UDPAddr) bool {
	conn, err := net.DialUDP(network, nil, remote)
	if err != nil {
		return false
	}
	defer conn.Close()

	local, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok || local.IP == nil || local.IP.IsUnspecified() {
		return false
	}
	return true
}

func detectNetworkFamilyAvailability() networkFamilyAvailability {
	return networkFamilyAvailability{
		IPv4: canRouteUDP("udp4", &net.UDPAddr{IP: net.ParseIP("1.1.1.1"), Port: 53}),
		IPv6: canRouteUDP("udp6", &net.UDPAddr{IP: net.ParseIP("2606:4700:4700::1111"), Port: 53}),
	}
}

func buildNetworkExitPolicyStatus(policy NetworkExitPolicy) NetworkExitPolicyStatus {
	availability := detectNetworkFamilyAvailability()
	return NetworkExitPolicyStatus{
		NetworkExitPolicy: policy,
		IPv4Available:     availability.IPv4,
		IPv6Available:     availability.IPv6,
		DualStack:         availability.IPv4 && availability.IPv6,
	}
}

func readNetworkExitPolicyUnlocked() NetworkExitPolicy {
	policy := defaultNetworkExitPolicy()
	data, err := os.ReadFile(networkExitPolicyPath)
	if err != nil {
		return policy
	}
	if err := json.Unmarshal(data, &policy); err != nil {
		return defaultNetworkExitPolicy()
	}
	if policy.Preferred != "" && policy.Preferred != "ipv4" && policy.Preferred != "ipv6" {
		policy.Preferred = ""
	}
	if !policy.IPv4Enabled && !policy.IPv6Enabled {
		return defaultNetworkExitPolicy()
	}
	return policy
}

func writeAtomicFile(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".dui-tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func writeNetworkExitPolicyUnlocked(policy NetworkExitPolicy) error {
	data, err := json.MarshalIndent(policy, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return writeAtomicFile(networkExitPolicyPath, data, 0600)
}

func GetNetworkExitPolicyForXray() NetworkExitPolicy {
	networkExitPolicyMu.Lock()
	policy := readNetworkExitPolicyUnlocked()
	networkExitPolicyMu.Unlock()

	availability := detectNetworkFamilyAvailability()
	if !availability.IPv4 || !availability.IPv6 {
		return defaultNetworkExitPolicy()
	}
	return policy
}

func networkExitDomainStrategy(policy NetworkExitPolicy) string {
	switch {
	case !policy.IPv4Enabled && policy.IPv6Enabled:
		return "ForceIPv6"
	case policy.IPv4Enabled && !policy.IPv6Enabled:
		return "ForceIPv4"
	case policy.Preferred == "ipv6":
		return "UseIPv6v4"
	case policy.Preferred == "ipv4":
		return "UseIPv4v6"
	default:
		return ""
	}
}

// applyNetworkExitPolicyToXrayConfig changes only Xray-generated outbound
// connections. It never changes host routes, nftables, DNS or gai.conf.
func applyNetworkExitPolicyToXrayConfig(config *xray.Config) error {
	policy := GetNetworkExitPolicyForXray()
	strategy := networkExitDomainStrategy(policy)
	if strategy == "" || len(config.OutboundConfigs) == 0 {
		return nil
	}

	var outbounds []map[string]any
	if err := json.Unmarshal(config.OutboundConfigs, &outbounds); err != nil {
		return fmt.Errorf("解析 Xray 出站配置失败: %w", err)
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
		sockopt, _ := stream["sockopt"].(map[string]any)
		if sockopt == nil {
			sockopt = map[string]any{}
		}
		sockopt["domainStrategy"] = strategy
		stream["sockopt"] = sockopt
		outbound["streamSettings"] = stream
	}
	data, err := json.Marshal(outbounds)
	if err != nil {
		return fmt.Errorf("生成 Xray 出站配置失败: %w", err)
	}
	config.OutboundConfigs = data
	return nil
}

func (s *ServerService) GetNetworkExitPolicy() NetworkExitPolicyStatus {
	networkExitPolicyMu.Lock()
	policy := readNetworkExitPolicyUnlocked()
	networkExitPolicyMu.Unlock()
	return buildNetworkExitPolicyStatus(policy)
}

func (s *ServerService) SetNetworkExitPolicy(family, action string) (NetworkExitPolicyStatus, error) {
	availability := detectNetworkFamilyAvailability()

	networkExitPolicyMu.Lock()
	oldPolicy := readNetworkExitPolicyUnlocked()
	policy := oldPolicy

	if !availability.IPv4 || !availability.IPv6 {
		networkExitPolicyMu.Unlock()
		return buildNetworkExitPolicyStatus(oldPolicy), fmt.Errorf("当前机器不是 IPv4/IPv6 双栈，节点出口切换已禁用")
	}
	if family != "ipv4" && family != "ipv6" {
		networkExitPolicyMu.Unlock()
		return buildNetworkExitPolicyStatus(oldPolicy), fmt.Errorf("网络类型必须是 ipv4 或 ipv6")
	}
	if action != "normal" && action != "disable" && action != "prefer" {
		networkExitPolicyMu.Unlock()
		return buildNetworkExitPolicyStatus(oldPolicy), fmt.Errorf("网络操作必须是 normal、disable 或 prefer")
	}

	otherEnabled := policy.IPv6Enabled
	if family == "ipv6" {
		otherEnabled = policy.IPv4Enabled
	}
	if action == "disable" && !otherEnabled {
		networkExitPolicyMu.Unlock()
		return buildNetworkExitPolicyStatus(oldPolicy), fmt.Errorf("V4 和 V6 不能同时关闭")
	}

	setEnabled := func(value bool) {
		if family == "ipv4" {
			policy.IPv4Enabled = value
		} else {
			policy.IPv6Enabled = value
		}
	}
	switch action {
	case "normal":
		setEnabled(true)
		if policy.Preferred == family {
			policy.Preferred = ""
		}
	case "disable":
		setEnabled(false)
		if policy.Preferred == family {
			policy.Preferred = ""
		}
	case "prefer":
		setEnabled(true)
		policy.Preferred = family
	}

	if err := writeNetworkExitPolicyUnlocked(policy); err != nil {
		networkExitPolicyMu.Unlock()
		return buildNetworkExitPolicyStatus(oldPolicy), err
	}
	networkExitPolicyMu.Unlock()

	if err := s.RestartXrayService(); err != nil {
		networkExitPolicyMu.Lock()
		_ = writeNetworkExitPolicyUnlocked(oldPolicy)
		networkExitPolicyMu.Unlock()
		_ = s.RestartXrayService()
		return buildNetworkExitPolicyStatus(oldPolicy), fmt.Errorf("应用 Xray 出口策略失败: %w", err)
	}
	return buildNetworkExitPolicyStatus(policy), nil
}
