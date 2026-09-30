package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
	"x-ui/database/model"
	"x-ui/internal/amneziawg"
	"x-ui/internal/amneziawgnet"
	"x-ui/internal/mtproto"
	"x-ui/internal/tuic"
	"x-ui/util/json_util"
	"x-ui/xray"
)

var helperMu sync.Mutex

func awgDesired(inst amneziawg.Instance) amneziawgnet.Desired {
	o := inst.Obfuscation
	return amneziawgnet.Desired{Instance: inst, Options: amneziawgnet.DeviceOptions{
		HeaderProtectionKey: o.HeaderProtectionKey, ContentPaddingAddition: o.ContentPaddingAddition,
		RekeyAfterTime: o.RekeyAfterTime, RekeyTimeout: o.RekeyTimeout, RejectAfterTime: o.RejectAfterTime,
		KeepaliveTimeout: o.KeepaliveTimeout, MaxHandshakeAttempts: o.MaxHandshakeAttempts,
		RandomTrailers: o.RandomTrailers, DisableCookies: o.DisableCookies,
	}}
}

func helperActiveInbound(ib *model.Inbound) *model.Inbound {
	out := *ib
	var cfg map[string]json.RawMessage
	if json.Unmarshal([]byte(out.Settings), &cfg) != nil {
		return &out
	}
	var clients []model.Client
	_ = json.Unmarshal(cfg["clients"], &clients)
	active := make([]model.Client, 0, len(clients))
	now := time.Now().UnixMilli()
	for _, c := range clients {
		if !c.Enable || (c.ExpiryTime > 0 && c.ExpiryTime <= now) {
			continue
		}
		blocked := false
		for _, st := range out.ClientStats {
			if st.Email == c.Email && (!st.Enable || (st.Total > 0 && st.Up+st.Down >= st.Total) || (st.ExpiryTime > 0 && st.ExpiryTime <= now)) {
				blocked = true
				break
			}
		}
		if !blocked {
			active = append(active, c)
		}
	}
	cfg["clients"], _ = json.Marshal(active)
	data, _ := json.Marshal(cfg)
	out.Settings = string(data)
	if out.Total > 0 && out.Up+out.Down >= out.Total || out.ExpiryTime > 0 && out.ExpiryTime <= now {
		out.Enable = false
	}
	return &out
}

func helperBridge(ib *model.Inbound) (*xray.InboundConfig, error) {
	if ib.Protocol != model.AmneziaWG || !ib.Enable {
		return nil, nil
	}
	inst, ok := amneziawg.InstanceFromInbound(helperActiveInbound(ib))
	if !ok {
		return nil, nil
	}
	emails := make([]string, 0, len(inst.Peers))
	for _, peer := range inst.Peers {
		emails = append(emails, peer.Email)
	}
	settings, err := amneziawgnet.SocksInboundSettings(emails, amneziawgnet.SocksPassword())
	if err != nil {
		return nil, err
	}
	return &xray.InboundConfig{Tag: ib.Tag, Listen: json_util.RawMessage(`"127.0.0.1"`), Port: amneziawgnet.SOCKSPortForInbound(ib.Id), Protocol: "socks", Settings: json_util.RawMessage(settings), Sniffing: json_util.RawMessage(`{"enabled":true,"destOverride":["http","tls","quic"],"routeOnly":true}`)}, nil
}

func stopHelper(id int) {
	tuic.GetManager().Remove(id)
	mtproto.GetManager().Remove(id)
	amneziawgnet.GetManager().Remove(id)
}

func applyHelper(ib *model.Inbound, replaceBridge bool) error {
	active := helperActiveInbound(ib)
	if !active.Enable {
		stopHelper(ib.Id)
		if replaceBridge && ib.Protocol == model.AmneziaWG {
			removeHelperBridge(ib.Id, ib.Tag)
		}
		return nil
	}
	switch ib.Protocol {
	case model.TUIC:
		inst, ok := tuic.InstanceFromInbound(active)
		if !ok {
			return fmt.Errorf("TUIC 配置无法解析")
		}
		return tuic.GetManager().Ensure(inst)
	case model.MTProto:
		inst, ok := mtproto.InstanceFromInbound(active)
		if !ok {
			mtproto.GetManager().Remove(ib.Id)
			return nil
		}
		return mtproto.GetManager().Ensure(inst)
	case model.AmneziaWG:
		if replaceBridge {
			if err := syncHelperBridge(active); err != nil {
				return err
			}
		}
		inst, ok := amneziawg.InstanceFromInbound(active)
		if !ok {
			amneziawgnet.GetManager().Remove(ib.Id)
			return nil
		}
		return amneziawgnet.GetManager().Ensure(awgDesired(inst))
	}
	return nil
}

func stopAllHelpers() {
	helperBridges = map[int]*xray.InboundConfig{}
	tuic.GetManager().StopAll()
	mtproto.GetManager().StopAll()
	amneziawgnet.GetManager().StopAll()
}

// Called after a successful core start and periodically. DB failures leave the
// existing runtime intact; intentional stop shuts down every owned helper.
func (s *XrayService) SyncHelperProtocols() error {
	helperMu.Lock()
	defer helperMu.Unlock()
	if isManuallyStopped.Load() || !s.IsXrayRunning() {
		stopAllHelpers()
		return nil
	}
	inbounds, err := s.inboundService.GetAllInbounds()
	if err != nil {
		return err
	}
	var ti []tuic.Instance
	var mi []mtproto.Instance
	var ai []amneziawgnet.Desired
	var bridgeErrors []error
	wantedBridges := map[int]bool{}
	for _, ib := range inbounds {
		if !ib.Protocol.IsHelperProtocol() {
			continue
		}
		active := helperActiveInbound(ib)
		if ib.Protocol == model.AmneziaWG {
			wantedBridges[ib.Id] = true
			if err := syncHelperBridge(active); err != nil {
				bridgeErrors = append(bridgeErrors, fmt.Errorf("AmneziaWG inbound %d: %w", ib.Id, err))
				continue
			}
		}
		if !active.Enable {
			continue
		}
		switch ib.Protocol {
		case model.TUIC:
			if v, ok := tuic.InstanceFromInbound(active); ok {
				ti = append(ti, v)
			}
		case model.MTProto:
			if v, ok := mtproto.InstanceFromInbound(active); ok {
				mi = append(mi, v)
			}
		case model.AmneziaWG:
			if v, ok := amneziawg.InstanceFromInbound(active); ok {
				ai = append(ai, awgDesired(v))
			}
		}
	}
	for id, bridge := range helperBridges {
		if !wantedBridges[id] {
			removeHelperBridge(id, bridge.Tag)
		}
	}
	tuic.GetManager().Reconcile(ti)
	mtproto.GetManager().Reconcile(mi)
	amneziawgnet.GetManager().Reconcile(ai)
	return errors.Join(bridgeErrors...)
}

func collectHelperTraffic() ([]*xray.Traffic, []*xray.ClientTraffic) {
	var in []*xray.Traffic
	var clients []*xray.ClientTraffic
	for _, d := range tuic.GetManager().CollectTraffic() {
		in = append(in, &xray.Traffic{Tag: d.Tag, IsInbound: true, Up: d.Up, Down: d.Down})
	}
	online, _ := tuic.GetManager().GetActiveClients(30 * time.Second)
	for _, email := range online {
		clients = append(clients, &xray.ClientTraffic{Email: email, Active: true})
	}
	deltas, _ := mtproto.GetManager().CollectTraffic()
	for _, d := range deltas {
		in = append(in, &xray.Traffic{Tag: d.Tag, IsInbound: true, Up: d.Up, Down: d.Down})
		clients = append(clients, &xray.ClientTraffic{Email: d.Email, Up: d.Up, Down: d.Down})
	}
	return in, clients
}
