package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"gorm.io/gorm"
	"x-ui/database/model"
	"x-ui/xray"
)

// Covers ID allocation, validation, persistence and hot updates across protocols.
var inboundMutationMu sync.Mutex

func finishInboundTransaction(tx *gorm.DB, result *error, restore func() error) {
	if *result == nil {
		*result = tx.Commit().Error
	}
	if *result != nil {
		_ = tx.Rollback().Error
		if restore != nil {
			if err := restore(); err != nil {
				*result = errors.Join(*result, fmt.Errorf("恢复原入站失败，请检查核心状态：%w", err))
			}
		}
	}
}

func restoreInboundRuntime(api *xray.XrayAPI, current, previous *model.Inbound) error {
	// A failed AddInbound may have allocated part of the handler before returning.
	_ = api.DelInbound(current.Tag)
	if previous == nil || !previous.Enable {
		return nil
	}
	data, err := json.Marshal(previous.GenXrayInboundConfig())
	if err != nil {
		return err
	}
	return api.AddInbound(data)
}
