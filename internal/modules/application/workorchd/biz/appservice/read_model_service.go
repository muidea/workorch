package appservice

import (
	"context"
	"errors"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/muidea/workorch/internal/modules/application/workorchd/pkg/observed"
	"github.com/muidea/workorch/internal/modules/application/workorchd/pkg/presenter"
	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	eventstreammodel "github.com/muidea/workorch/internal/modules/kernel/eventstream/pkg/models"
	mcpmodel "github.com/muidea/workorch/internal/modules/kernel/mcp/pkg/models"
	memorymodel "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/models"
	resultmodel "github.com/muidea/workorch/internal/modules/kernel/result/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
)

type RunSnapshot struct {
	Workspace            Workspace                `json:"workspace"`
	Run                  Run                      `json:"run"`
	Outcome              RunOutcome               `json:"outcome"`
	TaskProgress         TaskProgress             `json:"taskProgress"`
	TaskDecompositions   []TaskDecomposition      `json:"taskDecompositions,omitempty"`
	Step                 *presenter.StepView      `json:"step,omitempty"`
	Steps                []presenter.StepView     `json:"steps,omitempty"`
	ToolCalls            []ToolCall               `json:"toolCalls,omitempty"`
	Patches              []Patch                  `json:"patches,omitempty"`
	ValidationResults    []ValidationResult       `json:"validationResults,omitempty"`
	RuntimeCheckpoints   []RuntimeCheckpoint      `json:"runtimeCheckpoints,omitempty"`
	Results              []Result                 `json:"results,omitempty"`
	Artifacts            []Artifact               `json:"artifacts,omitempty"`
	ExecutionSections    []ExecutionSection       `json:"executionSections,omitempty"`
	MemoryRecallTraces   []MemoryRecallTrace      `json:"memoryRecallTraces,omitempty"`
	SelectedCapabilities []RuntimeCapability      `json:"selectedCapabilities,omitempty"`
	UsedCapabilities     []RuntimeCapability      `json:"usedCapabilities,omitempty"`
	EventRecords         []EventRecord            `json:"eventRecords,omitempty"`
	Decisions            []Decision               `json:"decisions,omitempty"`
	Approvals            []Approval               `json:"approvals,omitempty"`
	Observed             []presenter.ObservedView `json:"observed,omitempty"`
}

type RunStatus struct {
	ID                 string `json:"id"`
	WorkspaceID        string `json:"workspaceId"`
	State              string `json:"state"`
	CurrentStepID      string `json:"currentStepId,omitempty"`
	CurrentStepKind    string `json:"currentStepKind,omitempty"`
	CurrentStepState   string `json:"currentStepState,omitempty"`
	StepCount          int    `json:"stepCount"`
	DecisionCount      int    `json:"decisionCount"`
	ApprovalCount      int    `json:"approvalCount"`
	BlockingTicketID   string `json:"blockingTicketId,omitempty"`
	BlockingApprovalID string `json:"blockingApprovalId,omitempty"`
	FailureReason      string `json:"failureReason,omitempty"`
}

type RunObservability struct {
	Run                  Run                    `json:"run"`
	Outcome              RunOutcome             `json:"outcome"`
	Progress             RunProgress            `json:"progress"`
	TaskProgress         TaskProgress           `json:"taskProgress"`
	TaskDecompositions   []TaskDecomposition    `json:"taskDecompositions,omitempty"`
	ExecutionSections    []ExecutionSection     `json:"executionSections,omitempty"`
	MemoryRecallTraces   []MemoryRecallTrace    `json:"memoryRecallTraces,omitempty"`
	Evidence             RuntimeEvidenceSummary `json:"evidence"`
	LatestCheckpoint     *RuntimeCheckpoint     `json:"latestCheckpoint,omitempty"`
	SelectedCapabilities []RuntimeCapability    `json:"selectedCapabilities,omitempty"`
	UsedCapabilities     []RuntimeCapability    `json:"usedCapabilities,omitempty"`
	Timeline             []RuntimeTimelineItem  `json:"timeline,omitempty"`
}

type TaskProgress struct {
	WorkspaceID      string              `json:"workspaceId"`
	RunID            string              `json:"runId,omitempty"`
	State            string              `json:"state"`
	Summary          string              `json:"summary,omitempty"`
	CurrentMilestone string              `json:"currentMilestone,omitempty"`
	Milestones       []ProgressMilestone `json:"milestones,omitempty"`
	Items            []ProgressItem      `json:"items,omitempty"`
	Blockers         []ProgressBlocker   `json:"blockers,omitempty"`
	Recovery         RecoveryPlan        `json:"recovery"`
	EvidenceRefs     []string            `json:"evidenceRefs,omitempty"`
}

type RunOutcome struct {
	RunID        string   `json:"runId,omitempty"`
	State        string   `json:"state"`
	RawState     string   `json:"rawState,omitempty"`
	Accepted     bool     `json:"accepted"`
	Terminal     bool     `json:"terminal"`
	ResultID     string   `json:"resultId,omitempty"`
	Summary      string   `json:"summary,omitempty"`
	Reason       string   `json:"reason,omitempty"`
	EvidenceRefs []string `json:"evidenceRefs,omitempty"`
	ArtifactRefs []string `json:"artifactRefs,omitempty"`
}

type WorkspaceOutcome struct {
	WorkspaceID       string   `json:"workspaceId,omitempty"`
	State             string   `json:"state"`
	RawWorkspaceState string   `json:"rawWorkspaceState,omitempty"`
	RunID             string   `json:"runId,omitempty"`
	ResultID          string   `json:"resultId,omitempty"`
	Accepted          bool     `json:"accepted"`
	Terminal          bool     `json:"terminal"`
	Summary           string   `json:"summary,omitempty"`
	Reason            string   `json:"reason,omitempty"`
	EvidenceRefs      []string `json:"evidenceRefs,omitempty"`
	ArtifactRefs      []string `json:"artifactRefs,omitempty"`
}

type ProgressMilestone struct {
	ID           string   `json:"id"`
	Title        string   `json:"title"`
	State        string   `json:"state"`
	EvidenceRefs []string `json:"evidenceRefs,omitempty"`
}

type ProgressItem struct {
	ID           string   `json:"id"`
	Kind         string   `json:"kind"`
	Title        string   `json:"title"`
	State        string   `json:"state"`
	EvidenceRefs []string `json:"evidenceRefs,omitempty"`
}

type ProgressBlocker struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	State       string `json:"state"`
	Reason      string `json:"reason,omitempty"`
	EvidenceRef string `json:"evidenceRef,omitempty"`
}

type RecoveryPlan struct {
	Mode               string `json:"mode"`
	NextAction         string `json:"nextAction,omitempty"`
	RunID              string `json:"runId,omitempty"`
	StepID             string `json:"stepId,omitempty"`
	BlockingTicketID   string `json:"blockingTicketId,omitempty"`
	BlockingApprovalID string `json:"blockingApprovalId,omitempty"`
	CheckpointID       string `json:"checkpointId,omitempty"`
	FailureClass       string `json:"failureClass,omitempty"`
	FailureSignature   string `json:"failureSignature,omitempty"`
	Reason             string `json:"reason,omitempty"`
}

type ContextBundle struct {
	WorkspaceID           string                        `json:"workspaceId"`
	RunID                 string                        `json:"runId,omitempty"`
	Mode                  string                        `json:"mode"`
	Summary               string                        `json:"summary,omitempty"`
	CompressedContext     CompressedContext             `json:"compressedContext"`
	TaskProgress          TaskProgress                  `json:"taskProgress"`
	Recovery              RecoveryPlan                  `json:"recovery"`
	LatestCheckpoint      *RuntimeCheckpoint            `json:"latestCheckpoint,omitempty"`
	Sections              []ContextSection              `json:"sections,omitempty"`
	RecentEvidence        []ContextEvidence             `json:"recentEvidence,omitempty"`
	CompressionCandidates []ContextCompressionCandidate `json:"compressionCandidates,omitempty"`
}

type ContextSection struct {
	ID           string         `json:"id"`
	Kind         string         `json:"kind"`
	Title        string         `json:"title"`
	Priority     int            `json:"priority"`
	State        string         `json:"state,omitempty"`
	Summary      string         `json:"summary,omitempty"`
	EvidenceRefs []string       `json:"evidenceRefs,omitempty"`
	Data         map[string]any `json:"data,omitempty"`
}

type ContextEvidence struct {
	ID           string         `json:"id"`
	Kind         string         `json:"kind"`
	State        string         `json:"state,omitempty"`
	Summary      string         `json:"summary,omitempty"`
	OccurredAt   string         `json:"occurredAt,omitempty"`
	EvidenceRefs []string       `json:"evidenceRefs,omitempty"`
	Data         map[string]any `json:"data,omitempty"`
}

type ContextCompressionCandidate struct {
	SectionID     string   `json:"sectionId"`
	Reason        string   `json:"reason"`
	Retention     string   `json:"retention"`
	SourceRefs    []string `json:"sourceRefs,omitempty"`
	SuggestedMode string   `json:"suggestedMode,omitempty"`
}

type CompressedContext struct {
	Version            string              `json:"version"`
	Mode               string              `json:"mode"`
	WorkspaceID        string              `json:"workspaceId"`
	RunID              string              `json:"runId,omitempty"`
	Summary            string              `json:"summary,omitempty"`
	CurrentMilestone   string              `json:"currentMilestone,omitempty"`
	Milestones         []ProgressMilestone `json:"milestones,omitempty"`
	Blockers           []ProgressBlocker   `json:"blockers,omitempty"`
	Recovery           RecoveryPlan        `json:"recovery"`
	RecentEvidence     []ContextEvidence   `json:"recentEvidence,omitempty"`
	ResultRefs         []string            `json:"resultRefs,omitempty"`
	SourceSectionRefs  []string            `json:"sourceSectionRefs,omitempty"`
	SourceEvidenceRefs []string            `json:"sourceEvidenceRefs,omitempty"`
	RetentionPolicy    string              `json:"retentionPolicy,omitempty"`
}

type TaskDecomposition struct {
	ID            string                  `json:"id"`
	WorkspaceID   string                  `json:"workspaceId"`
	RunID         string                  `json:"runId"`
	RootStepID    string                  `json:"rootStepId"`
	State         string                  `json:"state"`
	Summary       string                  `json:"summary,omitempty"`
	GraphBranch   string                  `json:"graphBranch,omitempty"`
	NodeTotal     int                     `json:"nodeTotal"`
	NodeCompleted int                     `json:"nodeCompleted"`
	NodeFailed    int                     `json:"nodeFailed"`
	NodeRunning   int                     `json:"nodeRunning"`
	Nodes         []TaskDecompositionNode `json:"nodes,omitempty"`
	EvidenceRefs  []string                `json:"evidenceRefs,omitempty"`
}

type TaskDecompositionNode struct {
	ID           string         `json:"id"`
	Kind         string         `json:"kind"`
	Title        string         `json:"title,omitempty"`
	State        string         `json:"state"`
	DependsOn    []string       `json:"dependsOn,omitempty"`
	Branch       string         `json:"branch,omitempty"`
	StepID       string         `json:"stepId,omitempty"`
	StepState    string         `json:"stepState,omitempty"`
	EvidenceRefs []string       `json:"evidenceRefs,omitempty"`
	Input        map[string]any `json:"input,omitempty"`
}

type ExecutionSection struct {
	ID                 string   `json:"id"`
	WorkspaceID        string   `json:"workspaceId"`
	RunID              string   `json:"runId"`
	Title              string   `json:"title"`
	State              string   `json:"state"`
	GraphRootStepID    string   `json:"graphRootStepId,omitempty"`
	GraphNodeID        string   `json:"graphNodeId,omitempty"`
	StepID             string   `json:"stepId,omitempty"`
	StepKind           string   `json:"stepKind,omitempty"`
	RelatedDoDItems    []string `json:"relatedDoDItems,omitempty"`
	PlannedStepRefs    []string `json:"plannedStepRefs,omitempty"`
	ActualEvidenceRefs []string `json:"actualEvidenceRefs,omitempty"`
	DecisionRefs       []string `json:"decisionRefs,omitempty"`
	ApprovalRefs       []string `json:"approvalRefs,omitempty"`
	ToolCallRefs       []string `json:"toolCallRefs,omitempty"`
	PatchRefs          []string `json:"patchRefs,omitempty"`
	ValidationRefs     []string `json:"validationRefs,omitempty"`
	ResultRefs         []string `json:"resultRefs,omitempty"`
	ArtifactRefs       []string `json:"artifactRefs,omitempty"`
	MemoryRecallRefs   []string `json:"memoryRecallRefs,omitempty"`
	MemoryIDs          []string `json:"memoryIds,omitempty"`
	Summary            string   `json:"summary,omitempty"`
}

type RunProgress struct {
	State            string `json:"state"`
	CurrentStepID    string `json:"currentStepId,omitempty"`
	CurrentStepKind  string `json:"currentStepKind,omitempty"`
	CurrentStepState string `json:"currentStepState,omitempty"`
	StepTotal        int    `json:"stepTotal"`
	StepDispatched   int    `json:"stepDispatched"`
	StepCompleted    int    `json:"stepCompleted"`
	StepFailed       int    `json:"stepFailed"`
	Blocked          bool   `json:"blocked"`
	Terminal         bool   `json:"terminal"`
}

type RuntimeEvidenceSummary struct {
	ToolCallTotal          int `json:"toolCallTotal"`
	ToolCallRunning        int `json:"toolCallRunning"`
	ToolCallCompleted      int `json:"toolCallCompleted"`
	ToolCallFailed         int `json:"toolCallFailed"`
	PatchTotal             int `json:"patchTotal"`
	PatchApplied           int `json:"patchApplied"`
	PatchFailed            int `json:"patchFailed"`
	ValidationTotal        int `json:"validationTotal"`
	ValidationPassed       int `json:"validationPassed"`
	ValidationFailed       int `json:"validationFailed"`
	ValidationWarning      int `json:"validationWarning"`
	RuntimeCheckpointTotal int `json:"runtimeCheckpointTotal"`
	ResultTotal            int `json:"resultTotal"`
	ArtifactTotal          int `json:"artifactTotal"`
	EventTotal             int `json:"eventTotal"`
}

type RuntimeTimelineItem struct {
	Source     string         `json:"source"`
	Type       string         `json:"type"`
	ID         string         `json:"id,omitempty"`
	Topic      string         `json:"topic,omitempty"`
	RunState   string         `json:"runState,omitempty"`
	StepID     string         `json:"stepId,omitempty"`
	StepState  string         `json:"stepState,omitempty"`
	Summary    string         `json:"summary,omitempty"`
	OccurredAt string         `json:"occurredAt,omitempty"`
	Data       map[string]any `json:"data,omitempty"`
	Output     map[string]any `json:"output,omitempty"`
}

type MemoryRecallTrace struct {
	ID                  string            `json:"id"`
	WorkspaceID         string            `json:"workspaceId"`
	RunID               string            `json:"runId"`
	StepID              string            `json:"stepId,omitempty"`
	GraphNodeID         string            `json:"graphNodeId,omitempty"`
	Stage               string            `json:"stage,omitempty"`
	Source              string            `json:"source"`
	Policy              string            `json:"policy,omitempty"`
	Status              string            `json:"status"`
	ProviderID          string            `json:"providerId,omitempty"`
	Operation           string            `json:"operation,omitempty"`
	QueryTextPreview    string            `json:"queryTextPreview,omitempty"`
	QueryTextSha256     string            `json:"queryTextSha256,omitempty"`
	MinScore            float64           `json:"minScore,omitempty"`
	Limit               int               `json:"limit,omitempty"`
	EmbeddingModel      string            `json:"embeddingModel,omitempty"`
	VectorDimension     int               `json:"vectorDimension,omitempty"`
	RetrievedMemoryIDs  []string          `json:"retrievedMemoryIds,omitempty"`
	RetrievedMemoryHits []MemoryRecallHit `json:"retrievedMemoryHits,omitempty"`
	Reason              string            `json:"reason,omitempty"`
	OccurredAt          string            `json:"occurredAt,omitempty"`
	EvidenceRef         string            `json:"evidenceRef,omitempty"`
}

type MemoryRecallHit struct {
	ID              string  `json:"id"`
	Summary         string  `json:"summary,omitempty"`
	VectorScore     float64 `json:"vectorScore,omitempty"`
	VectorDimension int     `json:"vectorDimension,omitempty"`
}

type MemoryRecallTraceQuery struct {
	WorkspaceID string
	RunID       string
	StepID      string
	Stage       string
	Source      string
	Status      string
	Limit       int
}

type RuntimeCapability struct {
	ID             string   `json:"id"`
	ServerID       string   `json:"serverId,omitempty"`
	Kind           string   `json:"kind,omitempty"`
	Name           string   `json:"name,omitempty"`
	DisplayName    string   `json:"displayName,omitempty"`
	Description    string   `json:"description,omitempty"`
	Category       string   `json:"category,omitempty"`
	Tags           []string `json:"tags,omitempty"`
	RiskLevel      string   `json:"riskLevel,omitempty"`
	Enabled        bool     `json:"enabled"`
	ApprovalPolicy string   `json:"approvalPolicy,omitempty"`
	Status         string   `json:"status,omitempty"`
	UseCount       int      `json:"useCount,omitempty"`
	EvidenceRefs   []string `json:"evidenceRefs,omitempty"`
}

type EventRecord struct {
	ID           string         `json:"id"`
	Version      string         `json:"version"`
	Topic        string         `json:"topic"`
	OccurredAt   string         `json:"occurredAt,omitempty"`
	SourceModule string         `json:"sourceModule,omitempty"`
	SubjectType  string         `json:"subjectType,omitempty"`
	SubjectID    string         `json:"subjectId,omitempty"`
	WorkspaceID  string         `json:"workspaceId,omitempty"`
	RunID        string         `json:"runId,omitempty"`
	StepID       string         `json:"stepId,omitempty"`
	Payload      map[string]any `json:"payload,omitempty"`
}

type EventRecordQuery struct {
	WorkspaceID    string
	RunID          string
	Topic          string
	SubjectType    string
	SubjectID      string
	CursorAfterID  string
	OccurredAfter  string
	OccurredBefore string
	Limit          int
}

type ToolCall struct {
	ID          string         `json:"id"`
	WorkspaceID string         `json:"workspaceId"`
	RunID       string         `json:"runId"`
	StepID      string         `json:"stepId,omitempty"`
	ToolName    string         `json:"toolName"`
	Arguments   map[string]any `json:"arguments,omitempty"`
	State       string         `json:"state"`
	StartedAt   string         `json:"startedAt,omitempty"`
	EndedAt     string         `json:"endedAt,omitempty"`
	ExitSummary string         `json:"exitSummary,omitempty"`
	Output      map[string]any `json:"output,omitempty"`
	Error       string         `json:"error,omitempty"`
}

type Patch struct {
	ID          string `json:"id"`
	WorkspaceID string `json:"workspaceId"`
	RunID       string `json:"runId"`
	StepID      string `json:"stepId,omitempty"`
	ToolCallID  string `json:"toolCallId,omitempty"`
	PatchType   string `json:"patchType"`
	TargetRef   string `json:"targetRef,omitempty"`
	Summary     string `json:"summary,omitempty"`
	State       string `json:"state"`
	CreatedAt   string `json:"createdAt,omitempty"`
	AppliedAt   string `json:"appliedAt,omitempty"`
}

type ValidationResult struct {
	ID             string `json:"id"`
	WorkspaceID    string `json:"workspaceId"`
	RunID          string `json:"runId"`
	StepID         string `json:"stepId,omitempty"`
	ValidationKind string `json:"validationKind"`
	TargetRef      string `json:"targetRef,omitempty"`
	State          string `json:"state"`
	Summary        string `json:"summary,omitempty"`
	EvidenceRef    string `json:"evidenceRef,omitempty"`
	StartedAt      string `json:"startedAt,omitempty"`
	EndedAt        string `json:"endedAt,omitempty"`
}

type RuntimeCheckpoint struct {
	ID                 string         `json:"id"`
	WorkspaceID        string         `json:"workspaceId"`
	RunID              string         `json:"runId"`
	CheckpointType     string         `json:"checkpointType"`
	CurrentStepID      string         `json:"currentStepId,omitempty"`
	RunStateSnapshot   string         `json:"runStateSnapshot"`
	BlockingTicketID   string         `json:"blockingTicketId,omitempty"`
	BlockingApprovalID string         `json:"blockingApprovalId,omitempty"`
	ThreadRef          string         `json:"threadRef,omitempty"`
	OccurredAt         string         `json:"occurredAt,omitempty"`
	RestorableUntil    string         `json:"restorableUntil,omitempty"`
	Data               map[string]any `json:"data,omitempty"`
}

type Result struct {
	ID            string         `json:"id"`
	WorkspaceID   string         `json:"workspaceId"`
	RunID         string         `json:"runId"`
	GoalID        string         `json:"goalId,omitempty"`
	DoDID         string         `json:"doDId,omitempty"`
	State         string         `json:"state"`
	Title         string         `json:"title,omitempty"`
	Summary       string         `json:"summary,omitempty"`
	SourceType    string         `json:"sourceType,omitempty"`
	SourceRef     string         `json:"sourceRef,omitempty"`
	EvidenceRefs  []string       `json:"evidenceRefs,omitempty"`
	ArtifactRefs  []string       `json:"artifactRefs,omitempty"`
	DoDAssessment *DoDAssessment `json:"doDAssessment,omitempty"`
	SummaryDetail *ResultSummary `json:"summaryDetail,omitempty"`
	CreatedAt     string         `json:"createdAt,omitempty"`
	UpdatedAt     string         `json:"updatedAt,omitempty"`
}

type ResultSummary struct {
	GoalOutcome           string              `json:"goalOutcome,omitempty"`
	DoDItems              []DoDAssessmentItem `json:"doDItems,omitempty"`
	ImplementationSummary string              `json:"implementationSummary,omitempty"`
	VerificationSummary   string              `json:"verificationSummary,omitempty"`
	EvidenceRefs          []string            `json:"evidenceRefs,omitempty"`
	ArtifactRefs          []string            `json:"artifactRefs,omitempty"`
	KnownLimitations      []string            `json:"knownLimitations,omitempty"`
	ResidualRisks         []string            `json:"residualRisks,omitempty"`
	NextActions           []string            `json:"nextActions,omitempty"`
}

type DoDAssessment struct {
	Satisfied bool                `json:"satisfied"`
	Items     []DoDAssessmentItem `json:"items,omitempty"`
	Summary   string              `json:"summary,omitempty"`
}

type DoDAssessmentItem struct {
	Item          string   `json:"item"`
	State         string   `json:"state"`
	EvidenceRefs  []string `json:"evidenceRefs,omitempty"`
	FailureReason string   `json:"failureReason,omitempty"`
	Confidence    float64  `json:"confidence,omitempty"`
	SourceNote    string   `json:"sourceNote,omitempty"`
}

type Artifact struct {
	ID                 string         `json:"id"`
	WorkspaceID        string         `json:"workspaceId"`
	RunID              string         `json:"runId,omitempty"`
	ResultID           string         `json:"resultId,omitempty"`
	StepID             string         `json:"stepId,omitempty"`
	ToolCallID         string         `json:"toolCallId,omitempty"`
	PatchID            string         `json:"patchId,omitempty"`
	ValidationResultID string         `json:"validationResultId,omitempty"`
	ArtifactType       string         `json:"artifactType"`
	Category           string         `json:"category,omitempty"`
	Name               string         `json:"name,omitempty"`
	Summary            string         `json:"summary,omitempty"`
	URI                string         `json:"uri,omitempty"`
	ContentType        string         `json:"contentType,omitempty"`
	State              string         `json:"state"`
	EvidenceRefs       []string       `json:"evidenceRefs,omitempty"`
	Metadata           map[string]any `json:"metadata,omitempty"`
	CreatedAt          string         `json:"createdAt,omitempty"`
	UpdatedAt          string         `json:"updatedAt,omitempty"`
}

type WorkspaceSnapshot struct {
	Workspace          Workspace           `json:"workspace"`
	Outcome            WorkspaceOutcome    `json:"outcome"`
	TaskProgress       TaskProgress        `json:"taskProgress"`
	TaskDecompositions []TaskDecomposition `json:"taskDecompositions,omitempty"`
	ExecutionSections  []ExecutionSection  `json:"executionSections,omitempty"`
	MemoryRecallTraces []MemoryRecallTrace `json:"memoryRecallTraces,omitempty"`
	Runs               []Run               `json:"runs,omitempty"`
	Results            []Result            `json:"results,omitempty"`
	Artifacts          []Artifact          `json:"artifacts,omitempty"`
	EventRecords       []EventRecord       `json:"eventRecords,omitempty"`
	Decisions          []Decision          `json:"decisions,omitempty"`
	Approvals          []Approval          `json:"approvals,omitempty"`
}

type readModelDependencies struct {
	workspaces      repository.WorkspaceRepository
	runs            repository.RunRepository
	steps           repository.StepRepository
	results         repository.ResultRepository
	artifacts       repository.ArtifactRepository
	eventRecords    repository.EventRecordRepository
	memoryRecalls   repository.MemoryRecallTraceRepository
	toolCalls       repository.ToolCallRepository
	mcpCapabilities repository.MCPCapabilityRepository
	patches         repository.PatchRepository
	validations     repository.ValidationResultRepository
	checkpoints     repository.RuntimeCheckpointRepository
	tickets         repository.DecisionTicketRepository
	resolutions     repository.DecisionResolutionRepository
	approvals       repository.ApprovalRepository
	published       []events.WorkorchEvent
}

func ReadRunSnapshotWithSession(ctx context.Context, session *RuntimeSession, runID string) (ret RunSnapshot, err error) {
	deps, err := readModelDependenciesFromSession(session)
	if err != nil {
		return RunSnapshot{}, err
	}
	defer session.CloseOnReturn(ctx, &err)

	return readRunSnapshot(ctx, deps, common.RunID(runID))
}

func ReadRunSnapshotWithBackend(ctx context.Context, backend string, dataRoot string, runID string) (RunSnapshot, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return RunSnapshot{}, err
	}
	return ReadRunSnapshotWithSession(ctx, session, runID)
}

func ReadRunSnapshotFromEnv(ctx context.Context, runID string) (RunSnapshot, error) {
	session, err := NewRuntimeSessionFromEnv()
	if err != nil {
		return RunSnapshot{}, err
	}
	return ReadRunSnapshotWithSession(ctx, session, runID)
}

func ReadRunStatusWithSession(ctx context.Context, session *RuntimeSession, runID string) (ret RunStatus, err error) {
	deps, err := readModelDependenciesFromSession(session)
	if err != nil {
		return RunStatus{}, err
	}
	defer session.CloseOnReturn(ctx, &err)

	return readRunStatus(ctx, deps, common.RunID(runID))
}

func ReadRunStatusWithBackend(ctx context.Context, backend string, dataRoot string, runID string) (RunStatus, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return RunStatus{}, err
	}
	return ReadRunStatusWithSession(ctx, session, runID)
}

func ReadRunStatusFromEnv(ctx context.Context, runID string) (RunStatus, error) {
	session, err := NewRuntimeSessionFromEnv()
	if err != nil {
		return RunStatus{}, err
	}
	return ReadRunStatusWithSession(ctx, session, runID)
}

func ReadRunObservabilityWithSession(ctx context.Context, session *RuntimeSession, runID string) (ret RunObservability, err error) {
	deps, err := readModelDependenciesFromSession(session)
	if err != nil {
		return RunObservability{}, err
	}
	defer session.CloseOnReturn(ctx, &err)

	snapshot, err := readRunSnapshot(ctx, deps, common.RunID(runID))
	if err != nil {
		return RunObservability{}, err
	}
	return mapRunObservability(snapshot), nil
}

func ReadRunTaskProgressWithSession(ctx context.Context, session *RuntimeSession, runID string) (ret TaskProgress, err error) {
	deps, err := readModelDependenciesFromSession(session)
	if err != nil {
		return TaskProgress{}, err
	}
	defer session.CloseOnReturn(ctx, &err)

	snapshot, err := readRunSnapshot(ctx, deps, common.RunID(runID))
	if err != nil {
		return TaskProgress{}, err
	}
	return runSnapshotTaskProgress(snapshot), nil
}

func ReadRunTaskProgressWithBackend(ctx context.Context, backend string, dataRoot string, runID string) (TaskProgress, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return TaskProgress{}, err
	}
	return ReadRunTaskProgressWithSession(ctx, session, runID)
}

func ReadRunTaskProgressFromEnv(ctx context.Context, runID string) (TaskProgress, error) {
	session, err := NewRuntimeSessionFromEnv()
	if err != nil {
		return TaskProgress{}, err
	}
	return ReadRunTaskProgressWithSession(ctx, session, runID)
}

func ReadRunContextBundleWithSession(ctx context.Context, session *RuntimeSession, runID string) (ret ContextBundle, err error) {
	deps, err := readModelDependenciesFromSession(session)
	if err != nil {
		return ContextBundle{}, err
	}
	defer session.CloseOnReturn(ctx, &err)

	snapshot, err := readRunSnapshot(ctx, deps, common.RunID(runID))
	if err != nil {
		return ContextBundle{}, err
	}
	return mapRunContextBundle(snapshot), nil
}

func ReadRunContextBundleWithBackend(ctx context.Context, backend string, dataRoot string, runID string) (ContextBundle, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return ContextBundle{}, err
	}
	return ReadRunContextBundleWithSession(ctx, session, runID)
}

func ReadRunContextBundleFromEnv(ctx context.Context, runID string) (ContextBundle, error) {
	session, err := NewRuntimeSessionFromEnv()
	if err != nil {
		return ContextBundle{}, err
	}
	return ReadRunContextBundleWithSession(ctx, session, runID)
}

func ListMemoryRecallTracesWithSession(ctx context.Context, session *RuntimeSession, query MemoryRecallTraceQuery) (ret []MemoryRecallTrace, err error) {
	deps, err := readModelDependenciesFromSession(session)
	if err != nil {
		return nil, err
	}
	defer session.CloseOnReturn(ctx, &err)
	items, err := deps.memoryRecalls.List(ctx, repository.MemoryRecallTraceQuery{
		WorkspaceID: common.WorkspaceID(strings.TrimSpace(query.WorkspaceID)),
		RunID:       common.RunID(strings.TrimSpace(query.RunID)),
		StepID:      common.StepID(strings.TrimSpace(query.StepID)),
		Stage:       strings.TrimSpace(query.Stage),
		Source:      strings.TrimSpace(query.Source),
		Status:      strings.TrimSpace(query.Status),
		Limit:       query.Limit,
	})
	if err != nil {
		return nil, err
	}
	return mapMemoryRecallTraces(items), nil
}

func ListMemoryRecallTracesWithBackend(ctx context.Context, backend string, dataRoot string, query MemoryRecallTraceQuery) ([]MemoryRecallTrace, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return nil, err
	}
	return ListMemoryRecallTracesWithSession(ctx, session, query)
}

func ReadRunTaskDecompositionsWithSession(ctx context.Context, session *RuntimeSession, runID string) (ret []TaskDecomposition, err error) {
	deps, err := readModelDependenciesFromSession(session)
	if err != nil {
		return nil, err
	}
	defer session.CloseOnReturn(ctx, &err)

	snapshot, err := readRunSnapshot(ctx, deps, common.RunID(runID))
	if err != nil {
		return nil, err
	}
	return runSnapshotTaskDecompositions(snapshot), nil
}

func ReadRunTaskDecompositionsWithBackend(ctx context.Context, backend string, dataRoot string, runID string) ([]TaskDecomposition, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return nil, err
	}
	return ReadRunTaskDecompositionsWithSession(ctx, session, runID)
}

func ReadRunTaskDecompositionsFromEnv(ctx context.Context, runID string) ([]TaskDecomposition, error) {
	session, err := NewRuntimeSessionFromEnv()
	if err != nil {
		return nil, err
	}
	return ReadRunTaskDecompositionsWithSession(ctx, session, runID)
}

func ReadRunObservabilityWithBackend(ctx context.Context, backend string, dataRoot string, runID string) (RunObservability, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return RunObservability{}, err
	}
	return ReadRunObservabilityWithSession(ctx, session, runID)
}

func ReadRunObservabilityFromEnv(ctx context.Context, runID string) (RunObservability, error) {
	session, err := NewRuntimeSessionFromEnv()
	if err != nil {
		return RunObservability{}, err
	}
	return ReadRunObservabilityWithSession(ctx, session, runID)
}

func RunStatusFromSnapshot(snapshot RunSnapshot) RunStatus {
	return mapRunStatus(snapshot)
}

func RunObservabilityFromSnapshot(snapshot RunSnapshot) RunObservability {
	return mapRunObservability(snapshot)
}

func ListEventRecordsWithSession(ctx context.Context, session *RuntimeSession, query EventRecordQuery) (ret []EventRecord, err error) {
	deps, err := readModelDependenciesFromSession(session)
	if err != nil {
		return nil, err
	}
	defer session.CloseOnReturn(ctx, &err)

	items, err := deps.eventRecords.List(ctx, repository.EventRecordQuery{
		WorkspaceID:    common.WorkspaceID(query.WorkspaceID),
		RunID:          common.RunID(query.RunID),
		Topic:          query.Topic,
		SubjectType:    query.SubjectType,
		SubjectID:      query.SubjectID,
		CursorAfterID:  common.EventRecordID(query.CursorAfterID),
		OccurredAfter:  query.OccurredAfter,
		OccurredBefore: query.OccurredBefore,
		Limit:          query.Limit,
	})
	if err != nil {
		return nil, err
	}
	return mapEventRecords(items), nil
}

func ListEventRecordsWithBackend(ctx context.Context, backend string, dataRoot string, query EventRecordQuery) ([]EventRecord, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return nil, err
	}
	return ListEventRecordsWithSession(ctx, session, query)
}

func ListEventRecordsFromEnv(ctx context.Context, query EventRecordQuery) ([]EventRecord, error) {
	session, err := NewRuntimeSessionFromEnv()
	if err != nil {
		return nil, err
	}
	return ListEventRecordsWithSession(ctx, session, query)
}

func ReadWorkspaceSnapshotWithSession(ctx context.Context, session *RuntimeSession, workspaceID string) (ret WorkspaceSnapshot, err error) {
	deps, err := readModelDependenciesFromSession(session)
	if err != nil {
		return WorkspaceSnapshot{}, err
	}
	defer session.CloseOnReturn(ctx, &err)

	return readWorkspaceSnapshot(ctx, deps, common.WorkspaceID(workspaceID), nil)
}

func ReadWorkspaceSnapshotWithRunSnapshotWithSession(ctx context.Context, session *RuntimeSession, workspaceID string, runSnapshot RunSnapshot) (ret WorkspaceSnapshot, err error) {
	deps, err := readModelDependenciesFromSession(session)
	if err != nil {
		return WorkspaceSnapshot{}, err
	}
	defer session.CloseOnReturn(ctx, &err)

	return readWorkspaceSnapshot(ctx, deps, common.WorkspaceID(workspaceID), &runSnapshot)
}

func ReadWorkspaceTaskProgressWithSession(ctx context.Context, session *RuntimeSession, workspaceID string) (ret TaskProgress, err error) {
	deps, err := readModelDependenciesFromSession(session)
	if err != nil {
		return TaskProgress{}, err
	}
	defer session.CloseOnReturn(ctx, &err)

	return readWorkspaceTaskProgress(ctx, deps, common.WorkspaceID(workspaceID))
}

func ReadWorkspaceTaskProgressWithBackend(ctx context.Context, backend string, dataRoot string, workspaceID string) (TaskProgress, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return TaskProgress{}, err
	}
	return ReadWorkspaceTaskProgressWithSession(ctx, session, workspaceID)
}

func ReadWorkspaceTaskProgressFromEnv(ctx context.Context, workspaceID string) (TaskProgress, error) {
	session, err := NewRuntimeSessionFromEnv()
	if err != nil {
		return TaskProgress{}, err
	}
	return ReadWorkspaceTaskProgressWithSession(ctx, session, workspaceID)
}

func ReadWorkspaceContextBundleWithSession(ctx context.Context, session *RuntimeSession, workspaceID string) (ret ContextBundle, err error) {
	deps, err := readModelDependenciesFromSession(session)
	if err != nil {
		return ContextBundle{}, err
	}
	defer session.CloseOnReturn(ctx, &err)

	return readWorkspaceContextBundle(ctx, deps, common.WorkspaceID(workspaceID))
}

func ReadWorkspaceContextBundleWithBackend(ctx context.Context, backend string, dataRoot string, workspaceID string) (ContextBundle, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return ContextBundle{}, err
	}
	return ReadWorkspaceContextBundleWithSession(ctx, session, workspaceID)
}

func ReadWorkspaceContextBundleFromEnv(ctx context.Context, workspaceID string) (ContextBundle, error) {
	session, err := NewRuntimeSessionFromEnv()
	if err != nil {
		return ContextBundle{}, err
	}
	return ReadWorkspaceContextBundleWithSession(ctx, session, workspaceID)
}

func ReadWorkspaceTaskDecompositionsWithSession(ctx context.Context, session *RuntimeSession, workspaceID string) (ret []TaskDecomposition, err error) {
	deps, err := readModelDependenciesFromSession(session)
	if err != nil {
		return nil, err
	}
	defer session.CloseOnReturn(ctx, &err)

	return readWorkspaceTaskDecompositions(ctx, deps, common.WorkspaceID(workspaceID))
}

func ReadWorkspaceTaskDecompositionsWithBackend(ctx context.Context, backend string, dataRoot string, workspaceID string) ([]TaskDecomposition, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return nil, err
	}
	return ReadWorkspaceTaskDecompositionsWithSession(ctx, session, workspaceID)
}

func ReadWorkspaceTaskDecompositionsFromEnv(ctx context.Context, workspaceID string) ([]TaskDecomposition, error) {
	session, err := NewRuntimeSessionFromEnv()
	if err != nil {
		return nil, err
	}
	return ReadWorkspaceTaskDecompositionsWithSession(ctx, session, workspaceID)
}

func ReadWorkspaceSnapshotWithBackend(ctx context.Context, backend string, dataRoot string, workspaceID string) (WorkspaceSnapshot, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return WorkspaceSnapshot{}, err
	}
	return ReadWorkspaceSnapshotWithSession(ctx, session, workspaceID)
}

func ReadWorkspaceSnapshotFromEnv(ctx context.Context, workspaceID string) (WorkspaceSnapshot, error) {
	session, err := NewRuntimeSessionFromEnv()
	if err != nil {
		return WorkspaceSnapshot{}, err
	}
	return ReadWorkspaceSnapshotWithSession(ctx, session, workspaceID)
}

func readModelDependenciesFromSession(session *RuntimeSession) (readModelDependencies, error) {
	exports, err := session.Exports()
	if err != nil {
		return readModelDependencies{}, err
	}
	if exports.Repositories.Workspaces == nil ||
		exports.Repositories.Runs == nil ||
		exports.Repositories.Steps == nil ||
		exports.Repositories.Results == nil ||
		exports.Repositories.Artifacts == nil ||
		exports.Repositories.EventRecords == nil ||
		exports.Repositories.MemoryRecallTraces == nil ||
		exports.Repositories.ToolCalls == nil ||
		exports.Repositories.MCPCapabilities == nil ||
		exports.Repositories.Patches == nil ||
		exports.Repositories.ValidationResults == nil ||
		exports.Repositories.RuntimeCheckpoints == nil ||
		exports.Repositories.Tickets == nil ||
		exports.Repositories.Resolutions == nil ||
		exports.Repositories.Approvals == nil {
		return readModelDependencies{}, common.ErrInvalidArgument
	}

	var published []events.WorkorchEvent
	if session.Runtime != nil {
		published = session.Runtime.PublishedWorkorchEvents()
	}

	return readModelDependencies{
		workspaces:      exports.Repositories.Workspaces,
		runs:            exports.Repositories.Runs,
		steps:           exports.Repositories.Steps,
		results:         exports.Repositories.Results,
		artifacts:       exports.Repositories.Artifacts,
		eventRecords:    exports.Repositories.EventRecords,
		memoryRecalls:   exports.Repositories.MemoryRecallTraces,
		toolCalls:       exports.Repositories.ToolCalls,
		mcpCapabilities: exports.Repositories.MCPCapabilities,
		patches:         exports.Repositories.Patches,
		validations:     exports.Repositories.ValidationResults,
		checkpoints:     exports.Repositories.RuntimeCheckpoints,
		tickets:         exports.Repositories.Tickets,
		resolutions:     exports.Repositories.Resolutions,
		approvals:       exports.Repositories.Approvals,
		published:       published,
	}, nil
}

func readRunSnapshot(ctx context.Context, deps readModelDependencies, runID common.RunID) (RunSnapshot, error) {
	item, err := deps.runs.Get(ctx, runID)
	if err != nil {
		return RunSnapshot{}, err
	}
	var workspace *workspacemodel.Workspace
	var steps []*runmodel.Step
	var toolCalls []*runmodel.ToolCall
	var patches []*runmodel.Patch
	var validations []*runmodel.ValidationResult
	var checkpoints []*runmodel.RuntimeCheckpoint
	var results []*resultmodel.Result
	var artifacts []*resultmodel.Artifact
	var eventRecords []*eventstreammodel.EventRecord
	var memoryRecallTraces []*memorymodel.MemoryRecallTrace
	var decisions []Decision
	var selectedTicket *decisionmodel.Ticket
	var selectedResolution *decisionmodel.Resolution
	var approvals []Approval
	var selectedApproval *approvalmodel.Approval
	var mcpCapabilityItems []*mcpmodel.Capability
	tasks := []func() error{
		func() error {
			var err error
			workspace, err = deps.workspaces.Get(ctx, item.WorkspaceID)
			return err
		},
		func() error {
			var err error
			steps, err = deps.steps.ListByRun(ctx, item.ID)
			return err
		},
		func() error {
			var err error
			toolCalls, err = deps.toolCalls.ListByRun(ctx, item.ID)
			return err
		},
		func() error {
			var err error
			patches, err = deps.patches.ListByRun(ctx, item.ID)
			return err
		},
		func() error {
			var err error
			validations, err = deps.validations.ListByRun(ctx, item.ID)
			return err
		},
		func() error {
			var err error
			checkpoints, err = deps.checkpoints.ListByRun(ctx, item.ID)
			return err
		},
		func() error {
			var err error
			results, err = deps.results.ListByRun(ctx, item.ID)
			return err
		},
		func() error {
			var err error
			artifacts, err = deps.artifacts.ListByRun(ctx, item.ID)
			return err
		},
		func() error {
			var err error
			eventRecords, err = deps.eventRecords.List(ctx, repository.EventRecordQuery{RunID: item.ID})
			return err
		},
		func() error {
			var err error
			if deps.memoryRecalls != nil {
				memoryRecallTraces, err = deps.memoryRecalls.List(ctx, repository.MemoryRecallTraceQuery{RunID: item.ID})
			}
			return err
		},
		func() error {
			var err error
			decisions, selectedTicket, selectedResolution, err = decisionsForRun(ctx, deps, item)
			return err
		},
		func() error {
			var err error
			approvals, selectedApproval, err = approvalsForRun(ctx, deps, item)
			return err
		},
	}
	if deps.mcpCapabilities != nil {
		tasks = append(tasks, func() error {
			var err error
			mcpCapabilityItems, err = deps.mcpCapabilities.List(ctx)
			return err
		})
	}
	if err := runReadModelTasks(tasks...); err != nil {
		return RunSnapshot{}, err
	}

	currentStep := lastStep(steps)
	observedItems := observed.BuildWorkorchSnapshot(workspace, item, steps, selectedTicket, selectedResolution, selectedApproval, deps.published)

	ret := RunSnapshot{
		Workspace:          mapWorkspaceWithRunProjection(workspace, item),
		Run:                mapRun(item),
		Step:               presenter.MapStep(currentStep),
		Steps:              presenter.MapSteps(steps),
		ToolCalls:          mapToolCalls(toolCalls),
		Patches:            mapPatches(patches),
		ValidationResults:  mapValidationResults(validations),
		RuntimeCheckpoints: mapRuntimeCheckpoints(checkpoints),
		Results:            mapResults(results),
		Artifacts:          mapArtifacts(artifacts),
		MemoryRecallTraces: mapMemoryRecallTraces(memoryRecallTraces),
		EventRecords:       mapEventRecords(eventRecords),
		Decisions:          decisions,
		Approvals:          approvals,
		Observed:           presenter.MapObserved(observedItems),
	}
	ret.Outcome = mapRunOutcome(ret)
	capabilityCatalog := runtimeCapabilityCatalogFromItems(ret.Run.ExecutionContext, ret.ToolCalls, mcpCapabilityItems)
	ret.SelectedCapabilities = selectedRuntimeCapabilities(ret.Run.ExecutionContext, capabilityCatalog)
	ret.UsedCapabilities = usedRuntimeCapabilities(ret, capabilityCatalog)
	ret.TaskProgress = mapTaskProgress(ret)
	ret.TaskDecompositions = mapTaskDecompositions(ret)
	ret.ExecutionSections = mapExecutionSections(ret)
	return ret, nil
}

func readRunRecoverySnapshot(ctx context.Context, deps readModelDependencies, runID common.RunID) (RunSnapshot, error) {
	item, err := deps.runs.Get(ctx, runID)
	if err != nil {
		return RunSnapshot{}, err
	}
	var workspace *workspacemodel.Workspace
	var steps []*runmodel.Step
	var toolCalls []*runmodel.ToolCall
	var patches []*runmodel.Patch
	var validations []*runmodel.ValidationResult
	var checkpoints []*runmodel.RuntimeCheckpoint
	var results []*resultmodel.Result
	var artifacts []*resultmodel.Artifact
	var eventRecords []*eventstreammodel.EventRecord
	var memoryRecallTraces []*memorymodel.MemoryRecallTrace
	var decisions []Decision
	var approvals []Approval
	err = runReadModelTasks(
		func() error {
			var err error
			workspace, err = deps.workspaces.Get(ctx, item.WorkspaceID)
			return err
		},
		func() error {
			var err error
			steps, err = deps.steps.ListByRun(ctx, item.ID)
			return err
		},
		func() error {
			var err error
			toolCalls, err = deps.toolCalls.ListByRun(ctx, item.ID)
			return err
		},
		func() error {
			var err error
			patches, err = deps.patches.ListByRun(ctx, item.ID)
			return err
		},
		func() error {
			var err error
			validations, err = deps.validations.ListByRun(ctx, item.ID)
			return err
		},
		func() error {
			var err error
			checkpoints, err = deps.checkpoints.ListByRun(ctx, item.ID)
			return err
		},
		func() error {
			var err error
			results, err = deps.results.ListByRun(ctx, item.ID)
			return err
		},
		func() error {
			var err error
			artifacts, err = deps.artifacts.ListByRun(ctx, item.ID)
			return err
		},
		func() error {
			var err error
			eventRecords, err = deps.eventRecords.List(ctx, repository.EventRecordQuery{RunID: item.ID})
			return err
		},
		func() error {
			var err error
			if deps.memoryRecalls != nil {
				memoryRecallTraces, err = deps.memoryRecalls.List(ctx, repository.MemoryRecallTraceQuery{RunID: item.ID})
			}
			return err
		},
		func() error {
			var selectedTicket *decisionmodel.Ticket
			var selectedResolution *decisionmodel.Resolution
			var err error
			decisions, selectedTicket, selectedResolution, err = decisionsForRun(ctx, deps, item)
			_, _ = selectedTicket, selectedResolution
			return err
		},
		func() error {
			var selectedApproval *approvalmodel.Approval
			var err error
			approvals, selectedApproval, err = approvalsForRun(ctx, deps, item)
			_ = selectedApproval
			return err
		},
	)
	if err != nil {
		return RunSnapshot{}, err
	}
	ret := RunSnapshot{
		Workspace:          mapWorkspaceWithRunProjection(workspace, item),
		Run:                mapRun(item),
		Step:               presenter.MapStep(lastStep(steps)),
		Steps:              presenter.MapSteps(steps),
		ToolCalls:          mapToolCalls(toolCalls),
		Patches:            mapPatches(patches),
		ValidationResults:  mapValidationResults(validations),
		RuntimeCheckpoints: mapRuntimeCheckpoints(checkpoints),
		Results:            mapResults(results),
		Artifacts:          mapArtifacts(artifacts),
		MemoryRecallTraces: mapMemoryRecallTraces(memoryRecallTraces),
		EventRecords:       mapEventRecords(eventRecords),
		Decisions:          decisions,
		Approvals:          approvals,
	}
	ret.Outcome = mapRunOutcome(ret)
	ret.TaskProgress = mapTaskProgress(ret)
	return ret, nil
}

func readRunStatus(ctx context.Context, deps readModelDependencies, runID common.RunID) (RunStatus, error) {
	item, err := deps.runs.Get(ctx, runID)
	if err != nil {
		return RunStatus{}, err
	}
	var steps []*runmodel.Step
	var decisions []*decisionmodel.Ticket
	var approvals []*approvalmodel.Approval
	err = runReadModelTasks(
		func() error {
			var err error
			steps, err = deps.steps.ListByRun(ctx, item.ID)
			return err
		},
		func() error {
			var err error
			decisions, err = deps.tickets.ListByRunID(ctx, item.ID)
			return err
		},
		func() error {
			var err error
			approvals, err = deps.approvals.ListByRunID(ctx, item.ID)
			return err
		},
	)
	if err != nil {
		return RunStatus{}, err
	}
	ret := RunStatus{
		ID:                 string(item.ID),
		WorkspaceID:        string(item.WorkspaceID),
		State:              string(item.State),
		StepCount:          len(steps),
		DecisionCount:      len(decisions),
		ApprovalCount:      len(approvals),
		BlockingTicketID:   string(item.BlockingTicketID),
		BlockingApprovalID: string(item.BlockingApprovalID),
		FailureReason:      item.FailureReason,
	}
	if currentStep := lastStep(steps); currentStep != nil {
		ret.CurrentStepID = string(currentStep.ID)
		ret.CurrentStepKind = currentStep.Kind
		ret.CurrentStepState = string(currentStep.State)
	}
	return ret, nil
}

func runReadModelTasks(tasks ...func() error) error {
	var wg sync.WaitGroup
	var errMu sync.Mutex
	var ret error
	recordErr := func(err error) {
		if err == nil {
			return
		}
		errMu.Lock()
		defer errMu.Unlock()
		if ret == nil {
			ret = err
		}
	}
	for _, task := range tasks {
		if task == nil {
			continue
		}
		wg.Add(1)
		go func(task func() error) {
			defer wg.Done()
			recordErr(task())
		}(task)
	}
	wg.Wait()
	return ret
}

func readWorkspaceSnapshot(ctx context.Context, deps readModelDependencies, workspaceID common.WorkspaceID, cachedRunSnapshot *RunSnapshot) (WorkspaceSnapshot, error) {
	var item *workspacemodel.Workspace
	if cachedRunSnapshot != nil && cachedRunSnapshot.Workspace.ID == string(workspaceID) {
		item = workspaceModelFromView(cachedRunSnapshot.Workspace)
	}

	var runItems []*runmodel.Run
	var resultItems []*resultmodel.Result
	var eventRecordItems []*eventstreammodel.EventRecord
	var artifactItems []*resultmodel.Artifact
	var ticketItems []*decisionmodel.Ticket
	var approvalItems []*approvalmodel.Approval
	tasks := []func() error{
		func() error {
			var err error
			runItems, err = deps.runs.ListByWorkspaceID(ctx, workspaceID)
			return err
		},
		func() error {
			var err error
			resultItems, err = deps.results.ListByWorkspace(ctx, workspaceID)
			return err
		},
		func() error {
			var err error
			eventRecordItems, err = deps.eventRecords.List(ctx, repository.EventRecordQuery{WorkspaceID: workspaceID})
			return err
		},
		func() error {
			var err error
			artifactItems, err = deps.artifacts.ListByWorkspace(ctx, workspaceID)
			return err
		},
		func() error {
			var err error
			ticketItems, err = deps.tickets.ListByWorkspaceID(ctx, workspaceID)
			return err
		},
		func() error {
			var err error
			approvalItems, err = deps.approvals.ListByWorkspaceID(ctx, workspaceID)
			return err
		},
	}
	if item == nil {
		tasks = append(tasks, func() error {
			var err error
			item, err = deps.workspaces.Get(ctx, workspaceID)
			return err
		})
	}
	err := runReadModelTasks(tasks...)
	if err != nil {
		return WorkspaceSnapshot{}, err
	}
	runs := make([]Run, 0, len(runItems))
	projectionRun := selectWorkspaceProjectionRun(item, runItems, resultItems, eventRecordItems)
	for _, runItem := range runItems {
		if runItem.WorkspaceID != workspaceID {
			continue
		}
		runs = append(runs, mapRun(runItem))
	}
	decisions := make([]Decision, 0, len(ticketItems))
	resolutions, err := resolutionsForTickets(ctx, deps.resolutions, ticketItems)
	if err != nil {
		return WorkspaceSnapshot{}, err
	}
	for _, ticket := range ticketItems {
		if ticket.WorkspaceID != workspaceID {
			continue
		}
		decisions = append(decisions, mapDecision(ticket, resolutions[ticket.ID]))
	}

	approvals := make([]Approval, 0, len(approvalItems))
	for _, approval := range approvalItems {
		if approval.WorkspaceID != workspaceID {
			continue
		}
		approvals = append(approvals, mapApproval(approval))
	}

	ret := WorkspaceSnapshot{
		Workspace:    mapWorkspaceWithRunProjection(item, projectionRun),
		Runs:         runs,
		Results:      mapResults(resultItems),
		Artifacts:    mapArtifacts(artifactItems),
		EventRecords: mapEventRecords(eventRecordItems),
		Decisions:    decisions,
		Approvals:    approvals,
	}
	ret.TaskProgress = mapWorkspaceTaskProgress(ret)
	if projectionRun != nil {
		var runSnapshot RunSnapshot
		if cachedRunSnapshot != nil &&
			cachedRunSnapshot.Run.ID == string(projectionRun.ID) &&
			cachedRunSnapshot.Run.WorkspaceID == string(workspaceID) {
			runSnapshot = *cachedRunSnapshot
		} else {
			var err error
			runSnapshot, err = readRunSnapshot(ctx, deps, projectionRun.ID)
			if err != nil {
				return WorkspaceSnapshot{}, err
			}
		}
		ret.TaskProgress = runSnapshot.TaskProgress
		ret.TaskDecompositions = runSnapshot.TaskDecompositions
		ret.ExecutionSections = runSnapshot.ExecutionSections
		ret.MemoryRecallTraces = runSnapshot.MemoryRecallTraces
	}
	ret.Outcome = mapWorkspaceOutcome(ret)
	return ret, nil
}

func workspaceModelFromView(item Workspace) *workspacemodel.Workspace {
	return &workspacemodel.Workspace{
		ID:               common.WorkspaceID(item.ID),
		Title:            item.Title,
		Description:      item.Description,
		WorkingDirectory: item.WorkingDirectory,
		State:            common.WorkspaceState(item.State),
		ActiveRunID:      common.RunID(item.ActiveRunID),
		DefaultSoulID:    common.SoulID(item.DefaultSoulID),
		SoulBindings:     workspaceSoulBindingsFromView(item.SoulBindings),
	}
}

func workspaceSoulBindingsFromView(input map[string]string) map[string]common.SoulID {
	if len(input) == 0 {
		return nil
	}
	ret := make(map[string]common.SoulID, len(input))
	for key, value := range input {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		ret[key] = common.SoulID(value)
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func readWorkspaceTaskProgress(ctx context.Context, deps readModelDependencies, workspaceID common.WorkspaceID) (TaskProgress, error) {
	snapshot, err := readWorkspaceSnapshot(ctx, deps, workspaceID, nil)
	if err != nil {
		return TaskProgress{}, err
	}
	if snapshot.TaskProgress.WorkspaceID != "" {
		return snapshot.TaskProgress, nil
	}
	return mapWorkspaceTaskProgress(snapshot), nil
}

func readRunTaskProgress(ctx context.Context, deps readModelDependencies, runID common.RunID) (TaskProgress, error) {
	snapshot, err := readRunSnapshot(ctx, deps, runID)
	if err != nil {
		return TaskProgress{}, err
	}
	return runSnapshotTaskProgress(snapshot), nil
}

func readWorkspaceContextBundle(ctx context.Context, deps readModelDependencies, workspaceID common.WorkspaceID) (ContextBundle, error) {
	snapshot, err := readWorkspaceSnapshot(ctx, deps, workspaceID, nil)
	if err != nil {
		return ContextBundle{}, err
	}
	if snapshot.TaskProgress.RunID != "" {
		runSnapshot, err := readRunSnapshot(ctx, deps, common.RunID(snapshot.TaskProgress.RunID))
		if err != nil {
			return ContextBundle{}, err
		}
		return mapRunContextBundle(runSnapshot), nil
	}
	return mapWorkspaceContextBundle(snapshot), nil
}

func readWorkspaceTaskDecompositions(ctx context.Context, deps readModelDependencies, workspaceID common.WorkspaceID) ([]TaskDecomposition, error) {
	snapshot, err := readWorkspaceSnapshot(ctx, deps, workspaceID, nil)
	if err != nil {
		return nil, err
	}
	if len(snapshot.TaskDecompositions) > 0 {
		return snapshot.TaskDecompositions, nil
	}
	if snapshot.TaskProgress.RunID == "" {
		return nil, nil
	}
	runSnapshot, err := readRunSnapshot(ctx, deps, common.RunID(snapshot.TaskProgress.RunID))
	if err != nil {
		return nil, err
	}
	return runSnapshot.TaskDecompositions, nil
}

func selectWorkspaceProjectionRun(workspace *workspacemodel.Workspace, runs []*runmodel.Run, results []*resultmodel.Result, eventRecords []*eventstreammodel.EventRecord) *runmodel.Run {
	if workspace == nil || len(runs) == 0 {
		return nil
	}
	var latest *runmodel.Run
	var active *runmodel.Run
	runsByID := make(map[common.RunID]*runmodel.Run, len(runs))
	for _, item := range runs {
		if item == nil || item.WorkspaceID != workspace.ID {
			continue
		}
		runsByID[item.ID] = item
		if workspace.ActiveRunID != "" && item.ID == workspace.ActiveRunID {
			active = item
		}
		if latest == nil || string(item.ID) > string(latest.ID) {
			latest = item
		}
	}
	if active != nil && !isTerminalProjectionRunState(active.State) {
		return active
	}
	if latestByEvent := selectLatestRunByEvent(workspace.ID, runsByID, eventRecords); latestByEvent != nil {
		return latestByEvent
	}
	if resultRun := selectAcceptedResultRun(workspace.ID, runsByID, results); resultRun != nil {
		return resultRun
	}
	if active != nil {
		return active
	}
	return latest
}

func selectLatestRunByEvent(workspaceID common.WorkspaceID, runs map[common.RunID]*runmodel.Run, eventRecords []*eventstreammodel.EventRecord) *runmodel.Run {
	var selected *eventstreammodel.EventRecord
	for _, item := range eventRecords {
		if item == nil || item.WorkspaceID != workspaceID || item.RunID == "" {
			continue
		}
		if _, ok := runs[item.RunID]; !ok {
			continue
		}
		switch item.Topic {
		case events.TopicRunCreated, events.TopicRunStateChanged:
		default:
			continue
		}
		if selected == nil || item.OccurredAt >= selected.OccurredAt {
			selected = item
		}
	}
	if selected == nil {
		return nil
	}
	return runs[selected.RunID]
}

func mapWorkspaceWithRunProjection(workspace *workspacemodel.Workspace, run *runmodel.Run) Workspace {
	ret := mapWorkspace(workspace)
	if run == nil {
		return ret
	}
	ret.State = string(workspaceProjectionState(run.State))
	if isTerminalProjectionRunState(run.State) {
		ret.ActiveRunID = ""
	} else {
		ret.ActiveRunID = string(run.ID)
	}
	return ret
}

func selectAcceptedResultRun(workspaceID common.WorkspaceID, runs map[common.RunID]*runmodel.Run, results []*resultmodel.Result) *runmodel.Run {
	var selected *resultmodel.Result
	for _, item := range results {
		if item == nil || item.WorkspaceID != workspaceID || item.State != common.ResultStateAccepted || item.RunID == "" {
			continue
		}
		run := runs[item.RunID]
		if run == nil || run.State != common.RunStateCompleted {
			continue
		}
		if selected == nil || resultProjectionTime(item) >= resultProjectionTime(selected) {
			selected = item
		}
	}
	if selected == nil {
		return nil
	}
	return runs[selected.RunID]
}

func resultProjectionTime(item *resultmodel.Result) string {
	if item == nil {
		return ""
	}
	if item.UpdatedAt != "" {
		return item.UpdatedAt
	}
	return item.CreatedAt
}

func workspaceProjectionState(runState common.RunState) common.WorkspaceState {
	switch runState {
	case common.RunStateCreated:
		return common.WorkspaceStateReady
	case common.RunStateRunning:
		return common.WorkspaceStateRunning
	case common.RunStateBlockedOnDecision:
		return common.WorkspaceStateWaitingDecision
	case common.RunStateBlockedOnApproval:
		return common.WorkspaceStateBlocked
	case common.RunStateValidating:
		return common.WorkspaceStateValidating
	case common.RunStateCompleted:
		return common.WorkspaceStateCompleted
	case common.RunStateFailed, common.RunStateCancelled, common.RunStateSuperseded:
		return common.WorkspaceStateFailed
	default:
		return common.WorkspaceStateActive
	}
}

func isTerminalProjectionRunState(state common.RunState) bool {
	switch state {
	case common.RunStateCompleted, common.RunStateFailed, common.RunStateCancelled, common.RunStateSuperseded:
		return true
	default:
		return false
	}
}

func decisionsForRun(ctx context.Context, deps readModelDependencies, run *runmodel.Run) ([]Decision, *decisionmodel.Ticket, *decisionmodel.Resolution, error) {
	items, err := deps.tickets.ListByRunID(ctx, run.ID)
	if err != nil {
		return nil, nil, nil, err
	}
	resolutions, err := resolutionsForTickets(ctx, deps.resolutions, items)
	if err != nil {
		return nil, nil, nil, err
	}

	ret := make([]Decision, 0, len(items))
	var selectedTicket *decisionmodel.Ticket
	var selectedResolution *decisionmodel.Resolution
	for _, item := range items {
		if item.RunID != run.ID {
			continue
		}
		resolution := resolutions[item.ID]
		ret = append(ret, mapDecision(item, resolution))
		if run.BlockingTicketID != "" && item.ID == run.BlockingTicketID {
			selectedTicket = item
			selectedResolution = resolution
			continue
		}
		if selectedTicket == nil || run.BlockingTicketID == "" {
			selectedTicket = item
			selectedResolution = resolution
		}
	}
	return ret, selectedTicket, selectedResolution, nil
}

func resolutionsForTickets(ctx context.Context, resolutions repository.DecisionResolutionRepository, items []*decisionmodel.Ticket) (map[common.TicketID]*decisionmodel.Resolution, error) {
	ret := make(map[common.TicketID]*decisionmodel.Resolution)
	if resolutions == nil || len(items) == 0 {
		return ret, nil
	}
	var mu sync.Mutex
	err := runReadModelTasks(funcsForResolvedTickets(items, func(item *decisionmodel.Ticket) func() error {
		return func() error {
			resolution, err := resolutions.GetByTicketID(ctx, item.ID)
			if errors.Is(err, common.ErrNotFound) {
				return nil
			}
			if err != nil {
				return err
			}
			mu.Lock()
			ret[item.ID] = resolution
			mu.Unlock()
			return nil
		}
	})...)
	if err != nil {
		return nil, err
	}
	return ret, nil
}

func funcsForResolvedTickets(items []*decisionmodel.Ticket, build func(*decisionmodel.Ticket) func() error) []func() error {
	if len(items) == 0 || build == nil {
		return nil
	}
	ret := make([]func() error, 0, len(items))
	for _, item := range items {
		if item == nil || item.State != common.DecisionTicketStateResolved {
			continue
		}
		ret = append(ret, build(item))
	}
	return ret
}

func approvalsForRun(ctx context.Context, deps readModelDependencies, run *runmodel.Run) ([]Approval, *approvalmodel.Approval, error) {
	items, err := deps.approvals.ListByRunID(ctx, run.ID)
	if err != nil {
		return nil, nil, err
	}

	ret := make([]Approval, 0, len(items))
	var selectedApproval *approvalmodel.Approval
	for _, item := range items {
		if item.RunID != run.ID {
			continue
		}
		ret = append(ret, mapApproval(item))
		if run.BlockingApprovalID != "" && item.ID == run.BlockingApprovalID {
			selectedApproval = item
			continue
		}
		if selectedApproval == nil || run.BlockingApprovalID == "" {
			selectedApproval = item
		}
	}
	return ret, selectedApproval, nil
}

func lastStep(steps []*runmodel.Step) *runmodel.Step {
	if len(steps) == 0 {
		return nil
	}
	return steps[len(steps)-1]
}

func mapToolCalls(items []*runmodel.ToolCall) []ToolCall {
	if len(items) == 0 {
		return nil
	}
	ret := make([]ToolCall, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		ret = append(ret, ToolCall{
			ID:          string(item.ID),
			WorkspaceID: string(item.WorkspaceID),
			RunID:       string(item.RunID),
			StepID:      string(item.StepID),
			ToolName:    item.ToolName,
			Arguments:   cloneReadModelMap(item.Arguments),
			State:       string(item.State),
			StartedAt:   item.StartedAt,
			EndedAt:     item.EndedAt,
			ExitSummary: item.ExitSummary,
			Output:      cloneReadModelMap(item.Output),
			Error:       item.Error,
		})
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func selectedRuntimeCapabilities(executionContext map[string]any, catalog map[string]RuntimeCapability) []RuntimeCapability {
	if len(executionContext) == 0 {
		return nil
	}
	ret := make([]RuntimeCapability, 0)
	seen := map[string]struct{}{}
	for _, raw := range readModelMapSliceValue(executionContext["mcpCapabilities"]) {
		item := runtimeCapabilityFromContext(raw)
		if item.ID == "" {
			continue
		}
		if _, ok := seen[item.ID]; ok {
			continue
		}
		seen[item.ID] = struct{}{}
		ret = append(ret, item)
	}
	for _, id := range readModelStringSliceValue(executionContext["capabilityIds"]) {
		if _, ok := seen[id]; ok {
			continue
		}
		if item, ok := catalog[id]; ok {
			ret = append(ret, item)
		} else {
			ret = append(ret, RuntimeCapability{ID: id, DisplayName: id})
		}
		seen[id] = struct{}{}
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func usedRuntimeCapabilities(snapshot RunSnapshot, catalog map[string]RuntimeCapability) []RuntimeCapability {
	if len(snapshot.ToolCalls) == 0 {
		return nil
	}
	selected := map[string]RuntimeCapability{}
	for _, item := range snapshot.SelectedCapabilities {
		selected[item.ID] = item
	}
	used := map[string]RuntimeCapability{}
	order := make([]string, 0)
	for _, toolCall := range snapshot.ToolCalls {
		identity := runtimeCapabilityIdentity(toolCall)
		if identity.ID == "" {
			continue
		}
		item, ok := used[identity.ID]
		if !ok {
			if loaded, exists := catalog[identity.ID]; exists {
				item = loaded
			} else if selectedItem, exists := selected[identity.ID]; exists {
				item = selectedItem
			} else {
				item = identity
			}
			order = append(order, identity.ID)
		}
		item.UseCount++
		item.EvidenceRefs = compactUniqueStrings(append(item.EvidenceRefs, toolCall.ID, toolCall.StepID))
		used[identity.ID] = item
	}
	ret := make([]RuntimeCapability, 0, len(order))
	for _, id := range order {
		ret = append(ret, used[id])
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func runtimeCapabilityCatalog(ctx context.Context, deps readModelDependencies, executionContext map[string]any, toolCalls []ToolCall) map[string]RuntimeCapability {
	if deps.mcpCapabilities == nil || !requiresRuntimeCapabilityCatalog(executionContext, toolCalls) {
		return nil
	}
	items, err := deps.mcpCapabilities.List(ctx)
	if err != nil {
		return nil
	}
	return runtimeCapabilityCatalogFromItems(executionContext, toolCalls, items)
}

func runtimeCapabilityCatalogFromItems(executionContext map[string]any, toolCalls []ToolCall, items []*mcpmodel.Capability) map[string]RuntimeCapability {
	if items == nil || !requiresRuntimeCapabilityCatalog(executionContext, toolCalls) {
		return nil
	}
	ret := make(map[string]RuntimeCapability, len(items))
	for _, item := range items {
		capability := runtimeCapabilityFromModel(item)
		if capability.ID == "" {
			continue
		}
		ret[capability.ID] = capability
	}
	return ret
}

func requiresRuntimeCapabilityCatalog(executionContext map[string]any, toolCalls []ToolCall) bool {
	if len(readModelStringSliceValue(executionContext["capabilityIds"])) > 0 {
		return true
	}
	for _, toolCall := range toolCalls {
		if runtimeCapabilityIdentity(toolCall).ID != "" {
			return true
		}
	}
	return false
}

func runtimeCapabilityIdentity(toolCall ToolCall) RuntimeCapability {
	source := toolCall.Output
	if len(source) == 0 {
		source = toolCall.Arguments
	}
	serverID := readModelStringValue(source["serverId"])
	operation := readModelStringValue(source["operation"])
	var kind string
	var name string
	switch operation {
	case "mcp/tool_call":
		kind = mcpmodel.CapabilityKindTool
		name = readModelStringValue(source["tool"])
	case "mcp/resource_read":
		kind = mcpmodel.CapabilityKindResource
		name = readModelStringValue(source["uri"])
	case "mcp/prompt_get":
		kind = mcpmodel.CapabilityKindPrompt
		name = readModelStringValue(source["prompt"])
	}
	if serverID == "" || kind == "" || name == "" {
		return RuntimeCapability{}
	}
	id := defaultRuntimeCapabilityID(serverID, kind, name)
	return RuntimeCapability{
		ID:          id,
		ServerID:    serverID,
		Kind:        kind,
		Name:        name,
		DisplayName: name,
	}
}

func runtimeCapabilityFromModel(item *mcpmodel.Capability) RuntimeCapability {
	if item == nil {
		return RuntimeCapability{}
	}
	return RuntimeCapability{
		ID:             string(item.ID),
		ServerID:       string(item.ServerID),
		Kind:           item.Kind,
		Name:           item.Name,
		DisplayName:    firstNonEmptyReadModel(item.DisplayName, item.Name, string(item.ID)),
		Description:    item.Description,
		Category:       item.Category,
		Tags:           cloneReadModelStringSlice(item.Tags),
		RiskLevel:      item.RiskLevel,
		Enabled:        item.Enabled,
		ApprovalPolicy: item.ApprovalPolicy,
		Status:         item.Status,
	}
}

func runtimeCapabilityFromContext(raw map[string]any) RuntimeCapability {
	id := readModelStringValue(raw["id"])
	return RuntimeCapability{
		ID:             id,
		ServerID:       readModelStringValue(raw["serverId"]),
		Kind:           readModelStringValue(raw["kind"]),
		Name:           readModelStringValue(raw["name"]),
		DisplayName:    firstNonEmptyReadModel(readModelStringValue(raw["displayName"]), readModelStringValue(raw["name"]), id),
		Description:    readModelStringValue(raw["description"]),
		Category:       readModelStringValue(raw["category"]),
		Tags:           readModelStringSliceValue(raw["tags"]),
		RiskLevel:      readModelStringValue(raw["riskLevel"]),
		Enabled:        readModelBoolValue(raw["enabled"]),
		ApprovalPolicy: readModelStringValue(raw["approvalPolicy"]),
		Status:         readModelStringValue(raw["status"]),
	}
}

func defaultRuntimeCapabilityID(serverID string, kind string, name string) string {
	return strings.TrimSpace(serverID) + ":" + strings.TrimSpace(kind) + ":" + url.PathEscape(strings.TrimSpace(name))
}

func mapPatches(items []*runmodel.Patch) []Patch {
	if len(items) == 0 {
		return nil
	}
	ret := make([]Patch, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		ret = append(ret, Patch{
			ID:          string(item.ID),
			WorkspaceID: string(item.WorkspaceID),
			RunID:       string(item.RunID),
			StepID:      string(item.StepID),
			ToolCallID:  string(item.ToolCallID),
			PatchType:   item.PatchType,
			TargetRef:   item.TargetRef,
			Summary:     item.Summary,
			State:       string(item.State),
			CreatedAt:   item.CreatedAt,
			AppliedAt:   item.AppliedAt,
		})
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func mapValidationResults(items []*runmodel.ValidationResult) []ValidationResult {
	if len(items) == 0 {
		return nil
	}
	ret := make([]ValidationResult, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		ret = append(ret, ValidationResult{
			ID:             string(item.ID),
			WorkspaceID:    string(item.WorkspaceID),
			RunID:          string(item.RunID),
			StepID:         string(item.StepID),
			ValidationKind: item.ValidationKind,
			TargetRef:      item.TargetRef,
			State:          string(item.State),
			Summary:        item.Summary,
			EvidenceRef:    item.EvidenceRef,
			StartedAt:      item.StartedAt,
			EndedAt:        item.EndedAt,
		})
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func mapRuntimeCheckpoints(items []*runmodel.RuntimeCheckpoint) []RuntimeCheckpoint {
	if len(items) == 0 {
		return nil
	}
	ret := make([]RuntimeCheckpoint, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		ret = append(ret, RuntimeCheckpoint{
			ID:                 string(item.ID),
			WorkspaceID:        string(item.WorkspaceID),
			RunID:              string(item.RunID),
			CheckpointType:     string(item.CheckpointType),
			CurrentStepID:      string(item.CurrentStepID),
			RunStateSnapshot:   string(item.RunStateSnapshot),
			BlockingTicketID:   string(item.BlockingTicketID),
			BlockingApprovalID: string(item.BlockingApprovalID),
			ThreadRef:          item.ThreadRef,
			OccurredAt:         item.OccurredAt,
			RestorableUntil:    item.RestorableUntil,
			Data:               cloneReadModelMap(item.Data),
		})
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func mapResults(items []*resultmodel.Result) []Result {
	if len(items) == 0 {
		return nil
	}
	ret := make([]Result, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		artifactRefs := make([]string, 0, len(item.ArtifactRefs))
		for _, ref := range item.ArtifactRefs {
			if ref != "" {
				artifactRefs = append(artifactRefs, string(ref))
			}
		}
		ret = append(ret, Result{
			ID:            string(item.ID),
			WorkspaceID:   string(item.WorkspaceID),
			RunID:         string(item.RunID),
			GoalID:        string(item.GoalID),
			DoDID:         string(item.DoDID),
			State:         string(item.State),
			Title:         item.Title,
			Summary:       item.Summary,
			SourceType:    item.SourceType,
			SourceRef:     item.SourceRef,
			EvidenceRefs:  cloneReadModelStringSlice(item.EvidenceRefs),
			ArtifactRefs:  artifactRefs,
			DoDAssessment: mapDoDAssessment(item.DoDAssessment),
			SummaryDetail: mapResultSummary(item.SummaryDetail),
			CreatedAt:     item.CreatedAt,
			UpdatedAt:     item.UpdatedAt,
		})
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func mapResultSummary(item resultmodel.ResultSummary) *ResultSummary {
	if item.GoalOutcome == "" &&
		len(item.DoDItems) == 0 &&
		item.ImplementationSummary == "" &&
		item.VerificationSummary == "" &&
		len(item.EvidenceRefs) == 0 &&
		len(item.ArtifactRefs) == 0 &&
		len(item.KnownLimitations) == 0 &&
		len(item.ResidualRisks) == 0 &&
		len(item.NextActions) == 0 {
		return nil
	}
	artifactRefs := make([]string, 0, len(item.ArtifactRefs))
	for _, ref := range item.ArtifactRefs {
		if ref != "" {
			artifactRefs = append(artifactRefs, string(ref))
		}
	}
	ret := &ResultSummary{
		GoalOutcome:           item.GoalOutcome,
		ImplementationSummary: item.ImplementationSummary,
		VerificationSummary:   item.VerificationSummary,
		EvidenceRefs:          cloneReadModelStringSlice(item.EvidenceRefs),
		ArtifactRefs:          artifactRefs,
		KnownLimitations:      cloneReadModelStringSlice(item.KnownLimitations),
		ResidualRisks:         cloneReadModelStringSlice(item.ResidualRisks),
		NextActions:           cloneReadModelStringSlice(item.NextActions),
	}
	if len(item.DoDItems) > 0 {
		ret.DoDItems = make([]DoDAssessmentItem, 0, len(item.DoDItems))
		for _, assessmentItem := range item.DoDItems {
			ret.DoDItems = append(ret.DoDItems, DoDAssessmentItem{
				Item:          assessmentItem.Item,
				State:         assessmentItem.State,
				EvidenceRefs:  cloneReadModelStringSlice(assessmentItem.EvidenceRefs),
				FailureReason: assessmentItem.FailureReason,
				Confidence:    assessmentItem.Confidence,
				SourceNote:    assessmentItem.SourceNote,
			})
		}
	}
	return ret
}

func mapDoDAssessment(item resultmodel.DoDAssessment) *DoDAssessment {
	if item.Summary == "" && len(item.Items) == 0 && !item.Satisfied {
		return nil
	}
	ret := &DoDAssessment{
		Satisfied: item.Satisfied,
		Summary:   item.Summary,
	}
	if len(item.Items) > 0 {
		ret.Items = make([]DoDAssessmentItem, 0, len(item.Items))
		for _, assessmentItem := range item.Items {
			ret.Items = append(ret.Items, DoDAssessmentItem{
				Item:          assessmentItem.Item,
				State:         assessmentItem.State,
				EvidenceRefs:  cloneReadModelStringSlice(assessmentItem.EvidenceRefs),
				FailureReason: assessmentItem.FailureReason,
				Confidence:    assessmentItem.Confidence,
				SourceNote:    assessmentItem.SourceNote,
			})
		}
	}
	return ret
}

func mapArtifacts(items []*resultmodel.Artifact) []Artifact {
	if len(items) == 0 {
		return nil
	}
	ret := make([]Artifact, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		ret = append(ret, Artifact{
			ID:                 string(item.ID),
			WorkspaceID:        string(item.WorkspaceID),
			RunID:              string(item.RunID),
			ResultID:           string(item.ResultID),
			StepID:             string(item.StepID),
			ToolCallID:         string(item.ToolCallID),
			PatchID:            string(item.PatchID),
			ValidationResultID: string(item.ValidationResultID),
			ArtifactType:       item.ArtifactType,
			Category:           item.Category,
			Name:               item.Name,
			Summary:            item.Summary,
			URI:                item.URI,
			ContentType:        item.ContentType,
			State:              string(item.State),
			EvidenceRefs:       cloneReadModelStringSlice(item.EvidenceRefs),
			Metadata:           cloneReadModelMap(item.Metadata),
			CreatedAt:          item.CreatedAt,
			UpdatedAt:          item.UpdatedAt,
		})
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func mapEventRecords(items []*eventstreammodel.EventRecord) []EventRecord {
	if len(items) == 0 {
		return nil
	}
	ret := make([]EventRecord, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		ret = append(ret, EventRecord{
			ID:           string(item.ID),
			Version:      item.Version,
			Topic:        item.Topic,
			OccurredAt:   item.OccurredAt,
			SourceModule: item.SourceModule,
			SubjectType:  item.SubjectType,
			SubjectID:    item.SubjectID,
			WorkspaceID:  string(item.WorkspaceID),
			RunID:        string(item.RunID),
			StepID:       string(item.StepID),
			Payload:      cloneReadModelMap(item.Payload),
		})
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func mapMemoryRecallTraces(items []*memorymodel.MemoryRecallTrace) []MemoryRecallTrace {
	if len(items) == 0 {
		return nil
	}
	ret := make([]MemoryRecallTrace, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		ret = append(ret, MemoryRecallTrace{
			ID:                  string(item.ID),
			WorkspaceID:         string(item.WorkspaceID),
			RunID:               string(item.RunID),
			StepID:              string(item.StepID),
			GraphNodeID:         item.GraphNodeID,
			Stage:               item.Stage,
			Source:              item.Source,
			Policy:              item.Policy,
			Status:              item.Status,
			ProviderID:          item.ProviderID,
			Operation:           item.Operation,
			QueryTextPreview:    item.QueryTextPreview,
			QueryTextSha256:     item.QueryTextSha256,
			MinScore:            item.MinScore,
			Limit:               item.Limit,
			EmbeddingModel:      item.EmbeddingModel,
			VectorDimension:     item.VectorDimension,
			RetrievedMemoryIDs:  memoryIDsToReadModelStrings(item.RetrievedMemoryIDs),
			RetrievedMemoryHits: mapMemoryRecallHits(item.RetrievedMemoryHits),
			Reason:              item.Reason,
			OccurredAt:          item.OccurredAt,
			EvidenceRef:         item.EvidenceRef,
		})
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func mapMemoryRecallHits(items []memorymodel.MemoryRecallHit) []MemoryRecallHit {
	if len(items) == 0 {
		return nil
	}
	ret := make([]MemoryRecallHit, 0, len(items))
	for _, item := range items {
		ret = append(ret, MemoryRecallHit{
			ID:              string(item.ID),
			Summary:         item.Summary,
			VectorScore:     item.VectorScore,
			VectorDimension: item.VectorDimension,
		})
	}
	return ret
}

func memoryIDsToReadModelStrings(input []common.MemoryID) []string {
	if len(input) == 0 {
		return nil
	}
	ret := make([]string, 0, len(input))
	for _, item := range input {
		ret = append(ret, string(item))
	}
	return ret
}

func cloneReadModelMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	out := make(map[string]any, len(input))
	for k, v := range input {
		out[k] = v
	}
	return out
}

func cloneReadModelStringSlice(input []string) []string {
	if len(input) == 0 {
		return nil
	}
	return append([]string(nil), input...)
}

func readModelStringValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	default:
		return ""
	}
}

func readModelBoolValue(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	default:
		return false
	}
}

func readModelMapValue(value any) map[string]any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneReadModelMap(typed)
	default:
		return nil
	}
}

func readModelMapSliceValue(value any) []map[string]any {
	switch typed := value.(type) {
	case []map[string]any:
		ret := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			ret = append(ret, cloneReadModelMap(item))
		}
		return ret
	case []any:
		ret := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			if mapped, ok := item.(map[string]any); ok {
				ret = append(ret, cloneReadModelMap(mapped))
			}
		}
		return ret
	default:
		return nil
	}
}

func readModelStringSliceValue(value any) []string {
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		ret := make([]string, 0, len(typed))
		for _, item := range typed {
			if text, ok := item.(string); ok && text != "" {
				ret = append(ret, text)
			}
		}
		return ret
	default:
		return nil
	}
}

func mapRunStatus(snapshot RunSnapshot) RunStatus {
	ret := RunStatus{
		ID:                 snapshot.Run.ID,
		WorkspaceID:        snapshot.Run.WorkspaceID,
		State:              snapshot.Run.State,
		StepCount:          len(snapshot.Steps),
		DecisionCount:      len(snapshot.Decisions),
		ApprovalCount:      len(snapshot.Approvals),
		BlockingTicketID:   snapshot.Run.BlockingTicketID,
		BlockingApprovalID: snapshot.Run.BlockingApprovalID,
		FailureReason:      snapshot.Run.FailureReason,
	}
	if snapshot.Step != nil {
		ret.CurrentStepID = snapshot.Step.ID
		ret.CurrentStepKind = snapshot.Step.Kind
		ret.CurrentStepState = snapshot.Step.State
	}
	return ret
}

func mapRunObservability(snapshot RunSnapshot) RunObservability {
	timeline := runtimeTimeline(snapshot)
	return RunObservability{
		Run:                  snapshot.Run,
		Outcome:              snapshot.Outcome,
		Progress:             runtimeProgress(snapshot),
		TaskProgress:         snapshot.TaskProgress,
		TaskDecompositions:   snapshot.TaskDecompositions,
		ExecutionSections:    snapshot.ExecutionSections,
		MemoryRecallTraces:   snapshot.MemoryRecallTraces,
		Evidence:             runtimeEvidenceSummary(snapshot, len(timelineEvents(timeline))),
		LatestCheckpoint:     latestRuntimeCheckpoint(snapshot.RuntimeCheckpoints),
		SelectedCapabilities: snapshot.SelectedCapabilities,
		UsedCapabilities:     snapshot.UsedCapabilities,
		Timeline:             timeline,
	}
}

func mapWorkspaceTaskProgress(snapshot WorkspaceSnapshot) TaskProgress {
	language := "" // WorkspaceSnapshot does not carry dialogLanguage; use default
	ret := TaskProgress{
		WorkspaceID: snapshot.Workspace.ID,
		State:       snapshot.Workspace.State,
		Recovery: RecoveryPlan{
			Mode:       "idle",
			NextAction: "start_workspace_execution",
			Reason:     "workspace has no active run",
		},
	}
	if snapshot.Workspace.ActiveRunID != "" {
		ret.RunID = snapshot.Workspace.ActiveRunID
		ret.Recovery.RunID = snapshot.Workspace.ActiveRunID
		ret.Recovery.Mode = "continue"
		ret.Recovery.NextAction = "inspect_active_run"
		ret.Recovery.Reason = "workspace has an active run projection"
	}
	for _, item := range snapshot.Decisions {
		if isResolvedDecisionState(item.State) {
			continue
		}
		ret.Blockers = append(ret.Blockers, ProgressBlocker{
			ID:          item.ID,
			Kind:        "decision",
			State:       item.State,
			Reason:      firstNonEmptyReadModel(item.Question, item.Reason),
			EvidenceRef: item.StepID,
		})
		if ret.Recovery.BlockingTicketID == "" {
			ret.Recovery.Mode = "wait_decision"
			ret.Recovery.NextAction = "resolve_decision_ticket"
			ret.Recovery.BlockingTicketID = item.ID
			ret.Recovery.StepID = item.StepID
			ret.Recovery.Reason = firstNonEmptyReadModel(item.Question, item.Reason)
		}
	}
	for _, item := range snapshot.Approvals {
		if isResolvedApprovalState(item.State) {
			continue
		}
		ret.Blockers = append(ret.Blockers, ProgressBlocker{
			ID:          item.ID,
			Kind:        "approval",
			State:       item.State,
			Reason:      item.Reason,
			EvidenceRef: item.StepID,
		})
		if ret.Recovery.BlockingApprovalID == "" && ret.Recovery.BlockingTicketID == "" {
			ret.Recovery.Mode = "wait_approval"
			ret.Recovery.NextAction = "resolve_approval"
			ret.Recovery.BlockingApprovalID = item.ID
			ret.Recovery.StepID = item.StepID
			ret.Recovery.Reason = item.Reason
		}
	}
	ret.Milestones = []ProgressMilestone{
		{ID: "workspace_defined", Title: "Workspace defined", State: "completed", EvidenceRefs: []string{snapshot.Workspace.ID}},
		{ID: "formal_result", Title: localizedMilestoneTitle(language, "formal_result"), State: progressStateFromCount(len(snapshot.Results), false, snapshot.Workspace.State == string(common.WorkspaceStateCompleted)), EvidenceRefs: idsFromResults(snapshot.Results)},
	}
	ret.Items = []ProgressItem{
		{ID: "runs", Kind: "run", Title: "Runs", State: progressStateFromCount(len(snapshot.Runs), false, false)},
		{ID: "results", Kind: "result", Title: "Results", State: progressStateFromCount(len(snapshot.Results), false, false), EvidenceRefs: idsFromResults(snapshot.Results)},
		{ID: "artifacts", Kind: "artifact", Title: "Artifacts", State: progressStateFromCount(len(snapshot.Artifacts), false, false), EvidenceRefs: idsFromArtifacts(snapshot.Artifacts)},
	}
	ret.CurrentMilestone = currentProgressMilestone(ret.Milestones)
	ret.EvidenceRefs = compactUniqueStrings(append(idsFromResults(snapshot.Results), idsFromArtifacts(snapshot.Artifacts)...))
	ret.Summary = taskProgressSummary(ret, nil)
	return ret
}

func mapTaskProgress(snapshot RunSnapshot) TaskProgress {
	language := snapshotLanguage(snapshot)
	latestCheckpoint := latestRuntimeCheckpoint(snapshot.RuntimeCheckpoints)
	results := runScopedResults(snapshot)
	artifacts := runScopedArtifacts(snapshot)
	ret := TaskProgress{
		WorkspaceID: snapshot.Run.WorkspaceID,
		RunID:       snapshot.Run.ID,
		State:       snapshot.Run.State,
		Recovery: RecoveryPlan{
			Mode:       "continue",
			NextAction: localizedRecoveryNextAction(language),
			RunID:      snapshot.Run.ID,
		},
	}
	if snapshot.Step != nil {
		ret.Recovery.StepID = snapshot.Step.ID
	}
	if latestCheckpoint != nil {
		ret.Recovery.CheckpointID = latestCheckpoint.ID
	}

	ret.Blockers = taskProgressBlockers(snapshot)
	ret.Milestones = []ProgressMilestone{
		{ID: "run_created", Title: localizedMilestoneTitle(language, "run_created"), State: "completed", EvidenceRefs: []string{snapshot.Run.ID}},
		{ID: "execution_steps", Title: localizedMilestoneTitle(language, "execution_steps"), State: stepProgressState(snapshot), EvidenceRefs: idsFromSteps(snapshot.Steps)},
		{ID: "governance", Title: localizedMilestoneTitle(language, "governance"), State: governanceProgressState(snapshot), EvidenceRefs: governanceEvidenceRefs(snapshot)},
		{ID: "validation", Title: localizedMilestoneTitle(language, "validation"), State: validationProgressState(snapshot), EvidenceRefs: idsFromValidations(snapshot.ValidationResults)},
		{ID: "formal_result", Title: localizedMilestoneTitle(language, "formal_result"), State: resultProgressState(snapshot), EvidenceRefs: append(idsFromResults(results), idsFromArtifacts(artifacts)...)},
	}
	ret.Items = taskProgressItems(snapshot)
	ret.CurrentMilestone = currentProgressMilestone(ret.Milestones)
	ret.EvidenceRefs = taskProgressEvidenceRefs(snapshot)
	ret.Recovery = taskProgressRecovery(snapshot, latestCheckpoint, ret.Blockers, ret.Recovery)
	ret.Summary = taskProgressSummary(ret, &snapshot)
	return ret
}

func mapWorkspaceContextBundle(snapshot WorkspaceSnapshot) ContextBundle {
	progress := snapshot.TaskProgress
	ret := ContextBundle{
		WorkspaceID:  snapshot.Workspace.ID,
		Mode:         "workspace",
		Summary:      "workspace context bundle without active run",
		TaskProgress: progress,
		Recovery:     progress.Recovery,
		Sections: []ContextSection{
			{
				ID:       "workspace",
				Kind:     "workspace",
				Title:    "Workspace",
				Priority: 100,
				State:    snapshot.Workspace.State,
				Summary:  snapshot.Workspace.Title,
				Data: map[string]any{
					"id":               snapshot.Workspace.ID,
					"title":            snapshot.Workspace.Title,
					"description":      snapshot.Workspace.Description,
					"workingDirectory": snapshot.Workspace.WorkingDirectory,
				},
			},
			{
				ID:           "task_progress",
				Kind:         "task_progress",
				Title:        "Task progress",
				Priority:     90,
				State:        progress.State,
				Summary:      progress.Summary,
				EvidenceRefs: progress.EvidenceRefs,
			},
		},
	}
	ret.CompressionCandidates = contextCompressionCandidates(ret)
	ret.CompressedContext = compressedContextFromBundle(ret)
	return ret
}

func mapRunContextBundle(snapshot RunSnapshot) ContextBundle {
	progress := runSnapshotTaskProgress(snapshot)
	timeline := runtimeTimeline(snapshot)
	recentEvidence := contextRecentEvidence(timeline, 12)
	ret := ContextBundle{
		WorkspaceID:      snapshot.Run.WorkspaceID,
		RunID:            snapshot.Run.ID,
		Mode:             "run_recovery",
		Summary:          contextBundleSummary(snapshot, progress),
		TaskProgress:     progress,
		Recovery:         progress.Recovery,
		LatestCheckpoint: latestRuntimeCheckpoint(snapshot.RuntimeCheckpoints),
		RecentEvidence:   recentEvidence,
	}
	ret.Sections = []ContextSection{
		contextWorkspaceSection(snapshot),
		contextRunSection(snapshot),
		contextGoalSection(snapshot),
		contextDoDSection(snapshot),
		contextExecutionContextSection(snapshot),
		contextTaskProgressSection(progress),
		contextTaskDecompositionSection(snapshot.TaskDecompositions),
		contextExecutionSectionsSection(snapshot.ExecutionSections),
		contextMemoryRecallSection(snapshot.MemoryRecallTraces),
		contextGovernanceSection(snapshot),
		contextFormalOutputSection(snapshot),
		contextRecentEvidenceSection(recentEvidence),
		contextRecoverySection(progress.Recovery),
	}
	ret.Sections = compactContextSections(ret.Sections)
	ret.CompressionCandidates = contextCompressionCandidates(ret)
	ret.CompressedContext = compressedContextFromBundle(ret)
	return ret
}

func compressedContextFromBundle(bundle ContextBundle) CompressedContext {
	language := "" // bundle does not carry dialogLanguage; localized defaults use English
	sectionRefs := make([]string, 0, len(bundle.Sections))
	for _, section := range bundle.Sections {
		sectionRefs = append(sectionRefs, section.ID)
	}
	sourceEvidenceRefs := compactUniqueStrings(append(bundle.TaskProgress.EvidenceRefs, contextEvidenceRefs(bundle.RecentEvidence)...))
	return CompressedContext{
		Version:            "workorch.compressedContext.v1",
		Mode:               "deterministic_recovery_summary",
		WorkspaceID:        bundle.WorkspaceID,
		RunID:              bundle.RunID,
		Summary:            bundle.Summary,
		CurrentMilestone:   bundle.TaskProgress.CurrentMilestone,
		Milestones:         bundle.TaskProgress.Milestones,
		Blockers:           bundle.TaskProgress.Blockers,
		Recovery:           bundle.Recovery,
		RecentEvidence:     lastContextEvidence(bundle.RecentEvidence, 6),
		ResultRefs:         resultRefsFromTaskProgress(bundle.TaskProgress),
		SourceSectionRefs:  compactUniqueStrings(sectionRefs),
		SourceEvidenceRefs: sourceEvidenceRefs,
		RetentionPolicy:    localizedRetentionPolicy(language),
	}
}

func compressedContextFromRunSnapshot(snapshot RunSnapshot) CompressedContext {
	language := snapshotLanguage(snapshot)
	progress := runSnapshotTaskProgress(snapshot)
	timeline := runtimeTimeline(snapshot)
	recentEvidence := contextRecentEvidence(timeline, 12)
	sourceEvidenceRefs := compactUniqueStrings(append(progress.EvidenceRefs, contextEvidenceRefs(recentEvidence)...))
	return CompressedContext{
		Version:            "workorch.compressedContext.v1",
		Mode:               "deterministic_recovery_summary",
		WorkspaceID:        snapshot.Run.WorkspaceID,
		RunID:              snapshot.Run.ID,
		Summary:            contextBundleSummary(snapshot, progress),
		CurrentMilestone:   progress.CurrentMilestone,
		Milestones:         progress.Milestones,
		Blockers:           progress.Blockers,
		Recovery:           progress.Recovery,
		RecentEvidence:     lastContextEvidence(recentEvidence, 6),
		ResultRefs:         resultRefsFromTaskProgress(progress),
		SourceSectionRefs:  compressedContextSourceSectionRefs(snapshot, recentEvidence),
		SourceEvidenceRefs: sourceEvidenceRefs,
		RetentionPolicy:    localizedRetentionPolicy(language),
	}
}

func runSnapshotTaskProgress(snapshot RunSnapshot) TaskProgress {
	if snapshot.TaskProgress.RunID == snapshot.Run.ID && snapshot.TaskProgress.WorkspaceID == snapshot.Run.WorkspaceID {
		return snapshot.TaskProgress
	}
	return mapTaskProgress(snapshot)
}

func runSnapshotTaskDecompositions(snapshot RunSnapshot) []TaskDecomposition {
	if len(snapshot.TaskDecompositions) > 0 {
		return snapshot.TaskDecompositions
	}
	return mapTaskDecompositions(snapshot)
}

func compressedContextSourceSectionRefs(snapshot RunSnapshot, recentEvidence []ContextEvidence) []string {
	sections := []ContextSection{
		{ID: "workspace", Priority: 100},
		{ID: "run", Priority: 100},
	}
	if len(readModelMapValue(snapshot.Run.ExecutionContext["goal"])) > 0 {
		sections = append(sections, ContextSection{ID: "goal", Priority: 95})
	}
	if len(readModelMapValue(snapshot.Run.ExecutionContext["dod"])) > 0 {
		sections = append(sections, ContextSection{ID: "dod", Priority: 95})
	}
	if hasNonDefinitionExecutionContext(snapshot.Run.ExecutionContext) {
		sections = append(sections, ContextSection{ID: "execution_context", Priority: 85})
	}
	sections = append(sections, ContextSection{ID: "task_progress", Priority: 90})
	if len(snapshot.TaskDecompositions) > 0 {
		sections = append(sections, ContextSection{ID: "task_decomposition", Priority: 85})
	}
	if len(snapshot.ExecutionSections) > 0 {
		sections = append(sections, ContextSection{ID: "execution_sections", Priority: 84})
	}
	if len(snapshot.MemoryRecallTraces) > 0 {
		sections = append(sections, ContextSection{ID: "memory_recall", Priority: 83})
	}
	if len(snapshot.Decisions) > 0 || len(snapshot.Approvals) > 0 {
		sections = append(sections, ContextSection{ID: "governance", Priority: 80})
	}
	if len(runScopedResults(snapshot)) > 0 || len(runScopedArtifacts(snapshot)) > 0 {
		sections = append(sections, ContextSection{ID: "formal_outputs", Priority: 90})
	}
	if len(recentEvidence) > 0 {
		sections = append(sections, ContextSection{ID: "recent_evidence", Priority: 70})
	}
	sections = append(sections, ContextSection{ID: "recovery", Priority: 100})
	sections = compactContextSections(sections)
	refs := make([]string, 0, len(sections))
	for _, section := range sections {
		refs = append(refs, section.ID)
	}
	return compactUniqueStrings(refs)
}

func hasNonDefinitionExecutionContext(executionContext map[string]any) bool {
	for key := range executionContext {
		if key != "goal" && key != "dod" && key != "compressedContext" {
			return true
		}
	}
	return false
}

func mapTaskDecompositions(snapshot RunSnapshot) []TaskDecomposition {
	if len(snapshot.Steps) == 0 {
		return nil
	}
	stepByNodeRoot := taskExecutionStepsByRoot(snapshot.Steps)
	ret := make([]TaskDecomposition, 0)
	for _, step := range snapshot.Steps {
		graph := taskGraphFromValue(step.Output["executionGraph"])
		nodes := readModelMapSliceValue(graph["nodes"])
		if len(nodes) == 0 {
			continue
		}
		rootStepID := step.ID
		executedByNode := stepByNodeRoot[rootStepID]
		item := TaskDecomposition{
			ID:          rootStepID + "/task-decomposition",
			WorkspaceID: snapshot.Run.WorkspaceID,
			RunID:       snapshot.Run.ID,
			RootStepID:  rootStepID,
			State:       "pending",
			Summary:     timelineStepSummary(step.Output),
			GraphBranch: readModelStringValue(step.Output["graphBranch"]),
			NodeTotal:   len(nodes),
			EvidenceRefs: compactUniqueStrings([]string{
				rootStepID,
			}),
		}
		for _, node := range nodes {
			taskNode := taskDecompositionNode(node, executedByNode)
			item.Nodes = append(item.Nodes, taskNode)
			item.EvidenceRefs = append(item.EvidenceRefs, taskNode.EvidenceRefs...)
			switch taskNode.State {
			case "completed":
				item.NodeCompleted++
			case "failed":
				item.NodeFailed++
			case "running":
				item.NodeRunning++
			}
		}
		item.State = taskDecompositionState(item)
		item.EvidenceRefs = compactUniqueStrings(item.EvidenceRefs)
		ret = append(ret, item)
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func taskExecutionStepsByRoot(steps []presenter.StepView) map[string]map[string]presenter.StepView {
	ret := map[string]map[string]presenter.StepView{}
	for _, step := range steps {
		nodeID := readModelStringValue(step.Input["graphNodeId"])
		if nodeID == "" {
			continue
		}
		rootStepID := readModelStringValue(step.Input["graphRootStepId"])
		if rootStepID == "" {
			rootStepID = graphRootStepIDFromStepID(step.ID)
		}
		if rootStepID == "" {
			continue
		}
		if ret[rootStepID] == nil {
			ret[rootStepID] = map[string]presenter.StepView{}
		}
		ret[rootStepID][nodeID] = step
	}
	return ret
}

func taskGraphFromValue(value any) map[string]any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneReadModelMap(typed)
	default:
		return nil
	}
}

func taskDecompositionNode(raw map[string]any, executedByNode map[string]presenter.StepView) TaskDecompositionNode {
	nodeID := readModelStringValue(raw["id"])
	input, _ := raw["input"].(map[string]any)
	ret := TaskDecompositionNode{
		ID:        nodeID,
		Kind:      readModelStringValue(raw["kind"]),
		Title:     taskNodeTitle(raw),
		State:     "pending",
		DependsOn: readModelStringSliceValue(raw["dependsOn"]),
		Branch:    readModelStringValue(raw["branch"]),
		Input:     cloneReadModelMap(input),
	}
	if executedByNode != nil {
		if step, ok := executedByNode[nodeID]; ok {
			ret.StepID = step.ID
			ret.StepState = step.State
			ret.State = taskNodeStateFromStep(step.State)
			ret.EvidenceRefs = compactUniqueStrings([]string{nodeID, step.ID})
			return ret
		}
	}
	ret.EvidenceRefs = compactUniqueStrings([]string{nodeID})
	return ret
}

func taskNodeTitle(raw map[string]any) string {
	if title := readModelStringValue(raw["summary"]); title != "" {
		return title
	}
	input, _ := raw["input"].(map[string]any)
	if command := readModelStringValue(input["command"]); command != "" {
		return command
	}
	if tool := readModelStringValue(input["tool"]); tool != "" {
		return tool
	}
	if operation := readModelStringValue(input["operation"]); operation != "" {
		return operation
	}
	return readModelStringValue(raw["kind"])
}

func taskNodeStateFromStep(state string) string {
	switch state {
	case string(common.StepStateCompleted):
		return "completed"
	case string(common.StepStateFailed):
		return "failed"
	case string(common.StepStateDispatched):
		return "running"
	default:
		if state != "" {
			return state
		}
		return "pending"
	}
}

func taskDecompositionState(item TaskDecomposition) string {
	if item.NodeFailed > 0 {
		return "failed"
	}
	if item.NodeTotal > 0 && item.NodeCompleted == item.NodeTotal {
		return "completed"
	}
	if item.NodeCompleted > 0 || item.NodeRunning > 0 {
		return "running"
	}
	return "pending"
}

func mapExecutionSections(snapshot RunSnapshot) []ExecutionSection {
	if len(snapshot.TaskDecompositions) == 0 {
		return nil
	}
	dodItems := runDoDItems(snapshot)
	refs := executionEvidenceRefsByStep(snapshot)
	ret := make([]ExecutionSection, 0)
	for _, decomposition := range snapshot.TaskDecompositions {
		for _, node := range decomposition.Nodes {
			section := ExecutionSection{
				ID:              decomposition.ID + "/section/" + node.ID,
				WorkspaceID:     snapshot.Run.WorkspaceID,
				RunID:           snapshot.Run.ID,
				Title:           firstNonEmptyReadModel(node.Title, node.ID, node.Kind),
				State:           node.State,
				GraphRootStepID: decomposition.RootStepID,
				GraphNodeID:     node.ID,
				StepID:          node.StepID,
				StepKind:        node.Kind,
				RelatedDoDItems: relatedDoDItemsForSection(node, dodItems),
				PlannedStepRefs: compactUniqueStrings([]string{decomposition.RootStepID, node.ID}),
				Summary:         executionSectionSummary(node),
			}
			section.ActualEvidenceRefs = compactUniqueStrings(append(cloneReadModelStringSlice(node.EvidenceRefs), refs.evidenceByStep[node.StepID]...))
			section.DecisionRefs = compactUniqueStrings(refs.decisionsByStep[node.StepID])
			section.ApprovalRefs = compactUniqueStrings(refs.approvalsByStep[node.StepID])
			section.ToolCallRefs = compactUniqueStrings(refs.toolCallsByStep[node.StepID])
			section.PatchRefs = compactUniqueStrings(refs.patchesByStep[node.StepID])
			section.ValidationRefs = compactUniqueStrings(refs.validationsByStep[node.StepID])
			section.ResultRefs = compactUniqueStrings(refs.resultsByStep[node.StepID])
			section.ArtifactRefs = compactUniqueStrings(refs.artifactsByStep[node.StepID])
			section.MemoryRecallRefs = compactUniqueStrings(refs.memoryRecallsByStep[node.StepID])
			section.MemoryIDs = compactUniqueStrings(refs.memoryIDsByStep[node.StepID])
			ret = append(ret, section)
		}
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

type executionEvidenceRefs struct {
	evidenceByStep      map[string][]string
	decisionsByStep     map[string][]string
	approvalsByStep     map[string][]string
	toolCallsByStep     map[string][]string
	patchesByStep       map[string][]string
	validationsByStep   map[string][]string
	resultsByStep       map[string][]string
	artifactsByStep     map[string][]string
	memoryRecallsByStep map[string][]string
	memoryIDsByStep     map[string][]string
}

func executionEvidenceRefsByStep(snapshot RunSnapshot) executionEvidenceRefs {
	ret := executionEvidenceRefs{
		evidenceByStep:      map[string][]string{},
		decisionsByStep:     map[string][]string{},
		approvalsByStep:     map[string][]string{},
		toolCallsByStep:     map[string][]string{},
		patchesByStep:       map[string][]string{},
		validationsByStep:   map[string][]string{},
		resultsByStep:       map[string][]string{},
		artifactsByStep:     map[string][]string{},
		memoryRecallsByStep: map[string][]string{},
		memoryIDsByStep:     map[string][]string{},
	}
	add := func(target map[string][]string, stepID string, refs ...string) {
		stepID = strings.TrimSpace(stepID)
		if stepID == "" {
			return
		}
		for _, ref := range refs {
			ref = strings.TrimSpace(ref)
			if ref == "" {
				continue
			}
			target[stepID] = append(target[stepID], ref)
			ret.evidenceByStep[stepID] = append(ret.evidenceByStep[stepID], ref)
		}
	}
	for _, item := range snapshot.Decisions {
		add(ret.decisionsByStep, item.StepID, item.ID)
	}
	for _, item := range snapshot.Approvals {
		add(ret.approvalsByStep, item.StepID, item.ID)
	}
	for _, item := range snapshot.ToolCalls {
		add(ret.toolCallsByStep, item.StepID, item.ID)
	}
	for _, item := range snapshot.Patches {
		add(ret.patchesByStep, item.StepID, item.ID)
	}
	for _, item := range snapshot.ValidationResults {
		add(ret.validationsByStep, item.StepID, item.ID, item.EvidenceRef)
	}
	for _, item := range snapshot.Results {
		if item.SourceType == "dod_assessment" {
			add(ret.resultsByStep, item.SourceRef, append([]string{item.ID}, item.EvidenceRefs...)...)
		}
	}
	for _, item := range snapshot.Artifacts {
		add(ret.artifactsByStep, item.StepID, item.ID)
	}
	for _, item := range snapshot.MemoryRecallTraces {
		add(ret.memoryRecallsByStep, item.StepID, item.ID, item.EvidenceRef)
		add(ret.memoryIDsByStep, item.StepID, item.RetrievedMemoryIDs...)
	}
	for stepID, refs := range ret.evidenceByStep {
		ret.evidenceByStep[stepID] = compactUniqueStrings(refs)
	}
	return ret
}

func runDoDItems(snapshot RunSnapshot) []string {
	if snapshot.Run.DoDID == "" {
		return nil
	}
	for _, item := range snapshot.Results {
		if item.DoDID != snapshot.Run.DoDID || item.DoDAssessment == nil {
			continue
		}
		ret := make([]string, 0, len(item.DoDAssessment.Items))
		for _, assessmentItem := range item.DoDAssessment.Items {
			if strings.TrimSpace(assessmentItem.Item) != "" {
				ret = append(ret, assessmentItem.Item)
			}
		}
		if len(ret) > 0 {
			return ret
		}
	}
	return nil
}

func relatedDoDItemsForSection(node TaskDecompositionNode, dodItems []string) []string {
	if len(dodItems) == 0 {
		return nil
	}
	haystack := strings.ToLower(strings.Join([]string{node.ID, node.Title, node.Kind}, " "))
	ret := make([]string, 0)
	for _, item := range dodItems {
		if strings.Contains(haystack, strings.ToLower(strings.TrimSpace(item))) {
			ret = append(ret, item)
		}
	}
	return ret
}

func executionSectionSummary(node TaskDecompositionNode) string {
	switch node.State {
	case "completed":
		return "section completed with linked runtime evidence"
	case "failed":
		return "section failed; inspect linked evidence and governance records"
	case "running":
		return "section is running"
	default:
		return "section is planned"
	}
}

func graphRootStepIDFromStepID(stepID string) string {
	idx := strings.Index(stepID, "/block/")
	if idx <= 0 {
		return ""
	}
	return stepID[:idx]
}

func lastContextEvidence(items []ContextEvidence, limit int) []ContextEvidence {
	if len(items) == 0 || limit <= 0 {
		return nil
	}
	start := len(items) - limit
	if start < 0 {
		start = 0
	}
	return append([]ContextEvidence(nil), items[start:]...)
}

func resultRefsFromTaskProgress(progress TaskProgress) []string {
	var refs []string
	for _, item := range progress.Items {
		if item.Kind == "result" || item.Kind == "artifact" {
			refs = append(refs, item.ID)
			refs = append(refs, item.EvidenceRefs...)
		}
	}
	return compactUniqueStrings(refs)
}

func contextBundleSummary(snapshot RunSnapshot, progress TaskProgress) string {
	if progress.Summary != "" {
		return progress.Summary
	}
	return "run " + snapshot.Run.State + " context bundle for recovery and compression"
}

func contextWorkspaceSection(snapshot RunSnapshot) ContextSection {
	return ContextSection{
		ID:       "workspace",
		Kind:     "workspace",
		Title:    "Workspace",
		Priority: 100,
		State:    snapshot.Workspace.State,
		Summary:  snapshot.Workspace.Title,
		Data: map[string]any{
			"id":               snapshot.Workspace.ID,
			"title":            snapshot.Workspace.Title,
			"description":      snapshot.Workspace.Description,
			"workingDirectory": snapshot.Workspace.WorkingDirectory,
		},
	}
}

func contextRunSection(snapshot RunSnapshot) ContextSection {
	return ContextSection{
		ID:       "run",
		Kind:     "run",
		Title:    "Run",
		Priority: 100,
		State:    snapshot.Run.State,
		Summary:  snapshot.Run.FailureReason,
		EvidenceRefs: compactUniqueStrings([]string{
			snapshot.Run.ID,
			snapshot.Run.BlockingTicketID,
			snapshot.Run.BlockingApprovalID,
		}),
		Data: map[string]any{
			"id":                 snapshot.Run.ID,
			"workspaceId":        snapshot.Run.WorkspaceID,
			"goalId":             snapshot.Run.GoalID,
			"doDId":              snapshot.Run.DoDID,
			"state":              snapshot.Run.State,
			"currentStepId":      currentStepID(snapshot),
			"blockingTicketId":   snapshot.Run.BlockingTicketID,
			"blockingApprovalId": snapshot.Run.BlockingApprovalID,
			"failureReason":      snapshot.Run.FailureReason,
		},
	}
}

func contextGoalSection(snapshot RunSnapshot) ContextSection {
	goal, _ := snapshot.Run.ExecutionContext["goal"].(map[string]any)
	return ContextSection{
		ID:       "goal",
		Kind:     "goal",
		Title:    "Goal",
		Priority: 95,
		Summary:  readModelStringValue(goal["text"]),
		Data:     cloneReadModelMap(goal),
	}
}

func contextDoDSection(snapshot RunSnapshot) ContextSection {
	dod, _ := snapshot.Run.ExecutionContext["dod"].(map[string]any)
	return ContextSection{
		ID:       "dod",
		Kind:     "dod",
		Title:    "Definition of Done",
		Priority: 95,
		State:    readModelStringValue(dod["state"]),
		Summary:  contextDoDSummary(dod),
		Data:     cloneReadModelMap(dod),
	}
}

func contextExecutionContextSection(snapshot RunSnapshot) ContextSection {
	data := cloneReadModelMap(snapshot.Run.ExecutionContext)
	delete(data, "goal")
	delete(data, "dod")
	delete(data, "compressedContext")
	return ContextSection{
		ID:       "execution_context",
		Kind:     "execution_context",
		Title:    "Execution context",
		Priority: 85,
		Summary:  "selected Soul, Skill, MCP, Memory, and provider context",
		Data:     data,
	}
}

func contextTaskProgressSection(progress TaskProgress) ContextSection {
	return ContextSection{
		ID:           "task_progress",
		Kind:         "task_progress",
		Title:        "Task progress",
		Priority:     90,
		State:        progress.State,
		Summary:      progress.Summary,
		EvidenceRefs: progress.EvidenceRefs,
		Data: map[string]any{
			"currentMilestone": progress.CurrentMilestone,
			"milestones":       progress.Milestones,
			"blockers":         progress.Blockers,
			"recovery":         progress.Recovery,
		},
	}
}

func contextTaskDecompositionSection(items []TaskDecomposition) ContextSection {
	if len(items) == 0 {
		return ContextSection{}
	}
	refs := make([]string, 0, len(items))
	state := "completed"
	for _, item := range items {
		refs = append(refs, item.EvidenceRefs...)
		if item.State == "failed" {
			state = "failed"
			continue
		}
		if item.State != "completed" && state != "failed" {
			state = "running"
		}
	}
	return ContextSection{
		ID:           "task_decomposition",
		Kind:         "task_decomposition",
		Title:        "Task decomposition",
		Priority:     85,
		State:        state,
		Summary:      strconv.Itoa(len(items)) + " decomposition graphs",
		EvidenceRefs: refs,
		Data: map[string]any{
			"taskDecompositions": items,
		},
	}
}

func contextExecutionSectionsSection(items []ExecutionSection) ContextSection {
	if len(items) == 0 {
		return ContextSection{}
	}
	refs := make([]string, 0)
	state := "completed"
	for _, item := range items {
		refs = append(refs, item.ActualEvidenceRefs...)
		if item.State == "failed" {
			state = "failed"
			continue
		}
		if item.State != "completed" && state != "failed" {
			state = "running"
		}
	}
	return ContextSection{
		ID:           "execution_sections",
		Kind:         "execution_sections",
		Title:        "Execution sections",
		Priority:     84,
		State:        state,
		Summary:      strconv.Itoa(len(items)) + " execution sections",
		EvidenceRefs: compactUniqueStrings(refs),
		Data: map[string]any{
			"executionSections": items,
		},
	}
}

func contextMemoryRecallSection(items []MemoryRecallTrace) ContextSection {
	if len(items) == 0 {
		return ContextSection{}
	}
	refs := make([]string, 0)
	memoryIDs := make([]string, 0)
	status := "selected"
	for _, item := range items {
		refs = append(refs, item.ID, item.EvidenceRef)
		memoryIDs = append(memoryIDs, item.RetrievedMemoryIDs...)
		if item.Status == "degraded" {
			status = "degraded"
		}
	}
	return ContextSection{
		ID:           "memory_recall",
		Kind:         "memory_recall",
		Title:        "Memory recall",
		Priority:     83,
		State:        status,
		Summary:      strconv.Itoa(len(items)) + " memory recall traces",
		EvidenceRefs: compactUniqueStrings(append(refs, memoryIDs...)),
		Data: map[string]any{
			"memoryRecallTraces": items,
			"memoryIds":          compactUniqueStrings(memoryIDs),
		},
	}
}

func contextGovernanceSection(snapshot RunSnapshot) ContextSection {
	language := snapshotLanguage(snapshot)
	if len(snapshot.Decisions) == 0 && len(snapshot.Approvals) == 0 {
		return ContextSection{}
	}
	return ContextSection{
		ID:           "governance",
		Kind:         "governance",
		Title:        localizedMilestoneTitle(language, "governance"),
		Priority:     80,
		State:        governanceProgressState(snapshot),
		Summary:      "decision tickets and approvals that affected run progress",
		EvidenceRefs: governanceEvidenceRefs(snapshot),
		Data: map[string]any{
			"decisions": snapshot.Decisions,
			"approvals": snapshot.Approvals,
		},
	}
}

func contextFormalOutputSection(snapshot RunSnapshot) ContextSection {
	results := runScopedResults(snapshot)
	artifacts := runScopedArtifacts(snapshot)
	if len(results) == 0 && len(artifacts) == 0 {
		return ContextSection{}
	}
	return ContextSection{
		ID:           "formal_outputs",
		Kind:         "formal_outputs",
		Title:        "Formal outputs",
		Priority:     90,
		State:        resultProgressState(snapshot),
		Summary:      formalOutputSummary(results),
		EvidenceRefs: append(idsFromResults(results), idsFromArtifacts(artifacts)...),
		Data: map[string]any{
			"results":   results,
			"artifacts": artifacts,
		},
	}
}

func contextRecentEvidenceSection(items []ContextEvidence) ContextSection {
	if len(items) == 0 {
		return ContextSection{}
	}
	refs := make([]string, 0, len(items))
	for _, item := range items {
		refs = append(refs, item.EvidenceRefs...)
	}
	return ContextSection{
		ID:           "recent_evidence",
		Kind:         "recent_evidence",
		Title:        "Recent evidence",
		Priority:     70,
		Summary:      "latest runtime timeline items retained for recovery",
		EvidenceRefs: compactUniqueStrings(refs),
		Data: map[string]any{
			"items": items,
		},
	}
}

func contextRecoverySection(recovery RecoveryPlan) ContextSection {
	return ContextSection{
		ID:       "recovery",
		Kind:     "recovery",
		Title:    "Recovery recommendation",
		Priority: 100,
		State:    recovery.Mode,
		Summary:  recovery.NextAction,
		EvidenceRefs: compactUniqueStrings([]string{
			recovery.StepID,
			recovery.BlockingTicketID,
			recovery.BlockingApprovalID,
			recovery.CheckpointID,
		}),
		Data: map[string]any{
			"mode":               recovery.Mode,
			"nextAction":         recovery.NextAction,
			"runId":              recovery.RunID,
			"stepId":             recovery.StepID,
			"blockingTicketId":   recovery.BlockingTicketID,
			"blockingApprovalId": recovery.BlockingApprovalID,
			"checkpointId":       recovery.CheckpointID,
			"reason":             recovery.Reason,
		},
	}
}

func contextRecentEvidence(timeline []RuntimeTimelineItem, limit int) []ContextEvidence {
	if len(timeline) == 0 || limit <= 0 {
		return nil
	}
	start := len(timeline) - limit
	if start < 0 {
		start = 0
	}
	ret := make([]ContextEvidence, 0, len(timeline)-start)
	for _, item := range timeline[start:] {
		ret = append(ret, ContextEvidence{
			ID:         firstNonEmptyReadModel(item.ID, item.Topic, item.StepID),
			Kind:       firstNonEmptyReadModel(item.Source+"/"+item.Type, item.Source),
			State:      firstNonEmptyReadModel(item.StepState, item.RunState),
			Summary:    item.Summary,
			OccurredAt: item.OccurredAt,
			EvidenceRefs: compactUniqueStrings([]string{
				item.ID,
				item.StepID,
				item.Topic,
			}),
			Data: compactContextEvidenceData(item),
		})
	}
	return ret
}

func compactContextEvidenceData(item RuntimeTimelineItem) map[string]any {
	data := map[string]any{}
	if item.Topic != "" {
		data["topic"] = item.Topic
	}
	if item.RunState != "" {
		data["runState"] = item.RunState
	}
	if item.StepState != "" {
		data["stepState"] = item.StepState
	}
	if item.Summary != "" {
		data["summary"] = item.Summary
	}
	if len(data) == 0 {
		return nil
	}
	return data
}

func contextCompressionCandidates(bundle ContextBundle) []ContextCompressionCandidate {
	var ret []ContextCompressionCandidate
	if len(bundle.RecentEvidence) > 8 {
		ret = append(ret, ContextCompressionCandidate{
			SectionID:     "recent_evidence",
			Reason:        "recent evidence exceeds compact recovery window",
			Retention:     "retain latest evidence ids, states, summaries, and timestamps",
			SourceRefs:    contextEvidenceRefs(bundle.RecentEvidence),
			SuggestedMode: "summarize",
		})
	}
	if len(bundle.TaskProgress.Items) > 10 {
		ret = append(ret, ContextCompressionCandidate{
			SectionID:     "task_progress",
			Reason:        "task progress contains many low-level items",
			Retention:     "retain milestones, blockers, recovery, and accepted result refs",
			SourceRefs:    bundle.TaskProgress.EvidenceRefs,
			SuggestedMode: "milestone_summary",
		})
	}
	for _, section := range bundle.Sections {
		if section.ID == "execution_context" && len(section.Data) > 4 {
			ret = append(ret, ContextCompressionCandidate{
				SectionID:     section.ID,
				Reason:        "execution context catalog can be summarized after activation",
				Retention:     "retain selected ids and policy evidence, summarize verbose catalog entries",
				SuggestedMode: "catalog_summary",
			})
		}
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func contextEvidenceRefs(items []ContextEvidence) []string {
	var refs []string
	for _, item := range items {
		refs = append(refs, item.EvidenceRefs...)
	}
	return compactUniqueStrings(refs)
}

func compactContextSections(items []ContextSection) []ContextSection {
	ret := make([]ContextSection, 0, len(items))
	for _, item := range items {
		if item.ID == "" {
			continue
		}
		item.EvidenceRefs = compactUniqueStrings(item.EvidenceRefs)
		ret = append(ret, item)
	}
	sort.SliceStable(ret, func(i, j int) bool {
		if ret[i].Priority == ret[j].Priority {
			return ret[i].ID < ret[j].ID
		}
		return ret[i].Priority > ret[j].Priority
	})
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func currentStepID(snapshot RunSnapshot) string {
	if snapshot.Step == nil {
		return ""
	}
	return snapshot.Step.ID
}

func contextDoDSummary(dod map[string]any) string {
	if len(dod) == 0 {
		return ""
	}
	switch items := dod["items"].(type) {
	case []string:
		return strconv.Itoa(len(items)) + " DoD items"
	case []any:
		return strconv.Itoa(len(items)) + " DoD items"
	default:
		return ""
	}
}

func formalOutputSummary(items []Result) string {
	if result := primaryTaskProgressResult(items); result != nil {
		return result.Summary
	}
	return ""
}

func taskProgressBlockers(snapshot RunSnapshot) []ProgressBlocker {
	var ret []ProgressBlocker
	for _, item := range snapshot.Decisions {
		if isResolvedDecisionState(item.State) {
			continue
		}
		ret = append(ret, ProgressBlocker{
			ID:          item.ID,
			Kind:        "decision",
			State:       item.State,
			Reason:      firstNonEmptyReadModel(item.Question, item.Reason),
			EvidenceRef: item.StepID,
		})
	}
	for _, item := range snapshot.Approvals {
		if isResolvedApprovalState(item.State) {
			continue
		}
		ret = append(ret, ProgressBlocker{
			ID:          item.ID,
			Kind:        "approval",
			State:       item.State,
			Reason:      item.Reason,
			EvidenceRef: item.StepID,
		})
	}
	return ret
}

func taskProgressItems(snapshot RunSnapshot) []ProgressItem {
	results := runScopedResults(snapshot)
	artifacts := runScopedArtifacts(snapshot)
	ret := make([]ProgressItem, 0, len(snapshot.Steps)+len(snapshot.ToolCalls)+len(snapshot.ValidationResults)+len(results)+len(artifacts)+len(snapshot.Decisions)+len(snapshot.Approvals))
	for _, item := range snapshot.Steps {
		ret = append(ret, ProgressItem{ID: item.ID, Kind: "step", Title: firstNonEmptyReadModel(item.Kind, item.ID), State: item.State, EvidenceRefs: []string{item.ID}})
	}
	for _, item := range snapshot.ToolCalls {
		ret = append(ret, ProgressItem{ID: item.ID, Kind: "tool_call", Title: firstNonEmptyReadModel(item.ToolName, item.ID), State: item.State, EvidenceRefs: []string{item.ID, item.StepID}})
	}
	for _, item := range snapshot.Patches {
		ret = append(ret, ProgressItem{ID: item.ID, Kind: "patch", Title: firstNonEmptyReadModel(item.TargetRef, item.PatchType, item.ID), State: item.State, EvidenceRefs: []string{item.ID, item.ToolCallID, item.StepID}})
	}
	for _, item := range snapshot.ValidationResults {
		ret = append(ret, ProgressItem{ID: item.ID, Kind: "validation_result", Title: firstNonEmptyReadModel(item.ValidationKind, item.TargetRef, item.ID), State: item.State, EvidenceRefs: []string{item.ID, item.EvidenceRef, item.StepID}})
	}
	for _, item := range results {
		ret = append(ret, ProgressItem{ID: item.ID, Kind: "result", Title: firstNonEmptyReadModel(item.Title, item.SourceType, item.ID), State: item.State, EvidenceRefs: compactUniqueStrings(append([]string{item.ID, item.SourceRef}, item.EvidenceRefs...))})
	}
	for _, item := range artifacts {
		ret = append(ret, ProgressItem{ID: item.ID, Kind: "artifact", Title: firstNonEmptyReadModel(item.Name, item.URI, item.ArtifactType, item.ID), State: item.State, EvidenceRefs: compactUniqueStrings(append([]string{item.ID, item.ResultID, item.StepID, item.ToolCallID, item.PatchID, item.ValidationResultID}, item.EvidenceRefs...))})
	}
	for _, item := range snapshot.Decisions {
		ret = append(ret, ProgressItem{ID: item.ID, Kind: "decision", Title: firstNonEmptyReadModel(item.Question, item.DecisionKind, item.ID), State: item.State, EvidenceRefs: []string{item.ID, item.StepID}})
	}
	for _, item := range snapshot.Approvals {
		ret = append(ret, ProgressItem{ID: item.ID, Kind: "approval", Title: firstNonEmptyReadModel(item.Reason, item.ID), State: item.State, EvidenceRefs: []string{item.ID, item.StepID}})
	}
	for idx := range ret {
		ret[idx].EvidenceRefs = compactUniqueStrings(ret[idx].EvidenceRefs)
	}
	return ret
}

func taskProgressRecovery(snapshot RunSnapshot, latestCheckpoint *RuntimeCheckpoint, blockers []ProgressBlocker, fallback RecoveryPlan) RecoveryPlan {
	ret := fallback
	ret.RunID = snapshot.Run.ID
	ret.BlockingTicketID = snapshot.Run.BlockingTicketID
	ret.BlockingApprovalID = snapshot.Run.BlockingApprovalID
	if latestCheckpoint != nil {
		ret.CheckpointID = latestCheckpoint.ID
		if ret.StepID == "" {
			ret.StepID = latestCheckpoint.CurrentStepID
		}
		if ret.BlockingTicketID == "" {
			ret.BlockingTicketID = latestCheckpoint.BlockingTicketID
		}
		if ret.BlockingApprovalID == "" {
			ret.BlockingApprovalID = latestCheckpoint.BlockingApprovalID
		}
		if ret.FailureClass == "" {
			ret.FailureClass = strings.TrimSpace(readModelStringValue(latestCheckpoint.Data["failureClass"]))
		}
		if ret.FailureSignature == "" {
			ret.FailureSignature = strings.TrimSpace(readModelStringValue(latestCheckpoint.Data["failureSignature"]))
		}
	}
	if ret.BlockingTicketID != "" {
		ret.Mode = "wait_decision"
		ret.NextAction = "resolve_decision_ticket"
		ret.Reason = blockerReason(blockers, "decision", ret.BlockingTicketID)
		return ret
	}
	if ret.BlockingApprovalID != "" {
		ret.Mode = "wait_approval"
		ret.NextAction = "resolve_approval"
		ret.Reason = blockerReason(blockers, "approval", ret.BlockingApprovalID)
		return ret
	}
	switch common.RunState(snapshot.Run.State) {
	case common.RunStateCompleted:
		ret.Mode = "none"
		ret.NextAction = "inspect_result"
		ret.Reason = "run completed"
	case common.RunStateFailed:
		ret.Mode = "inspect_failure"
		ret.NextAction = "inspect_failure_reason_and_evidence"
		ret.Reason = snapshot.Run.FailureReason
	case common.RunStateCancelled, common.RunStateSuperseded:
		ret.Mode = "restart"
		ret.NextAction = "start_new_run"
		ret.Reason = "run is terminal but not completed"
	case common.RunStateValidating:
		ret.Mode = "continue"
		ret.NextAction = "continue_validation"
		ret.Reason = "run is validating"
	default:
		ret.Mode = "continue"
		ret.NextAction = "continue_run"
		ret.Reason = "run is not terminal"
	}
	return ret
}

func taskProgressSummary(progress TaskProgress, snapshot *RunSnapshot) string {
	if snapshot != nil {
		if result := primaryTaskProgressResult(snapshot.Results); result != nil && result.Summary != "" {
			return result.Summary
		}
		if snapshot.Run.FailureReason != "" {
			return snapshot.Run.FailureReason
		}
		return "run " + snapshot.Run.State + " with " + strconv.Itoa(len(snapshot.Steps)) + " steps, " + strconv.Itoa(len(snapshot.Results)) + " results, " + strconv.Itoa(len(snapshot.Artifacts)) + " artifacts"
	}
	return "workspace " + progress.State + " with " + strconv.Itoa(len(progress.Blockers)) + " open blockers"
}

func primaryTaskProgressResult(items []Result) *Result {
	for idx := range items {
		if items[idx].State == string(common.ResultStateAccepted) {
			return &items[idx]
		}
	}
	if len(items) == 0 {
		return nil
	}
	return &items[len(items)-1]
}

func stepProgressState(snapshot RunSnapshot) string {
	if len(snapshot.Steps) == 0 {
		return "pending"
	}
	if hasAcceptedRunResult(snapshot) && snapshot.Run.State == string(common.RunStateCompleted) {
		return "completed"
	}
	for _, item := range snapshot.Steps {
		if item.State == string(common.StepStateFailed) {
			return "failed"
		}
	}
	if runtimeProgress(snapshot).StepCompleted == len(snapshot.Steps) {
		return "completed"
	}
	return "running"
}

func governanceProgressState(snapshot RunSnapshot) string {
	if len(snapshot.Decisions) == 0 && len(snapshot.Approvals) == 0 {
		return "pending"
	}
	if snapshot.Run.BlockingTicketID != "" || snapshot.Run.BlockingApprovalID != "" {
		return "blocked"
	}
	for _, item := range snapshot.Decisions {
		if !isResolvedDecisionState(item.State) {
			return "blocked"
		}
	}
	for _, item := range snapshot.Approvals {
		if !isResolvedApprovalState(item.State) {
			return "blocked"
		}
	}
	return "completed"
}

func validationProgressState(snapshot RunSnapshot) string {
	if len(snapshot.ValidationResults) == 0 {
		if snapshot.Run.State == string(common.RunStateValidating) {
			return "running"
		}
		return "pending"
	}
	if hasAcceptedRunResult(snapshot) && snapshot.Run.State == string(common.RunStateCompleted) {
		return "completed"
	}
	for _, item := range snapshot.ValidationResults {
		if item.State == string(common.ValidationResultStateFailed) {
			return "failed"
		}
	}
	return "completed"
}

func resultProgressState(snapshot RunSnapshot) string {
	results := runScopedResults(snapshot)
	artifacts := runScopedArtifacts(snapshot)
	if len(results) == 0 && len(artifacts) == 0 {
		if isTerminalRunState(snapshot.Run.State) {
			return "failed"
		}
		return "pending"
	}
	for _, item := range results {
		if item.State == string(common.ResultStateAccepted) {
			return "completed"
		}
	}
	return progressStateFromCount(len(results)+len(artifacts), false, false)
}

func mapRunOutcome(snapshot RunSnapshot) RunOutcome {
	ret := RunOutcome{
		RunID:    snapshot.Run.ID,
		State:    snapshot.Run.State,
		RawState: snapshot.Run.State,
		Terminal: isTerminalRunState(snapshot.Run.State),
	}
	if result := primaryAcceptedResult(runScopedResults(snapshot)); result != nil {
		ret.State = "accepted"
		ret.Accepted = true
		ret.Terminal = true
		ret.ResultID = result.ID
		ret.Summary = result.Summary
		ret.EvidenceRefs = compactUniqueStrings(result.EvidenceRefs)
		ret.ArtifactRefs = compactUniqueStrings(result.ArtifactRefs)
		return ret
	}
	switch common.RunState(snapshot.Run.State) {
	case common.RunStateCompleted:
		ret.State = "failed"
		ret.Terminal = true
		ret.Reason = "run completed without accepted result"
	case common.RunStateFailed:
		ret.State = "failed"
		ret.Terminal = true
		ret.Reason = snapshot.Run.FailureReason
	case common.RunStateCancelled:
		ret.State = string(common.RunStateCancelled)
		ret.Terminal = true
		ret.Reason = "run cancelled"
	case common.RunStateSuperseded:
		ret.State = string(common.RunStateSuperseded)
		ret.Terminal = true
		ret.Reason = "run superseded"
	}
	if ret.Summary == "" {
		ret.Summary = ret.Reason
	}
	return ret
}

func mapWorkspaceOutcome(snapshot WorkspaceSnapshot) WorkspaceOutcome {
	ret := WorkspaceOutcome{
		WorkspaceID:       snapshot.Workspace.ID,
		State:             snapshot.Workspace.State,
		RawWorkspaceState: snapshot.Workspace.State,
	}
	var selectedRun *Run
	if snapshot.TaskProgress.RunID != "" {
		for idx := range snapshot.Runs {
			if snapshot.Runs[idx].ID == snapshot.TaskProgress.RunID {
				selectedRun = &snapshot.Runs[idx]
				break
			}
		}
	}
	if selectedRun == nil && snapshot.Workspace.ActiveRunID != "" {
		for idx := range snapshot.Runs {
			if snapshot.Runs[idx].ID == snapshot.Workspace.ActiveRunID {
				selectedRun = &snapshot.Runs[idx]
				break
			}
		}
	}
	if selectedRun == nil && len(snapshot.Runs) > 0 {
		selectedRun = &snapshot.Runs[len(snapshot.Runs)-1]
	}
	if selectedRun == nil {
		return ret
	}
	runSnapshot := RunSnapshot{
		Workspace: snapshot.Workspace,
		Run:       *selectedRun,
		Results:   snapshot.Results,
		Artifacts: snapshot.Artifacts,
	}
	outcome := mapRunOutcome(runSnapshot)
	ret.State = outcome.State
	ret.RunID = outcome.RunID
	ret.ResultID = outcome.ResultID
	ret.Accepted = outcome.Accepted
	ret.Terminal = outcome.Terminal
	ret.Summary = outcome.Summary
	ret.Reason = outcome.Reason
	ret.EvidenceRefs = outcome.EvidenceRefs
	ret.ArtifactRefs = outcome.ArtifactRefs
	return ret
}

func primaryAcceptedResult(items []Result) *Result {
	var selected *Result
	for idx := range items {
		if items[idx].State != string(common.ResultStateAccepted) {
			continue
		}
		if selected == nil || resultTimeValue(items[idx]) >= resultTimeValue(*selected) {
			selected = &items[idx]
		}
	}
	return selected
}

func resultTimeValue(item Result) string {
	if item.UpdatedAt != "" {
		return item.UpdatedAt
	}
	if item.CreatedAt != "" {
		return item.CreatedAt
	}
	return item.ID
}

func hasAcceptedRunResult(snapshot RunSnapshot) bool {
	for _, item := range runScopedResults(snapshot) {
		if item.State == string(common.ResultStateAccepted) {
			return true
		}
	}
	return false
}

func progressStateFromCount(count int, failed bool, completed bool) string {
	if failed {
		return "failed"
	}
	if completed || count > 0 {
		return "completed"
	}
	return "pending"
}

func currentProgressMilestone(items []ProgressMilestone) string {
	for _, item := range items {
		if item.State != "completed" {
			return item.ID
		}
	}
	if len(items) == 0 {
		return ""
	}
	return items[len(items)-1].ID
}

func taskProgressEvidenceRefs(snapshot RunSnapshot) []string {
	results := runScopedResults(snapshot)
	artifacts := runScopedArtifacts(snapshot)
	refs := []string{snapshot.Run.ID}
	if snapshot.Step != nil {
		refs = append(refs, snapshot.Step.ID)
	}
	refs = append(refs, idsFromSteps(snapshot.Steps)...)
	refs = append(refs, idsFromToolCalls(snapshot.ToolCalls)...)
	refs = append(refs, idsFromPatches(snapshot.Patches)...)
	refs = append(refs, idsFromValidations(snapshot.ValidationResults)...)
	refs = append(refs, idsFromCheckpoints(snapshot.RuntimeCheckpoints)...)
	refs = append(refs, idsFromResults(results)...)
	refs = append(refs, idsFromArtifacts(artifacts)...)
	refs = append(refs, governanceEvidenceRefs(snapshot)...)
	return compactUniqueStrings(refs)
}

func runScopedResults(snapshot RunSnapshot) []Result {
	if snapshot.Run.ID == "" {
		return snapshot.Results
	}
	ret := make([]Result, 0, len(snapshot.Results))
	for _, item := range snapshot.Results {
		if item.RunID == snapshot.Run.ID {
			ret = append(ret, item)
		}
	}
	return ret
}

func runScopedArtifacts(snapshot RunSnapshot) []Artifact {
	if snapshot.Run.ID == "" {
		return snapshot.Artifacts
	}
	ret := make([]Artifact, 0, len(snapshot.Artifacts))
	for _, item := range snapshot.Artifacts {
		if item.RunID == snapshot.Run.ID {
			ret = append(ret, item)
		}
	}
	return ret
}

func governanceEvidenceRefs(snapshot RunSnapshot) []string {
	refs := make([]string, 0, len(snapshot.Decisions)+len(snapshot.Approvals))
	for _, item := range snapshot.Decisions {
		refs = append(refs, item.ID, item.StepID, item.ResolutionID)
	}
	for _, item := range snapshot.Approvals {
		refs = append(refs, item.ID, item.StepID)
	}
	return compactUniqueStrings(refs)
}

func idsFromSteps(items []presenter.StepView) []string {
	ret := make([]string, 0, len(items))
	for _, item := range items {
		ret = append(ret, item.ID)
	}
	return compactUniqueStrings(ret)
}

func idsFromToolCalls(items []ToolCall) []string {
	ret := make([]string, 0, len(items))
	for _, item := range items {
		ret = append(ret, item.ID)
	}
	return compactUniqueStrings(ret)
}

func idsFromPatches(items []Patch) []string {
	ret := make([]string, 0, len(items))
	for _, item := range items {
		ret = append(ret, item.ID)
	}
	return compactUniqueStrings(ret)
}

func idsFromValidations(items []ValidationResult) []string {
	ret := make([]string, 0, len(items))
	for _, item := range items {
		ret = append(ret, item.ID, item.EvidenceRef)
	}
	return compactUniqueStrings(ret)
}

func idsFromCheckpoints(items []RuntimeCheckpoint) []string {
	ret := make([]string, 0, len(items))
	for _, item := range items {
		ret = append(ret, item.ID)
	}
	return compactUniqueStrings(ret)
}

func idsFromResults(items []Result) []string {
	ret := make([]string, 0, len(items))
	for _, item := range items {
		ret = append(ret, item.ID)
		ret = append(ret, item.EvidenceRefs...)
		ret = append(ret, item.ArtifactRefs...)
	}
	return compactUniqueStrings(ret)
}

func idsFromArtifacts(items []Artifact) []string {
	ret := make([]string, 0, len(items))
	for _, item := range items {
		ret = append(ret, item.ID)
		ret = append(ret, item.EvidenceRefs...)
	}
	return compactUniqueStrings(ret)
}

func blockerReason(items []ProgressBlocker, kind string, id string) string {
	for _, item := range items {
		if item.Kind == kind && item.ID == id {
			return item.Reason
		}
	}
	return ""
}

func isResolvedDecisionState(state string) bool {
	return state == string(common.DecisionTicketStateResolved)
}

func isResolvedApprovalState(state string) bool {
	return state == string(common.ApprovalStateApproved) || state == string(common.ApprovalStateRejected) || state == string(common.ApprovalStateResolved)
}

func firstNonEmptyReadModel(values ...string) string {
	for _, item := range values {
		if item != "" {
			return item
		}
	}
	return ""
}

func compactUniqueStrings(values []string) []string {
	seen := map[string]struct{}{}
	ret := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		ret = append(ret, value)
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func runtimeProgress(snapshot RunSnapshot) RunProgress {
	ret := RunProgress{
		State:     snapshot.Run.State,
		StepTotal: len(snapshot.Steps),
		Blocked:   snapshot.Run.BlockingTicketID != "" || snapshot.Run.BlockingApprovalID != "",
		Terminal:  isTerminalRunState(snapshot.Run.State),
	}
	if snapshot.Step != nil {
		ret.CurrentStepID = snapshot.Step.ID
		ret.CurrentStepKind = snapshot.Step.Kind
		ret.CurrentStepState = snapshot.Step.State
	}
	for _, step := range snapshot.Steps {
		switch step.State {
		case string(common.StepStateDispatched):
			ret.StepDispatched++
		case string(common.StepStateCompleted):
			ret.StepCompleted++
		case string(common.StepStateFailed):
			ret.StepFailed++
		}
	}
	return ret
}

func runtimeEvidenceSummary(snapshot RunSnapshot, eventTotal int) RuntimeEvidenceSummary {
	ret := RuntimeEvidenceSummary{
		ToolCallTotal:          len(snapshot.ToolCalls),
		PatchTotal:             len(snapshot.Patches),
		ValidationTotal:        len(snapshot.ValidationResults),
		RuntimeCheckpointTotal: len(snapshot.RuntimeCheckpoints),
		ResultTotal:            len(snapshot.Results),
		ArtifactTotal:          len(snapshot.Artifacts),
		EventTotal:             eventTotal,
	}
	for _, item := range snapshot.ToolCalls {
		switch item.State {
		case string(common.ToolCallStateRunning):
			ret.ToolCallRunning++
		case string(common.ToolCallStateCompleted):
			ret.ToolCallCompleted++
		case string(common.ToolCallStateFailed):
			ret.ToolCallFailed++
		}
	}
	for _, item := range snapshot.Patches {
		switch item.State {
		case string(common.PatchStateApplied):
			ret.PatchApplied++
		case string(common.PatchStateFailed):
			ret.PatchFailed++
		}
	}
	for _, item := range snapshot.ValidationResults {
		switch item.State {
		case string(common.ValidationResultStatePassed):
			ret.ValidationPassed++
		case string(common.ValidationResultStateFailed):
			ret.ValidationFailed++
		case string(common.ValidationResultStateWarning):
			ret.ValidationWarning++
		}
	}
	return ret
}

func latestRuntimeCheckpoint(items []RuntimeCheckpoint) *RuntimeCheckpoint {
	if len(items) == 0 {
		return nil
	}
	latest := items[0]
	for _, item := range items[1:] {
		if item.OccurredAt > latest.OccurredAt {
			latest = item
		}
	}
	return &latest
}

func runtimeTimeline(snapshot RunSnapshot) []RuntimeTimelineItem {
	items := make([]RuntimeTimelineItem, 0, len(snapshot.Steps)+len(snapshot.RuntimeCheckpoints)+len(snapshot.Observed))
	stepOccurrences := timelineStepOccurrences(snapshot)
	for _, trace := range snapshot.MemoryRecallTraces {
		items = append(items, RuntimeTimelineItem{
			Source:     "memory_recall",
			Type:       trace.Source,
			ID:         trace.ID,
			RunState:   snapshot.Run.State,
			StepID:     trace.StepID,
			Summary:    memoryRecallTimelineSummary(trace),
			OccurredAt: trace.OccurredAt,
			Data: map[string]any{
				"stage":              trace.Stage,
				"source":             trace.Source,
				"policy":             trace.Policy,
				"status":             trace.Status,
				"retrievedMemoryIds": append([]string(nil), trace.RetrievedMemoryIDs...),
				"reason":             trace.Reason,
			},
		})
	}
	for _, step := range snapshot.Steps {
		occurredAt := stepOccurrences[step.ID]
		items = append(items, RuntimeTimelineItem{
			Source:     "step",
			Type:       step.Kind,
			ID:         step.ID,
			RunState:   snapshot.Run.State,
			StepID:     step.ID,
			StepState:  step.State,
			Summary:    timelineStepSummary(step.Output),
			OccurredAt: occurredAt,
			Output:     cloneReadModelMap(step.Output),
		})
		items = append(items, streamTimelineItems(step, snapshot.Run.State, occurredAt)...)
	}
	for _, checkpoint := range snapshot.RuntimeCheckpoints {
		items = append(items, RuntimeTimelineItem{
			Source:     "checkpoint",
			Type:       checkpoint.CheckpointType,
			ID:         checkpoint.ID,
			RunState:   checkpoint.RunStateSnapshot,
			StepID:     checkpoint.CurrentStepID,
			OccurredAt: checkpoint.OccurredAt,
			Data:       cloneReadModelMap(checkpoint.Data),
		})
	}
	for _, item := range snapshot.EventRecords {
		if item.RunID != "" && item.RunID != snapshot.Run.ID {
			continue
		}
		items = append(items, RuntimeTimelineItem{
			Source:     "persisted_event",
			Type:       item.SubjectType,
			ID:         item.ID,
			Topic:      item.Topic,
			RunState:   readModelStringValue(item.Payload[events.PayloadKeyState]),
			StepID:     item.StepID,
			OccurredAt: item.OccurredAt,
			Data:       cloneReadModelMap(item.Payload),
		})
	}
	if len(snapshot.EventRecords) == 0 {
		for _, item := range snapshot.Observed {
			if item.RunID != "" && item.RunID != snapshot.Run.ID {
				continue
			}
			items = append(items, RuntimeTimelineItem{
				Source:     "event",
				Type:       item.SubjectType,
				ID:         item.SubjectID,
				Topic:      item.Topic,
				StepID:     item.StepID,
				OccurredAt: item.OccurredAt,
				Data:       cloneReadModelMap(item.Payload),
			})
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].OccurredAt == items[j].OccurredAt {
			return items[i].Source < items[j].Source
		}
		return items[i].OccurredAt < items[j].OccurredAt
	})
	return items
}

func timelineStepOccurrences(snapshot RunSnapshot) map[string]string {
	ret := map[string]string{}
	remember := func(stepID string, occurredAt string) {
		if stepID == "" || occurredAt == "" {
			return
		}
		if ret[stepID] == "" || occurredAt > ret[stepID] {
			ret[stepID] = occurredAt
		}
	}
	for _, item := range snapshot.EventRecords {
		stepID := item.StepID
		if stepID == "" && item.SubjectType == "step" {
			stepID = item.SubjectID
		}
		remember(stepID, item.OccurredAt)
	}
	for _, item := range snapshot.Observed {
		stepID := item.StepID
		if stepID == "" && item.SubjectType == "step" {
			stepID = item.SubjectID
		}
		remember(stepID, item.OccurredAt)
	}
	for _, checkpoint := range snapshot.RuntimeCheckpoints {
		if !isStepRuntimeCheckpoint(checkpoint) {
			continue
		}
		remember(checkpoint.CurrentStepID, checkpoint.OccurredAt)
	}
	return ret
}

func isStepRuntimeCheckpoint(checkpoint RuntimeCheckpoint) bool {
	if checkpoint.CurrentStepID == "" || len(checkpoint.Data) == 0 {
		return false
	}
	if readModelStringValue(checkpoint.Data["stepState"]) != "" {
		return true
	}
	eventName := readModelStringValue(checkpoint.Data["event"])
	return eventName == "step_dispatched" || eventName == "step_completed" || eventName == "step_failed"
}

func streamTimelineItems(step presenter.StepView, runState string, stepOccurredAt string) []RuntimeTimelineItem {
	if len(step.Output) == 0 {
		return nil
	}
	raw, ok := step.Output["streamEvents"]
	if !ok {
		return nil
	}
	events := readModelMapSliceValue(raw)
	if len(events) == 0 {
		return nil
	}
	ret := make([]RuntimeTimelineItem, 0, len(events))
	for idx, event := range events {
		eventType, _ := event["type"].(string)
		if eventType == "" {
			eventType = "delta"
		}
		occurredAt := streamEventOccurredAt(event)
		if occurredAt == "" {
			occurredAt = stepOccurredAt
		}
		ret = append(ret, RuntimeTimelineItem{
			Source:     "step_stream",
			Type:       eventType,
			ID:         step.ID + "/stream/" + strconv.Itoa(idx+1),
			RunState:   runState,
			StepID:     step.ID,
			StepState:  step.State,
			Summary:    readModelStringValue(event["delta"]),
			OccurredAt: occurredAt,
			Data:       cloneReadModelMap(event),
		})
	}
	return ret
}

func streamEventOccurredAt(event map[string]any) string {
	for _, key := range []string{"occurredAt", "timestamp", "time"} {
		if value := readModelStringValue(event[key]); value != "" {
			return value
		}
	}
	return ""
}

func timelineStepSummary(output map[string]any) string {
	if len(output) == 0 {
		return ""
	}
	if stage := readModelStringValue(output["stage"]); stage != "" {
		return stage
	}
	if status := readModelStringValue(output["status"]); status != "" {
		return status
	}
	return ""
}

func memoryRecallTimelineSummary(trace MemoryRecallTrace) string {
	count := len(trace.RetrievedMemoryIDs)
	if trace.Status == "degraded" && trace.Reason != "" {
		return "memory recall degraded: " + trace.Reason
	}
	if count == 0 {
		return "memory recall " + trace.Status
	}
	return "memory recall " + trace.Status + ": " + strconv.Itoa(count) + " memories"
}

func timelineEvents(items []RuntimeTimelineItem) []RuntimeTimelineItem {
	ret := make([]RuntimeTimelineItem, 0, len(items))
	for _, item := range items {
		if item.Source == "event" || item.Source == "persisted_event" {
			ret = append(ret, item)
		}
	}
	return ret
}

func isTerminalRunState(state string) bool {
	switch common.RunState(state) {
	case common.RunStateCompleted, common.RunStateFailed, common.RunStateCancelled, common.RunStateSuperseded:
		return true
	default:
		return false
	}
}

func snapshotLanguage(snapshot RunSnapshot) string {
	if snapshot.Run.ExecutionContext == nil {
		return ""
	}
	if lang, ok := snapshot.Run.ExecutionContext["dialogLanguage"].(string); ok {
		return strings.TrimSpace(lang)
	}
	return ""
}
