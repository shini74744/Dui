package model

import (
	"encoding/json"
	"fmt"
	"math"
)

// Hot-added and hot-edited inbounds need the same rate levels as a full rebuild.
func (i *Inbound) xraySettingsWithRates() string {
	switch i.Protocol {
	case VMESS, VLESS, Trojan, Shadowsocks:
	default:
		return i.Settings
	}
	var settings map[string]json.RawMessage
	if json.Unmarshal([]byte(i.Settings), &settings) != nil {
		return i.Settings
	}
	var clients []map[string]json.RawMessage
	if json.Unmarshal(settings["clients"], &clients) != nil || clients == nil {
		return i.Settings
	}
	for _, raw := range clients {
		if raw == nil {
			return i.Settings
		}
		data, err := json.Marshal(raw)
		if err != nil {
			return i.Settings
		}
		var client Client
		if json.Unmarshal(data, &client) != nil {
			return i.Settings
		}
		level, _ := json.Marshal(client.UserRateLevel())
		raw["level"] = level
	}
	settings["clients"], _ = json.Marshal(clients)
	data, err := json.Marshal(settings)
	if err != nil {
		return i.Settings
	}
	return string(data)
}

const DUIRateLevelFlag uint32 = 1 << 31
const MaxUserRateMbps = 10000

// XrayAPIUser preserves authentication, flow and user rate when restoring a user.
func (c Client) XrayAPIUser(cipher string) map[string]any {
	return map[string]any{"email": c.Email, "id": c.ID, "security": c.Security,
		"flow": c.Flow, "password": c.Password, "cipher": cipher, "level": c.UserRateLevel()}
}

func (c Client) UserRateMbps() float64 {
	if c.SpeedLimitMbps != nil {
		return *c.SpeedLimitMbps
	}
	return float64(c.SpeedLimit) * 1024 * 8 / 1000000
}
func (c Client) ValidateUserRate() error {
	n := c.UserRateMbps()
	if math.IsNaN(n) || math.IsInf(n, 0) || n < 0 || n > MaxUserRateMbps {
		return fmt.Errorf("user speed must be between 0 and %d Mbps", MaxUserRateMbps)
	}
	if n > 0 && c.Email == "" {
		return fmt.Errorf("user speed limit requires an email identifier")
	}
	return nil
}
func (c Client) UserRateLevel() uint32 {
	n := c.UserRateMbps()
	if n <= 0 || c.ValidateUserRate() != nil {
		return 0
	}
	b := uint32(math.Round(n * 1000000 / 8))
	if b == 0 {
		b = 1
	}
	return DUIRateLevelFlag | b
}
