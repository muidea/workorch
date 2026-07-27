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

func NewRuntimeStateChangeLedger(requirement RuntimeRunStateChangeRequirement) RuntimeStateChangeLedger {
	requirement = requirement.Normalized()
	return RuntimeStateChangeLedger{
		Required:            requirement.Required,
		Satisfied:           !requirement.Required,
		CoverageWorkItemIDs: append([]common.CoverageWorkItemID(nil), requirement.CoverageWorkItemIDs...),
	}.Normalized()
}
