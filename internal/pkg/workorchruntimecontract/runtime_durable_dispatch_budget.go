package workorchruntimecontract

import (
	"fmt"
	"sort"

	"github.com/muidea/workorch/internal/pkg/common"
)

// RuntimeDurableDispatchBudgetExhaustedError is a logical seal guard, not a
// physical capability allocation and not an authorization for additional work.
type RuntimeDurableDispatchBudgetExhaustedError struct {
	CoverageWorkItemID common.CoverageWorkItemID
	Limit              int
}

func (e *RuntimeDurableDispatchBudgetExhaustedError) Error() string {
	return fmt.Sprintf("runtime durable dispatch budget exhausted for CWI %q (limit %d)", e.CoverageWorkItemID, e.Limit)
}

// RuntimeDurableDispatchUsage consumes validated admission lineage. Accepted
// attempts identify original seals; governance resumes have no new accepted
// attempt. Completion, invalidation, receipt outcome and generation retirement
// do not refund a seal. No mutable focus or provider-supplied CWI ID is used.
func RuntimeDurableDispatchUsage(active *RuntimeDispatchRecord, completed []RuntimeDispatchRecord, attempts []RuntimeCandidateAttemptRecord) (map[common.CoverageWorkItemID]int, error) {
	dispatches := make(map[string]RuntimeDispatchRecord, len(completed)+1)
	all := append([]RuntimeDispatchRecord(nil), completed...)
	if active != nil {
		all = append(all, *active)
	}
	for _, dispatch := range all {
		if _, exists := dispatches[dispatch.ID]; exists {
			return nil, fmt.Errorf("duplicate dispatch in durable budget inventory: %q", dispatch.ID)
		}
		dispatches[dispatch.ID] = dispatch
	}
	usage := make(map[common.CoverageWorkItemID]int)
	seen := make(map[string]bool)
	for _, attempt := range attempts {
		if attempt.State != RuntimeCandidateAttemptStateAccepted {
			continue
		}
		dispatch, found := dispatches[attempt.DispatchID]
		if !found || seen[attempt.DispatchID] {
			return nil, fmt.Errorf("missing or duplicate accepted dispatch in durable budget inventory: %q", attempt.DispatchID)
		}
		seen[attempt.DispatchID] = true
		ids, err := runtimeDurableDispatchCoverage(dispatch)
		if err != nil {
			return nil, err
		}
		for _, id := range ids {
			usage[id]++
		}
	}
	return usage, nil
}

// A durable transaction reserves once for each original CWI in its sealed
// binding inventory, not once per file/operation. Non-durable seals reserve none.
func runtimeDurableDispatchCoverage(dispatch RuntimeDispatchRecord) ([]common.CoverageWorkItemID, error) {
	durable := false
	for _, execution := range dispatch.OperationExecutions {
		durable = durable || RuntimeEffectIsDurable(execution.ExpectedEffectScope)
	}
	if !durable {
		return nil, nil
	}
	seen := make(map[common.CoverageWorkItemID]bool)
	var ids []common.CoverageWorkItemID
	for _, binding := range dispatch.CoverageBindings {
		if binding.CoverageWorkItemID == "" {
			return nil, fmt.Errorf("durable dispatch has an empty original CWI")
		}
		if !seen[binding.CoverageWorkItemID] {
			seen[binding.CoverageWorkItemID] = true
			ids = append(ids, binding.CoverageWorkItemID)
		}
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("durable dispatch has no sealed CWI binding")
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids, nil
}

// CheckRuntimeDurableDispatchBudget runs before the accepted attempt and new
// seal are committed together. The caller must validate existing lineage first.
func CheckRuntimeDurableDispatchBudget(limits *RuntimeExecutionLimits, active *RuntimeDispatchRecord, completed []RuntimeDispatchRecord, attempts []RuntimeCandidateAttemptRecord, next RuntimeDispatchRecord) error {
	if limits == nil || limits.MaxDurableDispatchesPerCWI == 0 {
		return nil
	}
	usage, err := RuntimeDurableDispatchUsage(active, completed, attempts)
	if err != nil {
		return err
	}
	ids, err := runtimeDurableDispatchCoverage(next)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if usage[id] >= limits.MaxDurableDispatchesPerCWI {
			return &RuntimeDurableDispatchBudgetExhaustedError{CoverageWorkItemID: id, Limit: limits.MaxDurableDispatchesPerCWI}
		}
	}
	return nil
}

// ValidateRuntimeDurableDispatchLimits rejects persisted over-consumption as
// corruption, not as a new recoverable budget event. Admission lineage is also
// required by both aggregate validators; omission of an attempt cannot refund.
func ValidateRuntimeDurableDispatchLimits(limits *RuntimeExecutionLimits, active *RuntimeDispatchRecord, completed []RuntimeDispatchRecord, attempts []RuntimeCandidateAttemptRecord) error {
	if limits == nil || limits.MaxDurableDispatchesPerCWI == 0 {
		return nil
	}
	usage, err := RuntimeDurableDispatchUsage(active, completed, attempts)
	if err != nil {
		return err
	}
	for id, used := range usage {
		if used > limits.MaxDurableDispatchesPerCWI {
			return fmt.Errorf("persisted durable dispatch usage for CWI %q exceeds sealed limit", id)
		}
	}
	return nil
}
