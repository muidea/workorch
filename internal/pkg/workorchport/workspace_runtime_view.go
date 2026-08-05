package workorchport

import (
	"context"

	"github.com/muidea/workorch/internal/pkg/common"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

// WorkspaceProjectionSection identifies a changed workspace projection section.
// EventHub/SSE notifications only carry these section markers; the RuntimeView
// remains the authority for displayed state.
type WorkspaceProjectionSection string

const (
	WorkspaceProjectionSectionOverview        WorkspaceProjectionSection = "overview"
	WorkspaceProjectionSectionRuntimeProgress WorkspaceProjectionSection = "runtime_progress"
	WorkspaceProjectionSectionActivity        WorkspaceProjectionSection = "activity"
	WorkspaceProjectionSectionGovernance      WorkspaceProjectionSection = "governance"
	WorkspaceProjectionSectionEvidence        WorkspaceProjectionSection = "evidence"
	WorkspaceProjectionSectionResult          WorkspaceProjectionSection = "result"
	WorkspaceProjectionSectionArtifacts       WorkspaceProjectionSection = "artifacts"
	WorkspaceProjectionSectionMemory          WorkspaceProjectionSection = "memory"
	WorkspaceProjectionSectionLLMUsage        WorkspaceProjectionSection = "llm_usage"
)

// WorkspaceProjectionChangedInfo is the typed projection-invalidation notice
// shared by local EventHub and remote SSE backends.
type WorkspaceProjectionChangedInfo struct {
	ID              string
	WorkspaceID     common.WorkspaceID
	RunID           common.RunID
	Revision        projectioncontract.WorkspaceRefreshRevision
	ChangedSections []WorkspaceProjectionSection
	ProjectionState string
	UpdatedAt       string
}

func CloneWorkspaceProjectionChangedInfo(item WorkspaceProjectionChangedInfo) WorkspaceProjectionChangedInfo {
	item.ChangedSections = append([]WorkspaceProjectionSection(nil), item.ChangedSections...)
	return item
}

// WorkspaceRuntimeViewInfo is the lightweight runtime main-view contract used
// for active refresh and history recovery after workspace switch/reconnect.
type WorkspaceRuntimeViewInfo struct {
	WorkspaceID       common.WorkspaceID
	ActiveRunID       common.RunID
	DisplayRunID      common.RunID
	Projection        projectioncontract.ProjectionStatus
	Revision          projectioncontract.WorkspaceRefreshRevision
	PromptMode        string
	RunState          common.RunState
	TaskProgress      projectioncontract.TaskProgress
	RunHeartbeat      projectioncontract.RunHeartbeat
	ConvergenceStatus *projectioncontract.ConvergenceStatus
	Acceptance        projectioncontract.AcceptanceProjection
	RunCommand        *projectioncontract.RunCommandStatus
	BlockingDecision  *projectioncontract.Decision
	BlockingApproval  *projectioncontract.Approval
	Outcome           projectioncontract.WorkspaceOutcome
	Activity          projectioncontract.WorkspaceActivityFeed
}

func CloneWorkspaceRuntimeViewInfo(item WorkspaceRuntimeViewInfo) WorkspaceRuntimeViewInfo {
	item.Projection = CloneProjectionStatus(item.Projection)
	item.TaskProgress = projectioncontract.CloneTaskProgress(item.TaskProgress)
	item.ConvergenceStatus = projectioncontract.CloneConvergenceStatus(item.ConvergenceStatus)
	item.Acceptance = projectioncontract.CloneAcceptanceProjection(item.Acceptance)
	item.RunCommand = projectioncontract.CloneRunCommandStatus(item.RunCommand)
	item.BlockingDecision = cloneDecisionPtr(item.BlockingDecision)
	item.BlockingApproval = cloneApprovalPtr(item.BlockingApproval)
	item.Outcome = projectioncontract.CloneWorkspaceOutcome(item.Outcome)
	item.Activity = projectioncontract.CloneWorkspaceActivityFeed(item.Activity)
	return item
}

// ReadWorkspaceRuntimeViewRequest is the port-level RuntimeView read request.
type ReadWorkspaceRuntimeViewRequest struct {
	WorkspaceID   common.WorkspaceID
	ActivityLimit int
	BeforeCursor  string
}

// SubscriptionState describes EventHub/SSE subscription health.
type SubscriptionState string

const (
	SubscriptionConnecting   SubscriptionState = "connecting"
	SubscriptionLive         SubscriptionState = "live"
	SubscriptionReconnecting SubscriptionState = "reconnecting"
	SubscriptionDegraded     SubscriptionState = "degraded"
)

// SubscriptionStatusInfo is the typed subscription health view for TUI status.
type SubscriptionStatusInfo struct {
	State            SubscriptionState
	StreamGeneration string
	StreamRevision   int64
	LastEventAt      string
	LastError        string
}

// WorkspaceRuntimeViewReader is the stable RuntimeView read surface for TUI/CLI.
// Implemented by WorkspaceBackend.
type WorkspaceRuntimeViewReader interface {
	RuntimeView(ctx context.Context, req ReadWorkspaceRuntimeViewRequest) (*WorkspaceRuntimeViewInfo, error)
}

// WorkspaceProjectionChangedHandler receives typed projection invalidation notices.
// TUI must not receive generic Event payloads on this path.
type WorkspaceProjectionChangedHandler func(ctx context.Context, info WorkspaceProjectionChangedInfo)

// SubscriptionStatusHandler receives typed EventHub/SSE health updates.
type SubscriptionStatusHandler func(ctx context.Context, status SubscriptionStatusInfo)

// WorkspaceProjectionEventsBackend is the TUI projection subscription surface.
// It is intentionally separate from the generic EventsBackend so raw owner
// event streams cannot re-enter the TUI main view.
type WorkspaceProjectionEventsBackend interface {
	SubscribeWorkspaceProjection(
		ctx context.Context,
		onChanged WorkspaceProjectionChangedHandler,
		onStatus SubscriptionStatusHandler,
	) (unsubscribe func(), err error)
}
