package file

import (
	"fmt"

	"github.com/muidea/workorch/internal/pkg/common"
)

// effectInternalEvent is the File-owner-only execution input. Cross-owner
// commands cannot call the adapter or choose filesystem mutation semantics.
type effectInternalEvent struct {
	Kind  effectInternalEventKind
	Patch *PatchApplyRequest
}

type effectInternalEventKind string

const effectInternalEventApplyPatch effectInternalEventKind = "apply_patch"

func (m *Biz) submitEffectInternalEvent(event effectInternalEvent) (RuntimeFileOperationResult, error) {
	if m == nil || m.runtimeAdapter == nil || event.Kind != effectInternalEventApplyPatch || event.Patch == nil {
		return RuntimeFileOperationResult{}, common.ErrInvalidArgument
	}
	request := *event.Patch
	if request.CreateOnly && request.PatchType != patchTypeFileWrite {
		return RuntimeFileOperationResult{}, fmt.Errorf("%w: create-only effect requires file_write", common.ErrInvalidArgument)
	}
	return m.runtimeAdapter.ApplyPatch(request)
}
