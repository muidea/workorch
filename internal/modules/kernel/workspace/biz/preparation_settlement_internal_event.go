package workspace

import (
	"context"
	"errors"

	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

// Only the read-only original-receipt consumer constructs this private fact.
// A normal Restart response, absence, timeout or empty inventory is not proof.
type workspaceFreshIntentSettled struct {
	Command     workspaceevents.WorkspaceExecutionIntentCommand
	Observation runevents.QueryWorkspaceExecutionIntentReceiptResult
}

// Carries the already sealed owner identities, not a caller validation token.
// The private reducer still matches the entire reservation and exact receipt.
type workspaceFreshIntentSettlementFact struct {
	WorkspaceID common.WorkspaceID
	Reservation workspacemodel.WorkspaceFreshIntentReservation
	Observation runevents.QueryWorkspaceExecutionIntentReceiptResult
}

func freshIntentReceiptIdentity(cmd workspaceevents.WorkspaceExecutionIntentCommand) (workspacemodel.WorkspaceFreshIntentReservation, runevents.QueryWorkspaceExecutionIntentReceiptCommand, error) {
	// The revision is not part of payload identity. Recovery must resolve the
	// original before consulting today's definition or activation admission.
	fact, err := workspaceFreshIntentReservationFact(cmd, 1)
	if err != nil {
		return workspacemodel.WorkspaceFreshIntentReservation{}, runevents.QueryWorkspaceExecutionIntentReceiptCommand{}, err
	}
	return fact.Reservation, originalFreshReceiptQuery(cmd.WorkspaceID, fact.Reservation), nil
}

func originalFreshReceiptQuery(workspaceID common.WorkspaceID, r workspacemodel.WorkspaceFreshIntentReservation) runevents.QueryWorkspaceExecutionIntentReceiptCommand {
	return runevents.QueryWorkspaceExecutionIntentReceiptCommand{WorkspaceID: workspaceID, RequestID: r.RequestID, PayloadHash: r.RunPayloadHash}
}

// lookupFreshIntentOriginal is pure and does not consult mutable admission.
// Even after another request has been reserved, a settled original remains
// replayable by its exact payload. An unknown identity never grants a forward.
func lookupFreshIntentOriginal(current *workspacemodel.Workspace, reservation workspacemodel.WorkspaceFreshIntentReservation, query runevents.QueryWorkspaceExecutionIntentReceiptCommand) (*runmodel.WorkspaceExecutionIntentReceipt, error) {
	if current == nil || reservation.Validate() != nil || query.Validate() != nil || current.ID != query.WorkspaceID || query.RequestID != reservation.RequestID || query.PayloadHash != reservation.RunPayloadHash || current.PreparationFence == nil || current.PreparationFence.Validate(current.ID) != nil {
		return nil, common.ErrConflict
	}
	for _, settled := range current.PreparationFence.Settled {
		if settled.Reservation.RequestID == reservation.RequestID {
			if !sameWorkspaceFreshIntentInput(settled.Reservation, reservation) || settled.Receipt.PayloadHash != query.PayloadHash {
				return nil, common.ErrConflict
			}
			receipt := settled.Receipt
			return &receipt, nil
		}
	}
	if current.PreparationFence.Pending == nil || !sameWorkspaceFreshIntentInput(*current.PreparationFence.Pending, reservation) {
		return nil, common.ErrConflict
	}
	return nil, nil
}

func reduceWorkspaceFreshIntentSettled(current *workspacemodel.Workspace, fact workspaceFreshIntentSettled) (workspaceExecutionTransition, error) {
	reservation, _, err := freshIntentReceiptIdentity(fact.Command)
	if err != nil {
		return workspaceExecutionTransition{}, err
	}
	return reduceWorkspaceFreshIntentSettlement(current, workspaceFreshIntentSettlementFact{fact.Command.WorkspaceID, reservation, fact.Observation})
}

func reduceWorkspaceFreshIntentSettlement(current *workspacemodel.Workspace, fact workspaceFreshIntentSettlementFact) (workspaceExecutionTransition, error) {
	reservation := fact.Reservation
	query := originalFreshReceiptQuery(fact.WorkspaceID, reservation)
	if current == nil || current.ID != fact.WorkspaceID || fact.Observation.ValidateFor(query) != nil || fact.Observation.Receipt == nil {
		return workspaceExecutionTransition{}, common.ErrConflict
	}
	stored, err := lookupFreshIntentOriginal(current, reservation, query)
	if err != nil {
		return workspaceExecutionTransition{}, err
	}
	if stored != nil {
		if *stored != *fact.Observation.Receipt {
			return workspaceExecutionTransition{}, common.ErrConflict
		}
		return workspaceExecutionTransition{Next: current, Noop: true}, nil
	}
	next := *current
	next.PreparationFence = current.PreparationFence.Clone()
	next.PreparationFence.Settled = append(next.PreparationFence.Settled, workspacemodel.WorkspaceFreshIntentSettlement{
		Reservation: *current.PreparationFence.Pending, Receipt: *fact.Observation.Receipt,
	})
	next.PreparationFence.Pending = nil
	if err := workspacemodel.ValidatePreparationFenceTransition(current, &next); err != nil {
		return workspaceExecutionTransition{}, err
	}
	return workspaceExecutionTransition{Next: &next}, nil
}

func (m *Biz) submitWorkspaceFreshIntentSettled(ctx context.Context, fact workspaceFreshIntentSettled) (*workspacemodel.Workspace, error) {
	reservation, _, err := freshIntentReceiptIdentity(fact.Command)
	if err != nil {
		return nil, err
	}
	return m.submitWorkspaceFreshIntentSettlement(ctx, workspaceFreshIntentSettlementFact{fact.Command.WorkspaceID, reservation, fact.Observation})
}

func (m *Biz) submitWorkspaceFreshIntentSettlement(ctx context.Context, fact workspaceFreshIntentSettlementFact) (*workspacemodel.Workspace, error) {
	if m == nil || m.workspaces == nil {
		return nil, common.ErrInvalidArgument
	}
	bound, err := common.BindRuntimeWorkspaceContext(ctx, fact.WorkspaceID)
	if err != nil {
		return nil, err
	}
	if err := fact.Reservation.Validate(); err != nil {
		return nil, err
	}
	fact.Observation = fact.Observation.Clone()
	for attempt := 0; attempt < 4; attempt++ {
		if err := bound.Err(); err != nil {
			return nil, err
		}
		current, err := m.workspaces.Get(bound, fact.WorkspaceID)
		if err != nil {
			return nil, err
		}
		transition, err := reduceWorkspaceFreshIntentSettlement(current, fact)
		if err != nil {
			return nil, err
		}
		if transition.Noop {
			return current, nil
		}
		if err := m.workspaces.CompareAndSwap(bound, transition.Next, current.Revision); err != nil {
			if errors.Is(err, common.ErrConflict) {
				continue
			}
			return nil, err
		}
		m.publishWorkspaceUpdated(bound, transition.Next)
		return transition.Next, nil
	}
	return nil, common.ErrConflict
}

// Reconciliation is not an execution path. Nil receipt means precisely that
// the original remains unresolved; callers must not forward again. Errors
// remain errors and also cannot release the reservation. No owner lock is held
// over the concrete Run query, and a fresh local load/CAS rechecks its identity.
func (m *Biz) reconcileWorkspaceFreshIntent(ctx context.Context, cmd workspaceevents.WorkspaceExecutionIntentCommand) (*runmodel.WorkspaceExecutionIntentReceipt, error) {
	if m == nil || m.workspaces == nil || ctx == nil {
		return nil, common.ErrInvalidArgument
	}
	reservation, query, err := freshIntentReceiptIdentity(cmd)
	if err != nil {
		return nil, err
	}
	return m.reconcileOriginalFreshIdentity(ctx, cmd.WorkspaceID, reservation, query)
}

func (m *Biz) reconcileOriginalFreshIdentity(ctx context.Context, workspaceID common.WorkspaceID, reservation workspacemodel.WorkspaceFreshIntentReservation, query runevents.QueryWorkspaceExecutionIntentReceiptCommand) (*runmodel.WorkspaceExecutionIntentReceipt, error) {
	bound, err := common.BindRuntimeWorkspaceContext(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	if err := bound.Err(); err != nil {
		return nil, err
	}
	current, err := m.workspaces.Get(bound, workspaceID)
	if err != nil {
		return nil, err
	}
	stored, err := lookupFreshIntentOriginal(current, reservation, query)
	if err != nil || stored != nil {
		return stored, err
	}
	observation, err := m.queryRunExecutionIntentReceipt(bound, query)
	if err != nil || observation.Receipt == nil {
		return nil, err
	}
	settled, err := m.submitWorkspaceFreshIntentSettlement(bound, workspaceFreshIntentSettlementFact{workspaceID, reservation, observation})
	if err != nil {
		return nil, err
	}
	return lookupFreshIntentOriginal(settled, reservation, query)
}
