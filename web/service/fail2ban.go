package service

import (
	"bufio"
	"context"
	_ "embed"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"x-ui/database"
)

//go:embed fb5.sh
var embeddedFB5Script string

const (
	fail2banJailLocal   = "/etc/fail2ban/jail.local"
	fail2banTLSFilter   = "/etc/fail2ban/filter.d/3xui-tls.conf"
	fail2banTLSJail     = "/etc/fail2ban/jail.d/3xui-tls.local"
	fail2banLoginFilter = "/etc/fail2ban/filter.d/3xui-login.conf"
	fail2banLoginJail   = "/etc/fail2ban/jail.d/3xui-login.local"
	fail2banNftAction   = "/etc/fail2ban/action.d/xui-nftables-all.conf"
	fail2banIptAction   = "/etc/fail2ban/action.d/xui-iptables-all.conf"
	fail2banShortcut    = "/usr/local/bin/fb5"
)

type Fail2banLogEntry struct {
	Time  string `json:"time"`
	Jail  string `json:"jail"`
	Event string `json:"event"`
	IP    string `json:"ip"`
	Raw   string `json:"raw"`
}

type Fail2banJailInfo struct {
	Name        string             `json:"name"`
	Configured  bool               `json:"configured"`
	Active      bool               `json:"active"`
	Port        int                `json:"port"`
	MaxRetry    int                `json:"maxretry"`
	FindTime    string             `json:"findtime"`
	BanTime     string             `json:"bantime"`
	IgnoreIP    string             `json:"ignoreip"`
	BannedCount int                `json:"bannedCount"`
	BannedIPs   []string           `json:"bannedIps"`
	RecentLogs  []Fail2banLogEntry `json:"recentLogs"`
}

type Fail2banStatus struct {
	Installed bool             `json:"installed"`
	Running   bool             `json:"running"`
	Firewall  string           `json:"firewall"`
	Shortcut  bool             `json:"shortcut"`
	PanelPort int              `json:"panelPort"`
	SSHD      Fail2banJailInfo `json:"sshd"`
	XUITLS    Fail2banJailInfo `json:"xuiTls"`
	XUILogin  Fail2banJailInfo `json:"xuiLogin"`
}

type Fail2banApplyOptions struct {
	Target   string
	Port     int
	MaxRetry int
	FindTime string
	BanTime  string
	IgnoreIP string
}

type Fail2banService struct{}

var (
	fail2banDurationRe = regexp.MustCompile(`^[0-9]+([smhdw])?$`)
	fail2banLogRe      = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}).*\[([^]\r\n]+)\] (Found|Ban|Unban|Restore Ban) ([0-9A-Fa-f:.]+)`)
)

func runSystemCommand(timeout time.Duration, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(out))
	if ctx.Err() == context.DeadlineExceeded {
		return text, fmt.Errorf("%s timed out", name)
	}
	if err != nil {
		if text != "" {
			return text, fmt.Errorf("%s: %w: %s", name, err, text)
		}
		return text, fmt.Errorf("%s: %w", name, err)
	}
	return text, nil
}

func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func serviceActive(name string) bool {
	if !commandExists("systemctl") {
		return false
	}
	return exec.Command("systemctl", "is-active", "--quiet", name).Run() == nil
}

func detectFail2banFirewall() string {
	if commandExists("nft") {
		return "nftables"
	}
	if commandExists("iptables") {
		return "iptables"
	}
	if commandExists("firewall-cmd") && serviceActive("firewalld") {
		return "firewalld"
	}
	return "unknown"
}

func sshActionForFirewall(fw string) string {
	switch fw {
	case "nftables":
		return "nftables[type=multiport]"
	case "firewalld":
		return "firewallcmd-ipset[actiontype=<multiport>]"
	default:
		return "iptables[type=multiport]"
	}
}

func xuiActionForFirewall(fw string) (string, error) {
	switch fw {
	case "nftables":
		return "xui-nftables-all", nil
	case "iptables":
		return "xui-iptables-all", nil
	default:
		return "", fmt.Errorf("Dui 整机封禁仅支持 nftables 或 iptables，当前检测为 %s", fw)
	}
}

func readINISection(path, section string) map[string]string {
	result := map[string]string{}
	f, err := os.Open(path)
	if err != nil {
		return result
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	inSection := false
	target := "[" + section + "]"
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			inSection = line == target
			continue
		}
		if !inSection || line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if idx := strings.Index(line, "="); idx >= 0 {
			result[strings.TrimSpace(line[:idx])] = strings.TrimSpace(line[idx+1:])
		}
	}
	return result
}

func parseIntDefault(value string, fallback int) int {
	n, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return fallback
	}
	return n
}

func parseFail2banDuration(value string) (time.Duration, bool) {
	value = strings.ToLower(strings.TrimSpace(value))
	m := fail2banDurationRe.FindStringSubmatch(value)
	if len(m) == 0 {
		return 0, false
	}

	numberPart := value
	unit := byte('s')
	last := value[len(value)-1]
	if last < '0' || last > '9' {
		unit = last
		numberPart = value[:len(value)-1]
	}

	n, err := strconv.ParseInt(numberPart, 10, 64)
	if err != nil || n < 0 {
		return 0, false
	}

	multiplier := time.Second
	switch unit {
	case 's':
		multiplier = time.Second
	case 'm':
		multiplier = time.Minute
	case 'h':
		multiplier = time.Hour
	case 'd':
		multiplier = 24 * time.Hour
	case 'w':
		multiplier = 7 * 24 * time.Hour
	default:
		return 0, false
	}
	return time.Duration(n) * multiplier, true
}

func normalizeIPList(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		ip := strings.Trim(strings.TrimSpace(value), "[](),")
		if net.ParseIP(ip) == nil {
			continue
		}
		if _, ok := seen[ip]; ok {
			continue
		}
		seen[ip] = struct{}{}
		result = append(result, ip)
	}
	return result
}

func parseBannedIPsFromStatus(out string) []string {
	for _, line := range strings.Split(out, "\n") {
		if idx := strings.Index(line, "Banned IP list:"); idx >= 0 {
			return normalizeIPList(strings.Fields(line[idx+len("Banned IP list:"):]))
		}
	}
	return []string{}
}

func (s *Fail2banService) bannedIPs(jail string) []string {
	if !commandExists("fail2ban-client") || !serviceActive("fail2ban") {
		return []string{}
	}
	if out, err := runSystemCommand(15*time.Second, "fail2ban-client", "get", jail, "banip"); err == nil {
		if ips := normalizeIPList(strings.Fields(out)); len(ips) > 0 {
			return ips
		}
	}
	out, err := runSystemCommand(15*time.Second, "fail2ban-client", "status", jail)
	if err != nil {
		return []string{}
	}
	return parseBannedIPsFromStatus(out)
}

func readFail2banLogsFromPaths(jail string, limit int, findTime string, now time.Time, paths []string) []Fail2banLogEntry {
	if limit <= 0 {
		limit = 100
	}

	var cutoff time.Time
	if window, ok := parseFail2banDuration(findTime); ok && window > 0 {
		cutoff = now.Add(-window)
	}

	entries := make([]Fail2banLogEntry, 0, limit)
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		lines := strings.Split(string(data), "\n")
		for i := len(lines) - 1; i >= 0 && len(entries) < limit; i-- {
			line := strings.TrimSpace(lines[i])
			if line == "" {
				continue
			}
			m := fail2banLogRe.FindStringSubmatch(line)
			if len(m) != 5 || m[2] != jail {
				continue
			}
			if !cutoff.IsZero() {
				if eventTime, err := time.ParseInLocation("2006-01-02 15:04:05", m[1], time.Local); err == nil && eventTime.Before(cutoff) {
					continue
				}
			}
			entries = append(entries, Fail2banLogEntry{Time: m[1], Jail: m[2], Event: m[3], IP: m[4], Raw: line})
		}
		if len(entries) >= limit {
			break
		}
	}
	return entries
}

func readRecentFail2banLogs(jail string, limit int, findTime string) []Fail2banLogEntry {
	return readFail2banLogsFromPaths(
		jail,
		limit,
		findTime,
		time.Now(),
		[]string{"/var/log/fail2ban.log", "/var/log/fail2ban.log.1"},
	)
}

func (s *Fail2banService) jailActive(jail string) bool {
	if !commandExists("fail2ban-client") || !serviceActive("fail2ban") {
		return false
	}
	return exec.Command("fail2ban-client", "status", jail).Run() == nil
}

func jailInfoFromFile(name, path, section string, defaultPort, defaultRetry int, defaultFind, defaultBan string) Fail2banJailInfo {
	cfg := readINISection(path, section)
	info := Fail2banJailInfo{
		Name:       name,
		Configured: len(cfg) > 0,
		Port:       parseIntDefault(cfg["port"], defaultPort),
		MaxRetry:   parseIntDefault(cfg["maxretry"], defaultRetry),
		FindTime:   cfg["findtime"],
		BanTime:    cfg["bantime"],
		IgnoreIP:   cfg["ignoreip"],
	}
	if info.FindTime == "" {
		info.FindTime = defaultFind
	}
	if info.BanTime == "" {
		info.BanTime = defaultBan
	}
	return info
}

func (s *Fail2banService) Status() (*Fail2banStatus, error) {
	panelPort := 2053
	if database.GetDB() != nil {
		var settingService SettingService
		if p, err := settingService.GetPort(); err == nil && p > 0 {
			panelPort = p
		}
	}
	fw := detectFail2banFirewall()
	status := &Fail2banStatus{
		Installed: commandExists("fail2ban-client"),
		Running:   serviceActive("fail2ban"),
		Firewall:  fw,
		PanelPort: panelPort,
	}
	if st, err := os.Stat(fail2banShortcut); err == nil && st.Mode().IsRegular() {
		status.Shortcut = st.Mode().Perm()&0111 != 0
	}

	defaults := readINISection(fail2banJailLocal, "DEFAULT")
	status.SSHD = jailInfoFromFile("sshd", fail2banJailLocal, "sshd", 22, 3, "1d", "-1")
	status.SSHD.IgnoreIP = defaults["ignoreip"]
	if status.SSHD.IgnoreIP == "" {
		status.SSHD.IgnoreIP = "127.0.0.1/8 ::1"
	}
	status.XUITLS = jailInfoFromFile("3xui-tls", fail2banTLSJail, "3xui-tls", panelPort, 5, "1d", "1h")
	status.XUILogin = jailInfoFromFile("3xui-login", fail2banLoginJail, "3xui-login", panelPort, 3, "1d", "-1")
	if status.XUITLS.IgnoreIP == "" {
		status.XUITLS.IgnoreIP = "127.0.0.1/8 ::1"
	}
	if status.XUILogin.IgnoreIP == "" {
		status.XUILogin.IgnoreIP = "127.0.0.1/8 ::1"
	}

	status.SSHD.Active = s.jailActive("sshd")
	status.XUITLS.Active = s.jailActive("3xui-tls")
	status.XUILogin.Active = s.jailActive("3xui-login")
	status.SSHD.BannedIPs = s.bannedIPs("sshd")
	status.XUITLS.BannedIPs = s.bannedIPs("3xui-tls")
	status.XUILogin.BannedIPs = s.bannedIPs("3xui-login")
	status.SSHD.BannedCount = len(status.SSHD.BannedIPs)
	status.XUITLS.BannedCount = len(status.XUITLS.BannedIPs)
	status.XUILogin.BannedCount = len(status.XUILogin.BannedIPs)
	status.SSHD.RecentLogs = readRecentFail2banLogs("sshd", 120, status.SSHD.FindTime)
	status.XUITLS.RecentLogs = readRecentFail2banLogs("3xui-tls", 120, status.XUITLS.FindTime)
	status.XUILogin.RecentLogs = readRecentFail2banLogs("3xui-login", 120, status.XUILogin.FindTime)
	return status, nil
}

func validateFail2banOptions(o Fail2banApplyOptions) error {
	if o.Target != "sshd" && o.Target != "3xui-tls" && o.Target != "3xui-login" {
		return errors.New("未知保护类型")
	}
	if o.Port < 1 || o.Port > 65535 {
		return errors.New("端口必须为 1-65535")
	}
	if o.MaxRetry < 1 || o.MaxRetry > 100000 {
		return errors.New("maxretry 必须为正整数")
	}
	if o.FindTime == "" || !fail2banDurationRe.MatchString(o.FindTime) {
		return errors.New("findtime 格式无效，例如 600、30m、1h、1d")
	}
	if o.BanTime != "-1" && (o.BanTime == "" || !fail2banDurationRe.MatchString(o.BanTime)) {
		return errors.New("bantime 格式无效，例如 600、12h、1d、-1")
	}
	if strings.TrimSpace(o.IgnoreIP) != "" {
		for _, item := range strings.Fields(o.IgnoreIP) {
			if net.ParseIP(item) != nil {
				continue
			}
			if _, _, err := net.ParseCIDR(item); err == nil {
				continue
			}
			return fmt.Errorf("白名单地址无效: %s", item)
		}
	}
	return nil
}

func detectPackageManager() string {
	for _, p := range []string{"apt-get", "dnf", "yum"} {
		if commandExists(p) {
			return p
		}
	}
	return ""
}

func (s *Fail2banService) ensureInstalled() error {
	if commandExists("fail2ban-client") {
		return nil
	}
	pm := detectPackageManager()
	switch pm {
	case "apt-get":
		if _, err := runSystemCommand(4*time.Minute, "apt-get", "update", "-y"); err != nil {
			return err
		}
		if _, err := runSystemCommand(4*time.Minute, "apt-get", "install", "-y", "fail2ban"); err != nil {
			return err
		}
	case "dnf":
		_, _ = runSystemCommand(3*time.Minute, "dnf", "-y", "install", "epel-release")
		if _, err := runSystemCommand(4*time.Minute, "dnf", "-y", "install", "fail2ban"); err != nil {
			return err
		}
	case "yum":
		_, _ = runSystemCommand(3*time.Minute, "yum", "-y", "install", "epel-release")
		if _, err := runSystemCommand(4*time.Minute, "yum", "-y", "install", "fail2ban"); err != nil {
			return err
		}
	default:
		return errors.New("未找到 apt-get/dnf/yum，无法自动安装 Fail2ban")
	}
	return nil
}

func replaceINISection(path, section, block string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, _ := os.ReadFile(path)
	lines := strings.Split(string(data), "\n")
	target := "[" + section + "]"
	var out []string
	inTarget := false
	replaced := false
	for _, line := range lines {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "[") && strings.HasSuffix(trim, "]") {
			if inTarget {
				inTarget = false
			}
			if trim == target {
				if !replaced {
					if len(out) > 0 && strings.TrimSpace(out[len(out)-1]) != "" {
						out = append(out, "")
					}
					out = append(out, strings.Split(strings.TrimSpace(block), "\n")...)
					replaced = true
				}
				inTarget = true
				continue
			}
		}
		if !inTarget {
			out = append(out, line)
		}
	}
	if !replaced {
		if len(out) > 0 && strings.TrimSpace(out[len(out)-1]) != "" {
			out = append(out, "")
		}
		out = append(out, strings.Split(strings.TrimSpace(block), "\n")...)
	}
	return os.WriteFile(path, []byte(strings.TrimRight(strings.Join(out, "\n"), "\n")+"\n"), 0644)
}

func removeINISection(path, section string) error {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	lines := strings.Split(string(data), "\n")
	target := "[" + section + "]"
	var out []string
	inTarget := false
	for _, line := range lines {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "[") && strings.HasSuffix(trim, "]") {
			if trim == target {
				inTarget = true
				continue
			}
			if inTarget {
				inTarget = false
			}
		}
		if !inTarget {
			out = append(out, line)
		}
	}
	return os.WriteFile(path, []byte(strings.TrimSpace(strings.Join(out, "\n"))+"\n"), 0644)
}

func updateDefaultIgnoreIP(path, ignore string) error {
	if strings.TrimSpace(ignore) == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, _ := os.ReadFile(path)
	text := string(data)
	if !strings.Contains(text, "[DEFAULT]") {
		text = "[DEFAULT]\nignoreip = " + ignore + "\n\n" + text
		return os.WriteFile(path, []byte(text), 0644)
	}
	lines := strings.Split(text, "\n")
	inDefault := false
	written := false
	var out []string
	for _, line := range lines {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "[") && strings.HasSuffix(trim, "]") {
			if inDefault && !written {
				out = append(out, "ignoreip = "+ignore)
				written = true
			}
			inDefault = trim == "[DEFAULT]"
			out = append(out, line)
			continue
		}
		if inDefault && strings.HasPrefix(trim, "ignoreip") && strings.Contains(trim, "=") {
			if !written {
				out = append(out, "ignoreip = "+ignore)
				written = true
			}
			continue
		}
		out = append(out, line)
	}
	if inDefault && !written {
		out = append(out, "ignoreip = "+ignore)
	}
	return os.WriteFile(path, []byte(strings.TrimRight(strings.Join(out, "\n"), "\n")+"\n"), 0644)
}

func sshLogSettings() (backend, logpath, journalmatch string) {
	if _, err := os.Stat("/var/log/auth.log"); err == nil {
		return "file", "/var/log/auth.log", ""
	}
	if _, err := os.Stat("/var/log/secure"); err == nil {
		return "file", "/var/log/secure", ""
	}
	return "systemd", "", "_SYSTEMD_UNIT=sshd.service + _COMM=sshd"
}

func writeNftAction() error {
	if err := os.MkdirAll(filepath.Dir(fail2banNftAction), 0755); err != nil {
		return err
	}
	content := `[INCLUDES]
before = nftables.conf

[Init]
type = custom
blocktype = reject
`
	return os.WriteFile(fail2banNftAction, []byte(content), 0644)
}

func writeIptAction() error {
	if err := os.MkdirAll(filepath.Dir(fail2banIptAction), 0755); err != nil {
		return err
	}
	content := `[Definition]
actionstart = iptables -N f2b-<name> 2>/dev/null || true
              iptables -C INPUT -j f2b-<name> 2>/dev/null || iptables -I INPUT -j f2b-<name>
              iptables -C f2b-<name> -j RETURN 2>/dev/null || iptables -A f2b-<name> -j RETURN
actionstop = iptables -D INPUT -j f2b-<name> 2>/dev/null || true
             iptables -F f2b-<name> 2>/dev/null || true
             iptables -X f2b-<name> 2>/dev/null || true
actioncheck = iptables -n -L INPUT | grep -q "f2b-<name>"
actionban = iptables -I f2b-<name> 1 -s <ip> -j REJECT --reject-with icmp-port-unreachable
actionunban = iptables -D f2b-<name> -s <ip> -j REJECT --reject-with icmp-port-unreachable 2>/dev/null || true
`
	return os.WriteFile(fail2banIptAction, []byte(content), 0644)
}

func restartFail2ban() error {
	if !commandExists("systemctl") {
		return errors.New("未检测到 systemctl")
	}
	if out, err := runSystemCommand(30*time.Second, "systemctl", "restart", "fail2ban"); err != nil {
		return fmt.Errorf("Fail2ban 重启失败: %v %s", err, out)
	}
	_, _ = runSystemCommand(20*time.Second, "systemctl", "enable", "fail2ban")
	return nil
}

func (s *Fail2banService) ScriptPresent() bool {
	st, err := os.Stat(fail2banShortcut)
	return err == nil && st.Mode().IsRegular()
}

func (s *Fail2banService) SyncScript() error {
	if !s.ScriptPresent() {
		return errors.New("未检测到现有 /usr/local/bin/fb5，不创建新的 fb5 命令")
	}
	if err := os.WriteFile(fail2banShortcut, []byte(embeddedFB5Script), 0755); err != nil {
		return err
	}
	return os.Chmod(fail2banShortcut, 0755)
}

func (s *Fail2banService) syncScriptIfPresent() error {
	if !s.ScriptPresent() {
		return nil
	}
	return s.SyncScript()
}

func (s *Fail2banService) Apply(o Fail2banApplyOptions) error {
	if err := validateFail2banOptions(o); err != nil {
		return err
	}
	if err := s.ensureInstalled(); err != nil {
		return err
	}
	fw := detectFail2banFirewall()
	if err := s.syncScriptIfPresent(); err != nil {
		return fmt.Errorf("同步现有 fb5 失败: %w", err)
	}
	switch o.Target {
	case "sshd":
		backend, logpath, journalmatch := sshLogSettings()
		block := fmt.Sprintf("[sshd]\nenabled  = true\nport     = %d\nfilter   = sshd\n", o.Port)
		if backend == "systemd" {
			block += "backend  = systemd\njournalmatch = " + journalmatch + "\n"
		} else {
			block += "logpath  = " + logpath + "\n"
		}
		block += fmt.Sprintf("action   = %s\nmaxretry = %d\nfindtime = %s\nbantime  = %s", sshActionForFirewall(fw), o.MaxRetry, o.FindTime, o.BanTime)
		if err := replaceINISection(fail2banJailLocal, "sshd", block); err != nil {
			return err
		}
		if err := updateDefaultIgnoreIP(fail2banJailLocal, o.IgnoreIP); err != nil {
			return err
		}
	case "3xui-tls", "3xui-login":
		action, err := xuiActionForFirewall(fw)
		if err != nil {
			return err
		}
		if fw == "nftables" {
			if err := writeNftAction(); err != nil {
				return err
			}
		} else if fw == "iptables" {
			if err := writeIptAction(); err != nil {
				return err
			}
		}
		if err := os.MkdirAll("/etc/fail2ban/filter.d", 0755); err != nil {
			return err
		}
		if err := os.MkdirAll("/etc/fail2ban/jail.d", 0755); err != nil {
			return err
		}
		ignore := strings.TrimSpace(o.IgnoreIP)
		if ignore == "" {
			ignore = "127.0.0.1/8 ::1"
		}
		var filterPath, jailPath, filterName string
		var filterContent string
		if o.Target == "3xui-tls" {
			filterPath, jailPath, filterName = fail2banTLSFilter, fail2banTLSJail, "3xui-tls"
			filterContent = "[Definition]\nfailregex = ^.*http: TLS handshake error from <HOST>:\\d+:.*$\nignoreregex =\n"
		} else {
			filterPath, jailPath, filterName = fail2banLoginFilter, fail2banLoginJail, "3xui-login"
			filterContent = "[Definition]\nfailregex = ^.*WARNING - wrong username: .*IP:\\s*\"<HOST>\"\\s*$\nignoreregex =\n"
		}
		if err := os.WriteFile(filterPath, []byte(filterContent), 0644); err != nil {
			return err
		}
		jailContent := fmt.Sprintf("[%s]\nenabled = true\nbackend = systemd\njournalmatch = _SYSTEMD_UNIT=x-ui.service\nfilter = %s\nport = %d\nprotocol = tcp\nmaxretry = %d\nfindtime = %s\nbantime = %s\nignoreip = %s\naction = %s\n",
			filterName, filterName, o.Port, o.MaxRetry, o.FindTime, o.BanTime, ignore, action)
		if err := os.WriteFile(jailPath, []byte(jailContent), 0644); err != nil {
			return err
		}
	}
	return restartFail2ban()
}

func (s *Fail2banService) Unban(jail, ip string) error {
	switch jail {
	case "sshd", "3xui-tls", "3xui-login":
	default:
		return errors.New("未知 jail")
	}
	if net.ParseIP(strings.TrimSpace(ip)) == nil {
		return errors.New("IP 格式无效")
	}
	if !commandExists("fail2ban-client") {
		return errors.New("Fail2ban 未安装")
	}
	_, err := runSystemCommand(15*time.Second, "fail2ban-client", "set", jail, "unbanip", strings.TrimSpace(ip))
	return err
}

func (s *Fail2banService) Remove(target string) error {
	switch target {
	case "sshd":
		if err := removeINISection(fail2banJailLocal, "sshd"); err != nil {
			return err
		}
	case "3xui-tls":
		_ = os.Remove(fail2banTLSFilter)
		_ = os.Remove(fail2banTLSJail)
	case "3xui-login":
		_ = os.Remove(fail2banLoginFilter)
		_ = os.Remove(fail2banLoginJail)
	case "actions":
		_ = os.Remove(fail2banNftAction)
		_ = os.Remove(fail2banIptAction)
	case "shortcut":
		err := os.Remove(fail2banShortcut)
		if os.IsNotExist(err) {
			return nil
		}
		return err
	case "all":
		if err := removeINISection(fail2banJailLocal, "sshd"); err != nil {
			return err
		}
		for _, p := range []string{fail2banTLSFilter, fail2banTLSJail, fail2banLoginFilter, fail2banLoginJail, fail2banNftAction, fail2banIptAction} {
			_ = os.Remove(p)
		}
	case "package":
		pm := detectPackageManager()
		_, _ = runSystemCommand(20*time.Second, "systemctl", "stop", "fail2ban")
		switch pm {
		case "apt-get":
			_, err := runSystemCommand(4*time.Minute, "apt-get", "purge", "-y", "fail2ban")
			return err
		case "dnf":
			_, err := runSystemCommand(4*time.Minute, "dnf", "-y", "remove", "fail2ban")
			return err
		case "yum":
			_, err := runSystemCommand(4*time.Minute, "yum", "-y", "remove", "fail2ban")
			return err
		default:
			return errors.New("未找到包管理器")
		}
	default:
		return errors.New("未知删除目标")
	}
	if target != "shortcut" && target != "package" && commandExists("fail2ban-client") {
		return restartFail2ban()
	}
	return nil
}
