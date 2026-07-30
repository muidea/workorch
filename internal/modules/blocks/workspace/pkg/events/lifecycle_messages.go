package events

import (
	"fmt"
	"strings"

	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// RUNTIME-SMO workspace projection contracts.
//
// Workspace owner alone mutates ActiveRunID / Workspace.State. Inputs are
// immutable facts about a Run; they never carry a caller-chosen target state.

// ApplyRunProjectionFactCommand asks Workspace to converge its projection from
// an already-persisted Run state fact (active/blocked).
//
// Publisher: blocks/run (via RunStateChanged observation) or Orchestrator only
// as a typed fact forwarder — never as a model update with target State.
type ApplyRunProjectionFactCommand struct {
	WorkspaceID common.WorkspaceID
	RunID       common.RunID
	SourceOwner string
	CausationID string
	RunState    common.RunState
	// RuntimeStateRevision is zero only for the Run owner's initial
	// activation fact.  That fact is authoritative for the SM-1 transition,
	// but necessarily precedes SM-2 aggregate initialization.
	RuntimeStateRevision uint64
	RuntimeStateRef      string
}

func (c ApplyRunProjectionFactCommand) Normalized() ApplyRunProjectionFactCommand {
	c.SourceOwner = strings.TrimSpace(c.SourceOwner)
	c.CausationID = strings.TrimSpace(c.CausationID)
	c.RuntimeStateRef = strings.TrimSpace(c.RuntimeStateRef)
	return c
}

func (c ApplyRunProjectionFactCommand) Validate() error {
	c = c.Normalized()
	if c.WorkspaceID == "" || c.RunID == "" || c.SourceOwner != "blocks/run" || c.CausationID == "" {
		return fmt.Errorf("%w: run projection fact requires Run owner scope and causation", common.ErrInvalidArgument)
	}
	switch c.RunState {
	case common.RunStateActive, common.RunStateBlockedOnDecision, common.RunStateBlockedOnApproval, common.RunStatePending:
	default:
		return fmt.Errorf("%w: run projection fact has unsupported run state %q", common.ErrInvalidArgument, c.RunState)
	}
	if c.RuntimeStateRevision == 0 {
		// A first pending/active projection cannot carry an authority snapshot:
		// SM-2 is initialized only after the Run owner has committed active.
		// Keep this exception explicit and narrow so it cannot become a generic
		// partial-identity path for governance states.
		if c.RunState != common.RunStatePending && c.RunState != common.RunStateActive {
			return fmt.Errorf("%w: pre-runtime projection only permits pending or active Run", common.ErrInvalidArgument)
		}
		if c.RuntimeStateRef != "" {
			return fmt.Errorf("%w: pre-runtime projection must not mix runtime state ref", common.ErrInvalidArgument)
		}
		return nil
	}
	if c.RuntimeStateRef == "" {
		return fmt.Errorf("%w: runtime projection fact requires opaque Run runtime ref", common.ErrInvalidArgument)
	}
	return nil
}

// ApplyRuntimeTerminalDeliveryCommand asks Workspace to release ActiveRun for a
// sealed terminal delivery. Workspace never invents a new terminal.
type ApplyRuntimeTerminalDeliveryCommand struct {
	WorkspaceID         common.WorkspaceID
	RunID               common.RunID
	SourceOwner         string
	CausationID         string
	TerminalDeliveryID  string
	AuthoritySnapshotID string
	DefinitionRevision  string
	EvidenceRevision    string
	DispatchID          string
	Outcome             runtimecontract.TerminalOutcome
}

func (c ApplyRuntimeTerminalDeliveryCommand) Normalized() ApplyRuntimeTerminalDeliveryCommand {
	c.SourceOwner = strings.TrimSpace(c.SourceOwner)
	c.CausationID = strings.TrimSpace(c.CausationID)
	c.TerminalDeliveryID = strings.TrimSpace(c.TerminalDeliveryID)
	c.AuthoritySnapshotID = strings.TrimSpace(c.AuthoritySnapshotID)
	c.DefinitionRevision = strings.TrimSpace(c.DefinitionRevision)
	c.EvidenceRevision = strings.TrimSpace(c.EvidenceRevision)
	c.DispatchID = strings.TrimSpace(c.DispatchID)
	c.Outcome = c.Outcome.Normalized()
	return c
}

func (c ApplyRuntimeTerminalDeliveryCommand) Validate() error {
	c = c.Normalized()
	if c.WorkspaceID == "" || c.RunID == "" || c.SourceOwner != "kernel/orchestrator" || c.CausationID == "" {
		return fmt.Errorf("%w: workspace terminal delivery requires workspace, run, source and causation", common.ErrInvalidArgument)
	}
	if err := c.Identity().Validate(); err != nil {
		return fmt.Errorf("%w: workspace terminal delivery identity: %v", common.ErrInvalidArgument, err)
	}
	return nil
}

func (c ApplyRuntimeTerminalDeliveryCommand) Identity() runtimecontract.RuntimeTerminalDeliveryIdentity {
	c = c.Normalized()
	return runtimecontract.RuntimeTerminalDeliveryIdentity{
		TerminalDeliveryID:  c.TerminalDeliveryID,
		WorkspaceID:         c.WorkspaceID,
		RunID:               c.RunID,
		AuthoritySnapshotID: c.AuthoritySnapshotID,
		DefinitionRevision:  c.DefinitionRevision,
		EvidenceRevision:    c.EvidenceRevision,
		DispatchID:          c.DispatchID,
		Outcome:             c.Outcome,
	}
}

// WorkspaceLifecycleCommandResult is the synchronous result of a projection or
// terminal-delivery command. DeliveryAck is set only for terminal delivery.
type WorkspaceLifecycleCommandResult struct {
	Workspace    *workspacemodel.Workspace
	Transitioned bool
	DeliveryAck  *runtimecontract.RuntimeDeliveryAcknowledgement
	Diagnostic   string
}
