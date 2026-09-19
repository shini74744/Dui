package service

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"x-ui/database"
	"x-ui/database/model"
)

const (
	managedBlacklistKindDestination = "destination"
	managedBlacklistKindSourceIP    = "source_ip"
	managedBlacklistScopeGlobal     = "global"
	managedBlacklistScopeClient     = "client"
	managedBlacklistOutboundTag     = "__dui_blacklist_managed"
)

type ManagedBlacklistService struct{}

type ManagedBlacklistStatus struct {
	GlobalBlocked bool `json:"globalBlocked"`
	ClientBlocked bool `json:"clientBlocked"`
	SourceBlocked bool `json:"sourceBlocked"`
}

func normalizeBlacklistDestination(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("目标不能为空")
	}

	if strings.Contains(value, "://") {
		u, err := url.Parse(value)
		if err != nil {
			return "", fmt.Errorf("目标格式无效")
		}
		value = u.Hostname()
	}
	value = strings.Trim(strings.TrimSpace(value), "[]")
	value = strings.TrimSuffix(strings.ToLower(value), ".")
	if value == "" {
		return "", fmt.Errorf("目标格式无效")
	}
	if ip := net.ParseIP(value); ip != nil {
		return ip.String(), nil
	}
	if len(value) > 253 {
		return "", fmt.Errorf("域名过长")
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.' || r == '-' || r == '_' {
			continue
		}
		return "", fmt.Errorf("仅支持域名或 IP 地址")
	}
	if !strings.Contains(value, ".") {
		return "", fmt.Errorf("请输入完整域名或 IP")
	}
	return value, nil
}

func normalizeBlacklistSourceIP(value string) (string, error) {
	value = strings.Trim(strings.TrimSpace(value), "[]")
	ip := net.ParseIP(value)
	if ip == nil {
		return "", fmt.Errorf("来源 IP 无效")
	}
	if ip.IsLoopback() || ip.IsUnspecified() || ip.IsMulticast() {
		return "", fmt.Errorf("不允许拉黑本机回环/未指定/组播地址")
	}
	return ip.String(), nil
}

func managedRuleDomainValue(value string) string {
	if net.ParseIP(value) != nil {
		return ""
	}
	return "domain:" + value
}

func (s *ManagedBlacklistService) AddDestination(inboundID int, email, value, scope string) error {
	normalized, err := normalizeBlacklistDestination(value)
	if err != nil {
		return err
	}
	scope = strings.ToLower(strings.TrimSpace(scope))
	email = strings.TrimSpace(email)
	if scope != managedBlacklistScopeGlobal && scope != managedBlacklistScopeClient {
		return fmt.Errorf("拉黑范围无效")
	}
	if scope == managedBlacklistScopeClient {
		if inboundID <= 0 {
			return fmt.Errorf("客户端拉黑需要有效的入站")
		}
		if email == "" {
			return fmt.Errorf("客户端未填写 Email，无法只对该用户生效")
		}
		var inbound model.Inbound
		if err := database.GetDB().First(&inbound, inboundID).Error; err != nil {
			return fmt.Errorf("入站不存在")
		}
	}

	db := database.GetDB()
	entry := model.ManagedBlacklistEntry{}
	query := db.Where("kind = ? AND scope = ? AND inbound_id = ? AND email = ? AND value = ?",
		managedBlacklistKindDestination, scope,
		func() int {
			if scope == managedBlacklistScopeGlobal {
				return 0
			}
			return inboundID
		}(),
		func() string {
			if scope == managedBlacklistScopeGlobal {
				return ""
			}
			return email
		}(),
		normalized,
	)
	if err := query.First(&entry).Error; err == nil {
		return nil
	}

	entry = model.ManagedBlacklistEntry{
		Kind:      managedBlacklistKindDestination,
		Scope:     scope,
		InboundId: inboundID,
		Email:     email,
		Value:     normalized,
		CreatedAt: time.Now().Unix(),
	}
	if scope == managedBlacklistScopeGlobal {
		entry.InboundId = 0
		entry.Email = ""
	}
	if err := db.Create(&entry).Error; err != nil {
		return err
	}
	if err := s.ApplyManagedXrayRules(); err != nil {
		_ = db.Delete(&entry).Error
		return err
	}
	return nil
}

func (s *ManagedBlacklistService) RemoveDestination(inboundID int, email, value, scope string) error {
	normalized, err := normalizeBlacklistDestination(value)
	if err != nil {
		return err
	}
	scope = strings.ToLower(strings.TrimSpace(scope))
	if scope != managedBlacklistScopeGlobal && scope != managedBlacklistScopeClient {
		return fmt.Errorf("拉黑范围无效")
	}
	if scope == managedBlacklistScopeGlobal {
		inboundID = 0
		email = ""
	}
	db := database.GetDB()
	var entries []model.ManagedBlacklistEntry
	if err := db.Where("kind = ? AND scope = ? AND inbound_id = ? AND email = ? AND value = ?",
		managedBlacklistKindDestination, scope, inboundID, strings.TrimSpace(email), normalized).
		Find(&entries).Error; err != nil {
		return err
	}
	if len(entries) == 0 {
		return nil
	}
	if err := db.Delete(&entries).Error; err != nil {
		return err
	}
	if err := s.ApplyManagedXrayRules(); err != nil {
		for i := range entries {
			entries[i].Id = 0
			_ = db.Create(&entries[i]).Error
		}
		return err
	}
	return nil
}

func (s *ManagedBlacklistService) AddSourceIP(value string) error {
	ip, err := normalizeBlacklistSourceIP(value)
	if err != nil {
		return err
	}
	db := database.GetDB()
	var existing model.ManagedBlacklistEntry
	if err := db.Where("kind = ? AND value = ?", managedBlacklistKindSourceIP, ip).First(&existing).Error; err == nil {
		return nil
	}
	entry := model.ManagedBlacklistEntry{
		Kind:      managedBlacklistKindSourceIP,
		Scope:     managedBlacklistScopeGlobal,
		Value:     ip,
		CreatedAt: time.Now().Unix(),
	}
	if err := db.Create(&entry).Error; err != nil {
		return err
	}
	if err := s.ApplySourceIPBlacklist(); err != nil {
		_ = db.Delete(&entry).Error
		return err
	}
	return nil
}

func (s *ManagedBlacklistService) RemoveSourceIP(value string) error {
	ip, err := normalizeBlacklistSourceIP(value)
	if err != nil {
		return err
	}
	db := database.GetDB()
	var entries []model.ManagedBlacklistEntry
	if err := db.Where("kind = ? AND value = ?", managedBlacklistKindSourceIP, ip).Find(&entries).Error; err != nil {
		return err
	}
	if len(entries) == 0 {
		return nil
	}
	if err := db.Delete(&entries).Error; err != nil {
		return err
	}
	if err := s.ApplySourceIPBlacklist(); err != nil {
		for i := range entries {
			entries[i].Id = 0
			_ = db.Create(&entries[i]).Error
		}
		return err
	}
	return nil
}

func (s *ManagedBlacklistService) DestinationStatus(inboundID int, email, value string) ManagedBlacklistStatus {
	normalized, err := normalizeBlacklistDestination(value)
	if err != nil {
		return ManagedBlacklistStatus{}
	}
	db := database.GetDB()
	var globalCount int64
	var clientCount int64
	db.Model(&model.ManagedBlacklistEntry{}).
		Where("kind = ? AND scope = ? AND value = ?",
			managedBlacklistKindDestination, managedBlacklistScopeGlobal, normalized).
		Count(&globalCount)
	if inboundID > 0 && strings.TrimSpace(email) != "" {
		db.Model(&model.ManagedBlacklistEntry{}).
			Where("kind = ? AND scope = ? AND inbound_id = ? AND email = ? AND value = ?",
				managedBlacklistKindDestination, managedBlacklistScopeClient, inboundID, strings.TrimSpace(email), normalized).
			Count(&clientCount)
	}
	return ManagedBlacklistStatus{GlobalBlocked: globalCount > 0, ClientBlocked: clientCount > 0}
}

type ManagedBlacklistListItem struct {
	Id        int    `json:"id"`
	Kind      string `json:"kind"`
	Scope     string `json:"scope"`
	InboundId int    `json:"inboundId"`
	Protocol  string `json:"protocol"`
	Remark    string `json:"remark"`
	Port      int    `json:"port"`
	Email     string `json:"email"`
	Value     string `json:"value"`
	CreatedAt int64  `json:"createdAt"`
}

func (s *ManagedBlacklistService) List() ([]ManagedBlacklistListItem, error) {
	db := database.GetDB()
	var entries []model.ManagedBlacklistEntry
	if err := db.Order("created_at DESC, id DESC").Find(&entries).Error; err != nil {
		return nil, err
	}

	inboundIDs := make([]int, 0)
	seen := map[int]struct{}{}
	for _, entry := range entries {
		if entry.InboundId > 0 {
			if _, ok := seen[entry.InboundId]; !ok {
				seen[entry.InboundId] = struct{}{}
				inboundIDs = append(inboundIDs, entry.InboundId)
			}
		}
	}

	inboundMap := map[int]model.Inbound{}
	if len(inboundIDs) > 0 {
		var inbounds []model.Inbound
		if err := db.Where("id IN ?", inboundIDs).Find(&inbounds).Error; err != nil {
			return nil, err
		}
		for _, inbound := range inbounds {
			inboundMap[inbound.Id] = inbound
		}
	}

	rows := make([]ManagedBlacklistListItem, 0, len(entries))
	for _, entry := range entries {
		row := ManagedBlacklistListItem{
			Id:        entry.Id,
			Kind:      entry.Kind,
			Scope:     entry.Scope,
			InboundId: entry.InboundId,
			Email:     entry.Email,
			Value:     entry.Value,
			CreatedAt: entry.CreatedAt,
		}
		if inbound, ok := inboundMap[entry.InboundId]; ok {
			row.Protocol = strings.ToUpper(string(inbound.Protocol))
			row.Remark = inbound.Remark
			row.Port = inbound.Port
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func (s *ManagedBlacklistService) RemoveByID(id int) error {
	if id <= 0 {
		return fmt.Errorf("黑名单 ID 无效")
	}
	db := database.GetDB()
	var entry model.ManagedBlacklistEntry
	if err := db.First(&entry, id).Error; err != nil {
		return fmt.Errorf("黑名单记录不存在")
	}
	if err := db.Delete(&entry).Error; err != nil {
		return err
	}

	var applyErr error
	switch entry.Kind {
	case managedBlacklistKindDestination:
		applyErr = s.ApplyManagedXrayRules()
	case managedBlacklistKindSourceIP:
		applyErr = s.ApplySourceIPBlacklist()
	default:
		applyErr = fmt.Errorf("未知黑名单类型: %s", entry.Kind)
	}
	if applyErr == nil {
		return nil
	}

	entry.Id = 0
	_ = db.Create(&entry).Error
	if entry.Kind == managedBlacklistKindDestination {
		_ = s.ApplyManagedXrayRules()
	} else if entry.Kind == managedBlacklistKindSourceIP {
		_ = s.ApplySourceIPBlacklist()
	}
	return applyErr
}

func (s *ManagedBlacklistService) Clear(kind string) error {
	kind = strings.TrimSpace(kind)
	if kind != "" && kind != managedBlacklistKindDestination && kind != managedBlacklistKindSourceIP {
		return fmt.Errorf("黑名单类型无效")
	}

	db := database.GetDB()
	var entries []model.ManagedBlacklistEntry
	query := db
	if kind != "" {
		query = query.Where("kind = ?", kind)
	}
	if err := query.Find(&entries).Error; err != nil {
		return err
	}
	if len(entries) == 0 {
		return nil
	}

	ids := make([]int, 0, len(entries))
	for _, entry := range entries {
		ids = append(ids, entry.Id)
	}
	if err := db.Where("id IN ?", ids).Delete(&model.ManagedBlacklistEntry{}).Error; err != nil {
		return err
	}

	needDestination := kind == "" || kind == managedBlacklistKindDestination
	needSourceIP := kind == "" || kind == managedBlacklistKindSourceIP
	var applyErr error
	if needDestination {
		applyErr = s.ApplyManagedXrayRules()
	}
	if applyErr == nil && needSourceIP {
		applyErr = s.ApplySourceIPBlacklist()
	}
	if applyErr == nil {
		return nil
	}

	for i := range entries {
		entries[i].Id = 0
		_ = db.Create(&entries[i]).Error
	}
	if needDestination {
		_ = s.ApplyManagedXrayRules()
	}
	if needSourceIP {
		_ = s.ApplySourceIPBlacklist()
	}
	return applyErr
}

func (s *ManagedBlacklistService) SourceIPStatus(value string) ManagedBlacklistStatus {
	ip, err := normalizeBlacklistSourceIP(value)
	if err != nil {
		return ManagedBlacklistStatus{}
	}
	var count int64
	database.GetDB().Model(&model.ManagedBlacklistEntry{}).
		Where("kind = ? AND value = ?", managedBlacklistKindSourceIP, ip).
		Count(&count)
	return ManagedBlacklistStatus{SourceBlocked: count > 0}
}

func (s *ManagedBlacklistService) ApplyManagedXrayRules() error {
	var entries []model.ManagedBlacklistEntry
	if err := database.GetDB().
		Where("kind = ?", managedBlacklistKindDestination).
		Order("scope ASC, inbound_id ASC, email ASC, value ASC").
		Find(&entries).Error; err != nil {
		return err
	}

	var settingService SettingService
	oldSetting, err := settingService.GetXrayConfigTemplate()
	if err != nil {
		return err
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(oldSetting), &raw); err != nil {
		return err
	}

	outbounds, _ := raw["outbounds"].([]any)
	filteredOutbounds := make([]any, 0, len(outbounds)+1)
	for _, item := range outbounds {
		obj, ok := item.(map[string]any)
		if ok {
			if tag, _ := obj["tag"].(string); tag == managedBlacklistOutboundTag {
				continue
			}
		}
		filteredOutbounds = append(filteredOutbounds, item)
	}
	if len(entries) > 0 {
		filteredOutbounds = append(filteredOutbounds, map[string]any{
			"tag":      managedBlacklistOutboundTag,
			"protocol": "blackhole",
			"settings": map[string]any{},
		})
	}
	raw["outbounds"] = filteredOutbounds

	routing, _ := raw["routing"].(map[string]any)
	if routing == nil {
		routing = map[string]any{"domainStrategy": "AsIs"}
		raw["routing"] = routing
	}
	rules, _ := routing["rules"].([]any)
	userRules := make([]any, 0, len(rules))
	for _, item := range rules {
		obj, ok := item.(map[string]any)
		if ok {
			if tag, _ := obj["outboundTag"].(string); tag == managedBlacklistOutboundTag {
				continue
			}
		}
		userRules = append(userRules, item)
	}

	type group struct {
		InboundTag string
		Email      string
		Domains    []string
		IPs        []string
	}
	groups := map[string]*group{}
	global := &group{}
	inboundCache := map[int]model.Inbound{}

	for _, entry := range entries {
		var g *group
		if entry.Scope == managedBlacklistScopeGlobal {
			g = global
		} else {
			inbound, ok := inboundCache[entry.InboundId]
			if !ok {
				if err := database.GetDB().First(&inbound, entry.InboundId).Error; err != nil {
					continue
				}
				inboundCache[entry.InboundId] = inbound
			}
			inboundTag := strings.TrimSpace(inbound.Tag)
			if inboundTag == "" {
				inboundTag = fmt.Sprintf("inbound-%d", inbound.Port)
			}
			k := strconv.Itoa(entry.InboundId) + "|" + strings.ToLower(strings.TrimSpace(entry.Email))
			g = groups[k]
			if g == nil {
				g = &group{InboundTag: inboundTag, Email: strings.TrimSpace(entry.Email)}
				groups[k] = g
			}
		}
		if net.ParseIP(entry.Value) != nil {
			g.IPs = append(g.IPs, entry.Value)
		} else {
			g.Domains = append(g.Domains, managedRuleDomainValue(entry.Value))
		}
	}

	managedRules := make([]any, 0)
	appendRules := func(g *group) {
		if g == nil {
			return
		}
		common := map[string]any{
			"type":        "field",
			"outboundTag": managedBlacklistOutboundTag,
		}
		if g.InboundTag != "" {
			common["inboundTag"] = []string{g.InboundTag}
		}
		if g.Email != "" {
			common["user"] = []string{g.Email}
		}
		if len(g.Domains) > 0 {
			sort.Strings(g.Domains)
			r := map[string]any{}
			for k, v := range common {
				r[k] = v
			}
			r["domain"] = g.Domains
			managedRules = append(managedRules, r)
		}
		if len(g.IPs) > 0 {
			sort.Strings(g.IPs)
			r := map[string]any{}
			for k, v := range common {
				r[k] = v
			}
			r["ip"] = g.IPs
			managedRules = append(managedRules, r)
		}
	}
	appendRules(global)
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		appendRules(groups[k])
	}
	routing["rules"] = append(managedRules, userRules...)

	newBytes, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return err
	}
	xraySettingService := &XraySettingService{}
	if err := xraySettingService.SaveXraySetting(string(newBytes)); err != nil {
		return err
	}
	xrayService := &XrayService{}
	if err := xrayService.RestartXray(true); err != nil {
		_ = xraySettingService.SaveXraySetting(oldSetting)
		_ = xrayService.RestartXray(true)
		return fmt.Errorf("黑名单规则保存后 Xray 启动失败，已回滚: %w", err)
	}
	return nil
}

func (s *ManagedBlacklistService) ApplySourceIPBlacklist() error {
	var entries []model.ManagedBlacklistEntry
	if err := database.GetDB().
		Where("kind = ?", managedBlacklistKindSourceIP).
		Order("value ASC").
		Find(&entries).Error; err != nil {
		return err
	}

	if len(entries) == 0 {
		if commandExists("nft") {
			_, _ = runSystemCommand(5*time.Second, "nft", "delete", "table", "inet", "dui_blacklist")
		}
		if commandExists("iptables") {
			chain := "DUI-BLACKLIST"
			_, _ = runSystemCommand(5*time.Second, "iptables", "-D", "INPUT", "-j", chain)
			_, _ = runSystemCommand(5*time.Second, "iptables", "-F", chain)
			_, _ = runSystemCommand(5*time.Second, "iptables", "-X", chain)
			if commandExists("ip6tables") {
				_, _ = runSystemCommand(5*time.Second, "ip6tables", "-D", "INPUT", "-j", chain)
				_, _ = runSystemCommand(5*time.Second, "ip6tables", "-F", chain)
				_, _ = runSystemCommand(5*time.Second, "ip6tables", "-X", chain)
			}
		}
		return nil
	}
	if commandExists("nft") {
		return s.applyNftSourceBlacklist(entries)
	}
	if commandExists("iptables") {
		return s.applyIptablesSourceBlacklist(entries)
	}
	return fmt.Errorf("系统未检测到 nftables 或 iptables，无法拒绝来源 IP")
}

func (s *ManagedBlacklistService) applyNftSourceBlacklist(entries []model.ManagedBlacklistEntry) error {
	_, _ = runSystemCommand(5*time.Second, "nft", "delete", "table", "inet", "dui_blacklist")
	script := []string{
		"table inet dui_blacklist {",
		"  chain input {",
		"    type filter hook input priority -10; policy accept;",
	}
	for _, entry := range entries {
		ip := net.ParseIP(entry.Value)
		if ip == nil {
			continue
		}
		if ip.To4() != nil {
			script = append(script, "    ip saddr "+ip.String()+" reject")
		} else {
			script = append(script, "    ip6 saddr "+ip.String()+" reject")
		}
	}
	script = append(script, "  }", "}", "")
	path := filepath.Join(os.TempDir(), "dui-blacklist.nft")
	if err := os.WriteFile(path, []byte(strings.Join(script, "\n")), 0600); err != nil {
		return err
	}
	defer os.Remove(path)
	_, err := runSystemCommand(10*time.Second, "nft", "-f", path)
	return err
}

func (s *ManagedBlacklistService) applyIptablesSourceBlacklist(entries []model.ManagedBlacklistEntry) error {
	chain := "DUI-BLACKLIST"
	_, _ = runSystemCommand(5*time.Second, "iptables", "-N", chain)
	if _, err := runSystemCommand(5*time.Second, "iptables", "-C", "INPUT", "-j", chain); err != nil {
		if _, err := runSystemCommand(5*time.Second, "iptables", "-I", "INPUT", "1", "-j", chain); err != nil {
			return err
		}
	}
	if _, err := runSystemCommand(5*time.Second, "iptables", "-F", chain); err != nil {
		return err
	}

	hasIPv6 := false
	for _, entry := range entries {
		ip := net.ParseIP(entry.Value)
		if ip == nil {
			continue
		}
		if ip.To4() != nil {
			if _, err := runSystemCommand(5*time.Second, "iptables", "-A", chain, "-s", ip.String(), "-j", "REJECT"); err != nil {
				return err
			}
		} else {
			hasIPv6 = true
		}
	}
	if hasIPv6 && commandExists("ip6tables") {
		_, _ = runSystemCommand(5*time.Second, "ip6tables", "-N", chain)
		if _, err := runSystemCommand(5*time.Second, "ip6tables", "-C", "INPUT", "-j", chain); err != nil {
			if _, err := runSystemCommand(5*time.Second, "ip6tables", "-I", "INPUT", "1", "-j", chain); err != nil {
				return err
			}
		}
		if _, err := runSystemCommand(5*time.Second, "ip6tables", "-F", chain); err != nil {
			return err
		}
		for _, entry := range entries {
			ip := net.ParseIP(entry.Value)
			if ip != nil && ip.To4() == nil {
				if _, err := runSystemCommand(5*time.Second, "ip6tables", "-A", chain, "-s", ip.String(), "-j", "REJECT"); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
