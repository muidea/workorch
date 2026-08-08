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

// WorkspaceExecutionIntentCommand is a Workspace-owner selection fact. It
// contains the current definition identity, never a target RunState. Restart
// carries a sealed bootstrap compiled by application ingress and verified by
// Workspace before forwarding.
type WorkspaceExecutionIntentCommand struct {
	WorkspaceID        common.WorkspaceID
	RunID              common.RunID // required only for resume
	RequestID          string
	Kind               runtimecontract.RuntimeExecutionIntentKind
	GoalID             common.GoalID
	DoDID              common.DoDID
	DefinitionRevision string
	Priority           int
	Bootstrap          *runtimecontract.RuntimeBootstrap
}

func (c WorkspaceExecutionIntentCommand) Normalized() WorkspaceExecutionIntentCommand {
	c.RequestID = strings.TrimSpace(c.RequestID)
	c.Kind = runtimecontract.RuntimeExecutionIntentKind(strings.TrimSpace(string(c.Kind)))
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
	case runtimecontract.RuntimeExecutionIntentResume:
		if c.RunID == "" || c.Bootstrap != nil {
			return fmt.Errorf("%w: resume selection requires selected run and no replacement bootstrap", common.ErrInvalidArgument)
		}
	case runtimecontract.RuntimeExecutionIntentRestart:
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
	runtimecontract.RuntimeExecutionIntentAdmission
}

func (r WorkspaceExecutionIntentResult) Validate() error {
	if err := r.RuntimeExecutionIntentAdmission.Validate(); err != nil {
		return fmt.Errorf("%w: %v", common.ErrInvalidArgument, err)
	}
	return nil
}

// RUNTIME-SMO lifecycle contracts.
//
// Cross-owner inputs carry immutable intent/fact identity only. They never
// carry a target RunState. Run owner Biz alone decides the SM-1 transition.
//
// Owner of these commands/results: blocks/run.
// Publishers of governance/terminal facts: kernel/orchestrator. Activate and
// continue are Run-private internal events; external ingress can only submit
// the WorkspaceExecutionIntentCommand owned by blocks/workspace.

// RuntimeGovernanceWaitFact is an immutable Orchestrator fact that a
// governance wait was recorded in SM-2. Run owner may transition
// active|pending -> blocked_decision|blocked_approval.
//
// Exactly one of BlockingTicketID / BlockingApprovalID must be set.
type RuntimeGovernanceWaitFact struct {
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

func (c RuntimeGovernanceWaitFact) Normalized() RuntimeGovernanceWaitFact {
	c.SourceOwner = strings.TrimSpace(c.SourceOwner)
	c.CausationID = strings.TrimSpace(c.CausationID)
	c.AuthoritySnapshotID = strings.TrimSpace(c.AuthoritySnapshotID)
	c.DefinitionRevision = strings.TrimSpace(c.DefinitionRevision)
	c.EvidenceRevision = strings.TrimSpace(c.EvidenceRevision)
	c.DispatchID = strings.TrimSpace(c.DispatchID)
	return c
}

func (c RuntimeGovernanceWaitFact) Validate() error {
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

// RuntimeContinuationEligibleFact is an immutable Orchestrator fact
// that SM-2 governance evidence is committed and the Run may leave blocked_*.
// Run owner validates binding before blocked_* -> active.
type RuntimeContinuationEligibleFact struct {
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

func (c RuntimeContinuationEligibleFact) Normalized() RuntimeContinuationEligibleFact {
	c.SourceOwner = strings.TrimSpace(c.SourceOwner)
	c.CausationID = strings.TrimSpace(c.CausationID)
	c.AuthoritySnapshotID = strings.TrimSpace(c.AuthoritySnapshotID)
	c.DefinitionRevision = strings.TrimSpace(c.DefinitionRevision)
	c.EvidenceRevision = strings.TrimSpace(c.EvidenceRevision)
	c.DispatchID = strings.TrimSpace(c.DispatchID)
	return c
}

func (c RuntimeContinuationEligibleFact) Validate() error {
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

// RuntimeTerminalDeliveryFact is an immutable SM-4 terminal delivery fact.
// Run owner may transition active|blocked_* -> completed|failed|cancelled|superseded
// only when identity matches the sealed terminal. It never carries an arbitrary
// caller-chosen State beyond the sealed action / administrative kind.
type RuntimeTerminalDeliveryFact struct {
	runtimecontract.RuntimeTerminalDeliveryFact
}

// RuntimeStateIncompatibleFact is an immutable SM-4 observation
// that an existing opaque Runtime state cannot be read by this binary. It
// carries no requested Run state: Run owner converts it to its private SM-1
// lifecycle event and persists fresh_activation_required itself.
type RuntimeStateIncompatibleFact struct {
	WorkspaceID            common.WorkspaceID
	RunID                  common.RunID
	SourceOwner            string
	CausationID            string
	RuntimeStateRef        string
	ActualSchemaVersion    uint32
	SupportedSchemaVersion uint32
	Feedback               runtimecontract.RuntimeFeedback
}

func (c RuntimeStateIncompatibleFact) Normalized() RuntimeStateIncompatibleFact {
	c.SourceOwner = strings.TrimSpace(c.SourceOwner)
	c.CausationID = strings.TrimSpace(c.CausationID)
	c.RuntimeStateRef = strings.TrimSpace(c.RuntimeStateRef)
	c.Feedback = c.Feedback.CloneOriginal()
	return c
}

func (c RuntimeStateIncompatibleFact) Validate() error {
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

func (c RuntimeTerminalDeliveryFact) Normalized() RuntimeTerminalDeliveryFact {
	c.RuntimeTerminalDeliveryFact = c.RuntimeTerminalDeliveryFact.Normalized()
	return c
}

func (c RuntimeTerminalDeliveryFact) Validate() error {
	c = c.Normalized()
	if c.SourceOwner != runtimeLifecycleSourceOwner {
		return fmt.Errorf("%w: terminal delivery requires workspace, run, source owner and causation", common.ErrInvalidArgument)
	}
	if err := c.RuntimeTerminalDeliveryFact.Validate(); err != nil {
		return fmt.Errorf("%w: terminal delivery fact: %v", common.ErrInvalidArgument, err)
	}
	return nil
}

func (c RuntimeTerminalDeliveryFact) Identity() runtimecontract.RuntimeTerminalDeliveryIdentity {
	return c.Normalized().RuntimeTerminalDeliveryFact.Identity()
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
	RuntimeActivationDisposition runtimecontract.RuntimeActivationDisposition
}

func (r RunLifecycleCommandResult) Clone() RunLifecycleCommandResult {
	cloned := r
	cloned.Run = runmodel.CloneRun(r.Run)
	if r.DeliveryAck != nil {
		ack := *r.DeliveryAck
		cloned.DeliveryAck = &ack
	}
	return cloned
}
