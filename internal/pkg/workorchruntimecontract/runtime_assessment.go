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
	CoverageWorkItemIDs []common.CoverageWorkItemID
	// MissingEvidenceKinds and TargetRefs preserve the assessment producer's
	// safe observation. They are never canonical repair-routing input.
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

// RuntimeAssessmentGapStrictlyNarrows is a pure comparison of two persisted
// assessment gaps. Coverage is the only observation-independent convergence
// dimension: missing-kind and target claims are diagnostic lineage, not
// repair-budget or action authority.
//
// The Orchestrator remains the sole owner that decides whether this comparison
// authorizes a repair-ledger transition.
func RuntimeAssessmentGapStrictlyNarrows(previous, current RuntimeAssessmentGap) bool {
	previous = previous.Normalized()
	current = current.Normalized()
	coverageNarrowed, coverageExpanded := normalizedCoverageSubsetDelta(previous.CoverageWorkItemIDs, current.CoverageWorkItemIDs)
	return !coverageExpanded && coverageNarrowed
}

func normalizedCoverageSubsetDelta(previous, current []common.CoverageWorkItemID) (narrowed, expanded bool) {
	prior := make(map[common.CoverageWorkItemID]struct{}, len(previous))
	for _, id := range previous {
		prior[id] = struct{}{}
	}
	next := make(map[common.CoverageWorkItemID]struct{}, len(current))
	for _, id := range current {
		next[id] = struct{}{}
		if _, found := prior[id]; !found {
			expanded = true
		}
	}
	for id := range prior {
		if _, found := next[id]; !found {
			narrowed = true
		}
	}
	return narrowed, expanded
}

func normalizedStringSubsetDelta(previous, current []string) (narrowed, expanded bool) {
	prior := make(map[string]struct{}, len(previous))
	for _, value := range previous {
		prior[value] = struct{}{}
	}
	next := make(map[string]struct{}, len(current))
	for _, value := range current {
		next[value] = struct{}{}
		if _, found := prior[value]; !found {
			expanded = true
		}
	}
	for value := range prior {
		if _, found := next[value]; !found {
			narrowed = true
		}
	}
	return narrowed, expanded
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

// ValidateAssessmentDisposition validates a producer observation enum without
// reclassifying it from items or gap contents. The returned value is safe to
// persist as the original claimed disposition; the Orchestrator separately
// derives canonical coverage and repair authority from owner facts.
func ValidateAssessmentDisposition(disposition AssessmentDisposition, items []RuntimeAssessmentItemFact, gap *RuntimeAssessmentGap) (AssessmentDisposition, error) {
	disposition = disposition.Normalized()
	if disposition == "" {
		return "", fmt.Errorf("assessment disposition is required")
	}
	if err := disposition.Validate(); err != nil {
		return "", err
	}
	_ = items
	_ = gap
	return disposition, nil
}
