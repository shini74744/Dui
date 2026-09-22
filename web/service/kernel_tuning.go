package service

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

var (
	kernelProcRoot       = "/proc/sys"
	kernelSysctlConfig   = "/etc/sysctl.d/99-dui-network.conf"
	kernelOriginalBackup = "/var/lib/dui/kernel-tuning-original.json"
	kernelGeteuid        = os.Geteuid
)

type KernelTuningValues struct {
	TCPKeepaliveTime   int `json:"tcpKeepaliveTime" form:"tcpKeepaliveTime"`
	TCPKeepaliveIntvl  int `json:"tcpKeepaliveIntvl" form:"tcpKeepaliveIntvl"`
	TCPKeepaliveProbes int `json:"tcpKeepaliveProbes" form:"tcpKeepaliveProbes"`
	TCPFinTimeout      int `json:"tcpFinTimeout" form:"tcpFinTimeout"`
	TCPMaxSynBacklog   int `json:"tcpMaxSynBacklog" form:"tcpMaxSynBacklog"`
	Somaxconn          int `json:"somaxconn" form:"somaxconn"`
	NetdevMaxBacklog   int `json:"netdevMaxBacklog" form:"netdevMaxBacklog"`
}
type KernelTuningApplyOptions = KernelTuningValues

type KernelTuningStatus struct {
	Values           KernelTuningValues  `json:"values"`
	Supported        map[string]bool     `json:"supported"`
	Configured       bool                `json:"configured"`
	Persistent       bool                `json:"persistent"`
	ConfigPath       string              `json:"configPath"`
	IsRoot           bool                `json:"isRoot"`
	Warning          string              `json:"warning"`
	OriginalBackup   bool                `json:"originalBackup"`
	OriginalBackupAt int64               `json:"originalBackupAt"`
	OriginalValues   *KernelTuningValues `json:"originalValues,omitempty"`
	AtOriginal       bool                `json:"atOriginal"`
}

type kernelOriginalSnapshot struct {
	CreatedAt     int64              `json:"createdAt"`
	Values        KernelTuningValues `json:"values"`
	Supported     map[string]bool    `json:"supported"`
	ConfigExisted bool               `json:"configExisted"`
	ConfigContent string             `json:"configContent"`
}

type kernelTuningParam struct {
	Field string
	Key   string
	Min   int
	Max   int
}

var kernelTuningParams = []kernelTuningParam{
	{Field: "tcpKeepaliveTime", Key: "net.ipv4.tcp_keepalive_time", Min: 30, Max: 86400},
	{Field: "tcpKeepaliveIntvl", Key: "net.ipv4.tcp_keepalive_intvl", Min: 5, Max: 3600},
	{Field: "tcpKeepaliveProbes", Key: "net.ipv4.tcp_keepalive_probes", Min: 1, Max: 30},
	{Field: "tcpFinTimeout", Key: "net.ipv4.tcp_fin_timeout", Min: 5, Max: 300},
	{Field: "tcpMaxSynBacklog", Key: "net.ipv4.tcp_max_syn_backlog", Min: 128, Max: 1048576},
	{Field: "somaxconn", Key: "net.core.somaxconn", Min: 128, Max: 1048576},
	{Field: "netdevMaxBacklog", Key: "net.core.netdev_max_backlog", Min: 128, Max: 1048576},
}

type KernelTuningService struct{}

func kernelProcPath(key string) string {
	return filepath.Join(kernelProcRoot, strings.ReplaceAll(key, ".", "/"))
}

func kernelValue(v KernelTuningValues, field string) int {
	switch field {
	case "tcpKeepaliveTime":
		return v.TCPKeepaliveTime
	case "tcpKeepaliveIntvl":
		return v.TCPKeepaliveIntvl
	case "tcpKeepaliveProbes":
		return v.TCPKeepaliveProbes
	case "tcpFinTimeout":
		return v.TCPFinTimeout
	case "tcpMaxSynBacklog":
		return v.TCPMaxSynBacklog
	case "somaxconn":
		return v.Somaxconn
	case "netdevMaxBacklog":
		return v.NetdevMaxBacklog
	default:
		return 0
	}
}
func setKernelValue(v *KernelTuningValues, field string, value int) {
	switch field {
	case "tcpKeepaliveTime":
		v.TCPKeepaliveTime = value
	case "tcpKeepaliveIntvl":
		v.TCPKeepaliveIntvl = value
	case "tcpKeepaliveProbes":
		v.TCPKeepaliveProbes = value
	case "tcpFinTimeout":
		v.TCPFinTimeout = value
	case "tcpMaxSynBacklog":
		v.TCPMaxSynBacklog = value
	case "somaxconn":
		v.Somaxconn = value
	case "netdevMaxBacklog":
		v.NetdevMaxBacklog = value
	}
}

func readKernelInt(key string) (int, error) {
	data, err := os.ReadFile(kernelProcPath(key))
	if err != nil {
		return 0, err
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, fmt.Errorf("%s 当前值不是整数: %w", key, err)
	}
	return n, nil
}
func readKernelTuningValues() (KernelTuningValues, map[string]bool, error) {
	var values KernelTuningValues
	supported := make(map[string]bool, len(kernelTuningParams))
	for _, p := range kernelTuningParams {
		n, err := readKernelInt(p.Key)
		if os.IsNotExist(err) {
			supported[p.Field] = false
			continue
		}
		if err != nil {
			return values, supported, fmt.Errorf("读取 %s 失败: %w", p.Key, err)
		}
		supported[p.Field] = true
		setKernelValue(&values, p.Field, n)
	}
	return values, supported, nil
}

func validateKernelTuning(v KernelTuningValues, supported map[string]bool) error {
	for _, p := range kernelTuningParams {
		if !supported[p.Field] {
			continue
		}
		n := kernelValue(v, p.Field)
		if n < p.Min || n > p.Max {
			return fmt.Errorf("%s 必须在 %d-%d 之间", p.Key, p.Min, p.Max)
		}
	}
	return nil
}
func parseManagedKernelConfig() map[string]int {
	result := map[string]int{}
	data, err := os.ReadFile(kernelSysctlConfig)
	if err != nil {
		return result
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || !strings.Contains(line, "=") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		n, err := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err == nil {
			result[strings.TrimSpace(parts[0])] = n
		}
	}
	return result
}

func managedKernelConfigMatches(values KernelTuningValues, supported map[string]bool) bool {
	if _, err := os.Stat(kernelSysctlConfig); err != nil {
		return false
	}
	cfg := parseManagedKernelConfig()
	for _, p := range kernelTuningParams {
		if supported[p.Field] && cfg[p.Key] != kernelValue(values, p.Field) {
			return false
		}
	}
	return true
}

func loadKernelOriginalSnapshot() (*kernelOriginalSnapshot, error) {
	data, err := os.ReadFile(kernelOriginalBackup)
	if err != nil {
		return nil, err
	}
	var snapshot kernelOriginalSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return nil, fmt.Errorf("解析内核最初备份失败: %w", err)
	}
	if snapshot.Supported == nil {
		snapshot.Supported = map[string]bool{}
	}
	return &snapshot, nil
}

func ensureKernelOriginalSnapshot(values KernelTuningValues, supported map[string]bool, config []byte, configExisted bool) error {
	if _, err := os.Stat(kernelOriginalBackup); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("检查内核最初备份失败: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(kernelOriginalBackup), 0755); err != nil {
		return fmt.Errorf("创建内核备份目录失败: %w", err)
	}
	snapshot := kernelOriginalSnapshot{
		CreatedAt:     time.Now().Unix(),
		Values:        values,
		Supported:     supported,
		ConfigExisted: configExisted,
		ConfigContent: string(config),
	}
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return fmt.Errorf("生成内核最初备份失败: %w", err)
	}
	f, err := os.OpenFile(kernelOriginalBackup, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if os.IsExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("创建内核最初备份失败: %w", err)
	}
	defer f.Close()
	if _, err := f.Write(data); err != nil {
		_ = os.Remove(kernelOriginalBackup)
		return fmt.Errorf("写入内核最初备份失败: %w", err)
	}
	return nil
}

func kernelSnapshotMatches(values KernelTuningValues, supported map[string]bool, snapshot *kernelOriginalSnapshot) bool {
	if snapshot == nil {
		return false
	}
	for _, p := range kernelTuningParams {
		if !snapshot.Supported[p.Field] {
			continue
		}
		if !supported[p.Field] || kernelValue(values, p.Field) != kernelValue(snapshot.Values, p.Field) {
			return false
		}
	}
	data, err := os.ReadFile(kernelSysctlConfig)
	if snapshot.ConfigExisted {
		return err == nil && string(data) == snapshot.ConfigContent
	}
	return os.IsNotExist(err)
}

func (s *KernelTuningService) Status() (*KernelTuningStatus, error) {
	values, supported, err := readKernelTuningValues()
	if err != nil {
		return nil, err
	}
	_, statErr := os.Stat(kernelSysctlConfig)
	status := &KernelTuningStatus{
		Values:     values,
		Supported:  supported,
		Configured: statErr == nil,
		Persistent: managedKernelConfigMatches(values, supported),
		ConfigPath: kernelSysctlConfig,
		IsRoot:     kernelGeteuid() == 0,
	}
	if !status.IsRoot {
		status.Warning = "当前面板进程不是 root，无法修改 Linux 内核参数。"
	}
	snapshot, snapshotErr := loadKernelOriginalSnapshot()
	if snapshotErr == nil {
		status.OriginalBackup = true
		status.OriginalBackupAt = snapshot.CreatedAt
		originalValues := snapshot.Values
		status.OriginalValues = &originalValues
		status.AtOriginal = kernelSnapshotMatches(values, supported, snapshot)
	} else if !os.IsNotExist(snapshotErr) {
		return nil, snapshotErr
	}
	return status, nil
}

func rollbackKernelRuntime(old map[string]int, applied []kernelTuningParam) {
	for i := len(applied) - 1; i >= 0; i-- {
		p := applied[i]
		if n, ok := old[p.Key]; ok {
			_ = os.WriteFile(kernelProcPath(p.Key), []byte(strconv.Itoa(n)), 0644)
		}
	}
}
func renderKernelConfig(v KernelTuningValues, supported map[string]bool) []byte {
	var b strings.Builder
	b.WriteString("# Managed by DUI. Manual edits may be overwritten by the panel.\n")
	b.WriteString("# These settings are system-wide and affect Xray plus other network applications.\n")
	for _, p := range kernelTuningParams {
		if supported[p.Field] {
			fmt.Fprintf(&b, "%s = %d\n", p.Key, kernelValue(v, p.Field))
		}
	}
	return []byte(b.String())
}

func writeKernelConfigAtomic(content []byte) error {
	if err := os.MkdirAll(filepath.Dir(kernelSysctlConfig), 0755); err != nil {
		return err
	}
	tmp := kernelSysctlConfig + ".tmp"
	if err := os.WriteFile(tmp, content, 0644); err != nil {
		return err
	}
	if err := os.Rename(tmp, kernelSysctlConfig); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func restoreKernelConfig(content []byte, existed bool) {
	if existed {
		_ = writeKernelConfigAtomic(content)
		return
	}
	_ = os.Remove(kernelSysctlConfig)
}
func (s *KernelTuningService) Apply(v KernelTuningValues) (*KernelTuningStatus, error) {
	if kernelGeteuid() != 0 {
		return nil, fmt.Errorf("面板进程不是 root，无法修改 Linux 内核参数")
	}
	current, supported, err := readKernelTuningValues()
	if err != nil {
		return nil, err
	}
	if err := validateKernelTuning(v, supported); err != nil {
		return nil, err
	}

	oldConfig, configErr := os.ReadFile(kernelSysctlConfig)
	configExisted := configErr == nil
	if configErr != nil && !os.IsNotExist(configErr) {
		return nil, fmt.Errorf("读取现有持久化配置失败，未修改内核参数: %w", configErr)
	}
	if err := ensureKernelOriginalSnapshot(current, supported, oldConfig, configExisted); err != nil {
		return nil, err
	}

	old := make(map[string]int, len(kernelTuningParams))
	applied := make([]kernelTuningParam, 0, len(kernelTuningParams))
	for _, p := range kernelTuningParams {
		if !supported[p.Field] {
			continue
		}
		old[p.Key] = kernelValue(current, p.Field)
		target := kernelValue(v, p.Field)
		if err := os.WriteFile(kernelProcPath(p.Key), []byte(strconv.Itoa(target)), 0644); err != nil {
			rollbackKernelRuntime(old, applied)
			return nil, fmt.Errorf("应用 %s 失败，已回滚: %w", p.Key, err)
		}
		applied = append(applied, p)
		actual, err := readKernelInt(p.Key)
		if err != nil || actual != target {
			rollbackKernelRuntime(old, applied)
			if err != nil {
				return nil, fmt.Errorf("回读 %s 失败，已回滚: %w", p.Key, err)
			}
			return nil, fmt.Errorf("%s 回读值为 %d，期望 %d，已回滚", p.Key, actual, target)
		}
	}

	if err := writeKernelConfigAtomic(renderKernelConfig(v, supported)); err != nil {
		rollbackKernelRuntime(old, applied)
		return nil, fmt.Errorf("写入持久化配置失败，运行时值已回滚: %w", err)
	}

	status, err := s.Status()
	if err != nil {
		restoreKernelConfig(oldConfig, configExisted)
		rollbackKernelRuntime(old, applied)
		return nil, fmt.Errorf("应用后状态校验失败，已回滚: %w", err)
	}
	if !status.Persistent {
		restoreKernelConfig(oldConfig, configExisted)
		rollbackKernelRuntime(old, applied)
		return nil, fmt.Errorf("持久化回读校验失败，已恢复原配置和运行时参数")
	}
	return status, nil
}

func (s *KernelTuningService) RestoreOriginal() (*KernelTuningStatus, error) {
	if kernelGeteuid() != 0 {
		return nil, fmt.Errorf("面板进程不是 root，无法还原 Linux 内核参数")
	}
	snapshot, err := loadKernelOriginalSnapshot()
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("尚未生成最初内核备份，无法还原")
		}
		return nil, err
	}

	current, supported, err := readKernelTuningValues()
	if err != nil {
		return nil, err
	}
	currentConfig, configErr := os.ReadFile(kernelSysctlConfig)
	currentConfigExisted := configErr == nil
	if configErr != nil && !os.IsNotExist(configErr) {
		return nil, fmt.Errorf("读取当前持久化配置失败，未执行还原: %w", configErr)
	}
	old := make(map[string]int, len(kernelTuningParams))
	applied := make([]kernelTuningParam, 0, len(kernelTuningParams))
	for _, p := range kernelTuningParams {
		if !snapshot.Supported[p.Field] {
			continue
		}
		if !supported[p.Field] {
			rollbackKernelRuntime(old, applied)
			return nil, fmt.Errorf("原始参数 %s 当前系统已不支持，已回滚", p.Key)
		}
		old[p.Key] = kernelValue(current, p.Field)
		target := kernelValue(snapshot.Values, p.Field)
		if err := os.WriteFile(kernelProcPath(p.Key), []byte(strconv.Itoa(target)), 0644); err != nil {
			rollbackKernelRuntime(old, applied)
			return nil, fmt.Errorf("还原 %s 失败，已回滚: %w", p.Key, err)
		}
		applied = append(applied, p)
		actual, err := readKernelInt(p.Key)
		if err != nil || actual != target {
			rollbackKernelRuntime(old, applied)
			if err != nil {
				return nil, fmt.Errorf("回读 %s 失败，已回滚: %w", p.Key, err)
			}
			return nil, fmt.Errorf("%s 回读值为 %d，期望 %d，已回滚", p.Key, actual, target)
		}
	}
	if snapshot.ConfigExisted {
		if err := writeKernelConfigAtomic([]byte(snapshot.ConfigContent)); err != nil {
			rollbackKernelRuntime(old, applied)
			return nil, fmt.Errorf("恢复最初持久化配置失败，运行时值已回滚: %w", err)
		}
	} else if err := os.Remove(kernelSysctlConfig); err != nil && !os.IsNotExist(err) {
		rollbackKernelRuntime(old, applied)
		return nil, fmt.Errorf("删除 DUI 持久化配置失败，运行时值已回滚: %w", err)
	}

	status, err := s.Status()
	if err != nil || !status.AtOriginal {
		restoreKernelConfig(currentConfig, currentConfigExisted)
		rollbackKernelRuntime(old, applied)
		if err != nil {
			return nil, fmt.Errorf("还原后校验失败，已恢复还原前状态: %w", err)
		}
		return nil, fmt.Errorf("还原后状态与最初备份不一致，已恢复还原前状态")
	}
	return status, nil
}
