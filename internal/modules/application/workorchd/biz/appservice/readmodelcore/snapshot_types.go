package readmodelcore

import "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/presenter"

type RunSnapshot struct {
	Workspace               Workspace                `json:"workspace"`
	Run                     Run                      `json:"run"`
	Outcome                 RunOutcome               `json:"outcome"`
	TaskProgress            TaskProgress             `json:"taskProgress"`
	RepairLoop              RepairLoop               `json:"repairLoop"`
	TaskDecompositions      []TaskDecomposition      `json:"taskDecompositions,omitempty"`
	TaskPlans               []TaskPlan               `json:"taskPlans,omitempty"`
	Step                    *presenter.StepView      `json:"step,omitempty"`
	Steps                   []presenter.StepView     `json:"steps,omitempty"`
	ToolCalls               []ToolCall               `json:"toolCalls,omitempty"`
	Patches                 []Patch                  `json:"patches,omitempty"`
	GitOperations           []GitOperationTrace      `json:"gitOperations,omitempty"`
	WebSources              []WebSourceTrace         `json:"webSources,omitempty"`
	ValidationResults       []ValidationResult       `json:"validationResults,omitempty"`
	RuntimeCheckpoints      []RuntimeCheckpoint      `json:"runtimeCheckpoints,omitempty"`
	RuntimeExecution        *RuntimeExecutionState   `json:"runtimeExecution,omitempty"`
	SchedulerDiagnostics    *SchedulerDiagnostics    `json:"schedulerDiagnostics,omitempty"`
	Results                 []Result                 `json:"results,omitempty"`
	ClosureTraces           []ClosureTrace           `json:"closureTraces,omitempty"`
	DoDEvidence             []DoDEvidenceTrace       `json:"doDEvidence,omitempty"`
	ResultQuality           []ResultQualityTrace     `json:"resultQuality,omitempty"`
	Artifacts               []Artifact               `json:"artifacts,omitempty"`
	ArtifactContinuations   []ArtifactContinuation   `json:"artifactContinuations,omitempty"`
	ExecutionSections       []ExecutionSection       `json:"executionSections,omitempty"`
	LLMContextBudgets       []LLMContextBudgetTrace  `json:"llmContextBudgets,omitempty"`
	LLMUsageTraces          []LLMUsageTrace          `json:"llmUsageTraces,omitempty"`
	LLMUsageSummary         LLMUsageSummary          `json:"llmUsageSummary"`
	CoverageWorkItems       []map[string]any         `json:"coverageWorkItems,omitempty"`
	CoverageWorkItemContext map[string]any           `json:"coverageWorkItemContext,omitempty"`
	Acceptance              AcceptanceProjection     `json:"acceptance"`
	MemoryRecallTraces      []MemoryRecallTrace      `json:"memoryRecallTraces,omitempty"`
	SelectedCapabilities    []RuntimeCapability      `json:"selectedCapabilities,omitempty"`
	UsedCapabilities        []RuntimeCapability      `json:"usedCapabilities,omitempty"`
	EventRecords            []EventRecord            `json:"eventRecords,omitempty"`
	Decisions               []Decision               `json:"decisions,omitempty"`
	Approvals               []Approval               `json:"approvals,omitempty"`
	Observed                []presenter.ObservedView `json:"observed,omitempty"`
}
