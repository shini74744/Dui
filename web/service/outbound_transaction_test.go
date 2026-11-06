package service

import (
	"database/sql"
	"path/filepath"
	"testing"
	"x-ui/database"
	"x-ui/database/model"
	"x-ui/xray"
)

func TestOutboundTrafficReportsCommitFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "traffic.db")
	if err := database.InitDB(path); err != nil {
		t.Fatal(err)
	}
	defer database.CloseDB()
	db := database.GetDB()
	conn, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	conn.SetMaxOpenConns(1)
	if err = db.Exec("PRAGMA busy_timeout=50").Error; err != nil {
		t.Fatal(err)
	}
	if err = db.Create(&model.OutboundTraffics{Tag: "commit-test"}).Error; err != nil {
		t.Fatal(err)
	}
	reader, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	tx, err := reader.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	var rows int
	if err = tx.QueryRow("SELECT count(*) FROM outbound_traffics").Scan(&rows); err != nil {
		t.Fatal(err)
	}
	err, _ = (&OutboundService{}).AddTraffic([]*xray.Traffic{{Tag: "commit-test", IsOutbound: true, Up: 123}}, nil)
	if err == nil {
		t.Fatal("commit failure was reported as success")
	}
	tx.Rollback()
	var saved model.OutboundTraffics
	if err = db.Where("tag = ?", "commit-test").First(&saved).Error; err != nil {
		t.Fatal(err)
	}
	if saved.Up != 0 {
		t.Fatalf("uncommitted traffic persisted: %d", saved.Up)
	}
}
