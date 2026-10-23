package main

import (
	"context"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"github.com/byron1st/wharf/internal/adapter/atlassian"
	"github.com/byron1st/wharf/internal/adapter/db"
	"github.com/byron1st/wharf/internal/pkg/tool"
	"github.com/byron1st/wharf/internal/server"
	"github.com/byron1st/wharf/internal/service"
	"github.com/byron1st/wharf/scripts"
)

func main() {
	os.Exit(run())
}

func run() (code int) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	manager := db.NewManager(getenv("WHARF_DB_PATH", "wharf.db"), scripts.Migrations())
	if err := manager.Connect(ctx); err != nil {
		logger.Error("failed to connect database", "error", err)
		return 1
	}

	defer func() {
		if err := manager.Close(); err != nil {
			logger.Error("failed to close database", "error", err)

			code = 1
		}
	}()

	if err := manager.Migrate(ctx); err != nil {
		logger.Error("failed to migrate database", "error", err)
		return 1
	}

	acliPath := os.Getenv("WHARF_ACLI_PATH")
	if acliPath == "" {
		// tool.NewTool does not search PATH, so resolve the bare name here.
		path, err := exec.LookPath("acli")
		if err != nil {
			logger.Error("failed to find acli; set WHARF_ACLI_PATH", "error", err)
			return 1
		}

		acliPath = path
	}

	acli, err := tool.NewTool(acliPath)
	if err != nil {
		logger.Error("failed to create acli tool", "error", err)
		return 1
	}

	svc := service.NewService(atlassian.NewClient(acli), manager)
	srv := server.New(server.NewHandler(svc, logger), logger)

	addr := getenv("WHARF_ADDR", ":8080")

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		logger.Error("failed to listen", "addr", addr, "error", err)
		return 1
	}

	logger.Info("server started", "addr", ln.Addr().String())

	if err := srv.Run(ctx, ln); err != nil {
		logger.Error("failed to run server", "error", err)
		return 1
	}

	logger.Info("server stopped")

	return 0
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}

	return fallback
}
