package model

import (
	"encoding/json"
	"fmt"
	"net/netip"
	"regexp"
)

func (p Protocol) IsNativeExtension() bool { return p == Hysteria || p == Mixed || p == TUN }

// ValidateNativeProtocol validates panel-specific constraints before any saved or
// running inbound is replaced. The installed core performs its own schema check.
func (i *Inbound) ValidateNativeProtocol() error {
	if !i.Protocol.IsNativeExtension() {
		return nil
	}
	if i.Protocol != TUN && (i.Port < 1 || i.Port > 65535) {
		return fmt.Errorf("端口必须在 1–65535 之间")
	}
	switch i.Protocol {
	case Hysteria:
		var settings struct {
			Version int      `json:"version"`
			Clients []Client `json:"clients"`
		}
		if json.Unmarshal([]byte(i.Settings), &settings) != nil || (settings.Version != 2 || settings.Clients == nil) {
			return fmt.Errorf("Hysteria 仅支持版本 2")
		}
		auths := map[string]bool{}
		for _, c := range settings.Clients {
			if c.Auth == "" || c.Email == "" {
				return fmt.Errorf("Hysteria 用户必须填写认证密码和 Email 标识")
			}
			if auths[c.Auth] {
				return fmt.Errorf("同一 Hysteria 入站不能使用重复认证密码")
			}
			auths[c.Auth] = true
			if err := c.ValidateUserRate(); err != nil {
				return err
			}
		}
		var stream struct {
			Network  string `json:"network"`
			Security string `json:"security"`
			Hysteria struct {
				Version int    `json:"version"`
				Auth    string `json:"auth"`
				Idle    int    `json:"udpIdleTimeout"`
			} `json:"hysteriaSettings"`
			TLS struct {
				ALPN         []string          `json:"alpn"`
				Certificates []json.RawMessage `json:"certificates"`
			} `json:"tlsSettings"`
		}
		if json.Unmarshal([]byte(i.StreamSettings), &stream) != nil || stream.Network != "hysteria" || stream.Security != "tls" || stream.Hysteria.Version != 2 {
			return fmt.Errorf("Hysteria 2 必须使用 hysteria 传输和 TLS")
		}
		if stream.Hysteria.Auth != "" {
			return fmt.Errorf("请在用户中设置 Hysteria 认证密码；禁止使用绕过用户配额的全局认证")
		}
		if len(stream.TLS.Certificates) == 0 {
			return fmt.Errorf("Hysteria 2 需要 TLS 证书")
		}
		if stream.Hysteria.Idle != 0 && (stream.Hysteria.Idle < 2 || stream.Hysteria.Idle > 600) {
			return fmt.Errorf("UDP 空闲超时必须为 2–600 秒")
		}
		if len(stream.TLS.ALPN) > 0 && (len(stream.TLS.ALPN) != 1 || stream.TLS.ALPN[0] != "h3") {
			return fmt.Errorf("Hysteria 2 的 ALPN 必须为 h3")
		}
	case Mixed:
		var settings struct {
			Auth     string `json:"auth"`
			Accounts []struct {
				User string `json:"user"`
				Pass string `json:"pass"`
			} `json:"accounts"`
		}
		if json.Unmarshal([]byte(i.Settings), &settings) != nil {
			return fmt.Errorf("Mixed 配置无效")
		}
		switch settings.Auth {
		case "password":
			if len(settings.Accounts) == 0 {
				return fmt.Errorf("Mixed 密码认证需要至少一个账号")
			}
			users := map[string]bool{}
			for _, a := range settings.Accounts {
				if a.User == "" || a.Pass == "" || len(a.User) > 255 || len(a.Pass) > 255 || users[a.User] {
					return fmt.Errorf("Mixed 账号需要唯一用户名和非空密码，各不超过 255 字节")
				}
				users[a.User] = true
			}
		case "noauth":
			address, err := netip.ParseAddr(i.Listen)
			if err != nil || !address.IsLoopback() {
				return fmt.Errorf("Mixed 无认证模式只能监听本机回环地址")
			}
		default:
			return fmt.Errorf("Mixed 认证方式必须为 password 或 noauth")
		}
		if i.DeviceLimit != 0 {
			return fmt.Errorf("Mixed 仅支持入站流量统计，不支持用户设备限制")
		}
	case TUN:
		var settings struct {
			Name      string   `json:"name"`
			MTU       int      `json:"mtu"`
			Gateway   []string `json:"gateway"`
			DNS       []string `json:"dns"`
			Routes    []string `json:"autoSystemRoutingTable"`
			Interface string   `json:"autoOutboundsInterface"`
		}
		if json.Unmarshal([]byte(i.Settings), &settings) != nil {
			return fmt.Errorf("TUN 配置无效")
		}
		if !regexp.MustCompile(`^[a-zA-Z0-9_-]{1,15}$`).MatchString(settings.Name) {
			return fmt.Errorf("TUN 接口名须为 1–15 位字母、数字、下划线或短横线")
		}
		if settings.MTU < 576 || settings.MTU > 9000 {
			return fmt.Errorf("TUN MTU 必须为 576–9000")
		}
		if len(settings.Gateway) == 0 {
			return fmt.Errorf("TUN 需要至少一个接口地址 CIDR")
		}
		for _, cidr := range append(settings.Gateway, settings.Routes...) {
			if _, err := netip.ParsePrefix(cidr); err != nil {
				return fmt.Errorf("TUN 接口地址和路由必须采用有效 CIDR 格式")
			}
		}
		for _, ip := range settings.DNS {
			if _, err := netip.ParseAddr(ip); err != nil {
				return fmt.Errorf("TUN DNS 必须填写 IP 地址")
			}
		}
		if len(settings.Routes) > 0 && settings.Interface == "" {
			return fmt.Errorf("启用 TUN 系统路由时必须指定出口接口或 auto，以防代理回环")
		}
		if i.DeviceLimit != 0 {
			return fmt.Errorf("TUN 不支持用户设备限制")
		}
		// TUN has an interface rather than a socket; different TUNs may all use port 0.
		i.Listen = ""
		i.Port = 0
		i.Tag = "inbound-tun-" + settings.Name
	}
	return nil
}
