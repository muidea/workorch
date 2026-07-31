package events

import (
	"fmt"
	"strings"

	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const runtimeLifecycleSourceOwner = "kernel/orchestrator"
const workspaceExecutionIntentSourceOwner = "blocks/workspace"

// WorkspaceExecutionIntentKind mirrors the bounded Workspace owner intent.
// It remains local to the Run contract so the receiver owns the complete
// command type and no shared event wrapper is introduced.
type WorkspaceExecutionIntentKind string

const (
	WorkspaceExecutionIntentResume  WorkspaceExecutionIntentKind = "resume"
	WorkspaceExecutionIntentRestart WorkspaceExecutionIntentKind = "restart"
)

// WorkspaceExecutionIntentCommand is a Workspace-owner selection fact. It
// contains the current definition identity, never a target RunState. Restart
// carries a sealed bootstrap compiled by application ingress and verified by
// Workspace before forwarding.
type WorkspaceExecutionIntentCommand struct {
	WorkspaceID        common.WorkspaceID
	RunID              common.RunID // required only for resume
	RequestID          string
	Kind               WorkspaceExecutionIntentKind
	GoalID             common.GoalID
	DoDID              common.DoDID
	DefinitionRevision string
	Priority           int
	Bootstrap          *runtimecontract.RuntimeBootstrap
}

func (c WorkspaceExecutionIntentCommand) Normalized() WorkspaceExecutionIntentCommand {
	c.RequestID = strings.TrimSpace(c.RequestID)
	c.Kind = WorkspaceExecutionIntentKind(strings.TrimSpace(string(c.Kind)))
	c.DefinitionRevision = strings.TrimSpace(c.DefinitionRevision)
	if c.Bootstrap != nil {
		clone := c.Bootstrap.Clone()
		c.Bootstrap = &clone
	}
	return c
}

func (c WorkspaceExecutionIntentCommand) Validate() error {
	c = c.Normalized()
	if c.WorkspaceID == "" || c.RequestID == "" || c.GoalID == "" || c.DoDID == "" || c.DefinitionRevision == "" {
		return fmt.Errorf("%w: workspace execution selection requires scope, request and definition identity", common.ErrInvalidArgument)
	}
	switch c.Kind {
	case WorkspaceExecutionIntentResume:
		if c.RunID == "" || c.Bootstrap != nil {
			return fmt.Errorf("%w: resume selection requires selected run and no replacement bootstrap", common.ErrInvalidArgument)
		}
	case WorkspaceExecutionIntentRestart:
		if c.RunID != "" || c.Bootstrap == nil {
			return fmt.Errorf("%w: restart selection requires no predecessor run and one replacement bootstrap", common.ErrInvalidArgument)
		}
		if err := c.Bootstrap.Validate(); err != nil {
			return fmt.Errorf("%w: restart bootstrap: %v", common.ErrInvalidArgument, err)
		}
		definition := c.Bootstrap.Definition.Normalized()
		if definition.GoalID != c.GoalID || definition.DoDID != c.DoDID || definition.Revision != c.DefinitionRevision {
			return fmt.Errorf("%w: restart bootstrap definition does not match workspace selection", common.ErrConflict)
		}
	default:
		return fmt.Errorf("%w: unsupported workspace execution selection %q", common.ErrInvalidArgument, c.Kind)
	}
	return nil
}

// WorkspaceExecutionIntentResult reports Run-owner admission. It does not
// expose a desired state or let an ingress component infer one.
type WorkspaceExecutionIntentResult struct {
	WorkspaceID common.WorkspaceID
	RunID       common.RunID
	RequestID   string
	Kind        WorkspaceExecutionIntentKind
	Disposition string
	Diagnostic  string
}

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
	// ReconciliationRequestID is optional for ordinary activation and is set
	// by the Run owner when a pending Run is activated by /run resume.
	ReconciliationRequestID string
}

func (c ActivateRunCommand) Normalized() ActivateRunCommand {
	c.CausationID = strings.TrimSpace(c.CausationID)
	c.ReconciliationRequestID = strings.TrimSpace(c.ReconciliationRequestID)
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
	if c.WorkspaceID == "" || c.RunID == "" || c.CausationID == "" {
		return fmt.Errorf("%w: continue run requires workspace, run and causation", common.ErrInvalidArgument)
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
	Outcome             runtimecontract.TerminalOutcome
	FailureReason       string
}

// ApplyRuntimeStateIncompatibleFactCommand is an immutable SM-4 observation
// that an existing opaque Runtime state cannot be read by this binary. It
// carries no requested Run state: Run owner converts it to its private SM-1
// lifecycle event and persists fresh_activation_required itself.
type ApplyRuntimeStateIncompatibleFactCommand struct {
	WorkspaceID            common.WorkspaceID
	RunID                  common.RunID
	SourceOwner            string
	CausationID            string
	RuntimeStateRef        string
	ActualSchemaVersion    uint32
	SupportedSchemaVersion uint32
	Feedback               runtimecontract.RuntimeFeedback
}

func (c ApplyRuntimeStateIncompatibleFactCommand) Normalized() ApplyRuntimeStateIncompatibleFactCommand {
	c.SourceOwner = strings.TrimSpace(c.SourceOwner)
	c.CausationID = strings.TrimSpace(c.CausationID)
	c.RuntimeStateRef = strings.TrimSpace(c.RuntimeStateRef)
	c.Feedback = c.Feedback.CloneOriginal()
	return c
}

func (c ApplyRuntimeStateIncompatibleFactCommand) Validate() error {
	c = c.Normalized()
	if c.WorkspaceID == "" || c.RunID == "" || c.SourceOwner != runtimeLifecycleSourceOwner || c.CausationID == "" || c.RuntimeStateRef == "" {
		return fmt.Errorf("%w: runtime state incompatibility fact requires scope, source, causation and state reference", common.ErrInvalidArgument)
	}
	if c.ActualSchemaVersion == 0 || c.SupportedSchemaVersion == 0 || c.ActualSchemaVersion == c.SupportedSchemaVersion {
		return fmt.Errorf("%w: runtime state incompatibility requires distinct actual and supported schema versions", common.ErrInvalidArgument)
	}
	if c.Feedback.Domain != runtimecontract.RuntimeFeedbackDomainDrive || c.Feedback.StableCategory != runtimecontract.RuntimeFeedbackCategoryStateSchemaUnsupported ||
		c.Feedback.WorkspaceID != c.WorkspaceID || c.Feedback.RunID != c.RunID || c.Feedback.OriginalDiagnostic == nil {
		return fmt.Errorf("%w: runtime state incompatibility requires scoped schema feedback", common.ErrInvalidArgument)
	}
	if err := c.Feedback.Validate(); err != nil {
		return fmt.Errorf("%w: runtime state incompatibility feedback: %v", common.ErrInvalidArgument, err)
	}
	return nil
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
	c.FailureReason = strings.TrimSpace(c.FailureReason)
	return c
}

func (c ApplyRuntimeTerminalDeliveryCommand) Validate() error {
	c = c.Normalized()
	if c.WorkspaceID == "" || c.RunID == "" || c.SourceOwner != runtimeLifecycleSourceOwner || c.CausationID == "" {
		return fmt.Errorf("%w: terminal delivery requires workspace, run, source owner and causation", common.ErrInvalidArgument)
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
		Outcome:             c.Outcome,
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
	// RuntimeActivationDisposition is a Run-owned outcome for non-resumable
	// records. It is not an Orchestrator convergence or drive classification.
	RuntimeActivationDisposition runmodel.RuntimeActivationDisposition
	Diagnostic                   string
}
