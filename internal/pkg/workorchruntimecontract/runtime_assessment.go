package workorchruntimecontract

import (
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

// AssessmentDisposition is the neutral assessment outcome. Terminal authority remains
// with the Orchestrator reducer; LLM candidates only describe facts.
type AssessmentDisposition string

const (
	AssessmentDispositionSatisfied          AssessmentDisposition = "satisfied"
	AssessmentDispositionRepairableGap      AssessmentDisposition = "repairable_gap"
	AssessmentDispositionGovernanceRequired AssessmentDisposition = "governance_required"
	AssessmentDispositionTerminalRejection  AssessmentDisposition = "terminal_rejection"
)

type RuntimeAssessmentItemFact struct {
	CoverageWorkItemID common.CoverageWorkItemID
	Satisfied          bool
	Reason             string
	EvidenceRefs       []string
	ArtifactRefs       []common.ArtifactID
}

type RuntimeAssessmentGap struct {
	CoverageWorkItemIDs  []common.CoverageWorkItemID
	MissingEvidenceKinds []string
	TargetRefs           []string
	EvidenceRefs         []string
	Reason               string
	FailureClass         FailureClass
	FailureCode          string
	FailureSignature     string
}

func (d AssessmentDisposition) Normalized() AssessmentDisposition {
	return AssessmentDisposition(strings.TrimSpace(string(d)))
}

func (d AssessmentDisposition) Validate() error {
	switch d.Normalized() {
	case AssessmentDispositionSatisfied, AssessmentDispositionRepairableGap, AssessmentDispositionGovernanceRequired, AssessmentDispositionTerminalRejection:
		return nil
	case "":
		return fmt.Errorf("assessment disposition is required")
	default:
		return fmt.Errorf("unknown assessment disposition %q", d)
	}
}

func (g RuntimeAssessmentGap) Normalized() RuntimeAssessmentGap {
	g.CoverageWorkItemIDs = normalizeCoverageWorkItemIDs(g.CoverageWorkItemIDs)
	g.MissingEvidenceKinds = normalizeStrings(g.MissingEvidenceKinds)
	g.TargetRefs = normalizeStrings(g.TargetRefs)
	g.EvidenceRefs = normalizeStrings(g.EvidenceRefs)
	g.Reason = strings.TrimSpace(g.Reason)
	g.FailureClass = FailureClass(strings.TrimSpace(string(g.FailureClass)))
	g.FailureCode = strings.TrimSpace(g.FailureCode)
	g.FailureSignature = strings.TrimSpace(g.FailureSignature)
	return g
}

func (g RuntimeAssessmentGap) Validate() error {
	g = g.Normalized()
	if len(g.CoverageWorkItemIDs) == 0 {
		return fmt.Errorf("assessment gap requires coverage work item ids")
	}
	if len(g.MissingEvidenceKinds) == 0 && len(g.TargetRefs) == 0 && g.Reason == "" {
		return fmt.Errorf("assessment gap requires missing evidence, target refs, or reason")
	}
	return nil
}

func (i RuntimeAssessmentItemFact) Normalized() RuntimeAssessmentItemFact {
	i.CoverageWorkItemID = common.CoverageWorkItemID(strings.TrimSpace(string(i.CoverageWorkItemID)))
	i.Reason = strings.TrimSpace(i.Reason)
	i.EvidenceRefs = normalizeStrings(i.EvidenceRefs)
	if len(i.ArtifactRefs) > 0 {
		refs := make([]common.ArtifactID, 0, len(i.ArtifactRefs))
		for _, ref := range i.ArtifactRefs {
			ref = common.ArtifactID(strings.TrimSpace(string(ref)))
			if ref != "" {
				refs = append(refs, ref)
			}
		}
		if len(refs) == 0 {
			i.ArtifactRefs = nil
		} else {
			i.ArtifactRefs = refs
		}
	}
	return i
}

// DeriveAssessmentDisposition maps legacy Satisfied-only assessments and validates
// disposition consistency with item facts / gap payloads.
func DeriveAssessmentDisposition(satisfied bool, disposition AssessmentDisposition, items []RuntimeAssessmentItemFact, gap *RuntimeAssessmentGap) (AssessmentDisposition, error) {
	disposition = disposition.Normalized()
	if disposition == "" {
		if satisfied {
			return AssessmentDispositionSatisfied, nil
		}
		// Active legacy unsatisfied assessments are repairable, not terminal.
		return AssessmentDispositionRepairableGap, nil
	}
	if err := disposition.Validate(); err != nil {
		return "", err
	}
	switch disposition {
	case AssessmentDispositionSatisfied:
		for _, item := range items {
			if !item.Satisfied {
				// Partial satisfied item set is normalized to repairable_gap.
				return AssessmentDispositionRepairableGap, nil
			}
		}
		return AssessmentDispositionSatisfied, nil
	case AssessmentDispositionRepairableGap, AssessmentDispositionGovernanceRequired, AssessmentDispositionTerminalRejection:
		if gap == nil {
			// allow empty gap only when item-level unsatisfied facts exist
			hasUnsatisfied := false
			for _, item := range items {
				if !item.Satisfied {
					hasUnsatisfied = true
					break
				}
			}
			if !hasUnsatisfied && disposition == AssessmentDispositionRepairableGap {
				return "", fmt.Errorf("repairable_gap assessment requires gap or unsatisfied items")
			}
		} else if err := gap.Normalized().Validate(); err != nil && disposition == AssessmentDispositionRepairableGap {
			return "", err
		}
		return disposition, nil
	default:
		return "", fmt.Errorf("unknown assessment disposition %q", disposition)
	}
}
