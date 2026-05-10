package store

import (
	"fmt"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// Open 打开 SQLite 数据库并完成 GORM AutoMigrate（sync_mapping、sync_logs），返回包装后的 *DB。
func Open(dsnPath string) (*DB, error) {
	if dsnPath == "" {
		return nil, fmt.Errorf("store: empty database path")
	}
	gdb, err := gorm.Open(sqlite.Open(dsnPath), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if err := gdb.AutoMigrate(&SyncMapping{}, &SyncLog{}); err != nil {
		return nil, fmt.Errorf("auto migrate: %w", err)
	}
	return &DB{DB: gdb}, nil
}
