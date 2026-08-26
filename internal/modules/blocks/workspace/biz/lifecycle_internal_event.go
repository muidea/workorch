package workspace

import (
	"context"
	"fmt"
	"strings"

	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
)

type workspaceLifecycleInternalEventKind string

const (
	workspaceLifecycleInternalEventTerminalDeliveryObserved workspaceLifecycleInternalEventKind = "terminal_delivery_observed"
	workspaceLifecycleInternalEventRunProjectionObserved    workspaceLifecycleInternalEventKind = "run_projection_observed"
)

type workspaceLifecycleInternalEvent struct {
	Kind             workspaceLifecycleInternalEventKind
	TerminalDelivery *workspaceevents.RuntimeTerminalDeliveryFact
	RunProjection    *workspaceRunProjectionFact
}

// workspaceRunProjectionFact is the Workspace owner's private reducer input.
// The only cross-owner contract is RunStateChangedData; the Run observation
// handler validates that owner fact and translates it into this private shape.
type workspaceRunProjectionFact struct {
	WorkspaceID            common.WorkspaceID
	WorkspaceRunGeneration uint64
	RunID                  common.RunID
	RunRevision            uint64
	SourceOwner            string
	CausationID            string
	RunState               common.RunState
	// RuntimeStateRevision is zero only for the Run owner's initial
	// activation fact. That fact necessarily precedes SM-2 initialization.
	RuntimeStateRevision uint64
	RuntimeStateRef      string
}

func (f workspaceRunProjectionFact) normalized() workspaceRunProjectionFact {
	f.SourceOwner = strings.TrimSpace(f.SourceOwner)
	f.CausationID = strings.TrimSpace(f.CausationID)
	f.RuntimeStateRef = strings.TrimSpace(f.RuntimeStateRef)
	return f
}

func (f workspaceRunProjectionFact) validate() error {
	f = f.normalized()
	if f.WorkspaceID == "" || f.WorkspaceRunGeneration == 0 || f.RunID == "" || f.RunRevision == 0 || f.SourceOwner != runevents.CommandDestination || f.CausationID == "" {
		return fmt.Errorf("%w: run projection fact requires Run owner scope and causation", common.ErrInvalidArgument)
	}
	if f.CausationID != runevents.RunStateChangedCausationID(f.RunID, f.WorkspaceRunGeneration, f.RunRevision, f.RuntimeStateRevision) {
		return fmt.Errorf("%w: run projection fact causation does not match owner revisions", common.ErrInvalidArgument)
	}
	switch f.RunState {
	case common.RunStateActive, common.RunStateBlockedOnDecision, common.RunStateBlockedOnApproval, common.RunStatePending:
	default:
		return fmt.Errorf("%w: run projection fact has unsupported run state %q", common.ErrInvalidArgument, f.RunState)
	}
	if f.RuntimeStateRevision == 0 {
		if f.RunState != common.RunStatePending && f.RunState != common.RunStateActive {
			return fmt.Errorf("%w: pre-runtime projection only permits pending or active Run", common.ErrInvalidArgument)
		}
		if f.RuntimeStateRef != "" {
			return fmt.Errorf("%w: pre-runtime projection must not mix runtime state ref", common.ErrInvalidArgument)
		}
		return nil
	}
	if f.RuntimeStateRef == "" || f.RuntimeStateRef != runevents.RuntimeStateRef(f.WorkspaceID, f.RunID, f.RuntimeStateRevision) {
		return fmt.Errorf("%w: runtime projection fact requires opaque Run runtime ref", common.ErrInvalidArgument)
	}
	return nil
}

// submitWorkspaceLifecycleInternalEvent is the only owner-local dispatcher
// allowed to select a Workspace lifecycle reducer. Cross-owner handlers first
// validate an intent/fact and then translate it into one of these private
// events; callers cannot submit a target Workspace state.
func (m *Biz) submitWorkspaceLifecycleInternalEvent(ctx context.Context, event workspaceLifecycleInternalEvent) (workspaceevents.WorkspaceLifecycleCommandResult, error) {
	if m == nil || m.workspaces == nil {
		return workspaceevents.WorkspaceLifecycleCommandResult{}, common.ErrInvalidArgument
	}
	switch event.Kind {
	case workspaceLifecycleInternalEventTerminalDeliveryObserved:
		if event.TerminalDelivery == nil || event.RunProjection != nil {
			return workspaceevents.WorkspaceLifecycleCommandResult{}, common.ErrInvalidArgument
		}
		return m.applyTerminalDelivery(ctx, *event.TerminalDelivery)
	case workspaceLifecycleInternalEventRunProjectionObserved:
		if event.RunProjection == nil || event.TerminalDelivery != nil {
			return workspaceevents.WorkspaceLifecycleCommandResult{}, common.ErrInvalidArgument
		}
		return m.applyRunProjection(ctx, *event.RunProjection)
	default:
		return workspaceevents.WorkspaceLifecycleCommandResult{}, common.ErrInvalidArgument
	}
}
