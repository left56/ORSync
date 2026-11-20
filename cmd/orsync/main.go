package main

import (
	"context"
	"errors"
	"flag"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"go.uber.org/zap"

	"github.com/zln/orsync/internal/config"
	"github.com/zln/orsync/internal/outline"
	"github.com/zln/orsync/internal/ragflow"
	"github.com/zln/orsync/internal/server"
	"github.com/zln/orsync/internal/service"
	"github.com/zln/orsync/internal/store"
	"github.com/zln/orsync/pkg/httpclient"
)

const (
	httpShutdownTimeout          = 5 * time.Second
	backgroundWaitTimeout        = 30 * time.Minute
	logCleanerExitTimeout        = 5 * time.Second
	resolveOutlineCollectionsTTL = 2 * time.Minute
)

func main() {
	configPath := flag.String("config", "", "配置文件路径（YAML）。为空则读取环境变量 ORSYNC_CONFIG_FILE；仍为空则在当前目录查找 config.yaml（可不存在，仅靠环境变量）")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		// Logger 尚未就绪，配置错误只能打印到 stderr。
		_, _ = os.Stderr.WriteString("failed to load config: " + err.Error() + "\n")
		os.Exit(1)
	}

	logger, err := zap.NewProduction()
	if err != nil {
		_, _ = os.Stderr.WriteString("failed to init logger: " + err.Error() + "\n")
		os.Exit(1)
	}
	defer func() { _ = logger.Sync() }()
	zap.ReplaceGlobals(logger)

	configSource := describeConfigSource(*configPath)
	zap.L().Info("orsync_starting",
		zap.String("config_source", configSource),
		zap.String("listen", cfg.Server.Addr),
		zap.String("database_path", cfg.Database.Path),
		zap.String("outline_base_url", cfg.Outline.BaseURL),
		zap.String("ragflow_base_url", cfg.RAGFlow.BaseURL),
	)

	db, err := store.Open(cfg.Database.Path)
	if err != nil {
		zap.L().Fatal("open_database_failed", zap.Error(err))
	}
	zap.L().Info("database_opened")

	httpClient := httpclient.New()

	oc, err := outline.NewClient(httpClient, cfg.Outline.BaseURL, cfg.Outline.APIKey)
	if err != nil {
		zap.L().Fatal("outline_client_failed", zap.Error(err))
	}
	zap.L().Info("outline_client_ready")

	var outlineCollectionIDs []string
	if want := outline.NormalizeCollectionNameList(cfg.Outline.CollectionNames); len(want) > 0 {
		resolveCtx, cancelResolve := context.WithTimeout(context.Background(), resolveOutlineCollectionsTTL)
		ids, err := outline.ResolveCollectionNamesToIDs(resolveCtx, oc, want)
		cancelResolve()
		if err != nil {
			zap.L().Fatal("outline_resolve_collection_names_failed", zap.Strings("collection_names", want), zap.Error(err))
		}
		outlineCollectionIDs = ids
		zap.L().Info("outline_collection_filter_enabled",
			zap.Strings("collection_names", want),
			zap.Strings("collection_ids", outlineCollectionIDs),
		)
	}

	rc, err := ragflow.NewClient(httpClient, cfg.RAGFlow.BaseURL, cfg.RAGFlow.APIKey, cfg.RAGFlow.DatasetID)
	if err != nil {
		zap.L().Fatal("ragflow_client_failed", zap.Error(err))
	}
	zap.L().Info("ragflow_client_ready")

	syncSvc, err := service.NewSyncService(oc, rc, db, outlineCollectionIDs)
	if err != nil {
		zap.L().Fatal("sync_service_failed", zap.Error(err))
	}
	zap.L().Info("sync_service_ready")

	cleanerCtx, cleanerStop := context.WithCancel(context.Background())
	cleanerDone := service.StartLogCleaner(cleanerCtx, db)
	zap.L().Info("log_cleaner_started")

	srv, err := server.NewServer(&cfg, db, syncSvc)
	if err != nil {
		cleanerStop()
		<-cleanerDone
		zap.L().Fatal("http_server_init_failed", zap.Error(err))
	}
	zap.L().Info("http_routes_registered")

	httpSrv := &http.Server{
		Addr:    cfg.Server.Addr,
		Handler: srv.Engine(),
	}

	go func() {
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			zap.L().Fatal("http_listen_failed", zap.Error(err))
		}
	}()
	zap.L().Info("http_listen_started", zap.String("addr", cfg.Server.Addr))

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(quit)
	<-quit

	zap.L().Info("http_shutdown_begin", zap.String("msg", "正在关闭 HTTP 服务，拒绝新请求..."))

	shutCtx, shutCancel := context.WithTimeout(context.Background(), httpShutdownTimeout)
	defer shutCancel()
	if err := httpSrv.Shutdown(shutCtx); err != nil {
		zap.L().Warn("http_shutdown_error", zap.Error(err))
	}

	zap.L().Info("background_sync_wait_begin", zap.String("msg", "正在等待后台同步任务完成..."))
	if ok := syncSvc.WaitWithTimeout(backgroundWaitTimeout); !ok {
		zap.L().Warn("background_sync_wait_incomplete", zap.Duration("timeout", backgroundWaitTimeout))
	}

	zap.L().Info("log_cleaner_stop_begin", zap.String("msg", "正在停止日志清理任务..."))
	cleanerStop()
	select {
	case <-cleanerDone:
	case <-time.After(logCleanerExitTimeout):
		zap.L().Warn("log_cleaner_exit_timeout", zap.Duration("timeout", logCleanerExitTimeout))
	}

	zap.L().Info("database_close_begin", zap.String("msg", "正在关闭数据库连接..."))
	if sqlDB, err := db.DB.DB(); err == nil {
		if err := sqlDB.Close(); err != nil {
			zap.L().Warn("database_close_error", zap.Error(err))
		}
	} else {
		zap.L().Warn("database_sql_handle_failed", zap.Error(err))
	}

	zap.L().Info("orsync_stopped", zap.String("msg", "服务已安全退出"))
}

func describeConfigSource(flagPath string) string {
	flagPath = strings.TrimSpace(flagPath)
	if flagPath != "" {
		return "flag -config=" + flagPath
	}
	if p := strings.TrimSpace(os.Getenv("ORSYNC_CONFIG_FILE")); p != "" {
		return "env ORSYNC_CONFIG_FILE=" + p
	}
	return "default ./config.yaml (optional) + env"
}
