package workorchruntimecontract

import (
	"fmt"
	"strings"
)

// RuntimeOwnerInventoryBudget bounds a current focus/scope owner target
// inventory projection (or a closed selector representation) returned by
// operation-admissibility acquisition. Only TargetRefs consume target slots;
// SelectorRefs and ConstraintRefs are contract vocabulary and do not count
// against this budget. Required current-scope targets must never be truncated:
// when required targets exceed MaxItems, acquisition and manifest admission
// fail closed with RuntimeOwnerInventoryBudgetExceededError. This type must never
// be satisfied by borrowing CandidateContextBudget.
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

// RuntimeOwnerInventoryWithinBudget reports whether a complete target
// inventory representation fits the owner inventory budget. SelectorRefs and
// ConstraintRefs are contract vocabulary used to interpret targets; they are
// not additional inventory members and must not be counted here.
func RuntimeOwnerInventoryWithinBudget(budget RuntimeOwnerInventoryBudget, itemCount int) bool {
	budget = budget.Normalized()
	return itemCount >= 0 && itemCount <= budget.MaxItems
}

// RuntimeOwnerInventoryTargetCount is the single budget semantic shared by
// owner acquisition and candidate-manifest compilation. The target set is the
// bounded owner inventory; selectors and constraints describe how that set is
// interpreted and therefore do not consume target inventory capacity.
func RuntimeOwnerInventoryTargetCount(value RuntimeOperationAdmissibility) int {
	return len(value.Normalized().TargetRefs)
}

// RuntimeOwnerInventoryAdmissibilityWithinBudget checks one complete owner
// admissibility fact using the same target-only count used by acquisition.
func RuntimeOwnerInventoryAdmissibilityWithinBudget(budget RuntimeOwnerInventoryBudget, value RuntimeOperationAdmissibility) bool {
	return RuntimeOwnerInventoryWithinBudget(budget, RuntimeOwnerInventoryTargetCount(value))
}

// RuntimeOwnerInventoryBudgetExceededError identifies an owner fact whose
// target inventory cannot be admitted under the current request budget. It is
// a typed owner-fact deviation, not a business or Runtime-structure failure;
// the Orchestrator may invalidate only this prerequisite and reacquire it.
type RuntimeOwnerInventoryBudgetExceededError struct {
	OwnerModuleID  string
	PrerequisiteID string
	ItemCount      int
	MaxItems       int
}

func (e *RuntimeOwnerInventoryBudgetExceededError) Error() string {
	if e == nil {
		return "runtime owner inventory budget exceeded"
	}
	return fmt.Sprintf("runtime owner inventory budget exceeded: owner=%s prerequisite=%s targets=%d max=%d", strings.TrimSpace(e.OwnerModuleID), strings.TrimSpace(e.PrerequisiteID), e.ItemCount, e.MaxItems)
}

// ValidateRuntimeOwnerInventoryAdmissibilityBudget is the typed budget gate
// shared by acquisition validation and candidate-manifest compilation.
func ValidateRuntimeOwnerInventoryAdmissibilityBudget(budget RuntimeOwnerInventoryBudget, value RuntimeOperationAdmissibility) error {
	budget = budget.Normalized()
	value = value.Normalized()
	if !RuntimeOwnerInventoryWithinBudget(budget, RuntimeOwnerInventoryTargetCount(value)) {
		return &RuntimeOwnerInventoryBudgetExceededError{OwnerModuleID: value.OwnerModuleID, PrerequisiteID: value.PrerequisiteID, ItemCount: RuntimeOwnerInventoryTargetCount(value), MaxItems: budget.MaxItems}
	}
	return nil
}

// ValidateRuntimeRequiredTargetsWithinInventoryBudget guarantees that required
// current-scope targets are never silently truncated to fit an owner inventory
// projection. Duplicate or blank inputs are normalized before checking MaxItems.
func ValidateRuntimeRequiredTargetsWithinInventoryBudget(budget RuntimeOwnerInventoryBudget, ownerModuleID, prerequisiteID string, requiredTargetRefs []string) error {
	budget = budget.Normalized()
	required := normalizeStrings(requiredTargetRefs)
	if !RuntimeOwnerInventoryWithinBudget(budget, len(required)) {
		return &RuntimeOwnerInventoryBudgetExceededError{OwnerModuleID: strings.TrimSpace(ownerModuleID), PrerequisiteID: strings.TrimSpace(prerequisiteID), ItemCount: len(required), MaxItems: budget.MaxItems}
	}
	return nil
}
