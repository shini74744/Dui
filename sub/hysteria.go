package sub

import (
	"encoding/json"
	"net"
	"net/url"
	"strconv"
	"strings"
	"x-ui/database/model"
	"x-ui/util/json_util"
)

func (s *SubService) genHysteriaLink(inbound *model.Inbound, email string) string {
	var settings struct {
		Clients []model.Client `json:"clients"`
	}
	var stream struct {
		TLS struct {
			ServerName string `json:"serverName"`
			Settings   struct {
				AllowInsecure bool `json:"allowInsecure"`
			} `json:"settings"`
		} `json:"tlsSettings"`
	}
	if json.Unmarshal([]byte(inbound.Settings), &settings) != nil || json.Unmarshal([]byte(inbound.StreamSettings), &stream) != nil {
		return ""
	}
	host := s.address
	if inbound.Listen != "" && inbound.Listen != "0.0.0.0" && inbound.Listen != "::" && inbound.Listen != "::0" {
		host = inbound.Listen
	}
	for _, client := range settings.Clients {
		if client.Email != email {
			continue
		}
		q := url.Values{}
		if stream.TLS.ServerName != "" {
			q.Set("sni", stream.TLS.ServerName)
		}
		if stream.TLS.Settings.AllowInsecure {
			q.Set("insecure", "1")
		}
		u := url.URL{Scheme: "hysteria2", User: url.User(client.Auth), Host: net.JoinHostPort(strings.Trim(host, "[]"), strconv.Itoa(inbound.Port)), Path: "/", RawQuery: q.Encode(), Fragment: s.genRemark(inbound, email, "")}
		return u.String()
	}
	return ""
}

func (s *SubJsonService) genHysteria(inbound *model.Inbound, stream map[string]any, client model.Client) json_util.RawMessage {
	// Only client-side transport settings are exported; no server certificates,
	// masquerade paths, or other clients' authentication values enter subscriptions.
	stream["network"] = "hysteria"
	stream["security"] = "tls"
	stream["hysteriaSettings"] = map[string]any{"version": 2, "auth": client.Auth}
	if tls, ok := stream["tlsSettings"].(map[string]any); ok {
		delete(tls, "fingerprint")
	}
	data, _ := json.Marshal(map[string]any{"tag": "proxy", "protocol": "hysteria", "settings": map[string]any{"version": 2, "address": inbound.Listen, "port": inbound.Port}, "streamSettings": stream})
	return data
}
