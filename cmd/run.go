package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/liut/strata/pkg/config"
	"github.com/liut/strata/pkg/mcp"
	"github.com/liut/strata/pkg/rpc"
	"github.com/liut/strata/pkg/sandbox"
	"github.com/liut/strata/pkg/webapi"
	"github.com/mark3labs/mcp-go/server"
	"github.com/soheilhy/cmux"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
)

var runCmd = &cobra.Command{
	Use:     "run",
	Short:   "Start all services (HTTP + gRPC + MCP)",
	Aliases: []string{"web", "serve"},
	RunE:    runAll,
}

func runAll(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// 设置全局配置
	config.Current = cfg
	cfg.Version = version

	slog.Info("starting strata", "version", version)
	slog.Info("lightweight session sandbox service")

	// 检查运行环境依赖
	driver := sandbox.OverlayDriver(cfg.Sandbox.OverlayDriver)
	env := sandbox.CurrentEnv()
	missing := env.CheckOverlay(driver)
	if len(missing) > 0 {
		slog.Warn("missing dependencies", "dependencies", missing)
		if !env.HasBwrap {
			return fmt.Errorf("bwrap is required")
		}
		driver = sandbox.DriverNone
	}

	// 确保 session 工作目录存在
	if err := os.MkdirAll(cfg.Sandbox.SessionRoot, 0700); err != nil {
		return fmt.Errorf("failed to create session root: %w", err)
	}

	// 初始化共享的 Session Manager
	manager := sandbox.NewManager(sandbox.ManagerConfig{
		SessionRoot: cfg.Sandbox.SessionRoot,
		BaseRootfs:  cfg.Sandbox.BaseRootfs,
		Driver:      driver,
		IsolateNet:  cfg.Sandbox.IsolateNetwork,
		TTL:         cfg.Sandbox.SessionTTL,
		MaxSessions: cfg.Sandbox.MaxSessions,
	})

	// 启动服务
	addr := cfg.Server.Addr
	if addr == "" {
		addr = ":2280"
	}
	return serveAll(addr, manager)
}

func serveAll(addr string, manager *sandbox.Manager) error {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}

	cm := cmux.New(lis)
	httpL := cm.Match(cmux.HTTP1())
	grpcL := cm.Match(cmux.HTTP2())

	// HTTP 服务
	httpMux := http.NewServeMux()
	handler := webapi.NewHandler(manager)
	handler.Register(httpMux)

	// MCP 服务
	mcpServer := server.NewMCPServer("strata", version)
	mcpHandler := mcp.NewHandler(manager)
	mcp.SetupTools(mcpServer, mcpHandler)
	streamableHTTPServer := server.NewStreamableHTTPServer(mcpServer)
	httpMux.Handle("/mcp/", streamableHTTPServer)

	httpServer := &http.Server{Handler: loggingMiddleware(httpMux, slog.Default())}

	// gRPC 服务
	grpcServer := grpc.NewServer()
	rpcSvc := rpc.NewService(manager)
	rpcSvc.Register(grpcServer)

	// 使用 errgroup 管理服务
	g := new(errgroup.Group)

	// HTTP 服务
	g.Go(func() error {
		return httpServer.Serve(httpL)
	})

	// gRPC 服务
	g.Go(func() error {
		return grpcServer.Serve(grpcL)
	})

	// cmux 监听
	g.Go(func() error {
		return cm.Serve()
	})

	slog.Info("services started", "addr", lis.Addr().String(),
		"http", "REST+WS+MCP",
		"grpc", "enabled")

	// 等待退出信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// 等待任意一个服务退出或收到信号
	errCh := make(chan error, 1)
	go func() {
		errCh <- g.Wait()
	}()

	select {
	case <-quit:
		slog.Info("shutting down...")
	case err := <-errCh:
		if err != nil {
			slog.Error("service error", "error", err)
		}
	}

	// 关闭所有 session
	manager.CloseAll()

	// 立即停止 HTTP 服务
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	_ = httpServer.Shutdown(shutdownCtx)
	shutdownCancel()

	// 等待 gRPC 服务退出（带超时）
	gracefulDone := make(chan struct{})
	go func() {
		grpcServer.GracefulStop()
		close(gracefulDone)
	}()

	select {
	case <-gracefulDone:
	case <-time.After(5 * time.Second):
		slog.Warn("grpc shutdown timeout, forcing stop")
		// 用独立 goroutine 调用 Stop，避免卡住
		// go grpcServer.Stop()
	}

	slog.Info("all services stopped")
	return nil
}
