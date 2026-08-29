package run

import (
	"fmt"
	"strings"
	"time"

	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// lifecycleTransition is the owner-local SM-1 decision produced by a pure
// reducer. External inputs never supply Target directly.
type lifecycleTransition struct {
	Target                  common.RunState
	BlockingTicketID        common.TicketID
	BlockingApproval        common.ApprovalID
	FailureReason           string
	StartedAt               string
	EndedAt                 string
	TerminalDelivery        *runtimecontract.RuntimeTerminalDeliveryIdentity
	TerminalResultID        common.ResultID
	ReconciliationRequestID string
	// Noop means the input was accepted without rewriting owner state
	// (duplicate, already-terminal, or already-converged).
	Noop       bool
	Diagnostic string
}

type activateRunFact struct {
	WorkspaceID             common.WorkspaceID
	RunID                   common.RunID
	CausationID             string
	ReconciliationRequestID string
}

func (f activateRunFact) validate() error {
	if f.WorkspaceID == "" || f.RunID == "" {
		return fmt.Errorf("%w: activate run requires workspace and run", common.ErrInvalidArgument)
	}
	return nil
}

func lifecycleNow() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

func reduceActivateRun(current *runmodel.Run, cmd activateRunFact) (lifecycleTransition, error) {
	if current == nil {
		return lifecycleTransition{}, common.ErrInvalidArgument
	}
	if err := cmd.validate(); err != nil {
		return lifecycleTransition{}, err
	}
	if current.WorkspaceID != cmd.WorkspaceID || current.ID != cmd.RunID {
		return lifecycleTransition{}, common.ErrConflict
	}
	if runStateIsTerminal(current.State) {
		return lifecycleTransition{Noop: true, Diagnostic: "run already terminal"}, nil
	}
	if current.State == common.RunStateActive {
		return lifecycleTransition{Noop: true, Diagnostic: "run already active"}, nil
	}
	if current.State != common.RunStatePending {
		return lifecycleTransition{}, fmt.Errorf("%w: activate requires pending run", common.ErrConflict)
	}
	startedAt := strings.TrimSpace(current.StartedAt)
	if startedAt == "" {
		startedAt = lifecycleNow()
	}
	return lifecycleTransition{
		Target:                  common.RunStateActive,
		StartedAt:               startedAt,
		ReconciliationRequestID: strings.TrimSpace(cmd.ReconciliationRequestID),
	}, nil
}

func reduceGovernanceWait(current *runmodel.Run, cmd runevents.RuntimeGovernanceWaitFact) (lifecycleTransition, error) {
	if current == nil {
		return lifecycleTransition{}, common.ErrInvalidArgument
	}
	if err := cmd.Validate(); err != nil {
		return lifecycleTransition{}, err
	}
	if current.WorkspaceID != cmd.WorkspaceID || current.ID != cmd.RunID {
		return lifecycleTransition{}, common.ErrConflict
	}
	if runStateIsTerminal(current.State) {
		return lifecycleTransition{Noop: true, Diagnostic: "run already terminal"}, nil
	}
	if err := validateRuntimeFactIdentity(current, cmd.AuthoritySnapshotID, cmd.DefinitionRevision, cmd.EvidenceRevision, cmd.DispatchID); err != nil {
		return lifecycleTransition{}, err
	}
	if current.RuntimeLifecycleFence == nil {
		return lifecycleTransition{}, fmt.Errorf("%w: governance wait does not match persisted governance binding", common.ErrConflict)
	}
	fence := current.RuntimeLifecycleFence.Normalized()
	if fence.GovernanceFenceID != runtimeGovernanceFenceID(cmd.AuthoritySnapshotID, cmd.DispatchID) || fence.GovernanceResolutionID != "" || fence.TerminalFenceID != "" {
		return lifecycleTransition{}, fmt.Errorf("%w: governance wait does not match persisted governance binding", common.ErrConflict)
	}
	target := common.RunStateBlockedOnDecision
	if cmd.BlockingApprovalID != "" {
		target = common.RunStateBlockedOnApproval
	}
	// Duplicate wait with the same blocker is an idempotent no-op.
	if current.State == target {
		if target == common.RunStateBlockedOnDecision && current.BlockingTicketID == cmd.BlockingTicketID {
			return lifecycleTransition{Noop: true, Diagnostic: "governance wait already applied"}, nil
		}
		if target == common.RunStateBlockedOnApproval && current.BlockingApprovalID == cmd.BlockingApprovalID {
			return lifecycleTransition{Noop: true, Diagnostic: "governance wait already applied"}, nil
		}
		return lifecycleTransition{}, fmt.Errorf("%w: governance wait blocker mismatch", common.ErrConflict)
	}
	if current.State != common.RunStatePending && current.State != common.RunStateActive {
		return lifecycleTransition{}, fmt.Errorf("%w: governance wait requires pending or active run", common.ErrConflict)
	}
	if !runStateCanTransition(current.State, target) {
		return lifecycleTransition{}, common.ErrConflict
	}
	return lifecycleTransition{
		Target:           target,
		BlockingTicketID: cmd.BlockingTicketID,
		BlockingApproval: cmd.BlockingApprovalID,
	}, nil
}

func reduceContinuationEligible(current *runmodel.Run, cmd runevents.RuntimeContinuationEligibleFact) (lifecycleTransition, error) {
	if current == nil {
		return lifecycleTransition{}, common.ErrInvalidArgument
	}
	if err := cmd.Validate(); err != nil {
		return lifecycleTransition{}, err
	}
	if current.WorkspaceID != cmd.WorkspaceID || current.ID != cmd.RunID {
		return lifecycleTransition{}, common.ErrConflict
	}
	if runStateIsTerminal(current.State) {
		return lifecycleTransition{Noop: true, Diagnostic: "run already terminal"}, nil
	}
	if err := validateRuntimeFactIdentity(current, cmd.AuthoritySnapshotID, cmd.DefinitionRevision, cmd.EvidenceRevision, cmd.DispatchID); err != nil {
		return lifecycleTransition{}, err
	}
	if current.RuntimeLifecycleFence == nil {
		return lifecycleTransition{}, fmt.Errorf("%w: continuation eligible does not match persisted governance resolution", common.ErrConflict)
	}
	fence := current.RuntimeLifecycleFence.Normalized()
	if fence.GovernanceFenceID != runtimeGovernanceFenceID(cmd.AuthoritySnapshotID, cmd.DispatchID) || fence.GovernanceResolutionID != cmd.ResolutionID || fence.TerminalFenceID != "" {
		return lifecycleTransition{}, fmt.Errorf("%w: continuation eligible does not match persisted governance resolution", common.ErrConflict)
	}
	if current.State == common.RunStateActive {
		return lifecycleTransition{Noop: true, Diagnostic: "run already active"}, nil
	}
	switch current.State {
	case common.RunStateBlockedOnDecision:
		if cmd.TicketID == "" || current.BlockingTicketID == "" || cmd.TicketID != current.BlockingTicketID {
			return lifecycleTransition{}, fmt.Errorf("%w: continuation eligible ticket binding mismatch", common.ErrConflict)
		}
	case common.RunStateBlockedOnApproval:
		if cmd.ApprovalID == "" || current.BlockingApprovalID == "" || cmd.ApprovalID != current.BlockingApprovalID {
			return lifecycleTransition{}, fmt.Errorf("%w: continuation eligible approval binding mismatch", common.ErrConflict)
		}
	default:
		return lifecycleTransition{}, fmt.Errorf("%w: continuation eligible requires blocked run", common.ErrConflict)
	}
	if !runStateCanTransition(current.State, common.RunStateActive) {
		return lifecycleTransition{}, common.ErrConflict
	}
	startedAt := strings.TrimSpace(current.StartedAt)
	if startedAt == "" {
		startedAt = lifecycleNow()
	}
	return lifecycleTransition{
		Target:    common.RunStateActive,
		StartedAt: startedAt,
	}, nil
}

func reduceTerminalDelivery(current *runmodel.Run, cmd runevents.RuntimeTerminalDeliveryFact) (lifecycleTransition, error) {
	if current == nil {
		return lifecycleTransition{}, common.ErrInvalidArgument
	}
	if err := cmd.Validate(); err != nil {
		return lifecycleTransition{}, err
	}
	if current.WorkspaceID != cmd.WorkspaceID || current.ID != cmd.RunID {
		return lifecycleTransition{}, common.ErrConflict
	}
	target, reason, err := terminalTargetFromCommand(cmd)
	if err != nil {
		return lifecycleTransition{}, err
	}
	identity := cmd.Identity().Normalized()
	if current.TerminalDelivery != nil {
		if sameRunTerminalDelivery(current.TerminalDelivery, &identity) && current.TerminalResultID == cmd.ResultID {
			return lifecycleTransition{Noop: true, Diagnostic: "run terminal delivery already applied"}, nil
		}
		return lifecycleTransition{}, fmt.Errorf("%w: conflicting run terminal delivery", common.ErrConflict)
	}
	if current.State == target {
		return lifecycleTransition{}, fmt.Errorf("%w: terminal Run lacks delivery fence", common.ErrConflict)
	}
	if runStateIsTerminal(current.State) {
		return lifecycleTransition{}, fmt.Errorf("%w: terminal Run lacks delivery fence", common.ErrConflict)
	}
	if target == common.RunStateSuperseded && current.Lineage.LineageState != runmodel.RunLineageStateActive {
		return lifecycleTransition{}, fmt.Errorf("%w: Run lineage is outside the current data contract", common.ErrConflict)
	}
	if err := validateSealedTerminalIdentity(current, cmd); err != nil {
		return lifecycleTransition{}, err
	}
	if !runStateCanTransition(current.State, target) {
		return lifecycleTransition{}, common.ErrConflict
	}
	endedAt := strings.TrimSpace(current.EndedAt)
	if endedAt == "" {
		endedAt = lifecycleNow()
	}
	return lifecycleTransition{
		Target:           target,
		FailureReason:    reason,
		EndedAt:          endedAt,
		TerminalDelivery: &identity,
		TerminalResultID: cmd.ResultID,
	}, nil
}

func terminalTargetFromCommand(cmd runevents.RuntimeTerminalDeliveryFact) (common.RunState, string, error) {
	cmd = cmd.Normalized()
	switch cmd.Outcome.Kind {
	case runtimecontract.TerminalOutcomeKindCancelled:
		return common.RunStateCancelled, cmd.Outcome.Summary, nil
	case runtimecontract.TerminalOutcomeKindSuperseded:
		return common.RunStateSuperseded, cmd.Outcome.Summary, nil
	case runtimecontract.TerminalOutcomeKindCompleted:
		return common.RunStateCompleted, "", nil
	case runtimecontract.TerminalOutcomeKindFailed:
		return common.RunStateFailed, cmd.Outcome.Code + ": " + cmd.Outcome.Summary, nil
	default:
		return "", "", fmt.Errorf("%w: terminal delivery has invalid outcome kind", common.ErrInvalidArgument)
	}
}

func validateSealedTerminalIdentity(current *runmodel.Run, cmd runevents.RuntimeTerminalDeliveryFact) error {
	cmd = cmd.Normalized()
	if current.RuntimeBootstrap == nil || current.RuntimeState == nil || current.RuntimeLifecycleFence == nil {
		return fmt.Errorf("%w: sealed terminal delivery requires an opaque lifecycle fence", common.ErrConflict)
	}
	if cmd.AuthoritySnapshotID == "" || cmd.DefinitionRevision == "" || cmd.EvidenceRevision == "" {
		return fmt.Errorf("%w: terminal delivery requires complete sealed identity", common.ErrConflict)
	}
	fence := current.RuntimeLifecycleFence.Normalized()
	if fence.TerminalFenceID != cmd.TerminalDeliveryID || fence.WorkspaceID != cmd.WorkspaceID || fence.RunID != cmd.RunID ||
		fence.RuntimeStateRevision != current.RuntimeState.StateRevision || fence.RuntimeStateRef != runevents.RuntimeStateRef(current.WorkspaceID, current.ID, current.RuntimeState.StateRevision) {
		return fmt.Errorf("%w: terminal delivery lifecycle fence mismatch", common.ErrConflict)
	}
	return nil
}

// validateRuntimeFactIdentity verifies a cross-owner governance fact against
// the Runtime snapshot already persisted by the Run owner. It deliberately
// does not load Orchestrator state: Run can accept only the sealed identity it
// already owns, otherwise it records no lifecycle transition.
func validateRuntimeFactIdentity(current *runmodel.Run, authoritySnapshotID, definitionRevision, evidenceRevision, dispatchID string) error {
	if current == nil || current.RuntimeBootstrap == nil || current.RuntimeState == nil {
		return fmt.Errorf("%w: runtime lifecycle fact requires persisted typed runtime state", common.ErrConflict)
	}
	if strings.TrimSpace(authoritySnapshotID) == "" || strings.TrimSpace(definitionRevision) == "" ||
		strings.TrimSpace(evidenceRevision) == "" || strings.TrimSpace(dispatchID) == "" {
		return fmt.Errorf("%w: runtime lifecycle fact has incomplete sealed identity", common.ErrConflict)
	}
	fence := current.RuntimeLifecycleFence
	if fence == nil || fence.WorkspaceID != current.WorkspaceID || fence.RunID != current.ID ||
		fence.RuntimeStateRevision != current.RuntimeState.StateRevision || fence.GovernanceFenceID != runtimeGovernanceFenceID(authoritySnapshotID, dispatchID) {
		return fmt.Errorf("%w: runtime lifecycle fact identity mismatch", common.ErrConflict)
	}
	return nil
}

func runtimeGovernanceFenceID(authoritySnapshotID, dispatchID string) string {
	return "runtime-governance/" + strings.TrimSpace(authoritySnapshotID) + "/" + strings.TrimSpace(dispatchID)
}

func applyLifecycleTransition(current *runmodel.Run, transition lifecycleTransition) *runmodel.Run {
	if current == nil {
		return nil
	}
	next := *current
	next.State = transition.Target
	next.BlockingTicketID = transition.BlockingTicketID
	next.BlockingApprovalID = transition.BlockingApproval
	next.FailureReason = transition.FailureReason
	if transition.StartedAt != "" {
		next.StartedAt = transition.StartedAt
	}
	if transition.EndedAt != "" {
		next.EndedAt = transition.EndedAt
	}
	if transition.TerminalDelivery != nil {
		identity := transition.TerminalDelivery.Normalized()
		next.TerminalDelivery = &identity
		next.TerminalResultID = transition.TerminalResultID
	}
	if transition.ReconciliationRequestID != "" {
		next.ReconciliationRequestID = strings.TrimSpace(transition.ReconciliationRequestID)
	}
	if transition.Target == common.RunStateSuperseded {
		lineage := next.Lineage
		if lineage.LineageState == runmodel.RunLineageStateActive {
			lineage.LineageState = runmodel.RunLineageStateSuperseded
			next.Lineage = lineage
		}
	}
	return &next
}

func sameRunTerminalDelivery(left, right *runtimecontract.RuntimeTerminalDeliveryIdentity) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	l, r := left.Normalized(), right.Normalized()
	return l.TerminalDeliveryID == r.TerminalDeliveryID && l.WorkspaceID == r.WorkspaceID && l.RunID == r.RunID &&
		l.AuthoritySnapshotID == r.AuthoritySnapshotID && l.DefinitionRevision == r.DefinitionRevision &&
		l.EvidenceRevision == r.EvidenceRevision && l.DispatchID == r.DispatchID && l.Outcome == r.Outcome
}

func notificationTopicForRunState(state common.RunState) string {
	switch state {
	case common.RunStateActive:
		return runevents.TopicRunStarted
	case common.RunStateBlockedOnDecision:
		return runevents.TopicRunBlockedOnDecision
	case common.RunStateBlockedOnApproval:
		return runevents.TopicRunBlockedOnApproval
	case common.RunStateCompleted:
		return runevents.TopicRunCompleted
	case common.RunStateFailed:
		return runevents.TopicRunFailed
	case common.RunStateCancelled:
		return runevents.TopicRunCancelled
	case common.RunStateSuperseded:
		return runevents.TopicRunSuperseded
	default:
		return runevents.TopicRunStateChanged
	}
}
