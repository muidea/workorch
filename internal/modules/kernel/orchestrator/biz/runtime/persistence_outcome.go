package runtime

import (
	"fmt"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// RuntimeSemanticRejectionError is a Run-owner persistence fact that failed
// semantic validation. It intentionally does not unwrap common.ErrConflict:
// only RuntimeCASConflictError may authorize SM-4 reload/re-drive.
type RuntimeSemanticRejectionError struct {
	WorkspaceID      common.WorkspaceID
	RunID            common.RunID
	ExpectedRevision uint64
	ObservedRevision uint64
	Feedback         runtimecontract.RuntimeFeedback
}

func (e *RuntimeSemanticRejectionError) Error() string {
	if e == nil {
		return "runtime aggregate semantic rejection"
	}
	return fmt.Sprintf("runtime aggregate semantic rejection for workspace %s run %s at expected revision %d observed revision %d", e.WorkspaceID, e.RunID, e.ExpectedRevision, e.ObservedRevision)
}

// RuntimeOwnerTransientError is a persistence-owner availability fact. It is
// distinct from semantic rejection and exposes Temporary only so existing SM-4
// recovery policy can classify it without inspecting an error message.
type RuntimeOwnerTransientError struct {
	WorkspaceID      common.WorkspaceID
	RunID            common.RunID
	ExpectedRevision uint64
	ObservedRevision uint64
	Feedback         runtimecontract.RuntimeFeedback
}

func (e *RuntimeOwnerTransientError) Error() string {
	if e == nil {
		return "runtime aggregate owner transient"
	}
	return fmt.Sprintf("runtime aggregate owner transient for workspace %s run %s at expected revision %d observed revision %d", e.WorkspaceID, e.RunID, e.ExpectedRevision, e.ObservedRevision)
}

func (e *RuntimeOwnerTransientError) Temporary() bool { return e != nil }
