package readmodelcore

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

type RunObservability struct {
	Run                     appmodelcore.Run                           `json:"run"`
	Outcome                 projectioncontract.RunOutcome              `json:"outcome"`
	Progress                RunProgress                                `json:"progress"`
	TaskProgress            projectioncontract.TaskProgress            `json:"taskProgress"`
	RepairLoop              projectioncontract.RepairLoop              `json:"repairLoop"`
	Acceptance              projectioncontract.AcceptanceProjection    `json:"acceptance"`
	TaskDecompositions      []projectioncontract.TaskDecomposition     `json:"taskDecompositions,omitempty"`
	TaskPlans               []projectioncontract.TaskPlan              `json:"taskPlans,omitempty"`
	ExecutionSections       []ExecutionSection                         `json:"executionSections,omitempty"`
	LLMContextBudgets       []projectioncontract.LLMContextBudgetTrace `json:"llmContextBudgets,omitempty"`
	LLMContextBudgetSummary projectioncontract.LLMContextBudgetSummary `json:"llmContextBudgetSummary"`
	LLMUsageTraces          []projectioncontract.LLMUsageTrace         `json:"llmUsageTraces,omitempty"`
	LLMUsageSummary         projectioncontract.LLMUsageSummary         `json:"llmUsageSummary"`
	MemoryRecallTraces      []projectioncontract.MemoryRecallTrace     `json:"memoryRecallTraces,omitempty"`
	GitOperations           []GitOperationTrace                        `json:"gitOperations,omitempty"`
	WebSources              []WebSourceTrace                           `json:"webSources,omitempty"`
	ClosureTraces           []ClosureTrace                             `json:"closureTraces,omitempty"`
	DoDEvidence             []DoDEvidenceTrace                         `json:"doDEvidence,omitempty"`
	ResultQuality           []ResultQualityTrace                       `json:"resultQuality,omitempty"`
	Evidence                RuntimeEvidenceSummary                     `json:"evidence"`
	LatestCheckpoint        *projectioncontract.RuntimeCheckpoint      `json:"latestCheckpoint,omitempty"`
	SelectedCapabilities    []RuntimeCapability                        `json:"selectedCapabilities,omitempty"`
	UsedCapabilities        []RuntimeCapability                        `json:"usedCapabilities,omitempty"`
	Timeline                []RuntimeTimelineItem                      `json:"timeline,omitempty"`
}
