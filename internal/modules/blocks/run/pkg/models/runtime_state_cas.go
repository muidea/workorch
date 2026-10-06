package run

import (
	"reflect"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// ValidateOpaqueRuntimeStateCAS proves that a physical Runtime write changes
// only Orchestrator-authored opaque state. The Run repository still owns the
// physical revision and every SM-1/replacement field and therefore rechecks
// this invariant inside its generation-head lock.
func ValidateOpaqueRuntimeStateCAS(current, next *Run, expectedRuntimeStateRevision uint64) error {
	if current == nil || next == nil || current.ID == "" || current.WorkspaceID == "" ||
		current.ID != next.ID || current.WorkspaceID != next.WorkspaceID || current.Revision != next.Revision ||
		current.WorkspaceRunGeneration == 0 || current.WorkspaceRunGeneration != next.WorkspaceRunGeneration || next.RuntimeState == nil {
		return common.ErrConflict
	}
	currentRuntimeRevision := uint64(0)
	if current.RuntimeState != nil {
		currentRuntimeRevision = current.RuntimeState.StateRevision
	}
	if currentRuntimeRevision != expectedRuntimeStateRevision || next.RuntimeState.StateRevision != expectedRuntimeStateRevision+1 {
		return common.ErrConflict
	}
	// 当前 opaque aggregate 会被新值完整替换；不复制随后丢弃的旧账本。
	// 其余 Run 字段仍使用原深拷贝及完整比较，物理写集边界不变。
	currentWithoutRuntimeState := *current
	currentWithoutRuntimeState.RuntimeState = nil
	expected := CloneRun(&currentWithoutRuntimeState)
	expected.RuntimeState = cloneOpaqueRuntimeState(next.RuntimeState)
	expected.RuntimeLifecycleFence = cloneOpaqueRuntimeLifecycleFence(next.RuntimeLifecycleFence)
	if !reflect.DeepEqual(expected, next) {
		return common.ErrConflict
	}
	return nil
}

func cloneOpaqueRuntimeState(value *runtimecontract.RuntimeStateRecord) *runtimecontract.RuntimeStateRecord {
	if value == nil {
		return nil
	}
	cloned := value.Clone()
	return &cloned
}

func cloneOpaqueRuntimeLifecycleFence(value *runtimecontract.RuntimeOpaqueLifecycleFence) *runtimecontract.RuntimeOpaqueLifecycleFence {
	if value == nil {
		return nil
	}
	cloned := value.Normalized()
	return &cloned
}
