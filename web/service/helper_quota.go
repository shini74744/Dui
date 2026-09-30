package service

import (
	"fmt"
	"x-ui/database"
	"x-ui/database/model"
	"x-ui/internal/mtproto"
	"x-ui/xray"
)

func (s *InboundService) resetHelperClientTraffic(ib *model.Inbound, email string) error {
	result := database.GetDB().Model(&xray.ClientTraffic{}).
		Where("inbound_id = ? AND email = ?", ib.Id, email).
		Updates(map[string]any{"enable": true, "up": 0, "down": 0})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("未找到入站用户")
	}
	return s.resetHelperQuotas(ib.Id, []string{email})
}

// Called only after the DB transaction commits. A reset must reach the helper
// quota counter too; otherwise its own limiter can keep a renewed user blocked.
// nil emails means all users in the selected inbound(s).
func (s *InboundService) resetHelperQuotas(id int, emails []string) error {
	helperMu.Lock()
	defer helperMu.Unlock()
	if p == nil || !p.IsRunning() || isManuallyStopped.Load() {
		return nil
	}
	db := database.GetDB().Model(&model.Inbound{}).Preload("ClientStats").
		Where("protocol IN ?", []model.Protocol{model.TUIC, model.MTProto, model.AmneziaWG})
	if id >= 0 {
		db = db.Where("id = ?", id)
	}
	var inbounds []*model.Inbound
	if err := db.Find(&inbounds).Error; err != nil {
		return err
	}
	selected := map[string]bool{}
	for _, email := range emails {
		selected[email] = true
	}
	for _, ib := range inbounds {
		clients, err := s.GetClients(ib)
		if err != nil {
			return err
		}
		var hit []string
		for _, c := range clients {
			if emails == nil || selected[c.Email] {
				hit = append(hit, c.Email)
			}
		}
		if len(hit) == 0 {
			continue
		}
		if err := applyHelper(ib, true); err != nil {
			return err
		}
		if ib.Protocol == model.MTProto {
			for _, email := range hit {
				mtproto.GetManager().ResetQuota(email)
			}
		}
	}
	return nil
}
