package xray

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os/exec"
	"regexp"
	"time"

	"x-ui/logger"
	"x-ui/util/common"

	"github.com/xtls/xray-core/app/proxyman/command"
	statsService "github.com/xtls/xray-core/app/stats/command"
	"github.com/xtls/xray-core/common/protocol"
	"github.com/xtls/xray-core/common/serial"
	"github.com/xtls/xray-core/proxy/shadowsocks"
	"github.com/xtls/xray-core/proxy/trojan"
	"github.com/xtls/xray-core/proxy/vless"
	"github.com/xtls/xray-core/proxy/vmess"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/encoding/protowire"
)

type XrayAPI struct {
	HandlerServiceClient *command.HandlerServiceClient
	StatsServiceClient   *statsService.StatsServiceClient
	grpcClient           *grpc.ClientConn
	isConnected          bool
	apiPort              int
}

func (x *XrayAPI) Init(apiPort int) error {
	if apiPort <= 0 || apiPort > math.MaxUint16 {
		return fmt.Errorf("invalid Xray API port: %d", apiPort)
	}

	addr := fmt.Sprintf("127.0.0.1:%d", apiPort)
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("failed to connect to Xray API: %w", err)
	}

	x.grpcClient = conn
	x.isConnected = true
	x.apiPort = apiPort

	hsClient := command.NewHandlerServiceClient(conn)
	ssClient := statsService.NewStatsServiceClient(conn)

	x.HandlerServiceClient = &hsClient
	x.StatsServiceClient = &ssClient

	return nil
}

func (x *XrayAPI) Close() {
	if x.grpcClient != nil {
		x.grpcClient.Close()
	}
	x.HandlerServiceClient = nil
	x.StatsServiceClient = nil
	x.isConnected = false
	x.apiPort = 0
}

func (x *XrayAPI) AddInbound(inbound []byte) error {
	if !x.isConnected || x.apiPort <= 0 || x.apiPort > math.MaxUint16 {
		return fmt.Errorf("Xray API is not initialized")
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(inbound, &object); err != nil || object == nil {
		return fmt.Errorf("invalid inbound JSON object")
	}
	payload, err := json.Marshal(map[string]any{"inbounds": []json.RawMessage{inbound}})
	if err != nil {
		return err
	}
	// Use the installed core's parser so new fields survive hot additions.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, GetBinaryPath(), "api", "adi", fmt.Sprintf("--server=127.0.0.1:%d", x.apiPort), "--timeout=5")
	cmd.Stdin = bytes.NewReader(payload)
	// Do not log CLI output: config errors may contain private keys.
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("Xray add inbound: %w", ctx.Err())
		}
		return fmt.Errorf("Xray add inbound: %w", err)
	}
	return nil
}

func (x *XrayAPI) DelInbound(tag string) error {
	client := *x.HandlerServiceClient
	_, err := client.RemoveInbound(context.Background(), &command.RemoveInboundRequest{
		Tag: tag,
	})
	return err
}

func (x *XrayAPI) AddUser(Protocol string, inboundTag string, user map[string]any) error {
	if x.HandlerServiceClient == nil {
		return fmt.Errorf("Xray API is not initialized")
	}
	id, _ := user["id"].(string)
	flow, _ := user["flow"].(string)
	password, _ := user["password"].(string)
	email, _ := user["email"].(string)
	if (Protocol == "vmess" || Protocol == "vless") && id == "" {
		return fmt.Errorf("user ID is required")
	}
	if (Protocol == "trojan" || Protocol == "shadowsocks") && password == "" {
		return fmt.Errorf("user password is required")
	}
	var account *serial.TypedMessage
	switch Protocol {
	case "vmess":
		account = serial.ToTypedMessage(&vmess.Account{
			Id: id,
		})
	case "vless":
		account = serial.ToTypedMessage(&vless.Account{
			Id:   id,
			Flow: flow,
		})
	case "trojan":
		account = serial.ToTypedMessage(&trojan.Account{
			Password: password,
		})
	case "shadowsocks":
		var ssCipherType shadowsocks.CipherType
		cipher, _ := user["cipher"].(string)
		ss2022 := false
		switch cipher {
		case "aes-128-gcm":
			ssCipherType = shadowsocks.CipherType_AES_128_GCM
		case "aes-256-gcm":
			ssCipherType = shadowsocks.CipherType_AES_256_GCM
		case "chacha20-poly1305", "chacha20-ietf-poly1305":
			ssCipherType = shadowsocks.CipherType_CHACHA20_POLY1305
		case "xchacha20-poly1305", "xchacha20-ietf-poly1305":
			ssCipherType = shadowsocks.CipherType_XCHACHA20_POLY1305
		case "none", "plain":
			ssCipherType = shadowsocks.CipherType_NONE
		case "2022-blake3-aes-128-gcm", "2022-blake3-aes-256-gcm", "2022-blake3-chacha20-poly1305":
			ss2022 = true
		default:
			return fmt.Errorf("missing or unsupported Shadowsocks cipher")
		}

		if !ss2022 {
			account = serial.ToTypedMessage(&shadowsocks.Account{
				Password:   password,
				CipherType: ssCipherType,
			})
		} else {
			// v26.9.9 uses Account.key (field 1), not ServerConfig.key (field 2).
			account = &serial.TypedMessage{Type: "xray.proxy.shadowsocks_2022.Account", Value: protowire.AppendString(protowire.AppendTag(nil, 1, protowire.BytesType), password)}
		}
	default:
		return fmt.Errorf("unsupported user API protocol")
	}

	// 〔中文注释〕: (修改点) 创建一个有5秒超时限制的上下文（Context）。
	// 这确保了如果 Xray-Core API 因为某些原因没有及时响应，
	// 这个操作不会永久阻塞，而是在5秒后自动失败，从而提高程序的健壮性。
	// 这与 RemoveUser 函数中的超时设置保持了一致。
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client := *x.HandlerServiceClient

	_, err := client.AlterInbound(ctx, &command.AlterInboundRequest{ // 〔中文注释〕: (修改点) 使用上面创建的带超时的 ctx
		Tag: inboundTag,
		Operation: serial.ToTypedMessage(&command.AddUserOperation{
			User: &protocol.User{
				Email:   email,
				Level:   apiUserLevel(user["level"]),
				Account: account,
			},
		}),
	})

	// 〔中文注释〕: (修改点) 增加更详细的错误日志，方便排查问题。
	if err != nil {
		emailStr, _ := user["email"].(string)
		return fmt.Errorf("failed to add user '%s' to inbound '%s': %w", emailStr, inboundTag, err)
	}

	return nil
}

func (x *XrayAPI) RemoveUser(inboundTag, email string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	op := &command.RemoveUserOperation{Email: email}
	req := &command.AlterInboundRequest{
		Tag:       inboundTag,
		Operation: serial.ToTypedMessage(op),
	}

	_, err := (*x.HandlerServiceClient).AlterInbound(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to remove user: %w", err)
	}

	return nil
}

func (x *XrayAPI) GetTraffic(reset bool) ([]*Traffic, []*ClientTraffic, error) {
	if x.grpcClient == nil {
		return nil, nil, common.NewError("xray api is not initialized")
	}

	trafficRegex := regexp.MustCompile(`(inbound|outbound)>>>([^>]+)>>>traffic>>>(downlink|uplink)`)
	clientTrafficRegex := regexp.MustCompile(`user>>>([^>]+)>>>traffic>>>(downlink|uplink)`)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	if x.StatsServiceClient == nil {
		return nil, nil, common.NewError("xray StatusServiceClient is not initialized")
	}

	resp, err := (*x.StatsServiceClient).QueryStats(ctx, &statsService.QueryStatsRequest{Reset_: reset})
	if err != nil {
		logger.Debug("Failed to query Xray stats:", err)
		return nil, nil, err
	}

	tagTrafficMap := make(map[string]*Traffic)
	emailTrafficMap := make(map[string]*ClientTraffic)

	for _, stat := range resp.GetStat() {
		if matches := trafficRegex.FindStringSubmatch(stat.Name); len(matches) == 4 {
			processTraffic(matches, stat.Value, tagTrafficMap)
		} else if matches := clientTrafficRegex.FindStringSubmatch(stat.Name); len(matches) == 3 {
			processClientTraffic(matches, stat.Value, emailTrafficMap)
		}
	}
	return mapToSlice(tagTrafficMap), mapToSlice(emailTrafficMap), nil
}

func processTraffic(matches []string, value int64, trafficMap map[string]*Traffic) {
	isInbound := matches[1] == "inbound"
	tag := matches[2]
	isDown := matches[3] == "downlink"

	if tag == "api" {
		return
	}

	traffic, ok := trafficMap[tag]
	if !ok {
		traffic = &Traffic{
			IsInbound:  isInbound,
			IsOutbound: !isInbound,
			Tag:        tag,
		}
		trafficMap[tag] = traffic
	}

	if isDown {
		traffic.Down = value
	} else {
		traffic.Up = value
	}
}

func processClientTraffic(matches []string, value int64, clientTrafficMap map[string]*ClientTraffic) {
	email := matches[1]
	isDown := matches[2] == "downlink"

	traffic, ok := clientTrafficMap[email]
	if !ok {
		traffic = &ClientTraffic{Email: email}
		clientTrafficMap[email] = traffic
	}

	if isDown {
		traffic.Down = value
	} else {
		traffic.Up = value
	}
}

func mapToSlice[T any](m map[string]*T) []*T {
	result := make([]*T, 0, len(m))
	for _, v := range m {
		result = append(result, v)
	}
	return result
}
