package runtime

import (
	"fmt"

	"github.com/muidea/workorch/internal/pkg/common"
)

// RuntimeCASConflictError is the only typed signal that authorizes the SM-4
// CAS reload ledger. Semantic transition conflicts deliberately remain ordinary
// ErrConflict values and must never be converted into a persistence retry.
type RuntimeCASConflictError struct {
	WorkspaceID      common.WorkspaceID
	RunID            common.RunID
	ExpectedRevision uint64
	ObservedRevision uint64
	Cause            error
}

func (e *RuntimeCASConflictError) Error() string {
	if e == nil {
		return "runtime aggregate CAS conflict"
	}
	return fmt.Sprintf("runtime aggregate CAS conflict for workspace %s run %s: expected revision %d observed revision %d", e.WorkspaceID, e.RunID, e.ExpectedRevision, e.ObservedRevision)
}

func (e *RuntimeCASConflictError) Unwrap() error {
	if e == nil || e.Cause == nil {
		return common.ErrConflict
	}
	return e.Cause
}
