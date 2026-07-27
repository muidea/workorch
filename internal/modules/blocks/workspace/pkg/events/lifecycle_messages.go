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
	WorkspaceID         common.WorkspaceID
	RunID               common.RunID
	SourceOwner         string
	CausationID         string
	RunState            common.RunState
	AuthoritySnapshotID string
	DefinitionRevision  string
	EvidenceRevision    string
	DispatchID          string
}

func (c ApplyRunProjectionFactCommand) Normalized() ApplyRunProjectionFactCommand {
	c.SourceOwner = strings.TrimSpace(c.SourceOwner)
	c.CausationID = strings.TrimSpace(c.CausationID)
	c.AuthoritySnapshotID = strings.TrimSpace(c.AuthoritySnapshotID)
	c.DefinitionRevision = strings.TrimSpace(c.DefinitionRevision)
	c.EvidenceRevision = strings.TrimSpace(c.EvidenceRevision)
	c.DispatchID = strings.TrimSpace(c.DispatchID)
	return c
}

func (c ApplyRunProjectionFactCommand) Validate() error {
	c = c.Normalized()
	if c.WorkspaceID == "" || c.RunID == "" || c.SourceOwner != "blocks/run" || c.CausationID == "" ||
		c.AuthoritySnapshotID == "" || c.DefinitionRevision == "" || c.EvidenceRevision == "" {
		return fmt.Errorf("%w: run projection fact requires sealed Run owner identity", common.ErrInvalidArgument)
	}
	switch c.RunState {
	case common.RunStateActive, common.RunStateBlockedOnDecision, common.RunStateBlockedOnApproval, common.RunStatePending:
		return nil
	default:
		return fmt.Errorf("%w: run projection fact has unsupported run state %q", common.ErrInvalidArgument, c.RunState)
	}
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
	Action              runtimecontract.RuntimeAction
	TerminalCode        string
	TerminalSummary     string
	AdministrativeKind  runtimecontract.RuntimeTerminationKind
}

func (c ApplyRuntimeTerminalDeliveryCommand) Normalized() ApplyRuntimeTerminalDeliveryCommand {
	c.SourceOwner = strings.TrimSpace(c.SourceOwner)
	c.CausationID = strings.TrimSpace(c.CausationID)
	c.TerminalDeliveryID = strings.TrimSpace(c.TerminalDeliveryID)
	c.AuthoritySnapshotID = strings.TrimSpace(c.AuthoritySnapshotID)
	c.DefinitionRevision = strings.TrimSpace(c.DefinitionRevision)
	c.EvidenceRevision = strings.TrimSpace(c.EvidenceRevision)
	c.DispatchID = strings.TrimSpace(c.DispatchID)
	c.Action = runtimecontract.RuntimeAction(strings.TrimSpace(string(c.Action)))
	c.TerminalCode = strings.TrimSpace(c.TerminalCode)
	c.TerminalSummary = strings.TrimSpace(c.TerminalSummary)
	c.AdministrativeKind = runtimecontract.RuntimeTerminationKind(strings.TrimSpace(string(c.AdministrativeKind)))
	return c
}

func (c ApplyRuntimeTerminalDeliveryCommand) Validate() error {
	c = c.Normalized()
	if c.WorkspaceID == "" || c.RunID == "" || c.SourceOwner == "" {
		return fmt.Errorf("%w: workspace terminal delivery requires workspace, run and source", common.ErrInvalidArgument)
	}
	if c.TerminalDeliveryID == "" || c.AuthoritySnapshotID == "" {
		return fmt.Errorf("%w: workspace terminal delivery requires delivery id and authority snapshot", common.ErrInvalidArgument)
	}
	switch c.AdministrativeKind {
	case "":
		switch c.Action {
		case runtimecontract.RuntimeActionComplete, runtimecontract.RuntimeActionFail:
			if c.TerminalCode == "" || c.TerminalSummary == "" {
				return fmt.Errorf("%w: workspace terminal delivery requires terminal code and summary", common.ErrInvalidArgument)
			}
			return nil
		default:
			return fmt.Errorf("%w: workspace terminal delivery has invalid action %q", common.ErrInvalidArgument, c.Action)
		}
	case runtimecontract.RuntimeTerminationCancelled, runtimecontract.RuntimeTerminationSuperseded:
		if c.Action != runtimecontract.RuntimeActionFail || c.TerminalCode == "" || c.TerminalSummary == "" || c.CausationID == "" {
			return fmt.Errorf("%w: workspace administrative terminal delivery requires sealed failure identity", common.ErrInvalidArgument)
		}
		return nil
	default:
		return fmt.Errorf("%w: workspace terminal delivery has invalid administrative kind %q", common.ErrInvalidArgument, c.AdministrativeKind)
	}
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
		Action:              c.Action,
		TerminalCode:        c.TerminalCode,
		TerminalSummary:     c.TerminalSummary,
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
