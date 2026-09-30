package service

import (
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/netip"
	"os"
	"strings"

	"github.com/google/uuid"
	"x-ui/database/model"
	"x-ui/internal/amneziawg"
	"x-ui/internal/mtproto"
	"x-ui/internal/tuic"
	wg "x-ui/internal/util/wireguard"
)

// Each helper has explicit capabilities. Refuse unsupported controls instead of
// accepting a form value that the runtime would silently ignore.
func validateHelperInbound(ib *model.Inbound) error {
	if !ib.Protocol.IsHelperProtocol() {
		return nil
	}
	ib.Listen = strings.Trim(ib.Listen, "[]")
	if ib.Port < 1 || ib.Port > 65535 {
		return fmt.Errorf("端口必须为 1–65535")
	}
	if ib.Listen != "" && net.ParseIP(strings.Trim(ib.Listen, "[]")) == nil {
		return fmt.Errorf("监听地址必须是 IP")
	}
	if ib.DeviceLimit != 0 {
		return fmt.Errorf("此协议暂不支持设备数限制")
	}
	var settings struct {
		Clients          []model.Client `json:"clients"`
		RouteThroughXray bool           `json:"routeThroughXray"`
	}
	if err := json.Unmarshal([]byte(ib.Settings), &settings); err != nil || settings.Clients == nil {
		return fmt.Errorf("无效的用户配置")
	}
	emails, ids := map[string]bool{}, map[string]bool{}
	for _, c := range settings.Clients {
		if c.Email == "" || emails[c.Email] {
			return fmt.Errorf("用户邮箱不能为空或重复")
		}
		emails[c.Email] = true
		if err := c.ValidateUserRate(); err != nil {
			return err
		}
		if c.UserRateLevel() != 0 || c.LimitIP != 0 {
			return fmt.Errorf("此协议暂不支持用户限速或来源 IP 数限制")
		}
		id := c.ID
		switch ib.Protocol {
		case model.TUIC:
			if c.UUID != "" {
				id = c.UUID
			}
			if _, err := uuid.Parse(id); err != nil || c.Password == "" {
				return fmt.Errorf("TUIC 需要有效 UUID 和密码")
			}
			if c.TotalGB != 0 {
				return fmt.Errorf("TUIC 仅统计入站总流量，暂不支持单用户流量配额")
			}
			id = strings.ToLower(id)
		case model.MTProto:
			id = c.Secret
			b, err := hex.DecodeString(id)
			if err != nil || len(b) < 19 || b[0] != 0xee {
				return fmt.Errorf("MTProto 需要有效的 ee FakeTLS secret")
			}
			domain := string(b[17:])
			if strings.ContainsAny(domain, "\x00\r\n /:") || !strings.Contains(domain, ".") {
				return fmt.Errorf("MTProto FakeTLS secret 中域名无效")
			}
			if !model.ValidMtprotoAdTag(c.AdTag) {
				return fmt.Errorf("MTProto 广告标签必须是 32 位十六进制")
			}
		case model.AmneziaWG:
			id = c.PublicKey
			if _, err := wg.KeyToHex(id); err != nil || id == "" {
				return fmt.Errorf("AmneziaWG 用户公钥无效")
			}
			if c.PreSharedKey != "" {
				if _, err := wg.KeyToHex(c.PreSharedKey); err != nil {
					return fmt.Errorf("AmneziaWG 预共享密钥无效")
				}
			}
			if c.PrivateKey != "" {
				pub, err := wg.PublicKeyFromPrivate(c.PrivateKey)
				if err != nil || pub != id {
					return fmt.Errorf("AmneziaWG 用户私钥与公钥不匹配")
				}
			}
			if c.ForwardedPorts != "" {
				return fmt.Errorf("当前版本尚未开放 AmneziaWG 入站端口转发")
			}
		}
		if id == "" || ids[id] {
			return fmt.Errorf("用户认证信息不能为空或重复")
		}
		ids[id] = true
	}
	switch ib.Protocol {
	case model.TUIC:
		inst, ok := tuic.InstanceFromInbound(ib)
		if !ok {
			return fmt.Errorf("TUIC 配置无效")
		}
		if _, err := tls.LoadX509KeyPair(inst.Certificate, inst.PrivateKey); err != nil {
			return fmt.Errorf("TUIC 证书与私钥不可读取或不匹配")
		}
		if inst.CongestionControl != "bbr" && inst.CongestionControl != "cubic" && inst.CongestionControl != "new_reno" {
			return fmt.Errorf("TUIC 拥塞控制无效")
		}
		if inst.AuthenticationTimeout < 1 || inst.AuthenticationTimeout > 120 || inst.MaxIdleTime < 1 || inst.MaxIdleTime > 3600 {
			return fmt.Errorf("TUIC 超时时间超出范围")
		}
		if ib.Enable {
			if _, err := os.Stat(tuic.GetBinaryPath()); err != nil {
				return fmt.Errorf("缺少 DUI TUIC 配套组件，请先安装对应平台组件")
			}
		}
	case model.MTProto:
		if settings.RouteThroughXray {
			return fmt.Errorf("当前 MTProto 使用直连出口，尚未开放 Xray 路由")
		}
		if ib.Enable {
			if _, err := os.Stat(mtproto.GetBinaryPath()); err != nil {
				return fmt.Errorf("缺少 DUI MTProto 配套组件，请先安装对应平台组件")
			}
		}
	case model.AmneziaWG:
		var cfg amneziawg.InboundSettings
		if err := json.Unmarshal([]byte(ib.Settings), &cfg); err != nil || cfg.Server == nil {
			return fmt.Errorf("AmneziaWG 缺少服务端配置")
		}
		server := cfg.Server
		pub, err := wg.PublicKeyFromPrivate(server.PrivateKey)
		if err != nil || pub != server.PublicKey {
			return fmt.Errorf("AmneziaWG 服务端密钥不匹配")
		}
		if err := amneziawg.ValidateSubnetIPv4(server.SubnetIP, server.SubnetCIDR); err != nil {
			return err
		}
		if err := amneziawg.ValidateServerObfuscation(server.Obfuscation()); err != nil {
			return err
		}
		if server.IPv6Enabled {
			return fmt.Errorf("当前版本尚未开放 AmneziaWG 主机 IPv6 地址绑定")
		}
		if server.MTU != 0 && (server.MTU < 576 || server.MTU > 9000) {
			return fmt.Errorf("AmneziaWG MTU 无效")
		}
		network, _ := netip.ParsePrefix(fmt.Sprintf("%s/%d", server.SubnetIP, server.SubnetCIDR))
		network = network.Masked()
		used := map[netip.Addr]bool{}
		for _, c := range cfg.Clients {
			if len(c.AllowedIPs) != 1 {
				return fmt.Errorf("每个 AmneziaWG 用户需要一个独立的 /32 隧道地址")
			}
			ip, err := netip.ParsePrefix(c.AllowedIPs[0])
			if err != nil || ip.Bits() != 32 || !network.Contains(ip.Addr()) || ip.Addr() == network.Addr() || ip.Addr() == network.Addr().Next() || used[ip.Addr()] {
				return fmt.Errorf("AmneziaWG 用户地址不合法或重复")
			}
			used[ip.Addr()] = true
		}
	}
	return nil
}
