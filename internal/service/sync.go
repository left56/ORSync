package service

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/zln/orsync/internal/outline"
	"github.com/zln/orsync/internal/ragflow"
	"github.com/zln/orsync/internal/store"
)

const defaultListPageSize = 100

// SyncService 编排 Outline 与 RAGFlow 之间的文档同步及本地映射、日志。
type SyncService struct {
	outline  *outline.Client
	ragflow  *ragflow.Client
	db       *store.DB
	pageSize int

	// allowedOutlineCollectionIDs 非空时仅同步这些 Outline collectionId（已排序）；Webhook 写路径亦按 collection 校验。
	allowedOutlineCollectionIDs []string
	allowedOutlineCollectionSet map[string]struct{}

	fullMu      sync.Mutex
	fullRunning bool

	bgTaskWG sync.WaitGroup
}

// NewSyncService 注入 Outline / RAGFlow 客户端与 SQLite 包装器。
// allowedOutlineCollectionIDs 非空时启用文档集白名单（值为 Outline 的 collection UUID）；nil 或空切片表示不限制。
func NewSyncService(oc *outline.Client, rc *ragflow.Client, db *store.DB, allowedOutlineCollectionIDs []string) (*SyncService, error) {
	if oc == nil {
		return nil, fmt.Errorf("service: outline client is nil")
	}
	if rc == nil {
		return nil, fmt.Errorf("service: ragflow client is nil")
	}
	if db == nil {
		return nil, fmt.Errorf("service: store db is nil")
	}
	set := make(map[string]struct{})
	var ids []string
	for _, id := range allowedOutlineCollectionIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := set[id]; ok {
			continue
		}
		set[id] = struct{}{}
		ids = append(ids, id)
	}
	sort.Strings(ids)
	var allowSet map[string]struct{}
	if len(ids) > 0 {
		allowSet = set
	}
	return &SyncService{
		outline:                     oc,
		ragflow:                     rc,
		db:                          db,
		pageSize:                    defaultListPageSize,
		allowedOutlineCollectionIDs: ids,
		allowedOutlineCollectionSet: allowSet,
	}, nil
}

// FullSync 遍历 Outline 文档（若配置了文档集白名单则仅遍历这些文档集），将每篇文档同步至 RAGFlow 并维护 sync_mapping；每篇写入 sync_logs（event_type: full_sync）。
func (s *SyncService) FullSync(ctx context.Context) error {
	if len(s.allowedOutlineCollectionIDs) == 0 {
		return s.fullSyncListWithOptionalCollection(ctx, "")
	}
	for _, cid := range s.allowedOutlineCollectionIDs {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := s.fullSyncListWithOptionalCollection(ctx, cid); err != nil {
			return err
		}
	}
	return nil
}

func (s *SyncService) fullSyncListWithOptionalCollection(ctx context.Context, collectionID string) error {
	offset := 0
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		list, err := s.outline.ListDocuments(ctx, offset, s.pageSize, collectionID)
		if err != nil {
			if collectionID == "" {
				return fmt.Errorf("full sync: list documents at offset %d: %w", offset, err)
			}
			return fmt.Errorf("full sync: list documents in collection %s at offset %d: %w", collectionID, offset, err)
		}
		if len(list.Data) == 0 {
			break
		}
		for _, item := range list.Data {
			if err := ctx.Err(); err != nil {
				return err
			}
			_ = s.syncOneFromOutline(ctx, item.ID, "full_sync")
		}
		offset += len(list.Data)
		if len(list.Data) < s.pageSize {
			break
		}
	}
	return nil
}

// RunProcessWebhookInBackground 在独立 goroutine 中执行 ProcessWebhook（使用传入的 ctx），并纳入 bgTaskWG 生命周期统计。
func (s *SyncService) RunProcessWebhookInBackground(ctx context.Context, eventType, outlineDocID string) {
	s.bgTaskWG.Add(1)
	go func() {
		defer s.bgTaskWG.Done()
		if err := s.ProcessWebhook(ctx, eventType, outlineDocID); err != nil {
			zap.L().Error("webhook_background_failed",
				zap.String("event", eventType),
				zap.String("outline_doc_id", outlineDocID),
				zap.Error(err),
			)
		}
	}()
}

// StartFullSyncAsync 在成功占用全量槽位后启动后台 FullSync；若已有全量在进行则返回 false。
func (s *SyncService) StartFullSyncAsync(ctx context.Context) bool {
	if !s.TryBeginFullSync() {
		return false
	}
	s.bgTaskWG.Add(1)
	go func(c context.Context) {
		defer s.bgTaskWG.Done()
		defer s.EndFullSync()
		if err := s.FullSync(c); err != nil {
			zap.L().Error("full_sync_background_failed", zap.Error(err))
		}
	}(ctx)
	return true
}

// Wait 阻塞直至所有通过 RunProcessWebhookInBackground / StartFullSyncAsync 启动的后台任务完成。
func (s *SyncService) Wait() {
	s.bgTaskWG.Wait()
}

// WaitWithTimeout 在超时时间内等待后台任务；若在时限内全部完成返回 true，否则返回 false 并打 Warn 日志。
func (s *SyncService) WaitWithTimeout(timeout time.Duration) bool {
	if timeout <= 0 {
		s.Wait()
		return true
	}
	done := make(chan struct{})
	go func() {
		s.bgTaskWG.Wait()
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-time.After(timeout):
		zap.L().Warn("background_tasks_wait_timeout", zap.Duration("timeout", timeout))
		return false
	}
}

// TryBeginFullSync 尝试占用全量同步槽位；若已有全量在进行则返回 false。
func (s *SyncService) TryBeginFullSync() bool {
	s.fullMu.Lock()
	defer s.fullMu.Unlock()
	if s.fullRunning {
		return false
	}
	s.fullRunning = true
	return true
}

// EndFullSync 释放全量同步槽位（应在全量任务结束时调用，通常配合 defer）。
func (s *SyncService) EndFullSync() {
	s.fullMu.Lock()
	defer s.fullMu.Unlock()
	s.fullRunning = false
}

// ProcessWebhook 根据 Outline Webhook 事件类型处理单篇文档，并写入 sync_logs（event_type: webhook_<原事件>）。
// 失败路径仍会写入 sync_logs，并返回非 nil error 供调用方（如同步 HTTP 或测试）感知。
func (s *SyncService) ProcessWebhook(ctx context.Context, eventType, outlineDocID string) error {
	eventType = strings.TrimSpace(eventType)
	outlineDocID = strings.TrimSpace(outlineDocID)
	logEvent := "webhook_" + eventType
	if outlineDocID == "" {
		s.logSync(ctx, logEvent, "", "failed", "empty outline document id")
		return fmt.Errorf("empty outline document id")
	}
	switch eventType {
	case "documents.create", "documents.update":
		if err := s.syncOneFromOutline(ctx, outlineDocID, logEvent); err != nil {
			return err
		}
	case "documents.delete":
		if err := s.handleWebhookDelete(ctx, outlineDocID, logEvent); err != nil {
			return err
		}
	default:
		msg := fmt.Sprintf("unsupported event type: %q", eventType)
		s.logSync(ctx, logEvent, outlineDocID, "failed", msg)
		return fmt.Errorf("%s", msg)
	}
	return nil
}

func (s *SyncService) handleWebhookDelete(ctx context.Context, outlineDocID, logEvent string) error {
	m, err := s.db.FindMappingByOutline(ctx, outlineDocID)
	if err != nil {
		s.logSync(ctx, logEvent, outlineDocID, "failed", fmt.Sprintf("load mapping: %v", err))
		return fmt.Errorf("load mapping: %w", err)
	}
	if m == nil {
		s.logSync(ctx, logEvent, outlineDocID, "success", "no local mapping; nothing to delete in RAGFlow")
		return nil
	}
	if err := s.ragflow.DeleteDocument(ctx, []string{m.RagflowDocID}); err != nil {
		s.logSync(ctx, logEvent, outlineDocID, "failed", fmt.Sprintf("ragflow delete: %v", err))
		return fmt.Errorf("ragflow delete: %w", err)
	}
	if err := s.db.DeleteMappingByOutline(ctx, outlineDocID); err != nil {
		s.logSync(ctx, logEvent, outlineDocID, "failed", fmt.Sprintf("delete mapping after ragflow delete: %v", err))
		return fmt.Errorf("delete mapping: %w", err)
	}
	s.logSync(ctx, logEvent, outlineDocID, "success", "deleted in RAGFlow and removed local mapping")
	return nil
}

// syncOneFromOutline 拉取 Outline 文档正文，按需删除 RAGFlow 旧文档后上传新版本并 Upsert 映射；始终写 sync_logs。
func (s *SyncService) syncOneFromOutline(ctx context.Context, outlineDocID, logEventType string) error {
	doc, err := s.outline.GetDocument(ctx, outlineDocID)
	if err != nil {
		s.logSync(ctx, logEventType, outlineDocID, "failed", fmt.Sprintf("outline get document: %v", err))
		return fmt.Errorf("outline get document: %w", err)
	}
	if !s.isOutlineCollectionAllowed(doc.CollectionID) {
		msg := fmt.Sprintf("skipped: document collection_id=%q not in configured Outline collection allowlist", strings.TrimSpace(doc.CollectionID))
		s.logSync(ctx, logEventType, outlineDocID, "success", msg)
		return nil
	}
	md := []byte(doc.Text)
	m, err := s.db.FindMappingByOutline(ctx, outlineDocID)
	if err != nil {
		s.logSync(ctx, logEventType, outlineDocID, "failed", fmt.Sprintf("load mapping: %v", err))
		return fmt.Errorf("load mapping: %w", err)
	}
	if m != nil {
		if err := s.ragflow.DeleteDocument(ctx, []string{m.RagflowDocID}); err != nil {
			s.logSync(ctx, logEventType, outlineDocID, "failed", fmt.Sprintf("ragflow delete old: %v", err))
			return fmt.Errorf("ragflow delete old: %w", err)
		}
	}
	up, err := s.ragflow.UploadDocument(ctx, doc.ID, doc.Title, md)
	if err != nil {
		s.logSync(ctx, logEventType, outlineDocID, "failed", fmt.Sprintf("ragflow upload: %v", err))
		return fmt.Errorf("ragflow upload: %w", err)
	}
	now := time.Now()
	if err := s.db.UpsertSyncMapping(ctx, outlineDocID, up.ID, now); err != nil {
		s.logSync(ctx, logEventType, outlineDocID, "failed", fmt.Sprintf("upsert mapping after upload: %v", err))
		return fmt.Errorf("upsert mapping: %w", err)
	}
	s.logSync(ctx, logEventType, outlineDocID, "success", fmt.Sprintf("ragflow_doc_id=%s", up.ID))
	return nil
}

func (s *SyncService) isOutlineCollectionAllowed(collectionID string) bool {
	if s.allowedOutlineCollectionSet == nil {
		return true
	}
	collectionID = strings.TrimSpace(collectionID)
	_, ok := s.allowedOutlineCollectionSet[collectionID]
	return ok
}

func (s *SyncService) logSync(ctx context.Context, eventType, targetDocID, status, message string) {
	if err := s.db.InsertSyncLog(ctx, eventType, targetDocID, status, message); err != nil {
		zap.L().Warn("insert_sync_log_failed",
			zap.String("event_type", eventType),
			zap.String("target_doc_id", targetDocID),
			zap.String("status", status),
			zap.Error(err),
		)
	}
}
