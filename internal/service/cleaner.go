package service

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/zln/orsync/internal/store"
)

const logCleanerInterval = 24 * time.Hour

// StartLogCleaner 在独立 goroutine 中每 24 小时清理一次 sync_logs 中早于 7 天的记录；ctx 取消时退出，避免泄漏。
// 启动后会立即执行一次清理，再进入 Ticker 周期。返回的 channel 在清理 goroutine 退出后关闭，便于主流程等待收尾。
func StartLogCleaner(ctx context.Context, db *store.DB) <-chan struct{} {
	done := make(chan struct{})
	if db == nil {
		close(done)
		return done
	}
	go func() {
		defer close(done)
		runLogCleaner(ctx, db)
	}()
	return done
}

func runLogCleaner(ctx context.Context, db *store.DB) {
	ticker := time.NewTicker(logCleanerInterval)
	defer ticker.Stop()

	clean := func() {
		cutoff := time.Now().Add(-7 * 24 * time.Hour)
		n, err := db.DeleteSyncLogsOlderThan(context.Background(), cutoff)
		if err != nil {
			zap.L().Warn("sync_logs_cleanup_failed", zap.Error(err))
			return
		}
		if n > 0 {
			zap.L().Info("sync_logs_cleanup_done", zap.Int64("deleted_rows", n))
		}
	}

	clean()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			clean()
		}
	}
}
