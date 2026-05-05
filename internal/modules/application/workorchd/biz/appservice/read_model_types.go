package appservice

import "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/presenter"

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
