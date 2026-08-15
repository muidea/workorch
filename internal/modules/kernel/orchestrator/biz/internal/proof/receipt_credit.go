package proof

import (
	"sort"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// CoverageReceiptFinding is the detached outcome of comparing one sealed
// operation and owner receipt with the activation-sealed coverage bindings.
// Runtime alone decides whether the finding is accepted as CWI credit.
type CoverageReceiptFinding struct {
	BindingRefs         []string
	CoverageWorkItemIDs []common.CoverageWorkItemID
	Diagnostic          string
}

// MatchCoverageReceipt performs the single exact binding/receipt comparison
// used by SM-2. It cannot mutate evidence, coverage, or the Runtime aggregate.
func MatchCoverageReceipt(facts runtimecontract.RuntimeFacts, operation runtimecontract.RuntimeOperation, evidenceKind string, receipt *runtimecontract.RuntimeOperationReceipt) CoverageReceiptFinding {
	operation = operation.Normalized()
	bindings := facts.Focus.Normalized().CoverageEvidenceBindings
	refs := append([]string(nil), operation.BindingRefs...)
	if len(bindings) == 0 {
		return CoverageReceiptFinding{BindingRefs: refs, Diagnostic: "coverage_binding_mismatch: focus has no sealed coverage evidence bindings"}
	}
	if len(refs) > 1 {
		return CoverageReceiptFinding{BindingRefs: refs, Diagnostic: "coverage_binding_mismatch: an operation may select exactly one coverage evidence binding"}
	}
	if receipt == nil {
		return CoverageReceiptFinding{BindingRefs: refs, Diagnostic: "coverage_binding_mismatch: missing persisted owner receipt"}
	}
	credit := make([]common.CoverageWorkItemID, 0, len(refs))
	diagnostics := make([]string, 0)
	for _, ref := range refs {
		binding, found := runtimecontract.CoverageEvidenceBindingForRef(bindings, ref)
		if !found {
			diagnostics = append(diagnostics, "unknown binding "+ref)
			continue
		}
		if _, requiresKind := runtimecontract.CoverageEvidenceProofForKind(binding, evidenceKind); !requiresKind {
			diagnostics = append(diagnostics, "binding "+ref+" does not require evidence kind "+evidenceKind)
			continue
		}
		if err := runtimecontract.MatchCoverageEvidenceBindingReceipt(binding, evidenceKind, operation, *receipt); err != nil {
			diagnostics = append(diagnostics, err.Error())
			continue
		}
		credit = append(credit, binding.CoverageWorkItemID)
	}
	credit = normalizeCoverageWorkItemIDs(credit)
	if len(diagnostics) != 0 {
		sort.Strings(diagnostics)
		return CoverageReceiptFinding{BindingRefs: refs, CoverageWorkItemIDs: credit, Diagnostic: "coverage_binding_mismatch: " + strings.Join(diagnostics, " | ")}
	}
	return CoverageReceiptFinding{BindingRefs: refs, CoverageWorkItemIDs: credit}
}

func normalizeCoverageWorkItemIDs(values []common.CoverageWorkItemID) []common.CoverageWorkItemID {
	seen := make(map[common.CoverageWorkItemID]struct{}, len(values))
	ret := make([]common.CoverageWorkItemID, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, found := seen[value]; found {
			continue
		}
		seen[value] = struct{}{}
		ret = append(ret, value)
	}
	return ret
}
