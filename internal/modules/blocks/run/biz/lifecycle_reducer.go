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
	RuntimeActivationStatus *runmodel.RuntimeActivationStatus
	ReconciliationRequestID string
	// Noop means the input was accepted without rewriting owner state
	// (duplicate, already-terminal, or already-converged).
	Noop       bool
	Diagnostic string
}

func lifecycleNow() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

func reduceActivateRun(current *runmodel.Run, cmd runevents.ActivateRunCommand) (lifecycleTransition, error) {
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
	if runtimeActivationDispositionForRun(current) == runmodel.RuntimeActivationDispositionFreshActivationRequired {
		return lifecycleTransition{Noop: true, Diagnostic: string(runmodel.RuntimeActivationDispositionFreshActivationRequired)}, nil
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

func reduceGovernanceWait(current *runmodel.Run, cmd runevents.ApplyRuntimeGovernanceWaitCommand) (lifecycleTransition, error) {
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
	if cmd.BlockingTicketID == "" || !hasPersistedGovernanceRequest(current.RuntimeState, cmd.BlockingTicketID, cmd.AuthoritySnapshotID, cmd.DispatchID) {
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

func reduceContinuationEligible(current *runmodel.Run, cmd runevents.ApplyRuntimeContinuationEligibleCommand) (lifecycleTransition, error) {
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
	if cmd.TicketID == "" || !hasPersistedGovernanceResolution(current.RuntimeState, cmd.ResolutionID, cmd.TicketID, cmd.AuthoritySnapshotID, cmd.DispatchID) {
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

func reduceContinueRun(current *runmodel.Run, cmd runevents.ContinueRunCommand) (lifecycleTransition, error) {
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
	if runtimeActivationDispositionForRun(current) == runmodel.RuntimeActivationDispositionFreshActivationRequired {
		return lifecycleTransition{Noop: true, Diagnostic: string(runmodel.RuntimeActivationDispositionFreshActivationRequired)}, nil
	}
	if current.State == common.RunStateActive {
		return lifecycleTransition{Noop: true, Diagnostic: "run already active"}, nil
	}
	// Continue intent alone never rewrites blocked_* without a matching
	// governance continuation fact (ApplyRuntimeContinuationEligibleCommand).
	return lifecycleTransition{}, fmt.Errorf("%w: continue requires matching governance continuation fact", common.ErrConflict)
}

// reduceRuntimeStateIncompatible converts the immutable Orchestrator schema
// observation into Run-owned administrative state. The Runtime aggregate stays
// opaque: only its stored ref/version are compared and no authority, dispatch,
// governance, or delivery value is decoded here.
func reduceRuntimeStateIncompatible(current *runmodel.Run, cmd runevents.ApplyRuntimeStateIncompatibleFactCommand) (lifecycleTransition, error) {
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
	if current.RuntimeState == nil {
		return lifecycleTransition{}, fmt.Errorf("%w: runtime state incompatibility requires persisted runtime state", common.ErrConflict)
	}
	if runevents.RuntimeStateRef(current.WorkspaceID, current.ID, current.RuntimeState.StateRevision) != cmd.RuntimeStateRef ||
		current.RuntimeState.SchemaVersion != cmd.ActualSchemaVersion {
		return lifecycleTransition{}, fmt.Errorf("%w: runtime state incompatibility identity mismatch", common.ErrConflict)
	}
	feedback := cmd.Feedback.CloneOriginal()
	status := &runmodel.RuntimeActivationStatus{
		Disposition:            runmodel.RuntimeActivationDispositionFreshActivationRequired,
		RuntimeStateRef:        cmd.RuntimeStateRef,
		ActualSchemaVersion:    cmd.ActualSchemaVersion,
		SupportedSchemaVersion: cmd.SupportedSchemaVersion,
		Feedback:               &feedback,
	}
	if current.RuntimeActivationStatus != nil {
		if sameRuntimeActivationStatus(current.RuntimeActivationStatus, status) {
			return lifecycleTransition{Noop: true, Diagnostic: string(status.Disposition)}, nil
		}
		return lifecycleTransition{}, fmt.Errorf("%w: conflicting runtime activation status", common.ErrConflict)
	}
	return lifecycleTransition{
		Target:                  current.State,
		BlockingTicketID:        current.BlockingTicketID,
		BlockingApproval:        current.BlockingApprovalID,
		FailureReason:           current.FailureReason,
		RuntimeActivationStatus: status,
	}, nil
}

func reduceTerminalDelivery(current *runmodel.Run, cmd runevents.ApplyRuntimeTerminalDeliveryCommand) (lifecycleTransition, error) {
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
		if sameRunTerminalDelivery(current.TerminalDelivery, &identity) {
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
	}, nil
}

func terminalTargetFromCommand(cmd runevents.ApplyRuntimeTerminalDeliveryCommand) (common.RunState, string, error) {
	cmd = cmd.Normalized()
	switch cmd.Outcome.Kind {
	case runtimecontract.TerminalOutcomeKindCancelled:
		reason := cmd.FailureReason
		if reason == "" {
			reason = cmd.Outcome.Summary
		}
		return common.RunStateCancelled, reason, nil
	case runtimecontract.TerminalOutcomeKindSuperseded:
		reason := cmd.FailureReason
		if reason == "" {
			reason = cmd.Outcome.Summary
		}
		return common.RunStateSuperseded, reason, nil
	case runtimecontract.TerminalOutcomeKindCompleted:
		return common.RunStateCompleted, "", nil
	case runtimecontract.TerminalOutcomeKindFailed:
		reason := cmd.FailureReason
		if reason == "" {
			if cmd.Outcome.Code != "" && cmd.Outcome.Summary != "" {
				reason = cmd.Outcome.Code + ": " + cmd.Outcome.Summary
			} else if cmd.Outcome.Summary != "" {
				reason = cmd.Outcome.Summary
			} else {
				reason = cmd.Outcome.Code
			}
		}
		return common.RunStateFailed, reason, nil
	default:
		return "", "", fmt.Errorf("%w: terminal delivery has invalid outcome kind", common.ErrInvalidArgument)
	}
}

func validateSealedTerminalIdentity(current *runmodel.Run, cmd runevents.ApplyRuntimeTerminalDeliveryCommand) error {
	cmd = cmd.Normalized()
	if current.RuntimeContractVersion != runtimecontract.RuntimeContractVersionV2 || current.RuntimeState == nil {
		return fmt.Errorf("%w: sealed terminal delivery requires runtime v2 aggregate", common.ErrConflict)
	}
	state := current.RuntimeState
	expected := runtimecontract.TerminalOutcome{}
	if state.Termination != nil {
		switch state.Termination.Kind {
		case runtimecontract.RuntimeTerminationCancelled:
			expected.Kind = runtimecontract.TerminalOutcomeKindCancelled
		case runtimecontract.RuntimeTerminationSuperseded:
			expected.Kind = runtimecontract.TerminalOutcomeKindSuperseded
		default:
			return fmt.Errorf("%w: terminal delivery has invalid persisted administrative termination", common.ErrConflict)
		}
		expected.Code = "administrative_" + string(state.Termination.Kind)
		expected.Summary = state.Termination.Reason
	} else {
		if state.Decision.Terminal == nil {
			return fmt.Errorf("%w: sealed terminal delivery requires persisted terminal decision", common.ErrConflict)
		}
		switch state.Decision.State {
		case runtimecontract.ConvergenceStateComplete:
			expected.Kind = runtimecontract.TerminalOutcomeKindCompleted
		case runtimecontract.ConvergenceStateFailed:
			expected.Kind = runtimecontract.TerminalOutcomeKindFailed
		default:
			return fmt.Errorf("%w: terminal delivery has non-terminal convergence state", common.ErrConflict)
		}
		expected.Code = state.Decision.Terminal.Code
		expected.Summary = state.Decision.Terminal.Summary
	}
	if expected.Normalized() != cmd.Outcome.Normalized() {
		return fmt.Errorf("%w: terminal delivery outcome mismatch", common.ErrConflict)
	}
	if cmd.AuthoritySnapshotID == "" || cmd.DefinitionRevision == "" || cmd.EvidenceRevision == "" {
		return fmt.Errorf("%w: terminal delivery requires complete sealed identity", common.ErrConflict)
	}
	if cmd.AuthoritySnapshotID != strings.TrimSpace(state.Decision.AuthoritySnapshot.ID) {
		return fmt.Errorf("%w: terminal delivery authority snapshot mismatch", common.ErrConflict)
	}
	if cmd.DefinitionRevision != strings.TrimSpace(state.Facts.DefinitionRevision) ||
		cmd.EvidenceRevision != strings.TrimSpace(state.Facts.EvidenceRevision) {
		return fmt.Errorf("%w: terminal delivery definition revision mismatch", common.ErrConflict)
	}
	return nil
}

// validateRuntimeFactIdentity verifies a cross-owner governance fact against
// the Runtime V2 snapshot already persisted by the Run owner. It deliberately
// does not load Orchestrator state: Run can accept only the sealed identity it
// already owns, otherwise it records no lifecycle transition.
func validateRuntimeFactIdentity(current *runmodel.Run, authoritySnapshotID, definitionRevision, evidenceRevision, dispatchID string) error {
	if current == nil || current.RuntimeContractVersion != runtimecontract.RuntimeContractVersionV2 || current.RuntimeState == nil {
		return fmt.Errorf("%w: runtime lifecycle fact requires persisted runtime v2 state", common.ErrConflict)
	}
	state := current.RuntimeState
	if strings.TrimSpace(authoritySnapshotID) == "" || strings.TrimSpace(definitionRevision) == "" ||
		strings.TrimSpace(evidenceRevision) == "" || strings.TrimSpace(dispatchID) == "" {
		return fmt.Errorf("%w: runtime lifecycle fact has incomplete sealed identity", common.ErrConflict)
	}
	if definitionRevision != strings.TrimSpace(state.Facts.DefinitionRevision) ||
		evidenceRevision != strings.TrimSpace(state.Facts.EvidenceRevision) {
		return fmt.Errorf("%w: runtime lifecycle fact identity mismatch", common.ErrConflict)
	}
	return nil
}

func hasPersistedGovernanceRequest(state *runtimecontract.RuntimeStateRecord, ticketID common.TicketID, authoritySnapshotID, dispatchID string) bool {
	if state == nil {
		return false
	}
	for _, request := range state.GovernanceRequests {
		if request.TicketID == ticketID && strings.TrimSpace(request.AuthoritySnapshotID) == authoritySnapshotID && strings.TrimSpace(request.DispatchID) == dispatchID {
			return true
		}
	}
	return false
}

func hasPersistedGovernanceResolution(state *runtimecontract.RuntimeStateRecord, resolutionID common.ResolutionID, ticketID common.TicketID, authoritySnapshotID, dispatchID string) bool {
	if state == nil {
		return false
	}
	for _, resolution := range state.GovernanceResolutions {
		if resolution.ResolutionID == resolutionID && resolution.TicketID == ticketID &&
			strings.TrimSpace(resolution.AuthoritySnapshotID) == authoritySnapshotID && strings.TrimSpace(resolution.DispatchID) == dispatchID {
			return true
		}
	}
	return false
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
	}
	if transition.RuntimeActivationStatus != nil {
		next.RuntimeActivationStatus = transition.RuntimeActivationStatus.Clone()
	}
	if transition.ReconciliationRequestID != "" {
		next.ReconciliationRequestID = strings.TrimSpace(transition.ReconciliationRequestID)
	}
	if transition.Target == common.RunStateSuperseded {
		lineage := next.Lineage
		if strings.TrimSpace(lineage.LineageState) == "" || lineage.LineageState == "active" {
			lineage.LineageState = "superseded"
			next.Lineage = lineage
		}
	}
	return &next
}

func sameRuntimeActivationStatus(left, right *runmodel.RuntimeActivationStatus) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	if left.Disposition != right.Disposition || left.RuntimeStateRef != right.RuntimeStateRef ||
		left.ActualSchemaVersion != right.ActualSchemaVersion || left.SupportedSchemaVersion != right.SupportedSchemaVersion {
		return false
	}
	if left.Feedback == nil || right.Feedback == nil {
		return left.Feedback == nil && right.Feedback == nil
	}
	return runtimecontract.StableRuntimeFeedbackHash(*left.Feedback) == runtimecontract.StableRuntimeFeedbackHash(*right.Feedback)
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
