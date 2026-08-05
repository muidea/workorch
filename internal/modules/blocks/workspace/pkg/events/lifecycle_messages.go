package events

import (
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// WorkspaceExecutionIntentCommand is accepted by the Workspace owner only.
// A restart carries an application-compiled immutable bootstrap; a resume
// carries no bootstrap and must continue the Workspace-selected active Run.
// The Workspace owner validates its own slot and current definition before it
// forwards a typed selection fact to the Run owner.
type WorkspaceExecutionIntentCommand struct {
	WorkspaceID common.WorkspaceID
	RequestID   string
	Kind        runtimecontract.RuntimeExecutionIntentKind
	Priority    int
	Bootstrap   *runtimecontract.RuntimeBootstrap
}

func (c WorkspaceExecutionIntentCommand) Normalized() WorkspaceExecutionIntentCommand {
	c.RequestID = strings.TrimSpace(c.RequestID)
	c.Kind = runtimecontract.RuntimeExecutionIntentKind(strings.TrimSpace(string(c.Kind)))
	if c.Bootstrap != nil {
		clone := c.Bootstrap.Clone()
		c.Bootstrap = &clone
	}
	return c
}

func (c WorkspaceExecutionIntentCommand) Validate() error {
	c = c.Normalized()
	if c.WorkspaceID == "" || c.RequestID == "" {
		return fmt.Errorf("%w: workspace execution intent requires workspace and request identity", common.ErrInvalidArgument)
	}
	switch c.Kind {
	case runtimecontract.RuntimeExecutionIntentResume:
		if c.Bootstrap != nil {
			return fmt.Errorf("%w: resume intent must not carry a replacement bootstrap", common.ErrInvalidArgument)
		}
	case runtimecontract.RuntimeExecutionIntentRestart:
		if c.Bootstrap == nil {
			return fmt.Errorf("%w: restart intent requires immutable runtime bootstrap", common.ErrInvalidArgument)
		}
		if err := c.Bootstrap.Validate(); err != nil {
			return fmt.Errorf("%w: restart bootstrap: %v", common.ErrInvalidArgument, err)
		}
	default:
		return fmt.Errorf("%w: unsupported workspace execution intent %q", common.ErrInvalidArgument, c.Kind)
	}
	return nil
}

// WorkspaceExecutionIntentResult is an owner response.  Disposition is a
// factual admission result, never a caller-selected target state.
type WorkspaceExecutionIntentResult runtimecontract.RuntimeExecutionIntentAdmission

func (r WorkspaceExecutionIntentResult) Validate() error {
	if err := (runtimecontract.RuntimeExecutionIntentAdmission(r)).Validate(); err != nil {
		return fmt.Errorf("%w: %v", common.ErrInvalidArgument, err)
	}
	return nil
}

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

// ReleaseFreshActivationRunSlotFactCommand carries a persisted Run owner's
// administrative fact for an aggregate schema the current binary cannot
// drive. It deliberately cannot name a desired Workspace state. Workspace
// releases only a matching ActiveRunID and leaves the old Run unchanged.
type ReleaseFreshActivationRunSlotFactCommand struct {
	WorkspaceID                        common.WorkspaceID
	RunID                              common.RunID
	SourceOwner                        string
	CausationID                        string
	RuntimeStateRevision               uint64
	RuntimeStateRef                    string
	RuntimeActivationDisposition       runtimecontract.RuntimeActivationDisposition
	ActualRuntimeStateSchemaVersion    uint32
	SupportedRuntimeStateSchemaVersion uint32
}

func (c ReleaseFreshActivationRunSlotFactCommand) Normalized() ReleaseFreshActivationRunSlotFactCommand {
	c.SourceOwner = strings.TrimSpace(c.SourceOwner)
	c.CausationID = strings.TrimSpace(c.CausationID)
	c.RuntimeStateRef = strings.TrimSpace(c.RuntimeStateRef)
	return c
}

func (c ReleaseFreshActivationRunSlotFactCommand) Validate() error {
	c = c.Normalized()
	if c.WorkspaceID == "" || c.RunID == "" || c.SourceOwner != "blocks/run" || c.CausationID == "" {
		return fmt.Errorf("%w: fresh activation release fact requires Run owner scope and causation", common.ErrInvalidArgument)
	}
	if c.RuntimeActivationDisposition != runtimecontract.RuntimeActivationDispositionFreshActivationRequired {
		return fmt.Errorf("%w: fresh activation release requires non-resumable Run disposition", common.ErrInvalidArgument)
	}
	if c.RuntimeStateRevision == 0 || c.RuntimeStateRef == "" ||
		c.ActualRuntimeStateSchemaVersion == 0 || c.SupportedRuntimeStateSchemaVersion == 0 {
		return fmt.Errorf("%w: fresh activation release requires persisted runtime identity and schema versions", common.ErrInvalidArgument)
	}
	if c.ActualRuntimeStateSchemaVersion == c.SupportedRuntimeStateSchemaVersion {
		return fmt.Errorf("%w: fresh activation release requires incompatible runtime schema", common.ErrInvalidArgument)
	}
	return nil
}

// ApplyRuntimeTerminalDeliveryCommand asks Workspace to release ActiveRun for a
// sealed terminal delivery. Workspace never invents a new terminal.
type ApplyRuntimeTerminalDeliveryCommand struct {
	runtimecontract.RuntimeTerminalDeliveryFact
}

func (c ApplyRuntimeTerminalDeliveryCommand) Normalized() ApplyRuntimeTerminalDeliveryCommand {
	c.RuntimeTerminalDeliveryFact = c.RuntimeTerminalDeliveryFact.Normalized()
	return c
}

func (c ApplyRuntimeTerminalDeliveryCommand) Validate() error {
	c = c.Normalized()
	if c.SourceOwner != "kernel/orchestrator" {
		return fmt.Errorf("%w: workspace terminal delivery requires workspace, run, source and causation", common.ErrInvalidArgument)
	}
	if err := c.RuntimeTerminalDeliveryFact.Validate(); err != nil {
		return fmt.Errorf("%w: workspace terminal delivery fact: %v", common.ErrInvalidArgument, err)
	}
	return nil
}

func (c ApplyRuntimeTerminalDeliveryCommand) Identity() runtimecontract.RuntimeTerminalDeliveryIdentity {
	return c.Normalized().RuntimeTerminalDeliveryFact.Identity()
}

// WorkspaceLifecycleCommandResult is the synchronous result of a projection or
// terminal-delivery command. DeliveryAck is set only for terminal delivery.
type WorkspaceLifecycleCommandResult struct {
	Workspace    *Workspace
	Transitioned bool
	DeliveryAck  *runtimecontract.RuntimeDeliveryAcknowledgement
}
