package workorchruntimecontract

import (
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

// RuntimeRunStateChangeRequirement is the Run-level state-change policy sealed at activation.
// It does not force every DoD focus to mutate workspace; completion gate checks the ledger.
type RuntimeRunStateChangeRequirement struct {
	Required            bool
	CoverageWorkItemIDs []common.CoverageWorkItemID
}

// RuntimeStateChangeLedger records whether the Run has produced coverage-bound primary
// state-change evidence that satisfies the sealed Run-level requirement.
type RuntimeStateChangeLedger struct {
	Required             bool
	Satisfied            bool
	CoverageWorkItemIDs  []common.CoverageWorkItemID
	EvidenceRefs         []string
	LastEvidenceRevision string
}

func (r RuntimeRunStateChangeRequirement) Normalized() RuntimeRunStateChangeRequirement {
	r.CoverageWorkItemIDs = normalizeCoverageWorkItemIDs(r.CoverageWorkItemIDs)
	return r
}

func (l RuntimeStateChangeLedger) Normalized() RuntimeStateChangeLedger {
	l.CoverageWorkItemIDs = normalizeCoverageWorkItemIDs(l.CoverageWorkItemIDs)
	l.EvidenceRefs = normalizeStrings(l.EvidenceRefs)
	l.LastEvidenceRevision = strings.TrimSpace(l.LastEvidenceRevision)
	if !l.Required {
		l.Satisfied = true
	}
	return l
}

func (l RuntimeStateChangeLedger) SatisfiesRequirement() bool {
	l = l.Normalized()
	return !l.Required || l.Satisfied
}

// ApplyAcceptedStateChangeEvidence updates the Run-level ledger when accepted state-change
// evidence intersects the required coverage set (or any coverage when the required set is empty).
// Predecessor/unbound evidence must not satisfy the ledger.
func (l RuntimeStateChangeLedger) ApplyAcceptedStateChangeEvidence(evidence EvidenceFact, currentCoverage []common.CoverageWorkItemID) RuntimeStateChangeLedger {
	l = l.Normalized()
	if !l.Required || l.Satisfied {
		return l
	}
	if evidence.Kind != EvidenceKindStateChange {
		return l
	}
	evidenceCoverage := normalizeCoverageWorkItemIDs(evidence.CoverageWorkItemIDs)
	if len(evidenceCoverage) == 0 {
		// unbound evidence cannot satisfy Run-level state-change
		return l
	}
	required := l.CoverageWorkItemIDs
	if len(required) == 0 {
		required = normalizeCoverageWorkItemIDs(currentCoverage)
	}
	if len(required) == 0 {
		// no coverage identity available: still accept any bound state-change as progress
		l.Satisfied = true
		l.CoverageWorkItemIDs = evidenceCoverage
		l.EvidenceRefs = stableAppendUnique(l.EvidenceRefs, string(evidence.ID))
		l.LastEvidenceRevision = strings.TrimSpace(evidence.Revision)
		return l
	}
	if !coverageIDsIntersect(coverageIDSet(required), evidenceCoverage) {
		return l
	}
	l.Satisfied = true
	l.EvidenceRefs = stableAppendUnique(l.EvidenceRefs, string(evidence.ID))
	if strings.TrimSpace(evidence.Revision) != "" {
		l.LastEvidenceRevision = strings.TrimSpace(evidence.Revision)
	}
	// retain required coverage identity; do not expand with unrelated IDs
	if len(l.CoverageWorkItemIDs) == 0 {
		l.CoverageWorkItemIDs = required
	}
	return l
}

func stableAppendUnique(values []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(append([]string(nil), values...), value)
}

func NewRuntimeStateChangeLedger(requirement RuntimeRunStateChangeRequirement) RuntimeStateChangeLedger {
	requirement = requirement.Normalized()
	return RuntimeStateChangeLedger{
		Required:            requirement.Required,
		Satisfied:           !requirement.Required,
		CoverageWorkItemIDs: append([]common.CoverageWorkItemID(nil), requirement.CoverageWorkItemIDs...),
	}.Normalized()
}
