package service

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"strings"

	"x-ui/util/common"
	"x-ui/xray"
)

type XraySettingService struct {
	SettingService
}

func validXrayRedirectTarget(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	_, port, err := net.SplitHostPort(value)
	if err != nil {
		return false
	}
	n, err := strconv.Atoi(port)
	return err == nil && n >= 1 && n <= 65535
}

func validXrayIPRule(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	lower := strings.ToLower(value)
	if strings.HasPrefix(lower, "geoip:") {
		return true
	}
	if strings.HasPrefix(lower, "ext:") {
		return strings.Contains(lower, "geoip")
	}
	if net.ParseIP(value) != nil {
		return true
	}
	if _, _, err := net.ParseCIDR(value); err == nil {
		return true
	}
	return false
}

func (s *XraySettingService) SaveXraySetting(newXraySettings string) error {
	if err := s.CheckXrayConfig(newXraySettings); err != nil {
		return err
	}
	return s.SettingService.saveSetting("xrayTemplateConfig", newXraySettings)
}

func (s *XraySettingService) CheckXrayConfig(XrayTemplateConfig string) error {
	xrayConfig := &xray.Config{}
	if err := json.Unmarshal([]byte(XrayTemplateConfig), xrayConfig); err != nil {
		return common.NewError("xray template config invalid:", err)
	}

	var raw map[string]any
	if err := json.Unmarshal([]byte(XrayTemplateConfig), &raw); err != nil {
		return common.NewError("xray template config invalid:", err)
	}

	outboundTags := map[string]bool{}
	if outbounds, ok := raw["outbounds"].([]any); ok {
		for i, item := range outbounds {
			obj, ok := item.(map[string]any)
			if !ok {
				continue
			}
			tag, _ := obj["tag"].(string)
			tag = strings.TrimSpace(tag)
			if tag == "" {
				continue
			}
			if outboundTags[tag] {
				return fmt.Errorf("Xray 出口标签重复：%s（第 %d 个 outbound）", tag, i+1)
			}
			if protocol, _ := obj["protocol"].(string); protocol == "freedom" {
				if settings, ok := obj["settings"].(map[string]any); ok {
					if redirect, _ := settings["redirect"].(string); strings.TrimSpace(redirect) != "" && !validXrayRedirectTarget(redirect) {
						return fmt.Errorf("第 %d 个 freedom 出口 redirect 格式无效：%s；必须为 地址:端口，例如 ipleak.net:443", i+1, redirect)
					}
				}
			}
			outboundTags[tag] = true
		}
	}

	routing, _ := raw["routing"].(map[string]any)
	if routing == nil {
		return nil
	}
	balancerTags := map[string]bool{}
	if balancers, ok := routing["balancers"].([]any); ok {
		for _, item := range balancers {
			if obj, ok := item.(map[string]any); ok {
				if tag, _ := obj["tag"].(string); strings.TrimSpace(tag) != "" {
					balancerTags[strings.TrimSpace(tag)] = true
				}
			}
		}
	}

	rules, _ := routing["rules"].([]any)
	matchKeys := []string{"domain", "ip", "port", "sourcePort", "network", "source", "sourceIP", "user", "inboundTag", "protocol", "attrs"}
	for i, item := range rules {
		rule, ok := item.(map[string]any)
		if !ok {
			continue
		}
		outboundTag, _ := rule["outboundTag"].(string)
		balancerTag, _ := rule["balancerTag"].(string)
		outboundTag = strings.TrimSpace(outboundTag)
		balancerTag = strings.TrimSpace(balancerTag)
		if outboundTag == "" && balancerTag == "" {
			return fmt.Errorf("第 %d 条路由规则未设置出站 Tag 或负载均衡 Tag", i+1)
		}
		if outboundTag != "" && balancerTag != "" {
			return fmt.Errorf("第 %d 条路由规则同时设置了 outboundTag 和 balancerTag", i+1)
		}
		if outboundTag != "" && !outboundTags[outboundTag] {
			return fmt.Errorf("第 %d 条路由规则引用了不存在的出口标签：%s", i+1, outboundTag)
		}
		if balancerTag != "" && !balancerTags[balancerTag] {
			return fmt.Errorf("第 %d 条路由规则引用了不存在的负载均衡标签：%s", i+1, balancerTag)
		}

		if ipValues, ok := rule["ip"].([]any); ok {
			for _, rawIP := range ipValues {
				ipValue, ok := rawIP.(string)
				if !ok || !validXrayIPRule(ipValue) {
					return fmt.Errorf("第 %d 条路由规则包含无效 IP/CIDR/GeoIP：%v；域名请填写到 domain 字段", i+1, rawIP)
				}
			}
		}

		hasMatch := false
		for _, key := range matchKeys {
			v, exists := rule[key]
			if !exists || v == nil {
				continue
			}
			switch x := v.(type) {
			case string:
				if strings.TrimSpace(x) != "" {
					hasMatch = true
				}
			case []any:
				if len(x) > 0 {
					hasMatch = true
				}
			case map[string]any:
				if len(x) > 0 {
					hasMatch = true
				}
			default:
				hasMatch = true
			}
			if hasMatch {
				break
			}
		}
		if !hasMatch && (outboundTag != "" || balancerTag != "") && i < len(rules)-1 {
			return fmt.Errorf("第 %d 条路由规则是无条件全匹配规则，后面的 %d 条规则永远不会命中，请将该规则移到最后", i+1, len(rules)-i-1)
		}
	}
	return nil
}
