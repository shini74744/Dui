package service

import (
	"fmt"
	"x-ui/database"
	"x-ui/database/model"
	"x-ui/xray"
)

func dbClientStats(inbound *model.Inbound) error {
	return database.GetDB().Where("inbound_id = ?", inbound.Id).Find(&inbound.ClientStats).Error
}

func (s *InboundService) validateNativeInbound(inbound *model.Inbound) error {
	if !inbound.Protocol.IsNativeExtension() {
		return nil
	}
	if err := inbound.ValidateNativeProtocol(); err != nil {
		return err
	}
	if inbound.Protocol == model.TUN {
		if inbound.Enable {
			if err := checkTunPermissions(); err != nil {
				return err
			}
		}
		var count int64
		if err := database.GetDB().Model(&model.Inbound{}).Where("tag = ? AND id <> ?", inbound.Tag, inbound.Id).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return fmt.Errorf("TUN 接口名已被其他入站使用")
		}
	}
	cfg := &xray.Config{InboundConfigs: []xray.InboundConfig{*inbound.GenXrayInboundConfig()}}
	return xray.ValidateConfig(cfg)
}
