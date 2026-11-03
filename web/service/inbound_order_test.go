package service

import (
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"x-ui/database/model"
)

func TestInboundOrderLegacyMigrationAndPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "order.db")
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	// A pre-upgrade table has no sort_order; migration must retain its values.
	if err = db.Exec("CREATE TABLE inbounds (id integer PRIMARY KEY, user_id integer, tag text, remark text)").Error; err != nil {
		t.Fatal(err)
	}
	db.Exec("INSERT INTO inbounds (id,user_id,tag,remark) VALUES (1,7,'one','original'),(4,7,'four','keep'),(14,7,'fourteen','last'),(20,9,'other','private')")
	if err = db.AutoMigrate(&model.Inbound{}); err != nil {
		t.Fatal(err)
	}
	ids := func(conn *gorm.DB) []int {
		var values []int
		if err := conn.Model(&model.Inbound{}).Where("user_id = ?", 7).Order(inboundDisplayOrder).Pluck("id", &values).Error; err != nil {
			t.Fatal(err)
		}
		return values
	}
	original := []int{1, 4, 14}
	if !reflect.DeepEqual(ids(db), original) {
		t.Fatal(ids(db))
	}
	var before []model.Inbound
	db.Order("id").Find(&before)
	if err = reorderInbounds(db, 7, original, []int{14, 1, 4}); err != nil {
		t.Fatal(err)
	}
	var after []model.Inbound
	db.Order("id").Find(&after)
	for i := range before {
		if before[i].UserId == 7 {
			after[i].SortOrder = before[i].SortOrder
		}
		if !reflect.DeepEqual(after[i], before[i]) {
			t.Fatal("non-order fields or other user changed")
		}
	}
	reopened, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil || !reflect.DeepEqual(ids(reopened), []int{14, 1, 4}) {
		t.Fatal("order not persisted", err)
	}
	// A stale edit or traffic update must not reset newly saved ordering.
	stale := before[0]
	stale.Remark = "edited"
	stale.SortOrder = 999
	if err = db.Save(&stale).Error; err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(ids(db), []int{14, 1, 4}) {
		t.Fatal("stale save overwrote order")
	}
	// Newly created rows (including a reused ID) append after existing rows.
	rank, err := nextInboundSortOrder(db, 7)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Create(&model.Inbound{Id: 2, UserId: 7, Tag: "new", SortOrder: rank}).Error; err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(ids(db), []int{14, 1, 4, 2}) {
		t.Fatal(ids(db))
	}
}

func TestInboundOrderRejectsStaleForeignAndInvalidListsAtomically(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "order.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err = db.AutoMigrate(&model.Inbound{}); err != nil {
		t.Fatal(err)
	}
	for _, row := range []model.Inbound{{Id: 1, UserId: 7, Tag: "a"}, {Id: 4, UserId: 7, Tag: "b"}, {Id: 9, UserId: 8, Tag: "foreign"}} {
		if err = db.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, test := range []struct{ base, ids []int }{
		{nil, nil}, {[]int{1, 4}, []int{1}}, {[]int{1, 4}, []int{1, 1}},
		{[]int{1, 4}, []int{4, 9}}, {[]int{1, 4}, []int{-1, 4}},
		{[]int{4, 1}, []int{1, 4}}, {[]int{1, 4, 9}, []int{9, 4, 1}},
	} {
		if err = reorderInbounds(db, 7, test.base, test.ids); err == nil {
			t.Fatal("invalid request accepted", test)
		}
		var count int64
		db.Model(&model.Inbound{}).Where("sort_order != 0").Count(&count)
		if count != 0 {
			t.Fatal("partial update")
		}
	}
	// Force the second SQL update to fail; the first must be rolled back.
	db.Exec("CREATE TRIGGER fail_order BEFORE UPDATE OF sort_order ON inbounds WHEN NEW.id=1 BEGIN SELECT RAISE(ABORT,'test failure'); END")
	if err = reorderInbounds(db, 7, []int{1, 4}, []int{4, 1}); err == nil {
		t.Fatal("expected failure")
	}
	var row model.Inbound
	db.First(&row, 4)
	if row.SortOrder != 0 {
		t.Fatal("transaction did not roll back")
	}
	db.Exec("DROP TRIGGER fail_order")
	if err = reorderInbounds(db, 7, []int{1, 4}, []int{4, 1}); err != nil {
		t.Fatal(err)
	}
	if err = reorderInbounds(db, 7, []int{1, 4}, []int{1, 4}); err == nil {
		t.Fatal("stale tab replaced new order")
	}
	if err = reorderInbounds(db, 8, []int{1, 4}, []int{4, 1}); err == nil || errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatal("ownership check failed")
	}
}

func TestInboundOrderWaitsForBackgroundWriter(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "busy.db")+"?_busy_timeout=5"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err = db.AutoMigrate(&model.Inbound{}); err != nil {
		t.Fatal(err)
	}
	for _, row := range []model.Inbound{{Id: 1, UserId: 7, Tag: "one"}, {Id: 4, UserId: 7, Tag: "four"}} {
		if err = db.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
	}
	writer := db.Begin()
	if err = writer.Model(&model.Inbound{}).Where("id = ?", 1).Update("remark", "background write").Error; err != nil {
		t.Fatal(err)
	}
	committed := make(chan error, 1)
	go func() { time.Sleep(80 * time.Millisecond); committed <- writer.Commit().Error }()
	err = reorderInbounds(db, 7, []int{1, 4}, []int{4, 1})
	if commitErr := <-committed; commitErr != nil {
		t.Fatal(commitErr)
	}
	if err != nil {
		t.Fatalf("transient background write prevented reorder: %v", err)
	}
	var ids []int
	db.Model(&model.Inbound{}).Order(inboundDisplayOrder).Pluck("id", &ids)
	if !reflect.DeepEqual(ids, []int{4, 1}) {
		t.Fatal(ids)
	}
	var saved model.Inbound
	db.First(&saved, 1)
	if saved.Remark != "background write" {
		t.Fatal("background metadata was lost")
	}

	// A different tab can win the lock; retries must not overwrite its order.
	writer = db.Begin()
	if err = writer.Exec("UPDATE inbounds SET sort_order = CASE id WHEN 1 THEN 1 ELSE 2 END").Error; err != nil {
		t.Fatal(err)
	}
	go func() { time.Sleep(80 * time.Millisecond); committed <- writer.Commit().Error }()
	err = reorderInbounds(db, 7, []int{4, 1}, []int{1, 4})
	if commitErr := <-committed; commitErr != nil {
		t.Fatal(commitErr)
	}
	if err == nil || !strings.Contains(err.Error(), "顺序已变化") {
		t.Fatal("retry overwrote a concurrent order", err)
	}

	// A persistent writer lock must return an error after bounded retries.
	writer = db.Begin()
	if err = writer.Model(&model.Inbound{}).Where("id = ?", 1).Update("remark", "held lock").Error; err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	err = reorderInbounds(db, 7, []int{1, 4}, []int{4, 1})
	writer.Rollback()
	if err == nil || time.Since(started) > 2*time.Second {
		t.Fatal("unbounded lock retry", err)
	}
	ids = nil
	db.Model(&model.Inbound{}).Order(inboundDisplayOrder).Pluck("id", &ids)
	if !reflect.DeepEqual(ids, []int{1, 4}) {
		t.Fatal("failed retry changed the order", ids)
	}
}
