package database

import (
	"os"
	"path/filepath"
	"testing"
	"x-ui/database/model"
)

func TestInvalidDatabaseDoesNotReplaceLiveConnection(t *testing.T) {
	dir := t.TempDir()
	if err := InitDB(filepath.Join(dir, "live.db")); err != nil {
		t.Fatal(err)
	}
	defer CloseDB()
	original := GetDB()
	bad := filepath.Join(dir, "broken.db")
	if err := os.WriteFile(bad, []byte("SQLite format 3\x00broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := InitDB(bad); err == nil {
		t.Fatal("invalid database accepted")
	}
	if GetDB() != original {
		t.Fatal("live connection changed")
	}
	var user model.User
	if err := GetDB().First(&user).Error; err != nil {
		t.Fatal(err)
	}
}
