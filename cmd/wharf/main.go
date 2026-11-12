package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"runtime"
	"slices"
	"strings"
	"syscall"

	"github.com/byron1st/wharf/internal/adapter/atlassian"
	"github.com/byron1st/wharf/internal/adapter/db"
	"github.com/byron1st/wharf/internal/adapter/github"
	"github.com/byron1st/wharf/internal/adapter/system"
	"github.com/byron1st/wharf/internal/config"
	"github.com/byron1st/wharf/internal/pkg/buildinfo"
	"github.com/byron1st/wharf/internal/pkg/httputil"
	"github.com/byron1st/wharf/internal/server"
	"github.com/byron1st/wharf/internal/service"
	"github.com/byron1st/wharf/scripts"
	"github.com/byron1st/wharf/web"
)

// usage lists the subcommands; without one, wharf runs the server.
const usage = `usage: wharf [service install | service uninstall]

  service install    run wharf in the background at login (macOS LaunchAgent)
  service uninstall  stop it and remove the LaunchAgent`

func main() {
	if len(os.Args) > 1 {
		os.Exit(command(os.Args[1:]))
	}

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

	cfg, err := config.LoadForServer(cfgPath)
	if err != nil {
		logger.Error("failed to load config", "error", err)
		return 1, ""
	}

	desktop := system.NewDesktop()

	dbPassword, err := desktop.DBPassword(ctx)
	if err != nil {
		logger.Error("failed to read database password", "error", err)
		return 1, ""
	}

	dbMng := db.NewManager(cfg.DBURL, dbPassword, scripts.Migrations())
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
		System: desktop,
		App: service.AppSettings{
			ConfigPath: cfgPath,
			Addr:       cfg.Addr,
			DBTarget:   cfg.DBTarget(),
			Version:    buildinfo.Version(),
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
	// The token is optional: without it the update check is unauthenticated.
	githubToken, err := desktop.GitHubToken(ctx)
	if err != nil {
		logger.Warn("failed to read GitHub token; checking for updates without it", "error", err)
	}

	updater := service.NewUpdater(service.UpdaterDeps{
		Releases: github.NewClient(githubToken),
		System:   desktop,
		Version:  buildinfo.Version(),
		GOOS:     runtime.GOOS,
		GOARCH:   runtime.GOARCH,
		Enabled:  cfg.UpdateCheck,
		Interval: cfg.UpdateCheckInterval,
		Restart: func() {
			restartWith = cfgPath

			stop()
		},
		Logger: logger,
	})
	srv := server.New(server.NewHandler(svc, settings, updater, web.Dist(), logger), logger)

	ln, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		logger.Error("failed to listen", "addr", cfg.Addr, "error", err)
		return 1, ""
	}

	logger.Info("server started", "addr", ln.Addr().String())

	go updater.Run(ctx)

	if err := srv.Run(ctx, ln); err != nil {
		logger.Error("failed to run server", "error", err)
		return 1, ""
	}

	logger.Info("server stopped")

	return 0, restartWith
}

// command runs the subcommand in args and returns the exit code.
func command(args []string) int {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	desktop := system.NewDesktop()
	ctx := context.Background()

	var err error

	switch strings.Join(args, " ") {
	case "service install":
		// The agent keeps the config file this shell would use.
		err = desktop.InstallLaunchAgent(ctx, os.Getenv(config.PathEnv))
	case "service uninstall":
		err = desktop.UninstallLaunchAgent(ctx)
	default:
		_, _ = fmt.Fprintln(os.Stderr, usage)
		return 2
	}

	if err != nil {
		logger.Error("failed to run command", "command", strings.Join(args, " "), "error", err)
		return 1
	}

	return 0
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
