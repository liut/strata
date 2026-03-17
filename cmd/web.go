package cmd

import (
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/liut/strata/pkg/webapi"
	"github.com/liut/strata/pkg/config"
	"github.com/liut/strata/pkg/sandbox"
	"github.com/spf13/cobra"
)

var configPath string

var webCmd = &cobra.Command{
	Use:   "web",
	Short: "Start HTTP server",
	RunE:  runWeb,
}

func init() {
	webCmd.Flags().StringVar(&configPath, "config", "configs/config.yaml", "path to config file")
}

// loggingMiddleware logs HTTP requests in Apache Combined Log Format
func loggingMiddleware(h http.Handler, logger *log.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// 使用 ResponseWriter 包装来获取状态码
		rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}

		h.ServeHTTP(rw, r)

		// 记录日志：IP - - [时间] "METHOD PATH" STATUS SIZE "REFERER" "USER-AGENT"
		logger.Printf("%s - - [%s] \"%s %s %s\" %d %d \"%s\" \"%s\"",
			remoteAddr(r),
			start.Format("02/Jan/2006:15:04:05 -0700"),
			r.Method,
			r.URL.Path,
			r.Proto,
			rw.status,
			rw.size,
			r.Referer(),
			r.UserAgent(),
		)
	})
}

// remoteAddr 获取真实客户端 IP（考虑代理）
func remoteAddr(r *http.Request) string {
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		return forwarded
	}
	return r.RemoteAddr
}

// responseWriter 包装 http.ResponseWriter 以捕获状态码和响应大小
type responseWriter struct {
	http.ResponseWriter
	status int
	size   int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	size, err := rw.ResponseWriter.Write(b)
	rw.size += size
	return size, err
}

func runWeb(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	slog.Info("starting strata", "version", version)
	slog.Info("lightweight session sandbox service")

	// 检查运行环境依赖
	driver := sandbox.OverlayDriver(cfg.Sandbox.OverlayDriver)
	env := sandbox.CurrentEnv()
	missing := env.CheckOverlay(driver)
	if len(missing) > 0 {
		slog.Warn("missing dependencies", "dependencies", missing)

		// 检查是否缺少 bwrap（所有模式都必需）
		if !env.HasBwrap {
			return fmt.Errorf("cannot start: bwrap is required for all modes")
		}

		// 缺少其他依赖，尝试降级到 DriverNone
		slog.Warn("falling back to DriverNone mode (no persistence)")
		driver = sandbox.DriverNone
	}

	// 确保 session 工作目录存在
	if err := os.MkdirAll(cfg.Sandbox.SessionRoot, 0700); err != nil {
		return fmt.Errorf("failed to create session root %s: %w", cfg.Sandbox.SessionRoot, err)
	}

	// 初始化 Session Manager
	manager := sandbox.NewManager(sandbox.ManagerConfig{
		SessionRoot: cfg.Sandbox.SessionRoot,
		BaseRootfs:  cfg.Sandbox.BaseRootfs,
		Driver:      driver,
		IsolateNet:  cfg.Sandbox.IsolateNetwork,
		TTL:         cfg.Sandbox.SessionTTL,
		MaxSessions: cfg.Sandbox.MaxSessions,
	})

	// 注册路由
	handler := webapi.NewHandler(manager)
	mux := handler.Routes()

	// 配置日志
	var accessLog *log.Logger
	if cfg.Server.AccessLog != "" {
		f, err := os.OpenFile(cfg.Server.AccessLog, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return fmt.Errorf("failed to open access log: %w", err)
		}
		accessLog = log.New(f, "", 0)
		// 启用日志时使用带日志的 handler
		mux = loggingMiddleware(mux, accessLog)
	} else {
		// 默认输出到 stdout
		accessLog = log.New(os.Stdout, "", 0)
		mux = loggingMiddleware(mux, accessLog)
	}

	httpServer := &http.Server{
		Addr:         cfg.Server.Addr,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	// 优雅退出
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		slog.Info("HTTP server listening", "addr", cfg.Server.Addr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("HTTP server error", "error", err)
			os.Exit(1)
		}
	}()

	<-quit
	slog.Info("shutting down...")
	httpServer.Close()
	slog.Info("bye")

	return nil
}
