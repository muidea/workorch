package readmodelcore

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"

// WorkspaceActivityKind is the internal projection activity kind.
// Port layer maps this 1:1 onto workorchport.WorkspaceActivityKind.
type WorkspaceActivityKind string

const (
	WorkspaceActivityRuntimeTransition WorkspaceActivityKind = "runtime_transition"
	WorkspaceActivityDispatch          WorkspaceActivityKind = "dispatch"
	WorkspaceActivityEvidence          WorkspaceActivityKind = "evidence"
	WorkspaceActivityGovernance        WorkspaceActivityKind = "governance"
	WorkspaceActivityArtifact          WorkspaceActivityKind = "artifact"
	WorkspaceActivityPatch             WorkspaceActivityKind = "patch"
	WorkspaceActivityMemory            WorkspaceActivityKind = "memory"
	WorkspaceActivityResult            WorkspaceActivityKind = "result"
	WorkspaceActivityFailure           WorkspaceActivityKind = "failure"
)

// WorkspaceActivityItem is one map-free activity projection item.
type WorkspaceActivityItem struct {
	ID                   string                `json:"id"`
	WorkspaceID          string                `json:"workspaceId"`
	RunID                string                `json:"runId,omitempty"`
	StepID               string                `json:"stepId,omitempty"`
	Kind                 WorkspaceActivityKind `json:"kind"`
	State                string                `json:"state,omitempty"`
	Title                string                `json:"title,omitempty"`
	Summary              string                `json:"summary,omitempty"`
	OccurredAt           string                `json:"occurredAt,omitempty"`
	SourceEventRevision  int64                 `json:"sourceEventRevision,omitempty"`
	RuntimeStateRevision uint64                `json:"runtimeStateRevision,omitempty"`
	EvidenceSequence     uint64                `json:"evidenceSequence,omitempty"`
}

// WorkspaceActivityFeed is a bounded activity feed for RuntimeView.
type WorkspaceActivityFeed struct {
	// StreamGeneration identifies the projection-store process that assigned the
	// feed revision. It is internal transport metadata, not a product activity.
	StreamGeneration string                  `json:"streamGeneration,omitempty"`
	WorkspaceID      string                  `json:"workspaceId"`
	DisplayRunID     string                  `json:"displayRunId,omitempty"`
	ActivityRevision int64                   `json:"activityRevision,omitempty"`
	Items            []WorkspaceActivityItem `json:"items,omitempty"`
	Truncated        bool                    `json:"truncated,omitempty"`
	NextCursor       string                  `json:"nextCursor,omitempty"`
}

// WorkspaceRefreshRevision is the internal multi-domain revision set.
type WorkspaceRefreshRevision struct {
	StreamGeneration        string `json:"streamGeneration,omitempty"`
	StreamRevision          int64  `json:"streamRevision,omitempty"`
	WorkspaceRevision       int64  `json:"workspaceRevision,omitempty"`
	ProjectedSourceRevision int64  `json:"projectedSourceRevision,omitempty"`
	RuntimeStateRevision    uint64 `json:"runtimeStateRevision,omitempty"`
	EvidenceSequence        uint64 `json:"evidenceSequence,omitempty"`
	ActivityRevision        int64  `json:"activityRevision,omitempty"`
}

// WorkspaceRuntimeView is the internal lightweight runtime main-view projection.
type WorkspaceRuntimeView struct {
	WorkspaceID       string                          `json:"workspaceId"`
	ActiveRunID       string                          `json:"activeRunId,omitempty"`
	DisplayRunID      string                          `json:"displayRunId,omitempty"`
	Projection        appmodelcore.WorkspaceViewModel `json:"projection"`
	Revision          WorkspaceRefreshRevision        `json:"revision"`
	PromptMode        string                          `json:"promptMode,omitempty"`
	RunState          string                          `json:"runState,omitempty"`
	TaskProgress      TaskProgress                    `json:"taskProgress"`
	RunHeartbeat      RunHeartbeat                    `json:"runHeartbeat"`
	ConvergenceStatus *ConvergenceStatus              `json:"convergenceStatus,omitempty"`
	Acceptance        AcceptanceProjection            `json:"acceptance"`
	RunCommand        *RunCommandStatus               `json:"runCommand,omitempty"`
	BlockingDecision  *Decision                       `json:"blockingDecision,omitempty"`
	BlockingApproval  *Approval                       `json:"blockingApproval,omitempty"`
	Outcome           WorkspaceOutcome                `json:"outcome"`
	Activity          WorkspaceActivityFeed           `json:"activity"`
}
