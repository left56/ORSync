package store

import "time"

// SyncMapping：Outline 文档 ID 与 RAGFlow 文档 ID 映射表。
type SyncMapping struct {
	OutlineDocID string    `gorm:"column:outline_doc_id;primaryKey"`
	RagflowDocID string    `gorm:"column:ragflow_doc_id;not null"`
	LastSyncTime time.Time `gorm:"column:last_sync_time"`
}

func (SyncMapping) TableName() string {
	return "sync_mapping"
}

// SyncLog：同步事件日志表。
type SyncLog struct {
	ID          uint      `json:"id" gorm:"column:id;primaryKey;autoIncrement"`
	EventType   string    `json:"event_type" gorm:"column:event_type;not null"`
	TargetDocID string    `json:"target_doc_id" gorm:"column:target_doc_id"`
	Status      string    `json:"status" gorm:"column:status;not null"`
	Message     string    `json:"message" gorm:"column:message;type:text"`
	CreatedAt   time.Time `json:"created_at" gorm:"column:created_at;index"`
}

func (SyncLog) TableName() string {
	return "sync_logs"
}
