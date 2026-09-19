package model

import (
	"fmt"

	"x-ui/util/json_util"
	"x-ui/xray"
)

type Protocol string

const (
	VMESS       Protocol = "vmess"
	VLESS       Protocol = "vless"
	Tunnel      Protocol = "tunnel"
	HTTP        Protocol = "http"
	Trojan      Protocol = "trojan"
	Shadowsocks Protocol = "shadowsocks"
	Socks       Protocol = "socks"
	WireGuard   Protocol = "wireguard"
)

type User struct {
	Id       int    `json:"id" gorm:"primaryKey;autoIncrement"`
	Username string `json:"username"`
	Password string `json:"password"`
}

type Inbound struct {
	Id                   int    `json:"id" form:"id" gorm:"primaryKey"`
	UserId               int    `json:"-"`
	Up                   int64  `json:"up" form:"up"`
	Down                 int64  `json:"down" form:"down"`
	Total                int64  `json:"total" form:"total"`
	AllTime              int64  `json:"allTime" form:"allTime" gorm:"default:0"`
	Remark               string `json:"remark" form:"remark"`
	Enable               bool   `json:"enable" form:"enable" gorm:"index:idx_enable_traffic_reset,priority:1"`
	ExpiryTime           int64  `json:"expiryTime" form:"expiryTime"`
	TrafficReset         string `json:"trafficReset" form:"trafficReset" gorm:"default:never;index:idx_enable_traffic_reset,priority:2"`
	LastTrafficResetTime int64  `json:"lastTrafficResetTime" form:"lastTrafficResetTime" gorm:"default:0"`

	// 中文注释: 新增设备限制字段，用于存储每个入站的设备数限制。
	// gorm:"column:device_limit;default:0" 定义了数据库中的字段名和默认值。
	DeviceLimit int `json:"deviceLimit" form:"deviceLimit" gorm:"column:device_limit;default:0"`

	ClientStats []xray.ClientTraffic `gorm:"foreignKey:InboundId;references:Id" json:"clientStats" form:"clientStats"`

	// config part
	Listen         string   `json:"listen" form:"listen"`
	Port           int      `json:"port" form:"port"`
	Protocol       Protocol `json:"protocol" form:"protocol"`
	Settings       string   `json:"settings" form:"settings"`
	StreamSettings string   `json:"streamSettings" form:"streamSettings"`
	Tag            string   `json:"tag" form:"tag" gorm:"unique"`
	Sniffing       string   `json:"sniffing" form:"sniffing"`
}

type OutboundTraffics struct {
	Id    int    `json:"id" form:"id" gorm:"primaryKey;autoIncrement"`
	Tag   string `json:"tag" form:"tag" gorm:"unique"`
	Up    int64  `json:"up" form:"up" gorm:"default:0"`
	Down  int64  `json:"down" form:"down" gorm:"default:0"`
	Total int64  `json:"total" form:"total" gorm:"default:0"`
}

// DailyTrafficEmptyEmailPrefix is an internal key used when an inbound has exactly
// one client but that client intentionally has no Email. The UI converts it back
// to a human-readable label and never exposes this internal key.
const DailyTrafficEmptyEmailPrefix = "__dui_empty_inbound_"

// DailyClientTraffic keeps per-day traffic deltas as Xray reports them.
type DailyClientTraffic struct {
	Id        int    `json:"id" gorm:"primaryKey;autoIncrement"`
	Date      string `json:"date" gorm:"type:varchar(10);uniqueIndex:idx_daily_client,priority:1"`
	Email     string `json:"email" gorm:"uniqueIndex:idx_daily_client,priority:2"`
	InboundId int    `json:"inboundId" gorm:"index"`
	Up        int64  `json:"up" gorm:"default:0"`
	Down      int64  `json:"down" gorm:"default:0"`
}

type ClientNotifyState struct {
	Id              int    `json:"id" gorm:"primaryKey;autoIncrement"`
	InboundId       int    `json:"inboundId" gorm:"uniqueIndex:idx_client_notify,priority:1"`
	Email           string `json:"email" gorm:"uniqueIndex:idx_client_notify,priority:2"`
	ExpiryNotified  bool   `json:"expiryNotified" gorm:"default:false"`
	TrafficNotified bool   `json:"trafficNotified" gorm:"default:false"`
	UpdatedAt       int64  `json:"updatedAt" gorm:"default:0"`
}

type ClientTelegramBot struct {
	Id              int    `json:"id" gorm:"primaryKey;autoIncrement"`
	InboundId       int    `json:"inboundId" gorm:"uniqueIndex:idx_client_bot,priority:1"`
	Email           string `json:"email" gorm:"uniqueIndex:idx_client_bot,priority:2"`
	Enabled         bool   `json:"enabled" gorm:"default:false"`
	Mode            string `json:"mode" gorm:"type:varchar(16);default:panel"`
	BotToken        string `json:"botToken"`
	ChatId          int64  `json:"chatId"`
	ExpiryNotified  bool   `json:"expiryNotified" gorm:"default:false"`
	TrafficNotified bool   `json:"trafficNotified" gorm:"default:false"`
	UpdatedAt       int64  `json:"updatedAt" gorm:"default:0"`
}

type TelegramNotificationHistory struct {
	Id          int    `json:"id" gorm:"primaryKey;autoIncrement"`
	CreatedAt   int64  `json:"createdAt" gorm:"index"`
	BindingId   int    `json:"bindingId" gorm:"index"`
	InboundId   int    `json:"inboundId" gorm:"index"`
	Protocol    string `json:"protocol"`
	Remark      string `json:"remark"`
	Port        int    `json:"port"`
	Email       string `json:"email"`
	Mode        string `json:"mode"`
	BotUsername string `json:"botUsername"`
	ChatId      int64  `json:"chatId"`
	Subject     string `json:"subject"`
	Content     string `json:"content" gorm:"type:text"`
	Success     bool   `json:"success"`
	Error       string `json:"error" gorm:"type:text"`
}

type ManagedBlacklistEntry struct {
	Id        int    `json:"id" gorm:"primaryKey;autoIncrement"`
	Kind      string `json:"kind" gorm:"type:varchar(20);index:idx_managed_blacklist,priority:1"`
	Scope     string `json:"scope" gorm:"type:varchar(20);index:idx_managed_blacklist,priority:2"`
	InboundId int    `json:"inboundId" gorm:"index:idx_managed_blacklist,priority:3"`
	Email     string `json:"email" gorm:"index:idx_managed_blacklist,priority:4"`
	Value     string `json:"value" gorm:"index:idx_managed_blacklist,priority:5"`
	CreatedAt int64  `json:"createdAt"`
}

type InboundClientIps struct {
	Id          int    `json:"id" gorm:"primaryKey;autoIncrement"`
	ClientEmail string `json:"clientEmail" form:"clientEmail" gorm:"unique"`
	Ips         string `json:"ips" form:"ips"`
}

type HistoryOfSeeders struct {
	Id         int    `json:"id" gorm:"primaryKey;autoIncrement"`
	SeederName string `json:"seederName"`
}

func (i *Inbound) GenXrayInboundConfig() *xray.InboundConfig {
	listen := i.Listen
	if listen != "" {
		listen = fmt.Sprintf("\"%v\"", listen)
	}
	return &xray.InboundConfig{
		Listen:         json_util.RawMessage(listen),
		Port:           i.Port,
		Protocol:       string(i.Protocol),
		Settings:       json_util.RawMessage(i.Settings),
		StreamSettings: json_util.RawMessage(i.StreamSettings),
		Tag:            i.Tag,
		Sniffing:       json_util.RawMessage(i.Sniffing),
	}
}

type Setting struct {
	Id    int    `json:"id" form:"id" gorm:"primaryKey;autoIncrement"`
	Key   string `json:"key" form:"key"`
	Value string `json:"value" form:"value"`
}

type Client struct {
	ID       string `json:"id"`
	Security string `json:"security"`
	Password string `json:"password"`

	// 中文注释: 新增“限速”字段，单位 KB/s，0 表示不限速。
	SpeedLimit int `json:"speedLimit" form:"speedLimit"`

	Flow       string `json:"flow"`
	Email      string `json:"email"`
	LimitIP    int    `json:"limitIp"`
	TotalGB    int64  `json:"totalGB" form:"totalGB"`
	ExpiryTime int64  `json:"expiryTime" form:"expiryTime"`
	Enable     bool   `json:"enable" form:"enable"`
	TgID       int64  `json:"tgId" form:"tgId"`
	TgNotify   bool   `json:"tgNotify" form:"tgNotify"`
	SubID      string `json:"subId" form:"subId"`
	Comment    string `json:"comment" form:"comment"`
	Reset      int    `json:"reset" form:"reset"`
	CreatedAt  int64  `json:"created_at,omitempty"`
	UpdatedAt  int64  `json:"updated_at,omitempty"`
}

type VLESSSettings struct {
	Clients    []Client `json:"clients"`
	Decryption string   `json:"decryption"`
	Encryption string   `json:"encryption"`
	Fallbacks  []any    `json:"fallbacks"`
}
