package events

import (
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// RUNTIME-SMO Orchestrator lifecycle facts and SM-4 delivery ledger contracts.
//
// Orchestrator publishes immutable facts after persisting its own SM-2/SM-4
// state. It never carries a target Run/Workspace/Result state. Owners of these
// types: kernel/orchestrator.

// RuntimeGovernanceWaitRecordedData is posted after SM-2 records a governance
// wait. Run owner consumes the corresponding apply-governance-wait command
// (synchronous Send); this Data topic is the immutable audit/notification form.
type RuntimeGovernanceWaitRecordedData struct {
	WorkspaceID         common.WorkspaceID
	RunID               common.RunID
	SourceOwner         string
	CausationID         string
	AuthoritySnapshotID string
	DefinitionRevision  string
	EvidenceRevision    string
	DispatchID          string
	BlockingTicketID    common.TicketID
	BlockingApprovalID  common.ApprovalID
	StateRevision       uint64
}

func (d RuntimeGovernanceWaitRecordedData) Normalized() RuntimeGovernanceWaitRecordedData {
	d.SourceOwner = strings.TrimSpace(d.SourceOwner)
	d.CausationID = strings.TrimSpace(d.CausationID)
	d.AuthoritySnapshotID = strings.TrimSpace(d.AuthoritySnapshotID)
	d.DefinitionRevision = strings.TrimSpace(d.DefinitionRevision)
	d.EvidenceRevision = strings.TrimSpace(d.EvidenceRevision)
	d.DispatchID = strings.TrimSpace(d.DispatchID)
	return d
}

func (d RuntimeGovernanceWaitRecordedData) Validate() error {
	d = d.Normalized()
	if d.WorkspaceID == "" || d.RunID == "" || d.SourceOwner != "kernel/orchestrator" || d.CausationID == "" ||
		d.AuthoritySnapshotID == "" || d.DefinitionRevision == "" || d.EvidenceRevision == "" || d.DispatchID == "" {
		return fmt.Errorf("%w: governance wait fact requires sealed source, causation and runtime identity", common.ErrInvalidArgument)
	}
	if (d.BlockingTicketID == "") == (d.BlockingApprovalID == "") {
		return fmt.Errorf("%w: governance wait fact requires exactly one blocker", common.ErrInvalidArgument)
	}
	return nil
}

// RuntimeContinuationEligibleData is posted after SM-2 commits governance
// evidence that makes the Run eligible to leave blocked_*.
type RuntimeContinuationEligibleData struct {
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
	StateRevision       uint64
}

func (d RuntimeContinuationEligibleData) Normalized() RuntimeContinuationEligibleData {
	d.SourceOwner = strings.TrimSpace(d.SourceOwner)
	d.CausationID = strings.TrimSpace(d.CausationID)
	d.AuthoritySnapshotID = strings.TrimSpace(d.AuthoritySnapshotID)
	d.DefinitionRevision = strings.TrimSpace(d.DefinitionRevision)
	d.EvidenceRevision = strings.TrimSpace(d.EvidenceRevision)
	d.DispatchID = strings.TrimSpace(d.DispatchID)
	return d
}

func (d RuntimeContinuationEligibleData) Validate() error {
	d = d.Normalized()
	if d.WorkspaceID == "" || d.RunID == "" || d.SourceOwner != "kernel/orchestrator" || d.CausationID == "" ||
		d.AuthoritySnapshotID == "" || d.DefinitionRevision == "" || d.EvidenceRevision == "" || d.DispatchID == "" || d.ResolutionID == "" {
		return fmt.Errorf("%w: continuation eligible fact requires sealed source, causation, runtime identity and resolution", common.ErrInvalidArgument)
	}
	if d.TicketID == "" && d.ApprovalID == "" {
		return fmt.Errorf("%w: continuation eligible fact requires ticket or approval binding", common.ErrInvalidArgument)
	}
	return nil
}

// RuntimeTerminalDeliveryRequestData is the immutable SM-4 request fact that
// Result/Workspace/Run owners each consume. Order is Result -> Workspace -> Run.
type RuntimeTerminalDeliveryRequestData struct {
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
	StateRevision       uint64
}

func (d RuntimeTerminalDeliveryRequestData) Normalized() RuntimeTerminalDeliveryRequestData {
	d.SourceOwner = strings.TrimSpace(d.SourceOwner)
	d.CausationID = strings.TrimSpace(d.CausationID)
	d.TerminalDeliveryID = strings.TrimSpace(d.TerminalDeliveryID)
	d.AuthoritySnapshotID = strings.TrimSpace(d.AuthoritySnapshotID)
	d.DefinitionRevision = strings.TrimSpace(d.DefinitionRevision)
	d.EvidenceRevision = strings.TrimSpace(d.EvidenceRevision)
	d.DispatchID = strings.TrimSpace(d.DispatchID)
	d.Outcome = d.Outcome.Normalized()
	return d
}

func (d RuntimeTerminalDeliveryRequestData) Validate() error {
	d = d.Normalized()
	identity := runtimecontract.RuntimeTerminalDeliveryIdentity{
		TerminalDeliveryID:  d.TerminalDeliveryID,
		WorkspaceID:         d.WorkspaceID,
		RunID:               d.RunID,
		AuthoritySnapshotID: d.AuthoritySnapshotID,
		DefinitionRevision:  d.DefinitionRevision,
		EvidenceRevision:    d.EvidenceRevision,
		DispatchID:          d.DispatchID,
		Outcome:             d.Outcome,
	}
	if d.SourceOwner == "" {
		return fmt.Errorf("%w: terminal delivery request requires source owner", common.ErrInvalidArgument)
	}
	return identity.Validate()
}

func (d RuntimeTerminalDeliveryRequestData) Identity() runtimecontract.RuntimeTerminalDeliveryIdentity {
	d = d.Normalized()
	return runtimecontract.RuntimeTerminalDeliveryIdentity{
		TerminalDeliveryID:  d.TerminalDeliveryID,
		WorkspaceID:         d.WorkspaceID,
		RunID:               d.RunID,
		AuthoritySnapshotID: d.AuthoritySnapshotID,
		DefinitionRevision:  d.DefinitionRevision,
		EvidenceRevision:    d.EvidenceRevision,
		DispatchID:          d.DispatchID,
		Outcome:             d.Outcome,
	}
}

// RequestRuntimeTerminationCommand is an application administrative intent.
// Orchestrator persists RuntimeTerminationRecord before it starts SM-4.
type RequestRuntimeTerminationCommand struct {
	WorkspaceID common.WorkspaceID
	RunID       common.RunID
	Kind        runtimecontract.RuntimeTerminationKind
	Reason      string
	RequestedBy string
}

// RequestRuntimeTerminationResult is the synchronous acceptance receipt for an
// administrative termination intent. It proves that the SM-4 saga persisted
// the Workspace release acknowledgement before a caller may create a
// replacement Run. It carries no caller-selected lifecycle state.
type RequestRuntimeTerminationResult struct {
	TerminalDeliveryID string
	WorkspaceReleased  bool
	StateRevision      uint64
}

func (c RequestRuntimeTerminationCommand) Validate() error {
	if c.WorkspaceID == "" || c.RunID == "" || !runtimecontract.ValidRuntimeTerminationKind(c.Kind) || strings.TrimSpace(c.Reason) == "" || strings.TrimSpace(c.RequestedBy) == "" {
		return fmt.Errorf("%w: runtime termination intent requires scope, kind, requester and reason", common.ErrInvalidArgument)
	}
	return nil
}
