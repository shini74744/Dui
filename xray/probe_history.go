package xray

import (
	"context"
	"encoding/json"
	"fmt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/structpb"
)

type ProbeEvent struct {
	Sequence uint64  `json:"sequence"`
	Time     int64   `json:"time"`
	Strategy string  `json:"strategy"`
	Outbound string  `json:"outbound"`
	Status   string  `json:"status"`
	Delay    float64 `json:"delay"`
}
type ProbeBatch struct {
	Session string       `json:"session"`
	Events  []ProbeEvent `json:"events"`
	Next    uint64       `json:"next"`
	Latest  uint64       `json:"latest"`
	Missed  bool         `json:"missed"`
}

func ReadProbeHistory(ctx context.Context, port int, session string, after uint64) (ProbeBatch, error) {
	var batch ProbeBatch
	if port < 1 || port > 65535 {
		return batch, fmt.Errorf("core API unavailable")
	}
	conn, err := grpc.NewClient(fmt.Sprintf("127.0.0.1:%d", port), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return batch, err
	}
	defer conn.Close()
	request, _ := structpb.NewStruct(map[string]interface{}{"session": session, "after": float64(after)})
	response := new(structpb.Struct)
	if err = conn.Invoke(ctx, "/xray.app.stats.command.DUIProbeHistoryService/GetProbeHistory", request, response); err != nil {
		return batch, err
	}
	payload, err := response.MarshalJSON()
	if err != nil {
		return batch, err
	}
	err = json.Unmarshal(payload, &batch)
	return batch, err
}
