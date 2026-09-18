package service

import (
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"x-ui/database"
)

const (
	certManagerMetaPath    = "/etc/x-ui/cert-manager.json"
	certManagerServicePath = "/etc/systemd/system/dui-cert-renew.service"
	certManagerTimerPath   = "/etc/systemd/system/dui-cert-renew.timer"
	acmeShPath             = "/root/.acme.sh/acme.sh"
)

type CertManagerMeta struct {
	Source              string `json:"source"`
	Menu                int    `json:"menu"`
	Domain              string `json:"domain"`
	Wildcard            bool   `json:"wildcard"`
	CertFile            string `json:"certFile"`
	KeyFile             string `json:"keyFile"`
	IssuePort           int    `json:"issuePort,omitempty"`
	ForceRenewEnabled   bool   `json:"forceRenewEnabled"`
	ForceRenewMonths    int    `json:"forceRenewMonths"`
	IssuedAt            int64  `json:"issuedAt"`
	LastForcedRenewAt   int64  `json:"lastForcedRenewAt"`
	LastForcedRenewOK   bool   `json:"lastForcedRenewOk"`
	LastForcedRenewInfo string `json:"lastForcedRenewInfo,omitempty"`
}

type CertManagerStatus struct {
	Managed           bool             `json:"managed"`
	Inferred          bool             `json:"inferred"`
	SourceLabel       string           `json:"sourceLabel"`
	CurrentCertFile   string           `json:"currentCertFile"`
	CurrentKeyFile    string           `json:"currentKeyFile"`
	NotBefore         int64            `json:"notBefore"`
	NotAfter          int64            `json:"notAfter"`
	DaysRemaining     int              `json:"daysRemaining"`
	NextForcedRenewAt int64            `json:"nextForcedRenewAt"`
	TimerActive       bool             `json:"timerActive"`
	TimerEnabled      bool             `json:"timerEnabled"`
	CredentialNote    string           `json:"credentialNote"`
	Metadata          *CertManagerMeta `json:"metadata,omitempty"`
}

type CertManagerService struct{}

func loadCertManagerMeta() (*CertManagerMeta, error) {
	data, err := os.ReadFile(certManagerMetaPath)
	if err != nil {
		return nil, err
	}
	var meta CertManagerMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, err
	}
	return &meta, nil
}

func saveCertManagerMeta(meta *CertManagerMeta) error {
	if meta == nil {
		return errors.New("证书管理信息为空")
	}
	if meta.ForceRenewMonths < 1 || meta.ForceRenewMonths > 24 {
		return errors.New("强制更新周期必须在 1-24 个月")
	}
	if err := os.MkdirAll(filepath.Dir(certManagerMetaPath), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	tmp := certManagerMetaPath + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0600); err != nil {
		return err
	}
	return os.Rename(tmp, certManagerMetaPath)
}

func readCertificateTimes(path string) (notBefore, notAfter time.Time, dnsNames []string, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return time.Time{}, time.Time{}, nil, err
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return time.Time{}, time.Time{}, nil, errors.New("证书 PEM 解析失败")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return time.Time{}, time.Time{}, nil, err
	}
	return cert.NotBefore, cert.NotAfter, cert.DNSNames, nil
}

func certDomainFromPath(path string) string {
	clean := filepath.Clean(path)
	const prefix = "/root/cert/"
	if strings.HasPrefix(clean, prefix) {
		rest := strings.TrimPrefix(clean, prefix)
		if i := strings.Index(rest, "/"); i > 0 {
			return rest[:i]
		}
	}
	return ""
}

func acmeDomainConfig(domain string) string {
	candidates := []string{
		filepath.Join("/root/.acme.sh", domain+"_ecc", domain+".conf"),
		filepath.Join("/root/.acme.sh", domain, domain+".conf"),
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

func readAcmeValue(path, key string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	prefix := key + "='"
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, prefix) && strings.HasSuffix(line, "'") {
			return strings.TrimSuffix(strings.TrimPrefix(line, prefix), "'")
		}
	}
	return ""
}

func inferCertManagerMeta(certFile, keyFile string) (*CertManagerMeta, bool) {
	if certFile == "" || keyFile == "" {
		return nil, false
	}
	notBefore, _, dnsNames, err := readCertificateTimes(certFile)
	if err != nil {
		return nil, false
	}
	domain := certDomainFromPath(certFile)
	if domain == "" {
		for _, name := range dnsNames {
			if !strings.HasPrefix(name, "*.") {
				domain = name
				break
			}
		}
	}
	if domain == "" {
		return nil, false
	}
	conf := acmeDomainConfig(domain)
	if conf == "" {
		return nil, false
	}
	webroot := readAcmeValue(conf, "Le_Webroot")
	alt := readAcmeValue(conf, "Le_Alt")
	source := "menu18-standalone"
	menu := 18
	wildcard := strings.Contains(alt, "*."+domain)
	if strings.Contains(webroot, "dns_cf") {
		source = "menu19-cloudflare"
		menu = 19
		wildcard = true
	}
	meta := &CertManagerMeta{
		Source:            source,
		Menu:              menu,
		Domain:            domain,
		Wildcard:          wildcard,
		CertFile:          certFile,
		KeyFile:           keyFile,
		ForceRenewEnabled: true,
		ForceRenewMonths:  6,
		IssuedAt:          notBefore.Unix(),
		LastForcedRenewAt: notBefore.Unix(),
	}
	return meta, true
}

func certSourceLabel(source string) string {
	switch source {
	case "menu19-cloudflare":
		return "菜单 19 · Cloudflare DNS"
	case "menu18-standalone":
		return "菜单 18 · Standalone HTTP"
	case "menu18-existing":
		return "菜单 18 · 已有证书"
	default:
		return source
	}
}

func (s *CertManagerService) currentPanelCertPaths() (string, string) {
	if database.GetDB() == nil {
		return "", ""
	}
	var setting SettingService
	cert, _ := setting.GetCertFile()
	key, _ := setting.GetKeyFile()
	return cert, key
}

func (s *CertManagerService) ensureMeta() (*CertManagerMeta, bool) {
	meta, err := loadCertManagerMeta()
	if err == nil && meta != nil {
		return meta, false
	}
	cert, key := s.currentPanelCertPaths()
	if inferred, ok := inferCertManagerMeta(cert, key); ok {
		_ = saveCertManagerMeta(inferred)
		_ = s.syncTimer(inferred)
		return inferred, true
	}
	return nil, false
}

func (s *CertManagerService) Status() (*CertManagerStatus, error) {
	certFile, keyFile := s.currentPanelCertPaths()
	st := &CertManagerStatus{
		CurrentCertFile: certFile,
		CurrentKeyFile:  keyFile,
		TimerActive:     systemctlState("is-active", "dui-cert-renew.timer"),
		TimerEnabled:    systemctlState("is-enabled", "dui-cert-renew.timer"),
	}
	meta, inferred := s.ensureMeta()
	if meta != nil {
		st.Managed = true
		st.Inferred = inferred
		st.Metadata = meta
		st.SourceLabel = certSourceLabel(meta.Source)
		if meta.Source == "menu19-cloudflare" {
			st.CredentialNote = "Cloudflare 凭据由 acme.sh 保存；面板仅记录申请方式，不显示 API 密钥/Token。"
		}
		base := meta.LastForcedRenewAt
		if base <= 0 {
			base = meta.IssuedAt
		}
		if base > 0 {
			st.NextForcedRenewAt = time.Unix(base, 0).AddDate(0, meta.ForceRenewMonths, 0).Unix()
		}
	}
	if certFile != "" {
		nb, na, _, err := readCertificateTimes(certFile)
		if err == nil {
			st.NotBefore = nb.Unix()
			st.NotAfter = na.Unix()
			st.DaysRemaining = int(time.Until(na).Hours() / 24)
		}
	}
	return st, nil
}

func (s *CertManagerService) RecordFromMenu(source, domain, certFile, keyFile string, wildcard bool, issuePort int) error {
	domain = strings.TrimSpace(domain)
	if domain == "" || strings.ContainsAny(domain, " \t\r\n/") {
		return errors.New("证书域名无效")
	}
	if certFile == "" || keyFile == "" {
		return errors.New("证书路径不能为空")
	}
	if _, err := os.Stat(certFile); err != nil {
		return fmt.Errorf("证书文件不存在: %w", err)
	}
	if _, err := os.Stat(keyFile); err != nil {
		return fmt.Errorf("私钥文件不存在: %w", err)
	}
	menu := 18
	if source == "menu19-cloudflare" {
		menu = 19
	}
	now := time.Now().Unix()
	if nb, _, _, err := readCertificateTimes(certFile); err == nil {
		now = nb.Unix()
	}
	meta := &CertManagerMeta{
		Source:            source,
		Menu:              menu,
		Domain:            domain,
		Wildcard:          wildcard,
		CertFile:          certFile,
		KeyFile:           keyFile,
		IssuePort:         issuePort,
		ForceRenewEnabled: true,
		ForceRenewMonths:  6,
		IssuedAt:          now,
		LastForcedRenewAt: now,
	}
	if old, err := loadCertManagerMeta(); err == nil && old != nil && old.Domain == domain {
		meta.ForceRenewEnabled = old.ForceRenewEnabled
		if old.ForceRenewMonths >= 1 && old.ForceRenewMonths <= 24 {
			meta.ForceRenewMonths = old.ForceRenewMonths
		}
	}
	if err := saveCertManagerMeta(meta); err != nil {
		return err
	}
	return s.syncTimer(meta)
}

func (s *CertManagerService) Update(enabled bool, months int) error {
	if months < 1 || months > 24 {
		return errors.New("强制更新周期必须在 1-24 个月")
	}
	meta, _ := s.ensureMeta()
	if meta == nil {
		return errors.New("当前证书不是菜单 18/19 管理的 acme.sh 证书")
	}
	meta.ForceRenewEnabled = enabled
	meta.ForceRenewMonths = months
	if err := saveCertManagerMeta(meta); err != nil {
		return err
	}
	return s.syncTimer(meta)
}

func certRenewUnit() string {
	return `[Unit]
Description=Dui managed certificate forced renewal check
After=network-online.target
Wants=network-online.target

[Service]
Type=oneshot
ExecStart=/usr/local/x-ui/x-ui cert-renew-check
`
}

func certRenewTimer() string {
	return `[Unit]
Description=Check Dui managed certificate renewal schedule daily

[Timer]
OnBootSec=10min
OnUnitActiveSec=1d
RandomizedDelaySec=30min
Persistent=true
Unit=dui-cert-renew.service

[Install]
WantedBy=timers.target
`
}

func (s *CertManagerService) syncTimer(meta *CertManagerMeta) error {
	if !commandExists("systemctl") {
		return nil
	}
	if meta == nil || !meta.ForceRenewEnabled {
		_, _ = runSystemCommand(20*time.Second, "systemctl", "disable", "--now", "dui-cert-renew.timer")
		return nil
	}
	if err := os.WriteFile(certManagerServicePath, []byte(certRenewUnit()), 0644); err != nil {
		return err
	}
	if err := os.WriteFile(certManagerTimerPath, []byte(certRenewTimer()), 0644); err != nil {
		return err
	}
	if _, err := runSystemCommand(20*time.Second, "systemctl", "daemon-reload"); err != nil {
		return err
	}
	_, err := runSystemCommand(30*time.Second, "systemctl", "enable", "--now", "dui-cert-renew.timer")
	return err
}

func renewAcmeInstallArgs(meta *CertManagerMeta) []string {
	args := []string{"--installcert", "-d", meta.Domain}
	if meta.Wildcard {
		args = append(args, "-d", "*."+meta.Domain)
	}
	args = append(args,
		"--key-file", meta.KeyFile,
		"--fullchain-file", meta.CertFile,
		"--reloadcmd", "x-ui restart",
	)
	return args
}

func (s *CertManagerService) ForceRenewNow() (string, error) {
	meta, _ := s.ensureMeta()
	if meta == nil {
		return "", errors.New("没有可管理的菜单 18/19 证书")
	}
	return s.forceRenew(meta)
}

func (s *CertManagerService) forceRenew(meta *CertManagerMeta) (string, error) {
	if _, err := os.Stat(acmeShPath); err != nil {
		return "", errors.New("未找到 /root/.acme.sh/acme.sh")
	}
	out1, err := runSystemCommand(8*time.Minute, acmeShPath, "--renew", "-d", meta.Domain, "--force")
	if err != nil {
		meta.LastForcedRenewOK = false
		meta.LastForcedRenewInfo = err.Error()
		_ = saveCertManagerMeta(meta)
		return out1, err
	}
	out2, err := runSystemCommand(3*time.Minute, acmeShPath, renewAcmeInstallArgs(meta)...)
	if err != nil {
		meta.LastForcedRenewOK = false
		meta.LastForcedRenewInfo = err.Error()
		_ = saveCertManagerMeta(meta)
		return strings.TrimSpace(out1 + "\n" + out2), err
	}
	meta.LastForcedRenewAt = time.Now().Unix()
	meta.LastForcedRenewOK = true
	meta.LastForcedRenewInfo = "强制更新成功"
	if nb, _, _, err := readCertificateTimes(meta.CertFile); err == nil {
		meta.IssuedAt = nb.Unix()
	}
	if err := saveCertManagerMeta(meta); err != nil {
		return strings.TrimSpace(out1 + "\n" + out2), err
	}
	return strings.TrimSpace(out1 + "\n" + out2), nil
}

func (s *CertManagerService) CheckDueAndRenew() (string, error) {
	meta, _ := s.ensureMeta()
	if meta == nil || !meta.ForceRenewEnabled {
		return "certificate forced renewal disabled or unmanaged", nil
	}
	if meta.ForceRenewMonths < 1 {
		meta.ForceRenewMonths = 6
	}
	base := meta.LastForcedRenewAt
	if base <= 0 {
		base = meta.IssuedAt
	}
	if base <= 0 {
		base = time.Now().Unix()
		meta.LastForcedRenewAt = base
		_ = saveCertManagerMeta(meta)
		return "renewal baseline initialized", nil
	}
	due := time.Unix(base, 0).AddDate(0, meta.ForceRenewMonths, 0)
	if time.Now().Before(due) {
		return "not due until " + due.Format(time.RFC3339), nil
	}
	return s.forceRenew(meta)
}

func (s *CertManagerService) RecordRenewedNow(domain string) error {
	meta, _ := s.ensureMeta()
	if meta == nil {
		return nil
	}
	if domain != "" && meta.Domain != domain {
		return nil
	}
	meta.LastForcedRenewAt = time.Now().Unix()
	meta.LastForcedRenewOK = true
	meta.LastForcedRenewInfo = "菜单手动强制更新成功"
	if nb, _, _, err := readCertificateTimes(meta.CertFile); err == nil {
		meta.IssuedAt = nb.Unix()
	}
	return saveCertManagerMeta(meta)
}
