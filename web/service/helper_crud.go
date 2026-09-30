package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"gorm.io/gorm"
	"time"
	"x-ui/database"
	"x-ui/database/model"
	"x-ui/internal/amneziawgnet"
	"x-ui/xray"
)

func helperClientID(p model.Protocol, c model.Client) string {
	switch p {
	case model.MTProto:
		return c.Secret
	case model.AmneziaWG:
		if c.ID != "" {
			return c.ID
		}
		return c.PublicKey
	case model.TUIC:
		if c.UUID != "" {
			return c.UUID
		}
	}
	return c.ID
}

func (s *InboundService) saveHelperInbound(ib *model.Inbound, create bool) (*model.Inbound, bool, error) {
	helperMu.Lock()
	defer helperMu.Unlock()
	if err := validateHelperInbound(ib); err != nil {
		return ib, false, err
	}
	all, err := s.GetAllInbounds()
	if err != nil {
		return ib, false, err
	}
	var previous *model.Inbound
	ids := map[int]bool{}
	for _, v := range all {
		ids[v.Id] = true
		if !create && v.Id == ib.Id {
			previous = v
		}
	}
	if create {
		ib.Id = 1
		for ids[ib.Id] {
			ib.Id++
		}
	} else {
		if previous == nil {
			return ib, false, gorm.ErrRecordNotFound
		}
		if previous.Protocol != ib.Protocol {
			return ib, false, fmt.Errorf("请新建入站切换协议类型")
		}
		ib.UserId = previous.UserId
		ib.Up = previous.Up
		ib.Down = previous.Down
		ib.AllTime = previous.AllTime
		ib.LastTrafficResetTime = previous.LastTrafficResetTime
	}
	if exists, err := s.checkPortExist(ib.Listen, ib.Port, ib.Id); err != nil || exists {
		if err == nil {
			err = fmt.Errorf("端口已使用")
		}
		return ib, false, err
	}
	// Reserve all configured AWG bridge ports, including disabled interfaces.
	for _, other := range append(all, ib) {
		if other.Protocol != model.AmneziaWG {
			continue
		}
		bridge := amneziawgnet.SOCKSPortForInbound(other.Id)
		if bridge == ib.Port {
			return ib, false, fmt.Errorf("端口与 AmneziaWG 内部转发端口冲突")
		}
		if ib.Protocol == model.AmneziaWG && other.Id != ib.Id && bridge == amneziawgnet.SOCKSPortForInbound(ib.Id) {
			return ib, false, fmt.Errorf("AmneziaWG 内部转发端口冲突")
		}
	}
	if ib.Protocol == model.AmneziaWG {
		for _, other := range all {
			if other.Id != ib.Id && other.Port == amneziawgnet.SOCKSPortForInbound(ib.Id) {
				return ib, false, fmt.Errorf("AmneziaWG 内部端口已被其他入站使用")
			}
		}
	}
	clients, err := s.GetClients(ib)
	if err != nil {
		return ib, false, err
	}
	for _, other := range all {
		if other.Id == ib.Id {
			continue
		}
		ocs, err := s.GetClients(other)
		if err != nil {
			return ib, false, err
		}
		for _, c := range clients {
			for _, oc := range ocs {
				if c.Email == oc.Email {
					return ib, false, fmt.Errorf("用户邮箱重复")
				}
			}
		}
	}
	ib.Tag = fmt.Sprintf("inbound-%d", ib.Port)
	if ib.Listen != "" && ib.Listen != "0.0.0.0" && ib.Listen != "::" {
		ib.Tag = fmt.Sprintf("inbound-%s:%d", ib.Listen, ib.Port)
	}
	oldByID := map[string]model.Client{}
	oldByEmail := map[string]model.Client{}
	if previous != nil {
		ocs, _ := s.GetClients(previous)
		for _, c := range ocs {
			oldByID[helperClientID(ib.Protocol, c)] = c
			oldByEmail[c.Email] = c
		}
	}
	now := time.Now().UnixMilli()
	for n := range clients {
		c := &clients[n]
		old, ok := oldByID[helperClientID(ib.Protocol, *c)]
		if !ok {
			old = oldByEmail[c.Email]
		}
		if old.CreatedAt > 0 {
			c.CreatedAt = old.CreatedAt
		} else if c.CreatedAt == 0 {
			c.CreatedAt = now
		}
		c.UpdatedAt = now
	}
	var cfg map[string]json.RawMessage
	_ = json.Unmarshal([]byte(ib.Settings), &cfg)
	cfg["clients"], _ = json.Marshal(clients)
	data, _ := json.Marshal(cfg)
	ib.Settings = string(data)
	tx := database.GetDB().Begin()
	if tx.Error != nil {
		return ib, false, tx.Error
	}
	defer tx.Rollback()
	if err = tx.Omit("ClientStats").Save(ib).Error; err != nil {
		return ib, false, err
	}
	keep := map[string]bool{}
	for _, c := range clients {
		keep[c.Email] = true
		if old, ok := oldByEmail[c.Email]; ok {
			// Parent edits must not undo an exhausted quota; the active filter checks
			// both persisted usage and the new limit before serving the user.
			_ = old
			err = s.UpdateClientStat(tx, c.Email, &c)
		} else if old, ok := oldByID[helperClientID(ib.Protocol, c)]; ok {
			err = s.UpdateClientStat(tx, old.Email, &c)
			if err == nil {
				err = s.UpdateClientIPs(tx, old.Email, c.Email)
			}
		} else {
			err = s.AddClientStat(tx, ib.Id, &c)
		}
		if err != nil {
			return ib, false, err
		}
	}
	for email := range oldByEmail {
		if !keep[email] {
			if err = tx.Where("inbound_id = ? AND email = ?", ib.Id, email).Delete(&xray.ClientTraffic{}).Error; err != nil {
				return ib, false, err
			}
		}
	}
	if err = tx.Where("inbound_id = ?", ib.Id).Find(&ib.ClientStats).Error; err != nil {
		return ib, false, err
	}
	live := p != nil && p.IsRunning() && !isManuallyStopped.Load()
	restore := func() error {
		stopHelper(ib.Id)
		if ib.Protocol == model.AmneziaWG {
			removeHelperBridge(ib.Id, ib.Tag)
		}
		if previous != nil {
			return applyHelper(previous, true)
		}
		return nil
	}
	if live {
		if err = applyHelper(ib, true); err != nil {
			return ib, false, errors.Join(fmt.Errorf("运行时应用失败，未保存配置：%w", err), restore())
		}
	}
	if err = tx.Commit().Error; err != nil {
		if live {
			err = errors.Join(err, restore())
		}
		return ib, false, err
	}
	return ib, false, nil
}

func (s *InboundService) deleteHelperInbound(ib *model.Inbound) (bool, error) {
	helperMu.Lock()
	defer helperMu.Unlock()
	err := database.GetDB().Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("inbound_id = ?", ib.Id).Delete(&model.ClientTelegramBot{}).Error; err != nil {
			return err
		}
		if err := tx.Where("inbound_id = ?", ib.Id).Delete(&xray.ClientTraffic{}).Error; err != nil {
			return err
		}
		return tx.Delete(ib).Error
	})
	if err != nil {
		return false, err
	}
	stopHelper(ib.Id)
	if ib.Protocol == model.AmneziaWG {
		removeHelperBridge(ib.Id, ib.Tag)
	}
	return false, nil
}

func (s *InboundService) changeHelperClients(ib *model.Inbound, data *model.Inbound, id, action string) (bool, error) {
	var cfg map[string]json.RawMessage
	if err := json.Unmarshal([]byte(ib.Settings), &cfg); err != nil {
		return false, err
	}
	clients, err := s.GetClients(ib)
	if err != nil {
		return false, err
	}
	var changed []model.Client
	if data != nil {
		changed, err = s.GetClients(data)
		if err != nil {
			return false, err
		}
	}
	switch action {
	case "add":
		clients = append(clients, changed...)
	case "update", "delete":
		found := -1
		for i, c := range clients {
			if helperClientID(ib.Protocol, c) == id {
				found = i
				break
			}
		}
		if found < 0 {
			return false, fmt.Errorf("未找到用户")
		}
		if action == "delete" {
			clients = append(clients[:found], clients[found+1:]...)
		} else {
			if len(changed) != 1 {
				return false, fmt.Errorf("更新请求必须包含一个用户")
			}
			clients[found] = changed[0]
		}
	}
	cfg["clients"], _ = json.Marshal(clients)
	raw, _ := json.Marshal(cfg)
	ib.Settings = string(raw)
	_, restart, err := s.saveHelperInbound(ib, false)
	return restart, err
}
