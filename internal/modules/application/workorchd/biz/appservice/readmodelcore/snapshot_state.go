package readmodelcore

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

// RunSnapshotState is application-private projection input. It may retain the
// private RuntimeState carried by appmodelcore.Run and must not cross a
// protocol boundary directly.
type RunSnapshotState struct {
	Workspace               appmodelcore.Workspace                     `json:"workspace"`
	Run                     appmodelcore.Run                           `json:"run"`
	Outcome                 projectioncontract.RunOutcome              `json:"outcome"`
	TaskProgress            projectioncontract.TaskProgress            `json:"taskProgress"`
	RepairLoop              projectioncontract.RepairLoop              `json:"repairLoop"`
	TaskDecompositions      []projectioncontract.TaskDecomposition     `json:"taskDecompositions,omitempty"`
	TaskPlans               []projectioncontract.TaskPlan              `json:"taskPlans,omitempty"`
	Step                    *projectioncontract.StepView               `json:"step,omitempty"`
	Steps                   []projectioncontract.StepView              `json:"steps,omitempty"`
	ToolCalls               []projectioncontract.ToolCall              `json:"toolCalls,omitempty"`
	Patches                 []projectioncontract.Patch                 `json:"patches,omitempty"`
	GitOperations           []projectioncontract.GitOperationTrace     `json:"gitOperations,omitempty"`
	WebSources              []projectioncontract.WebSourceTrace        `json:"webSources,omitempty"`
	ValidationResults       []projectioncontract.ValidationResult      `json:"validationResults,omitempty"`
	RuntimeCheckpoints      []projectioncontract.RuntimeCheckpoint     `json:"runtimeCheckpoints,omitempty"`
	ConvergenceStatus       *projectioncontract.ConvergenceStatus      `json:"convergenceStatus,omitempty"`
	SchedulerDiagnostics    *projectioncontract.SchedulerDiagnostics   `json:"schedulerDiagnostics,omitempty"`
	Results                 []projectioncontract.Result                `json:"results,omitempty"`
	ClosureTraces           []projectioncontract.ClosureTrace          `json:"closureTraces,omitempty"`
	DoDEvidence             []projectioncontract.DoDEvidenceTrace      `json:"doDEvidence,omitempty"`
	ResultQuality           []projectioncontract.ResultQualityTrace    `json:"resultQuality,omitempty"`
	Artifacts               []projectioncontract.Artifact              `json:"artifacts,omitempty"`
	ArtifactContinuations   []projectioncontract.ArtifactContinuation  `json:"artifactContinuations,omitempty"`
	ExecutionSections       []projectioncontract.ExecutionSection      `json:"executionSections,omitempty"`
	LLMContextBudgets       []projectioncontract.LLMContextBudgetTrace `json:"llmContextBudgets,omitempty"`
	LLMContextBudgetSummary projectioncontract.LLMContextBudgetSummary `json:"llmContextBudgetSummary"`
	LLMUsageTraces          []projectioncontract.LLMUsageTrace         `json:"llmUsageTraces,omitempty"`
	LLMUsageSummary         projectioncontract.LLMUsageSummary         `json:"llmUsageSummary"`
	CoverageWorkItems       []map[string]any                           `json:"coverageWorkItems,omitempty"`
	CoverageWorkItemContext map[string]any                             `json:"coverageWorkItemContext,omitempty"`
	GapBaseline             *projectioncontract.GapBaseline            `json:"gapBaseline,omitempty"`
	AuthorityDiagnostics    []projectioncontract.GapDiagnostic         `json:"authorityDiagnostics,omitempty"`
	Acceptance              projectioncontract.AcceptanceProjection    `json:"acceptance"`
	MemoryRecallTraces      []projectioncontract.MemoryRecallTrace     `json:"memoryRecallTraces,omitempty"`
	SelectedCapabilities    []projectioncontract.RuntimeCapability     `json:"selectedCapabilities,omitempty"`
	UsedCapabilities        []projectioncontract.RuntimeCapability     `json:"usedCapabilities,omitempty"`
	EventRecords            []projectioncontract.EventRecord           `json:"eventRecords,omitempty"`
	Decisions               []projectioncontract.Decision              `json:"decisions,omitempty"`
	Approvals               []projectioncontract.Approval              `json:"approvals,omitempty"`
	Observed                []projectioncontract.ObservedView          `json:"observed,omitempty"`
}
