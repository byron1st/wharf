package service

import (
	"context"
	"errors"
	"log/slog"
	"time"
)

// autoSyncRetryDelay is how long a failed automatic sync waits before the
// next try, so an unreachable or rejecting Atlassian is not called every
// check.
const autoSyncRetryDelay = 5 * time.Minute

type AutoSyncer interface {
	Run(ctx context.Context)
}

// AutoSyncerDeps are what NewAutoSyncer needs.
type AutoSyncerDeps struct {
	Service  Service
	Settings Settings
	// Every is the time between checks for a due sync.
	Every  time.Duration
	Now    func() time.Time
	Logger *slog.Logger
}

type autoSyncer struct {
	svc      Service
	settings Settings
	every    time.Duration
	now      func() time.Time
	logger   *slog.Logger

	// failedAt is when the latest automatic sync failed, zero when it did not.
	failedAt time.Time
}

// NewAutoSyncer returns the AutoSyncer that syncs Atlassian through
// deps.Service on the interval deps.Settings holds.
func NewAutoSyncer(deps AutoSyncerDeps) AutoSyncer {
	return &autoSyncer{
		svc:      deps.Service,
		settings: deps.Settings,
		every:    deps.Every,
		now:      deps.Now,
		logger:   deps.Logger,
	}
}

// Run checks now and then every deps.Every until ctx ends, and syncs
// Atlassian when automatic syncs are on, Atlassian is configured, and the
// interval has passed since the last recorded sync, manual or automatic, of
// any device. Before the first recorded sync one is due at once. A failed
// sync is logged and tried again after autoSyncRetryDelay at the earliest.
func (a *autoSyncer) Run(ctx context.Context) {
	ticker := time.NewTicker(a.every)
	defer ticker.Stop()

	for ctx.Err() == nil {
		a.syncIfDue(ctx)

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (a *autoSyncer) syncIfDue(ctx context.Context) {
	interval, err := a.settings.GetAtlassianSyncInterval(ctx)
	if err != nil {
		a.warn(ctx, "failed to get atlassian sync interval", err)
		return
	}

	if interval == 0 || a.settings.GetAtlassianSettings(ctx).SiteURL == "" {
		return
	}

	now := a.now()
	if !a.failedAt.IsZero() && now.Before(a.failedAt.Add(autoSyncRetryDelay)) {
		return
	}

	last, err := a.svc.GetLastAtlassianSync(ctx)
	if err != nil && !errors.Is(err, ErrNoAtlassianSync) {
		a.warn(ctx, "failed to get last atlassian sync", err)
		return
	}

	if err == nil && now.Before(last.SyncedAt.Add(interval)) {
		return
	}

	if _, err := a.svc.SyncAtlassian(ctx); err != nil {
		a.failedAt = now
		a.warn(ctx, "failed to sync atlassian automatically", err)

		return
	}

	a.failedAt = time.Time{}
}

// warn logs err unless ctx ended, since a check cut short by a shutdown says
// nothing about the sync.
func (a *autoSyncer) warn(ctx context.Context, msg string, err error) {
	if ctx.Err() == nil {
		a.logger.WarnContext(ctx, msg, "error", err)
	}
}
