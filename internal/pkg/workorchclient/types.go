package workorchclient

import (
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// Workspace resource types (matches workorchd HTTP JSON format with lowercase camelCase tags).
type Workspace struct {
	ID                   string                                `json:"id"`
	Title                string                                `json:"title"`
	Description          string                                `json:"description,omitempty"`
	DeliverableDirectory string                                `json:"deliverableDirectory,omitempty"`
	State                string                                `json:"state"`
	ActiveRunID          string                                `json:"activeRunId,omitempty"`
	BlockingTicketID     string                                `json:"blockingTicketId,omitempty"`
	BlockingApprovalID   string                                `json:"blockingApprovalId,omitempty"`
	TemplateID           string                                `json:"templateId,omitempty"`
	ProfileID            string                                `json:"profileId,omitempty"`
	DefaultLLMTarget     *runtimecontract.ProviderModelTarget  `json:"defaultLLMTarget,omitempty"`
	DefaultSoulID        string                                `json:"defaultSoulId,omitempty"`
	SoulBindings         map[string]string                     `json:"soulBindings,omitempty"`
	ContextHints         map[string]any                        `json:"contextHints,omitempty"`
	EvidencePolicies     []map[string]any                      `json:"evidencePolicies,omitempty"`
	Projection           ProjectionStatus                      `json:"projection,omitempty"`
	ViewModel            projectioncontract.WorkspaceViewModel `json:"viewModel,omitempty"`
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

// Snapshot is the full workspace/run snapshot returned by the read model.
type WorkspaceSnapshot struct {
	Workspace               Workspace                                  `json:"workspace"`
	ViewModel               projectioncontract.WorkspaceViewModel      `json:"viewModel"`
	Outcome                 projectioncontract.WorkspaceOutcome        `json:"outcome"`
	TaskProgress            projectioncontract.TaskProgress            `json:"taskProgress"`
	RunHeartbeat            projectioncontract.RunHeartbeat            `json:"runHeartbeat,omitempty"`
	ConvergenceStatus       *projectioncontract.ConvergenceStatus      `json:"convergenceStatus,omitempty"`
	RunCommand              *projectioncontract.RunCommandStatus       `json:"runCommand,omitempty"`
	SchedulerDiagnostics    *projectioncontract.SchedulerDiagnostics   `json:"schedulerDiagnostics,omitempty"`
	ContextBundle           map[string]any                             `json:"contextBundle,omitempty"`
	CoverageSummary         map[string]any                             `json:"coverageSummary,omitempty"`
	GapBaseline             *projectioncontract.GapBaseline            `json:"gapBaseline,omitempty"`
	Acceptance              projectioncontract.AcceptanceProjection    `json:"acceptance"`
	TaskDecompositions      []projectioncontract.TaskDecomposition     `json:"taskDecompositions,omitempty"`
	TaskPlans               []projectioncontract.TaskPlan              `json:"taskPlans,omitempty"`
	Goals                   []workorchmanagementcontract.Goal          `json:"goals,omitempty"`
	DoDs                    []workorchmanagementcontract.DoD           `json:"dods,omitempty"`
	Runs                    []Run                                      `json:"runs,omitempty"`
	Results                 []projectioncontract.Result                `json:"results,omitempty"`
	Artifacts               []projectioncontract.Artifact              `json:"artifacts,omitempty"`
	ArtifactContinuations   []projectioncontract.ArtifactContinuation  `json:"artifactContinuations,omitempty"`
	Decisions               []projectioncontract.Decision              `json:"decisions,omitempty"`
	Approvals               []projectioncontract.Approval              `json:"approvals,omitempty"`
	Steps                   []Step                                     `json:"steps,omitempty"`
	ToolCalls               []projectioncontract.ToolCall              `json:"toolCalls,omitempty"`
	Patches                 []projectioncontract.Patch                 `json:"patches,omitempty"`
	ValidationResults       []projectioncontract.ValidationResult      `json:"validationResults,omitempty"`
	RuntimeCheckpoints      []projectioncontract.RuntimeCheckpoint     `json:"runtimeCheckpoints,omitempty"`
	MemoryRecallTraces      []projectioncontract.MemoryRecallTrace     `json:"memoryRecallTraces,omitempty"`
	LLMContextBudgets       []projectioncontract.LLMContextBudgetTrace `json:"llmContextBudgets,omitempty"`
	LLMContextBudgetSummary projectioncontract.LLMContextBudgetSummary `json:"llmContextBudgetSummary"`
	LLMUsageTraces          []projectioncontract.LLMUsageTrace         `json:"llmUsageTraces,omitempty"`
	LLMUsageSummary         projectioncontract.LLMUsageSummary         `json:"llmUsageSummary"`
	EventRecords            []projectioncontract.EventRecord           `json:"eventRecords,omitempty"`
}

type RunSnapshot struct {
	Workspace               Workspace                                  `json:"workspace"`
	Run                     Run                                        `json:"run"`
	Outcome                 projectioncontract.RunOutcome              `json:"outcome"`
	TaskProgress            projectioncontract.TaskProgress            `json:"taskProgress"`
	RepairLoop              projectioncontract.RepairLoop              `json:"repairLoop"`
	TaskDecompositions      []projectioncontract.TaskDecomposition     `json:"taskDecompositions,omitempty"`
	TaskPlans               []projectioncontract.TaskPlan              `json:"taskPlans,omitempty"`
	ConvergenceStatus       *projectioncontract.ConvergenceStatus      `json:"convergenceStatus,omitempty"`
	SchedulerDiagnostics    *projectioncontract.SchedulerDiagnostics   `json:"schedulerDiagnostics,omitempty"`
	Steps                   []Step                                     `json:"steps,omitempty"`
	ToolCalls               []projectioncontract.ToolCall              `json:"toolCalls,omitempty"`
	Patches                 []projectioncontract.Patch                 `json:"patches,omitempty"`
	ValidationResults       []projectioncontract.ValidationResult      `json:"validationResults,omitempty"`
	RuntimeCheckpoints      []projectioncontract.RuntimeCheckpoint     `json:"runtimeCheckpoints,omitempty"`
	Results                 []projectioncontract.Result                `json:"results,omitempty"`
	Artifacts               []projectioncontract.Artifact              `json:"artifacts,omitempty"`
	ArtifactContinuations   []projectioncontract.ArtifactContinuation  `json:"artifactContinuations,omitempty"`
	Decisions               []projectioncontract.Decision              `json:"decisions,omitempty"`
	Approvals               []projectioncontract.Approval              `json:"approvals,omitempty"`
	MemoryRecallTraces      []projectioncontract.MemoryRecallTrace     `json:"memoryRecallTraces,omitempty"`
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

type WorkspaceRuntimeViewProjection struct {
	Service     string                                  `json:"service"`
	Backend     string                                  `json:"backend,omitempty"`
	DataRoot    string                                  `json:"dataRoot,omitempty"`
	Projection  ProjectionStatus                        `json:"projection"`
	RuntimeView projectioncontract.WorkspaceRuntimeView `json:"runtimeView"`
	Error       string                                  `json:"error,omitempty"`
}

type RunObservability struct {
	Run                     Run                                        `json:"run"`
	Outcome                 projectioncontract.RunOutcome              `json:"outcome"`
	TaskProgress            projectioncontract.TaskProgress            `json:"taskProgress"`
	RepairLoop              projectioncontract.RepairLoop              `json:"repairLoop"`
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
