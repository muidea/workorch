package run

import "github.com/muidea/workorch/internal/pkg/common"

// RuntimeStateWriteBase is a detached SM-1 image for an opaque CAS. The old
// Runtime body is deliberately absent: it is not an input to the replacement.
// The physical repository still compares the actual stored image under lock.
type RuntimeStateWriteBase struct {
	Run                  *Run
	RuntimeStateRevision uint64
}

// RuntimeStateWriteBaseFromRun borrows only under the repository read lock.
func RuntimeStateWriteBaseFromRun(current *Run) (RuntimeStateWriteBase, error) {
	if current == nil {
		return RuntimeStateWriteBase{}, common.ErrNotFound
	}
	var revision uint64
	if current.RuntimeState != nil {
		revision = current.RuntimeState.StateRevision
	}
	metadata := *current
	metadata.RuntimeState, metadata.RuntimeLifecycleFence = nil, nil
	return RuntimeStateWriteBase{Run: CloneRun(&metadata), RuntimeStateRevision: revision}, nil
}
