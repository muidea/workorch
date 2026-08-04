package workorchclient

import (
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// Workspace resource types (matches workorchd HTTP JSON format with lowercase camelCase tags).
type Workspace struct {
	ID                   string                               `json:"id"`
	Title                string                               `json:"title"`
	Description          string                               `json:"description,omitempty"`
	DeliverableDirectory string                               `json:"deliverableDirectory,omitempty"`
	State                string                               `json:"state"`
	ActiveRunID          string                               `json:"activeRunId,omitempty"`
	BlockingTicketID     string                               `json:"blockingTicketId,omitempty"`
	BlockingApprovalID   string                               `json:"blockingApprovalId,omitempty"`
	TemplateID           string                               `json:"templateId,omitempty"`
	ProfileID            string                               `json:"profileId,omitempty"`
	DefaultLLMTarget     *runtimecontract.ProviderModelTarget `json:"defaultLLMTarget,omitempty"`
	DefaultSoulID        string                               `json:"defaultSoulId,omitempty"`
	SoulBindings         map[string]string                    `json:"soulBindings,omitempty"`
	ContextHints         map[string]any                       `json:"contextHints,omitempty"`
	EvidencePolicies     []map[string]any                     `json:"evidencePolicies,omitempty"`
	Projection           ProjectionStatus                     `json:"projection,omitempty"`
	ViewModel            WorkspaceViewModel                   `json:"viewModel,omitempty"`
}

type ProjectionStatus struct {
	State                   string   `json:"state"`
	StreamGeneration        string   `json:"streamGeneration,omitempty"`
	GlobalRevision          int64    `json:"globalRevision"`
	WorkspaceRevision       int64    `json:"workspaceRevision,omitempty"`
	SourceEventRevision     int64    `json:"sourceEventRevision,omitempty"`
	ProjectedSourceRevision int64    `json:"projectedSourceRevision,omitempty"`
	WorkspaceTotal          int      `json:"workspaceTotal,omitempty"`
	UpdatedAt               string   `json:"updatedAt,omitempty"`
	RebuiltAt               string   `json:"rebuiltAt,omitempty"`
	Degraded                bool     `json:"degraded,omitempty"`
	StaleReason             string   `json:"staleReason,omitempty"`
	Error                   string   `json:"error,omitempty"`
	FailedWorkspaceIds      []string `json:"failedWorkspaceIds,omitempty"`
	DegradedSince           string   `json:"degradedSince,omitempty"`
}

type WorkspaceListProjection struct {
	Projection ProjectionStatus `json:"projection"`
	Workspaces []Workspace      `json:"workspaces"`
}

// Run resource types.
type Run struct {
	ID                 string                                      `json:"id"`
	WorkspaceID        string                                      `json:"workspaceId"`
	GoalID             string                                      `json:"goalId,omitempty"`
	DoDID              string                                      `json:"doDId,omitempty"`
	DefinitionRevision string                                      `json:"definitionRevision,omitempty"`
	InitialStepKind    string                                      `json:"initialStepKind,omitempty"`
	Lineage            projectioncontract.RunLineage               `json:"lineage,omitempty"`
	State              string                                      `json:"state"`
	CurrentStepID      string                                      `json:"currentStepId,omitempty"`
	CurrentStepKind    string                                      `json:"currentStepKind,omitempty"`
	CurrentStepState   string                                      `json:"currentStepState,omitempty"`
	Priority           int                                         `json:"priority,omitempty"`
	BlockingTicketID   string                                      `json:"blockingTicketId,omitempty"`
	BlockingApprovalID string                                      `json:"blockingApprovalId,omitempty"`
	FailureReason      string                                      `json:"failureReason,omitempty"`
	SealedLLMTarget    *runtimecontract.ProviderModelTarget        `json:"sealedLLMTarget,omitempty"`
	RuntimeActivation  *projectioncontract.RuntimeActivationStatus `json:"runtimeActivation,omitempty"`
}

type Step struct {
	ID            string         `json:"id"`
	RunID         string         `json:"runId"`
	Kind          string         `json:"kind"`
	State         string         `json:"state"`
	Input         map[string]any `json:"input,omitempty"`
	Output        map[string]any `json:"output,omitempty"`
	ContractAudit map[string]any `json:"contractAudit,omitempty"`
}

// Decision & Approval types.
type Decision struct {
	ID                     string                       `json:"id"`
	WorkspaceID            string                       `json:"workspaceId"`
	RunID                  string                       `json:"runId"`
	StepID                 string                       `json:"stepId,omitempty"`
	Reason                 string                       `json:"reason"`
	DecisionKind           string                       `json:"decisionKind,omitempty"`
	Question               string                       `json:"question,omitempty"`
	Context                workorcheventpayload.Payload `json:"context,omitempty"`
	FailureClass           string                       `json:"failureClass,omitempty"`
	FailureSignature       string                       `json:"failureSignature,omitempty"`
	AllowedRecoveryActions []string                     `json:"allowedRecoveryActions,omitempty"`
	State                  string                       `json:"state"`
	ResolutionID           string                       `json:"resolutionId,omitempty"`
	ResolutionType         string                       `json:"resolutionType,omitempty"`
	ContinuationAction     string                       `json:"continuationAction,omitempty"`
	ResumeRunID            string                       `json:"resumeRunId,omitempty"`
	SuccessorRunID         string                       `json:"successorRunId,omitempty"`
	AutoGovernance         *AutoGovernanceSummary       `json:"autoGovernance,omitempty"`
}

type Approval struct {
	ID             string                       `json:"id"`
	WorkspaceID    string                       `json:"workspaceId"`
	RunID          string                       `json:"runId"`
	StepID         string                       `json:"stepId,omitempty"`
	Reason         string                       `json:"reason"`
	Action         string                       `json:"action,omitempty"`
	Context        workorcheventpayload.Payload `json:"context,omitempty"`
	State          string                       `json:"state"`
	AutoGovernance *AutoGovernanceSummary       `json:"autoGovernance,omitempty"`
}

type AutoGovernanceSummary struct {
	TraceID           string   `json:"traceId"`
	Action            string   `json:"action"`
	Status            string   `json:"status"`
	RiskLevel         string   `json:"riskLevel,omitempty"`
	PolicyMatched     string   `json:"policyMatched,omitempty"`
	PolicyBlocked     string   `json:"policyBlocked,omitempty"`
	Confidence        float64  `json:"confidence,omitempty"`
	Reason            string   `json:"reason,omitempty"`
	EvidenceRefs      []string `json:"evidenceRefs,omitempty"`
	PromptFingerprint string   `json:"promptFingerprint,omitempty"`
	OccurredAt        string   `json:"occurredAt,omitempty"`
}

type EventRecordQuery struct {
	WorkspaceID    string
	RunID          string
	Topic          string
	SubjectType    string
	SubjectID      string
	CursorAfterID  string
	Limit          int
	IncludePayload bool
}

type RepairLoop struct {
	State                    string         `json:"state"`
	AttemptCount             int            `json:"attemptCount"`
	LastAttempt              int            `json:"lastAttempt,omitempty"`
	LastRepairStepID         string         `json:"lastRepairStepId,omitempty"`
	LastFailureStepID        string         `json:"lastFailureStepId,omitempty"`
	LastFailureClass         string         `json:"lastFailureClass,omitempty"`
	LastFailureSignature     string         `json:"lastFailureSignature,omitempty"`
	LastFailureTargetScope   string         `json:"lastFailureTargetScope,omitempty"`
	LastFailureAt            string         `json:"lastFailureAt,omitempty"`
	StructuredFailureSignals map[string]any `json:"structuredFailureSignals,omitempty"`
	RepairInputCompaction    map[string]any `json:"repairInputCompaction,omitempty"`
	LastRepairInputBytes     int            `json:"lastRepairInputBytes,omitempty"`
	RepeatedFailureReason    string         `json:"repeatedFailureReason,omitempty"`
	NextAction               string         `json:"nextAction,omitempty"`
	Escalated                bool           `json:"escalated"`
	EscalationReason         string         `json:"escalationReason,omitempty"`
	EscalationTicketID       string         `json:"escalationTicketId,omitempty"`
	EvidenceRefs             []string       `json:"evidenceRefs,omitempty"`
}

type TaskProgress struct {
	WorkspaceID        string                                 `json:"workspaceId"`
	RunID              string                                 `json:"runId,omitempty"`
	State              string                                 `json:"state"`
	Summary            string                                 `json:"summary,omitempty"`
	CurrentStepID      string                                 `json:"currentStepId,omitempty"`
	CurrentStepKind    string                                 `json:"currentStepKind,omitempty"`
	CurrentStepState   string                                 `json:"currentStepState,omitempty"`
	CurrentActionLabel string                                 `json:"currentActionLabel,omitempty"`
	DiagnosticLineage  string                                 `json:"diagnosticLineage,omitempty"`
	CurrentMilestone   string                                 `json:"currentMilestone,omitempty"`
	BlockingTicketID   string                                 `json:"blockingTicketId,omitempty"`
	BlockingApprovalID string                                 `json:"blockingApprovalId,omitempty"`
	Milestones         []projectioncontract.ProgressMilestone `json:"milestones,omitempty"`
	Recovery           projectioncontract.RecoveryPlan        `json:"recovery"`
	RepairLoop         RepairLoop                             `json:"repairLoop"`
	AssessmentOutcome  projectioncontract.AssessmentOutcome   `json:"assessmentOutcome,omitempty"`
	Runtime            *projectioncontract.RuntimeProgress    `json:"runtime,omitempty"`
}

type RunOutcome struct {
	WorkspaceID  string   `json:"workspaceId,omitempty"`
	State        string   `json:"state"`
	RunID        string   `json:"runId,omitempty"`
	ResultID     string   `json:"resultId,omitempty"`
	Accepted     bool     `json:"accepted"`
	Terminal     bool     `json:"terminal"`
	Summary      string   `json:"summary,omitempty"`
	Reason       string   `json:"reason,omitempty"`
	EvidenceRefs []string `json:"evidenceRefs,omitempty"`
	ArtifactRefs []string `json:"artifactRefs,omitempty"`
}

type ResultSummary struct {
	GoalOutcome           string                                 `json:"goalOutcome"`
	DoDItems              []projectioncontract.DoDAssessmentItem `json:"doDItems,omitempty"`
	ImplementationSummary string                                 `json:"implementationSummary,omitempty"`
	VerificationSummary   string                                 `json:"verificationSummary,omitempty"`
	EvidenceRefs          []string                               `json:"evidenceRefs,omitempty"`
	ArtifactRefs          []string                               `json:"artifactRefs,omitempty"`
	KnownLimitations      []string                               `json:"knownLimitations,omitempty"`
	ResidualRisks         []string                               `json:"residualRisks,omitempty"`
	NextActions           []string                               `json:"nextActions,omitempty"`
	Diagnostics           map[string]any                         `json:"diagnostics,omitempty"`
}

type Result struct {
	ID            string                           `json:"id"`
	WorkspaceID   string                           `json:"workspaceId"`
	RunID         string                           `json:"runId"`
	GoalID        string                           `json:"goalId,omitempty"`
	DoDID         string                           `json:"doDId,omitempty"`
	State         string                           `json:"state"`
	Title         string                           `json:"title,omitempty"`
	Summary       string                           `json:"summary,omitempty"`
	SourceType    string                           `json:"sourceType,omitempty"`
	SourceRef     string                           `json:"sourceRef,omitempty"`
	EvidenceRefs  []string                         `json:"evidenceRefs,omitempty"`
	ArtifactRefs  []string                         `json:"artifactRefs,omitempty"`
	DoDAssessment projectioncontract.DoDAssessment `json:"doDAssessment,omitempty"`
	SummaryDetail ResultSummary                    `json:"summaryDetail,omitempty"`
	CreatedAt     string                           `json:"createdAt,omitempty"`
	UpdatedAt     string                           `json:"updatedAt,omitempty"`
}

type MemoryRecallTrace struct {
	ID                  string                               `json:"id"`
	WorkspaceID         string                               `json:"workspaceId"`
	RunID               string                               `json:"runId"`
	StepID              string                               `json:"stepId,omitempty"`
	GraphNodeID         string                               `json:"graphNodeId,omitempty"`
	Stage               string                               `json:"stage,omitempty"`
	Source              string                               `json:"source"`
	Policy              string                               `json:"policy,omitempty"`
	Status              string                               `json:"status"`
	ProviderID          string                               `json:"providerId,omitempty"`
	Operation           string                               `json:"operation,omitempty"`
	QueryTextPreview    string                               `json:"queryTextPreview,omitempty"`
	QueryTextSha256     string                               `json:"queryTextSha256,omitempty"`
	MinScore            float64                              `json:"minScore,omitempty"`
	Limit               int                                  `json:"limit,omitempty"`
	EmbeddingModel      string                               `json:"embeddingModel,omitempty"`
	VectorDimension     int                                  `json:"vectorDimension,omitempty"`
	RetrievedMemoryIDs  []string                             `json:"retrievedMemoryIds,omitempty"`
	RetrievedMemoryHits []projectioncontract.MemoryRecallHit `json:"retrievedMemoryHits,omitempty"`
	Reason              string                               `json:"reason,omitempty"`
	OccurredAt          string                               `json:"occurredAt,omitempty"`
	EvidenceRef         string                               `json:"evidenceRef,omitempty"`
}

type TaskDecompositionNode struct {
	ID               string         `json:"id,omitempty"`
	Kind             string         `json:"kind,omitempty"`
	Title            string         `json:"title,omitempty"`
	State            string         `json:"state,omitempty"`
	DependsOn        []string       `json:"dependsOn,omitempty"`
	Branch           string         `json:"branch,omitempty"`
	StepID           string         `json:"stepId,omitempty"`
	StepState        string         `json:"stepState,omitempty"`
	RetryOf          string         `json:"retryOf,omitempty"`
	RetryAttempt     int            `json:"retryAttempt,omitempty"`
	FailureClass     string         `json:"failureClass,omitempty"`
	FailureSignature string         `json:"failureSignature,omitempty"`
	TargetScope      string         `json:"targetScope,omitempty"`
	BlockingTicketID string         `json:"blockingTicketId,omitempty"`
	EvidenceRefs     []string       `json:"evidenceRefs,omitempty"`
	CoverageTargets  []string       `json:"coverageTargets,omitempty"`
	Input            map[string]any `json:"input,omitempty"`
}

type TaskDecomposition struct {
	ID            string                  `json:"id,omitempty"`
	WorkspaceID   string                  `json:"workspaceId,omitempty"`
	RunID         string                  `json:"runId,omitempty"`
	RootStepID    string                  `json:"rootStepId,omitempty"`
	State         string                  `json:"state,omitempty"`
	Summary       string                  `json:"summary,omitempty"`
	GraphBranch   string                  `json:"graphBranch,omitempty"`
	NodeTotal     int                     `json:"nodeTotal,omitempty"`
	NodeCompleted int                     `json:"nodeCompleted,omitempty"`
	NodeFailed    int                     `json:"nodeFailed,omitempty"`
	NodeRunning   int                     `json:"nodeRunning,omitempty"`
	Nodes         []TaskDecompositionNode `json:"nodes,omitempty"`
	EvidenceRefs  []string                `json:"evidenceRefs,omitempty"`
}

type TaskPlan struct {
	ID                string                  `json:"id,omitempty"`
	WorkspaceID       string                  `json:"workspaceId,omitempty"`
	RunID             string                  `json:"runId,omitempty"`
	AnchorStepID      string                  `json:"anchorStepId,omitempty"`
	RevisionStepID    string                  `json:"revisionStepId,omitempty"`
	Revision          int                     `json:"revision,omitempty"`
	State             string                  `json:"state,omitempty"`
	Stage             string                  `json:"stage,omitempty"`
	SourceOutputState string                  `json:"sourceOutputState,omitempty"`
	Summary           string                  `json:"summary,omitempty"`
	GraphBranch       string                  `json:"graphBranch,omitempty"`
	NodeTotal         int                     `json:"nodeTotal,omitempty"`
	NodeCompleted     int                     `json:"nodeCompleted,omitempty"`
	NodeFailed        int                     `json:"nodeFailed,omitempty"`
	NodeRunning       int                     `json:"nodeRunning,omitempty"`
	Nodes             []TaskDecompositionNode `json:"nodes,omitempty"`
	RetryOf           string                  `json:"retryOf,omitempty"`
	RetryAttempt      int                     `json:"retryAttempt,omitempty"`
	FailureClass      string                  `json:"failureClass,omitempty"`
	FailureSignature  string                  `json:"failureSignature,omitempty"`
	TargetScope       string                  `json:"targetScope,omitempty"`
	BlockingTicketID  string                  `json:"blockingTicketId,omitempty"`
	EvidenceRefs      []string                `json:"evidenceRefs,omitempty"`
	OccurredAt        string                  `json:"occurredAt,omitempty"`
	SourceProjection  string                  `json:"sourceProjection,omitempty"`
}

// Snapshot is the full workspace/run snapshot returned by the read model.
type WorkspaceSnapshot struct {
	Workspace               Workspace                                  `json:"workspace"`
	ViewModel               WorkspaceViewModel                         `json:"viewModel"`
	Outcome                 RunOutcome                                 `json:"outcome"`
	TaskProgress            TaskProgress                               `json:"taskProgress"`
	RunHeartbeat            projectioncontract.RunHeartbeat            `json:"runHeartbeat,omitempty"`
	ConvergenceStatus       *projectioncontract.ConvergenceStatus      `json:"convergenceStatus,omitempty"`
	RunCommand              *RunCommandStatus                          `json:"runCommand,omitempty"`
	SchedulerDiagnostics    *projectioncontract.SchedulerDiagnostics   `json:"schedulerDiagnostics,omitempty"`
	ContextBundle           map[string]any                             `json:"contextBundle,omitempty"`
	CoverageSummary         map[string]any                             `json:"coverageSummary,omitempty"`
	GapBaseline             *projectioncontract.GapBaseline            `json:"gapBaseline,omitempty"`
	Acceptance              projectioncontract.AcceptanceProjection    `json:"acceptance"`
	TaskDecompositions      []TaskDecomposition                        `json:"taskDecompositions,omitempty"`
	TaskPlans               []TaskPlan                                 `json:"taskPlans,omitempty"`
	Goals                   []workorchmanagementcontract.Goal          `json:"goals,omitempty"`
	DoDs                    []workorchmanagementcontract.DoD           `json:"dods,omitempty"`
	Runs                    []Run                                      `json:"runs,omitempty"`
	Results                 []Result                                   `json:"results,omitempty"`
	Artifacts               []projectioncontract.Artifact              `json:"artifacts,omitempty"`
	ArtifactContinuations   []projectioncontract.ArtifactContinuation  `json:"artifactContinuations,omitempty"`
	Decisions               []Decision                                 `json:"decisions,omitempty"`
	Approvals               []Approval                                 `json:"approvals,omitempty"`
	Steps                   []Step                                     `json:"steps,omitempty"`
	ToolCalls               []projectioncontract.ToolCall              `json:"toolCalls,omitempty"`
	Patches                 []projectioncontract.Patch                 `json:"patches,omitempty"`
	ValidationResults       []projectioncontract.ValidationResult      `json:"validationResults,omitempty"`
	RuntimeCheckpoints      []projectioncontract.RuntimeCheckpoint     `json:"runtimeCheckpoints,omitempty"`
	MemoryRecallTraces      []MemoryRecallTrace                        `json:"memoryRecallTraces,omitempty"`
	LLMContextBudgets       []projectioncontract.LLMContextBudgetTrace `json:"llmContextBudgets,omitempty"`
	LLMContextBudgetSummary projectioncontract.LLMContextBudgetSummary `json:"llmContextBudgetSummary"`
	LLMUsageTraces          []projectioncontract.LLMUsageTrace         `json:"llmUsageTraces,omitempty"`
	LLMUsageSummary         projectioncontract.LLMUsageSummary         `json:"llmUsageSummary"`
	EventRecords            []projectioncontract.EventRecord           `json:"eventRecords,omitempty"`
}

type WorkspaceViewModel struct {
	WorkspaceID             string                               `json:"workspaceId,omitempty"`
	State                   string                               `json:"state,omitempty"`
	PromptMode              string                               `json:"promptMode,omitempty"`
	Revision                int64                                `json:"revision,omitempty"`
	GlobalRevision          int64                                `json:"globalRevision,omitempty"`
	WorkspaceRevision       int64                                `json:"workspaceRevision,omitempty"`
	SourceEventRevision     int64                                `json:"sourceEventRevision,omitempty"`
	ProjectedSourceRevision int64                                `json:"projectedSourceRevision,omitempty"`
	UpdatedAt               string                               `json:"updatedAt,omitempty"`
	RebuiltAt               string                               `json:"rebuiltAt,omitempty"`
	Degraded                bool                                 `json:"degraded,omitempty"`
	StaleReason             string                               `json:"staleReason,omitempty"`
	Error                   string                               `json:"error,omitempty"`
	Sections                map[string]WorkspaceViewModelSection `json:"sections,omitempty"`
}

type WorkspaceViewModelSection struct {
	Name                    string `json:"name"`
	Revision                int64  `json:"revision,omitempty"`
	WorkspaceRevision       int64  `json:"workspaceRevision,omitempty"`
	SourceEventRevision     int64  `json:"sourceEventRevision,omitempty"`
	ProjectedSourceRevision int64  `json:"projectedSourceRevision,omitempty"`
	State                   string `json:"state,omitempty"`
}

type RunCommandStatus struct {
	WorkspaceID        string `json:"workspaceId,omitempty"`
	RunID              string `json:"runId,omitempty"`
	State              string `json:"state,omitempty"`
	Intent             string `json:"intent,omitempty"`
	Reason             string `json:"reason,omitempty"`
	RequestID          string `json:"requestId,omitempty"`
	IdempotencyKey     string `json:"idempotencyKey,omitempty"`
	Source             string `json:"source,omitempty"`
	DefinitionRevision string `json:"definitionRevision,omitempty"`
	DecisionID         string `json:"decisionId,omitempty"`
	AcceptedAt         string `json:"acceptedAt,omitempty"`
	RejectedAt         string `json:"rejectedAt,omitempty"`
	Error              string `json:"error,omitempty"`
	Revision           int64  `json:"revision,omitempty"`
}

type RunSnapshot struct {
	Workspace               Workspace                                  `json:"workspace"`
	Run                     Run                                        `json:"run"`
	Outcome                 RunOutcome                                 `json:"outcome"`
	TaskProgress            TaskProgress                               `json:"taskProgress"`
	RepairLoop              RepairLoop                                 `json:"repairLoop"`
	TaskDecompositions      []TaskDecomposition                        `json:"taskDecompositions,omitempty"`
	TaskPlans               []TaskPlan                                 `json:"taskPlans,omitempty"`
	ConvergenceStatus       *projectioncontract.ConvergenceStatus      `json:"convergenceStatus,omitempty"`
	SchedulerDiagnostics    *projectioncontract.SchedulerDiagnostics   `json:"schedulerDiagnostics,omitempty"`
	Steps                   []Step                                     `json:"steps,omitempty"`
	ToolCalls               []projectioncontract.ToolCall              `json:"toolCalls,omitempty"`
	Patches                 []projectioncontract.Patch                 `json:"patches,omitempty"`
	ValidationResults       []projectioncontract.ValidationResult      `json:"validationResults,omitempty"`
	RuntimeCheckpoints      []projectioncontract.RuntimeCheckpoint     `json:"runtimeCheckpoints,omitempty"`
	Results                 []Result                                   `json:"results,omitempty"`
	Artifacts               []projectioncontract.Artifact              `json:"artifacts,omitempty"`
	ArtifactContinuations   []projectioncontract.ArtifactContinuation  `json:"artifactContinuations,omitempty"`
	Decisions               []Decision                                 `json:"decisions,omitempty"`
	Approvals               []Approval                                 `json:"approvals,omitempty"`
	MemoryRecallTraces      []MemoryRecallTrace                        `json:"memoryRecallTraces,omitempty"`
	LLMContextBudgets       []projectioncontract.LLMContextBudgetTrace `json:"llmContextBudgets,omitempty"`
	LLMContextBudgetSummary projectioncontract.LLMContextBudgetSummary `json:"llmContextBudgetSummary"`
	LLMUsageTraces          []projectioncontract.LLMUsageTrace         `json:"llmUsageTraces,omitempty"`
	LLMUsageSummary         projectioncontract.LLMUsageSummary         `json:"llmUsageSummary"`
	CoverageWorkItems       []map[string]any                           `json:"coverageWorkItems,omitempty"`
	CoverageWorkItemContext map[string]any                             `json:"coverageWorkItemContext,omitempty"`
	GapBaseline             *projectioncontract.GapBaseline            `json:"gapBaseline,omitempty"`
	Acceptance              projectioncontract.AcceptanceProjection    `json:"acceptance"`
	EventRecords            []projectioncontract.EventRecord           `json:"eventRecords,omitempty"`
}

type Activation struct {
	Workspace Workspace         `json:"workspace"`
	Run       Run               `json:"run"`
	Snapshot  WorkspaceSnapshot `json:"snapshot"`
}

type WorkspaceSnapshotProjection struct {
	Projection ProjectionStatus  `json:"projection"`
	Snapshot   WorkspaceSnapshot `json:"snapshot"`
}

type WorkspaceActivityItem struct {
	ID                   string `json:"id"`
	WorkspaceID          string `json:"workspaceId"`
	RunID                string `json:"runId,omitempty"`
	StepID               string `json:"stepId,omitempty"`
	Kind                 string `json:"kind"`
	State                string `json:"state,omitempty"`
	Title                string `json:"title,omitempty"`
	Summary              string `json:"summary,omitempty"`
	OccurredAt           string `json:"occurredAt,omitempty"`
	SourceEventRevision  int64  `json:"sourceEventRevision,omitempty"`
	RuntimeStateRevision uint64 `json:"runtimeStateRevision,omitempty"`
	EvidenceSequence     uint64 `json:"evidenceSequence,omitempty"`
}

type WorkspaceActivityFeed struct {
	WorkspaceID      string                  `json:"workspaceId"`
	DisplayRunID     string                  `json:"displayRunId,omitempty"`
	ActivityRevision int64                   `json:"activityRevision,omitempty"`
	Items            []WorkspaceActivityItem `json:"items,omitempty"`
	Truncated        bool                    `json:"truncated,omitempty"`
	NextCursor       string                  `json:"nextCursor,omitempty"`
}

type WorkspaceRuntimeView struct {
	WorkspaceID       string                                      `json:"workspaceId"`
	ActiveRunID       string                                      `json:"activeRunId,omitempty"`
	DisplayRunID      string                                      `json:"displayRunId,omitempty"`
	Projection        WorkspaceViewModel                          `json:"projection"`
	Revision          projectioncontract.WorkspaceRefreshRevision `json:"revision"`
	PromptMode        string                                      `json:"promptMode,omitempty"`
	RunState          string                                      `json:"runState,omitempty"`
	TaskProgress      TaskProgress                                `json:"taskProgress"`
	RunHeartbeat      projectioncontract.RunHeartbeat             `json:"runHeartbeat"`
	ConvergenceStatus *projectioncontract.ConvergenceStatus       `json:"convergenceStatus,omitempty"`
	Acceptance        projectioncontract.AcceptanceProjection     `json:"acceptance"`
	RunCommand        *RunCommandStatus                           `json:"runCommand,omitempty"`
	BlockingDecision  *Decision                                   `json:"blockingDecision,omitempty"`
	BlockingApproval  *Approval                                   `json:"blockingApproval,omitempty"`
	Outcome           RunOutcome                                  `json:"outcome"`
	Activity          WorkspaceActivityFeed                       `json:"activity"`
}

type WorkspaceRuntimeViewProjection struct {
	Service     string               `json:"service"`
	Backend     string               `json:"backend,omitempty"`
	DataRoot    string               `json:"dataRoot,omitempty"`
	Projection  ProjectionStatus     `json:"projection"`
	RuntimeView WorkspaceRuntimeView `json:"runtimeView"`
	Error       string               `json:"error,omitempty"`
}

type RunObservability struct {
	Run                     Run                                        `json:"run"`
	Outcome                 RunOutcome                                 `json:"outcome"`
	TaskProgress            TaskProgress                               `json:"taskProgress"`
	RepairLoop              RepairLoop                                 `json:"repairLoop"`
	Acceptance              projectioncontract.AcceptanceProjection    `json:"acceptance"`
	LLMContextBudgets       []projectioncontract.LLMContextBudgetTrace `json:"llmContextBudgets,omitempty"`
	LLMContextBudgetSummary projectioncontract.LLMContextBudgetSummary `json:"llmContextBudgetSummary"`
	LLMUsageTraces          []projectioncontract.LLMUsageTrace         `json:"llmUsageTraces,omitempty"`
	LLMUsageSummary         projectioncontract.LLMUsageSummary         `json:"llmUsageSummary"`
	Timeline                []TimelineEntry                            `json:"timeline,omitempty"`
}

type TimelineEntry struct {
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

// Workspace soul bindings.
type WorkspaceSoulBindings struct {
	WorkspaceID     string            `json:"workspaceId,omitempty"`
	DefaultSoulID   string            `json:"defaultSoulId,omitempty"`
	Bindings        map[string]string `json:"bindings,omitempty"`
	PriorityPolicy  []string          `json:"priorityPolicy,omitempty"`
	AvailableStages []string          `json:"availableStages,omitempty"`
}

// Configuration types.
type AIProxyConfigurationView struct {
	BaseURL                 string `json:"baseUrl"`
	APIKeyConfigured        bool   `json:"apiKeyConfigured"`
	Timeout                 string `json:"timeout,omitempty"`
	CatalogRequestTimeout   string `json:"catalogRequestTimeout,omitempty"`
	CatalogTTL              string `json:"catalogTTL,omitempty"`
	CatalogMaxStaleAge      string `json:"catalogMaxStaleAge,omitempty"`
	MaxCatalogResponseBytes int    `json:"maxCatalogResponseBytes,omitempty"`
	MaxCatalogModels        int    `json:"maxCatalogModels,omitempty"`
}

type LLMContextBudgetPolicyView struct {
	ContextUtilizationRatio float64 `json:"contextUtilizationRatio"`
	EstimatedTokensPerByte  float64 `json:"estimatedTokensPerByte"`
}

type CustomProviderModelView struct {
	ID                  string   `json:"id"`
	ContextWindowTokens int      `json:"contextWindowTokens"`
	MaxOutputTokens     int      `json:"maxOutputTokens"`
	Operations          []string `json:"operations"`
}

type CustomProviderConfigurationView struct {
	ID               string                    `json:"id"`
	Provider         string                    `json:"provider"`
	APISurface       string                    `json:"apiSurface,omitempty"`
	BaseURL          string                    `json:"baseUrl,omitempty"`
	Timeout          string                    `json:"timeout,omitempty"`
	APIKeyConfigured bool                      `json:"apiKeyConfigured"`
	Models           []CustomProviderModelView `json:"models"`
}

// Keep legacy provider profile structs for usage/trace DTOs that still project them.
type ProviderCacheCapability struct {
	Surface             string   `json:"surface,omitempty"`
	Mode                string   `json:"mode,omitempty"`
	PromptCacheKey      bool     `json:"promptCacheKey,omitempty"`
	MessageCacheControl bool     `json:"messageCacheControl,omitempty"`
	UsageFields         []string `json:"usageFields,omitempty"`
}

type ProviderInferenceProfile struct {
	Temperature          *float64 `json:"temperature,omitempty"`
	MaxOutputTokens      int      `json:"maxOutputTokens,omitempty"`
	ReasoningEffort      string   `json:"reasoningEffort,omitempty"`
	ThinkingBudgetTokens int      `json:"thinkingBudgetTokens,omitempty"`
}

type ProviderContextProfile struct {
	CapabilitySource        string  `json:"capabilitySource,omitempty"`
	ContextWindowTokens     int     `json:"contextWindowTokens,omitempty"`
	MaxInputTokens          int     `json:"maxInputTokens,omitempty"`
	ReservedOutputTokens    int     `json:"reservedOutputTokens,omitempty"`
	ContextUtilizationRatio float64 `json:"contextUtilizationRatio,omitempty"`
	MaxRequestBytes         int     `json:"maxRequestBytes,omitempty"`
	EstimatedTokensPerByte  float64 `json:"estimatedTokensPerByte,omitempty"`
}

type ServiceConfiguration struct {
	Server struct {
		Addr                  string `json:"addr"`
		ReadHeaderTimeout     string `json:"readHeaderTimeout"`
		ReadTimeout           string `json:"readTimeout"`
		WriteTimeout          string `json:"writeTimeout"`
		IdleTimeout           string `json:"idleTimeout"`
		ShutdownTimeout       string `json:"shutdownTimeout"`
		MaxConcurrentRuns     int64  `json:"maxConcurrentRuns"`
		DefaultDialogLanguage string `json:"defaultDialogLanguage,omitempty"`
	} `json:"server"`
	Repository struct {
		Backend string `json:"backend"`
		DataDir string `json:"dataDir"`
	} `json:"repository"`
	Runtime struct {
		DefaultSoulID            string `json:"defaultSoulId,omitempty"`
		MaxRuntimeRepairAttempts int    `json:"maxRuntimeRepairAttempts"`
		MaxProviderRetryAttempts int    `json:"maxProviderRetryAttempts"`
		ShellCommandTimeout      string `json:"shellCommandTimeout"`
		LLMOutput                struct {
			MaxPatchContentBytes               int `json:"maxPatchContentBytes"`
			MaxGraphBytes                      int `json:"maxGraphBytes"`
			MaxGraphFileWrites                 int `json:"maxGraphFileWrites"`
			SchemaInvalidLargePreviewThreshold int `json:"schemaInvalidLargePreviewThreshold"`
			MaxArtifactChunkBytes              int `json:"maxArtifactChunkBytes"`
			PreviousChunkTailBytes             int `json:"previousChunkTailBytes"`
			MaxChunkRetryAttempts              int `json:"maxChunkRetryAttempts"`
			MaxArtifactPlanAttempts            int `json:"maxArtifactPlanAttempts"`
			MaxArtifactReplanAttempts          int `json:"maxArtifactReplanAttempts"`
		} `json:"llmOutput"`
	} `json:"runtime"`
	LLM struct {
		AIProxy          AIProxyConfigurationView                          `json:"aiProxy"`
		DefaultChat      runtimecontract.ProviderModelTarget               `json:"defaultChat"`
		DefaultEmbedding *runtimecontract.ProviderModelTarget              `json:"defaultEmbedding,omitempty"`
		Budget           LLMContextBudgetPolicyView                        `json:"budget"`
		CustomProviders  []CustomProviderConfigurationView                 `json:"customProviders,omitempty"`
		Catalog          workorchmanagementcontract.ProviderCatalogSummary `json:"catalog"`
	} `json:"llm"`
	WebSearch struct {
		DefaultProviderID string `json:"defaultProviderId,omitempty"`
		Providers         []struct {
			ID               string            `json:"id"`
			Provider         string            `json:"provider"`
			SearchEndpoint   string            `json:"searchEndpoint,omitempty"`
			FetchEndpoint    string            `json:"fetchEndpoint,omitempty"`
			Headers          map[string]string `json:"headers,omitempty"`
			Timeout          string            `json:"timeout,omitempty"`
			MaxResults       int               `json:"maxResults,omitempty"`
			MaxFetchBytes    int64             `json:"maxFetchBytes,omitempty"`
			MaxRedirects     int               `json:"maxRedirects,omitempty"`
			AllowDomains     []string          `json:"allowDomains,omitempty"`
			DenyDomains      []string          `json:"denyDomains,omitempty"`
			Disabled         bool              `json:"disabled"`
			Ready            bool              `json:"ready"`
			Reason           string            `json:"reason,omitempty"`
			Default          bool              `json:"default"`
			APIKeyConfigured bool              `json:"apiKeyConfigured"`
		} `json:"providers"`
	} `json:"webSearch"`
	Governance struct {
		AutoTicket struct {
			Enabled       bool    `json:"enabled"`
			MinConfidence float64 `json:"minConfidence"`
			MaxPerRun     int     `json:"maxPerRun"`
		} `json:"autoTicket"`
		AutoApproval struct {
			Enabled             bool     `json:"enabled"`
			MinConfidence       float64  `json:"minConfidence"`
			MaxPerRun           int      `json:"maxPerRun"`
			AllowReadOnly       bool     `json:"allowReadOnly"`
			AllowWorkspaceWrite bool     `json:"allowWorkspaceWrite"`
			RequireHumanFor     []string `json:"requireHumanFor,omitempty"`
		} `json:"autoApproval"`
		Budget struct {
			MaxConsecutiveFailures int `json:"maxConsecutiveFailures"`
			MaxRepeatedSamePrompt  int `json:"maxRepeatedSamePrompt"`
		} `json:"budget"`
	} `json:"governance"`
}
