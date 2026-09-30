package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"x-ui/database/model"
	"x-ui/xray"
)

// Protected by helperMu. The cache records only successfully applied bridges.
var helperBridges = map[int]*xray.InboundConfig{}

func removeHelperBridge(id int, tag string) {
	previous := helperBridges[id]
	delete(helperBridges, id)
	if p == nil || !p.IsRunning() {
		return
	}
	var api xray.XrayAPI
	if api.Init(p.GetAPIPort()) != nil {
		return
	}
	defer api.Close()
	if previous != nil && previous.Tag != tag {
		_ = api.DelInbound(previous.Tag)
	}
	if tag != "" {
		_ = api.DelInbound(tag)
	}
}

func syncHelperBridge(ib *model.Inbound) error {
	bridge, err := helperBridge(helperActiveInbound(ib))
	if err != nil {
		return err
	}
	if bridge == nil {
		removeHelperBridge(ib.Id, ib.Tag)
		return nil
	}
	data, err := json.Marshal(bridge)
	if err != nil {
		return err
	}
	if old := helperBridges[ib.Id]; old != nil {
		previous, _ := json.Marshal(old)
		if bytes.Equal(previous, data) {
			return nil
		}
	}
	if p == nil || !p.IsRunning() {
		return fmt.Errorf("Xray 未运行")
	}
	removeHelperBridge(ib.Id, ib.Tag)
	var api xray.XrayAPI
	if err := api.Init(p.GetAPIPort()); err != nil {
		return err
	}
	defer api.Close()
	if err := api.AddInbound(data); err != nil {
		return err
	}
	helperBridges[ib.Id] = bridge
	return nil
}
