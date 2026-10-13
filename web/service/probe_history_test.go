package service

import (
	"encoding/json"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"path/filepath"
	"reflect"
	"testing"
	"time"
	"x-ui/database/model"
	"x-ui/xray"
)

func TestProbeHistoryRetentionDedupAndPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.db")
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err = db.AutoMigrate(&model.ProbeHistory{}); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	batch := xray.ProbeBatch{Session: "core-a", Events: []xray.ProbeEvent{
		{Sequence: 1, Time: now.UnixMilli(), Strategy: "random", Outbound: "node-a", Status: "success", Delay: 13.2},
		{Sequence: 2, Time: now.UnixMilli(), Strategy: "leastLoad", Outbound: "node-a", Status: "failed"},
		{Sequence: 3, Time: now.UnixMilli(), Strategy: "random", Outbound: "node-b", Status: "network_unavailable"},
		{Sequence: 4, Time: now.Add(-8 * 24 * time.Hour).UnixMilli(), Strategy: "random", Outbound: "expired", Status: "success"},
	}}
	for i := 0; i < 2; i++ {
		if err = storeProbeBatch(db, batch, now, true); err != nil {
			t.Fatal(err)
		}
	}
	var count int64
	db.Model(&model.ProbeHistory{}).Count(&count)
	if count != 3 {
		t.Fatalf("dedup/age failed: %d", count)
	}
	page, err := queryProbeHistory(db, "random", "", 1)
	if err != nil || page.Total != 2 || len(page.Tags) != 2 {
		t.Fatalf("query failed: %+v %v", page, err)
	}
	for _, row := range page.Rows {
		if row.Strategy != "random" {
			t.Fatal("strategy leaked")
		}
	}
	page, err = queryProbeHistory(db, "random", "node-a", 1)
	if err != nil || page.Total != 1 || page.Rows[0].Delay != 13.2 {
		t.Fatal("tag filter failed")
	}
	if _, err = queryProbeHistory(db, "bogus", "", 1); err == nil {
		t.Fatal("accepted invalid filter")
	}
	sqlDB, _ := db.DB()
	sqlDB.Close()
	db, err = gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { sqlDB, _ := db.DB(); sqlDB.Close() }()
	page, err = queryProbeHistory(db, "random", "", 1)
	if err != nil || page.Total != 2 {
		t.Fatal("history did not persist")
	}
	// A new core session can reuse sequence numbers without losing new events.
	batch.Session = "core-b"
	batch.Events = batch.Events[:1]
	if err = storeProbeBatch(db, batch, now, false); err != nil {
		t.Fatal(err)
	}
	db.Model(&model.ProbeHistory{}).Count(&count)
	if count != 4 {
		t.Fatal("restart sequence collided")
	}
	if err = db.Exec("WITH RECURSIVE n(x) AS (SELECT 1 UNION ALL SELECT x+1 FROM n WHERE x<100005) INSERT INTO probe_histories(session,sequence,time,strategy,outbound,status,delay) SELECT 'bulk',x,?,'random','bulk','success',1 FROM n", now.UnixMilli()).Error; err != nil {
		t.Fatal(err)
	}
	if err = storeProbeBatch(db, xray.ProbeBatch{Session: "retention"}, now, true); err != nil {
		t.Fatal(err)
	}
	db.Model(&model.ProbeHistory{}).Count(&count)
	if count != ProbeHistoryLimit {
		t.Fatalf("unbounded history: %d", count)
	}
	if err = storeProbeBatch(db, xray.ProbeBatch{Session: "retention"}, now.Add(8*24*time.Hour), true); err != nil {
		t.Fatal(err)
	}
	db.Model(&model.ProbeHistory{}).Count(&count)
	if count != 0 {
		t.Fatalf("expired history retained: %d", count)
	}
}

func TestOutboundProbeHistoryAllScopesAndRuntimeSelection(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "history.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	if err = db.AutoMigrate(&model.ProbeHistory{}); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	batch := xray.ProbeBatch{Session: "scopes", Events: []xray.ProbeEvent{}}
	for i, scope := range []string{"outbound", "leastPing", "leastLoad", "random", "roundRobin", "legacyBurst"} {
		batch.Events = append(batch.Events, xray.ProbeEvent{Sequence: uint64(i + 1), Time: now.UnixMilli(), Strategy: scope, Outbound: "a & 中文", Status: "success", Delay: 12})
	}
	batch.Events = append(batch.Events, xray.ProbeEvent{Sequence: 7, Time: now.UnixMilli(), Strategy: "random", Outbound: "other", Status: "failed"})
	if err = storeProbeBatch(db, batch, now, true); err != nil {
		t.Fatal(err)
	}
	page, err := queryProbeHistory(db, "all", "a & 中文", 1)
	if err != nil || page.Total != 6 || len(page.Tags) != 1 {
		t.Fatalf("outbound filtering failed: %+v %v", page, err)
	}
	for _, row := range page.Rows {
		if row.Outbound != "a & 中文" {
			t.Fatal("other outbound leaked")
		}
	}
	if _, err = queryProbeHistory(db, "all", "", 1); err == nil {
		t.Fatal("unbounded all-scopes query accepted")
	}
	page, err = queryProbeHistory(db, "outbound", "a & 中文", 1)
	if err != nil || page.Total != 1 {
		t.Fatal("standalone scope contaminated")
	}
	cfg := &xray.Config{StrategyObservatory: []byte(`{"outbound":{"subjectSelector":["a"]},"random":{"subjectSelector":["a"]}}`)}
	if got := activeProbeStrategies(cfg, "abc"); !reflect.DeepEqual(got, []string{"random"}) {
		t.Fatal(got)
	}
	if got := activeProbeStrategies(cfg, "a"); !reflect.DeepEqual(got, []string{"outbound", "random"}) {
		t.Fatal(got)
	}
	if len(activeProbeStrategies(cfg, "other")) != 0 {
		t.Fatal("wrong active scope")
	}
	var config xray.Config
	input := `{"duiDisabledProbes":{"outbound":{"pingConfig":{"interval":"17m","future":"keep"}}}}`
	if err = json.Unmarshal([]byte(input), &config); err != nil {
		t.Fatal(err)
	}
	output, _ := json.Marshal(config)
	var raw map[string]any
	json.Unmarshal(output, &raw)
	if _, exists := raw["duiDisabledProbes"]; exists {
		t.Fatal("UI metadata leaked into runtime")
	}
}
