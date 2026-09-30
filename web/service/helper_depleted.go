package service

import (
	"encoding/json"
	"x-ui/database"
	"x-ui/database/model"
	"x-ui/xray"
)

// Reuse helper CRUD so deleting the final user also stops the owned helper.
// Do this before the legacy native transaction to avoid nested SQLite writers.
func (s *InboundService) deleteDepletedHelperClients(id int) error {
	query := database.GetDB().Model(&model.Inbound{}).
		Where("protocol IN ?", []model.Protocol{model.TUIC, model.MTProto, model.AmneziaWG})
	if id >= 0 {
		query = query.Where("id = ?", id)
	}
	var inbounds []*model.Inbound
	if err := query.Find(&inbounds).Error; err != nil {
		return err
	}
	for _, ib := range inbounds {
		var expired []xray.ClientTraffic
		if err := database.GetDB().Where("inbound_id = ? AND reset = 0 AND enable = ?", ib.Id, false).Find(&expired).Error; err != nil {
			return err
		}
		if len(expired) == 0 {
			continue
		}
		remove := map[string]bool{}
		for _, c := range expired {
			remove[c.Email] = true
		}
		clients, err := s.GetClients(ib)
		if err != nil {
			return err
		}
		keep := make([]model.Client, 0, len(clients))
		for _, c := range clients {
			if !remove[c.Email] {
				keep = append(keep, c)
			}
		}
		if len(keep) == 0 {
			if _, err := s.deleteHelperInbound(ib); err != nil {
				return err
			}
			continue
		}
		var cfg map[string]json.RawMessage
		if err := json.Unmarshal([]byte(ib.Settings), &cfg); err != nil {
			return err
		}
		cfg["clients"], _ = json.Marshal(keep)
		raw, _ := json.Marshal(cfg)
		ib.Settings = string(raw)
		if _, _, err := s.saveHelperInbound(ib, false); err != nil {
			return err
		}
	}
	return nil
}
