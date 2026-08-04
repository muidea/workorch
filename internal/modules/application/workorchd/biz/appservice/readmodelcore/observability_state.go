package readmodelcore

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

type RunObservabilityState struct {
	Run                     appmodelcore.Run                           `json:"run"`
	Outcome                 projectioncontract.RunOutcome              `json:"outcome"`
	Progress                projectioncontract.RunProgress             `json:"progress"`
	TaskProgress            projectioncontract.TaskProgress            `json:"taskProgress"`
	RepairLoop              projectioncontract.RepairLoop              `json:"repairLoop"`
	Acceptance              projectioncontract.AcceptanceProjection    `json:"acceptance"`
	TaskDecompositions      []projectioncontract.TaskDecomposition     `json:"taskDecompositions,omitempty"`
	TaskPlans               []projectioncontract.TaskPlan              `json:"taskPlans,omitempty"`
	ExecutionSections       []projectioncontract.ExecutionSection      `json:"executionSections,omitempty"`
	LLMContextBudgets       []projectioncontract.LLMContextBudgetTrace `json:"llmContextBudgets,omitempty"`
	LLMContextBudgetSummary projectioncontract.LLMContextBudgetSummary `json:"llmContextBudgetSummary"`
	LLMUsageTraces          []projectioncontract.LLMUsageTrace         `json:"llmUsageTraces,omitempty"`
	LLMUsageSummary         projectioncontract.LLMUsageSummary         `json:"llmUsageSummary"`
	MemoryRecallTraces      []projectioncontract.MemoryRecallTrace     `json:"memoryRecallTraces,omitempty"`
	GitOperations           []projectioncontract.GitOperationTrace     `json:"gitOperations,omitempty"`
	WebSources              []projectioncontract.WebSourceTrace        `json:"webSources,omitempty"`
	ClosureTraces           []projectioncontract.ClosureTrace          `json:"closureTraces,omitempty"`
	DoDEvidence             []projectioncontract.DoDEvidenceTrace      `json:"doDEvidence,omitempty"`
	ResultQuality           []projectioncontract.ResultQualityTrace    `json:"resultQuality,omitempty"`
	Evidence                projectioncontract.RuntimeEvidenceSummary  `json:"evidence"`
	LatestCheckpoint        *projectioncontract.RuntimeCheckpoint      `json:"latestCheckpoint,omitempty"`
	SelectedCapabilities    []projectioncontract.RuntimeCapability     `json:"selectedCapabilities,omitempty"`
	UsedCapabilities        []projectioncontract.RuntimeCapability     `json:"usedCapabilities,omitempty"`
	Timeline                []projectioncontract.RuntimeTimelineItem   `json:"timeline,omitempty"`
}
