// cli.go - Interactive CLI shell
//
// Usage: strata cli [--addr host:port] [--user userid] [--session sessionid]
package cmd

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"

	pb "github.com/liut/strata/pkg/proto/sandbox"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var (
	cliAddr   string
	cliUser   string
	cliSession string
)

var cliCmd = &cobra.Command{
	Use:   "cli",
	Short: "Interactive shell client",
	Long:  `Connect to strata server and start an interactive shell session.`,
	RunE:  runCLI,
}

func init() {
	RootCmd.AddCommand(cliCmd)

	cliCmd.Flags().StringVar(&cliAddr, "addr", "localhost:2280", "server address")
	cliCmd.Flags().StringVar(&cliUser, "user", "testuser", "user id")
	cliCmd.Flags().StringVar(&cliSession, "session", "", "session id (auto-generated if empty)")
}

func runCLI(cmd *cobra.Command, args []string) error {
	if cliSession == "" {
		cliSession = fmt.Sprintf("cli-%d", os.Getpid())
	}

	fmt.Printf("=== Strata CLI ===\n")
	fmt.Printf("Server: %s\n", cliAddr)
	fmt.Printf("User: %s\n", cliUser)
	fmt.Printf("Session: %s\n", cliSession)
	fmt.Println("Type 'exit' or Ctrl+C to quit")

	conn, err := grpc.NewClient(cliAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	defer conn.Close()

	client := pb.NewSandboxServiceClient(conn)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create session first
	_, err = client.CreateSession(ctx, &pb.CreateSessionRequest{
		UserId:    cliUser,
		SessionId: cliSession,
	})
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}

	// Start shell stream
	stream, err := client.Shell(ctx)
	if err != nil {
		return fmt.Errorf("failed to start shell: %w", err)
	}

	// Send initial message with session info
	if err := stream.Send(&pb.ShellInput{
		UserId:    cliUser,
		SessionId: cliSession,
	}); err != nil {
		return fmt.Errorf("failed to send initial message: %w", err)
	}

	var wg sync.WaitGroup
	wg.Add(2)

	// Goroutine: receive server output
	go func() {
		defer wg.Done()
		for {
			resp, err := stream.Recv()
			if err == io.EOF {
				fmt.Println("\nConnection closed by server")
				return
			}
			if err != nil {
				fmt.Fprintf(os.Stderr, "\nConnection error: %v\n", err)
				return
			}
			if resp.Closed {
				fmt.Println("\nSession closed")
				return
			}
			if resp.Error != "" {
				fmt.Fprintf(os.Stderr, "Error: %s\n", resp.Error)
			}
			fmt.Print(string(resp.Data))
		}
	}()

	// Goroutine: send user input
	go func() {
		defer wg.Done()
		defer cancel()

		reader := bufio.NewReader(os.Stdin)

		for {
			// Read user input line by line
			line, err := reader.ReadString('\n')
			if err != nil {
				if err.Error() == "unexpected EOF" || err == io.EOF {
					fmt.Println("\nExiting...")
					return
				}
				fmt.Fprintf(os.Stderr, "Read error: %v\n", err)
				return
			}

			// Handle escape character for line continuation
			line = strings.TrimRight(line, "\r\n")
			var input strings.Builder
			input.WriteString(line)

			for strings.HasSuffix(input.String(), "\\") {
				fmt.Print("> ")
				nextLine, err := reader.ReadString('\n')
				if err != nil {
					break
				}
				nextLine = strings.TrimRight(nextLine, "\r\n")
				input.WriteString("\n")
				input.WriteString(nextLine)
			}

			userInput := input.String()

			// Check for exit command
			trimmed := strings.TrimSpace(userInput)
			if trimmed == "exit" || trimmed == "quit" || trimmed == "logout" {
				fmt.Println("Exiting...")
				return
			}

			// Send to server
			if err := stream.Send(&pb.ShellInput{
				Payload: &pb.ShellInput_StdinData{StdinData: []byte(userInput + "\n")},
			}); err != nil {
				fmt.Fprintf(os.Stderr, "Send error: %v\n", err)
				return
			}
		}
	}()

	// Handle signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		fmt.Println("\nInterrupt received, exiting...")
		cancel()
	}()

	wg.Wait()

	// Close session
	_, _ = client.CloseSession(ctx, &pb.CloseSessionRequest{
		UserId:    cliUser,
		SessionId: cliSession,
	})

	return nil
}
