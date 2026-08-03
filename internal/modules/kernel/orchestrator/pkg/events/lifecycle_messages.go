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
