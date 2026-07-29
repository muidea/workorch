package workorchruntimecontract

import "fmt"

// RuntimeOwnerInventoryBudget bounds a complete capability-owner inventory or a
// closed selector representation returned by acquisition. It is an SM-2 authority
// budget: truncating inventory and treating it as complete is forbidden, and this
// type must never be satisfied by borrowing CandidateContextBudget.
type RuntimeOwnerInventoryBudget struct {
	MaxItems int
}

// RuntimeCandidateContextBudget bounds C3～C5 display/reference projection for one
// candidate request. It must not reduce owner authority completeness or change
// admissibility.
type RuntimeCandidateContextBudget struct {
	MaxItems int
}

// RuntimeRequestRecoveryBudget bounds fragment/transport/unknown submission
// recovery on the request session. It does not create candidate attempts and
// must not share counters with CandidateAttemptBudget.
type RuntimeRequestRecoveryBudget struct {
	MaxRecoveries int
}

// RuntimeCandidateAttemptBudget bounds complete-document candidate attempts
// (attempt 0/1). Transport recovery, prerequisite refresh and focus repair use
// other budgets.
type RuntimeCandidateAttemptBudget struct {
	MaxAttempts int
}

// RuntimePrerequisiteRefreshBudget bounds read-only stale/no-progress recovery.
// Owner-receipt resource changes advance the persisted prerequisite generation
// and must never consume a recovery budget.
type RuntimePrerequisiteRefreshBudget struct {
	MaxStaleRefreshes int
}

func DefaultRuntimeOwnerInventoryBudget() RuntimeOwnerInventoryBudget {
	return RuntimeOwnerInventoryBudget{MaxItems: RuntimeOwnerInventoryItemsMaxCount}
}

func DefaultRuntimeCandidateContextBudget() RuntimeCandidateContextBudget {
	return RuntimeCandidateContextBudget{MaxItems: RuntimeCandidateContextItemsMaxCount}
}

func DefaultRuntimeCandidateAttemptBudget() RuntimeCandidateAttemptBudget {
	// Attempt 0 plus one correctable-once attempt 1. There is no attempt 2.
	return RuntimeCandidateAttemptBudget{MaxAttempts: 2}
}

func DefaultRuntimePrerequisiteRefreshBudget() RuntimePrerequisiteRefreshBudget {
	return RuntimePrerequisiteRefreshBudget{
		MaxStaleRefreshes: DefaultMaxPrerequisiteRefreshes,
	}
}

func (b RuntimeOwnerInventoryBudget) Normalized() RuntimeOwnerInventoryBudget {
	if b.MaxItems <= 0 {
		b.MaxItems = RuntimeOwnerInventoryItemsMaxCount
	}
	return b
}

func (b RuntimeOwnerInventoryBudget) Validate() error {
	b = b.Normalized()
	if b.MaxItems < 1 || b.MaxItems > RuntimeOwnerInventoryItemsMaxCount {
		return fmt.Errorf("owner inventory budget maxItems must be in 1..%d", RuntimeOwnerInventoryItemsMaxCount)
	}
	return nil
}

func (b RuntimeCandidateContextBudget) Normalized() RuntimeCandidateContextBudget {
	if b.MaxItems <= 0 {
		b.MaxItems = RuntimeCandidateContextItemsMaxCount
	}
	return b
}

func (b RuntimeCandidateContextBudget) Validate() error {
	b = b.Normalized()
	if b.MaxItems < 1 || b.MaxItems > RuntimeCandidateContextItemsMaxCount {
		return fmt.Errorf("candidate context budget maxItems must be in 1..%d", RuntimeCandidateContextItemsMaxCount)
	}
	return nil
}

func (b RuntimeCandidateAttemptBudget) Normalized() RuntimeCandidateAttemptBudget {
	if b.MaxAttempts <= 0 {
		b.MaxAttempts = 2
	}
	return b
}

func (b RuntimeCandidateAttemptBudget) Validate() error {
	b = b.Normalized()
	if b.MaxAttempts < 1 || b.MaxAttempts > 2 {
		return fmt.Errorf("candidate attempt budget maxAttempts must be in 1..2")
	}
	return nil
}

func (b RuntimePrerequisiteRefreshBudget) Normalized() RuntimePrerequisiteRefreshBudget {
	if b.MaxStaleRefreshes <= 0 {
		b.MaxStaleRefreshes = DefaultMaxPrerequisiteRefreshes
	}
	return b
}

func (b RuntimePrerequisiteRefreshBudget) Validate() error {
	b = b.Normalized()
	if b.MaxStaleRefreshes < 1 {
		return fmt.Errorf("prerequisite refresh budget requires a positive stale limit")
	}
	return nil
}

// RuntimeOwnerInventoryWithinBudget reports whether a complete inventory
// representation fits the owner inventory budget. Callers must not use a
// CandidateContextBudget value here.
func RuntimeOwnerInventoryWithinBudget(budget RuntimeOwnerInventoryBudget, itemCount int) bool {
	budget = budget.Normalized()
	return itemCount >= 0 && itemCount <= budget.MaxItems
}
