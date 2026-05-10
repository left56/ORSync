package server

import (
	"fmt"

	"github.com/gin-gonic/gin"

	"github.com/zln/orsync/internal/config"
	"github.com/zln/orsync/internal/service"
	"github.com/zln/orsync/internal/store"
)

// Server 承载 Gin 引擎与业务依赖。
type Server struct {
	engine  *gin.Engine
	cfg     *config.Config
	db      *store.DB
	syncSvc *service.SyncService
}

// NewServer 构造 Server 并注册路由；cfg / db / syncSvc 均不可为 nil。
func NewServer(cfg *config.Config, db *store.DB, syncSvc *service.SyncService) (*Server, error) {
	if cfg == nil {
		return nil, fmt.Errorf("server: nil config")
	}
	if db == nil {
		return nil, fmt.Errorf("server: nil store db")
	}
	if syncSvc == nil {
		return nil, fmt.Errorf("server: nil sync service")
	}
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())

	s := &Server{
		engine:  r,
		cfg:     cfg,
		db:      db,
		syncSvc: syncSvc,
	}
	s.RegisterRoutes()
	return s, nil
}

// Engine 返回底层 *gin.Engine。
func (s *Server) Engine() *gin.Engine {
	return s.engine
}

// Run 在指定地址上启动 HTTP 服务（阻塞）。
func (s *Server) Run(addr string) error {
	return s.engine.Run(addr)
}
