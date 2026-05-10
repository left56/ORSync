package store

import (
	"context"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// DB 为 GORM SQLite 连接的薄包装，供业务层注入。
type DB struct {
	*gorm.DB
}

// FindMappingByOutline 按 Outline 文档 ID 查询映射；不存在时返回 (nil, nil)。
// 使用 Find + RowsAffected，避免 First 在无记录时触发 GORM 的 record not found 日志。
func (d *DB) FindMappingByOutline(ctx context.Context, outlineDocID string) (*SyncMapping, error) {
	if d == nil || d.DB == nil {
		return nil, fmt.Errorf("store: nil db")
	}
	var m SyncMapping
	tx := d.WithContext(ctx).Where("outline_doc_id = ?", outlineDocID).Limit(1).Find(&m)
	if tx.Error != nil {
		return nil, tx.Error
	}
	if tx.RowsAffected == 0 {
		return nil, nil
	}
	return &m, nil
}

// UpsertSyncMapping 插入或更新 sync_mapping（主键 outline_doc_id 冲突时更新 ragflow 与时间）。
func (d *DB) UpsertSyncMapping(ctx context.Context, outlineDocID, ragflowDocID string, at time.Time) error {
	if d == nil || d.DB == nil {
		return fmt.Errorf("store: nil db")
	}
	if outlineDocID == "" || ragflowDocID == "" {
		return fmt.Errorf("store: empty mapping id")
	}
	m := SyncMapping{
		OutlineDocID: outlineDocID,
		RagflowDocID: ragflowDocID,
		LastSyncTime: at.UTC(),
	}
	return d.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "outline_doc_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"ragflow_doc_id",
			"last_sync_time",
		}),
	}).Create(&m).Error
}

// DeleteMappingByOutline 按 Outline 文档 ID 物理删除映射行。
func (d *DB) DeleteMappingByOutline(ctx context.Context, outlineDocID string) error {
	if d == nil || d.DB == nil {
		return fmt.Errorf("store: nil db")
	}
	res := d.WithContext(ctx).Where("outline_doc_id = ?", outlineDocID).Delete(&SyncMapping{})
	if res.Error != nil {
		return res.Error
	}
	return nil
}

// InsertSyncLog 写入一条同步日志。
func (d *DB) InsertSyncLog(ctx context.Context, eventType, targetDocID, status, message string) error {
	if d == nil || d.DB == nil {
		return fmt.Errorf("store: nil db")
	}
	if eventType == "" || status == "" {
		return fmt.Errorf("store: empty log event_type or status")
	}
	log := SyncLog{
		EventType:   eventType,
		TargetDocID: targetDocID,
		Status:      status,
		Message:     message,
		CreatedAt:   time.Now().UTC(),
	}
	return d.WithContext(ctx).Create(&log).Error
}

// ListSyncLogs 按条件分页查询 sync_logs，按 created_at 降序；total 为过滤后的总行数。
func (d *DB) ListSyncLogs(ctx context.Context, page, pageSize int, status, eventType string) ([]SyncLog, int64, error) {
	if d == nil || d.DB == nil {
		return nil, 0, fmt.Errorf("store: nil db")
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	applyFilters := func(tx *gorm.DB) *gorm.DB {
		if strings.TrimSpace(status) != "" {
			tx = tx.Where("status = ?", strings.TrimSpace(status))
		}
		if strings.TrimSpace(eventType) != "" {
			tx = tx.Where("event_type = ?", strings.TrimSpace(eventType))
		}
		return tx
	}
	var total int64
	countTx := applyFilters(d.WithContext(ctx).Model(&SyncLog{}))
	if err := countTx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	offset := (page - 1) * pageSize
	var logs []SyncLog
	findTx := applyFilters(d.WithContext(ctx).Model(&SyncLog{}))
	err := findTx.Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&logs).Error
	if err != nil {
		return nil, 0, err
	}
	return logs, total, nil
}

// DeleteSyncLogsOlderThan 物理删除 created_at 早于 cutoff 的日志行。
func (d *DB) DeleteSyncLogsOlderThan(ctx context.Context, cutoff time.Time) (int64, error) {
	if d == nil || d.DB == nil {
		return 0, fmt.Errorf("store: nil db")
	}
	res := d.WithContext(ctx).Where("created_at < ?", cutoff.UTC()).Delete(&SyncLog{})
	return res.RowsAffected, res.Error
}
