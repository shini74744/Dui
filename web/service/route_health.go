package service

import (
	"fmt"
	"strings"
)

func validateRouteHealth(raw map[string]any, eligible map[string]bool) error {
	scopes, _ := raw["strategyObservatory"].(map[string]any)
	routing, _ := raw["routing"].(map[string]any)
	selected := map[string]bool{}
	if scope, exists := scopes["outbound"]; exists {
		probe, ok := scope.(map[string]any)
		if !ok {
			return fmt.Errorf("单独出口探测配置无效")
		}
		tags, _ := probe["subjectSelector"].([]any)
		for _, v := range tags {
			tag, ok := v.(string)
			if !ok || !eligible[tag] {
				return fmt.Errorf("单独出口探测引用了无效出口")
			}
			selected[tag] = true
		}
	}
	rules, _ := routing["rules"].([]any)
	for i, item := range rules {
		r, _ := item.(map[string]any)
		fallback, _ := r["fallbackTag"].(string)
		if fallback == "" {
			continue
		}
		primary, _ := r["outboundTag"].(string)
		balancer, _ := r["balancerTag"].(string)
		if balancer != "" || primary == fallback || !eligible[primary] || !eligible[fallback] || !selected[primary] || !selected[fallback] {
			return fmt.Errorf("第 %d 条路由的 Fallback 无效：主备出口必须不同、存在且已启用单独出口探测", i+1)
		}
	}
	balancers, _ := routing["balancers"].([]any)
	for _, item := range balancers {
		b, _ := item.(map[string]any)
		strategy, _ := b["strategy"].(map[string]any)
		if strategy["type"] != "priority" {
			continue
		}
		if fallback, _ := b["fallbackTag"].(string); fallback != "" {
			return fmt.Errorf("分级优先请通过级别设置备用出口")
		}
		settings, _ := strategy["settings"].(map[string]any)
		levels, _ := settings["levels"].([]any)
		if len(levels) < 1 || len(levels) > 10 {
			return fmt.Errorf("分级优先需要 1 至 10 个级别")
		}
		for i, item := range levels {
			l, _ := item.(map[string]any)
			s, _ := l["strategy"].(map[string]any)
			name, _ := s["type"].(string)
			if name == "" {
				name = "random"
			}
			if name != "random" && name != "roundRobin" && name != "leastLoad" && name != "leastPing" {
				return fmt.Errorf("第 %d 级策略无效", i+1)
			}
			tags, _ := l["selector"].([]any)
			if len(tags) == 0 {
				return fmt.Errorf("第 %d 级需要至少一个出口", i+1)
			}
			probe, ok := scopes[name].(map[string]any)
			if !ok {
				return fmt.Errorf("第 %d 级需要配置对应策略探测", i+1)
			}
			prefixes, _ := probe["subjectSelector"].([]any)
			for _, v := range tags {
				tag, ok := v.(string)
				if !ok || !eligible[tag] {
					return fmt.Errorf("第 %d 级包含无效出口", i+1)
				}
				covered := false
				for _, p := range prefixes {
					if prefix, ok := p.(string); ok && strings.HasPrefix(tag, prefix) {
						covered = true
						break
					}
				}
				if !covered {
					return fmt.Errorf("第 %d 级出口未被对应策略探测覆盖", i+1)
				}
			}
		}
	}
	return nil
}
