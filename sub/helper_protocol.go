package sub

import (
	"encoding/json"
	"net"
	"net/url"
	"strconv"
	"strings"
	"x-ui/database/model"
	"x-ui/internal/tuic"
)

func (s *SubService) genHelperLink(ib *model.Inbound, email string) string {
	var settings struct {
		Clients       []model.Client `json:"clients"`
		AllowInsecure bool           `json:"allowInsecure"`
	}
	if json.Unmarshal([]byte(ib.Settings), &settings) != nil {
		return ""
	}
	host := s.address
	if ib.Listen != "" && ib.Listen != "0.0.0.0" && ib.Listen != "::" && ib.Listen != "::0" {
		host = ib.Listen
	}
	host = strings.Trim(host, "[]")
	for _, c := range settings.Clients {
		if c.Email != email {
			continue
		}
		switch ib.Protocol {
		case model.MTProto:
			q := url.Values{"server": {host}, "port": {strconv.Itoa(ib.Port)}, "secret": {c.Secret}}
			return "tg://proxy?" + q.Encode()
		case model.TUIC:
			inst, ok := tuic.InstanceFromInbound(ib)
			if !ok {
				return ""
			}
			id := c.UUID
			if id == "" {
				id = c.ID
			}
			q := url.Values{"congestion_control": {inst.CongestionControl}, "alpn": {strings.Join(inst.ALPN, ",")}, "udp_relay_mode": {inst.UDPRelayMode}}
			if inst.SNI != "" {
				q.Set("sni", inst.SNI)
			}
			if settings.AllowInsecure {
				q.Set("allow_insecure", "1")
			}
			u := url.URL{Scheme: "tuic", User: url.UserPassword(id, c.Password), Host: net.JoinHostPort(host, strconv.Itoa(ib.Port)), RawQuery: q.Encode(), Fragment: s.genRemark(ib, email, "")}
			return u.String()
		}
	}
	return ""
}
