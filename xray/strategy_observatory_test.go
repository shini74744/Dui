package xray

import (
	"encoding/json"
	"testing"
)

func TestStrategyObservatoryVersionGuard(t *testing.T) {
	for _, v := range []string{"vx-26.5", "vx-26.10", "vx-27.0"} {
		if !supportsStrategyObservatoryVersion("Xray " + v + " (Xray)") {
			t.Fatal(v)
		}
	}
	for _, v := range []string{"vx-26.4", "v26.9.9", "latest", "vx-26.5-fake", "vx-26.-1"} {
		if supportsStrategyObservatoryVersion("Xray " + v) {
			t.Fatal(v)
		}
	}
	cfg := &Config{}
	if err := ValidateStrategyObservatoryTarget(cfg, "vx-26.4"); err != nil {
		t.Fatal(err)
	}
	cfg.StrategyObservatory = []byte("{\"random\":{\"subjectSelector\":[],\"pingConfig\":{\"interval\":\"10s\",\"timeout\":\"1s\",\"sampling\":2}}}")
	if err := ValidateStrategyObservatoryTarget(cfg, "vx-26.4"); err == nil {
		t.Fatal("downgrade accepted")
	}
	if err := ValidateStrategyObservatoryTarget(cfg, "vx-26.5"); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var round Config
	if err = json.Unmarshal(encoded, &round); err != nil {
		t.Fatal(err)
	}
	if string(cfg.StrategyObservatory) != string(round.StrategyObservatory) {
		t.Fatal("settings lost during config round trip")
	}
	other := round
	other.StrategyObservatory = []byte("{}")
	if other.Equals(&round) {
		t.Fatal("probe edits not detected")
	}
}
