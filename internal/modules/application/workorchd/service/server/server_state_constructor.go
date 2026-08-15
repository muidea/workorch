package server

import (
	"context"

	"github.com/muidea/magicCommon/task"
)

// newServerStateBase creates process-local transport state around one typed
// application dependency set. It has no owner transition logic.
func newServerStateBase(config Config, backgroundRoutine task.BackgroundRoutine, operations applicationOperations) *serverState {
	backgroundCtx, backgroundCancel := context.WithCancel(context.Background())
	diagnostics := newSchedulerDiagnosticsStore()
	state := &serverState{
		config:                      config,
		eventReplay:                 newSSEReplayBuffer(defaultSSEReplayLimit),
		streamSubscribers:           make(map[uint64]chan streamEvent),
		metrics:                     &serverMetrics{schedulerDiagnostics: diagnostics, observedTerminalStates: make(map[string]struct{}), observedRepairAttempts: make(map[string]int), observedRuntimeDrives: make(map[string]runtimeDriveMetric)},
		applicationOperations:       operations,
		backgroundRoutine:           backgroundRoutine,
		backgroundCtx:               backgroundCtx,
		backgroundCancel:            backgroundCancel,
		schedulerDiagnostics:        diagnostics,
		workspaceSetupDraftSessions: map[string]*workspaceSetupDraftSession{},
	}
	return state
}
