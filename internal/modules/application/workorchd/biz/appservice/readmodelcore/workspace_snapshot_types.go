package readmodelcore

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

type WorkspaceSnapshot struct {
	Workspace               appmodelcore.Workspace                     `json:"workspace"`
	ViewModel               appmodelcore.WorkspaceViewModel            `json:"viewModel"`
	Run                     *appmodelcore.Run                          `json:"run,omitempty"`
	Step                    *StepView                                  `json:"step,omitempty"`
	Result                  *Result                                    `json:"result,omitempty"`
	Outcome                 WorkspaceOutcome                           `json:"outcome"`
	TaskProgress            TaskProgress                               `json:"taskProgress"`
	RunHeartbeat            projectioncontract.RunHeartbeat            `json:"runHeartbeat,omitempty"`
	ConvergenceStatus       *projectioncontract.ConvergenceStatus      `json:"convergenceStatus,omitempty"`
	RunCommand              *RunCommandStatus                          `json:"runCommand,omitempty"`
	SchedulerDiagnostics    *projectioncontract.SchedulerDiagnostics   `json:"schedulerDiagnostics,omitempty"`
	ContextBundle           *ContextBundle                             `json:"contextBundle,omitempty"`
	CoverageSummary         map[string]any                             `json:"coverageSummary,omitempty"`
	GapBaseline             *projectioncontract.GapBaseline            `json:"gapBaseline,omitempty"`
	AuthorityDiagnostics    []projectioncontract.GapDiagnostic         `json:"authorityDiagnostics,omitempty"`
	Acceptance              projectioncontract.AcceptanceProjection    `json:"acceptance"`
	TaskDecompositions      []TaskDecomposition                        `json:"taskDecompositions,omitempty"`
	TaskPlans               []TaskPlan                                 `json:"taskPlans,omitempty"`
	ExecutionSections       []ExecutionSection                         `json:"executionSections,omitempty"`
	MemoryRecallTraces      []MemoryRecallTrace                        `json:"memoryRecallTraces,omitempty"`
	Goals                   []workorchmanagementcontract.Goal          `json:"goals,omitempty"`
	DoDs                    []workorchmanagementcontract.DoD           `json:"dods,omitempty"`
	Runs                    []appmodelcore.Run                         `json:"runs,omitempty"`
	Results                 []Result                                   `json:"results,omitempty"`
	Artifacts               []projectioncontract.Artifact              `json:"artifacts,omitempty"`
	ArtifactContinuations   []projectioncontract.ArtifactContinuation  `json:"artifactContinuations,omitempty"`
	Patches                 []projectioncontract.Patch                 `json:"patches,omitempty"`
	WebSources              []WebSourceTrace                           `json:"webSources,omitempty"`
	LLMContextBudgets       []projectioncontract.LLMContextBudgetTrace `json:"llmContextBudgets,omitempty"`
	LLMContextBudgetSummary projectioncontract.LLMContextBudgetSummary `json:"llmContextBudgetSummary"`
	LLMUsageTraces          []projectioncontract.LLMUsageTrace         `json:"llmUsageTraces,omitempty"`
	LLMUsageSummary         projectioncontract.LLMUsageSummary         `json:"llmUsageSummary"`
	EventRecords            []projectioncontract.EventRecord           `json:"eventRecords,omitempty"`
	Decisions               []appmodelcore.Decision                    `json:"decisions,omitempty"`
	Approvals               []appmodelcore.Approval                    `json:"approvals,omitempty"`
}

type RunCommandStatus struct {
	WorkspaceID        string `json:"workspaceId,omitempty"`
	RunID              string `json:"runId,omitempty"`
	State              string `json:"state,omitempty"`
	Intent             string `json:"intent,omitempty"`
	Reason             string `json:"reason,omitempty"`
	Disposition        string `json:"disposition,omitempty"`
	Diagnostic         string `json:"diagnostic,omitempty"`
	RequestID          string `json:"requestId,omitempty"`
	IdempotencyKey     string `json:"idempotencyKey,omitempty"`
	Source             string `json:"source,omitempty"`
	DefinitionRevision string `json:"definitionRevision,omitempty"`
	DecisionID         string `json:"decisionId,omitempty"`
	QueuedAt           string `json:"queuedAt,omitempty"`
	AcceptedAt         string `json:"acceptedAt,omitempty"`
	RejectedAt         string `json:"rejectedAt,omitempty"`
	Error              string `json:"error,omitempty"`
	Revision           int64  `json:"revision,omitempty"`
}

type WorkspaceExecutionActivation struct {
	Workspace appmodelcore.Workspace `json:"workspace"`
	Run       appmodelcore.Run       `json:"run"`
	Snapshot  WorkspaceSnapshot      `json:"snapshot"`
}
