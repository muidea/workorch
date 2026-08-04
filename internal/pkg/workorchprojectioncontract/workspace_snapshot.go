package workorchprojectioncontract

import (
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

type WorkspaceSnapshot struct {
	Workspace               Workspace                         `json:"workspace"`
	ViewModel               WorkspaceViewModel                `json:"viewModel"`
	Run                     *Run                              `json:"run,omitempty"`
	Step                    *StepView                         `json:"step,omitempty"`
	Result                  *Result                           `json:"result,omitempty"`
	Outcome                 WorkspaceOutcome                  `json:"outcome"`
	TaskProgress            TaskProgress                      `json:"taskProgress"`
	RunHeartbeat            RunHeartbeat                      `json:"runHeartbeat,omitempty"`
	ConvergenceStatus       *ConvergenceStatus                `json:"convergenceStatus,omitempty"`
	RunCommand              *RunCommandStatus                 `json:"runCommand,omitempty"`
	SchedulerDiagnostics    *SchedulerDiagnostics             `json:"schedulerDiagnostics,omitempty"`
	ContextBundle           *ContextBundle                    `json:"contextBundle,omitempty"`
	CoverageSummary         map[string]any                    `json:"coverageSummary,omitempty"`
	GapBaseline             *GapBaseline                      `json:"gapBaseline,omitempty"`
	AuthorityDiagnostics    []GapDiagnostic                   `json:"authorityDiagnostics,omitempty"`
	Acceptance              AcceptanceProjection              `json:"acceptance"`
	TaskDecompositions      []TaskDecomposition               `json:"taskDecompositions,omitempty"`
	TaskPlans               []TaskPlan                        `json:"taskPlans,omitempty"`
	ExecutionSections       []ExecutionSection                `json:"executionSections,omitempty"`
	MemoryRecallTraces      []MemoryRecallTrace               `json:"memoryRecallTraces,omitempty"`
	Goals                   []workorchmanagementcontract.Goal `json:"goals,omitempty"`
	DoDs                    []workorchmanagementcontract.DoD  `json:"dods,omitempty"`
	Runs                    []Run                             `json:"runs,omitempty"`
	Results                 []Result                          `json:"results,omitempty"`
	Artifacts               []Artifact                        `json:"artifacts,omitempty"`
	ArtifactContinuations   []ArtifactContinuation            `json:"artifactContinuations,omitempty"`
	Patches                 []Patch                           `json:"patches,omitempty"`
	WebSources              []WebSourceTrace                  `json:"webSources,omitempty"`
	LLMContextBudgets       []LLMContextBudgetTrace           `json:"llmContextBudgets,omitempty"`
	LLMContextBudgetSummary LLMContextBudgetSummary           `json:"llmContextBudgetSummary"`
	LLMUsageTraces          []LLMUsageTrace                   `json:"llmUsageTraces,omitempty"`
	LLMUsageSummary         LLMUsageSummary                   `json:"llmUsageSummary"`
	EventRecords            []EventRecord                     `json:"eventRecords,omitempty"`
	Decisions               []Decision                        `json:"decisions,omitempty"`
	Approvals               []Approval                        `json:"approvals,omitempty"`
}

type WorkspaceExecutionActivation struct {
	Workspace Workspace         `json:"workspace"`
	Run       Run               `json:"run"`
	Snapshot  WorkspaceSnapshot `json:"snapshot"`
}
