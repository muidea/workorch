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

// RuntimeTerminalDeliveryFact asks Workspace to release ActiveRun for a
// sealed terminal delivery. Workspace never invents a new terminal.
type RuntimeTerminalDeliveryFact struct {
	runtimecontract.RuntimeTerminalDeliveryFact
}

func (c RuntimeTerminalDeliveryFact) Normalized() RuntimeTerminalDeliveryFact {
	c.RuntimeTerminalDeliveryFact = c.RuntimeTerminalDeliveryFact.Normalized()
	return c
}

func (c RuntimeTerminalDeliveryFact) Validate() error {
	c = c.Normalized()
	if c.SourceOwner != "kernel/orchestrator" {
		return fmt.Errorf("%w: workspace terminal delivery requires workspace, run, source and causation", common.ErrInvalidArgument)
	}
	if err := c.RuntimeTerminalDeliveryFact.Validate(); err != nil {
		return fmt.Errorf("%w: workspace terminal delivery fact: %v", common.ErrInvalidArgument, err)
	}
	return nil
}

func (c RuntimeTerminalDeliveryFact) Identity() runtimecontract.RuntimeTerminalDeliveryIdentity {
	return c.Normalized().RuntimeTerminalDeliveryFact.Identity()
}

// WorkspaceLifecycleCommandResult is the synchronous result of a projection or
// terminal-delivery command. DeliveryAck is set only for terminal delivery.
type WorkspaceLifecycleCommandResult struct {
	Workspace    *Workspace
	Transitioned bool
	DeliveryAck  *runtimecontract.RuntimeDeliveryAcknowledgement
}
