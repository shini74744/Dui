package database

import (
	"bytes"
	"io"
	"io/fs"
	"log"
	"os"
	"path"
	"slices"
	"sync"
	"time"

	"x-ui/config"
	"x-ui/database/model"
	"x-ui/util/crypto"
	"x-ui/xray"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var db *gorm.DB

// Requests hold a read lock; database replacement holds the write lock after
// pausing scheduled jobs. The import request itself must not acquire a read lock.
var MaintenanceMu sync.RWMutex
var dbMu sync.RWMutex

const (
	defaultUsername = "admin"
	defaultPassword = "admin"
)

func initModels(db *gorm.DB) error {
	models := []any{
		&model.User{},
		&model.Inbound{},
		&model.OutboundTraffics{},
		&model.DailyClientTraffic{},
		&model.ClientNotifyState{},
		&model.ClientTelegramBot{},
		&model.TelegramNotificationHistory{},
		&model.ManagedBlacklistEntry{},
		&model.Setting{},
		&model.ProbeHistory{},
		&model.InboundClientIps{},
		&xray.ClientTraffic{},
		&model.HistoryOfSeeders{},
		&LinkHistory{}, // 把 LinkHistory 表也迁移
		&ShortLink{},   // 新增 ShortLink 模型
		&model.LotteryWin{},
	}
	for _, model := range models {
		if err := db.AutoMigrate(model); err != nil {
			log.Printf("Error auto migrating model: %v", err)
			return err
		}
	}
	return nil
}

func initUser(db *gorm.DB) error {
	empty, err := isTableEmpty(db, "users")
	if err != nil {
		log.Printf("Error checking if users table is empty: %v", err)
		return err
	}
	if empty {
		hashedPassword, err := crypto.HashPasswordAsBcrypt(defaultPassword)

		if err != nil {
			log.Printf("Error hashing default password: %v", err)
			return err
		}

		user := &model.User{
			Username: defaultUsername,
			Password: hashedPassword,
		}
		return db.Create(user).Error
	}
	return nil
}

func runSeeders(db *gorm.DB, isUsersEmpty bool) error {
	empty, err := isTableEmpty(db, "history_of_seeders")
	if err != nil {
		log.Printf("Error checking if users table is empty: %v", err)
		return err
	}

	if empty && isUsersEmpty {
		hashSeeder := &model.HistoryOfSeeders{
			SeederName: "UserPasswordHash",
		}
		return db.Create(hashSeeder).Error
	} else {
		var seedersHistory []string
		db.Model(&model.HistoryOfSeeders{}).Pluck("seeder_name", &seedersHistory)

		if !slices.Contains(seedersHistory, "UserPasswordHash") && !isUsersEmpty {
			var users []model.User
			db.Find(&users)

			for _, user := range users {
				hashedPassword, err := crypto.HashPasswordAsBcrypt(user.Password)
				if err != nil {
					log.Printf("Error hashing password for user '%s': %v", user.Username, err)
					return err
				}
				db.Model(&user).Update("password", hashedPassword)
			}

			hashSeeder := &model.HistoryOfSeeders{
				SeederName: "UserPasswordHash",
			}
			return db.Create(hashSeeder).Error
		}
	}

	return nil
}

func isTableEmpty(db *gorm.DB, tableName string) (bool, error) {
	var count int64
	err := db.Table(tableName).Count(&count).Error
	return count == 0, err
}

func openDB(dbPath string) (_ *gorm.DB, resultErr error) {
	dir := path.Dir(dbPath)
	err := os.MkdirAll(dir, fs.ModePerm)
	if err != nil {
		return nil, err
	}

	var gormLogger logger.Interface

	if config.IsDebug() {
		gormLogger = logger.Default
	} else {
		gormLogger = logger.Discard
	}

	c := &gorm.Config{
		Logger: gormLogger,
	}
	opened, err := gorm.Open(sqlite.Open(dbPath), c)
	defer func() {
		if resultErr != nil && opened != nil {
			if conn, err := opened.DB(); err == nil {
				_ = conn.Close()
			}
		}
	}()
	if err != nil {
		return nil, err
	}

	if err := initModels(opened); err != nil {
		return nil, err
	}

	isUsersEmpty, err := isTableEmpty(opened, "users")
	if err != nil {
		return nil, err
	}

	if err := initUser(opened); err != nil {
		return nil, err
	}
	if err := runSeeders(opened, isUsersEmpty); err != nil {
		return nil, err
	}
	return opened, nil
}

func InitDB(dbPath string) error {
	opened, err := openDB(dbPath)
	if err != nil {
		return err
	}
	dbMu.Lock()
	db = opened
	dbMu.Unlock()
	return nil
}

func CloseDB() error {
	db := GetDB()
	if db != nil {
		sqlDB, err := db.DB()
		if err != nil {
			return err
		}
		return sqlDB.Close()
	}
	return nil
}

func GetDB() *gorm.DB {
	dbMu.RLock()
	defer dbMu.RUnlock()
	return db
}

func IsNotFound(err error) bool {
	return err == gorm.ErrRecordNotFound
}

func IsSQLiteDB(file io.ReaderAt) (bool, error) {
	signature := []byte("SQLite format 3\x00")
	buf := make([]byte, len(signature))
	_, err := file.ReadAt(buf, 0)
	if err != nil {
		return false, err
	}
	return bytes.Equal(buf, signature), nil
}

func Checkpoint() error {
	db := GetDB()
	// Update WAL
	err := db.Exec("PRAGMA wal_checkpoint;").Error
	if err != nil {
		return err
	}
	return nil
}

// HasUserWonToday 检查指定用户今天是否已经中过奖
// 〔中文注释〕:【修正】将 gorm.DB() 替换为全局变量 db
func HasUserWonToday(userID int64) (bool, error) {
	db := GetDB()
	now := time.Now()
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	endOfDay := startOfDay.Add(24 * time.Hour)

	var count int64
	// 在 lottery_wins 表中查找符合条件（用户ID匹配且中奖日期在今天之内）的记录数量
	err := db.Model(&model.LotteryWin{}).Where("user_id = ? AND win_date >= ? AND win_date < ?", userID, startOfDay, endOfDay).Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// RecordUserWin 记录用户的中奖信息
// 〔中文注释〕:【修正】将 gorm.DB() 替换为全局变量 db
func RecordUserWin(userID int64, prize string) error {
	db := GetDB()
	winRecord := &model.LotteryWin{
		UserID:  userID,
		Prize:   prize,
		WinDate: time.Now(),
	}
	// 在 lottery_wins 表中创建一条新的记录
	return db.Create(winRecord).Error
}
