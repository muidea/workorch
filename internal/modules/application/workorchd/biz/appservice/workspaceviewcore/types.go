package workspaceviewcore

import (
	"context"
	"time"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	eventstreamevents "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
)

const (
	StateBuilding = "building"
	StateReady    = "ready"
	StateStale    = "stale"
	StateDegraded = "degraded"
	StateFailed   = "failed"
	StateStopped  = "stopped"
)

// projectionLifecycleState is the Workorchd-owned stored lifecycle for its
// derived workspace projection. Public State* strings remain protocol values;
// only this private type may enter the projection lifecycle reducer.
type projectionLifecycleState string

// ProjectionFailure is a current-generation projection health fact.
// Cumulative metrics counters must not decide ready/degraded.
type ProjectionFailure struct {
	Generation string
	Error      string
	Since      time.Time
}

type Metrics struct {
	FullRebuildTotal                uint64
	FullRebuildFailedTotal          uint64
	FullRebuildDurationMsTotal      int64
	PendingWorkspaceRebuilds        int
	WorkspaceRebuildDebouncedTotal  uint64
	ScheduleDroppedTotal            uint64
	WorkspaceRebuilds               []WorkspaceRebuildMetrics
	ActiveRunHeartbeatTotal         int
	ActiveRunHeartbeatAgeSecondsMax int64
	StalledRunTotal                 int
	IneffectiveRetryRunTotal        int
	ConvergenceActiveSlices         int
	ConvergenceOpenFocuses          int
	ConvergenceRepairAttemptsTotal  int
	ConvergenceNoDeltaRoundsTotal   int
	ConvergenceBudgetEscalations    int
}

type WorkspaceRebuildMetrics struct {
	WorkspaceID     string
	RebuildTotal    uint64
	FailedTotal     uint64
	DurationMsTotal int64
}

type Loader interface {
	ListWorkspaces(context.Context) ([]appmodelcore.Workspace, error)
	ReadWorkspace(context.Context, string) (appmodelcore.Workspace, error)
	ReadWorkspaceSnapshot(context.Context, string) (readmodelcore.WorkspaceSnapshotState, error)
}

type Scheduler func(name string, fn func(context.Context)) bool

// EventPorts are owned by the application Biz. They keep EventHub activity
// outside the projection read-model manager.
type EventPorts struct {
	ReadHighWatermarks func(context.Context, []common.WorkspaceID) ([]eventstreamevents.WorkspaceHighWatermark, error)
	PublishChanged     func(context.Context, workorchdevents.WorkspaceProjectionChangedData)
}

type ManagerOptions struct {
	Loader               Loader
	Schedule             Scheduler
	RebuildCoalesceDelay time.Duration
	EventPorts           EventPorts
}
