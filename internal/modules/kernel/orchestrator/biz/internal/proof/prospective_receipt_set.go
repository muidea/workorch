package proof

import (
	"fmt"
	"sort"
	"strings"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// ProspectiveCoverageReceiptMember is one detached operation/execution/receipt
// tuple from a single sealed dispatch. It contains facts only; the caller
// remains responsible for choosing any SM-2 transition.
type ProspectiveCoverageReceiptMember struct {
	DispatchID       string
	Operation        runtimecontract.RuntimeOperation
	Execution        runtimecontract.RuntimeOperationExecutionRecord
	Receipt          runtimecontract.RuntimeOperationReceipt
	AcceptedEvidence *runtimecontract.EvidenceFact
}

// ProspectiveCoverageReceiptSetFinding reports whether one sealed dispatch has
// owner-committed, accepted, non-weak exact proof for every target in a
// prospective all-of binding. Missing/invalid members remain factual detail and
// never select retry, repair, settlement, or terminal state here.
type ProspectiveCoverageReceiptSetFinding struct {
	RequiredTargetRefs []string
	AcceptedTargetRefs []string
	MissingTargetRefs  []string
	Diagnostic         string
}

func (f ProspectiveCoverageReceiptSetFinding) Complete() bool {
	return len(f.RequiredTargetRefs) > 1 && len(f.MissingTargetRefs) == 0 && strings.TrimSpace(f.Diagnostic) == ""
}

// MatchProspectiveCoverageReceiptSet is the collective counterpart of
// MatchCoverageReceipt. A multi-target prospective binding is an all-of proof:
// one successful receipt proves only its exact target and cannot credit or
// settle the CWI until the same sealed dispatch has a matching receipt for
// every target.
func MatchProspectiveCoverageReceiptSet(binding runtimecontract.CoverageEvidenceBinding, evidenceKind string, members []ProspectiveCoverageReceiptMember) ProspectiveCoverageReceiptSetFinding {
	binding = binding.Normalized()
	evidenceKind = strings.TrimSpace(evidenceKind)
	required := append([]string(nil), binding.ProspectiveTargetRefs...)
	sort.Strings(required)
	finding := ProspectiveCoverageReceiptSetFinding{RequiredTargetRefs: required}
	if len(required) <= 1 {
		return finding
	}
	if err := binding.Validate(); err != nil {
		finding.Diagnostic = "prospective coverage binding is invalid: " + err.Error()
		finding.MissingTargetRefs = required
		return finding
	}
	if _, found := runtimecontract.CoverageEvidenceProofForKind(binding, evidenceKind); !found {
		finding.Diagnostic = fmt.Sprintf("prospective coverage binding %q does not require evidence kind %q", binding.ID, evidenceKind)
		finding.MissingTargetRefs = required
		return finding
	}

	accepted := make(map[string]string, len(required))
	conflicts := make([]string, 0)
	for _, raw := range members {
		dispatchID := strings.TrimSpace(raw.DispatchID)
		operation := raw.Operation.Normalized()
		execution := raw.Execution.Normalized()
		receipt := raw.Receipt.Normalized()
		evidence := raw.AcceptedEvidence
		if len(operation.BindingRefs) != 1 || operation.BindingRefs[0] != binding.ID {
			continue
		}
		// Coverage is an Orchestrator evidence transition. An owner_committed
		// receipt remains recoverable audit input, but cannot participate in the
		// collective gate until its evidence commit is in the same aggregate.
		if execution.State != runtimecontract.OperationExecutionEvidenceCommitted {
			continue
		}
		if execution.OperationID != operation.ID || receipt.OperationID != operation.ID ||
			execution.ExecutionKey == "" || execution.ExecutionKey != receipt.ExecutionKey ||
			execution.Fence == 0 || execution.Fence != receipt.Fence ||
			execution.OwnerReceiptRef == "" || execution.OwnerReceiptHash != runtimecontract.StableRuntimeOperationReceiptHash(receipt) {
			continue
		}
		if err := runtimecontract.MatchCoverageEvidenceBindingReceipt(binding, evidenceKind, operation, receipt); err != nil {
			continue
		}
		if evidence == nil || dispatchID == "" || strings.TrimSpace(evidence.DispatchID) != dispatchID ||
			strings.TrimSpace(evidence.OperationID) != operation.ID || strings.TrimSpace(evidence.Kind) != evidenceKind ||
			!CoverageWorkItemIncluded(evidence.CoverageWorkItemIDs, binding.CoverageWorkItemID) ||
			!EvidenceReferencesBinding(*evidence, binding.ID) || WeakEvidence(*evidence) ||
			!sameProspectiveReceiptTargetRefs(evidence.TargetRefs, operation.TargetRefs) {
			continue
		}
		// The binding matcher already enforces singleton membership. Retain the
		// explicit guard so this collective helper remains fail-closed if that
		// lower-level contract changes.
		if len(receipt.TargetRefs) != 1 {
			continue
		}
		targetRef := receipt.TargetRefs[0]
		receiptHash := runtimecontract.StableRuntimeOperationReceiptHash(receipt)
		if previous, duplicate := accepted[targetRef]; duplicate {
			if previous != receiptHash {
				conflicts = append(conflicts, targetRef)
			}
			continue
		}
		accepted[targetRef] = receiptHash
	}

	for _, targetRef := range required {
		if _, found := accepted[targetRef]; found {
			finding.AcceptedTargetRefs = append(finding.AcceptedTargetRefs, targetRef)
			continue
		}
		finding.MissingTargetRefs = append(finding.MissingTargetRefs, targetRef)
	}
	if len(conflicts) != 0 {
		sort.Strings(conflicts)
		finding.Diagnostic = "prospective coverage target has multiple distinct accepted receipts: " + strings.Join(conflicts, ",")
	}
	return finding
}

func sameProspectiveReceiptTargetRefs(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	left = append([]string(nil), left...)
	right = append([]string(nil), right...)
	for index := range left {
		left[index] = strings.TrimSpace(left[index])
	}
	for index := range right {
		right[index] = strings.TrimSpace(right[index])
	}
	sort.Strings(left)
	sort.Strings(right)
	for index := range left {
		if left[index] == "" || left[index] != right[index] {
			return false
		}
	}
	return true
}
