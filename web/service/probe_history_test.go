package service

import (
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"path/filepath"
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
