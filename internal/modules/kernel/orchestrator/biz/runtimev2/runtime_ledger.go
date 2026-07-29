package runtimev2

import (
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// The helpers in this file are the only Runtime V2 ledger transition owner.
// Runtime contract values remain serializable DTOs; deciding and recording
// progress, repair and recovery belongs to the Orchestrator state machines.

func applyAcceptedStateChangeEvidence(ledger runtimecontract.RuntimeStateChangeLedger, evidence runtimecontract.EvidenceFact, currentCoverage []common.CoverageWorkItemID) runtimecontract.RuntimeStateChangeLedger {
	ledger = ledger.Normalized()
	if !ledger.Required || ledger.Satisfied || evidence.Kind != runtimecontract.EvidenceKindStateChange {
		return ledger
	}
	evidenceCoverage := normalizeCoverageIDs(evidence.CoverageWorkItemIDs)
	if len(evidenceCoverage) == 0 {
		return ledger
	}
	required := ledger.CoverageWorkItemIDs
	if len(required) == 0 {
		required = normalizeCoverageIDs(currentCoverage)
	}
	if len(required) == 0 {
		ledger.Satisfied = true
		ledger.CoverageWorkItemIDs = evidenceCoverage
		ledger.EvidenceRefs = appendUniqueString(ledger.EvidenceRefs, string(evidence.ID))
		ledger.LastEvidenceRevision = strings.TrimSpace(evidence.Revision)
		return ledger
	}
	if !coverageIDsIntersect(required, evidenceCoverage) {
		return ledger
	}
	ledger.Satisfied = true
	ledger.EvidenceRefs = appendUniqueString(ledger.EvidenceRefs, string(evidence.ID))
	if revision := strings.TrimSpace(evidence.Revision); revision != "" {
		ledger.LastEvidenceRevision = revision
	}
	if len(ledger.CoverageWorkItemIDs) == 0 {
		ledger.CoverageWorkItemIDs = required
	}
	return ledger
}

func beginRepairCycle(ledger runtimecontract.RuntimeRepairLedger, focusID, sliceID, cycleID, signature string, policy runtimecontract.RuntimeConvergencePolicy) (runtimecontract.RuntimeRepairLedger, bool, error) {
	ledger = ledger.Normalized()
	policy = policy.Normalized()
	focusID, sliceID = strings.TrimSpace(focusID), strings.TrimSpace(sliceID)
	cycleID, signature = strings.TrimSpace(cycleID), strings.TrimSpace(signature)
	if focusID == "" || cycleID == "" || signature == "" {
		return ledger, false, fmt.Errorf("repair cycle requires focus, cycle id and signature")
	}
	if ledger.FocusID != "" && ledger.FocusID != focusID {
		ledger = runtimecontract.RuntimeRepairLedger{FocusID: focusID, SliceID: sliceID}
	} else {
		ledger.FocusID = focusID
		if sliceID != "" {
			ledger.SliceID = sliceID
		}
	}
	if ledger.LastCycleID == cycleID {
		return ledger, false, nil
	}
	if ledger.RepairAttempts >= policy.MaxRepairAttemptsPerFocus {
		ledger.Exhausted = true
		return ledger, false, nil
	}
	ledger.LastCycleID = cycleID
	ledger.RepairAttempts++
	ledger.SignatureCounts = incrementFailureSignature(ledger.SignatureCounts, signature)
	return ledger, true, nil
}

func markValidationOnlyRetry(ledger runtimecontract.RuntimeRepairLedger) runtimecontract.RuntimeRepairLedger {
	ledger = ledger.Normalized()
	ledger.ValidationOnlyRetries++
	return ledger
}

func noteRepairProgress(ledger runtimecontract.RuntimeRepairLedger, revision string) runtimecontract.RuntimeRepairLedger {
	ledger = ledger.Normalized()
	ledger.NoProgressCount = 0
	ledger.ValidationOnlyRetries = 0
	ledger.SignatureCounts = nil
	ledger.NoProgressOperationShapes = nil
	ledger.LastCycleID = ""
	ledger.ActiveGap = nil
	ledger.Exhausted = false
	if revision = strings.TrimSpace(revision); revision != "" {
		ledger.LastProgressRevision = revision
	}
	return ledger
}

// noteRepairProgressPreservingNoOpProvenance closes the ordinary repair
// window while retaining the exact no-op observation needed by the C8-F
// reducer branch. The retained shape is not accepted evidence and cannot
// satisfy a state-change requirement on its own.
func noteRepairProgressPreservingNoOpProvenance(ledger runtimecontract.RuntimeRepairLedger, revision string) runtimecontract.RuntimeRepairLedger {
	ledger = ledger.Normalized()
	noOps := make([]runtimecontract.RuntimeRejectedOperationShape, 0, len(ledger.NoProgressOperationShapes))
	for _, shape := range ledger.NoProgressOperationShapes {
		if shape.FailureClass == runtimecontract.FailureClassEffectMismatch && shape.FailureCode == runtimeStateChangeNoOpWithoutProvenanceCode {
			noOps = append(noOps, shape)
		}
	}
	noProgressCount := ledger.NoProgressCount
	ledger = noteRepairProgress(ledger, revision)
	ledger.NoProgressOperationShapes = runtimecontract.NormalizeRuntimeRejectedOperationShapes(noOps)
	ledger.NoProgressCount = noProgressCount
	return ledger
}

func noteRepairGapProgress(ledger runtimecontract.RuntimeRepairLedger, gap runtimecontract.RuntimeAssessmentGap, revision string) runtimecontract.RuntimeRepairLedger {
	ledger = noteRepairProgress(ledger, revision)
	gap = gap.Normalized()
	ledger.ActiveGap = &gap
	return ledger
}

func recordNoProgressOperationShape(ledger runtimecontract.RuntimeRepairLedger, shape runtimecontract.RuntimeRejectedOperationShape) runtimecontract.RuntimeRepairLedger {
	ledger = ledger.Normalized()
	ledger.NoProgressOperationShapes = runtimecontract.NormalizeRuntimeRejectedOperationShapes(append(ledger.NoProgressOperationShapes, shape))
	return ledger
}

func resetRepairForFocus(focusID, sliceID, revision string) runtimecontract.RuntimeRepairLedger {
	return runtimecontract.RuntimeRepairLedger{FocusID: strings.TrimSpace(focusID), SliceID: strings.TrimSpace(sliceID), LastProgressRevision: strings.TrimSpace(revision)}
}

func noteRepairNoProgress(ledger runtimecontract.RuntimeRepairLedger) runtimecontract.RuntimeRepairLedger {
	ledger = ledger.Normalized()
	ledger.NoProgressCount++
	return ledger
}

func consumeRecoveryBudget(ledger runtimecontract.RuntimeRecoveryLedger, kind runtimecontract.RuntimeRecoveryBudgetKind, policy runtimecontract.RuntimeRecoveryPolicy) (runtimecontract.RuntimeRecoveryLedger, bool, error) {
	if policy.Limit(kind) == 0 {
		return ledger, false, fmt.Errorf("unknown runtime recovery budget %q", kind)
	}
	if ledger.Used(kind) >= policy.Limit(kind) {
		return ledger, false, nil
	}
	switch kind {
	case runtimecontract.RuntimeRecoveryBudgetProviderTransport:
		ledger.ProviderTransportAttempts++
	case runtimecontract.RuntimeRecoveryBudgetOperationRecovery:
		ledger.OperationRecoveryAttempts++
	case runtimecontract.RuntimeRecoveryBudgetCASReload:
		ledger.CASReloadAttempts++
	case runtimecontract.RuntimeRecoveryBudgetTerminalDelivery:
		ledger.TerminalDeliveryAttempts++
	case runtimecontract.RuntimeRecoveryBudgetDriveSchedule:
		ledger.DriveScheduleAttempts++
	case runtimecontract.RuntimeRecoveryBudgetPrerequisiteRefresh:
		ledger.PrerequisiteRefreshes++
	case runtimecontract.RuntimeRecoveryBudgetPrerequisiteResourceChangeRefresh:
		ledger.PrerequisiteResourceChangeRefreshes++
	}
	return ledger, true, nil
}

func normalizeCoverageIDs(values []common.CoverageWorkItemID) []common.CoverageWorkItemID {
	seen := make(map[common.CoverageWorkItemID]struct{}, len(values))
	ret := make([]common.CoverageWorkItemID, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		ret = append(ret, value)
	}
	return ret
}

func coverageIDsIntersect(left, right []common.CoverageWorkItemID) bool {
	seen := make(map[common.CoverageWorkItemID]struct{}, len(left))
	for _, value := range left {
		seen[value] = struct{}{}
	}
	for _, value := range right {
		if _, ok := seen[value]; ok {
			return true
		}
	}
	return false
}

func appendUniqueString(values []string, value string) []string {
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

func incrementFailureSignature(values []runtimecontract.RuntimeFailureSignatureCount, signature string) []runtimecontract.RuntimeFailureSignatureCount {
	for idx := range values {
		if values[idx].Signature == signature {
			values[idx].Count++
			return values
		}
	}
	return append(append([]runtimecontract.RuntimeFailureSignatureCount(nil), values...), runtimecontract.RuntimeFailureSignatureCount{Signature: signature, Count: 1})
}
