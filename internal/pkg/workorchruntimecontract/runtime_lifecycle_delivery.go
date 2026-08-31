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
	RuntimeDeliveryAcknowledgedResultRecorded    RuntimeDeliveryAcknowledgedState = "result_recorded"
	RuntimeDeliveryAcknowledgedWorkspaceReleased RuntimeDeliveryAcknowledgedState = "workspace_released"
	RuntimeDeliveryAcknowledgedRunTerminal       RuntimeDeliveryAcknowledgedState = "run_terminal"
	RuntimeDeliveryAcknowledgedNoop              RuntimeDeliveryAcknowledgedState = "noop"
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
	Outcome             TerminalOutcome
}

func (id RuntimeTerminalDeliveryIdentity) Normalized() RuntimeTerminalDeliveryIdentity {
	id.TerminalDeliveryID = strings.TrimSpace(id.TerminalDeliveryID)
	id.AuthoritySnapshotID = strings.TrimSpace(id.AuthoritySnapshotID)
	id.DefinitionRevision = strings.TrimSpace(id.DefinitionRevision)
	id.EvidenceRevision = strings.TrimSpace(id.EvidenceRevision)
	id.DispatchID = strings.TrimSpace(id.DispatchID)
	id.Outcome = id.Outcome.Normalized()
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
	if id.DefinitionRevision == "" || id.EvidenceRevision == "" || id.DispatchID == "" {
		return fmt.Errorf("terminal delivery identity requires definition, evidence and dispatch identity")
	}
	return id.Outcome.Validate()
}

// RuntimeTerminalDeliveryFact is the owner-neutral immutable SM-4 input shared
// by the Run, Workspace and Result owners. It contains no requested owner
// state; each owner wraps it in its own EventHub command and decides its own
// transition internally.
type RuntimeTerminalDeliveryFact struct {
	RuntimeTerminalDeliveryIdentity
	SourceOwner string
	CausationID string
}

func (f RuntimeTerminalDeliveryFact) Normalized() RuntimeTerminalDeliveryFact {
	f.RuntimeTerminalDeliveryIdentity = f.RuntimeTerminalDeliveryIdentity.Normalized()
	f.SourceOwner = strings.TrimSpace(f.SourceOwner)
	f.CausationID = strings.TrimSpace(f.CausationID)
	return f
}

func (f RuntimeTerminalDeliveryFact) Validate() error {
	f = f.Normalized()
	if f.SourceOwner == "" || f.CausationID == "" {
		return fmt.Errorf("terminal delivery fact requires source owner and causation")
	}
	return f.RuntimeTerminalDeliveryIdentity.Validate()
}

func (f RuntimeTerminalDeliveryFact) Identity() RuntimeTerminalDeliveryIdentity {
	return f.Normalized().RuntimeTerminalDeliveryIdentity
}

// StableTerminalDeliveryID derives a deterministic delivery id from sealed
// terminal identity. Restart retries must reuse the same id.
func StableTerminalDeliveryID(workspaceID common.WorkspaceID, runID common.RunID, authoritySnapshotID string, outcome TerminalOutcome) string {
	outcome = outcome.Normalized()
	return "terminal-delivery:" + string(workspaceID) + "/" + string(runID) + "/" +
		strings.TrimSpace(authoritySnapshotID) + "/" + string(outcome.Kind) + "/" + outcome.Code
}

// DeriveRuntimeTerminalOutcome is the shared canonical terminal identity
// function used by both live SM-4 delivery and fresh-load validation. It is
// intentionally pure: no owner can reinterpret an administrative or
// convergence terminal after restart.
func DeriveRuntimeTerminalOutcome(decision RuntimeDecision, termination *RuntimeTerminationRecord) (TerminalOutcome, bool, error) {
	if termination != nil {
		value := termination.Normalized()
		if err := value.Validate(); err != nil {
			return TerminalOutcome{}, false, err
		}
		if decision.Terminal != nil || decision.State == ConvergenceStateComplete || decision.State == ConvergenceStateFailed {
			return TerminalOutcome{}, false, fmt.Errorf("runtime cannot carry administrative and convergence terminals together")
		}
		kind := TerminalOutcomeKindCancelled
		if value.Kind == RuntimeTerminationSuperseded {
			kind = TerminalOutcomeKindSuperseded
		}
		outcome := TerminalOutcome{Kind: kind, Code: "administrative_" + string(value.Kind), Summary: value.Reason}.Normalized()
		if err := outcome.Validate(); err != nil {
			return TerminalOutcome{}, false, err
		}
		return outcome, true, nil
	}
	if decision.State != ConvergenceStateComplete && decision.State != ConvergenceStateFailed {
		if decision.Terminal != nil {
			return TerminalOutcome{}, false, fmt.Errorf("non-terminal convergence state carries terminal detail")
		}
		return TerminalOutcome{}, false, nil
	}
	if decision.Terminal == nil {
		return TerminalOutcome{}, false, fmt.Errorf("terminal convergence state has no terminal detail")
	}
	kind := TerminalOutcomeKindFailed
	if decision.State == ConvergenceStateComplete {
		kind = TerminalOutcomeKindCompleted
	}
	outcome := TerminalOutcome{Kind: kind, Code: decision.Terminal.Code, Summary: decision.Terminal.Summary}.Normalized()
	if err := outcome.Validate(); err != nil {
		return TerminalOutcome{}, false, err
	}
	return outcome, true, nil
}

// RuntimeTerminalResultID is the shared immutable Result identity carried by
// terminal delivery acknowledgements. Result remains the mutation owner; this
// helper only prevents Orchestrator validation from copying a private formula.
func RuntimeTerminalResultID(workspaceID common.WorkspaceID, runID common.RunID) common.ResultID {
	return common.ResultID(string(workspaceID) + "/" + string(runID) + "/result/dod-assessment")
}

// RuntimeDeliveryAcknowledgement is the immutable fact an owner returns after
// applying a terminal delivery request. Orchestrator SM-4 may only record it in
// the delivery ledger; it must not rewrite SM-2 terminal.
type RuntimeDeliveryAcknowledgement struct {
	WorkspaceID common.WorkspaceID
	RunID       common.RunID
	// ResultID is present only on the Result-owner acknowledgement. It is the
	// exact owner-derived terminal Result identity that SM-4 may relay to the Run
	// owner as immutable lineage input; it grants no Result mutation authority.
	ResultID            common.ResultID
	TerminalDeliveryID  string
	Owner               RuntimeDeliveryOwner
	AcknowledgedState   RuntimeDeliveryAcknowledgedState
	AuthoritySnapshotID string
	CausationID         string
	Outcome             TerminalOutcome
}

func (a RuntimeDeliveryAcknowledgement) Normalized() RuntimeDeliveryAcknowledgement {
	a.ResultID = common.ResultID(strings.TrimSpace(string(a.ResultID)))
	a.TerminalDeliveryID = strings.TrimSpace(a.TerminalDeliveryID)
	a.Owner = RuntimeDeliveryOwner(strings.TrimSpace(string(a.Owner)))
	a.AcknowledgedState = RuntimeDeliveryAcknowledgedState(strings.TrimSpace(string(a.AcknowledgedState)))
	a.AuthoritySnapshotID = strings.TrimSpace(a.AuthoritySnapshotID)
	a.CausationID = strings.TrimSpace(a.CausationID)
	a.Outcome = a.Outcome.Normalized()
	return a
}

func (a RuntimeDeliveryAcknowledgement) Validate() error {
	a = a.Normalized()
	if a.WorkspaceID == "" || a.RunID == "" || a.TerminalDeliveryID == "" {
		return fmt.Errorf("delivery acknowledgement requires workspace, run and delivery id")
	}
	if a.AuthoritySnapshotID == "" || a.CausationID == "" {
		return fmt.Errorf("delivery acknowledgement requires sealed authority, causation and terminal code")
	}
	if err := a.Outcome.Validate(); err != nil {
		return fmt.Errorf("delivery acknowledgement outcome: %w", err)
	}
	switch a.Owner {
	case RuntimeDeliveryOwnerResult:
		if a.ResultID == "" {
			return fmt.Errorf("result delivery acknowledgement requires result identity")
		}
	case RuntimeDeliveryOwnerWorkspace, RuntimeDeliveryOwnerRun:
		if a.ResultID != "" {
			return fmt.Errorf("non-result delivery acknowledgement cannot carry result identity")
		}
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

func (o TerminalOutcome) Normalized() TerminalOutcome {
	o.Kind = TerminalOutcomeKind(strings.TrimSpace(string(o.Kind)))
	o.Code = strings.TrimSpace(o.Code)
	o.Summary = strings.TrimSpace(o.Summary)
	return o
}

func (o TerminalOutcome) Validate() error {
	o = o.Normalized()
	switch o.Kind {
	case TerminalOutcomeKindCompleted, TerminalOutcomeKindFailed, TerminalOutcomeKindCancelled, TerminalOutcomeKindSuperseded:
	default:
		return fmt.Errorf("terminal outcome has invalid kind %q", o.Kind)
	}
	if o.Code == "" || o.Summary == "" {
		return fmt.Errorf("terminal outcome requires code and summary")
	}
	return nil
}
