package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/muidea/workorch/internal/pkg/preparationtrace"
	"time"

	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/preparationcompiler"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// This owner-private fact is not an EventHub command and cannot forward an
// execution intent. Only its successful initial CAS can identify a new reserve.
type workspaceFreshIntentReserved struct {
	WorkspaceID       common.WorkspaceID
	ExpectedRevision  uint64
	Reservation       workspacemodel.WorkspaceFreshIntentReservation
	ProviderSelection runtimecontract.ProviderInferenceSelection
	FirstPreparation  *workspaceFirstExecutionPreparation
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
		ProviderSelection: cmd.Bootstrap.Activation.ProviderSelection,
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
		if sameWorkspaceFreshIntentInput(settled.Reservation, fact.Reservation) {
			return workspaceExecutionTransition{Next: current, Noop: true}, nil
		}
		return workspaceExecutionTransition{}, common.ErrConflict
	}
	if fence.Pending != nil {
		if sameWorkspaceFreshIntentInput(*fence.Pending, fact.Reservation) {
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
	var firstEvidence *workspacemodel.WorkspaceFirstExecutionPreparationEvidence
	if requiresFirstExecutionPreparation(current) {
		p := fact.FirstPreparation
		if p == nil {
			return workspaceExecutionTransition{}, common.ErrConflict
		}
		now := time.Now().UTC()
		compiled, err := preparationcompiler.CompileObservedFirstExecutionPreparation(current, &p.Goal, &p.DoD, p.Bootstrap, now, p.Files)
		if err != nil {
			return workspaceExecutionTransition{}, err
		}
		// Reverse-prove the full Run payload, not just a self-reported bootstrap
		// hash. The original request identity is still resolved before this gate.
		runHash, err := runmodel.ExecutionIntentPayloadHash(current.ID, fact.Reservation.RequestID, p.Bootstrap)
		if err != nil || runHash != fact.Reservation.RunPayloadHash || !fact.ProviderSelection.Same(p.Bootstrap.Activation.ProviderSelection) {
			return workspaceExecutionTransition{}, common.ErrConflict
		}
		// Compilation may be expensive. Never stamp consumption with the clock
		// captured before it, or let CPU work renew the original observation.
		now = time.Now().UTC()
		if err := preparationcompiler.CheckFirstExecutionPreparationWindow(current, now, p.Files.OwnerEpoch); err != nil {
			return workspaceExecutionTransition{}, err
		}
		if now.Before(p.Files.ObservedAt) || !now.Before(p.Files.ExpiresAt) {
			return workspaceExecutionTransition{}, common.ErrConflict
		}
		fact.Reservation.PreparationCompletionID = compiled.CompletionID
		fact.Reservation.PreparationRevision = current.Revision
		fact.Reservation.PreparationConsumedAt = now
		firstEvidence = &workspacemodel.WorkspaceFirstExecutionPreparationEvidence{
			Version: workspacemodel.FirstExecutionPreparationEvidenceVersion, WorkspaceID: current.ID,
			RequestID: fact.Reservation.RequestID, Files: p.Files.Clone(),
		}
	}
	// Compile metadata against the SAME reservation preimage. A separate
	// provider-selection CAS would stale a freshly completed preparation before
	// its reservation could consume that revision. No metadata write may survive
	// a rejected/failed reservation. Original identities above remain first.
	selection, err := normalizeWorkspaceExecutionInternalEvent(workspaceExecutionInternalEvent{
		Kind:        workspaceExecutionInternalEventActivationProviderSelectionObserved,
		WorkspaceID: current.ID, StartupInputHash: fact.Reservation.StartupInputHash,
		ProviderSelection: fact.ProviderSelection,
	})
	if err != nil {
		return workspaceExecutionTransition{}, err
	}
	metadata, err := reduceWorkspaceExecutionInternalEvent(current, selection)
	if err != nil {
		return workspaceExecutionTransition{}, err
	}
	next := *metadata.Next
	if firstEvidence != nil {
		next.FirstExecutionPreparationEvidence = firstEvidence
	}
	next.PreparationFence = fence.Clone()
	next.PreparationFence.PreparationEpoch++
	reservation := fact.Reservation
	next.PreparationFence.Pending = &reservation
	return workspaceExecutionTransition{Next: &next}, nil
}

// Consumption metadata is owner-derived, not part of the incoming request
// identity. A replay never writes, rechecks or replaces the original metadata.
func sameWorkspaceFreshIntentInput(original, incoming workspacemodel.WorkspaceFreshIntentReservation) bool {
	incoming.PreparationCompletionID = original.PreparationCompletionID
	incoming.PreparationRevision = original.PreparationRevision
	incoming.PreparationConsumedAt = original.PreparationConsumedAt
	return incoming == original
}

// newlyReserved is true ONLY for the caller whose CAS returned success. An
// error (even after persistence), duplicate or reload never grants a forward
// permit. No cross-owner I/O is performed while reserving.
func (m *Biz) submitWorkspaceFreshIntentReserved(ctx context.Context, fact workspaceFreshIntentReserved) (value *workspacemodel.Workspace, newlyReserved bool, err error) {
	defer preparationtrace.Start(ctx, fact.WorkspaceID, fact.Reservation.RequestID, "first_execution_reservation", fact.ExpectedRevision)()
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
