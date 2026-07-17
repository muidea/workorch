package workspaceviewcore

import (
	"context"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
)

const (
	StateBuilding = "building"
	StateReady    = "ready"
	StateStale    = "stale"
	StateDegraded = "degraded"
	StateFailed   = "failed"
	StateStopped  = "stopped"
)

type Status struct {
	State                   string   `json:"state"`
	StreamGeneration        string   `json:"streamGeneration,omitempty"`
	GlobalRevision          int64    `json:"globalRevision"`
	WorkspaceRevision       int64    `json:"workspaceRevision,omitempty"`
	SourceEventRevision     int64    `json:"sourceEventRevision"`
	ProjectedSourceRevision int64    `json:"projectedSourceRevision"`
	WorkspaceTotal          int      `json:"workspaceTotal"`
	UpdatedAt               string   `json:"updatedAt,omitempty"`
	RebuiltAt               string   `json:"rebuiltAt,omitempty"`
	Degraded                bool     `json:"degraded,omitempty"`
	StaleReason             string   `json:"staleReason,omitempty"`
	Error                   string   `json:"error,omitempty"`
	FailedWorkspaceIds      []string `json:"failedWorkspaceIds,omitempty"`
	DegradedSince           string   `json:"degradedSince,omitempty"`
}

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

type RunCommandAcceptedUpdate struct {
	WorkspaceID        string
	RunID              string
	Intent             string
	Reason             string
	RequestID          string
	IdempotencyKey     string
	Source             string
	DefinitionRevision string
	DecisionID         string
	Revision           int64
	AcceptedAt         string
}

type RunCommandRejectedUpdate struct {
	WorkspaceID        string
	RunID              string
	Intent             string
	Reason             string
	RequestID          string
	IdempotencyKey     string
	Source             string
	DefinitionRevision string
	DecisionID         string
	Revision           int64
	RejectedAt         string
	Error              string
}

type Loader interface {
	ListWorkspaces(context.Context) ([]appmodelcore.Workspace, error)
	ReadWorkspace(context.Context, string) (appmodelcore.Workspace, error)
	ReadWorkspaceSnapshot(context.Context, string) (readmodelcore.WorkspaceSnapshot, error)
}

type Scheduler func(name string, fn func(context.Context)) bool

type ManagerOptions struct {
	Loader               Loader
	CommandHub           magicevent.Hub
	Schedule             Scheduler
	RebuildCoalesceDelay time.Duration
}
