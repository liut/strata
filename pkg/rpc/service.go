package rpc

import (
	"context"
	"fmt"
	"time"

	pb "github.com/liut/strata/pkg/proto/sandbox"
	"github.com/liut/strata/pkg/sandbox"
	"github.com/liut/strata/pkg/webapi"
	"google.golang.org/grpc"
)

// Service 实现 gRPC 服务
type Service struct {
	manager *sandbox.Manager
	pb.UnimplementedSandboxServiceServer
}

func NewService(manager *sandbox.Manager) *Service {
	return &Service{manager: manager}
}

func (s *Service) Register(grpcServer *grpc.Server) {
	pb.RegisterSandboxServiceServer(grpcServer, s)
}

func (s *Service) CreateSession(ctx context.Context, req *pb.CreateSessionRequest) (*pb.CreateSessionResponse, error) {
	sess, err := s.manager.GetOrCreate(req.OwnerID, req.SessionID)
	if err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}

	return &pb.CreateSessionResponse{
		OwnerID:   sess.UID(),
		SessionID: sess.ID(),
		CreatedAt: sess.Created().Format("2006-01-02T15:04:05Z07:00"),
	}, nil
}

func (s *Service) CloseSession(ctx context.Context, req *pb.CloseSessionRequest) (*pb.CloseSessionResponse, error) {
	ok := s.manager.Close(req.OwnerID, req.SessionID)
	return &pb.CloseSessionResponse{Success: ok}, nil
}

func (s *Service) Exec(ctx context.Context, req *pb.ExecRequest) (*pb.ExecResponse, error) {
	sess, err := s.manager.GetOrCreate(req.OwnerID, req.SessionID)
	if err != nil {
		return &pb.ExecResponse{Error: err.Error()}, nil
	}

	timeout := 30000
	if req.TimeoutMs > 0 {
		timeout = int(req.TimeoutMs)
	}

	output, err := webapi.ExecInSession(sess, req.Command, time.Duration(timeout)*time.Millisecond)
	if err != nil {
		return &pb.ExecResponse{Error: err.Error()}, nil
	}

	return &pb.ExecResponse{Output: output}, nil
}

func (s *Service) Stats(ctx context.Context, req *pb.StatsRequest) (*pb.StatsResponse, error) {
	stats := s.manager.Stats()
	return &pb.StatsResponse{
		ActiveSessions: int32(stats["active_sessions"]),
		MaxSessions:    100,
	}, nil
}

// Shell 双向流
func (s *Service) Shell(stream pb.SandboxService_ShellServer) error {
	ctx := stream.Context()
	first, err := stream.Recv()
	if err != nil {
		return err
	}

	uid := first.OwnerID
	sid := first.SessionID
	if uid == "" || sid == "" {
		return fmt.Errorf("user_id and session_id required in first message")
	}

	sess, err := s.manager.GetOrCreate(uid, sid)
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}

	done := make(chan struct{})

	// PTY → gRPC stream
	go func() {
		buf := make([]byte, 4096)
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			default:
			}
			n, err := sess.Read(buf)
			if err != nil {
				return
			}
			if err := stream.Send(&pb.ShellOutput{Data: buf[:n]}); err != nil {
				return
			}
		}
	}()

	// gRPC stream → PTY
	go func() {
		defer close(done) // 通知发送端退出
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			msg, err := stream.Recv()
			if err != nil {
				return
			}
			if msg.Payload == nil {
				continue
			}
			switch p := msg.Payload.(type) {
			case *pb.ShellInput_StdinData:
				_, _ = sess.Write(p.StdinData)
			case *pb.ShellInput_Resize:
				_ = sess.Resize(uint16(p.Resize.Rows), uint16(p.Resize.Cols))
			}
		}
	}()

	// 等待任一 goroutine 退出
	<-done
	return nil
}
