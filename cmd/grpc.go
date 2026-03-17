package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"

	"google.golang.org/grpc"

	"github.com/liut/strata/pkg/config"
	"github.com/liut/strata/pkg/sandbox"
	pb "github.com/liut/strata/pkg/proto/sandbox"
	"github.com/spf13/cobra"
)

var configPathGRPC string

var grpcCmd = &cobra.Command{
	Use:   "grpc",
	Short: "Start gRPC server",
	RunE:  runGRPC,
}

func init() {
	grpcCmd.Flags().StringVar(&configPathGRPC, "config", "configs/config.yaml", "path to config file")
}

func runGRPC(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(configPathGRPC)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	slog.Info("starting gRPC service")

	// 初始化 Manager
	driver := sandbox.OverlayDriver(cfg.Sandbox.OverlayDriver)
	manager := sandbox.NewManager(sandbox.ManagerConfig{
		SessionRoot: cfg.Sandbox.SessionRoot,
		BaseRootfs:  cfg.Sandbox.BaseRootfs,
		Driver:      driver,
		IsolateNet:  cfg.Sandbox.IsolateNetwork,
		TTL:         cfg.Sandbox.SessionTTL,
		MaxSessions: cfg.Sandbox.MaxSessions,
	})

	// 启动 gRPC
	lis, err := net.Listen("tcp", cfg.GRPC.Addr)
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}

	grpcServer := grpc.NewServer()
	pb.RegisterSandboxServiceServer(grpcServer, &grpcHandler{manager: manager})

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		slog.Info("gRPC server listening", "addr", cfg.GRPC.Addr)
		if err := grpcServer.Serve(lis); err != nil {
			slog.Error("gRPC serve error", "error", err)
			os.Exit(1)
		}
	}()

	<-quit
	slog.Info("shutting down...")
	grpcServer.GracefulStop()

	return nil
}

// ───────────────────────────────────────────────────────────
// gRPC Handler
// ───────────────────────────────────────────────────────────

type grpcHandler struct {
	manager *sandbox.Manager
	pb.UnimplementedSandboxServiceServer
}

func (h *grpcHandler) CreateSession(ctx context.Context, req *pb.CreateSessionRequest) (*pb.CreateSessionResponse, error) {
	sess, err := h.manager.GetOrCreate(req.UserId, req.SessionId)
	if err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}
	return &pb.CreateSessionResponse{
		UserId:    sess.UserID,
		SessionId: sess.ID,
		CreatedAt: sess.Created.Format("2006-01-02T15:04:05Z07:00"),
	}, nil
}

func (h *grpcHandler) CloseSession(ctx context.Context, req *pb.CloseSessionRequest) (*pb.CloseSessionResponse, error) {
	ok := h.manager.Close(req.UserId, req.SessionId)
	return &pb.CloseSessionResponse{
		Success: ok,
		Message:  "closed",
	}, nil
}

func (h *grpcHandler) Exec(ctx context.Context, req *pb.ExecRequest) (*pb.ExecResponse, error) {
	_, err := h.manager.GetOrCreate(req.UserId, req.SessionId)
	if err != nil {
		return nil, fmt.Errorf("get session: %w", err)
	}

	return &pb.ExecResponse{
		Error: "not implemented yet, use HTTP /api/exec",
	}, nil
}

func (h *grpcHandler) Shell(stream pb.SandboxService_ShellServer) error {
	first, err := stream.Recv()
	if err != nil {
		return err
	}

	uid := first.UserId
	sid := first.SessionId
	if uid == "" || sid == "" {
		return fmt.Errorf("user_id and session_id required in first message")
	}

	sess, err := h.manager.GetOrCreate(uid, sid)
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}

	errCh := make(chan error, 2)

	// PTY → gRPC stream
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := sess.Read(buf)
			if err != nil {
				errCh <- nil
				return
			}
			if err := stream.Send(&pb.ShellOutput{Data: buf[:n]}); err != nil {
				errCh <- err
				return
			}
		}
	}()

	// gRPC stream → PTY
	go func() {
		for {
			msg, err := stream.Recv()
			if err != nil {
				errCh <- nil
				return
			}
			if msg.Payload == nil {
				continue
			}
			switch p := msg.Payload.(type) {
			case *pb.ShellInput_StdinData:
				if _, err := sess.Write(p.StdinData); err != nil {
					errCh <- err
					return
				}
			case *pb.ShellInput_Resize:
				_ = sess.Resize(uint16(p.Resize.Rows), uint16(p.Resize.Cols))
			}
		}
	}()

	e := <-errCh
	if e != nil {
		slog.Error("shell stream error", "error", e)
	}
	return nil
}

func (h *grpcHandler) Stats(ctx context.Context, req *pb.StatsRequest) (*pb.StatsResponse, error) {
	stats := h.manager.Stats()
	return &pb.StatsResponse{
		ActiveSessions: int32(stats["active_sessions"]),
		MaxSessions:    100,
	}, nil
}
