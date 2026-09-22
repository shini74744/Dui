package service

import (
	"encoding/json"
	"fmt"
)

type XrayConnectionPolicy struct {
	Handshake    int `json:"handshake" form:"handshake"`
	ConnIdle     int `json:"connIdle" form:"connIdle"`
	UplinkOnly   int `json:"uplinkOnly" form:"uplinkOnly"`
	DownlinkOnly int `json:"downlinkOnly" form:"downlinkOnly"`
}

func defaultXrayConnectionPolicy() XrayConnectionPolicy {
	return XrayConnectionPolicy{
		Handshake:    4,
		ConnIdle:     300,
		UplinkOnly:   0,
		DownlinkOnly: 0,
	}
}

func xrayPolicyInt(m map[string]any, key string, fallback int) int {
	v, ok := m[key]
	if !ok {
		return fallback
	}
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	case json.Number:
		if i, err := n.Int64(); err == nil {
			return int(i)
		}
	}
	return fallback
}
func validateXrayConnectionPolicy(v XrayConnectionPolicy) error {
	if v.Handshake < 1 || v.Handshake > 120 {
		return fmt.Errorf("handshake 必须在 1-120 秒之间")
	}
	if v.ConnIdle < 30 || v.ConnIdle > 86400 {
		return fmt.Errorf("connIdle 必须在 30-86400 秒之间")
	}
	if v.UplinkOnly < 0 || v.UplinkOnly > 600 {
		return fmt.Errorf("uplinkOnly 必须在 0-600 秒之间")
	}
	if v.DownlinkOnly < 0 || v.DownlinkOnly > 600 {
		return fmt.Errorf("downlinkOnly 必须在 0-600 秒之间")
	}
	return nil
}

func xrayPolicyLevel0(root map[string]any, create bool) map[string]any {
	policy, _ := root["policy"].(map[string]any)
	if policy == nil {
		if !create {
			return nil
		}
		policy = map[string]any{}
		root["policy"] = policy
	}
	levels, _ := policy["levels"].(map[string]any)
	if levels == nil {
		if !create {
			return nil
		}
		levels = map[string]any{}
		policy["levels"] = levels
	}
	level0, _ := levels["0"].(map[string]any)
	if level0 == nil && create {
		level0 = map[string]any{}
		levels["0"] = level0
	}
	return level0
}
func GetXrayConnectionPolicy(template string) (XrayConnectionPolicy, error) {
	var root map[string]any
	if err := json.Unmarshal([]byte(template), &root); err != nil {
		return XrayConnectionPolicy{}, fmt.Errorf("解析 Xray 模板失败: %w", err)
	}
	result := defaultXrayConnectionPolicy()
	level0 := xrayPolicyLevel0(root, false)
	if level0 == nil {
		return result, nil
	}
	result.Handshake = xrayPolicyInt(level0, "handshake", result.Handshake)
	result.ConnIdle = xrayPolicyInt(level0, "connIdle", result.ConnIdle)
	result.UplinkOnly = xrayPolicyInt(level0, "uplinkOnly", result.UplinkOnly)
	result.DownlinkOnly = xrayPolicyInt(level0, "downlinkOnly", result.DownlinkOnly)
	return result, nil
}

func UpdateXrayConnectionPolicyTemplate(template string, v XrayConnectionPolicy) (string, error) {
	if err := validateXrayConnectionPolicy(v); err != nil {
		return "", err
	}
	var root map[string]any
	if err := json.Unmarshal([]byte(template), &root); err != nil {
		return "", fmt.Errorf("解析 Xray 模板失败: %w", err)
	}
	level0 := xrayPolicyLevel0(root, true)
	level0["handshake"] = v.Handshake
	level0["connIdle"] = v.ConnIdle
	level0["uplinkOnly"] = v.UplinkOnly
	level0["downlinkOnly"] = v.DownlinkOnly
	level0["statsUserUplink"] = true
	level0["statsUserDownlink"] = true
	level0["statsUserOnline"] = true
	data, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return "", fmt.Errorf("序列化 Xray 模板失败: %w", err)
	}
	return string(data), nil
}
