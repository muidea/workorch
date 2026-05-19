package appservice

import "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/presenter"

type RunSnapshot struct {
	Workspace            Workspace                `json:"workspace"`
	Run                  Run                      `json:"run"`
	Outcome              RunOutcome               `json:"outcome"`
	TaskProgress         TaskProgress             `json:"taskProgress"`
	RepairLoop           RepairLoop               `json:"repairLoop"`
	TaskDecompositions   []TaskDecomposition      `json:"taskDecompositions,omitempty"`
	TaskPlans            []TaskPlan               `json:"taskPlans,omitempty"`
	Step                 *presenter.StepView      `json:"step,omitempty"`
	Steps                []presenter.StepView     `json:"steps,omitempty"`
	ToolCalls            []ToolCall               `json:"toolCalls,omitempty"`
	Patches              []Patch                  `json:"patches,omitempty"`
	WebSources           []WebSourceTrace         `json:"webSources,omitempty"`
	ValidationResults    []ValidationResult       `json:"validationResults,omitempty"`
	RuntimeCheckpoints   []RuntimeCheckpoint      `json:"runtimeCheckpoints,omitempty"`
	Results              []Result                 `json:"results,omitempty"`
	ClosureTraces        []ClosureTrace           `json:"closureTraces,omitempty"`
	DoDEvidence          []DoDEvidenceTrace       `json:"doDEvidence,omitempty"`
	ResultQuality        []ResultQualityTrace     `json:"resultQuality,omitempty"`
	Artifacts            []Artifact               `json:"artifacts,omitempty"`
	ExecutionSections    []ExecutionSection       `json:"executionSections,omitempty"`
	LLMContextBudgets    []LLMContextBudgetTrace  `json:"llmContextBudgets,omitempty"`
	LLMUsageTraces       []LLMUsageTrace          `json:"llmUsageTraces,omitempty"`
	LLMUsageSummary      LLMUsageSummary          `json:"llmUsageSummary"`
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

type StepQuery struct {
	RunID         string
	CursorAfterID string
	Limit         int
}

type StepPage struct {
	RunID             string               `json:"runId"`
	Steps             []presenter.StepView `json:"steps,omitempty"`
	Limit             int                  `json:"limit"`
	CursorAfterID     string               `json:"cursorAfterId,omitempty"`
	NextCursorAfterID string               `json:"nextCursorAfterId,omitempty"`
	HasMore           bool                 `json:"hasMore"`
}

type ToolCallQuery struct {
	RunID         string
	CursorAfterID string
	Limit         int
}

type ToolCallPage struct {
	RunID             string     `json:"runId"`
	ToolCalls         []ToolCall `json:"toolCalls,omitempty"`
	Limit             int        `json:"limit"`
	CursorAfterID     string     `json:"cursorAfterId,omitempty"`
	NextCursorAfterID string     `json:"nextCursorAfterId,omitempty"`
	HasMore           bool       `json:"hasMore"`
}

type RuntimeCheckpointQuery struct {
	RunID         string
	CursorAfterID string
	Limit         int
}

type RuntimeCheckpointPage struct {
	RunID              string              `json:"runId"`
	RuntimeCheckpoints []RuntimeCheckpoint `json:"runtimeCheckpoints,omitempty"`
	Limit              int                 `json:"limit"`
	CursorAfterID      string              `json:"cursorAfterId,omitempty"`
	NextCursorAfterID  string              `json:"nextCursorAfterId,omitempty"`
	HasMore            bool                `json:"hasMore"`
}

type ResultQuery struct {
	RunID         string
	CursorAfterID string
	Limit         int
}

type ResultPage struct {
	RunID             string   `json:"runId"`
	Results           []Result `json:"results,omitempty"`
	Limit             int      `json:"limit"`
	CursorAfterID     string   `json:"cursorAfterId,omitempty"`
	NextCursorAfterID string   `json:"nextCursorAfterId,omitempty"`
	HasMore           bool     `json:"hasMore"`
}

type PatchQuery struct {
	RunID         string
	CursorAfterID string
	Limit         int
}

type PatchPage struct {
	RunID             string  `json:"runId"`
	Patches           []Patch `json:"patches,omitempty"`
	Limit             int     `json:"limit"`
	CursorAfterID     string  `json:"cursorAfterId,omitempty"`
	NextCursorAfterID string  `json:"nextCursorAfterId,omitempty"`
	HasMore           bool    `json:"hasMore"`
}

type ValidationResultQuery struct {
	RunID         string
	CursorAfterID string
	Limit         int
}

type ValidationResultPage struct {
	RunID             string             `json:"runId"`
	ValidationResults []ValidationResult `json:"validationResults,omitempty"`
	Limit             int                `json:"limit"`
	CursorAfterID     string             `json:"cursorAfterId,omitempty"`
	NextCursorAfterID string             `json:"nextCursorAfterId,omitempty"`
	HasMore           bool               `json:"hasMore"`
}

type ArtifactQuery struct {
	RunID         string
	CursorAfterID string
	Limit         int
}

type ArtifactPage struct {
	RunID             string     `json:"runId"`
	Artifacts         []Artifact `json:"artifacts,omitempty"`
	Limit             int        `json:"limit"`
	CursorAfterID     string     `json:"cursorAfterId,omitempty"`
	NextCursorAfterID string     `json:"nextCursorAfterId,omitempty"`
	HasMore           bool       `json:"hasMore"`
}

type RunObservability struct {
	Run                  Run                     `json:"run"`
	Outcome              RunOutcome              `json:"outcome"`
	Progress             RunProgress             `json:"progress"`
	TaskProgress         TaskProgress            `json:"taskProgress"`
	RepairLoop           RepairLoop              `json:"repairLoop"`
	TaskDecompositions   []TaskDecomposition     `json:"taskDecompositions,omitempty"`
	TaskPlans            []TaskPlan              `json:"taskPlans,omitempty"`
	ExecutionSections    []ExecutionSection      `json:"executionSections,omitempty"`
	LLMContextBudgets    []LLMContextBudgetTrace `json:"llmContextBudgets,omitempty"`
	LLMUsageTraces       []LLMUsageTrace         `json:"llmUsageTraces,omitempty"`
	LLMUsageSummary      LLMUsageSummary         `json:"llmUsageSummary"`
	MemoryRecallTraces   []MemoryRecallTrace     `json:"memoryRecallTraces,omitempty"`
	WebSources           []WebSourceTrace        `json:"webSources,omitempty"`
	ClosureTraces        []ClosureTrace          `json:"closureTraces,omitempty"`
	DoDEvidence          []DoDEvidenceTrace      `json:"doDEvidence,omitempty"`
	ResultQuality        []ResultQualityTrace    `json:"resultQuality,omitempty"`
	Evidence             RuntimeEvidenceSummary  `json:"evidence"`
	LatestCheckpoint     *RuntimeCheckpoint      `json:"latestCheckpoint,omitempty"`
	SelectedCapabilities []RuntimeCapability     `json:"selectedCapabilities,omitempty"`
	UsedCapabilities     []RuntimeCapability     `json:"usedCapabilities,omitempty"`
	Timeline             []RuntimeTimelineItem   `json:"timeline,omitempty"`
}

type TaskProgress struct {
	WorkspaceID        string              `json:"workspaceId"`
	RunID              string              `json:"runId,omitempty"`
	State              string              `json:"state"`
	Summary            string              `json:"summary,omitempty"`
	CurrentStepID      string              `json:"currentStepId,omitempty"`
	CurrentStepKind    string              `json:"currentStepKind,omitempty"`
	CurrentStepState   string              `json:"currentStepState,omitempty"`
	CurrentMilestone   string              `json:"currentMilestone,omitempty"`
	BlockingTicketID   string              `json:"blockingTicketId,omitempty"`
	BlockingApprovalID string              `json:"blockingApprovalId,omitempty"`
	Milestones         []ProgressMilestone `json:"milestones,omitempty"`
	Items              []ProgressItem      `json:"items,omitempty"`
	Blockers           []ProgressBlocker   `json:"blockers,omitempty"`
	Recovery           RecoveryPlan        `json:"recovery"`
	RepairLoop         RepairLoop          `json:"repairLoop"`
	EvidenceRefs       []string            `json:"evidenceRefs,omitempty"`
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

type RepairLoop struct {
	State                    string           `json:"state"`
	AttemptCount             int              `json:"attemptCount"`
	LastAttempt              int              `json:"lastAttempt,omitempty"`
	LastRepairStepID         string           `json:"lastRepairStepId,omitempty"`
	LastFailureStepID        string           `json:"lastFailureStepId,omitempty"`
	LastFailureClass         string           `json:"lastFailureClass,omitempty"`
	LastFailureSignature     string           `json:"lastFailureSignature,omitempty"`
	LastFailureTargetScope   string           `json:"lastFailureTargetScope,omitempty"`
	LastFailureAt            string           `json:"lastFailureAt,omitempty"`
	StructuredFailureSignals map[string]any   `json:"structuredFailureSignals,omitempty"`
	RepairInputCompaction    map[string]any   `json:"repairInputCompaction,omitempty"`
	LastRepairInputBytes     int              `json:"lastRepairInputBytes,omitempty"`
	RepeatedFailureReason    string           `json:"repeatedFailureReason,omitempty"`
	MissingDoDItems          []map[string]any `json:"missingDoDItems,omitempty"`
	RecentAttempts           []map[string]any `json:"recentAttempts,omitempty"`
	RecommendedNextSteps     []string         `json:"recommendedNextSteps,omitempty"`
	NextAction               string           `json:"nextAction,omitempty"`
	Escalated                bool             `json:"escalated"`
	EscalationReason         string           `json:"escalationReason,omitempty"`
	EscalationTicketID       string           `json:"escalationTicketId,omitempty"`
	EvidenceRefs             []string         `json:"evidenceRefs,omitempty"`
}

type ClosureTrace struct {
	StepID                              string           `json:"stepId"`
	Kind                                string           `json:"kind,omitempty"`
	State                               string           `json:"state,omitempty"`
	GraphFollowUp                       bool             `json:"graphFollowUp,omitempty"`
	AssessmentOnly                      bool             `json:"assessmentOnly,omitempty"`
	GraphExpansionLimitFollowUp         bool             `json:"graphExpansionLimitFollowUp,omitempty"`
	GraphFollowUpExpansionLimitExceeded bool             `json:"graphFollowUpExpansionLimitExceeded,omitempty"`
	GraphFollowUpExpansionCount         int              `json:"graphFollowUpExpansionCount,omitempty"`
	GraphFollowUpExpansionLimit         int              `json:"graphFollowUpExpansionLimit,omitempty"`
	IgnoredExecutionGraph               bool             `json:"ignoredExecutionGraph,omitempty"`
	Reason                              string           `json:"reason,omitempty"`
	MissingDoDItems                     []map[string]any `json:"missingDoDItems,omitempty"`
	EvidenceRefs                        []string         `json:"evidenceRefs,omitempty"`
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

type TaskPlan struct {
	ID               string   `json:"id"`
	WorkspaceID      string   `json:"workspaceId"`
	RunID            string   `json:"runId"`
	AnchorStepID     string   `json:"anchorStepId"`
	RevisionStepID   string   `json:"revisionStepId"`
	Revision         int      `json:"revision"`
	State            string   `json:"state"`
	Stage            string   `json:"stage,omitempty"`
	Summary          string   `json:"summary,omitempty"`
	GraphBranch      string   `json:"graphBranch,omitempty"`
	NodeTotal        int      `json:"nodeTotal"`
	NodeCompleted    int      `json:"nodeCompleted"`
	NodeFailed       int      `json:"nodeFailed"`
	NodeRunning      int      `json:"nodeRunning"`
	EvidenceRefs     []string `json:"evidenceRefs,omitempty"`
	OccurredAt       string   `json:"occurredAt,omitempty"`
	SourceProjection string   `json:"sourceProjection,omitempty"`
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
	ID                     string                       `json:"id"`
	WorkspaceID            string                       `json:"workspaceId"`
	RunID                  string                       `json:"runId"`
	StepID                 string                       `json:"stepId,omitempty"`
	GraphNodeID            string                       `json:"graphNodeId,omitempty"`
	Stage                  string                       `json:"stage,omitempty"`
	Source                 string                       `json:"source"`
	Policy                 string                       `json:"policy,omitempty"`
	Status                 string                       `json:"status"`
	ProviderID             string                       `json:"providerId,omitempty"`
	Operation              string                       `json:"operation,omitempty"`
	QueryTextPreview       string                       `json:"queryTextPreview,omitempty"`
	QueryTextSha256        string                       `json:"queryTextSha256,omitempty"`
	MinScore               float64                      `json:"minScore,omitempty"`
	Limit                  int                          `json:"limit,omitempty"`
	EmbeddingModel         string                       `json:"embeddingModel,omitempty"`
	VectorDimension        int                          `json:"vectorDimension,omitempty"`
	RefreshSource          string                       `json:"refreshSource,omitempty"`
	SelectionPolicyVersion string                       `json:"selectionPolicyVersion,omitempty"`
	CandidateMemoryIDs     []string                     `json:"candidateMemoryIds,omitempty"`
	SelectedMemoryIDs      []string                     `json:"selectedMemoryIds,omitempty"`
	ExcludedMemoryIDs      []string                     `json:"excludedMemoryIds,omitempty"`
	TypeQuota              map[string]int               `json:"typeQuota,omitempty"`
	SelectionScores        []MemoryRecallSelectionScore `json:"selectionScores,omitempty"`
	RetrievedMemoryIDs     []string                     `json:"retrievedMemoryIds,omitempty"`
	RetrievedMemoryHits    []MemoryRecallHit            `json:"retrievedMemoryHits,omitempty"`
	Reason                 string                       `json:"reason,omitempty"`
	OccurredAt             string                       `json:"occurredAt,omitempty"`
	EvidenceRef            string                       `json:"evidenceRef,omitempty"`
}

type MemoryRecallHit struct {
	ID              string  `json:"id"`
	Summary         string  `json:"summary,omitempty"`
	VectorScore     float64 `json:"vectorScore,omitempty"`
	VectorDimension int     `json:"vectorDimension,omitempty"`
}

type MemoryRecallSelectionScore struct {
	ID     string `json:"id"`
	Score  int    `json:"score"`
	Reason string `json:"reason,omitempty"`
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
	ID               string `json:"id"`
	WorkspaceID      string `json:"workspaceId"`
	RunID            string `json:"runId"`
	StepID           string `json:"stepId,omitempty"`
	ToolCallID       string `json:"toolCallId,omitempty"`
	ApprovalID       string `json:"approvalId,omitempty"`
	RevertApprovalID string `json:"revertApprovalId,omitempty"`
	PatchType        string `json:"patchType"`
	TargetRef        string `json:"targetRef,omitempty"`
	Summary          string `json:"summary,omitempty"`
	State            string `json:"state"`
	AddedLines       int    `json:"addedLines,omitempty"`
	DeletedLines     int    `json:"deletedLines,omitempty"`
	BeforeHash       string `json:"beforeHash,omitempty"`
	AfterHash        string `json:"afterHash,omitempty"`
	CreatedAt        string `json:"createdAt,omitempty"`
	AppliedAt        string `json:"appliedAt,omitempty"`
	RevertedAt       string `json:"revertedAt,omitempty"`
}

type WebSourceTrace struct {
	Query          string             `json:"query,omitempty"`
	ProviderID     string             `json:"providerId,omitempty"`
	SearchedAt     string             `json:"searchedAt,omitempty"`
	Sources        []WebSource        `json:"sources,omitempty"`
	FetchedSources []FetchedWebSource `json:"fetchedSources,omitempty"`
	EvidenceRefs   []string           `json:"evidenceRefs,omitempty"`
	ApprovalID     string             `json:"approvalId,omitempty"`
	PolicyDecision string             `json:"policyDecision,omitempty"`
}

type LLMContextBudgetTrace struct {
	StepID                string         `json:"stepId"`
	ProviderID            string         `json:"providerId,omitempty"`
	Model                 string         `json:"model,omitempty"`
	MaxBytes              int            `json:"maxBytes"`
	ActualBytes           int            `json:"actualBytes"`
	MaxTokens             int            `json:"maxTokens,omitempty"`
	EstimatedInputTokens  int            `json:"estimatedInputTokens,omitempty"`
	EstimatedOutputTokens int            `json:"estimatedOutputTokens,omitempty"`
	ActualTokens          int            `json:"actualTokens,omitempty"`
	ShrinkLevel           int            `json:"shrinkLevel,omitempty"`
	Truncated             bool           `json:"truncated"`
	Rules                 []string       `json:"rules,omitempty"`
	SourceRefs            []string       `json:"sourceRefs,omitempty"`
	IncludedSections      []string       `json:"includedSections,omitempty"`
	NormalizedSections    []string       `json:"normalizedSections,omitempty"`
	CompressedSections    []string       `json:"compressedSections,omitempty"`
	OmittedSections       []string       `json:"omittedSections,omitempty"`
	GoalDoDCompressed     bool           `json:"goalDoDCompressed"`
	OverBudget            bool           `json:"overBudget"`
	P0Overflow            bool           `json:"p0Overflow"`
	ProfileMode           string         `json:"profileMode,omitempty"`
	CapabilitySource      string         `json:"capabilitySource,omitempty"`
	ContextUtilization    float64        `json:"contextUtilizationRatio,omitempty"`
	MaxRequestBytes       int            `json:"maxRequestBytes,omitempty"`
	StageBudgetRatio      float64        `json:"stageBudgetRatio,omitempty"`
	StageBudgetSource     string         `json:"stageBudgetSource,omitempty"`
	TracePolicy           string         `json:"tracePolicy,omitempty"`
	PromptCaptured        bool           `json:"promptCaptured"`
	Estimator             string         `json:"estimator,omitempty"`
	Profile               map[string]any `json:"profile,omitempty"`
}

type LLMUsageTrace struct {
	ID            string         `json:"id"`
	WorkspaceID   string         `json:"workspaceId"`
	RunID         string         `json:"runId"`
	StepID        string         `json:"stepId,omitempty"`
	ProviderID    string         `json:"providerId,omitempty"`
	Provider      string         `json:"provider,omitempty"`
	Model         string         `json:"model,omitempty"`
	Operation     string         `json:"operation,omitempty"`
	InputTokens   int            `json:"inputTokens,omitempty"`
	OutputTokens  int            `json:"outputTokens,omitempty"`
	TotalTokens   int            `json:"totalTokens,omitempty"`
	Estimated     bool           `json:"estimated"`
	UsageSource   string         `json:"usageSource,omitempty"`
	EstimatedCost float64        `json:"estimatedCost,omitempty"`
	CostEstimated bool           `json:"costEstimated"`
	CostSource    string         `json:"costSource,omitempty"`
	Currency      string         `json:"currency,omitempty"`
	RequestBytes  int            `json:"requestBytes,omitempty"`
	ResponseBytes int            `json:"responseBytes,omitempty"`
	Streaming     bool           `json:"streaming"`
	ContextBudget map[string]any `json:"contextBudget,omitempty"`
	StartedAt     string         `json:"startedAt,omitempty"`
	CompletedAt   string         `json:"completedAt,omitempty"`
	DurationMs    int            `json:"durationMs,omitempty"`
	Status        string         `json:"status,omitempty"`
	FailureClass  string         `json:"failureClass,omitempty"`
	OccurredAt    string         `json:"occurredAt,omitempty"`
	EvidenceRef   string         `json:"evidenceRef,omitempty"`
}

type LLMUsageSummary struct {
	EntryTotal             int                              `json:"entryTotal"`
	InputTokens            int                              `json:"inputTokens"`
	OutputTokens           int                              `json:"outputTokens"`
	TotalTokens            int                              `json:"totalTokens"`
	EstimatedCost          float64                          `json:"estimatedCost,omitempty"`
	Currency               string                           `json:"currency,omitempty"`
	EstimatedEntries       int                              `json:"estimatedEntries"`
	ProviderNativeEntries  int                              `json:"providerNativeEntries"`
	CostEstimatedEntries   int                              `json:"costEstimatedEntries"`
	FailedEntries          int                              `json:"failedEntries"`
	CostBreakdown          []LLMUsageCostBreakdown          `json:"costBreakdown,omitempty"`
	ProviderModelBreakdown []LLMUsageProviderModelBreakdown `json:"providerModelBreakdown,omitempty"`
}

type LLMUsageCostBreakdown struct {
	Currency      string  `json:"currency"`
	EstimatedCost float64 `json:"estimatedCost"`
	EntryTotal    int     `json:"entryTotal"`
}

type LLMUsageProviderModelBreakdown struct {
	ProviderID       string  `json:"providerId,omitempty"`
	Provider         string  `json:"provider,omitempty"`
	Model            string  `json:"model,omitempty"`
	Currency         string  `json:"currency,omitempty"`
	EntryTotal       int     `json:"entryTotal"`
	InputTokens      int     `json:"inputTokens"`
	OutputTokens     int     `json:"outputTokens"`
	TotalTokens      int     `json:"totalTokens"`
	EstimatedCost    float64 `json:"estimatedCost,omitempty"`
	EstimatedEntries int     `json:"estimatedEntries"`
	FailedEntries    int     `json:"failedEntries"`
}

type WebSource struct {
	Title       string `json:"title,omitempty"`
	URL         string `json:"url"`
	Snippet     string `json:"snippet,omitempty"`
	Source      string `json:"source,omitempty"`
	PublishedAt string `json:"publishedAt,omitempty"`
	Rank        int    `json:"rank,omitempty"`
	EvidenceRef string `json:"evidenceRef,omitempty"`
}

type FetchedWebSource struct {
	URL         string `json:"url"`
	FinalURL    string `json:"finalUrl,omitempty"`
	Title       string `json:"title,omitempty"`
	Source      string `json:"source,omitempty"`
	RetrievedAt string `json:"retrievedAt,omitempty"`
	ContentHash string `json:"contentHash,omitempty"`
	Truncated   bool   `json:"truncated,omitempty"`
	EvidenceRef string `json:"evidenceRef,omitempty"`
}

type ValidationResult struct {
	ID              string   `json:"id"`
	WorkspaceID     string   `json:"workspaceId"`
	RunID           string   `json:"runId"`
	StepID          string   `json:"stepId,omitempty"`
	ValidationKind  string   `json:"validationKind"`
	TargetRef       string   `json:"targetRef,omitempty"`
	State           string   `json:"state"`
	Summary         string   `json:"summary,omitempty"`
	EvidenceRef     string   `json:"evidenceRef,omitempty"`
	EvidenceQuality string   `json:"evidenceQuality,omitempty"`
	EvidenceReason  string   `json:"evidenceReason,omitempty"`
	ZeroWork        bool     `json:"zeroWork,omitempty"`
	EvidenceTags    []string `json:"evidenceTags,omitempty"`
	StartedAt       string   `json:"startedAt,omitempty"`
	EndedAt         string   `json:"endedAt,omitempty"`
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
	Satisfied            bool                `json:"satisfied"`
	Items                []DoDAssessmentItem `json:"items,omitempty"`
	GoalCoverage         *DoDCoverage        `json:"goalCoverage,omitempty"`
	AcceptanceDimensions *DoDCoverage        `json:"acceptanceDimensions,omitempty"`
	DeliverableCoverage  *DoDCoverage        `json:"deliverableCoverage,omitempty"`
	Summary              string              `json:"summary,omitempty"`
}

type DoDAssessmentItem struct {
	Item          string   `json:"item"`
	State         string   `json:"state"`
	EvidenceRefs  []string `json:"evidenceRefs,omitempty"`
	FailureReason string   `json:"failureReason,omitempty"`
	Confidence    float64  `json:"confidence,omitempty"`
	SourceNote    string   `json:"sourceNote,omitempty"`
}

type DoDCoverage struct {
	Satisfied bool              `json:"satisfied"`
	Items     []DoDCoverageItem `json:"items,omitempty"`
	Summary   string            `json:"summary,omitempty"`
}

type DoDCoverageItem struct {
	ID           string   `json:"id,omitempty"`
	Deliverable  string   `json:"deliverable,omitempty"`
	Name         string   `json:"name,omitempty"`
	Path         string   `json:"path,omitempty"`
	Goal         string   `json:"goal,omitempty"`
	GoalItem     string   `json:"goalItem,omitempty"`
	Item         string   `json:"item,omitempty"`
	Requirement  string   `json:"requirement,omitempty"`
	State        string   `json:"state"`
	EvidenceRefs []string `json:"evidenceRefs,omitempty"`
	ArtifactRefs []string `json:"artifactRefs,omitempty"`
}

type DoDEvidenceTrace struct {
	ResultID  string            `json:"resultId"`
	DoDID     string            `json:"doDId,omitempty"`
	Satisfied bool              `json:"satisfied"`
	Summary   string            `json:"summary,omitempty"`
	Items     []DoDEvidenceItem `json:"items,omitempty"`
}

type DoDEvidenceItem struct {
	Item                string   `json:"item"`
	State               string   `json:"state"`
	FailureReason       string   `json:"failureReason,omitempty"`
	Confidence          float64  `json:"confidence,omitempty"`
	SourceNote          string   `json:"sourceNote,omitempty"`
	EvidenceRefs        []string `json:"evidenceRefs,omitempty"`
	EvidenceStatus      string   `json:"evidenceStatus,omitempty"`
	MissingEvidenceRefs []string `json:"missingEvidenceRefs,omitempty"`
}

type ResultQualityTrace struct {
	ResultID     string               `json:"resultId"`
	DoDID        string               `json:"doDId,omitempty"`
	State        string               `json:"state"`
	Summary      string               `json:"summary,omitempty"`
	EvidenceRefs []string             `json:"evidenceRefs,omitempty"`
	Issues       []ResultQualityIssue `json:"issues,omitempty"`
}

type ResultQualityIssue struct {
	Code         string   `json:"code"`
	Severity     string   `json:"severity"`
	Message      string   `json:"message"`
	Suggestion   string   `json:"suggestion,omitempty"`
	EvidenceRefs []string `json:"evidenceRefs,omitempty"`
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
	Workspace          Workspace               `json:"workspace"`
	Run                *Run                    `json:"run,omitempty"`
	Step               *presenter.StepView     `json:"step,omitempty"`
	Result             *Result                 `json:"result,omitempty"`
	Outcome            WorkspaceOutcome        `json:"outcome"`
	TaskProgress       TaskProgress            `json:"taskProgress"`
	TaskDecompositions []TaskDecomposition     `json:"taskDecompositions,omitempty"`
	ExecutionSections  []ExecutionSection      `json:"executionSections,omitempty"`
	MemoryRecallTraces []MemoryRecallTrace     `json:"memoryRecallTraces,omitempty"`
	Goals              []Goal                  `json:"goals,omitempty"`
	DoDs               []DoD                   `json:"dods,omitempty"`
	Runs               []Run                   `json:"runs,omitempty"`
	Results            []Result                `json:"results,omitempty"`
	Artifacts          []Artifact              `json:"artifacts,omitempty"`
	Patches            []Patch                 `json:"patches,omitempty"`
	WebSources         []WebSourceTrace        `json:"webSources,omitempty"`
	LLMContextBudgets  []LLMContextBudgetTrace `json:"llmContextBudgets,omitempty"`
	LLMUsageTraces     []LLMUsageTrace         `json:"llmUsageTraces,omitempty"`
	LLMUsageSummary    LLMUsageSummary         `json:"llmUsageSummary"`
	EventRecords       []EventRecord           `json:"eventRecords,omitempty"`
	Decisions          []Decision              `json:"decisions,omitempty"`
	Approvals          []Approval              `json:"approvals,omitempty"`
}
