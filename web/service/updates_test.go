package service

import (
	"testing"
	"x-ui/internal/update"
)

func TestUpdateItemUnknownAndStoppedCore(t *testing.T) {
	for _, tc := range []struct {
		name, current, latest          string
		known, available, currentState bool
	}{
		{"stopped_old", "vx-26.6", "vx-26.7", true, true, false},
		{"official_core", "26.9.9", "vx-26.7", true, true, false},
		{"official_core_v_prefix", "v26.9.9", "vx-26.7", true, true, false},
		{"current", "vx-26.7", "vx-26.7", true, false, true},
		{"newer_installed", "vx-26.8", "vx-26.7", true, false, true},
		{"unknown", "Unknown", "vx-26.7", false, true, false},
		{"unreadable", "", "vx-26.7", false, true, false},
		{"malformed", "not-a-version", "vx-26.7", false, true, false},
		{"not_checked", "vx-26.6", "", true, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			item := updateItem("core", tc.current, update.Offer{Version: tc.latest})
			if item["currentKnown"] != tc.known || item["available"] != tc.available || item["upToDate"] != tc.currentState {
				t.Fatalf("%+v", item)
			}
		})
	}
}
