package workorchport

import (
	"context"

	"github.com/muidea/workorch/internal/pkg/common"
)

type CreateWorkspaceRequest struct {
	Title                string
	Description          string
	DeliverableDirectory string
	TemplateID           string
}

type WorkspaceInfo struct {
	ID                   common.WorkspaceID
	Title                string
	Description          string
	DeliverableDirectory string
	State                common.WorkspaceState
	ActiveRunID          common.RunID
	TemplateID           string
	ProfileID            string
	DefaultSoulID        common.SoulID
	SoulBindings         map[string]common.SoulID
}

type WorkspaceOutcomeInfo struct {
	WorkspaceID  common.WorkspaceID
	State        string
	RunID        common.RunID
	ResultID     common.ResultID
	Accepted     bool
	Terminal     bool
	Summary      string
	Reason       string
	EvidenceRefs []string
	ArtifactRefs []string
}

type WorkspaceSnapshotInfo struct {
	Workspace             WorkspaceInfo
	Outcome               WorkspaceOutcomeInfo
	TaskProgress          TaskProgressInfo
	RunHeartbeat          RunHeartbeatInfo
	ContextBundle         map[string]any
	CoverageSummary       map[string]any
	TaskDecompositions    []TaskDecompositionInfo
	Goals                 []GoalInfo
	DoDs                  []DoDInfo
	Runs                  []RunInfo
	Results               []ResultInfo
	Artifacts             []ArtifactInfo
	ArtifactContinuations []ArtifactContinuationInfo
	Patches               []PatchInfo
	MemoryRecallTraces    []MemoryRecallTraceInfo
	WebSources            []WebSourceTraceInfo
	LLMUsageTraces        []LLMUsageTraceInfo
	LLMUsageSummary       LLMUsageSummaryInfo
	EventRecords          []EventRecordInfo
	Decisions             []DecisionInfo
	Approvals             []ApprovalInfo
}

type RunHeartbeatInfo struct {
	WorkspaceID               common.WorkspaceID
	RunID                     common.RunID
	State                     string
	ActivityState             string
	LatestStepID              common.StepID
	LatestStepKind            string
	LatestStepState           string
	LatestEvidenceRef         string
	LatestEventTopic          string
	LatestEventRevision       int64
	LatestEventAt             string
	LatestLLMUsageAt          string
	LatestResultAt            string
	LatestArtifactAt          string
	LatestPatchAt             string
	LatestEffectiveProgressAt string
	IneffectiveRetry          bool
	Stalled                   bool
	Summary                   string
}

type WorkspaceBackend interface {
	Create(ctx context.Context, req CreateWorkspaceRequest) (*WorkspaceInfo, error)
	Get(ctx context.Context, id common.WorkspaceID) (*WorkspaceInfo, error)
	List(ctx context.Context) ([]WorkspaceInfo, error)
	Snapshot(ctx context.Context, id common.WorkspaceID) (*WorkspaceSnapshotInfo, error)
	Delete(ctx context.Context, id common.WorkspaceID, purge bool) error
}
