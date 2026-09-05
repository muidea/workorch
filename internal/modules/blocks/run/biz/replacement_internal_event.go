package run

import (
	"context"
	"errors"
	"strings"
	"time"

	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type replacementInternalEventKind string

const (
	replacementInternalEventRequested                       replacementInternalEventKind = "replacement_requested"
	replacementInternalEventRecoveryClaimed                 replacementInternalEventKind = "replacement_recovery_claimed"
	replacementInternalEventRecoveryFailed                  replacementInternalEventKind = "replacement_recovery_failed"
	replacementInternalEventRecoveryRecovered               replacementInternalEventKind = "replacement_recovery_recovered"
	replacementInternalEventTerminationAcknowledged         replacementInternalEventKind = "termination_acknowledged"
	replacementInternalEventSuccessorMaterialized           replacementInternalEventKind = "successor_materialized"
	replacementInternalEventSuccessorActivated              replacementInternalEventKind = "successor_activated"
	replacementInternalEventClarificationResolutionObserved replacementInternalEventKind = "clarification_resolution_observed"
)

type replacementRecoveryClaimedFact struct {
	RequestID  string
	Generation uint64
	ClaimedAt  string
}

type replacementTerminationAcknowledgedFact struct {
	RequestID       string
	Generation      uint64
	Acknowledgement orchestratorevents.RequestRuntimeTerminationResult
	NextDueAt       string
}

type replacementRecoveryFailedFact struct {
	RequestID  string
	Generation uint64
	Stage      runmodel.ReplacementRecoveryStage
	FailedAt   string
	RetryDueAt string
	Error      string
}

type replacementRecoveryRecoveredFact struct {
	RequestID   string
	Generation  uint64
	RecoveredAt string
	RetryDueAt  string
}

type replacementSuccessorMaterializedFact struct {
	RequestID          string
	Generation         uint64
	SuccessorRunID     common.RunID
	TerminalDeliveryID string
}

type replacementSuccessorActivatedFact struct {
	RequestID          string
	Generation         uint64
	SuccessorRunID     common.RunID
	TerminalDeliveryID string
	AcknowledgedAt     string
}

type replacementClarificationResolutionFact struct {
	RequestID                    string
	Generation                   uint64
	ResolutionID                 common.ResolutionID
	ResolutionBindingHash        string
	ConflictKind                 decisionevents.RuntimeDefinitionClarificationConflictKind
	CurrentReplacementIntentID   string
	CurrentReplacementIntentHash string
	NextDueAt                    string
}

// replacementInternalEvent is the sole Run-owner input for ReplacementIntent
// progress. Cross-owner acknowledgements are first validated and detached into
// one of these facts; no caller can supply a generic mutation callback.
type replacementInternalEvent struct {
	Kind        replacementInternalEventKind
	WorkspaceID common.WorkspaceID
	RunID       common.RunID
	OccurredAt  string

	Requested               *runmodel.ReplacementIntent
	RecoveryClaimed         *replacementRecoveryClaimedFact
	RecoveryFailed          *replacementRecoveryFailedFact
	RecoveryRecovered       *replacementRecoveryRecoveredFact
	TerminationAcknowledged *replacementTerminationAcknowledgedFact
	SuccessorMaterialized   *replacementSuccessorMaterializedFact
	SuccessorActivated      *replacementSuccessorActivatedFact
	ClarificationResolution *replacementClarificationResolutionFact
}

type replacementTransition struct {
	Next   *runmodel.Run
	Intent *runmodel.ReplacementIntent
	Drive  bool
	Noop   bool
}

func normalizeReplacementInternalEvent(event replacementInternalEvent) (replacementInternalEvent, error) {
	event.OccurredAt = strings.TrimSpace(event.OccurredAt)
	if event.WorkspaceID == "" || event.RunID == "" || event.OccurredAt == "" {
		return replacementInternalEvent{}, common.ErrInvalidArgument
	}
	payloadCount := 0
	for _, present := range []bool{
		event.Requested != nil,
		event.RecoveryClaimed != nil,
		event.RecoveryFailed != nil,
		event.RecoveryRecovered != nil,
		event.TerminationAcknowledged != nil,
		event.SuccessorMaterialized != nil,
		event.SuccessorActivated != nil,
		event.ClarificationResolution != nil,
	} {
		if present {
			payloadCount++
		}
	}
	if payloadCount != 1 {
		return replacementInternalEvent{}, common.ErrInvalidArgument
	}
	switch event.Kind {
	case replacementInternalEventRequested:
		if event.Requested == nil || event.Requested.WorkspaceID != event.WorkspaceID || event.Requested.State != "" ||
			event.Requested.Recovery != (runmodel.ReplacementRecoveryLease{}) ||
			event.Requested.ClarificationResolutionID != "" || strings.TrimSpace(event.Requested.ClarificationResolutionBindingHash) != "" || strings.TrimSpace(event.Requested.ClarificationConflictKind) != "" ||
			strings.TrimSpace(event.Requested.ConflictingReplacementIntentID) != "" || strings.TrimSpace(event.Requested.ConflictingReplacementIntentHash) != "" ||
			event.Requested.SuccessorRunID != "" || strings.TrimSpace(event.Requested.TerminalDeliveryID) != "" || event.Requested.RuntimeStateRevision != 0 || strings.TrimSpace(event.Requested.AcknowledgedAt) != "" {
			return replacementInternalEvent{}, common.ErrInvalidArgument
		}
		if event.Requested.Bootstrap != nil {
			if err := runtimecontract.ValidateRuntimeBootstrapRaw(*event.Requested.Bootstrap); err != nil {
				return replacementInternalEvent{}, err
			}
		}
		intent := runmodel.CloneReplacementIntent(*event.Requested)
		event.Requested = &intent
	case replacementInternalEventRecoveryClaimed:
		if event.RecoveryClaimed == nil || strings.TrimSpace(event.RecoveryClaimed.RequestID) == "" || event.RecoveryClaimed.Generation == 0 || strings.TrimSpace(event.RecoveryClaimed.ClaimedAt) == "" {
			return replacementInternalEvent{}, common.ErrInvalidArgument
		}
		fact := *event.RecoveryClaimed
		fact.RequestID = strings.TrimSpace(fact.RequestID)
		fact.ClaimedAt = strings.TrimSpace(fact.ClaimedAt)
		event.RecoveryClaimed = &fact
	case replacementInternalEventRecoveryFailed:
		if event.RecoveryFailed == nil || strings.TrimSpace(event.RecoveryFailed.RequestID) == "" || event.RecoveryFailed.Generation == 0 || strings.TrimSpace(event.RecoveryFailed.FailedAt) == "" || strings.TrimSpace(event.RecoveryFailed.Error) == "" {
			return replacementInternalEvent{}, common.ErrInvalidArgument
		}
		fact := *event.RecoveryFailed
		fact.RequestID = strings.TrimSpace(fact.RequestID)
		fact.FailedAt = strings.TrimSpace(fact.FailedAt)
		fact.RetryDueAt = strings.TrimSpace(fact.RetryDueAt)
		fact.Error = strings.TrimSpace(fact.Error)
		switch fact.Stage {
		case runmodel.ReplacementRecoveryClarificationResolution, runmodel.ReplacementRecoveryAdministrativeTermination, runmodel.ReplacementRecoverySuccessorMaterialization:
		default:
			return replacementInternalEvent{}, common.ErrInvalidArgument
		}
		event.RecoveryFailed = &fact
	case replacementInternalEventRecoveryRecovered:
		if event.RecoveryRecovered == nil || strings.TrimSpace(event.RecoveryRecovered.RequestID) == "" || event.RecoveryRecovered.Generation == 0 || strings.TrimSpace(event.RecoveryRecovered.RecoveredAt) == "" || strings.TrimSpace(event.RecoveryRecovered.RetryDueAt) == "" {
			return replacementInternalEvent{}, common.ErrInvalidArgument
		}
		fact := *event.RecoveryRecovered
		fact.RequestID = strings.TrimSpace(fact.RequestID)
		fact.RecoveredAt = strings.TrimSpace(fact.RecoveredAt)
		fact.RetryDueAt = strings.TrimSpace(fact.RetryDueAt)
		event.RecoveryRecovered = &fact
	case replacementInternalEventTerminationAcknowledged:
		if event.TerminationAcknowledged == nil || strings.TrimSpace(event.TerminationAcknowledged.RequestID) == "" || event.TerminationAcknowledged.Generation == 0 || strings.TrimSpace(event.TerminationAcknowledged.NextDueAt) == "" || strings.TrimSpace(event.TerminationAcknowledged.Acknowledgement.TerminalDeliveryID) == "" || !event.TerminationAcknowledged.Acknowledgement.WorkspaceReleased || event.TerminationAcknowledged.Acknowledgement.StateRevision == 0 {
			return replacementInternalEvent{}, common.ErrInvalidArgument
		}
		fact := *event.TerminationAcknowledged
		fact.RequestID = strings.TrimSpace(fact.RequestID)
		fact.NextDueAt = strings.TrimSpace(fact.NextDueAt)
		fact.Acknowledgement.TerminalDeliveryID = strings.TrimSpace(fact.Acknowledgement.TerminalDeliveryID)
		event.TerminationAcknowledged = &fact
	case replacementInternalEventSuccessorMaterialized:
		if event.SuccessorMaterialized == nil || strings.TrimSpace(event.SuccessorMaterialized.RequestID) == "" || event.SuccessorMaterialized.Generation == 0 || event.SuccessorMaterialized.SuccessorRunID == "" || strings.TrimSpace(event.SuccessorMaterialized.TerminalDeliveryID) == "" {
			return replacementInternalEvent{}, common.ErrInvalidArgument
		}
		fact := *event.SuccessorMaterialized
		fact.RequestID = strings.TrimSpace(fact.RequestID)
		fact.TerminalDeliveryID = strings.TrimSpace(fact.TerminalDeliveryID)
		event.SuccessorMaterialized = &fact
	case replacementInternalEventSuccessorActivated:
		if event.SuccessorActivated == nil || strings.TrimSpace(event.SuccessorActivated.RequestID) == "" || event.SuccessorActivated.Generation == 0 || event.SuccessorActivated.SuccessorRunID == "" || strings.TrimSpace(event.SuccessorActivated.TerminalDeliveryID) == "" || strings.TrimSpace(event.SuccessorActivated.AcknowledgedAt) == "" {
			return replacementInternalEvent{}, common.ErrInvalidArgument
		}
		fact := *event.SuccessorActivated
		fact.RequestID = strings.TrimSpace(fact.RequestID)
		fact.TerminalDeliveryID = strings.TrimSpace(fact.TerminalDeliveryID)
		fact.AcknowledgedAt = strings.TrimSpace(fact.AcknowledgedAt)
		event.SuccessorActivated = &fact
	case replacementInternalEventClarificationResolutionObserved:
		if event.ClarificationResolution == nil || strings.TrimSpace(event.ClarificationResolution.RequestID) == "" || event.ClarificationResolution.Generation == 0 || strings.TrimSpace(event.ClarificationResolution.ResolutionBindingHash) == "" || strings.TrimSpace(event.ClarificationResolution.NextDueAt) == "" {
			return replacementInternalEvent{}, common.ErrInvalidArgument
		}
		fact := *event.ClarificationResolution
		fact.RequestID = strings.TrimSpace(fact.RequestID)
		fact.ResolutionBindingHash = strings.TrimSpace(fact.ResolutionBindingHash)
		fact.CurrentReplacementIntentID = strings.TrimSpace(fact.CurrentReplacementIntentID)
		fact.CurrentReplacementIntentHash = strings.TrimSpace(fact.CurrentReplacementIntentHash)
		fact.NextDueAt = strings.TrimSpace(fact.NextDueAt)
		switch fact.ConflictKind {
		case decisionevents.RuntimeDefinitionClarificationNoConflict:
			if fact.ResolutionID == "" || fact.CurrentReplacementIntentID != "" || fact.CurrentReplacementIntentHash != "" {
				return replacementInternalEvent{}, common.ErrInvalidArgument
			}
		case decisionevents.RuntimeDefinitionClarificationScopeMismatch:
			if fact.ResolutionID != "" || fact.CurrentReplacementIntentID != "" || fact.CurrentReplacementIntentHash != "" {
				return replacementInternalEvent{}, common.ErrInvalidArgument
			}
		case decisionevents.RuntimeDefinitionClarificationTicketAlreadyBound:
			if fact.ResolutionID != "" || fact.CurrentReplacementIntentID == "" || fact.CurrentReplacementIntentHash == "" {
				return replacementInternalEvent{}, common.ErrInvalidArgument
			}
		default:
			return replacementInternalEvent{}, common.ErrInvalidArgument
		}
		event.ClarificationResolution = &fact
	default:
		return replacementInternalEvent{}, common.ErrInvalidArgument
	}
	return event, nil
}

func reduceReplacementInternalEvent(current *runmodel.Run, event replacementInternalEvent) (replacementTransition, error) {
	if current == nil || current.WorkspaceID != event.WorkspaceID || current.ID != event.RunID {
		return replacementTransition{}, common.ErrConflict
	}
	if current.ReplacementIntent != nil && !current.ReplacementIntent.ValidFor(current) {
		return replacementTransition{}, common.ErrConflict
	}
	switch event.Kind {
	case replacementInternalEventRequested:
		return reduceReplacementRequested(current, *event.Requested, event.OccurredAt)
	case replacementInternalEventRecoveryClaimed:
		return reduceReplacementRecoveryClaimed(current, *event.RecoveryClaimed)
	case replacementInternalEventRecoveryFailed:
		return reduceReplacementRecoveryFailed(current, *event.RecoveryFailed)
	case replacementInternalEventRecoveryRecovered:
		return reduceReplacementRecoveryRecovered(current, *event.RecoveryRecovered)
	case replacementInternalEventTerminationAcknowledged:
		return reduceReplacementTerminationAcknowledged(current, *event.TerminationAcknowledged)
	case replacementInternalEventSuccessorMaterialized:
		return reduceReplacementSuccessorMaterialized(current, *event.SuccessorMaterialized)
	case replacementInternalEventSuccessorActivated:
		return reduceReplacementSuccessorActivated(current, *event.SuccessorActivated)
	case replacementInternalEventClarificationResolutionObserved:
		return reduceReplacementClarificationResolution(current, *event.ClarificationResolution)
	default:
		return replacementTransition{}, common.ErrInvalidArgument
	}
}

func reduceReplacementRequested(current *runmodel.Run, requested runmodel.ReplacementIntent, requestedAt string) (replacementTransition, error) {
	requested.State = runmodel.ReplacementIntentPending
	stage := runmodel.ReplacementRecoveryAdministrativeTermination
	if runStateIsTerminal(current.State) {
		if requested.ClarificationTicketID != "" || current.TerminalDelivery == nil || current.TerminalResultID == "" || current.RuntimeState == nil || current.RuntimeState.StateRevision == 0 {
			return replacementTransition{}, common.ErrConflict
		}
		requested.State = runmodel.ReplacementIntentTerminationAcknowledged
		requested.TerminalDeliveryID = strings.TrimSpace(current.TerminalDelivery.TerminalDeliveryID)
		requested.RuntimeStateRevision = current.RuntimeState.StateRevision
		stage = runmodel.ReplacementRecoverySuccessorMaterialization
	} else if requested.ClarificationTicketID != "" {
		requested.State = runmodel.ReplacementIntentPendingClarificationResolution
		stage = runmodel.ReplacementRecoveryClarificationResolution
	}
	requested.Recovery = newReplacementRecoveryLease(requested, stage, 1, requestedAt)
	if !requested.ValidFor(current) {
		return replacementTransition{}, common.ErrInvalidArgument
	}
	if current.ReplacementIntent != nil {
		if current.ReplacementIntent.RequestID == requested.RequestID {
			if !replacementIntentMatches(current.ReplacementIntent, requested) {
				return replacementTransition{}, common.ErrConflict
			}
			intent := runmodel.CloneReplacementIntent(*current.ReplacementIntent)
			return replacementTransition{Intent: &intent, Noop: true}, nil
		}
		if current.ReplacementIntent.State != runmodel.ReplacementIntentClarificationResolutionRejected || current.ReplacementIntent.ClarificationConflictKind != string(decisionevents.RuntimeDefinitionClarificationScopeMismatch) || !current.ReplacementIntent.ValidFor(current) {
			return replacementTransition{}, common.ErrConflict
		}
		for idx := range current.RetiredReplacementIntents {
			if current.RetiredReplacementIntents[idx].RequestID == requested.RequestID {
				return replacementTransition{}, common.ErrConflict
			}
		}
	}
	next := runmodel.CloneRun(current)
	if next.ReplacementIntent != nil {
		next.RetiredReplacementIntents = append(next.RetiredReplacementIntents, runmodel.CloneReplacementIntent(*next.ReplacementIntent))
	}
	intent := runmodel.CloneReplacementIntent(requested)
	next.ReplacementIntent = &intent
	if !intent.ValidFor(next) {
		return replacementTransition{}, common.ErrConflict
	}
	return replacementTransition{Next: next, Intent: cloneReplacementIntentPointer(&intent)}, nil
}

func reduceReplacementRecoveryClaimed(current *runmodel.Run, fact replacementRecoveryClaimedFact) (replacementTransition, error) {
	if current.ReplacementIntent == nil {
		return replacementTransition{Noop: true}, nil
	}
	if !current.ReplacementIntent.ValidFor(current) {
		return replacementTransition{}, common.ErrConflict
	}
	lease := current.ReplacementIntent.Recovery
	if lease.RequestID != fact.RequestID || lease.Generation != fact.Generation || lease.State != runmodel.ReplacementRecoveryScheduled {
		intent := runmodel.CloneReplacementIntent(*current.ReplacementIntent)
		return replacementTransition{Intent: &intent, Noop: true}, nil
	}
	if replacementRecoveryDueAfter(lease.DueAt, fact.ClaimedAt) {
		intent := runmodel.CloneReplacementIntent(*current.ReplacementIntent)
		return replacementTransition{Intent: &intent, Noop: true}, nil
	}
	next := runmodel.CloneRun(current)
	next.ReplacementIntent.Recovery.State = runmodel.ReplacementRecoveryRunning
	next.ReplacementIntent.Recovery.DueAt = ""
	next.ReplacementIntent.Recovery.LastError = ""
	if !next.ReplacementIntent.ValidFor(next) {
		return replacementTransition{}, common.ErrConflict
	}
	return replacementTransition{Next: next, Intent: cloneReplacementIntentPointer(next.ReplacementIntent), Drive: true}, nil
}

func reduceReplacementTerminationAcknowledged(current *runmodel.Run, fact replacementTerminationAcknowledgedFact) (replacementTransition, error) {
	if current.ReplacementIntent == nil || current.ReplacementIntent.RequestID != fact.RequestID {
		return replacementTransition{}, common.ErrConflict
	}
	if current.ReplacementIntent.State == runmodel.ReplacementIntentTerminationAcknowledged || current.ReplacementIntent.State == runmodel.ReplacementIntentSuccessorCreated || current.ReplacementIntent.State == runmodel.ReplacementIntentCompleted {
		if current.ReplacementIntent.TerminalDeliveryID != fact.Acknowledgement.TerminalDeliveryID || current.ReplacementIntent.RuntimeStateRevision != fact.Acknowledgement.StateRevision {
			return replacementTransition{}, common.ErrConflict
		}
		intent := runmodel.CloneReplacementIntent(*current.ReplacementIntent)
		return replacementTransition{Intent: &intent, Noop: true}, nil
	}
	if current.ReplacementIntent.State != runmodel.ReplacementIntentPending && current.ReplacementIntent.State != runmodel.ReplacementIntentClarificationResolutionBound {
		return replacementTransition{}, common.ErrConflict
	}
	if current.ReplacementIntent.Recovery.State != runmodel.ReplacementRecoveryRunning || current.ReplacementIntent.Recovery.Stage != runmodel.ReplacementRecoveryAdministrativeTermination || current.ReplacementIntent.Recovery.Generation != fact.Generation {
		return replacementTransition{}, common.ErrConflict
	}
	next := runmodel.CloneRun(current)
	next.ReplacementIntent.State = runmodel.ReplacementIntentTerminationAcknowledged
	next.ReplacementIntent.TerminalDeliveryID = fact.Acknowledgement.TerminalDeliveryID
	next.ReplacementIntent.RuntimeStateRevision = fact.Acknowledgement.StateRevision
	next.ReplacementIntent.Recovery = nextReplacementRecoveryStage(*next.ReplacementIntent, runmodel.ReplacementRecoverySuccessorMaterialization, fact.NextDueAt)
	if !next.ReplacementIntent.ValidFor(next) {
		return replacementTransition{}, common.ErrConflict
	}
	return replacementTransition{Next: next, Intent: cloneReplacementIntentPointer(next.ReplacementIntent)}, nil
}

func reduceReplacementRecoveryFailed(current *runmodel.Run, fact replacementRecoveryFailedFact) (replacementTransition, error) {
	if current.ReplacementIntent == nil || current.ReplacementIntent.RequestID != fact.RequestID {
		return replacementTransition{}, common.ErrConflict
	}
	if current.ReplacementIntent.State == runmodel.ReplacementIntentCompleted || current.ReplacementIntent.State == runmodel.ReplacementIntentClarificationResolutionRejected {
		intent := runmodel.CloneReplacementIntent(*current.ReplacementIntent)
		return replacementTransition{Intent: &intent, Noop: true}, nil
	}
	lease := current.ReplacementIntent.Recovery
	if lease.State != runmodel.ReplacementRecoveryRunning || lease.RequestID != fact.RequestID || lease.Generation != fact.Generation || lease.Stage != fact.Stage {
		return replacementTransition{}, common.ErrConflict
	}
	next := runmodel.CloneRun(current)
	next.ReplacementIntent.Recovery.Attempt++
	next.ReplacementIntent.Recovery.LastError = fact.Error
	next.ReplacementIntent.Recovery.Generation++
	if next.ReplacementIntent.Recovery.Attempt >= next.ReplacementIntent.Recovery.MaxAttempts {
		next.ReplacementIntent.Recovery.State = runmodel.ReplacementRecoveryExhausted
		next.ReplacementIntent.Recovery.DueAt = ""
	} else {
		if strings.TrimSpace(fact.RetryDueAt) == "" {
			return replacementTransition{}, common.ErrInvalidArgument
		}
		next.ReplacementIntent.Recovery.State = runmodel.ReplacementRecoveryScheduled
		next.ReplacementIntent.Recovery.DueAt = fact.RetryDueAt
	}
	if !next.ReplacementIntent.ValidFor(next) {
		return replacementTransition{}, common.ErrConflict
	}
	return replacementTransition{Next: next, Intent: cloneReplacementIntentPointer(next.ReplacementIntent)}, nil
}

func reduceReplacementRecoveryRecovered(current *runmodel.Run, fact replacementRecoveryRecoveredFact) (replacementTransition, error) {
	if current.ReplacementIntent == nil || current.ReplacementIntent.RequestID != fact.RequestID {
		return replacementTransition{}, common.ErrConflict
	}
	if !current.ReplacementIntent.ValidFor(current) {
		return replacementTransition{}, common.ErrConflict
	}
	lease := current.ReplacementIntent.Recovery
	if lease.Generation != fact.Generation || lease.State != runmodel.ReplacementRecoveryRunning {
		intent := runmodel.CloneReplacementIntent(*current.ReplacementIntent)
		return replacementTransition{Intent: &intent, Noop: true}, nil
	}
	next := runmodel.CloneRun(current)
	next.ReplacementIntent.Recovery.State = runmodel.ReplacementRecoveryScheduled
	next.ReplacementIntent.Recovery.Generation++
	next.ReplacementIntent.Recovery.DueAt = fact.RetryDueAt
	next.ReplacementIntent.Recovery.LastError = "owner process restarted during replacement recovery"
	if !next.ReplacementIntent.ValidFor(next) {
		return replacementTransition{}, common.ErrConflict
	}
	return replacementTransition{Next: next, Intent: cloneReplacementIntentPointer(next.ReplacementIntent)}, nil
}

func reduceReplacementSuccessorMaterialized(current *runmodel.Run, fact replacementSuccessorMaterializedFact) (replacementTransition, error) {
	if current.ReplacementIntent == nil || current.ReplacementIntent.RequestID != fact.RequestID || !runStateIsTerminal(current.State) || current.TerminalDelivery == nil || strings.TrimSpace(current.TerminalDelivery.TerminalDeliveryID) != fact.TerminalDeliveryID {
		return replacementTransition{}, common.ErrConflict
	}
	if current.ReplacementIntent.Recovery.State != runmodel.ReplacementRecoveryRunning || current.ReplacementIntent.Recovery.Stage != runmodel.ReplacementRecoverySuccessorMaterialization || current.ReplacementIntent.Recovery.Generation != fact.Generation {
		return replacementTransition{}, common.ErrConflict
	}
	if current.ReplacementIntent.State == runmodel.ReplacementIntentSuccessorCreated && current.ReplacementIntent.SuccessorRunID == fact.SuccessorRunID && current.Lineage.SuccessorRunID == fact.SuccessorRunID {
		intent := runmodel.CloneReplacementIntent(*current.ReplacementIntent)
		return replacementTransition{Intent: &intent, Noop: true}, nil
	}
	if current.ReplacementIntent.State != runmodel.ReplacementIntentTerminationAcknowledged || current.ReplacementIntent.SuccessorRunID != "" {
		return replacementTransition{}, common.ErrConflict
	}
	next := runmodel.CloneRun(current)
	next.ReplacementIntent.State = runmodel.ReplacementIntentSuccessorCreated
	next.ReplacementIntent.SuccessorRunID = fact.SuccessorRunID
	next.ReplacementIntent.TerminalDeliveryID = fact.TerminalDeliveryID
	next.Lineage.SuccessorRunID = fact.SuccessorRunID
	next.Lineage.LineageState = runmodel.RunLineageStateSuperseded
	if !next.ReplacementIntent.ValidFor(next) {
		return replacementTransition{}, common.ErrConflict
	}
	return replacementTransition{Next: next, Intent: cloneReplacementIntentPointer(next.ReplacementIntent)}, nil
}

func reduceReplacementSuccessorActivated(current *runmodel.Run, fact replacementSuccessorActivatedFact) (replacementTransition, error) {
	if current.ReplacementIntent != nil && current.ReplacementIntent.State == runmodel.ReplacementIntentCompleted &&
		current.ReplacementIntent.RequestID == fact.RequestID && current.ReplacementIntent.SuccessorRunID == fact.SuccessorRunID &&
		current.ReplacementIntent.TerminalDeliveryID == fact.TerminalDeliveryID {
		intent := runmodel.CloneReplacementIntent(*current.ReplacementIntent)
		return replacementTransition{Intent: &intent, Noop: true}, nil
	}
	if current.ReplacementIntent == nil || current.ReplacementIntent.RequestID != fact.RequestID || current.ReplacementIntent.State != runmodel.ReplacementIntentSuccessorCreated ||
		current.ReplacementIntent.SuccessorRunID != fact.SuccessorRunID || current.ReplacementIntent.TerminalDeliveryID != fact.TerminalDeliveryID ||
		current.ReplacementIntent.Recovery.State != runmodel.ReplacementRecoveryRunning || current.ReplacementIntent.Recovery.Stage != runmodel.ReplacementRecoverySuccessorMaterialization || current.ReplacementIntent.Recovery.Generation != fact.Generation {
		return replacementTransition{}, common.ErrConflict
	}
	next := runmodel.CloneRun(current)
	next.ReplacementIntent.State = runmodel.ReplacementIntentCompleted
	next.ReplacementIntent.AcknowledgedAt = fact.AcknowledgedAt
	next.ReplacementIntent.Recovery.State = runmodel.ReplacementRecoveryCompleted
	next.ReplacementIntent.Recovery.DueAt = ""
	next.ReplacementIntent.Recovery.LastError = ""
	if !next.ReplacementIntent.ValidFor(next) {
		return replacementTransition{}, common.ErrConflict
	}
	return replacementTransition{Next: next, Intent: cloneReplacementIntentPointer(next.ReplacementIntent)}, nil
}

func reduceReplacementClarificationResolution(current *runmodel.Run, fact replacementClarificationResolutionFact) (replacementTransition, error) {
	if current.ReplacementIntent == nil || current.ReplacementIntent.RequestID != fact.RequestID {
		return replacementTransition{}, common.ErrConflict
	}
	intent := current.ReplacementIntent
	if intent.Recovery.State != runmodel.ReplacementRecoveryRunning || intent.Recovery.Stage != runmodel.ReplacementRecoveryClarificationResolution || intent.Recovery.Generation != fact.Generation {
		if (intent.State == runmodel.ReplacementIntentClarificationResolutionBound || intent.State == runmodel.ReplacementIntentClarificationResolutionRejected) && intent.Recovery.Generation > fact.Generation {
			return replacementTransition{Intent: cloneReplacementIntentPointer(intent), Noop: true}, nil
		}
		return replacementTransition{}, common.ErrConflict
	}
	if fact.ConflictKind == decisionevents.RuntimeDefinitionClarificationNoConflict {
		if intent.State == runmodel.ReplacementIntentClarificationResolutionBound && intent.ClarificationResolutionID == fact.ResolutionID && intent.ClarificationResolutionBindingHash == fact.ResolutionBindingHash {
			return replacementTransition{Intent: cloneReplacementIntentPointer(intent), Noop: true}, nil
		}
		if intent.State != runmodel.ReplacementIntentPendingClarificationResolution {
			return replacementTransition{}, common.ErrConflict
		}
		next := runmodel.CloneRun(current)
		next.ReplacementIntent.State = runmodel.ReplacementIntentClarificationResolutionBound
		next.ReplacementIntent.ClarificationResolutionID = fact.ResolutionID
		next.ReplacementIntent.ClarificationResolutionBindingHash = fact.ResolutionBindingHash
		next.ReplacementIntent.ClarificationConflictKind = ""
		next.ReplacementIntent.ConflictingReplacementIntentID = ""
		next.ReplacementIntent.ConflictingReplacementIntentHash = ""
		next.ReplacementIntent.Recovery = nextReplacementRecoveryStage(*next.ReplacementIntent, runmodel.ReplacementRecoveryAdministrativeTermination, fact.NextDueAt)
		if !next.ReplacementIntent.ValidFor(next) {
			return replacementTransition{}, common.ErrConflict
		}
		return replacementTransition{Next: next, Intent: cloneReplacementIntentPointer(next.ReplacementIntent)}, nil
	}
	if intent.State == runmodel.ReplacementIntentClarificationResolutionRejected && intent.ClarificationResolutionBindingHash == fact.ResolutionBindingHash && intent.ClarificationConflictKind == string(fact.ConflictKind) && intent.ConflictingReplacementIntentID == fact.CurrentReplacementIntentID && intent.ConflictingReplacementIntentHash == fact.CurrentReplacementIntentHash {
		return replacementTransition{Intent: cloneReplacementIntentPointer(intent), Noop: true}, nil
	}
	if intent.State != runmodel.ReplacementIntentPendingClarificationResolution {
		return replacementTransition{}, common.ErrConflict
	}
	next := runmodel.CloneRun(current)
	next.ReplacementIntent.State = runmodel.ReplacementIntentClarificationResolutionRejected
	next.ReplacementIntent.ClarificationResolutionBindingHash = fact.ResolutionBindingHash
	next.ReplacementIntent.ClarificationConflictKind = string(fact.ConflictKind)
	next.ReplacementIntent.ConflictingReplacementIntentID = fact.CurrentReplacementIntentID
	next.ReplacementIntent.ConflictingReplacementIntentHash = fact.CurrentReplacementIntentHash
	next.ReplacementIntent.Recovery.State = runmodel.ReplacementRecoveryCompleted
	next.ReplacementIntent.Recovery.DueAt = ""
	next.ReplacementIntent.Recovery.LastError = ""
	if !next.ReplacementIntent.ValidFor(next) {
		return replacementTransition{}, common.ErrConflict
	}
	return replacementTransition{Next: next, Intent: cloneReplacementIntentPointer(next.ReplacementIntent)}, nil
}

const replacementRecoveryMaxAttempts uint64 = 8

func newReplacementRecoveryLease(intent runmodel.ReplacementIntent, stage runmodel.ReplacementRecoveryStage, generation uint64, dueAt string) runmodel.ReplacementRecoveryLease {
	return runmodel.ReplacementRecoveryLease{
		RequestID: intent.RequestID, IntentHash: runmodel.StableReplacementIntentHash(intent),
		Stage: stage, State: runmodel.ReplacementRecoveryScheduled, Generation: generation,
		MaxAttempts: replacementRecoveryMaxAttempts, DueAt: strings.TrimSpace(dueAt),
	}
}

func nextReplacementRecoveryStage(intent runmodel.ReplacementIntent, stage runmodel.ReplacementRecoveryStage, dueAt string) runmodel.ReplacementRecoveryLease {
	generation := intent.Recovery.Generation + 1
	return newReplacementRecoveryLease(intent, stage, generation, dueAt)
}

func replacementRecoveryDueAfter(dueAt, observedAt string) bool {
	due, dueErr := time.Parse(time.RFC3339Nano, strings.TrimSpace(dueAt))
	observed, observedErr := time.Parse(time.RFC3339Nano, strings.TrimSpace(observedAt))
	return dueErr != nil || observedErr != nil || due.After(observed)
}

func cloneReplacementIntentPointer(intent *runmodel.ReplacementIntent) *runmodel.ReplacementIntent {
	if intent == nil {
		return nil
	}
	clone := runmodel.CloneReplacementIntent(*intent)
	return &clone
}

func (m *Biz) submitReplacementInternalEvent(ctx context.Context, runs repository.RunRepository, event replacementInternalEvent) (replacementTransition, error) {
	if m == nil || runs == nil {
		return replacementTransition{}, common.ErrInvalidArgument
	}
	event, err := normalizeReplacementInternalEvent(event)
	if err != nil {
		return replacementTransition{}, err
	}
	bound, err := common.BindRuntimeWorkspaceContext(ctx, event.WorkspaceID)
	if err != nil {
		return replacementTransition{}, err
	}
	for attempt := 0; attempt < 8; attempt++ {
		current, err := runs.GetByWorkspaceID(bound, event.WorkspaceID, event.RunID)
		if err != nil {
			return replacementTransition{}, err
		}
		transition, err := reduceReplacementInternalEvent(current, event)
		if err != nil {
			return replacementTransition{}, err
		}
		if transition.Noop {
			return transition, nil
		}
		if transition.Next == nil {
			return replacementTransition{}, common.ErrInvalidArgument
		}
		if err := runs.CompareAndSwap(bound, transition.Next, current.Revision); err != nil {
			if errors.Is(err, common.ErrConflict) {
				continue
			}
			return replacementTransition{}, err
		}
		return transition, nil
	}
	return replacementTransition{}, common.ErrConflict
}
