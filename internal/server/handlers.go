package server

import (
	"context"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func (s *Server) handleOutlineWebhook(c *gin.Context) {
	var body OutlineWebhookPayload
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	event := strings.TrimSpace(body.Event)
	docID := strings.TrimSpace(body.Data.ID)
	if event == "" || docID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing event or data.id"})
		return
	}

	s.syncSvc.RunProcessWebhookInBackground(context.Background(), event, docID)

	c.JSON(http.StatusOK, gin.H{"ok": true})
}

type listLogsQuery struct {
	Page      int    `form:"page"`
	PageSize  int    `form:"page_size"`
	Status    string `form:"status"`
	EventType string `form:"event_type"`
}

const maxLogPageSize = 100

func (s *Server) handleListLogs(c *gin.Context) {
	var q listLogsQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	page := q.Page
	if page < 1 {
		page = 1
	}
	pageSize := q.PageSize
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > maxLogPageSize {
		pageSize = maxLogPageSize
	}
	status := strings.TrimSpace(q.Status)
	if status != "" && status != "success" && status != "failed" {
		c.JSON(http.StatusBadRequest, gin.H{"error": `status must be "success", "failed", or empty`})
		return
	}
	eventType := strings.TrimSpace(q.EventType)

	logs, total, err := s.db.ListSyncLogs(c.Request.Context(), page, pageSize, status, eventType)
	if err != nil {
		zap.L().Error("list_sync_logs_failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":      logs,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

func (s *Server) handleFullSync(c *gin.Context) {
	if !s.syncSvc.StartFullSyncAsync(context.Background()) {
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "Sync already in progress"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Full sync started in background"})
}

func (s *Server) handleRoot(c *gin.Context) {
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(rootDashboardHTML))
}

// RegisterRoutes 将 HTTP 路由注册到 Engine（便于测试复用同一 Engine）。
func (s *Server) RegisterRoutes() {
	s.engine.GET("/", s.handleRoot)
	s.engine.POST("/webhook/outline", s.handleOutlineWebhook)
	s.engine.GET("/api/logs", s.handleListLogs)
	s.engine.POST("/api/sync/full", s.handleFullSync)
}
