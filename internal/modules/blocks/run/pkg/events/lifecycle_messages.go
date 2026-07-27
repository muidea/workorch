package events

import (
	"fmt"
	"strings"

	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const runtimeLifecycleSourceOwner = "kernel/orchestrator"

// RUNTIME-SMO lifecycle contracts.
//
// Cross-owner inputs carry immutable intent/fact identity only. They never
// carry a target RunState. Run owner Biz alone decides the SM-1 transition.
//
// Owner of these commands/results: blocks/run.
// Publishers of governance/terminal facts: kernel/orchestrator.
// Publisher of activation/continue intents: application/workorchd (or admin).

// ActivateRunCommand is an application activation intent.
// Run owner may transition pending -> active when scope matches.
// Fields:
//   - WorkspaceID/RunID: required scope (owner: caller)
//   - CausationID: optional caller correlation id (owner: caller)
type ActivateRunCommand struct {
	WorkspaceID common.WorkspaceID
	RunID       common.RunID
	CausationID string
}

func (c ActivateRunCommand) Normalized() ActivateRunCommand {
	c.CausationID = strings.TrimSpace(c.CausationID)
	return c
}

func (c ActivateRunCommand) Validate() error {
	c = c.Normalized()
	if c.WorkspaceID == "" || c.RunID == "" {
		return fmt.Errorf("%w: activate run requires workspace and run", common.ErrInvalidArgument)
	}
	return nil
}

// ContinueRunCommand is a scoped continuation intent after external readiness.
// Run owner still requires a matching persisted governance/continuation fact
// before leaving blocked_*; this command alone never rewrites state.
type ContinueRunCommand struct {
	WorkspaceID common.WorkspaceID
	RunID       common.RunID
	CausationID string
}

func (c ContinueRunCommand) Normalized() ContinueRunCommand {
	c.CausationID = strings.TrimSpace(c.CausationID)
	return c
}

func (c ContinueRunCommand) Validate() error {
	c = c.Normalized()
	if c.WorkspaceID == "" || c.RunID == "" {
		return fmt.Errorf("%w: continue run requires workspace and run", common.ErrInvalidArgument)
	}
	return nil
}

// ApplyRuntimeGovernanceWaitCommand is an immutable Orchestrator fact that a
// governance wait was recorded in SM-2. Run owner may transition
// active|pending -> blocked_decision|blocked_approval.
//
// Exactly one of BlockingTicketID / BlockingApprovalID must be set.
type ApplyRuntimeGovernanceWaitCommand struct {
	WorkspaceID         common.WorkspaceID
	RunID               common.RunID
	SourceOwner         string // must be kernel/orchestrator
	CausationID         string
	AuthoritySnapshotID string
	DefinitionRevision  string
	EvidenceRevision    string
	DispatchID          string
	BlockingTicketID    common.TicketID
	BlockingApprovalID  common.ApprovalID
}

func (c ApplyRuntimeGovernanceWaitCommand) Normalized() ApplyRuntimeGovernanceWaitCommand {
	c.SourceOwner = strings.TrimSpace(c.SourceOwner)
	c.CausationID = strings.TrimSpace(c.CausationID)
	c.AuthoritySnapshotID = strings.TrimSpace(c.AuthoritySnapshotID)
	c.DefinitionRevision = strings.TrimSpace(c.DefinitionRevision)
	c.EvidenceRevision = strings.TrimSpace(c.EvidenceRevision)
	c.DispatchID = strings.TrimSpace(c.DispatchID)
	return c
}

func (c ApplyRuntimeGovernanceWaitCommand) Validate() error {
	c = c.Normalized()
	if c.WorkspaceID == "" || c.RunID == "" {
		return fmt.Errorf("%w: governance wait requires workspace and run", common.ErrInvalidArgument)
	}
	if c.SourceOwner != runtimeLifecycleSourceOwner || c.CausationID == "" || c.AuthoritySnapshotID == "" ||
		c.DefinitionRevision == "" || c.EvidenceRevision == "" || c.DispatchID == "" {
		return fmt.Errorf("%w: governance wait requires sealed source, causation and runtime identity", common.ErrInvalidArgument)
	}
	hasTicket := c.BlockingTicketID != ""
	hasApproval := c.BlockingApprovalID != ""
	if hasTicket == hasApproval {
		return fmt.Errorf("%w: governance wait requires exactly one of ticket or approval", common.ErrInvalidArgument)
	}
	return nil
}

// ApplyRuntimeContinuationEligibleCommand is an immutable Orchestrator fact
// that SM-2 governance evidence is committed and the Run may leave blocked_*.
// Run owner validates binding before blocked_* -> active.
type ApplyRuntimeContinuationEligibleCommand struct {
	WorkspaceID         common.WorkspaceID
	RunID               common.RunID
	SourceOwner         string
	CausationID         string
	AuthoritySnapshotID string
	DefinitionRevision  string
	EvidenceRevision    string
	DispatchID          string
	ResolutionID        common.ResolutionID
	TicketID            common.TicketID
	ApprovalID          common.ApprovalID
}

func (c ApplyRuntimeContinuationEligibleCommand) Normalized() ApplyRuntimeContinuationEligibleCommand {
	c.SourceOwner = strings.TrimSpace(c.SourceOwner)
	c.CausationID = strings.TrimSpace(c.CausationID)
	c.AuthoritySnapshotID = strings.TrimSpace(c.AuthoritySnapshotID)
	c.DefinitionRevision = strings.TrimSpace(c.DefinitionRevision)
	c.EvidenceRevision = strings.TrimSpace(c.EvidenceRevision)
	c.DispatchID = strings.TrimSpace(c.DispatchID)
	return c
}

func (c ApplyRuntimeContinuationEligibleCommand) Validate() error {
	c = c.Normalized()
	if c.WorkspaceID == "" || c.RunID == "" {
		return fmt.Errorf("%w: continuation eligible requires workspace and run", common.ErrInvalidArgument)
	}
	if c.SourceOwner != runtimeLifecycleSourceOwner || c.CausationID == "" || c.AuthoritySnapshotID == "" ||
		c.DefinitionRevision == "" || c.EvidenceRevision == "" || c.DispatchID == "" || c.ResolutionID == "" {
		return fmt.Errorf("%w: continuation eligible requires sealed source, causation, runtime identity and resolution", common.ErrInvalidArgument)
	}
	if c.TicketID == "" && c.ApprovalID == "" {
		return fmt.Errorf("%w: continuation eligible requires ticket or approval binding", common.ErrInvalidArgument)
	}
	return nil
}

// ApplyRuntimeTerminalDeliveryCommand is an SM-4 terminal delivery request.
// Run owner may transition active|blocked_* -> completed|failed|cancelled|superseded
// only when identity matches the sealed terminal. It never carries an arbitrary
// caller-chosen State beyond the sealed action / administrative kind.
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
	// AdministrativeKind is empty for reducer terminals. Only cancel/supersede
	// administrative closures set it; fail without sealed terminal is rejected.
	AdministrativeKind runtimecontract.RuntimeTerminationKind
	FailureReason      string
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
	c.FailureReason = strings.TrimSpace(c.FailureReason)
	return c
}

func (c ApplyRuntimeTerminalDeliveryCommand) Validate() error {
	c = c.Normalized()
	if c.WorkspaceID == "" || c.RunID == "" || c.SourceOwner != runtimeLifecycleSourceOwner || c.CausationID == "" {
		return fmt.Errorf("%w: terminal delivery requires workspace, run, source owner and causation", common.ErrInvalidArgument)
	}
	if c.AdministrativeKind != "" && c.AdministrativeKind != runtimecontract.RuntimeTerminationCancelled && c.AdministrativeKind != runtimecontract.RuntimeTerminationSuperseded {
		return fmt.Errorf("%w: terminal delivery has invalid administrative kind %q", common.ErrInvalidArgument, c.AdministrativeKind)
	}
	if err := c.Identity().Validate(); err != nil {
		return fmt.Errorf("%w: terminal delivery identity: %v", common.ErrInvalidArgument, err)
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
		Action:              c.Action,
		TerminalCode:        c.TerminalCode,
		TerminalSummary:     c.TerminalSummary,
	}
}

// RunLifecycleCommandResult is the synchronous result of a lifecycle command.
// Transitioned=false means the input was accepted as a no-op (duplicate/stale/
// already-terminal) without rewriting owner state.
type RunLifecycleCommandResult struct {
	Run           *runmodel.Run
	Transitioned  bool
	PreviousState common.RunState
	DeliveryAck   *runtimecontract.RuntimeDeliveryAcknowledgement
	Diagnostic    string
}
