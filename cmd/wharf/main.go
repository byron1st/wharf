package main

import (
	"context"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/byron1st/wharf/internal/adapter/atlassian"
	"github.com/byron1st/wharf/internal/adapter/db"
	"github.com/byron1st/wharf/internal/config"
	"github.com/byron1st/wharf/internal/pkg/httputil"
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

	cfgPath, err := config.Path()
	if err != nil {
		logger.Error("failed to find config file", "error", err)
		return 1
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		logger.Error("failed to load config", "error", err)
		return 1
	}

	manager := db.NewManager(cfg.DBPath, scripts.Migrations())
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

	jira := atlassian.NewClient(
		httputil.NewHTTPClient(),
		cfg.AtlassianSiteURL,
		cfg.AtlassianEmail,
		cfg.AtlassianAPIToken,
	)
	svc := service.NewService(jira, manager)
	srv := server.New(server.NewHandler(svc, logger), logger)

	ln, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		logger.Error("failed to listen", "addr", cfg.Addr, "error", err)
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
