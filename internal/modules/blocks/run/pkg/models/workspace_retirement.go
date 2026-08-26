package run

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	deletioncontract "github.com/muidea/workorch/internal/pkg/workorchdeletioncontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

var ErrWorkspaceRetirementBusy = errors.New("Workspace retirement is waiting for Run-owned activity to become idle")

// WorkspaceRetirementPhase is Run-owner private state. Draining closes every
// ordinary Run/Step/Checkpoint mutation lane while retaining one exact opaque
// Runtime-state CAS lane so an already-terminal SM-4 delivery can finish.
// Retired closes that final lane as well.
type WorkspaceRetirementPhase string

const (
	WorkspaceRetirementPhaseDraining WorkspaceRetirementPhase = "draining"
	WorkspaceRetirementPhaseRetired  WorkspaceRetirementPhase = "retired"
)

type WorkspaceRetirementIntent struct {
	WorkspaceID           common.WorkspaceID
	DeletionID            string
	DeletionGeneration    uint64
	DataDispositionIntent deletioncontract.Mode
	WorkspaceIntentHash   string
}

func (i WorkspaceRetirementIntent) Normalized() WorkspaceRetirementIntent {
	i.WorkspaceID = common.WorkspaceID(strings.TrimSpace(string(i.WorkspaceID)))
	i.DeletionID = strings.TrimSpace(i.DeletionID)
	i.DataDispositionIntent = deletioncontract.Mode(strings.TrimSpace(string(i.DataDispositionIntent)))
	i.WorkspaceIntentHash = strings.TrimSpace(i.WorkspaceIntentHash)
	return i
}

func (i WorkspaceRetirementIntent) Validate() error {
	i = i.Normalized()
	if i.WorkspaceID == "" || i.DeletionID == "" || i.DeletionGeneration != 1 || i.WorkspaceIntentHash == "" {
		return common.ErrInvalidArgument
	}
	switch i.DataDispositionIntent {
	case deletioncontract.ModeRetain, deletioncontract.ModePurge:
		return nil
	default:
		return common.ErrInvalidArgument
	}
}

// WorkspaceRetirementFence is stored beside the monotonic Run-generation head
// so generation allocation, drain admission, final Runtime persistence and
// permanent retirement share one linearization boundary.
type WorkspaceRetirementFence struct {
	WorkspaceID            common.WorkspaceID
	DeletionID             string
	DeletionGeneration     uint64
	DataDispositionIntent  deletioncontract.Mode
	WorkspaceIntentHash    string
	Phase                  WorkspaceRetirementPhase
	FenceRevision          uint64
	WorkspaceRunGeneration uint64
	QuiescenceID           string
	RuntimeInventoryHash   string
}

func (f WorkspaceRetirementFence) Intent() WorkspaceRetirementIntent {
	return WorkspaceRetirementIntent{
		WorkspaceID: f.WorkspaceID, DeletionID: f.DeletionID, DeletionGeneration: f.DeletionGeneration,
		DataDispositionIntent: f.DataDispositionIntent, WorkspaceIntentHash: f.WorkspaceIntentHash,
	}.Normalized()
}

func (f WorkspaceRetirementFence) Normalized() WorkspaceRetirementFence {
	f.WorkspaceID = common.WorkspaceID(strings.TrimSpace(string(f.WorkspaceID)))
	f.DeletionID = strings.TrimSpace(f.DeletionID)
	f.DataDispositionIntent = deletioncontract.Mode(strings.TrimSpace(string(f.DataDispositionIntent)))
	f.WorkspaceIntentHash = strings.TrimSpace(f.WorkspaceIntentHash)
	f.Phase = WorkspaceRetirementPhase(strings.TrimSpace(string(f.Phase)))
	f.QuiescenceID = strings.TrimSpace(f.QuiescenceID)
	f.RuntimeInventoryHash = strings.TrimSpace(f.RuntimeInventoryHash)
	return f
}

func (f WorkspaceRetirementFence) Validate() error {
	f = f.Normalized()
	if err := f.Intent().Validate(); err != nil {
		return fmt.Errorf("%w: Run Workspace retirement fence requires exact deletion identity", err)
	}
	switch f.Phase {
	case WorkspaceRetirementPhaseDraining:
		if f.FenceRevision != 1 || f.QuiescenceID != "" || f.RuntimeInventoryHash != "" {
			return fmt.Errorf("%w: Run Workspace draining fence has invalid revision or quiescence proof", common.ErrConflict)
		}
	case WorkspaceRetirementPhaseRetired:
		if f.FenceRevision != 2 || f.QuiescenceID == "" || f.RuntimeInventoryHash == "" {
			return fmt.Errorf("%w: Run Workspace retired fence has invalid revision or quiescence proof", common.ErrConflict)
		}
	default:
		return fmt.Errorf("%w: Run Workspace retirement fence has invalid phase", common.ErrInvalidArgument)
	}
	return nil
}

func (f WorkspaceRetirementFence) IsDraining() bool {
	return f.Normalized().Phase == WorkspaceRetirementPhaseDraining
}

func (f WorkspaceRetirementFence) IsRetired() bool {
	return f.Normalized().Phase == WorkspaceRetirementPhaseRetired
}

func SameWorkspaceRetirementFence(left, right WorkspaceRetirementFence) bool {
	left = left.Normalized()
	right = right.Normalized()
	return left.WorkspaceID == right.WorkspaceID &&
		left.DeletionID == right.DeletionID &&
		left.DeletionGeneration == right.DeletionGeneration &&
		left.DataDispositionIntent == right.DataDispositionIntent &&
		left.WorkspaceIntentHash == right.WorkspaceIntentHash &&
		left.Phase == right.Phase &&
		left.FenceRevision == right.FenceRevision &&
		left.WorkspaceRunGeneration == right.WorkspaceRunGeneration &&
		left.QuiescenceID == right.QuiescenceID &&
		left.RuntimeInventoryHash == right.RuntimeInventoryHash
}

func SameWorkspaceRetirementIdentity(left, right WorkspaceRetirementFence) bool {
	left = left.Normalized()
	right = right.Normalized()
	return left.WorkspaceID == right.WorkspaceID &&
		left.DeletionID == right.DeletionID &&
		left.DeletionGeneration == right.DeletionGeneration &&
		left.DataDispositionIntent == right.DataDispositionIntent &&
		left.WorkspaceIntentHash == right.WorkspaceIntentHash &&
		left.WorkspaceRunGeneration == right.WorkspaceRunGeneration
}

const (
	WorkspaceRetirementPurgeSurfaceRuns        = "runs"
	WorkspaceRetirementPurgeSurfaceSteps       = "steps"
	WorkspaceRetirementPurgeSurfaceCheckpoints = "runtime_checkpoints"
)

var workspaceRetirementPurgeSurfaces = [...]string{
	WorkspaceRetirementPurgeSurfaceRuns,
	WorkspaceRetirementPurgeSurfaceSteps,
	WorkspaceRetirementPurgeSurfaceCheckpoints,
}

// WorkspaceRetirementPurgeObservation is the Run owner's typed proof that one
// physical resource surface is empty after a retired-fence purge. The fixed
// ordering makes the proof stable across first execution and response-loss
// retries.
type WorkspaceRetirementPurgeObservation struct {
	Surface        string
	ObservedBefore uint64
	DeletedCount   uint64
	RemainingCount uint64
}

func EmptyWorkspaceRetirementPurgeObservations() []WorkspaceRetirementPurgeObservation {
	ret := make([]WorkspaceRetirementPurgeObservation, len(workspaceRetirementPurgeSurfaces))
	for idx, surface := range workspaceRetirementPurgeSurfaces {
		ret[idx] = WorkspaceRetirementPurgeObservation{Surface: surface}
	}
	return ret
}

func ValidateEmptyWorkspaceRetirementPurgeObservations(observations []WorkspaceRetirementPurgeObservation) error {
	if len(observations) != len(workspaceRetirementPurgeSurfaces) {
		return common.ErrConflict
	}
	for idx, observation := range observations {
		if strings.TrimSpace(observation.Surface) != workspaceRetirementPurgeSurfaces[idx] || observation.RemainingCount != 0 || observation.DeletedCount > observation.ObservedBefore || observation.ObservedBefore-observation.DeletedCount != observation.RemainingCount {
			return common.ErrConflict
		}
	}
	return nil
}

func StableWorkspaceRetirementPurgeProofHash(fence WorkspaceRetirementFence, observations []WorkspaceRetirementPurgeObservation) string {
	fence = fence.Normalized()
	material := fmt.Sprintf("run-workspace-resource-purge\x00%s\x00%s\x00%d\x00%s\x00%s\x00%d\x00%d\x00%s\x00%s", fence.WorkspaceID, fence.DeletionID, fence.DeletionGeneration, fence.DataDispositionIntent, fence.WorkspaceIntentHash, fence.FenceRevision, fence.WorkspaceRunGeneration, fence.RuntimeInventoryHash, fence.QuiescenceID)
	for _, observation := range observations {
		material += fmt.Sprintf("\x00%s\x00%d\x00%d\x00%d", strings.TrimSpace(observation.Surface), observation.ObservedBefore, observation.DeletedCount, observation.RemainingCount)
	}
	digest := sha256.Sum256([]byte(material))
	return "run-workspace-resource-purge-sha256:" + hex.EncodeToString(digest[:])
}

// ValidateWorkspaceRetirementRuntimePhysicalState proves the Run-owned
// physical projection before Orchestrator may seal its quiescing gate. It does
// not decode Runtime semantics; it only compares the opaque record, lifecycle
// fence and terminal-delivery identities byte-for-byte.
func ValidateWorkspaceRetirementRuntimePhysicalState(item *Run, workspaceID common.WorkspaceID) error {
	if item == nil || workspaceID == "" || item.WorkspaceID != workspaceID || item.ID == "" || item.WorkspaceRunGeneration == 0 || item.Revision == 0 || !workspaceRetirementTerminalRunState(item.State) {
		return common.ErrConflict
	}
	if item.RuntimeState == nil {
		if item.RuntimeLifecycleFence != nil || item.TerminalDelivery != nil {
			return common.ErrConflict
		}
		return nil
	}
	if item.RuntimeState.StateRevision == 0 || item.RuntimeLifecycleFence == nil || item.TerminalDelivery == nil || strings.TrimSpace(item.TerminalDelivery.TerminalDeliveryID) == "" {
		return common.ErrConflict
	}
	lifecycle := item.RuntimeLifecycleFence.Normalized()
	expectedRef := fmt.Sprintf("runtime-state/%s/%s/%d", workspaceID, item.ID, item.RuntimeState.StateRevision)
	expectedHash := runtimecontract.StableRuntimeStateRecordHash(*item.RuntimeState)
	if lifecycle.Validate() != nil || lifecycle.WorkspaceID != workspaceID || lifecycle.RunID != item.ID ||
		lifecycle.RuntimeStateRevision != item.RuntimeState.StateRevision || lifecycle.RuntimeStateRef != expectedRef ||
		lifecycle.RuntimeStateHash != expectedHash || lifecycle.TerminalFenceID != strings.TrimSpace(item.TerminalDelivery.TerminalDeliveryID) {
		return common.ErrConflict
	}
	return nil
}

// ValidateWorkspaceRetirementFinalization compares a quiescence fact with
// Run-owned physical state. It deliberately treats RuntimeState as opaque: the
// token proves semantic quiescence, while Run compares only exact identity,
// revision and stable bytes-derived hashes.
func ValidateWorkspaceRetirementFinalization(fence WorkspaceRetirementFence, items []*Run, token deletioncontract.WorkspaceRuntimeQuiescenceToken) error {
	fence = fence.Normalized()
	token = token.Normalized()
	if err := fence.Validate(); err != nil || !fence.IsDraining() || token.Validate() != nil {
		return common.ErrConflict
	}
	if fence.WorkspaceID != token.WorkspaceID || fence.DeletionID != token.DeletionID ||
		fence.DeletionGeneration != token.DeletionGeneration || fence.DataDispositionIntent != token.DataDispositionIntent ||
		fence.WorkspaceIntentHash != token.WorkspaceIntentHash || fence.FenceRevision != token.RunDrainingRevision ||
		fence.WorkspaceRunGeneration != token.WorkspaceRunGeneration {
		return common.ErrConflict
	}
	if err := WorkspaceRetirementEligible(items, fence.WorkspaceID); err != nil {
		return err
	}
	items = CloneRuns(items)
	sort.Slice(items, func(i, j int) bool {
		if items[i].WorkspaceRunGeneration != items[j].WorkspaceRunGeneration {
			return items[i].WorkspaceRunGeneration < items[j].WorkspaceRunGeneration
		}
		return items[i].ID < items[j].ID
	})
	if len(items) != len(token.Runs) {
		return common.ErrConflict
	}
	for idx, item := range items {
		fact := token.Runs[idx].Normalized()
		if item == nil || item.ID != fact.RunID || item.WorkspaceRunGeneration != fact.WorkspaceRunGeneration ||
			item.Revision != fact.RunRevision || item.State != fact.RunState {
			return common.ErrConflict
		}
		if err := ValidateWorkspaceRetirementRuntimePhysicalState(item, fence.WorkspaceID); err != nil {
			return err
		}
		if item.RuntimeState == nil {
			if fact.RuntimeStatePresent || item.RuntimeLifecycleFence != nil || item.TerminalDelivery != nil {
				return common.ErrConflict
			}
			continue
		}
		if !fact.RuntimeStatePresent || item.RuntimeState.StateRevision != fact.RuntimeStateRevision ||
			fact.RuntimeStateRef != fmt.Sprintf("runtime-state/%s/%s/%d", fence.WorkspaceID, item.ID, item.RuntimeState.StateRevision) ||
			fact.RuntimeStateHash != runtimecontract.StableRuntimeStateRecordHash(*item.RuntimeState) || !fact.TerminalDeliveryAcked ||
			item.TerminalDelivery == nil || item.TerminalDelivery.TerminalDeliveryID != fact.TerminalDeliveryID ||
			item.RuntimeLifecycleFence == nil {
			return common.ErrConflict
		}
		lifecycle := item.RuntimeLifecycleFence.Normalized()
		if lifecycle.WorkspaceID != fence.WorkspaceID || lifecycle.RunID != item.ID ||
			lifecycle.RuntimeStateRevision != fact.RuntimeStateRevision || lifecycle.RuntimeStateRef != fact.RuntimeStateRef ||
			lifecycle.RuntimeStateHash != fact.RuntimeStateHash || lifecycle.TerminalFenceID != fact.TerminalDeliveryID {
			return common.ErrConflict
		}
	}
	return nil
}

// ValidateWorkspaceRetirementFinalizationReplay accepts only the exact token
// that already produced a retired fence. The stable hashes bind the ordered
// physical inventory, while the explicit fields prevent a different deletion
// identity from relying on hash equality alone.
func ValidateWorkspaceRetirementFinalizationReplay(fence WorkspaceRetirementFence, token deletioncontract.WorkspaceRuntimeQuiescenceToken) error {
	fence = fence.Normalized()
	token = token.Normalized()
	if err := fence.Validate(); err != nil || !fence.IsRetired() || token.Validate() != nil {
		return common.ErrConflict
	}
	if fence.WorkspaceID != token.WorkspaceID || fence.DeletionID != token.DeletionID ||
		fence.DeletionGeneration != token.DeletionGeneration || fence.DataDispositionIntent != token.DataDispositionIntent ||
		fence.WorkspaceIntentHash != token.WorkspaceIntentHash || fence.FenceRevision != token.RunDrainingRevision+1 ||
		fence.WorkspaceRunGeneration != token.WorkspaceRunGeneration || fence.QuiescenceID != token.QuiescenceID ||
		fence.RuntimeInventoryHash != token.RuntimeInventoryHash {
		return common.ErrConflict
	}
	return nil
}

// WorkspaceRetirementEligible is the Run-owner persistence invariant used by
// the private retirement reducer and by the repository commit fence. It never
// selects a terminal transition: callers must first let every Run and every
// subordinate replacement transaction reach its existing terminal state.
func WorkspaceRetirementEligible(items []*Run, workspaceID common.WorkspaceID) error {
	if workspaceID == "" {
		return common.ErrInvalidArgument
	}
	for _, item := range items {
		if item == nil || item.WorkspaceID != workspaceID {
			return fmt.Errorf("%w: Run Workspace retirement inventory scope mismatch", common.ErrConflict)
		}
		if item.ReplacementIntent != nil && !item.ReplacementIntent.ValidFor(item) {
			return fmt.Errorf("%w: Workspace has invalid replacement intent on Run %s", common.ErrConflict, item.ID)
		}
		if !workspaceRetirementTerminalRunState(item.State) {
			return fmt.Errorf("%w: Workspace has non-terminal Run %s", ErrWorkspaceRetirementBusy, item.ID)
		}
		if item.ReplacementIntent != nil {
			switch item.ReplacementIntent.State {
			case ReplacementIntentCompleted, ReplacementIntentClarificationResolutionRejected:
			default:
				return fmt.Errorf("%w: Workspace has open replacement intent on Run %s", ErrWorkspaceRetirementBusy, item.ID)
			}
		}
	}
	return nil
}

func workspaceRetirementTerminalRunState(state common.RunState) bool {
	switch state {
	case common.RunStateCompleted, common.RunStateFailed, common.RunStateCancelled, common.RunStateSuperseded:
		return true
	default:
		return false
	}
}
