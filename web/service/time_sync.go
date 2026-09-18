package service

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

//go:embed ntp.sh
var embeddedNTPManager string

const (
	timeSyncManagerPath = "/usr/local/sbin/time-sync-manager"
	timeSyncConfigPath  = "/etc/time-sync-manager.conf"
	timeSyncServicePath = "/etc/systemd/system/time-sync-manager.service"
	timeSyncTimerPath   = "/etc/systemd/system/time-sync-manager.timer"
)

type TimeSyncStatus struct {
	Supported          bool   `json:"supported"`
	Installed          bool   `json:"installed"`
	Version            string `json:"version"`
	ConfiguredTimezone string `json:"configuredTimezone"`
	SystemTimezone     string `json:"systemTimezone"`
	LocalTime          string `json:"localTime"`
	NTPSynchronized    bool   `json:"ntpSynchronized"`
	TimeSyncdActive    bool   `json:"timesyncdActive"`
	TimeSyncdEnabled   bool   `json:"timesyncdEnabled"`
	TimerActive        bool   `json:"timerActive"`
	TimerEnabled       bool   `json:"timerEnabled"`
	NextSync           string `json:"nextSync"`
	Conflict           string `json:"conflict"`
}

type TimeSyncLogs struct {
	Manager   string `json:"manager"`
	TimeSyncd string `json:"timesyncd"`
}

type TimeSyncService struct{}

var versionLineRe = regexp.MustCompile(`(?m)^VERSION=["']([^"']+)["']$`)

func runCommandWithEnv(timeout time.Duration, env []string, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(os.Environ(), env...)
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

func writeTempEmbeddedScript(prefix, content string) (string, func(), error) {
	f, err := os.CreateTemp("/tmp", prefix+"-*.sh")
	if err != nil {
		return "", nil, err
	}
	path := f.Name()
	cleanup := func() { _ = os.Remove(path) }
	if _, err := f.WriteString(content); err != nil {
		_ = f.Close()
		cleanup()
		return "", nil, err
	}
	if err := f.Close(); err != nil {
		cleanup()
		return "", nil, err
	}
	if err := os.Chmod(path, 0700); err != nil {
		cleanup()
		return "", nil, err
	}
	return path, cleanup, nil
}

func fileVersion(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	m := versionLineRe.FindSubmatch(data)
	if len(m) == 2 {
		return string(m[1])
	}
	return ""
}

func systemctlState(kind, unit string) bool {
	if !commandExists("systemctl") {
		return false
	}
	return exec.Command("systemctl", kind, "--quiet", unit).Run() == nil
}

func detectedTimeConflict() string {
	for _, name := range []string{"chrony", "ntp", "ntpsec", "openntpd"} {
		if !commandExists("dpkg-query") {
			break
		}
		out, _ := exec.Command("dpkg-query", "-W", "-f=`{db:Status-Abbrev}", name).Output()
		if strings.HasPrefix(string(out), "ii") {
			return name
		}
	}
	return ""
}

func isSupportedTimeSyncHost() bool {
	if !commandExists("apt-get") || !commandExists("systemctl") {
		return false
	}
	if _, err := os.Stat("/run/systemd/system"); err != nil {
		return false
	}
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return false
	}
	text := string(data)
	return strings.Contains(text, "ID=debian") || strings.Contains(text, "ID=ubuntu") ||
		strings.Contains(text, "ID=\"debian\"") || strings.Contains(text, "ID=\"ubuntu\"")
}

func readTimeSyncConfigTimezone() string {
	data, err := os.ReadFile(timeSyncConfigPath)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "TIMEZONE=") {
			return strings.TrimSpace(strings.TrimPrefix(line, "TIMEZONE="))
		}
	}
	return ""
}

func currentSystemTimezone() string {
	out, err := runSystemCommand(8*time.Second, "timedatectl", "show", "-p", "Timezone", "--value")
	if err == nil {
		return strings.TrimSpace(out)
	}
	return ""
}

func validTimezone(tz string) bool {
	tz = strings.TrimSpace(tz)
	if tz == "" || strings.HasPrefix(tz, "/") || strings.Contains(tz, "..") {
		return false
	}
	ok, _ := regexp.MatchString(`^[A-Za-z0-9_+.-]+(/[A-Za-z0-9_+.-]+)+$`, tz)
	if !ok {
		return false
	}
	st, err := os.Stat(filepath.Join("/usr/share/zoneinfo", tz))
	return err == nil && !st.IsDir()
}

func (s *TimeSyncService) Status() (*TimeSyncStatus, error) {
	status := &TimeSyncStatus{
		Supported: isSupportedTimeSyncHost(),
		Conflict:  detectedTimeConflict(),
	}
	if st, err := os.Stat(timeSyncManagerPath); err == nil && st.Mode().IsRegular() {
		status.Installed = true
		status.Version = fileVersion(timeSyncManagerPath)
	}
	status.ConfiguredTimezone = readTimeSyncConfigTimezone()
	status.SystemTimezone = currentSystemTimezone()
	if status.ConfiguredTimezone == "" {
		status.ConfiguredTimezone = status.SystemTimezone
	}
	if out, err := runSystemCommand(5*time.Second, "date", "+%Y-%m-%d %H:%M:%S %Z (%z)"); err == nil {
		status.LocalTime = strings.TrimSpace(out)
	}
	if out, err := runSystemCommand(5*time.Second, "timedatectl", "show", "-p", "NTPSynchronized", "--value"); err == nil {
		status.NTPSynchronized = strings.TrimSpace(out) == "yes"
	}
	status.TimeSyncdActive = systemctlState("is-active", "systemd-timesyncd")
	status.TimeSyncdEnabled = systemctlState("is-enabled", "systemd-timesyncd")
	status.TimerActive = systemctlState("is-active", "time-sync-manager.timer")
	status.TimerEnabled = systemctlState("is-enabled", "time-sync-manager.timer")
	if commandExists("systemctl") {
		if out, err := runSystemCommand(8*time.Second, "systemctl", "list-timers", "time-sync-manager.timer", "--all", "--no-pager", "--no-legend"); err == nil {
			status.NextSync = strings.TrimSpace(out)
		}
	}
	return status, nil
}

func (s *TimeSyncService) Timezones() ([]string, error) {
	var zones []string
	if commandExists("timedatectl") {
		out, err := runSystemCommand(15*time.Second, "timedatectl", "list-timezones")
		if err == nil {
			for _, z := range strings.Split(out, "\n") {
				z = strings.TrimSpace(z)
				if validTimezone(z) {
					zones = append(zones, z)
				}
			}
		}
	}
	if len(zones) == 0 {
		data, err := os.ReadFile("/usr/share/zoneinfo/zone.tab")
		if err != nil {
			return nil, err
		}
		for _, line := range strings.Split(string(data), "\n") {
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.Fields(line)
			if len(parts) >= 3 && validTimezone(parts[2]) {
				zones = append(zones, parts[2])
			}
		}
		sort.Strings(zones)
	}
	return zones, nil
}

func countryZones(code string) []string {
	data, err := os.ReadFile("/usr/share/zoneinfo/zone.tab")
	if err != nil {
		return nil
	}
	code = strings.ToUpper(strings.TrimSpace(code))
	var zones []string
	seen := map[string]bool{}
	for _, line := range strings.Split(string(data), "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 3 {
			continue
		}
		matched := false
		for _, c := range strings.Split(parts[0], ",") {
			if strings.EqualFold(c, code) {
				matched = true
				break
			}
		}
		if matched && validTimezone(parts[2]) && !seen[parts[2]] {
			seen[parts[2]] = true
			zones = append(zones, parts[2])
		}
	}
	return zones
}

func normalizeTimezoneQuery(q string) string {
	q = strings.TrimSpace(strings.ToLower(q))
	q = strings.ReplaceAll(q, " ", "")
	q = strings.ReplaceAll(q, "-", "")
	q = strings.ReplaceAll(q, "_", "")
	return q
}

func (s *TimeSyncService) ResolveTimezones(query string) ([]string, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return s.Timezones()
	}
	if validTimezone(query) {
		return []string{query}, nil
	}

	aliases := map[string]string{
		"hk": "HK", "香港": "HK", "hongkong": "HK", "港澳": "HK",
		"tw": "TW", "台湾": "TW", "台灣": "TW", "台北": "TW", "taiwan": "TW", "taipei": "TW",
		"cn": "CN", "中国": "CN", "中國": "CN", "大陆": "CN", "大陸": "CN", "内地": "CN", "china": "CN",
		"jp": "JP", "日本": "JP", "东京": "JP", "東京": "JP", "japan": "JP", "tokyo": "JP",
		"kr": "KR", "韩国": "KR", "韓國": "KR", "南韩": "KR", "南韓": "KR", "korea": "KR", "seoul": "KR",
		"sg": "SG", "新加坡": "SG", "singapore": "SG",
		"my": "MY", "马来西亚": "MY", "馬來西亞": "MY", "大马": "MY", "大馬": "MY", "malaysia": "MY",
		"th": "TH", "泰国": "TH", "泰國": "TH", "thailand": "TH",
		"vn": "VN", "越南": "VN", "vietnam": "VN",
		"ph": "PH", "菲律宾": "PH", "菲律賓": "PH", "philippines": "PH",
		"id": "ID", "印度尼西亚": "ID", "印度尼西亞": "ID", "印尼": "ID", "indonesia": "ID",
		"in": "IN", "印度": "IN", "india": "IN",
		"au": "AU", "澳大利亚": "AU", "澳大利亞": "AU", "澳洲": "AU", "australia": "AU",
		"nz": "NZ", "新西兰": "NZ", "新西蘭": "NZ", "newzealand": "NZ",
		"us": "US", "usa": "US", "美国": "US", "美國": "US", "unitedstates": "US",
		"ca": "CA", "加拿大": "CA", "canada": "CA",
		"uk": "GB", "gb": "GB", "英国": "GB", "英國": "GB", "unitedkingdom": "GB", "britain": "GB",
		"fr": "FR", "法国": "FR", "法國": "FR", "france": "FR",
		"de": "DE", "德国": "DE", "德國": "DE", "germany": "DE",
		"nl": "NL", "荷兰": "NL", "荷蘭": "NL", "netherlands": "NL",
		"ch": "CH", "瑞士": "CH", "switzerland": "CH",
		"it": "IT", "意大利": "IT", "義大利": "IT", "italy": "IT",
		"es": "ES", "西班牙": "ES", "spain": "ES",
		"pt": "PT", "葡萄牙": "PT", "portugal": "PT",
	}
	if code, ok := aliases[normalizeTimezoneQuery(query)]; ok {
		if zones := countryZones(code); len(zones) > 0 {
			return zones, nil
		}
	}

	all, err := s.Timezones()
	if err != nil {
		return nil, err
	}
	needle := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(query), " ", "_"))
	needleFlat := normalizeTimezoneQuery(query)
	var result []string
	for _, zone := range all {
		lower := strings.ToLower(zone)
		flat := normalizeTimezoneQuery(zone)
		if strings.Contains(lower, needle) || strings.Contains(flat, needleFlat) {
			result = append(result, zone)
		}
		if len(result) >= 80 {
			break
		}
	}
	return result, nil
}

func (s *TimeSyncService) Install(timezone string) (string, error) {
	timezone = strings.TrimSpace(timezone)
	if !validTimezone(timezone) {
		return "", errors.New("无效时区，请选择标准 IANA 时区")
	}
	if !isSupportedTimeSyncHost() {
		return "", errors.New("时间同步功能仅支持使用 systemd 的 Ubuntu/Debian")
	}
	path, cleanup, err := writeTempEmbeddedScript("dui-ntp", embeddedNTPManager)
	if err != nil {
		return "", err
	}
	defer cleanup()
	return runCommandWithEnv(8*time.Minute, []string{"NO_COLOR=1"}, "bash", path, "--install", timezone)
}

func (s *TimeSyncService) SyncNow() (string, error) {
	if _, err := os.Stat(timeSyncManagerPath); err != nil {
		return "", errors.New("时间同步管理器尚未安装")
	}
	return runCommandWithEnv(4*time.Minute, []string{"NO_COLOR=1"}, timeSyncManagerPath, "--sync")
}

func (s *TimeSyncService) SetTimezone(timezone string) (string, error) {
	if !validTimezone(timezone) {
		return "", errors.New("无效时区")
	}
	if _, err := os.Stat(timeSyncManagerPath); err != nil {
		return "", errors.New("请先安装时间同步管理器")
	}
	return runCommandWithEnv(90*time.Second, []string{"NO_COLOR=1"}, timeSyncManagerPath, "--timezone", timezone)
}

func (s *TimeSyncService) Logs() (*TimeSyncLogs, error) {
	logs := &TimeSyncLogs{}
	if commandExists("journalctl") {
		logs.Manager, _ = runSystemCommand(15*time.Second, "journalctl", "-u", "time-sync-manager.service", "-n", "120", "--no-pager")
		logs.TimeSyncd, _ = runSystemCommand(15*time.Second, "journalctl", "-u", "systemd-timesyncd", "-n", "80", "--no-pager")
	}
	return logs, nil
}

func (s *TimeSyncService) Uninstall() error {
	if commandExists("systemctl") {
		_, _ = runSystemCommand(30*time.Second, "systemctl", "disable", "--now", "time-sync-manager.timer")
		_, _ = runSystemCommand(20*time.Second, "systemctl", "stop", "time-sync-manager.service")
	}
	for _, path := range []string{timeSyncServicePath, timeSyncTimerPath, timeSyncManagerPath, timeSyncConfigPath} {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	if commandExists("systemctl") {
		_, _ = runSystemCommand(20*time.Second, "systemctl", "daemon-reload")
		_, _ = runSystemCommand(20*time.Second, "systemctl", "reset-failed", "time-sync-manager.service")
	}
	return nil
}
