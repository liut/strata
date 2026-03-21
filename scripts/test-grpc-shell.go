// test-grpc-shell.go - 测试 gRPC Shell 双向流
//
// 用法: go run test-grpc-shell.go [host:port]
//
// 注意: 需要先生成 pb 代码
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	pb "github.com/liut/strata/pkg/proto/sandbox"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var (
	addr   = flag.String("addr", "localhost:2280", "server address")
	userID = flag.String("user", "testuser", "user id")
	sessID = flag.String("session", "", "session id")
)

func main() {
	flag.Parse()

	if *sessID == "" {
		*sessID = fmt.Sprintf("shell-%d", os.Getpid())
	}

	fmt.Printf("=== gRPC Shell Test ===\n")
	fmt.Printf("Server: %s\n", *addr)
	fmt.Printf("User: %s\n", *userID)
	fmt.Printf("Session: %s\n", *sessID)
	fmt.Println()

	conn, err := grpc.NewClient(*addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("failed to connect: %v", err)
	}
	defer conn.Close()

	client := pb.NewSandboxServiceClient(conn)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 测试 CreateSession
	fmt.Println("--- Test: CreateSession ---")
	createResp, err := client.CreateSession(ctx, &pb.CreateSessionRequest{
		OwnerID:   *userID,
		SessionID: *sessID,
	})
	if err != nil {
		log.Printf("CreateSession error: %v", err)
	} else {
		fmt.Printf("Created: %s/%s\n", createResp.OwnerID, createResp.SessionID)
	}

	// 测试 Exec
	fmt.Println("\n--- Test: Exec ---")
	execResp, err := client.Exec(ctx, &pb.ExecRequest{
		OwnerID:   *userID,
		SessionID: *sessID,
		Command:   "echo 'hello from grpc exec' && pwd",
		TimeoutMs: 10000,
	})
	if err != nil {
		log.Printf("Exec error: %v", err)
	} else {
		fmt.Printf("Output: %s\n", execResp.Output)
		if execResp.Error != "" {
			fmt.Printf("Error: %s\n", execResp.Error)
		}
	}

	// 测试 Stats
	fmt.Println("\n--- Test: Stats ---")
	statsResp, err := client.Stats(ctx, &pb.StatsRequest{})
	if err != nil {
		log.Printf("Stats error: %v", err)
	} else {
		fmt.Printf("Active: %d, Max: %d\n", statsResp.ActiveSessions, statsResp.MaxSessions)
	}

	// 测试 Shell 双向流
	fmt.Println("\n--- Test: Shell (streaming) ---")
	fmt.Println("Starting interactive shell... (type 'exit' to quit)")

	stream, err := client.Shell(ctx)
	if err != nil {
		log.Printf("Shell error: %v", err)
		return
	}

	// 发送第一个消息（包含 owner_id/session_id）
	if err := stream.Send(&pb.ShellInput{
		OwnerID:   *userID,
		SessionID: *sessID,
	}); err != nil {
		log.Printf("Send initial error: %v", err)
		return
	}

	var wg sync.WaitGroup
	wg.Add(2)

	// goroutine: 接收服务端输出
	go func() {
		defer wg.Done()
		commandsSent := false
		for {
			resp, err := stream.Recv()
			if err == io.EOF {
				fmt.Println("\nStream closed by server")
				return
			}
			if err != nil {
				fmt.Printf("\nRecv error: %v\n", err)
				return
			}
			if resp.Closed {
				fmt.Println("\nSession closed")
				return
			}
			if resp.Error != "" {
				fmt.Printf("Error: %s\n", resp.Error)
			}
			fmt.Print(string(resp.Data))

			// 收到第一行输出后，发送演示命令
			if !commandsSent {
				commandsSent = true
				commands := []string{
					"pwd\n",
					"ls -la\n",
					"echo 'Hello from strata' > test.txt\n",
					"cat test.txt\n",
					"mkdir -p demoDir\n",
					"touch demoDir/file1.txt demoDir/file2.txt\n",
					"ls -la demoDir/\n",
					"rm test.txt\n",
					"rm -rf demoDir\n",
					"ls -la\n",
					"whoami\n",
					"date\n",
					"echo 'All demos completed!'\n",
				}
				go func() {
					time.Sleep(300 * time.Millisecond)
					for _, cmd := range commands {
						if err := stream.Send(&pb.ShellInput{Payload: &pb.ShellInput_StdinData{StdinData: []byte(cmd)}}); err != nil {
							fmt.Printf("Send error: %v\n", err)
							return
						}
						time.Sleep(200 * time.Millisecond)
					}
				}()
			}
		}
	}()

	// goroutine: 发送用户输入
	go func() {
		defer wg.Done()
		defer cancel()

		// 发送初始命令
		_ = stream.Send(&pb.ShellInput{Payload: &pb.ShellInput_StdinData{StdinData: []byte("echo 'hello from shell stream'\n")}})

		// 等待信号或 context 取消退出
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-sigChan:
				fmt.Println("\nExiting...")
				return
			case <-ctx.Done():
				fmt.Println("\nContext cancelled, exiting...")
				return
			case <-ticker.C:
				// 这里可以添加定时命令
			}
		}
	}()

	// 等待命令执行完成
	time.Sleep(5 * time.Second)
	cancel()
	wg.Wait()

	// 测试 CloseSession
	fmt.Println("\n--- Test: CloseSession ---")
	closeResp, err := client.CloseSession(ctx, &pb.CloseSessionRequest{
		OwnerID:   *userID,
		SessionID: *sessID,
	})
	if err != nil {
		log.Printf("CloseSession error: %v", err)
	} else {
		fmt.Printf("Closed: %v\n", closeResp.Success)
	}

	fmt.Println("\n=== Done ===")
}
