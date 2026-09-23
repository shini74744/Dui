package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	blackmatrixCatalogURL = "https://api.github.com/repos/blackmatrix7/ios_rule_script/contents/rule/Clash?ref=master"
	blackmatrixRawBase    = "https://raw.githubusercontent.com/blackmatrix7/ios_rule_script/master/rule/Clash/"
	ruleCatalogCachePath  = "/var/cache/dui/blackmatrix-clash.json"
)

var customRuleListsPath = "/etc/x-ui/custom_rule_lists.json"

type RuleSetApp struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	Category string `json:"category,omitempty"`
}

type RuleSetImportResult struct {
	Name        string         `json:"name"`
	Path        string         `json:"path"`
	Domains     []string       `json:"domains"`
	IPs         []string       `json:"ips"`
	Imported    int            `json:"imported"`
	Skipped     int            `json:"skipped"`
	TypeCounts  map[string]int `json:"typeCounts"`
	SkippedType map[string]int `json:"skippedTypes"`
	Updated     string         `json:"updated,omitempty"`
}

type RuleSetBatchResult struct {
	Domains    []string       `json:"domains"`
	IPs        []string       `json:"ips"`
	Imported   int            `json:"imported"`
	Skipped    int            `json:"skipped"`
	Files      int            `json:"files"`
	Failed     []string       `json:"failed"`
	TypeCounts map[string]int `json:"typeCounts"`
}

type CustomRuleList struct {
	ID        string   `json:"id" form:"id"`
	Name      string   `json:"name" form:"name"`
	URLs      []string `json:"urls" form:"urls"`
	URL       string   `json:"url,omitempty" form:"url"` // legacy single-address format, migrated on read
	CreatedAt int64    `json:"createdAt"`
	UpdatedAt int64    `json:"updatedAt"`
}

type RuleSetService struct{}

var (
	ruleCatalogMu       sync.Mutex
	ruleCatalogMemory   []RuleSetApp
	ruleCatalogLoadedAt time.Time
	customRuleListsMu   sync.Mutex
	ruleSetPathRe       = regexp.MustCompile(`^[A-Za-z0-9_.@+'-]+/[A-Za-z0-9_.@+'-]+\.list$`)
	ruleAppNameRe       = regexp.MustCompile(`^[A-Za-z0-9_.@+'-]+$`)
)

type githubContentItem struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Type string `json:"type"`
}

type ruleCatalogDiskCache struct {
	Updated int64        `json:"updated"`
	Apps    []RuleSetApp `json:"apps"`
}

func publicRuleIP(ip net.IP) bool {
	if ip == nil || !ip.IsGlobalUnicast() || ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() || ip.IsMulticast() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return false
	}
	// CGNAT 100.64.0.0/10 is not returned by IsPrivate but must not be reachable by custom rules.
	if v4 := ip.To4(); v4 != nil && v4[0] == 100 && v4[1] >= 64 && v4[1] <= 127 {
		return false
	}
	return true
}

func validatePublicRuleURL(rawURL string) (*url.URL, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return nil, errors.New("自定义 List 地址不能为空")
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, errors.New("自定义 List 地址格式无效")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, errors.New("自定义 List 仅支持 http/https 地址")
	}
	if u.User != nil || strings.TrimSpace(u.Hostname()) == "" {
		return nil, errors.New("自定义 List 地址格式无效")
	}
	host := strings.TrimSpace(u.Hostname())
	if strings.EqualFold(host, "localhost") || strings.HasSuffix(strings.ToLower(host), ".localhost") {
		return nil, errors.New("自定义 List 不允许访问 localhost/内网地址")
	}
	if ip := net.ParseIP(host); ip != nil {
		if !publicRuleIP(ip) {
			return nil, errors.New("自定义 List 不允许访问 localhost/内网地址")
		}
	}
	return u, nil
}

func normalizeCustomRuleURLs(urls []string, legacyURL string) ([]string, error) {
	if len(urls) == 0 && strings.TrimSpace(legacyURL) != "" {
		urls = []string{legacyURL}
	}
	if len(urls) == 0 {
		return nil, errors.New("自定义标签至少需要一个 List 地址")
	}
	if len(urls) > 20 {
		return nil, errors.New("一个自定义标签最多保存 20 个 List 地址")
	}
	result := make([]string, 0, len(urls))
	seen := map[string]struct{}{}
	for _, rawURL := range urls {
		rawURL = strings.TrimSpace(rawURL)
		if rawURL == "" {
			continue
		}
		if len(rawURL) > 2048 {
			return nil, errors.New("自定义 List 地址过长")
		}
		u, err := validatePublicRuleURL(rawURL)
		if err != nil {
			return nil, err
		}
		normalized := u.String()
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		result = append(result, normalized)
	}
	if len(result) == 0 {
		return nil, errors.New("自定义标签至少需要一个有效 List 地址")
	}
	return result, nil
}

func readCustomRuleListsUnlocked() ([]CustomRuleList, error) {
	data, err := os.ReadFile(customRuleListsPath)
	if os.IsNotExist(err) {
		return []CustomRuleList{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("读取自定义 List 收藏失败: %w", err)
	}
	var items []CustomRuleList
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, fmt.Errorf("解析自定义 List 收藏失败: %w", err)
	}
	if items == nil {
		items = []CustomRuleList{}
	}
	for i := range items {
		if len(items[i].URLs) == 0 && strings.TrimSpace(items[i].URL) != "" {
			items[i].URLs = []string{strings.TrimSpace(items[i].URL)}
		}
		items[i].URL = ""
	}
	return items, nil
}

func writeCustomRuleListsUnlocked(items []CustomRuleList) error {
	data, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := writeAtomicFile(customRuleListsPath, data, 0600); err != nil {
		return fmt.Errorf("保存自定义 List 收藏失败: %w", err)
	}
	return nil
}

func (s *RuleSetService) CustomLists() ([]CustomRuleList, error) {
	customRuleListsMu.Lock()
	defer customRuleListsMu.Unlock()
	items, err := readCustomRuleListsUnlocked()
	if err != nil {
		return nil, err
	}
	return append([]CustomRuleList(nil), items...), nil
}

func (s *RuleSetService) SaveCustomList(item CustomRuleList) (*CustomRuleList, error) {
	item.Name = strings.TrimSpace(item.Name)
	item.ID = strings.TrimSpace(item.ID)
	if item.Name == "" {
		return nil, errors.New("自定义标签名称不能为空")
	}
	if len(item.Name) > 80 {
		return nil, errors.New("自定义标签名称最多 80 个字符")
	}
	urls, err := normalizeCustomRuleURLs(item.URLs, item.URL)
	if err != nil {
		return nil, err
	}
	item.URLs = urls
	item.URL = ""

	customRuleListsMu.Lock()
	defer customRuleListsMu.Unlock()
	items, err := readCustomRuleListsUnlocked()
	if err != nil {
		return nil, err
	}

	index := -1
	for i := range items {
		if item.ID != "" && items[i].ID == item.ID {
			index = i
			break
		}
	}
	for i := range items {
		if index >= 0 && i == index {
			continue
		}
		existing := map[string]struct{}{}
		for _, existingURL := range items[i].URLs {
			existing[strings.TrimSpace(existingURL)] = struct{}{}
		}
		for _, candidate := range item.URLs {
			if _, ok := existing[candidate]; ok {
				return nil, fmt.Errorf("List 地址已存在于自定义标签“%s”中", items[i].Name)
			}
		}
	}

	now := time.Now().Unix()
	if index >= 0 {
		item.CreatedAt = items[index].CreatedAt
		if item.CreatedAt == 0 {
			item.CreatedAt = now
		}
		item.UpdatedAt = now
		items[index] = item
	} else {
		item.ID = fmt.Sprintf("custom-%d", time.Now().UnixNano())
		item.CreatedAt = now
		item.UpdatedAt = now
		items = append(items, item)
	}
	if err := writeCustomRuleListsUnlocked(items); err != nil {
		return nil, err
	}
	saved := item
	return &saved, nil
}

func (s *RuleSetService) DeleteCustomList(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("自定义 List ID 不能为空")
	}
	customRuleListsMu.Lock()
	defer customRuleListsMu.Unlock()
	items, err := readCustomRuleListsUnlocked()
	if err != nil {
		return err
	}
	next := make([]CustomRuleList, 0, len(items))
	found := false
	for _, item := range items {
		if item.ID == id {
			found = true
			continue
		}
		next = append(next, item)
	}
	if !found {
		return errors.New("自定义 List 不存在")
	}
	return writeCustomRuleListsUnlocked(next)
}

func dialPublicRuleURL(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
	if err != nil {
		return nil, err
	}
	for _, ip := range ips {
		if publicRuleIP(ip) {
			dialer := &net.Dialer{Timeout: 10 * time.Second}
			return dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		}
	}
	return nil, errors.New("自定义 List 域名解析到 localhost/内网地址")
}

func fetchPublicURLLimited(rawURL string, limit int64) ([]byte, error) {
	u, err := validatePublicRuleURL(rawURL)
	if err != nil {
		return nil, err
	}
	transport := &http.Transport{
		DialContext:           dialPublicRuleURL,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 15 * time.Second,
	}
	client := &http.Client{
		Timeout:   25 * time.Second,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("自定义 List 重定向次数过多")
			}
			_, err := validatePublicRuleURL(req.URL.String())
			return err
		},
	}
	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Dui")
	req.Header.Set("Accept", "text/plain,*/*;q=0.8")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("远端规则返回 HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength > limit && resp.ContentLength > 0 {
		return nil, errors.New("远端规则内容过大")
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errors.New("远端规则内容超过安全限制")
	}
	return data, nil
}

func fetchURLLimited(rawURL string, limit int64) ([]byte, error) {

	client := &http.Client{Timeout: 20 * time.Second}
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Dui")
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub 返回 HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength > limit && resp.ContentLength > 0 {
		return nil, errors.New("远端规则内容过大")
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errors.New("远端规则内容超过安全限制")
	}
	return data, nil
}

func loadRuleCatalogDisk() ([]RuleSetApp, time.Time) {
	data, err := os.ReadFile(ruleCatalogCachePath)
	if err != nil {
		return nil, time.Time{}
	}
	var cache ruleCatalogDiskCache
	if json.Unmarshal(data, &cache) != nil || len(cache.Apps) == 0 {
		return nil, time.Time{}
	}
	for i := range cache.Apps {
		if cache.Apps[i].Category == "" {
			cache.Apps[i].Category = ruleAppCategory(cache.Apps[i].Name)
		}
	}
	return cache.Apps, time.Unix(cache.Updated, 0)
}

func saveRuleCatalogDisk(apps []RuleSetApp) {
	if len(apps) == 0 {
		return
	}
	_ = os.MkdirAll(filepath.Dir(ruleCatalogCachePath), 0755)
	data, err := json.Marshal(ruleCatalogDiskCache{Updated: time.Now().Unix(), Apps: apps})
	if err != nil {
		return
	}
	tmp := ruleCatalogCachePath + ".tmp"
	if os.WriteFile(tmp, data, 0644) == nil {
		_ = os.Rename(tmp, ruleCatalogCachePath)
	}
}

func ruleAppCategory(name string) string {
	ai := map[string]bool{
		"Anthropic": true, "BardAI": true, "Civitai": true, "Claude": true,
		"Copilot": true, "Gemini": true, "OpenAI": true,
	}
	social := map[string]bool{
		"Discord": true, "Facebook": true, "Instagram": true, "Line": true,
		"LinkedIn": true, "Pinterest": true, "Reddit": true, "Telegram": true,
		"TelegramNL": true, "TelegramSG": true, "TelegramUS": true, "Threads": true,
		"Twitter": true, "WeChat": true, "Whatsapp": true,
	}
	adult := map[string]bool{"EHGallery": true, "Japonx": true}
	music := map[string]bool{
		"Spotify": true, "AppleMusic": true, "YouTubeMusic": true, "Tidal": true,
		"Pandora": true, "SoundCloud": true, "Deezer": true, "KKBOX": true, "JOOX": true,
	}
	game := map[string]bool{
		"Steam": true, "SteamCN": true, "Epic": true, "EpicGames": true,
		"PlayStation": true, "Nintendo": true, "Xbox": true, "Blizzard": true,
		"EA": true, "Ubisoft": true, "Riot": true,
	}
	video := map[string]bool{
		"Abema": true, "AbemaTV": true, "AmazonPrimeVideo": true, "Bahamut": true,
		"BiliBili": true, "BiliBiliIntl": true, "Dailymotion": true, "Disney": true,
		"HBO": true, "HBOAsia": true, "HBOHK": true, "HBOUSA": true, "Hulu": true,
		"HuluJP": true, "HuluUSA": true, "iQIYI": true, "iQIYIIntl": true,
		"Netflix": true, "Niconico": true, "PrimeVideo": true, "TikTok": true,
		"Twitch": true, "Vimeo": true, "YouTube": true,
	}

	switch {
	case ai[name]:
		return "ai"
	case social[name]:
		return "social"
	case adult[name]:
		return "adult"
	case music[name]:
		return "music"
	case game[name]:
		return "game"
	case video[name]:
		return "video"
	default:
		return "other"
	}
}

func buildRuleApp(name string) RuleSetApp {
	return RuleSetApp{
		Name:     name,
		Path:     name + "/" + name + ".list",
		Category: ruleAppCategory(name),
	}
}

func (s *RuleSetService) Apps(force bool) ([]RuleSetApp, error) {
	ruleCatalogMu.Lock()
	defer ruleCatalogMu.Unlock()

	if !force && len(ruleCatalogMemory) > 0 && time.Since(ruleCatalogLoadedAt) < 6*time.Hour {
		return append([]RuleSetApp(nil), ruleCatalogMemory...), nil
	}
	if len(ruleCatalogMemory) == 0 {
		if apps, updated := loadRuleCatalogDisk(); len(apps) > 0 {
			ruleCatalogMemory = apps
			ruleCatalogLoadedAt = updated
			if !force && time.Since(updated) < 6*time.Hour {
				return append([]RuleSetApp(nil), apps...), nil
			}
		}
	}

	data, err := fetchURLLimited(blackmatrixCatalogURL, 4<<20)
	if err != nil {
		if len(ruleCatalogMemory) > 0 {
			return append([]RuleSetApp(nil), ruleCatalogMemory...), nil
		}
		return nil, fmt.Errorf("获取 blackmatrix7 应用目录失败: %w", err)
	}
	var items []githubContentItem
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, err
	}
	apps := make([]RuleSetApp, 0, len(items))
	for _, item := range items {
		if item.Type != "dir" {
			continue
		}
		name := strings.TrimSpace(item.Name)
		if name == "" || !ruleAppNameRe.MatchString(name) {
			continue
		}
		apps = append(apps, buildRuleApp(name))
	}
	sort.Slice(apps, func(i, j int) bool {
		return strings.ToLower(apps[i].Name) < strings.ToLower(apps[j].Name)
	})
	if len(apps) == 0 {
		return nil, errors.New("blackmatrix7 应用目录为空")
	}
	ruleCatalogMemory = apps
	ruleCatalogLoadedAt = time.Now()
	saveRuleCatalogDisk(apps)
	return append([]RuleSetApp(nil), apps...), nil
}

func rawRuleURL(rel string) (string, error) {
	rel = strings.TrimSpace(rel)
	if !ruleSetPathRe.MatchString(rel) {
		return "", errors.New("第三方规则路径无效")
	}
	parts := strings.Split(rel, "/")
	if len(parts) != 2 {
		return "", errors.New("第三方规则路径层级无效")
	}
	return blackmatrixRawBase + url.PathEscape(parts[0]) + "/" + url.PathEscape(parts[1]), nil
}

func addUnique(values *[]string, seen map[string]struct{}, value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	if _, ok := seen[value]; ok {
		return false
	}
	seen[value] = struct{}{}
	*values = append(*values, value)
	return true
}

func parseBlackmatrixList(name, rel string, data []byte) *RuleSetImportResult {
	result := &RuleSetImportResult{
		Name:        name,
		Path:        rel,
		Domains:     []string{},
		IPs:         []string{},
		TypeCounts:  map[string]int{},
		SkippedType: map[string]int{},
	}
	domainSeen := map[string]struct{}{}
	ipSeen := map[string]struct{}{}

	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(strings.TrimPrefix(raw, "\ufeff"))
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#") {
			if strings.HasPrefix(line, "# UPDATED:") {
				result.Updated = strings.TrimSpace(strings.TrimPrefix(line, "# UPDATED:"))
			}
			continue
		}
		parts := strings.Split(line, ",")
		if len(parts) < 2 {
			result.Skipped++
			result.SkippedType["INVALID"]++
			continue
		}
		kind := strings.ToUpper(strings.TrimSpace(parts[0]))
		value := strings.TrimSpace(parts[1])
		if value == "" {
			result.Skipped++
			result.SkippedType[kind]++
			continue
		}
		result.TypeCounts[kind]++

		imported := false
		switch kind {
		case "DOMAIN":
			imported = addUnique(&result.Domains, domainSeen, "full:"+value)
		case "DOMAIN-SUFFIX":
			imported = addUnique(&result.Domains, domainSeen, "domain:"+value)
		case "DOMAIN-KEYWORD":
			imported = addUnique(&result.Domains, domainSeen, "keyword:"+value)
		case "DOMAIN-REGEX":
			imported = addUnique(&result.Domains, domainSeen, "regexp:"+value)
		case "IP-CIDR", "IP-CIDR6":
			imported = addUnique(&result.IPs, ipSeen, value)
		case "GEOIP":
			imported = addUnique(&result.IPs, ipSeen, "geoip:"+strings.ToLower(value))
		case "GEOSITE":
			imported = addUnique(&result.Domains, domainSeen, "geosite:"+value)
		default:
			result.Skipped++
			result.SkippedType[kind]++
		}
		if imported {
			result.Imported++
		}
	}
	return result
}

func (s *RuleSetService) Resolve(path string) (*RuleSetImportResult, error) {
	rawURL, err := rawRuleURL(path)
	if err != nil {
		return nil, err
	}
	data, err := fetchURLLimited(rawURL, 8<<20)
	if err != nil {
		return nil, fmt.Errorf("下载规则失败: %w", err)
	}
	name := strings.Split(path, "/")[0]
	result := parseBlackmatrixList(name, path, data)
	if result.Imported == 0 {
		return result, errors.New("该规则文件没有可转换为 Xray 路由的条目")
	}
	return result, nil
}

func (s *RuleSetService) ResolveURL(rawURL string) (*RuleSetImportResult, error) {
	u, err := validatePublicRuleURL(rawURL)
	if err != nil {
		return nil, err
	}
	data, err := fetchPublicURLLimited(u.String(), 8<<20)
	if err != nil {
		return nil, fmt.Errorf("下载自定义 List 失败: %w", err)
	}
	name := strings.TrimSpace(filepath.Base(u.Path))
	name = strings.TrimSuffix(name, filepath.Ext(name))
	if name == "" || name == "." || name == "/" {
		name = "CustomList"
	}
	result := parseBlackmatrixList(name, u.String(), data)
	if result.Imported == 0 {
		return result, errors.New("该自定义 List 没有可转换为 Xray 路由的条目")
	}
	return result, nil
}

func (s *RuleSetService) ResolveURLs(urls []string) (*RuleSetBatchResult, error) {
	normalized, err := normalizeCustomRuleURLs(urls, "")
	if err != nil {
		return nil, err
	}

	result := &RuleSetBatchResult{
		Domains:    []string{},
		IPs:        []string{},
		Failed:     []string{},
		TypeCounts: map[string]int{},
	}
	domainSeen := map[string]struct{}{}
	ipSeen := map[string]struct{}{}

	type itemResult struct {
		rawURL string
		res    *RuleSetImportResult
		err    error
	}
	jobs := make(chan string)
	out := make(chan itemResult, len(normalized))
	workerCount := 6
	if len(normalized) < workerCount {
		workerCount = len(normalized)
	}
	var wg sync.WaitGroup
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for rawURL := range jobs {
				res, err := s.ResolveURL(rawURL)
				out <- itemResult{rawURL: rawURL, res: res, err: err}
			}
		}()
	}
	go func() {
		for _, rawURL := range normalized {
			jobs <- rawURL
		}
		close(jobs)
		wg.Wait()
		close(out)
	}()

	for item := range out {
		if item.err != nil || item.res == nil {
			result.Failed = append(result.Failed, item.rawURL)
			continue
		}
		result.Files++
		result.Skipped += item.res.Skipped
		for kind, count := range item.res.TypeCounts {
			result.TypeCounts[kind] += count
		}
		for _, domain := range item.res.Domains {
			if addUnique(&result.Domains, domainSeen, domain) {
				result.Imported++
			}
		}
		for _, ip := range item.res.IPs {
			if addUnique(&result.IPs, ipSeen, ip) {
				result.Imported++
			}
		}
	}
	sort.Strings(result.Failed)
	if result.Imported == 0 {
		return result, errors.New("自定义标签下的 List 没有可导入的 Xray 路由条目")
	}
	return result, nil
}

func (s *RuleSetService) ResolveMany(paths []string) (*RuleSetBatchResult, error) {

	if len(paths) == 0 {
		return nil, errors.New("至少选择一个应用规则")
	}
	if len(paths) > 40 {
		return nil, errors.New("一次最多合并 40 个应用规则")
	}

	result := &RuleSetBatchResult{
		Domains:    []string{},
		IPs:        []string{},
		Failed:     []string{},
		TypeCounts: map[string]int{},
	}
	domainSeen := map[string]struct{}{}
	ipSeen := map[string]struct{}{}

	type itemResult struct {
		path string
		res  *RuleSetImportResult
		err  error
	}
	jobs := make(chan string)
	out := make(chan itemResult, len(paths))
	workerCount := 6
	if len(paths) < workerCount {
		workerCount = len(paths)
	}
	var wg sync.WaitGroup
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for path := range jobs {
				res, err := s.Resolve(path)
				out <- itemResult{path: path, res: res, err: err}
			}
		}()
	}
	go func() {
		for _, path := range paths {
			jobs <- path
		}
		close(jobs)
		wg.Wait()
		close(out)
	}()

	for item := range out {
		if item.err != nil || item.res == nil {
			result.Failed = append(result.Failed, item.path)
			continue
		}
		result.Files++
		result.Skipped += item.res.Skipped
		for kind, count := range item.res.TypeCounts {
			result.TypeCounts[kind] += count
		}
		for _, domain := range item.res.Domains {
			if addUnique(&result.Domains, domainSeen, domain) {
				result.Imported++
			}
		}
		for _, ip := range item.res.IPs {
			if addUnique(&result.IPs, ipSeen, ip) {
				result.Imported++
			}
		}
	}
	sort.Strings(result.Failed)
	if result.Imported == 0 {
		return result, errors.New("所选应用没有可导入的 Xray 路由条目")
	}
	return result, nil
}
