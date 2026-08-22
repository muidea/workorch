package server

import (
	"context"
	"sync"
	"time"

	"github.com/muidea/magicCommon/task"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
)

// Mutable HTTP adapter state is kept in one package-private type. This is
// transport/application-module state only; it does not own Runtime/Run
// lifecycle transitions or rebuild owner authority.

type serverState struct {
	config                      Config
	workspaceExecutionAdmission WorkspaceExecutionAdmission
	eventReplay                 *sseReplayBuffer
	streamSubscribersMu         sync.Mutex
	streamSubscribers           map[uint64]chan streamEvent
	streamSubscriberSequence    uint64
	metrics                     *serverMetrics
	applicationOperations
	backgroundRoutine           task.BackgroundRoutine
	backgroundWG                sync.WaitGroup
	backgroundCtx               context.Context
	backgroundCancel            context.CancelFunc
	runStatusNotifyMu           sync.Mutex
	runStatusNotify             chan struct{}
	eventUnsubscribes           []func()
	workspaceProjection         WorkspaceProjection
	workspaceProjectionWriter   WorkspaceProjectionWriter
	runSnapshotCacheMu          sync.Mutex
	runSnapshotEntries          map[string]*runSnapshotCacheEntry
	workspaceSetupDraftMu       sync.Mutex
	workspaceSetupDraftSessions map[string]*workspaceSetupDraftSession
}

type cachedRunSnapshot struct {
	snapshot readmodelcore.RunSnapshotState
	cachedAt time.Time
}

type runSnapshotCacheEntry struct {
	cached   cachedRunSnapshot
	hasCache bool
	inflight *runSnapshotInflight
}

type runSnapshotInflight struct {
	done     chan struct{}
	snapshot readmodelcore.RunSnapshotState
	err      error
}
