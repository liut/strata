package cmd

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"
)

var version = "dev"
var logLevel string

// initSlog 配置 slog 输出格式，包含文件名和行号
func initSlog(levelStr string) {
	var level slog.Level
	switch levelStr {
	case "debug":
		level = slog.LevelDebug
	case "info":
		level = slog.LevelInfo
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	slog.SetLogLoggerLevel(level)
}

var RootCmd = &cobra.Command{
	Use:   "strata",
	Short: "strata - lightweight session sandbox service",
	Run: func(cmd *cobra.Command, args []string) {
		_ = cmd.Help()
	},
}

func Execute() {
	RootCmd.Version = version
	if err := RootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	RootCmd.AddCommand(runCmd)

	RootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "info", "log level: debug, info, warn, error")
	RootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		initSlog(logLevel)
		return nil
	}
}
