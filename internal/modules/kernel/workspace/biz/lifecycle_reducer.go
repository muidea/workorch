package workspace

import (
	"fmt"

	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// workspaceProjectionTransition is the owner-local decision for ActiveRunID /
// Workspace.State. External inputs never supply those fields as targets.
type workspaceProjectionTransition struct {
	ActiveRunID      common.RunID
	State            common.WorkspaceState
	RunProjection    *workspacemodel.WorkspaceRunProjectionCursor
	ClearActive      bool
	TerminalDelivery *runtimecontract.RuntimeTerminalDeliveryIdentity
	Noop             bool
	Diagnostic       string
}

func reduceRunProjectionFact(current *workspacemodel.Workspace, cmd workspaceRunProjectionFact) (workspaceProjectionTransition, error) {
	if current == nil {
		return workspaceProjectionTransition{}, common.ErrInvalidArgument
	}
	if err := cmd.validate(); err != nil {
		return workspaceProjectionTransition{}, err
	}
	if current.ID != cmd.WorkspaceID {
		return workspaceProjectionTransition{}, common.ErrConflict
	}
	if current.Deletion != nil {
		if err := current.Deletion.Validate(current.ID); err != nil {
			return workspaceProjectionTransition{}, common.ErrConflict
		}
		if current.Deletion.Phase != workspacemodel.WorkspaceDeletionPhaseWaitingForIdle {
			return workspaceProjectionTransition{}, fmt.Errorf("%w: Workspace deletion fence rejects non-terminal Run projection", common.ErrConflict)
		}
	}

	desiredState := workspaceStateForRunState(cmd.RunState)
	if desiredState == "" {
		return workspaceProjectionTransition{}, fmt.Errorf("%w: unsupported run projection state %q", common.ErrInvalidArgument, cmd.RunState)
	}

	cursor := current.RunProjectionCursor
	if err := validateWorkspaceRunProjectionCursor(current.ID, cursor); err != nil {
		return workspaceProjectionTransition{}, err
	}
	if err := validateWorkspaceTerminalProjectionFence(current); err != nil {
		return workspaceProjectionTransition{}, err
	}
	if cursor.RunID == "" {
		if current.ActiveRunID != "" || current.TerminalDelivery != nil {
			return workspaceProjectionTransition{}, fmt.Errorf("%w: initial Run generation requires an empty Workspace slot", common.ErrConflict)
		}
	} else if cmd.WorkspaceRunGeneration < cursor.WorkspaceRunGeneration {
		return workspaceProjectionTransition{Noop: true, Diagnostic: "stale Run generation"}, nil
	} else if cmd.WorkspaceRunGeneration == cursor.WorkspaceRunGeneration {
		if cursor.RunID != cmd.RunID {
			return workspaceProjectionTransition{}, fmt.Errorf("%w: one Workspace Run generation names multiple Runs", common.ErrConflict)
		}
		if current.TerminalDelivery != nil {
			return workspaceProjectionTransition{Noop: true, Diagnostic: "run projection retired by terminal delivery"}, nil
		}
		lowerRunRevision := cmd.RunRevision < cursor.RunRevision
		lowerRuntimeRevision := cmd.RuntimeStateRevision < cursor.RuntimeStateRevision
		higherRunRevision := cmd.RunRevision > cursor.RunRevision
		higherRuntimeRevision := cmd.RuntimeStateRevision > cursor.RuntimeStateRevision
		if (lowerRunRevision && higherRuntimeRevision) || (higherRunRevision && lowerRuntimeRevision) {
			return workspaceProjectionTransition{}, fmt.Errorf("%w: Run fact revisions are not monotonically ordered", common.ErrConflict)
		}
		if lowerRunRevision || lowerRuntimeRevision {
			return workspaceProjectionTransition{Noop: true, Diagnostic: "stale run projection fact"}, nil
		}
		if cmd.RunRevision == cursor.RunRevision {
			if cmd.RuntimeStateRevision != cursor.RuntimeStateRevision || cmd.RuntimeStateRef != cursor.RuntimeStateRef || cmd.CausationID != cursor.CausationID {
				return workspaceProjectionTransition{}, fmt.Errorf("%w: conflicting Run fact at persisted projection revision", common.ErrConflict)
			}
			if current.ActiveRunID == cmd.RunID && common.CanonicalWorkspaceOwnerState(current.State) == desiredState {
				return workspaceProjectionTransition{Noop: true, Diagnostic: "workspace projection already converged"}, nil
			}
			return workspaceProjectionTransition{}, fmt.Errorf("%w: matching Run fact disagrees with Workspace projection", common.ErrConflict)
		}
		if current.ActiveRunID != cmd.RunID {
			return workspaceProjectionTransition{}, fmt.Errorf("%w: current Run generation does not own the active slot", common.ErrConflict)
		}
	} else {
		if cursor.RunID == cmd.RunID {
			return workspaceProjectionTransition{}, fmt.Errorf("%w: one Run identity carries multiple Workspace generations", common.ErrConflict)
		}
		if current.ActiveRunID != "" || current.TerminalDelivery == nil ||
			current.TerminalDelivery.RunID != cursor.RunID {
			return workspaceProjectionTransition{}, fmt.Errorf("%w: newer Run generation lacks prior terminal release fence", common.ErrConflict)
		}
	}

	// Another live run owns the slot: do not steal.
	if current.ActiveRunID != "" && current.ActiveRunID != cmd.RunID {
		return workspaceProjectionTransition{}, fmt.Errorf("%w: workspace active run slot owned by %s", common.ErrConflict, current.ActiveRunID)
	}

	nextCursor := workspacemodel.WorkspaceRunProjectionCursor{
		WorkspaceRunGeneration: cmd.WorkspaceRunGeneration,
		RunID:                  cmd.RunID, RunRevision: cmd.RunRevision, RuntimeStateRevision: cmd.RuntimeStateRevision,
		RuntimeStateRef: cmd.RuntimeStateRef, CausationID: cmd.CausationID,
	}
	return workspaceProjectionTransition{
		ActiveRunID:   cmd.RunID,
		State:         desiredState,
		RunProjection: &nextCursor,
	}, nil
}

func validateWorkspaceRunProjectionCursor(workspaceID common.WorkspaceID, cursor workspacemodel.WorkspaceRunProjectionCursor) error {
	if cursor.RunID == "" {
		if cursor.WorkspaceRunGeneration != 0 || cursor.RunRevision != 0 || cursor.RuntimeStateRevision != 0 || cursor.RuntimeStateRef != "" || cursor.CausationID != "" {
			return fmt.Errorf("%w: Workspace Run projection cursor is partially populated", common.ErrConflict)
		}
		return nil
	}
	if cursor.WorkspaceRunGeneration == 0 || cursor.RunRevision == 0 || cursor.CausationID == "" {
		return fmt.Errorf("%w: Workspace Run projection cursor lacks owner revision identity", common.ErrConflict)
	}
	if cursor.CausationID != runevents.RunStateChangedCausationID(cursor.RunID, cursor.WorkspaceRunGeneration, cursor.RunRevision, cursor.RuntimeStateRevision) {
		return fmt.Errorf("%w: Workspace Run projection cursor causation mismatch", common.ErrConflict)
	}
	if cursor.RuntimeStateRevision == 0 && cursor.RuntimeStateRef != "" {
		return fmt.Errorf("%w: pre-runtime Workspace Run projection cursor carries Runtime ref", common.ErrConflict)
	}
	if cursor.RuntimeStateRevision > 0 && cursor.RuntimeStateRef == "" {
		return fmt.Errorf("%w: Workspace Run projection cursor lacks Runtime ref", common.ErrConflict)
	}
	if cursor.RuntimeStateRevision > 0 && cursor.RuntimeStateRef != runevents.RuntimeStateRef(workspaceID, cursor.RunID, cursor.RuntimeStateRevision) {
		return fmt.Errorf("%w: Workspace Run projection cursor Runtime ref mismatch", common.ErrConflict)
	}
	return nil
}

func validateWorkspaceTerminalProjectionFence(current *workspacemodel.Workspace) error {
	if current == nil {
		return common.ErrInvalidArgument
	}
	if current.TerminalDelivery == nil {
		return nil
	}
	cursor := current.RunProjectionCursor
	if current.ActiveRunID != "" || cursor.WorkspaceRunGeneration == 0 ||
		cursor.RunID != current.TerminalDelivery.RunID ||
		cursor.CausationID == "" {
		return fmt.Errorf("%w: Workspace terminal delivery generation fence is inconsistent", common.ErrConflict)
	}
	return nil
}

func reduceWorkspaceTerminalDelivery(current *workspacemodel.Workspace, cmd workspaceevents.RuntimeTerminalDeliveryFact) (workspaceProjectionTransition, error) {
	if current == nil {
		return workspaceProjectionTransition{}, common.ErrInvalidArgument
	}
	if err := cmd.Validate(); err != nil {
		return workspaceProjectionTransition{}, err
	}
	if current.ID != cmd.WorkspaceID {
		return workspaceProjectionTransition{}, common.ErrConflict
	}
	if err := validateWorkspaceRunProjectionCursor(current.ID, current.RunProjectionCursor); err != nil {
		return workspaceProjectionTransition{}, err
	}
	if err := validateWorkspaceTerminalProjectionFence(current); err != nil {
		return workspaceProjectionTransition{}, err
	}
	identity := cmd.Identity().Normalized()
	if current.TerminalDelivery != nil {
		if sameWorkspaceTerminalDelivery(current.TerminalDelivery, &identity) {
			return workspaceProjectionTransition{Noop: true, Diagnostic: "workspace terminal delivery already applied"}, nil
		}
		return workspaceProjectionTransition{}, fmt.Errorf("%w: conflicting workspace terminal delivery", common.ErrConflict)
	}

	desiredState := workspaceStateForTerminal(cmd)
	if current.ActiveRunID == "" {
		return workspaceProjectionTransition{}, fmt.Errorf("%w: workspace has no active run for terminal delivery", common.ErrConflict)
	}
	if current.ActiveRunID != cmd.RunID {
		return workspaceProjectionTransition{}, fmt.Errorf("%w: workspace active run owned by %s", common.ErrConflict, current.ActiveRunID)
	}
	if current.RunProjectionCursor.RunID != cmd.RunID || current.RunProjectionCursor.WorkspaceRunGeneration == 0 {
		return workspaceProjectionTransition{}, fmt.Errorf("%w: terminal delivery lacks current Run generation projection", common.ErrConflict)
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

func workspaceStateForTerminal(cmd workspaceevents.RuntimeTerminalDeliveryFact) common.WorkspaceState {
	cmd = cmd.Normalized()
	if cmd.Outcome.Kind == runtimecontract.TerminalOutcomeKindSuperseded {
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
	if transition.RunProjection != nil {
		next.RunProjectionCursor = *transition.RunProjection
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
		l.EvidenceRevision == r.EvidenceRevision && l.DispatchID == r.DispatchID && l.Outcome == r.Outcome
}
