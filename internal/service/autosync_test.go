package service_test

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/byron1st/wharf/internal/pkg/atlassiansync"
	"github.com/byron1st/wharf/internal/pkg/mocks"
	"github.com/byron1st/wharf/internal/service"
)

var autoSyncStart = time.Date(2026, 10, 26, 5, 2, 0, 0, time.UTC)

type autoSyncFixture struct {
	svc      *mocks.MockService
	settings *mocks.MockSettings
	ctx      context.Context
	cancel   context.CancelFunc

	mu  sync.Mutex
	now time.Time
}

func newAutoSyncFixture(t *testing.T) *autoSyncFixture {
	t.Helper()

	ctx, cancel := context.WithCancel(t.Context())

	return &autoSyncFixture{
		svc:      mocks.NewMockService(t),
		settings: mocks.NewMockSettings(t),
		ctx:      ctx,
		cancel:   cancel,
		now:      autoSyncStart,
	}
}

func (f *autoSyncFixture) clock() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.now
}

func (f *autoSyncFixture) setClock(now time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.now = now
}

// run runs an AutoSyncer checking every millisecond until f.ctx ends, and
// fails the test when it does not end in time.
func (f *autoSyncFixture) run(t *testing.T) {
	t.Helper()

	a := service.NewAutoSyncer(service.AutoSyncerDeps{
		Service:  f.svc,
		Settings: f.settings,
		Every:    time.Millisecond,
		Now:      f.clock,
		Logger:   slog.New(slog.DiscardHandler),
	})

	done := make(chan struct{})

	go func() {
		a.Run(f.ctx)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		f.cancel()
		t.Fatal("auto syncer did not stop")
	}
}

// interval makes the interval getter return interval and stop the run on its
// call number stopAt, or never when stopAt is 0.
func (f *autoSyncFixture) interval(interval time.Duration, err error, stopAt int) {
	calls := 0

	f.settings.EXPECT().GetAtlassianSyncInterval(mock.Anything).
		Run(func(context.Context) {
			calls++
			if calls == stopAt {
				f.cancel()
			}
		}).
		Return(interval, err)
}

func (f *autoSyncFixture) configured(siteURL string) {
	f.settings.EXPECT().GetAtlassianSettings(mock.Anything).
		Return(service.AtlassianSettings{SiteURL: siteURL}).Maybe()
}

func (f *autoSyncFixture) lastSync(syncedAt time.Time, err error) {
	f.svc.EXPECT().GetLastAtlassianSync(mock.Anything).
		Return(atlassiansync.Record{SyncedAt: syncedAt}, err).Maybe()
}

func TestAutoSyncerSyncsWhenDue(t *testing.T) {
	tests := []struct {
		name     string
		syncedAt time.Time
		err      error
	}{
		{name: "interval passed", syncedAt: autoSyncStart.Add(-2 * time.Hour)},
		{name: "interval just passed", syncedAt: autoSyncStart.Add(-time.Hour)},
		{name: "never synced", err: service.ErrNoAtlassianSync},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newAutoSyncFixture(t)
			f.interval(time.Hour, nil, 0)
			f.configured(saved.SiteURL)
			f.lastSync(tt.syncedAt, tt.err)
			f.svc.EXPECT().SyncAtlassian(mock.Anything).
				Run(func(context.Context) { f.cancel() }).
				Return(service.AtlassianSync{}, nil).Once()

			f.run(t)
		})
	}
}

func TestAutoSyncerSkipsWhenNotDue(t *testing.T) {
	tests := []struct {
		name  string
		setup func(f *autoSyncFixture)
	}{
		{
			name: "interval not passed",
			setup: func(f *autoSyncFixture) {
				f.interval(time.Hour, nil, 3)
				f.configured(saved.SiteURL)
				f.lastSync(autoSyncStart.Add(-59*time.Minute), nil)
			},
		},
		{
			name: "automatic syncs off",
			setup: func(f *autoSyncFixture) {
				f.interval(0, nil, 3)
			},
		},
		{
			name: "atlassian not configured",
			setup: func(f *autoSyncFixture) {
				f.interval(time.Hour, nil, 3)
				f.configured("")
			},
		},
		{
			name: "interval unreadable",
			setup: func(f *autoSyncFixture) {
				f.interval(0, errors.New("db down"), 3)
			},
		},
		{
			name: "last sync unreadable",
			setup: func(f *autoSyncFixture) {
				f.interval(time.Hour, nil, 3)
				f.configured(saved.SiteURL)
				f.lastSync(time.Time{}, errors.New("db down"))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newAutoSyncFixture(t)
			tt.setup(f)

			// SyncAtlassian has no expectation, so a call fails the test.
			f.run(t)
		})
	}
}

func TestAutoSyncerRetriesFailedSyncAfterDelay(t *testing.T) {
	f := newAutoSyncFixture(t)
	retryAt := autoSyncStart.Add(5 * time.Minute)

	calls := 0

	f.settings.EXPECT().GetAtlassianSyncInterval(mock.Anything).
		Run(func(context.Context) {
			calls++
			// The first check fails; the next ones stay within the retry
			// delay, and the fifth reaches it.
			switch calls {
			case 3:
				f.setClock(retryAt.Add(-time.Second))
			case 5:
				f.setClock(retryAt)
			}
		}).
		Return(time.Hour, nil)
	f.configured(saved.SiteURL)
	f.lastSync(time.Time{}, service.ErrNoAtlassianSync)
	f.svc.EXPECT().SyncAtlassian(mock.Anything).
		Return(service.AtlassianSync{}, service.ErrAtlassianUnreachable).Once()

	var retriedAt time.Time

	f.svc.EXPECT().SyncAtlassian(mock.Anything).
		Run(func(context.Context) {
			retriedAt = f.clock()
			f.cancel()
		}).
		Return(service.AtlassianSync{}, nil).Once()

	f.run(t)

	require.Equal(t, retryAt, retriedAt)
}
