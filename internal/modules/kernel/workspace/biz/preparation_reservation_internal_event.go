package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// This owner-private fact is not an EventHub command and cannot forward an
// execution intent. Only its successful initial CAS can identify a new reserve.
type workspaceFreshIntentReserved struct {
	WorkspaceID      common.WorkspaceID
	ExpectedRevision uint64
	Reservation      workspacemodel.WorkspaceFreshIntentReservation
}

func workspaceFreshIntentReservationFact(cmd workspaceevents.WorkspaceExecutionIntentCommand, expectedRevision uint64) (workspaceFreshIntentReserved, error) {
	if err := cmd.Validate(); err != nil {
		return workspaceFreshIntentReserved{}, err
	}
	cmd = cmd.Normalized()
	if cmd.Kind != runtimecontract.RuntimeExecutionIntentRestart || expectedRevision == 0 {
		return workspaceFreshIntentReserved{}, common.ErrInvalidArgument
	}
	// Full typed input, including scope, kind and every bootstrap field. This
	// is hashing only, never a JSON bridge between owners or a config digest.
	encoded, err := json.Marshal(struct {
		Domain string
		Intent workspaceevents.WorkspaceExecutionIntentCommand
	}{"workorch-fresh-intent-payload/v1", cmd})
	if err != nil {
		return workspaceFreshIntentReserved{}, err
	}
	digest := sha256.Sum256(encoded)
	runHash, err := runmodel.ExecutionIntentPayloadHash(cmd.WorkspaceID, cmd.RequestID, *cmd.Bootstrap)
	if err != nil {
		return workspaceFreshIntentReserved{}, err
	}
	fact := workspaceFreshIntentReserved{
		WorkspaceID: cmd.WorkspaceID, ExpectedRevision: expectedRevision,
		Reservation: workspacemodel.WorkspaceFreshIntentReservation{
			RequestID: cmd.RequestID, Kind: cmd.Kind,
			StartupInputHash: cmd.Bootstrap.Definition.Normalized().StartupInputHash,
			PayloadHash:      hex.EncodeToString(digest[:]),
			RunPayloadHash:   runHash,
		},
	}
	if err := fact.Reservation.Validate(); err != nil {
		return workspaceFreshIntentReserved{}, err
	}
	return fact, nil
}

func reduceWorkspaceFreshIntentReserved(current *workspacemodel.Workspace, fact workspaceFreshIntentReserved) (workspaceExecutionTransition, error) {
	if current == nil || current.ID != fact.WorkspaceID || fact.ExpectedRevision == 0 || fact.Reservation.Validate() != nil {
		return workspaceExecutionTransition{}, common.ErrInvalidArgument
	}
	if err := workspaceDeletionRejectsMutation(current); err != nil {
		return workspaceExecutionTransition{}, err
	}
	fence := current.PreparationFence
	if fence == nil || fence.Validate(current.ID) != nil {
		return workspaceExecutionTransition{}, fmt.Errorf("%w: legacy or invalid preparation creation fence", common.ErrConflict)
	}
	// Resolve original identity before consulting mutable admission/revision.
	// Neither a changed Definition nor a process restart permits re-forward.
	for _, settled := range fence.Settled {
		if settled.Reservation.RequestID != fact.Reservation.RequestID {
			continue
		}
		if settled.Reservation == fact.Reservation {
			return workspaceExecutionTransition{Next: current, Noop: true}, nil
		}
		return workspaceExecutionTransition{}, common.ErrConflict
	}
	if fence.Pending != nil {
		if *fence.Pending == fact.Reservation {
			return workspaceExecutionTransition{Next: current, Noop: true}, nil
		}
		return workspaceExecutionTransition{}, fmt.Errorf("%w: unresolved fresh intent %s", common.ErrConflict, fence.Pending.RequestID)
	}
	if len(fence.Settled) == workspacemodel.WorkspaceFreshIntentHistoryLimit {
		return workspaceExecutionTransition{}, fmt.Errorf("%w: fresh intent history capacity reached", common.ErrConflict)
	}
	if current.Revision != fact.ExpectedRevision || validateWorkspaceExecutionAdmissionPersistence(current) != nil || current.StartupInput == nil || current.ActivationAdmission == nil || current.ActivationAdmission.State != workspacemodel.WorkspaceActivationEligible || current.StartupInput.StartupInputHash != fact.Reservation.StartupInputHash {
		return workspaceExecutionTransition{}, common.ErrConflict
	}
	next := *current
	next.PreparationFence = fence.Clone()
	next.PreparationFence.PreparationEpoch++
	reservation := fact.Reservation
	next.PreparationFence.Pending = &reservation
	return workspaceExecutionTransition{Next: &next}, nil
}

// newlyReserved is true ONLY for the caller whose CAS returned success. An
// error (even after persistence), duplicate or reload never grants a forward
// permit. No cross-owner I/O is performed while reserving.
func (m *Biz) submitWorkspaceFreshIntentReserved(ctx context.Context, fact workspaceFreshIntentReserved) (value *workspacemodel.Workspace, newlyReserved bool, err error) {
	if m == nil || m.workspaces == nil {
		return nil, false, common.ErrInvalidArgument
	}
	bound, err := common.BindRuntimeWorkspaceContext(ctx, fact.WorkspaceID)
	if err != nil {
		return nil, false, err
	}
	for attempt := 0; attempt < 4; attempt++ {
		if err := bound.Err(); err != nil {
			return nil, false, err
		}
		current, err := m.workspaces.Get(bound, fact.WorkspaceID)
		if err != nil {
			return nil, false, err
		}
		transition, err := reduceWorkspaceFreshIntentReserved(current, fact)
		if err != nil {
			return nil, false, err
		}
		if transition.Noop {
			return current, false, nil
		}
		if err := m.workspaces.CompareAndSwap(bound, transition.Next, current.Revision); err != nil {
			if errors.Is(err, common.ErrConflict) {
				continue
			}
			return nil, false, err
		}
		m.publishWorkspaceUpdated(bound, transition.Next)
		return transition.Next, true, nil
	}
	return nil, false, common.ErrConflict
}
