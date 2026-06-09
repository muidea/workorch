package readmodelcore

import "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/presenter"

type WorkspaceSnapshot struct {
	Workspace             Workspace               `json:"workspace"`
	Run                   *Run                    `json:"run,omitempty"`
	Step                  *presenter.StepView     `json:"step,omitempty"`
	Result                *Result                 `json:"result,omitempty"`
	Outcome               WorkspaceOutcome        `json:"outcome"`
	TaskProgress          TaskProgress            `json:"taskProgress"`
	ContextBundle         *ContextBundle          `json:"contextBundle,omitempty"`
	CoverageSummary       map[string]any          `json:"coverageSummary,omitempty"`
	TaskDecompositions    []TaskDecomposition     `json:"taskDecompositions,omitempty"`
	ExecutionSections     []ExecutionSection      `json:"executionSections,omitempty"`
	MemoryRecallTraces    []MemoryRecallTrace     `json:"memoryRecallTraces,omitempty"`
	Goals                 []Goal                  `json:"goals,omitempty"`
	DoDs                  []DoD                   `json:"dods,omitempty"`
	Runs                  []Run                   `json:"runs,omitempty"`
	Results               []Result                `json:"results,omitempty"`
	Artifacts             []Artifact              `json:"artifacts,omitempty"`
	ArtifactContinuations []ArtifactContinuation  `json:"artifactContinuations,omitempty"`
	Patches               []Patch                 `json:"patches,omitempty"`
	WebSources            []WebSourceTrace        `json:"webSources,omitempty"`
	LLMContextBudgets     []LLMContextBudgetTrace `json:"llmContextBudgets,omitempty"`
	LLMUsageTraces        []LLMUsageTrace         `json:"llmUsageTraces,omitempty"`
	LLMUsageSummary       LLMUsageSummary         `json:"llmUsageSummary"`
	EventRecords          []EventRecord           `json:"eventRecords,omitempty"`
	Decisions             []Decision              `json:"decisions,omitempty"`
	Approvals             []Approval              `json:"approvals,omitempty"`
}

type WorkspaceExecutionActivation struct {
	Workspace Workspace         `json:"workspace"`
	Run       Run               `json:"run"`
	Snapshot  WorkspaceSnapshot `json:"snapshot"`
}
