package service

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"x-ui/database"
	"x-ui/database/model"
	"x-ui/xray"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const ProbeHistoryDays = 7
const ProbeHistoryLimit = 100000

var probeCollector = struct {
	sync.Mutex
	session     string
	next        uint64
	state       string
	lastSuccess int64
	gap         bool
	lastPrune   time.Time
}{state: "waiting"}

func probeStrategyValid(s string) bool {
	return s == "leastPing" || s == "leastLoad" || s == "random" || s == "roundRobin"
}
func storeProbeBatch(db *gorm.DB, b xray.ProbeBatch, now time.Time, prune bool) error {
	if b.Session == "" || len(b.Session) > 64 || len(b.Events) > 500 {
		return fmt.Errorf("invalid probe batch")
	}
	rows := []model.ProbeHistory{}
	cutoff := now.Add(-ProbeHistoryDays * 24 * time.Hour).UnixMilli()
	for _, e := range b.Events {
		if (!probeStrategyValid(e.Strategy) && e.Strategy != "legacyPing" && e.Strategy != "legacyBurst") || e.Sequence == 0 || e.Time < cutoff || e.Time > now.Add(time.Minute).UnixMilli() || len(e.Outbound) > 1024 || math.IsNaN(e.Delay) || math.IsInf(e.Delay, 0) || e.Delay < 0 {
			continue
		}
		if e.Status != "success" && e.Status != "failed" && e.Status != "network_unavailable" {
			continue
		}
		rows = append(rows, model.ProbeHistory{Session: b.Session, Sequence: e.Sequence, Time: e.Time, Strategy: e.Strategy, Outbound: e.Outbound, Status: e.Status, Delay: e.Delay})
	}
	if len(rows) == 0 && !prune {
		return nil
	}
	return db.Transaction(func(tx *gorm.DB) error {
		if len(rows) > 0 {
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).CreateInBatches(rows, 100).Error; err != nil {
				return err
			}
		}
		if err := tx.Where("time < ?", cutoff).Delete(&model.ProbeHistory{}).Error; err != nil {
			return err
		}
		return tx.Exec("DELETE FROM probe_histories WHERE id < COALESCE((SELECT id FROM probe_histories ORDER BY id DESC LIMIT 1 OFFSET ?), 0)", ProbeHistoryLimit-1).Error
	})
}
func CollectProbeHistory(port int) {
	probeCollector.Lock()
	defer probeCollector.Unlock()
	db := database.GetDB()
	if db == nil {
		return
	}
	now := time.Now()
	prune := now.Sub(probeCollector.lastPrune) > time.Hour
	if prune {
		if err := storeProbeBatch(db, xray.ProbeBatch{Session: "retention"}, now, true); err != nil {
			probeCollector.state = "storage_error"
			return
		}
		probeCollector.lastPrune = now
	}
	if port == 0 {
		probeCollector.state = "offline"
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	for i := 0; i < 8; i++ {
		batch, err := xray.ReadProbeHistory(ctx, port, probeCollector.session, probeCollector.next)
		if err != nil {
			probeCollector.state = "unavailable"
			if status.Code(err) == codes.Unimplemented {
				probeCollector.state = "unsupported"
			}
			return
		}
		if batch.Next > batch.Latest || len(batch.Events) > 500 {
			probeCollector.state = "unavailable"
			return
		}
		if err = storeProbeBatch(db, batch, now, false); err != nil {
			probeCollector.state = "storage_error"
			return
		}
		probeCollector.session = batch.Session
		probeCollector.next = batch.Next
		probeCollector.gap = probeCollector.gap || batch.Missed
		probeCollector.lastSuccess = now.UnixMilli()
		probeCollector.state = "ok"
		if batch.Next >= batch.Latest {
			return
		}
	}
}
func ProbeHistoryAPIPort() int {
	lock.Lock()
	defer lock.Unlock()
	if p == nil || !p.IsRunning() {
		return 0
	}
	return p.GetAPIPort()
}

type ProbeHistoryPage struct {
	Rows          []model.ProbeHistory `json:"rows"`
	Total         int64                `json:"total"`
	Page          int                  `json:"page"`
	Tags          []string             `json:"tags"`
	State         string               `json:"state"`
	LastSuccess   int64                `json:"lastSuccess"`
	Gap           bool                 `json:"gap"`
	RetentionDays int                  `json:"retentionDays"`
	Limit         int                  `json:"limit"`
}

func QueryProbeHistory(strategy, tag string, page int) (ProbeHistoryPage, error) {
	return queryProbeHistory(database.GetDB(), strategy, tag, page)
}
func queryProbeHistory(db *gorm.DB, strategy, tag string, page int) (ProbeHistoryPage, error) {
	result := ProbeHistoryPage{Rows: []model.ProbeHistory{}, Tags: []string{}, Page: page, RetentionDays: ProbeHistoryDays, Limit: ProbeHistoryLimit}
	if !probeStrategyValid(strategy) || len(tag) > 1024 || page < 1 || page > 2000 {
		return result, fmt.Errorf("invalid history filter")
	}
	sources := []string{strategy}
	// Legacy observers shared their measurements; label these rows rather than inventing strategy-specific probes.
	sources = append(sources, "legacyPing", "legacyBurst")
	base := db.Model(&model.ProbeHistory{}).Where("strategy IN ? AND time >= ?", sources, time.Now().Add(-ProbeHistoryDays*24*time.Hour).UnixMilli())
	if err := base.Session(&gorm.Session{}).Distinct("outbound").Order("outbound").Limit(1000).Pluck("outbound", &result.Tags).Error; err != nil {
		return result, err
	}
	if tag != "" {
		base = base.Where("outbound = ?", tag)
	}
	if err := base.Session(&gorm.Session{}).Count(&result.Total).Error; err != nil {
		return result, err
	}
	if err := base.Order("id DESC").Offset((page - 1) * 50).Limit(50).Find(&result.Rows).Error; err != nil {
		return result, err
	}
	probeCollector.Lock()
	result.State = probeCollector.state
	result.LastSuccess = probeCollector.lastSuccess
	result.Gap = probeCollector.gap
	probeCollector.Unlock()
	return result, nil
}
