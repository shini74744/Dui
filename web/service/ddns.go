package service

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

//go:embed ddns.sh
var embeddedDDNSManager string

const (
	ddnsConfigDir   = "/etc/cloudflare-ddns"
	ddnsConfigPath  = "/etc/cloudflare-ddns/config.json"
	ddnsLegacyPath  = "/etc/cloudflare-ddns.env"
	ddnsManagerPath = "/usr/local/sbin/cloudflare-ddns"
	ddnsRuntimePath = "/usr/local/sbin/cloudflare-ddns-update"
	ddnsStateDir    = "/var/lib/cloudflare-ddns"
	ddnsLogPath     = "/var/log/cf_ddns.log"
	ddnsServicePath = "/etc/systemd/system/cf-ddns.service"
	ddnsTimerPath   = "/etc/systemd/system/cf-ddns.timer"
)

type ddnsConfigInternal struct {
	Version  int                  `json:"version"`
	Interval int                  `json:"interval"`
	Records  []ddnsRecordInternal `json:"records"`
}

type ddnsRecordInternal struct {
	Name    string `json:"name"`
	Zone    string `json:"zone"`
	Token   string `json:"token"`
	ZoneID  string `json:"zone_id"`
	Enabled bool   `json:"enabled"`
}

type DDNSRecord struct {
	Name         string `json:"name"`
	Zone         string `json:"zone"`
	Enabled      bool   `json:"enabled"`
	TokenPresent bool   `json:"tokenPresent"`
	Status       string `json:"status"`
	Message      string `json:"message"`
	CloudflareIP string `json:"cloudflareIp"`
	CheckedAt    string `json:"checkedAt"`
	UpdatedAt    string `json:"updatedAt"`
	Proxied      *bool  `json:"proxied,omitempty"`
}

type DDNSLastRun struct {
	Status     string `json:"status"`
	Message    string `json:"message"`
	PublicIPv4 string `json:"public_ipv4"`
	CheckedAt  string `json:"checked_at"`
}

type DDNSStatus struct {
	Installed       bool         `json:"installed"`
	Configured      bool         `json:"configured"`
	LegacyDetected  bool         `json:"legacyDetected"`
	Version         string       `json:"version"`
	Interval        int          `json:"interval"`
	Init            string       `json:"init"`
	ScheduleActive  bool         `json:"scheduleActive"`
	ScheduleEnabled bool         `json:"scheduleEnabled"`
	Records         []DDNSRecord `json:"records"`
	LastRun         *DDNSLastRun `json:"lastRun,omitempty"`
}

type DDNSService struct{}

var (
	ddnsDomainRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)+$`)
	ddnsTokenRe  = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
)

func detectDDNSInit() string {
	if _, err := os.Stat("/run/systemd/system"); err == nil && commandExists("systemctl") {
		if exec.Command("systemctl", "show", "--property=Version").Run() == nil {
			return "systemd"
		}
	}
	if _, err := os.Stat("/run/openrc"); err == nil && commandExists("rc-service") && commandExists("rc-update") {
		return "openrc"
	}
	return ""
}

func readDDNSConfig() (*ddnsConfigInternal, error) {
	data, err := os.ReadFile(ddnsConfigPath)
	if err != nil {
		return nil, err
	}
	var cfg ddnsConfigInternal
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	if cfg.Version != 2 || cfg.Interval < 30 || cfg.Interval > 86400 {
		return nil, errors.New("DDNS 配置格式或版本不受支持")
	}
	return &cfg, nil
}

func ddnsRecordStatePath(name string) string {
	if len(name) <= 250 {
		return filepath.Join(ddnsStateDir, "records", name+".json")
	}
	sum := sha256.Sum256([]byte(name))
	return filepath.Join(ddnsStateDir, "records", "sha256-"+hex.EncodeToString(sum[:])+".json")
}

func loadJSON(path string, v any) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return json.Unmarshal(data, v) == nil
}

func tailTextFile(path string, maxLines int) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if maxLines > 0 && len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}
	return strings.Join(lines, "\n")
}

func (s *DDNSService) Status() (*DDNSStatus, error) {
	st := &DDNSStatus{
		Installed:      false,
		LegacyDetected: false,
		Init:           detectDDNSInit(),
		Records:        []DDNSRecord{},
	}
	if info, err := os.Stat(ddnsManagerPath); err == nil && info.Mode().IsRegular() {
		st.Installed = true
		st.Version = fileVersion(ddnsManagerPath)
	}
	if _, err := os.Stat(ddnsLegacyPath); err == nil {
		st.LegacyDetected = true
	}
	cfg, err := readDDNSConfig()
	if err == nil {
		st.Configured = true
		st.Interval = cfg.Interval
		for _, rec := range cfg.Records {
			item := DDNSRecord{
				Name:         rec.Name,
				Zone:         rec.Zone,
				Enabled:      rec.Enabled,
				TokenPresent: rec.Token != "",
			}
			var state struct {
				Status       string `json:"status"`
				Message      string `json:"message"`
				CloudflareIP string `json:"cloudflare_ip"`
				CheckedAt    string `json:"checked_at"`
				UpdatedAt    string `json:"updated_at"`
				Proxied      *bool  `json:"proxied"`
			}
			if loadJSON(ddnsRecordStatePath(rec.Name), &state) {
				item.Status = state.Status
				item.Message = state.Message
				item.CloudflareIP = state.CloudflareIP
				item.CheckedAt = state.CheckedAt
				item.UpdatedAt = state.UpdatedAt
				item.Proxied = state.Proxied
			}
			st.Records = append(st.Records, item)
		}
		var last DDNSLastRun
		if loadJSON(filepath.Join(ddnsStateDir, "last-run.json"), &last) {
			st.LastRun = &last
		}
	}
	switch st.Init {
	case "systemd":
		st.ScheduleActive = systemctlState("is-active", "cf-ddns.timer")
		st.ScheduleEnabled = systemctlState("is-enabled", "cf-ddns.timer")
	case "openrc":
		if commandExists("rc-service") {
			st.ScheduleActive = exec.Command("rc-service", "cf-ddns", "status").Run() == nil
		}
		if commandExists("rc-update") {
			out, _ := exec.Command("rc-update", "show", "default").CombinedOutput()
			st.ScheduleEnabled = strings.Contains(string(out), "cf-ddns")
		}
	}
	return st, nil
}

func validateDDNSDomain(v string) string {
	v = strings.TrimSpace(strings.ToLower(v))
	v = strings.TrimSuffix(v, ".")
	return v
}

func validateDDNSToken(token string) error {
	if token == "" || len(token) > 2048 || !ddnsTokenRe.MatchString(token) {
		return errors.New("Cloudflare API Token 格式无效")
	}
	return nil
}

func (s *DDNSService) runEmbedded(args []string, token string, timeout time.Duration) (string, error) {
	path, cleanup, err := writeTempEmbeddedScript("dui-ddns", embeddedDDNSManager)
	if err != nil {
		return "", err
	}
	defer cleanup()
	env := []string{}
	var tokenPath string
	if token != "" {
		if err := validateDDNSToken(token); err != nil {
			return "", err
		}
		f, err := os.CreateTemp("/tmp", "dui-ddns-token-*")
		if err != nil {
			return "", err
		}
		tokenPath = f.Name()
		_ = os.Chmod(tokenPath, 0600)
		if _, err := f.WriteString(token); err != nil {
			_ = f.Close()
			_ = os.Remove(tokenPath)
			return "", err
		}
		_ = f.Close()
		defer os.Remove(tokenPath)
		env = append(env, "CF_PANEL_TOKEN_FILE="+tokenPath)
	}
	allArgs := append([]string{path}, args...)
	return runCommandWithEnv(timeout, env, "/bin/sh", allArgs...)
}

func (s *DDNSService) Install() (string, error) {
	init := detectDDNSInit()
	if init == "" {
		return "", errors.New("未检测到可用的 systemd/OpenRC")
	}
	return s.runEmbedded([]string{"--panel-install"}, "", 8*time.Minute)
}

func (s *DDNSService) Add(zone, name, token string, createMissing bool) (string, error) {
	zone = validateDDNSDomain(zone)
	name = strings.TrimSpace(name)
	if name != "@" {
		name = validateDDNSDomain(name)
	}
	if !ddnsDomainRe.MatchString(zone) {
		return "", errors.New("Zone 格式无效")
	}
	if name != "@" && (!ddnsDomainRe.MatchString(name) || (name != zone && !strings.HasSuffix(name, "."+zone))) {
		return "", errors.New("完整域名格式无效或不属于指定 Zone")
	}
	flag := "false"
	if createMissing {
		flag = "true"
	}
	return s.runEmbedded([]string{"--panel-add", zone, name, flag}, token, 4*time.Minute)
}

func (s *DDNSService) Delete(name string) (string, error) {
	name = validateDDNSDomain(name)
	if !ddnsDomainRe.MatchString(name) {
		return "", errors.New("域名格式无效")
	}
	return s.runEmbedded([]string{"--panel-delete", name}, "", 90*time.Second)
}

func (s *DDNSService) SetEnabled(name string, enabled bool) (string, error) {
	name = validateDDNSDomain(name)
	if !ddnsDomainRe.MatchString(name) {
		return "", errors.New("域名格式无效")
	}
	value := "false"
	if enabled {
		value = "true"
	}
	return s.runEmbedded([]string{"--panel-enabled", name, value}, "", 90*time.Second)
}

func (s *DDNSService) ReplaceToken(name, token string) (string, error) {
	name = validateDDNSDomain(name)
	if !ddnsDomainRe.MatchString(name) {
		return "", errors.New("域名格式无效")
	}
	return s.runEmbedded([]string{"--panel-token", name}, token, 3*time.Minute)
}

func (s *DDNSService) SetInterval(interval int) (string, error) {
	if interval < 30 || interval > 86400 {
		return "", errors.New("检查间隔必须在 30~86400 秒")
	}
	return s.runEmbedded([]string{"--panel-interval", fmt.Sprintf("%d", interval)}, "", 90*time.Second)
}

func (s *DDNSService) SetSchedule(enabled bool) (string, error) {
	mode := "stop"
	if enabled {
		mode = "start"
	}
	return s.runEmbedded([]string{"--panel-schedule", mode}, "", 90*time.Second)
}

func (s *DDNSService) RunNow() (string, error) {
	if _, err := os.Stat(ddnsManagerPath); err != nil {
		return "", errors.New("DDNS 尚未安装")
	}
	return runSystemCommand(6*time.Minute, ddnsManagerPath, "--run")
}

func (s *DDNSService) Logs() string {
	return tailTextFile(ddnsLogPath, 200)
}

func (s *DDNSService) UpdateScript() (string, error) {
	if _, err := os.Stat(ddnsManagerPath); err != nil {
		return "", errors.New("DDNS 尚未安装")
	}
	return runSystemCommand(5*time.Minute, ddnsManagerPath, "--update")
}

func (s *DDNSService) Uninstall() (string, error) {
	return s.runEmbedded([]string{"--panel-uninstall"}, "", 3*time.Minute)
}
