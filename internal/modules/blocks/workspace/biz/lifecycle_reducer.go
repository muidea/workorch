package workspace

import (
	"fmt"

	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// workspaceProjectionTransition is the owner-local decision for ActiveRunID /
// Workspace.State. External inputs never supply those fields as targets.
type workspaceProjectionTransition struct {
	ActiveRunID      common.RunID
	State            common.WorkspaceState
	ClearActive      bool
	TerminalDelivery *runtimecontract.RuntimeTerminalDeliveryIdentity
	Noop             bool
	Diagnostic       string
}

func reduceRunProjectionFact(current *workspacemodel.Workspace, cmd workspaceevents.ApplyRunProjectionFactCommand) (workspaceProjectionTransition, error) {
	if current == nil {
		return workspaceProjectionTransition{}, common.ErrInvalidArgument
	}
	if err := cmd.Validate(); err != nil {
		return workspaceProjectionTransition{}, err
	}
	if current.ID != cmd.WorkspaceID {
		return workspaceProjectionTransition{}, common.ErrConflict
	}

	desiredState := workspaceStateForRunState(cmd.RunState)
	if desiredState == "" {
		return workspaceProjectionTransition{}, fmt.Errorf("%w: unsupported run projection state %q", common.ErrInvalidArgument, cmd.RunState)
	}

	// Already converged for this live run.
	if current.ActiveRunID == cmd.RunID && common.CanonicalWorkspaceOwnerState(current.State) == desiredState {
		return workspaceProjectionTransition{Noop: true, Diagnostic: "workspace projection already converged"}, nil
	}

	// Another live run owns the slot: do not steal.
	if current.ActiveRunID != "" && current.ActiveRunID != cmd.RunID {
		return workspaceProjectionTransition{}, fmt.Errorf("%w: workspace active run slot owned by %s", common.ErrConflict, current.ActiveRunID)
	}

	return workspaceProjectionTransition{
		ActiveRunID: cmd.RunID,
		State:       desiredState,
	}, nil
}

func reduceWorkspaceTerminalDelivery(current *workspacemodel.Workspace, cmd workspaceevents.ApplyRuntimeTerminalDeliveryCommand) (workspaceProjectionTransition, error) {
	if current == nil {
		return workspaceProjectionTransition{}, common.ErrInvalidArgument
	}
	if err := cmd.Validate(); err != nil {
		return workspaceProjectionTransition{}, err
	}
	if current.ID != cmd.WorkspaceID {
		return workspaceProjectionTransition{}, common.ErrConflict
	}
	identity := cmd.Identity().Normalized()
	if current.TerminalDelivery != nil {
		if sameWorkspaceTerminalDelivery(current.TerminalDelivery, &identity) {
			return workspaceProjectionTransition{Noop: true, Diagnostic: "workspace terminal delivery already applied"}, nil
		}
		if current.ActiveRunID == "" {
			return workspaceProjectionTransition{}, fmt.Errorf("%w: conflicting workspace terminal delivery", common.ErrConflict)
		}
	}

	desiredState := workspaceStateForTerminal(cmd)
	// An empty slot can persist the first release fence after a recovery crash,
	// but it cannot accept an unrelated terminal identity.
	if current.ActiveRunID == "" {
		return workspaceProjectionTransition{
			ClearActive:      true,
			State:            desiredState,
			TerminalDelivery: &identity,
		}, nil
	}
	if current.ActiveRunID != cmd.RunID {
		return workspaceProjectionTransition{}, fmt.Errorf("%w: workspace active run owned by %s", common.ErrConflict, current.ActiveRunID)
	}
	return workspaceProjectionTransition{
		ClearActive:      true,
		State:            desiredState,
		TerminalDelivery: &identity,
	}, nil
}

func workspaceStateForRunState(state common.RunState) common.WorkspaceState {
	switch state {
	case common.RunStatePending, common.RunStateActive, common.RunStateBlockedOnDecision:
		return common.WorkspaceStateActive
	case common.RunStateBlockedOnApproval:
		return common.WorkspaceStateBlockedOnApproval
	default:
		return ""
	}
}

func workspaceStateForTerminal(cmd workspaceevents.ApplyRuntimeTerminalDeliveryCommand) common.WorkspaceState {
	cmd = cmd.Normalized()
	if cmd.AdministrativeKind == runtimecontract.RuntimeTerminationSuperseded {
		return common.WorkspaceStateSuperseded
	}
	return common.WorkspaceStateActive
}

func applyWorkspaceProjection(current *workspacemodel.Workspace, transition workspaceProjectionTransition) *workspacemodel.Workspace {
	if current == nil {
		return nil
	}
	next := *current
	if transition.ClearActive {
		next.ActiveRunID = ""
	} else {
		next.ActiveRunID = transition.ActiveRunID
		if transition.ActiveRunID != "" {
			next.TerminalDelivery = nil
		}
	}
	if transition.TerminalDelivery != nil {
		identity := transition.TerminalDelivery.Normalized()
		next.TerminalDelivery = &identity
	}
	next.State = common.CanonicalWorkspaceOwnerState(transition.State)
	return &next
}

func sameWorkspaceTerminalDelivery(left, right *runtimecontract.RuntimeTerminalDeliveryIdentity) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	l, r := left.Normalized(), right.Normalized()
	return l.TerminalDeliveryID == r.TerminalDeliveryID && l.WorkspaceID == r.WorkspaceID && l.RunID == r.RunID &&
		l.AuthoritySnapshotID == r.AuthoritySnapshotID && l.DefinitionRevision == r.DefinitionRevision &&
		l.EvidenceRevision == r.EvidenceRevision && l.DispatchID == r.DispatchID && l.Action == r.Action &&
		l.TerminalCode == r.TerminalCode && l.TerminalSummary == r.TerminalSummary
}
