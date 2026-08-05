package governance

import "github.com/muidea/workorch/internal/pkg/common"

type AutoGovernanceTrace struct {
	ID                 common.AutoGovernanceTraceID
	WorkspaceID        common.WorkspaceID
	RunID              common.RunID
	StepID             common.StepID
	ItemType           string
	ItemID             string
	Action             string
	RiskLevel          string
	Confidence         float64
	PolicyMatched      string
	PolicyBlocked      string
	BudgetBefore       int
	BudgetAfter        int
	Reason             string
	EvidenceRefs       []string
	PromptFingerprint  string
	LLMProviderID      string
	LLMModel           string
	LLMDecisionPreview string
	Status             string
	OccurredAt         string
}

func CloneAutoGovernanceTrace(item *AutoGovernanceTrace) *AutoGovernanceTrace {
	if item == nil {
		return nil
	}
	cloned := *item
	cloned.EvidenceRefs = append([]string(nil), item.EvidenceRefs...)
	return &cloned
}

func CloneAutoGovernanceTraces(items []*AutoGovernanceTrace) []*AutoGovernanceTrace {
	if items == nil {
		return nil
	}
	ret := make([]*AutoGovernanceTrace, 0, len(items))
	for _, item := range items {
		ret = append(ret, CloneAutoGovernanceTrace(item))
	}
	return ret
}
