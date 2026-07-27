package workorchruntimecontract

import (
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

// RuntimeDeliveryOwner identifies which resource owner produced a terminal
// delivery acknowledgement. Values are module IDs, not free-form labels.
type RuntimeDeliveryOwner string

const (
	RuntimeDeliveryOwnerResult    RuntimeDeliveryOwner = "blocks/result"
	RuntimeDeliveryOwnerWorkspace RuntimeDeliveryOwner = "blocks/workspace"
	RuntimeDeliveryOwnerRun       RuntimeDeliveryOwner = "blocks/run"
)

// RuntimeDeliveryAcknowledgedState is the owner-local outcome recorded in an
// acknowledgement. It is an immutable fact about that owner's resource, not a
// command that rewrites another owner.
type RuntimeDeliveryAcknowledgedState string

const (
	RuntimeDeliveryAcknowledgedResultRecorded   RuntimeDeliveryAcknowledgedState = "result_recorded"
	RuntimeDeliveryAcknowledgedWorkspaceReleased RuntimeDeliveryAcknowledgedState = "workspace_released"
	RuntimeDeliveryAcknowledgedRunTerminal      RuntimeDeliveryAcknowledgedState = "run_terminal"
	RuntimeDeliveryAcknowledgedNoop             RuntimeDeliveryAcknowledgedState = "noop"
)

// RuntimeTerminalDeliveryIdentity is the sealed identity of one SM-4 terminal
// delivery attempt sequence. Owners match on the full identity; partial or
// guessed matches must no-op.
type RuntimeTerminalDeliveryIdentity struct {
	TerminalDeliveryID  string
	WorkspaceID         common.WorkspaceID
	RunID               common.RunID
	AuthoritySnapshotID string
	DefinitionRevision  string
	EvidenceRevision    string
	DispatchID          string
	Action              RuntimeAction
	TerminalCode        string
	TerminalSummary     string
}

func (id RuntimeTerminalDeliveryIdentity) Normalized() RuntimeTerminalDeliveryIdentity {
	id.TerminalDeliveryID = strings.TrimSpace(id.TerminalDeliveryID)
	id.AuthoritySnapshotID = strings.TrimSpace(id.AuthoritySnapshotID)
	id.DefinitionRevision = strings.TrimSpace(id.DefinitionRevision)
	id.EvidenceRevision = strings.TrimSpace(id.EvidenceRevision)
	id.DispatchID = strings.TrimSpace(id.DispatchID)
	id.Action = RuntimeAction(strings.TrimSpace(string(id.Action)))
	id.TerminalCode = strings.TrimSpace(id.TerminalCode)
	id.TerminalSummary = strings.TrimSpace(id.TerminalSummary)
	return id
}

func (id RuntimeTerminalDeliveryIdentity) Validate() error {
	id = id.Normalized()
	if id.TerminalDeliveryID == "" || id.WorkspaceID == "" || id.RunID == "" {
		return fmt.Errorf("terminal delivery identity requires delivery id, workspace and run")
	}
	if id.AuthoritySnapshotID == "" {
		return fmt.Errorf("terminal delivery identity requires authority snapshot")
	}
	switch id.Action {
	case RuntimeActionComplete, RuntimeActionFail:
		if id.TerminalCode == "" || id.TerminalSummary == "" {
			return fmt.Errorf("terminal delivery identity requires terminal code and summary")
		}
		return nil
	default:
		return fmt.Errorf("terminal delivery identity has invalid action %q", id.Action)
	}
}

// StableTerminalDeliveryID derives a deterministic delivery id from sealed
// terminal identity. Restart retries must reuse the same id.
func StableTerminalDeliveryID(workspaceID common.WorkspaceID, runID common.RunID, authoritySnapshotID, action, terminalCode string) string {
	return "terminal-delivery:" + string(workspaceID) + "/" + string(runID) + "/" +
		strings.TrimSpace(authoritySnapshotID) + "/" + strings.TrimSpace(action) + "/" + strings.TrimSpace(terminalCode)
}

// RuntimeDeliveryAcknowledgement is the immutable fact an owner returns after
// applying a terminal delivery request. Orchestrator SM-4 may only record it in
// the delivery ledger; it must not rewrite SM-2 terminal.
type RuntimeDeliveryAcknowledgement struct {
	WorkspaceID         common.WorkspaceID
	RunID               common.RunID
	TerminalDeliveryID  string
	Owner               RuntimeDeliveryOwner
	AcknowledgedState   RuntimeDeliveryAcknowledgedState
	AuthoritySnapshotID string
	CausationID         string
	Action              RuntimeAction
	TerminalCode        string
}

func (a RuntimeDeliveryAcknowledgement) Normalized() RuntimeDeliveryAcknowledgement {
	a.TerminalDeliveryID = strings.TrimSpace(a.TerminalDeliveryID)
	a.Owner = RuntimeDeliveryOwner(strings.TrimSpace(string(a.Owner)))
	a.AcknowledgedState = RuntimeDeliveryAcknowledgedState(strings.TrimSpace(string(a.AcknowledgedState)))
	a.AuthoritySnapshotID = strings.TrimSpace(a.AuthoritySnapshotID)
	a.CausationID = strings.TrimSpace(a.CausationID)
	a.Action = RuntimeAction(strings.TrimSpace(string(a.Action)))
	a.TerminalCode = strings.TrimSpace(a.TerminalCode)
	return a
}

func (a RuntimeDeliveryAcknowledgement) Validate() error {
	a = a.Normalized()
	if a.WorkspaceID == "" || a.RunID == "" || a.TerminalDeliveryID == "" {
		return fmt.Errorf("delivery acknowledgement requires workspace, run and delivery id")
	}
	if a.AuthoritySnapshotID == "" || a.CausationID == "" || a.TerminalCode == "" {
		return fmt.Errorf("delivery acknowledgement requires sealed authority, causation and terminal code")
	}
	switch a.Action {
	case RuntimeActionComplete, RuntimeActionFail:
	default:
		return fmt.Errorf("delivery acknowledgement has invalid terminal action %q", a.Action)
	}
	switch a.Owner {
	case RuntimeDeliveryOwnerResult, RuntimeDeliveryOwnerWorkspace, RuntimeDeliveryOwnerRun:
	default:
		return fmt.Errorf("delivery acknowledgement has invalid owner %q", a.Owner)
	}
	switch a.AcknowledgedState {
	case RuntimeDeliveryAcknowledgedResultRecorded,
		RuntimeDeliveryAcknowledgedWorkspaceReleased,
		RuntimeDeliveryAcknowledgedRunTerminal,
		RuntimeDeliveryAcknowledgedNoop:
		return nil
	default:
		return fmt.Errorf("delivery acknowledgement has invalid state %q", a.AcknowledgedState)
	}
}
