package main

import (
	"context"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"runtime/debug"
	"slices"
	"strings"
	"syscall"

	"github.com/byron1st/wharf/internal/adapter/atlassian"
	"github.com/byron1st/wharf/internal/adapter/db"
	"github.com/byron1st/wharf/internal/adapter/system"
	"github.com/byron1st/wharf/internal/config"
	"github.com/byron1st/wharf/internal/pkg/httputil"
	"github.com/byron1st/wharf/internal/server"
	"github.com/byron1st/wharf/internal/service"
	"github.com/byron1st/wharf/scripts"
	"github.com/byron1st/wharf/web"
)

func main() {
	code, restartWith := run()
	if restartWith != "" {
		code = restart(restartWith)
	}

	os.Exit(code)
}

// run serves until a signal or a restart request. restartWith is the config
// path to restart with, or empty to exit with code.
func run() (code int, restartWith string) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfgPath, err := config.Path()
	if err != nil {
		logger.Error("failed to find config file", "error", err)
		return 1, ""
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		logger.Error("failed to load config", "error", err)
		return 1, ""
	}

	dbMng := db.NewManager(cfg.DBPath, scripts.Migrations())
	if err := dbMng.Connect(ctx); err != nil {
		logger.Error("failed to connect database", "error", err)
		return 1, ""
	}

	defer func() {
		if err := dbMng.Close(); err != nil {
			logger.Error("failed to close database", "error", err)

			code, restartWith = 1, ""
		}
	}()

	if err := dbMng.Migrate(ctx); err != nil {
		logger.Error("failed to migrate database", "error", err)
		return 1, ""
	}

	atls := atlassian.NewClient(
		httputil.NewHTTPClient(),
		cfg.AtlassianSiteURL,
		cfg.AtlassianEmail,
		cfg.AtlassianAPIToken,
	)
	svc := service.NewService(atls, dbMng)
	settings := service.NewSettings(service.SettingsDeps{
		Jira:   atls,
		System: system.NewDesktop(),
		App: service.AppSettings{
			ConfigPath: cfgPath,
			Addr:       cfg.Addr,
			DBPath:     cfg.DBPath,
			Version:    version(),
		},
		Atlassian: service.AtlassianSettings{
			SiteURL:  cfg.AtlassianSiteURL,
			Email:    cfg.AtlassianEmail,
			APIToken: cfg.AtlassianAPIToken,
		},
		Restart: func(configPath string) {
			restartWith = configPath

			stop()
		},
	})
	srv := server.New(server.NewHandler(svc, settings, web.Dist(), logger), logger)

	ln, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		logger.Error("failed to listen", "addr", cfg.Addr, "error", err)
		return 1, ""
	}

	logger.Info("server started", "addr", ln.Addr().String())

	if err := srv.Run(ctx, ln); err != nil {
		logger.Error("failed to run server", "error", err)
		return 1, ""
	}

	logger.Info("server stopped")

	return 0, restartWith
}

// restart replaces the process with a new run of the same executable that
// reads configPath. It returns only when that fails.
func restart(configPath string) int {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	exe, err := os.Executable()
	if err != nil {
		logger.Error("failed to find executable to restart", "error", err)
		return 1
	}

	// A duplicate would not override: the first entry of a name wins.
	env := slices.DeleteFunc(os.Environ(), func(kv string) bool { return strings.HasPrefix(kv, config.PathEnv+"=") })
	env = append(env, config.PathEnv+"="+configPath)
	logger.Info("restarting server", "config", configPath)

	err = syscall.Exec(exe, os.Args, env)
	logger.Error("failed to restart server", "error", err)

	return 1
}

// version is the module version of the binary, "(devel)" for a local build.
func version() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		return info.Main.Version
	}

	return "(devel)"
}
