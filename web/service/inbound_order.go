package service

import (
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/mattn/go-sqlite3"
	"gorm.io/gorm"
	"x-ui/database"
	"x-ui/database/model"
)

// Zero is the pre-upgrade value: retain the original ascending ID order.
const inboundDisplayOrder = "CASE WHEN sort_order > 0 THEN sort_order ELSE id END ASC, id ASC"

func nextInboundSortOrder(tx *gorm.DB, userID int) (int64, error) {
	var value int64
	err := tx.Model(&model.Inbound{}).Where("user_id = ?", userID).
		Select("COALESCE(MAX(CASE WHEN sort_order > 0 THEN sort_order ELSE id END), 0) + 1").Scan(&value).Error
	return value, err
}

func (s *InboundService) ReorderInbounds(userID int, base, ids []int) error {
	return reorderInbounds(database.GetDB(), userID, base, ids)
}

func reorderInbounds(db *gorm.DB, userID int, base, ids []int) error {
	if len(ids) == 0 || len(ids) != len(base) {
		return fmt.Errorf("排序列表无效，请刷新后重试")
	}
	seen := make(map[int]bool, len(ids))
	for _, id := range ids {
		if id <= 0 || seen[id] {
			return fmt.Errorf("排序列表包含无效或重复的入站")
		}
		seen[id] = true
	}
	apply := func() error {
		return db.Transaction(func(tx *gorm.DB) error {
			var current []int
			if err := tx.Model(&model.Inbound{}).Where("user_id = ?", userID).
				Order(inboundDisplayOrder).Pluck("id", &current).Error; err != nil {
				return err
			}
			if !slices.Equal(current, base) {
				return fmt.Errorf("入站列表或顺序已变化，请刷新后重新排序")
			}
			for _, id := range current {
				if !seen[id] {
					return fmt.Errorf("排序列表与当前入站不一致，请刷新后重试")
				}
			}
			if slices.Equal(current, ids) {
				return nil
			}
			// Raw, scoped column updates are intentional. Ordinary inbound saves cannot
			// overwrite this metadata, and reordering never invokes the core API.
			for index, id := range ids {
				result := tx.Exec("UPDATE inbounds SET sort_order = ? WHERE id = ? AND user_id = ?", index+1, id, userID)
				if result.Error != nil {
					return result.Error
				}
				if result.RowsAffected != 1 {
					return fmt.Errorf("入站列表已变化，请刷新后重试")
				}
			}
			return nil
		})
	}
	// A statistics write can briefly hold SQLite's writer lock. Retry the whole
	// transaction so each attempt revalidates ownership and the original order.
	deadline := time.Now().Add(time.Second)
	for attempt := 0; ; attempt++ {
		err := apply()
		var sqliteErr sqlite3.Error
		if err == nil || attempt >= 5 || time.Now().After(deadline) ||
			!errors.As(err, &sqliteErr) || (sqliteErr.Code != sqlite3.ErrBusy && sqliteErr.Code != sqlite3.ErrLocked) {
			return err
		}
		time.Sleep(time.Duration(10<<attempt) * time.Millisecond)
	}
}
