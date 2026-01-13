package storage

import (
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var (
	db   *gorm.DB
	once sync.Once
)

// Init 初始化数据库连接，自动执行迁移
func Init() error {
	var initErr error
	once.Do(func() {
		dbPath, err := getDBPath()
		if err != nil {
			initErr = err
			return
		}

		// 确保目录存在
		if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
			initErr = err
			return
		}

		// 打开数据库连接
		db, err = gorm.Open(sqlite.Open(dbPath), &gorm.Config{
			Logger: logger.Default.LogMode(logger.Silent),
		})
		if err != nil {
			initErr = err
			return
		}

		// 自动迁移
		initErr = autoMigrate()
	})
	return initErr
}

// DB 获取数据库实例
func DB() *gorm.DB {
	return db
}

// Close 关闭数据库连接
func Close() error {
	if db == nil {
		return nil
	}
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

// getDBPath 获取跨平台的数据库文件路径
func getDBPath() (string, error) {
	var baseDir string

	switch runtime.GOOS {
	case "windows":
		// %APPDATA%/cdpnetool/data.db
		baseDir = os.Getenv("APPDATA")
		if baseDir == "" {
			baseDir = filepath.Join(os.Getenv("USERPROFILE"), "AppData", "Roaming")
		}
	case "darwin":
		// ~/Library/Application Support/cdpnetool/data.db
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		baseDir = filepath.Join(home, "Library", "Application Support")
	default:
		// Linux: ~/.local/share/cdpnetool/data.db
		baseDir = os.Getenv("XDG_DATA_HOME")
		if baseDir == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", err
			}
			baseDir = filepath.Join(home, ".local", "share")
		}
	}

	return filepath.Join(baseDir, "cdpnetool", "data.db"), nil
}

// autoMigrate 自动迁移所有模型
func autoMigrate() error {
	return db.AutoMigrate(
		&Setting{},
		&RuleSetRecord{},
		&InterceptEventRecord{},
	)
}

// GetDBPath 导出获取数据库路径的方法（用于调试）
func GetDBPath() (string, error) {
	return getDBPath()
}
