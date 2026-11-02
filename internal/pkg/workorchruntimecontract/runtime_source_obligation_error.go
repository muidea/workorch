package workorchruntimecontract

import (
	"errors"
	"fmt"
)

const RuntimeSourceObligationConflictCode = "runtime_source_obligation_conflict"

// RuntimeSourceObligationConflictError identifies a deterministic source gate.
// Only pure source authentication/applicability/coverage errors belong here;
// owner I/O, provider failures and raw diagnostics must keep their own boundary.
// The original cause is retained, including when there is no Run yet.
type RuntimeSourceObligationConflictError struct {
	Stage string
	Cause error
}

func (e *RuntimeSourceObligationConflictError) Error() string {
	return fmt.Sprintf("%s [%s]: %s", RuntimeSourceObligationConflictCode, e.Stage, e.Cause)
}

func (e *RuntimeSourceObligationConflictError) Unwrap() error { return e.Cause }

// This classification is not a CAS/stale signal. Preserve the cause's existing
// errors.Is behavior so Runtime reducers/recovery cannot mistake a deterministic
// source rejection for a retryable concurrent write.
func NewRuntimeSourceObligationConflict(stage string, cause error) error {
	if cause == nil {
		return nil
	}
	var existing *RuntimeSourceObligationConflictError
	if errors.As(cause, &existing) {
		return cause
	}
	return &RuntimeSourceObligationConflictError{Stage: stage, Cause: cause}
}
