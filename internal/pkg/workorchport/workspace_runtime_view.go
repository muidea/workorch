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

// WorkspaceActivityKind is a domain-neutral activity classification for TUI.
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

// WorkspaceActivityItemInfo is one projected activity line for product display.
// Summaries are produced by workorchd; TUI only lays them out.
type WorkspaceActivityItemInfo struct {
	ID                   string
	WorkspaceID          common.WorkspaceID
	RunID                common.RunID
	StepID               common.StepID
	Kind                 WorkspaceActivityKind
	State                string
	Title                string
	Summary              string
	OccurredAt           string
	SourceEventRevision  int64
	RuntimeStateRevision uint64
	EvidenceSequence     uint64
}

// WorkspaceActivityFeedInfo is a bounded, ordered activity feed for a workspace.
type WorkspaceActivityFeedInfo struct {
	StreamGeneration string
	WorkspaceID      common.WorkspaceID
	DisplayRunID     common.RunID
	ActivityRevision int64
	Items            []WorkspaceActivityItemInfo
	Truncated        bool
	NextCursor       string
}

func WorkspaceActivityFeedInfoFromProjection(item projectioncontract.WorkspaceActivityFeed) WorkspaceActivityFeedInfo {
	return WorkspaceActivityFeedInfo{
		StreamGeneration: item.StreamGeneration,
		WorkspaceID:      common.WorkspaceID(item.WorkspaceID),
		DisplayRunID:     common.RunID(item.DisplayRunID),
		ActivityRevision: item.ActivityRevision,
		Items:            WorkspaceActivityItemInfosFromProjection(item.Items),
		Truncated:        item.Truncated,
		NextCursor:       item.NextCursor,
	}
}

func WorkspaceActivityItemInfosFromProjection(items []projectioncontract.WorkspaceActivityItem) []WorkspaceActivityItemInfo {
	if len(items) == 0 {
		return nil
	}
	ret := make([]WorkspaceActivityItemInfo, 0, len(items))
	for _, item := range items {
		ret = append(ret, WorkspaceActivityItemInfo{
			ID:                   item.ID,
			WorkspaceID:          common.WorkspaceID(item.WorkspaceID),
			RunID:                common.RunID(item.RunID),
			StepID:               common.StepID(item.StepID),
			Kind:                 WorkspaceActivityKind(item.Kind),
			State:                item.State,
			Title:                item.Title,
			Summary:              item.Summary,
			OccurredAt:           item.OccurredAt,
			SourceEventRevision:  item.SourceEventRevision,
			RuntimeStateRevision: item.RuntimeStateRevision,
			EvidenceSequence:     item.EvidenceSequence,
		})
	}
	return ret
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
	TaskProgress      TaskProgressInfo
	RunHeartbeat      RunHeartbeatInfo
	ConvergenceStatus *ConvergenceStatusInfo
	Acceptance        AcceptanceProjectionInfo
	RunCommand        *RunCommandStatusInfo
	BlockingDecision  *DecisionInfo
	BlockingApproval  *ApprovalInfo
	Outcome           WorkspaceOutcomeInfo
	Activity          WorkspaceActivityFeedInfo
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
