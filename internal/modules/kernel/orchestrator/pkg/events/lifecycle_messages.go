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
	runtimecontract.RuntimeTerminalDeliveryIdentity
	SourceOwner   string
	CausationID   string
	StateRevision uint64
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
	if d.SourceOwner == "" {
		return fmt.Errorf("%w: terminal delivery request requires source owner", common.ErrInvalidArgument)
	}
	return d.RuntimeTerminalDeliveryIdentity.Validate()
}

func (d RuntimeTerminalDeliveryRequestData) Identity() runtimecontract.RuntimeTerminalDeliveryIdentity {
	return d.Normalized().RuntimeTerminalDeliveryIdentity
}

// RequestRuntimeTerminationCommand carries a Run-owned administrative intent.
// Restart binds the exact claimed replacement recovery generation. If SM-2
// already sealed a terminal, SM-4 delivers that original terminal instead of
// writing a competing RuntimeTerminationRecord.
type RequestRuntimeTerminationCommand struct {
	WorkspaceID           common.WorkspaceID
	RunID                 common.RunID
	Kind                  runtimecontract.RuntimeTerminationKind
	Reason                string
	RequestedBy           string
	ReplacementRequestID  string
	ReplacementIntentHash string
	RecoveryGeneration    uint64
}

// RequestRuntimeTerminationResult is the synchronous acceptance receipt for an
// administrative intent, including delivery of an already-sealed convergence
// terminal. It proves that SM-4 persisted the Workspace release acknowledgement
// before a caller may create a replacement Run. The original terminal outcome
// remains authoritative; this receipt carries no selected lifecycle state.
type RequestRuntimeTerminationResult struct {
	TerminalDeliveryID string
	WorkspaceReleased  bool
	StateRevision      uint64
}

func (c RequestRuntimeTerminationCommand) Validate() error {
	if c.WorkspaceID == "" || c.RunID == "" || !runtimecontract.ValidRuntimeTerminationKind(c.Kind) || strings.TrimSpace(c.Reason) == "" || strings.TrimSpace(c.RequestedBy) == "" {
		return fmt.Errorf("%w: runtime termination intent requires scope, kind, requester and reason", common.ErrInvalidArgument)
	}
	if c.Kind.Normalized() == runtimecontract.RuntimeTerminationSuperseded {
		if strings.TrimSpace(c.ReplacementRequestID) == "" || strings.TrimSpace(c.ReplacementIntentHash) == "" || c.RecoveryGeneration == 0 {
			return fmt.Errorf("%w: restart handoff requires exact replacement request, intent hash and recovery generation", common.ErrInvalidArgument)
		}
	} else if c.ReplacementRequestID != "" || c.ReplacementIntentHash != "" || c.RecoveryGeneration != 0 {
		return fmt.Errorf("%w: cancellation cannot carry replacement recovery authority", common.ErrInvalidArgument)
	}
	return nil
}
