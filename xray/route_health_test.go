package xray

import (
	"encoding/json"
	"testing"
)

func TestRouteHealthCoreVersionGate(t *testing.T) {
	for _, raw := range []string{
		`{"strategyObservatory":{"outbound":{}}}`,
		`{"routing":{"rules":[{"outboundTag":"a","fallbackTag":"b"}]}}`,
		`{"routing":{"balancers":[{"strategy":{"type":"priority"}}]}}`,
	} {
		var c Config
		if err := json.Unmarshal([]byte(raw), &c); err != nil {
			t.Fatal(err)
		}
		for _, version := range []string{"vx-26.5", "vx-26.9", "26.9.9", "Unknown"} {
			if ValidateStrategyObservatoryTarget(&c, version) == nil {
				t.Fatalf("accepted incompatible %s", version)
			}
		}
		for _, version := range []string{"vx-26.10", "vx-26.11", "vx-27.0"} {
			if err := ValidateStrategyObservatoryTarget(&c, version); err != nil {
				t.Fatal(err)
			}
		}
	}
}
