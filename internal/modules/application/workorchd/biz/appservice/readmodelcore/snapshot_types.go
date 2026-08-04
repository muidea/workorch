package readmodelcore

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

type RunSnapshot struct {
	Workspace               appmodelcore.Workspace                     `json:"workspace"`
	Run                     appmodelcore.Run                           `json:"run"`
	Outcome                 RunOutcome                                 `json:"outcome"`
	TaskProgress            TaskProgress                               `json:"taskProgress"`
	RepairLoop              RepairLoop                                 `json:"repairLoop"`
	TaskDecompositions      []TaskDecomposition                        `json:"taskDecompositions,omitempty"`
	TaskPlans               []TaskPlan                                 `json:"taskPlans,omitempty"`
	Step                    *StepView                                  `json:"step,omitempty"`
	Steps                   []StepView                                 `json:"steps,omitempty"`
	ToolCalls               []projectioncontract.ToolCall              `json:"toolCalls,omitempty"`
	Patches                 []projectioncontract.Patch                 `json:"patches,omitempty"`
	GitOperations           []GitOperationTrace                        `json:"gitOperations,omitempty"`
	WebSources              []WebSourceTrace                           `json:"webSources,omitempty"`
	ValidationResults       []projectioncontract.ValidationResult      `json:"validationResults,omitempty"`
	RuntimeCheckpoints      []projectioncontract.RuntimeCheckpoint     `json:"runtimeCheckpoints,omitempty"`
	ConvergenceStatus       *projectioncontract.ConvergenceStatus      `json:"convergenceStatus,omitempty"`
	SchedulerDiagnostics    *projectioncontract.SchedulerDiagnostics   `json:"schedulerDiagnostics,omitempty"`
	Results                 []Result                                   `json:"results,omitempty"`
	ClosureTraces           []ClosureTrace                             `json:"closureTraces,omitempty"`
	DoDEvidence             []DoDEvidenceTrace                         `json:"doDEvidence,omitempty"`
	ResultQuality           []ResultQualityTrace                       `json:"resultQuality,omitempty"`
	Artifacts               []projectioncontract.Artifact              `json:"artifacts,omitempty"`
	ArtifactContinuations   []projectioncontract.ArtifactContinuation  `json:"artifactContinuations,omitempty"`
	ExecutionSections       []ExecutionSection                         `json:"executionSections,omitempty"`
	LLMContextBudgets       []projectioncontract.LLMContextBudgetTrace `json:"llmContextBudgets,omitempty"`
	LLMContextBudgetSummary projectioncontract.LLMContextBudgetSummary `json:"llmContextBudgetSummary"`
	LLMUsageTraces          []projectioncontract.LLMUsageTrace         `json:"llmUsageTraces,omitempty"`
	LLMUsageSummary         projectioncontract.LLMUsageSummary         `json:"llmUsageSummary"`
	CoverageWorkItems       []map[string]any                           `json:"coverageWorkItems,omitempty"`
	CoverageWorkItemContext map[string]any                             `json:"coverageWorkItemContext,omitempty"`
	GapBaseline             *projectioncontract.GapBaseline            `json:"gapBaseline,omitempty"`
	AuthorityDiagnostics    []projectioncontract.GapDiagnostic         `json:"authorityDiagnostics,omitempty"`
	Acceptance              projectioncontract.AcceptanceProjection    `json:"acceptance"`
	MemoryRecallTraces      []MemoryRecallTrace                        `json:"memoryRecallTraces,omitempty"`
	SelectedCapabilities    []RuntimeCapability                        `json:"selectedCapabilities,omitempty"`
	UsedCapabilities        []RuntimeCapability                        `json:"usedCapabilities,omitempty"`
	EventRecords            []projectioncontract.EventRecord           `json:"eventRecords,omitempty"`
	Decisions               []appmodelcore.Decision                    `json:"decisions,omitempty"`
	Approvals               []appmodelcore.Approval                    `json:"approvals,omitempty"`
	Observed                []ObservedView                             `json:"observed,omitempty"`
}
