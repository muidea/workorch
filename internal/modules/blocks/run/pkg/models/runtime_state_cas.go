package run

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"reflect"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// ErrRuntimeStateIntegrity is a rejected write, not a concurrent CAS conflict.
// The Run command maps it to semantic rejection without retrying the write.
var ErrRuntimeStateIntegrity = fmt.Errorf("%w: runtime state integrity mismatch", common.ErrConflict)

// RuntimeStateWriteHash computes the physical fingerprint once, before a new
// record is published. The same value checks the incoming opaque lifecycle
// fence and becomes the stored integrity identity; no business state is read.
func RuntimeStateWriteHash(item *Run) (string, error) {
	if item == nil {
		return "", common.ErrInvalidArgument
	}
	if item.RuntimeState == nil {
		if item.RuntimeStateHash != "" || item.RuntimeLifecycleFence != nil {
			return "", ErrRuntimeStateIntegrity
		}
		return "", nil
	}
	digest := runtimecontract.StableRuntimeStateRecordHash(*item.RuntimeState)
	return runtimeStateWriteHash(item, digest)
}

// AppendRuntimeStateWrite appends the exact physical JSON value and checks its
// identity against the lifecycle fence. File persistence publishes this same
// buffer; neither a second state encoding nor a second full payload is needed.
// This is encoding only, not adjudication of opaque Runtime business state.
func AppendRuntimeStateWrite(dst *bytes.Buffer, item *Run) (string, error) {
	digest, _, err := AppendRuntimeStateWriteWithPreimage(dst, item, nil, nil, nil)
	return digest, err
}

// AppendRuntimeStateWriteWithPreimage appends exact canonical RuntimeState JSON
// reusing verified byte spans from an immutable previous Run image when provided.
func AppendRuntimeStateWriteWithPreimage(
	dst *bytes.Buffer,
	item *Run,
	prev *runtimecontract.RuntimeStateRecord,
	prevRaw []byte,
	prevSpans *runtimecontract.RuntimeStateJSONSpans,
) (string, *runtimecontract.RuntimeStateJSONSpans, error) {
	if dst == nil {
		return "", nil, common.ErrInvalidArgument
	}
	if item == nil || item.RuntimeState == nil {
		digest, err := RuntimeStateWriteHash(item)
		if err == nil {
			dst.WriteString("null")
		}
		return digest, nil, err
	}
	start := dst.Len()
	spans, err := runtimecontract.WriteRuntimeStateRecordJSONWithPreimage(dst, *item.RuntimeState, prev, prevRaw, prevSpans)
	if err != nil {
		dst.Truncate(start)
		return "", nil, ErrRuntimeStateIntegrity
	}
	sum := runtimecontract.SumRuntimeStateJSONWithPreimage(dst.Bytes()[start:], prevRaw, prevSpans, &spans)
	digest, err := runtimeStateWriteHash(item, "runtime-state-"+hex.EncodeToString(sum[:]))
	if err != nil {
		dst.Truncate(start)
		return "", nil, err
	}
	return digest, &spans, nil
}

func runtimeStateWriteHash(item *Run, digest string) (string, error) {
	if digest == "" || (item.RuntimeLifecycleFence != nil && item.RuntimeLifecycleFence.RuntimeStateHash != digest) {
		return "", ErrRuntimeStateIntegrity
	}
	return digest, nil
}

// ValidateStoredRuntimeStateIntegrity belongs to the physical read boundary.
// It checks bytes decoded from storage, not the business meaning of a result.
// Run-owner queries consume detached values from that boundary without a second
// complete-state serialization. Memory storage publishes only isolated writes.
func ValidateStoredRuntimeStateIntegrity(item *Run) error {
	if item == nil {
		return common.ErrInvalidArgument
	}
	if item.RuntimeState == nil {
		if item.RuntimeStateHash != "" {
			return fmt.Errorf("%w: runtime integrity without state", common.ErrConflict)
		}
		return nil
	}
	if item.RuntimeStateHash == "" || item.RuntimeStateHash != runtimecontract.StableRuntimeStateRecordHash(*item.RuntimeState) {
		return fmt.Errorf("%w: persisted runtime state integrity mismatch", common.ErrConflict)
	}
	return nil
}

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
	if next.RuntimeLifecycleFence != nil && !reflect.DeepEqual(*next.RuntimeLifecycleFence, next.RuntimeLifecycleFence.Normalized()) {
		return common.ErrConflict
	}
	before, after := *current, *next
	before.RuntimeState, after.RuntimeState = nil, nil
	before.RuntimeLifecycleFence, after.RuntimeLifecycleFence = nil, nil
	if !runtimecontract.RuntimeRawEqual(before, after) {
		// Preserve the Run metadata projection contract without copying Runtime.
		before = *CloneRun(&before)
		if !runtimecontract.RuntimeRawEqual(before, after) {
			return common.ErrConflict
		}
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

// ValidateRuntimeStatePreserved fences ordinary SM-1/metadata writes away from
// the opaque Orchestrator state. Only CompareAndSwapRuntimeState may replace it.
func ValidateRuntimeStatePreserved(current, next *Run) error {
	if current == nil || next == nil || current.RuntimeStateHash != next.RuntimeStateHash ||
		!reflect.DeepEqual(current.RuntimeState, next.RuntimeState) ||
		!reflect.DeepEqual(current.RuntimeLifecycleFence, next.RuntimeLifecycleFence) {
		return common.ErrConflict
	}
	return nil
}
