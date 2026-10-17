package service

import (
	"encoding/json"
	"fmt"
	"gorm.io/gorm"
	"regexp"
	"slices"
	"strings"
	"x-ui/database"
	"x-ui/database/model"
)

// A remark is panel metadata. Update only its column, including for helper
// protocols, so stale editor snapshots cannot replace live settings or counters.
func (s *InboundService) UpdateInboundRemark(id, userID int, remark string) error {
	return updateInboundRemark(database.GetDB(), id, userID, remark)
}

func updateInboundRemark(db *gorm.DB, id, userID int, remark string) error {
	result := db.Model(&model.Inbound{}).Where("id = ? AND user_id = ?", id, userID).UpdateColumn("remark", remark)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

type ClientRouteReference struct {
	Index       int    `json:"index"`
	RuleTag     string `json:"ruleTag,omitempty"`
	OutboundTag string `json:"outboundTag,omitempty"`
	BalancerTag string `json:"balancerTag,omitempty"`
	FallbackTag string `json:"fallbackTag,omitempty"`
	Regexp      bool   `json:"regexp"`
}
type ClientRouteReferences struct {
	InboundID int                    `json:"inboundId"`
	Email     string                 `json:"email"`
	Rules     []ClientRouteReference `json:"rules"`
}

func (s *InboundService) GetClientRouteReferences(userID int) ([]ClientRouteReferences, error) {
	raw, err := (&SettingService{}).GetXrayConfigTemplate()
	if err != nil {
		return nil, err
	}
	var inbounds []model.Inbound
	err = database.GetDB().Select("id", "tag", "settings").Where("user_id = ?", userID).Find(&inbounds).Error
	if err != nil {
		return nil, err
	}
	return clientRouteReferences(raw, inbounds)
}

func clientRouteReferences(raw string, inbounds []model.Inbound) ([]ClientRouteReferences, error) {
	type rule struct {
		User        []string `json:"user"`
		InboundTag  []string `json:"inboundTag"`
		RuleTag     string   `json:"ruleTag"`
		OutboundTag string   `json:"outboundTag"`
		BalancerTag string   `json:"balancerTag"`
		FallbackTag string   `json:"fallbackTag"`
	}
	var template struct {
		Routing struct {
			Rules []rule `json:"rules"`
		} `json:"routing"`
	}
	if err := json.Unmarshal([]byte(raw), &template); err != nil {
		return nil, fmt.Errorf("read routing references: %w", err)
	}
	type matcher struct {
		exact    map[string]bool
		patterns []*regexp.Regexp
	}
	matchers := make([]matcher, len(template.Routing.Rules))
	for i, r := range template.Routing.Rules {
		matchers[i].exact = make(map[string]bool)
		for _, user := range r.User {
			// Match the core's case-sensitive exact / Go regexp semantics.
			if len(user) > 7 && strings.HasPrefix(user, "regexp:") {
				if re, err := regexp.Compile(user[7:]); err == nil {
					matchers[i].patterns = append(matchers[i].patterns, re)
				}
			} else if user != "" {
				matchers[i].exact[user] = true
			}
		}
	}
	result := make([]ClientRouteReferences, 0)
	for _, inbound := range inbounds {
		if strings.TrimSpace(inbound.Settings) == "" {
			continue
		}
		var settings struct {
			Clients []struct {
				Email string `json:"email"`
			} `json:"clients"`
		}
		if err := json.Unmarshal([]byte(inbound.Settings), &settings); err != nil {
			return nil, fmt.Errorf("read inbound %d users: %w", inbound.Id, err)
		}
		for _, client := range settings.Clients {
			if client.Email == "" {
				continue
			}
			refs := ClientRouteReferences{InboundID: inbound.Id, Email: client.Email, Rules: make([]ClientRouteReference, 0)}
			for i, r := range template.Routing.Rules {
				if len(r.InboundTag) > 0 && !slices.Contains(r.InboundTag, inbound.Tag) {
					continue
				}
				exact, pattern := matchers[i].exact[client.Email], false
				if !exact {
					for _, re := range matchers[i].patterns {
						if re.MatchString(client.Email) {
							pattern = true
							break
						}
					}
				}
				if !exact && !pattern {
					continue
				}
				refs.Rules = append(refs.Rules, ClientRouteReference{Index: i + 1, RuleTag: r.RuleTag, OutboundTag: r.OutboundTag, BalancerTag: r.BalancerTag, FallbackTag: r.FallbackTag, Regexp: pattern})
			}
			if len(refs.Rules) > 0 {
				result = append(result, refs)
			}
		}
	}
	return result, nil
}
