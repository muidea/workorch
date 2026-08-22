package run

import (
	"context"
	"errors"
	"strings"

	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
)

type replacementInternalEventKind string

const (
	replacementInternalEventRequested                       replacementInternalEventKind = "replacement_requested"
	replacementInternalEventTerminationAttemptClaimed       replacementInternalEventKind = "termination_attempt_claimed"
	replacementInternalEventTerminationAcknowledged         replacementInternalEventKind = "termination_acknowledged"
	replacementInternalEventTerminationFailed               replacementInternalEventKind = "termination_failed"
	replacementInternalEventSuccessorMaterialized           replacementInternalEventKind = "successor_materialized"
	replacementInternalEventClarificationResolutionObserved replacementInternalEventKind = "clarification_resolution_observed"
)

type replacementTerminationAttemptFact struct {
	AttemptedAt string
}

type replacementTerminationAcknowledgedFact struct {
	RequestID       string
	Acknowledgement orchestratorevents.RequestRuntimeTerminationResult
	AcknowledgedAt  string
}

type replacementTerminationFailedFact struct {
	RequestID string
	Error     string
}

type replacementSuccessorMaterializedFact struct {
	RequestID          string
	SuccessorRunID     common.RunID
	TerminalDeliveryID string
}

type replacementClarificationResolutionFact struct {
	RequestID                    string
	ResolutionID                 common.ResolutionID
	ResolutionBindingHash        string
	ConflictKind                 decisionevents.RuntimeDefinitionClarificationConflictKind
	CurrentReplacementIntentID   string
	CurrentReplacementIntentHash string
}

// replacementInternalEvent is the sole Run-owner input for ReplacementIntent
// progress. Cross-owner acknowledgements are first validated and detached into
// one of these facts; no caller can supply a generic mutation callback.
type replacementInternalEvent struct {
	Kind        replacementInternalEventKind
	WorkspaceID common.WorkspaceID
	RunID       common.RunID

	Requested               *runmodel.ReplacementIntent
	TerminationAttempt      *replacementTerminationAttemptFact
	TerminationAcknowledged *replacementTerminationAcknowledgedFact
	TerminationFailed       *replacementTerminationFailedFact
	SuccessorMaterialized   *replacementSuccessorMaterializedFact
	ClarificationResolution *replacementClarificationResolutionFact
}

type replacementTransition struct {
	Next    *runmodel.Run
	Intent  *runmodel.ReplacementIntent
	Deliver bool
	Noop    bool
}

func normalizeReplacementInternalEvent(event replacementInternalEvent) (replacementInternalEvent, error) {
	if event.WorkspaceID == "" || event.RunID == "" {
		return replacementInternalEvent{}, common.ErrInvalidArgument
	}
	payloadCount := 0
	for _, present := range []bool{
		event.Requested != nil,
		event.TerminationAttempt != nil,
		event.TerminationAcknowledged != nil,
		event.TerminationFailed != nil,
		event.SuccessorMaterialized != nil,
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
			event.Requested.AttemptCount != 0 || strings.TrimSpace(event.Requested.LastAttemptAt) != "" || strings.TrimSpace(event.Requested.LastError) != "" ||
			event.Requested.ClarificationResolutionID != "" || strings.TrimSpace(event.Requested.ClarificationResolutionBindingHash) != "" || strings.TrimSpace(event.Requested.ClarificationConflictKind) != "" ||
			strings.TrimSpace(event.Requested.ConflictingReplacementIntentID) != "" || strings.TrimSpace(event.Requested.ConflictingReplacementIntentHash) != "" ||
			event.Requested.SuccessorRunID != "" || strings.TrimSpace(event.Requested.TerminalDeliveryID) != "" || event.Requested.RuntimeStateRevision != 0 || strings.TrimSpace(event.Requested.AcknowledgedAt) != "" {
			return replacementInternalEvent{}, common.ErrInvalidArgument
		}
		intent := runmodel.CloneReplacementIntent(*event.Requested)
		event.Requested = &intent
	case replacementInternalEventTerminationAttemptClaimed:
		if event.TerminationAttempt == nil || strings.TrimSpace(event.TerminationAttempt.AttemptedAt) == "" {
			return replacementInternalEvent{}, common.ErrInvalidArgument
		}
		fact := *event.TerminationAttempt
		fact.AttemptedAt = strings.TrimSpace(fact.AttemptedAt)
		event.TerminationAttempt = &fact
	case replacementInternalEventTerminationAcknowledged:
		if event.TerminationAcknowledged == nil || strings.TrimSpace(event.TerminationAcknowledged.RequestID) == "" || strings.TrimSpace(event.TerminationAcknowledged.AcknowledgedAt) == "" || strings.TrimSpace(event.TerminationAcknowledged.Acknowledgement.TerminalDeliveryID) == "" || !event.TerminationAcknowledged.Acknowledgement.WorkspaceReleased || event.TerminationAcknowledged.Acknowledgement.StateRevision == 0 {
			return replacementInternalEvent{}, common.ErrInvalidArgument
		}
		fact := *event.TerminationAcknowledged
		fact.RequestID = strings.TrimSpace(fact.RequestID)
		fact.AcknowledgedAt = strings.TrimSpace(fact.AcknowledgedAt)
		fact.Acknowledgement.TerminalDeliveryID = strings.TrimSpace(fact.Acknowledgement.TerminalDeliveryID)
		event.TerminationAcknowledged = &fact
	case replacementInternalEventTerminationFailed:
		if event.TerminationFailed == nil || strings.TrimSpace(event.TerminationFailed.RequestID) == "" || strings.TrimSpace(event.TerminationFailed.Error) == "" {
			return replacementInternalEvent{}, common.ErrInvalidArgument
		}
		fact := *event.TerminationFailed
		fact.RequestID = strings.TrimSpace(fact.RequestID)
		fact.Error = strings.TrimSpace(fact.Error)
		event.TerminationFailed = &fact
	case replacementInternalEventSuccessorMaterialized:
		if event.SuccessorMaterialized == nil || strings.TrimSpace(event.SuccessorMaterialized.RequestID) == "" || event.SuccessorMaterialized.SuccessorRunID == "" || strings.TrimSpace(event.SuccessorMaterialized.TerminalDeliveryID) == "" {
			return replacementInternalEvent{}, common.ErrInvalidArgument
		}
		fact := *event.SuccessorMaterialized
		fact.RequestID = strings.TrimSpace(fact.RequestID)
		fact.TerminalDeliveryID = strings.TrimSpace(fact.TerminalDeliveryID)
		event.SuccessorMaterialized = &fact
	case replacementInternalEventClarificationResolutionObserved:
		if event.ClarificationResolution == nil || strings.TrimSpace(event.ClarificationResolution.RequestID) == "" || strings.TrimSpace(event.ClarificationResolution.ResolutionBindingHash) == "" {
			return replacementInternalEvent{}, common.ErrInvalidArgument
		}
		fact := *event.ClarificationResolution
		fact.RequestID = strings.TrimSpace(fact.RequestID)
		fact.ResolutionBindingHash = strings.TrimSpace(fact.ResolutionBindingHash)
		fact.CurrentReplacementIntentID = strings.TrimSpace(fact.CurrentReplacementIntentID)
		fact.CurrentReplacementIntentHash = strings.TrimSpace(fact.CurrentReplacementIntentHash)
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
		return reduceReplacementRequested(current, *event.Requested)
	case replacementInternalEventTerminationAttemptClaimed:
		return reduceReplacementTerminationAttempt(current, *event.TerminationAttempt)
	case replacementInternalEventTerminationAcknowledged:
		return reduceReplacementTerminationAcknowledged(current, *event.TerminationAcknowledged)
	case replacementInternalEventTerminationFailed:
		return reduceReplacementTerminationFailed(current, *event.TerminationFailed)
	case replacementInternalEventSuccessorMaterialized:
		return reduceReplacementSuccessorMaterialized(current, *event.SuccessorMaterialized)
	case replacementInternalEventClarificationResolutionObserved:
		return reduceReplacementClarificationResolution(current, *event.ClarificationResolution)
	default:
		return replacementTransition{}, common.ErrInvalidArgument
	}
}

func reduceReplacementRequested(current *runmodel.Run, requested runmodel.ReplacementIntent) (replacementTransition, error) {
	requested.State = runmodel.ReplacementIntentPending
	if requested.ClarificationTicketID != "" {
		requested.State = runmodel.ReplacementIntentPendingClarificationResolution
	}
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
	if runStateIsTerminal(current.State) {
		return replacementTransition{}, common.ErrConflict
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

func reduceReplacementTerminationAttempt(current *runmodel.Run, fact replacementTerminationAttemptFact) (replacementTransition, error) {
	if current.ReplacementIntent == nil {
		return replacementTransition{Noop: true}, nil
	}
	if !current.ReplacementIntent.ValidFor(current) {
		return replacementTransition{}, common.ErrConflict
	}
	if !replacementIntentMayDeliver(current.ReplacementIntent) {
		intent := runmodel.CloneReplacementIntent(*current.ReplacementIntent)
		return replacementTransition{Intent: &intent, Noop: true}, nil
	}
	next := runmodel.CloneRun(current)
	next.ReplacementIntent.AttemptCount++
	next.ReplacementIntent.LastAttemptAt = fact.AttemptedAt
	if !next.ReplacementIntent.ValidFor(next) {
		return replacementTransition{}, common.ErrConflict
	}
	return replacementTransition{Next: next, Intent: cloneReplacementIntentPointer(next.ReplacementIntent), Deliver: true}, nil
}

func reduceReplacementTerminationAcknowledged(current *runmodel.Run, fact replacementTerminationAcknowledgedFact) (replacementTransition, error) {
	if current.ReplacementIntent == nil || current.ReplacementIntent.RequestID != fact.RequestID {
		return replacementTransition{}, common.ErrConflict
	}
	if current.ReplacementIntent.State == runmodel.ReplacementIntentCompleted {
		if current.ReplacementIntent.TerminalDeliveryID != fact.Acknowledgement.TerminalDeliveryID || current.ReplacementIntent.RuntimeStateRevision != fact.Acknowledgement.StateRevision {
			return replacementTransition{}, common.ErrConflict
		}
		intent := runmodel.CloneReplacementIntent(*current.ReplacementIntent)
		return replacementTransition{Intent: &intent, Noop: true}, nil
	}
	if current.ReplacementIntent.State != runmodel.ReplacementIntentSuccessorCreated {
		return replacementTransition{}, common.ErrConflict
	}
	next := runmodel.CloneRun(current)
	next.ReplacementIntent.State = runmodel.ReplacementIntentCompleted
	next.ReplacementIntent.LastError = ""
	next.ReplacementIntent.TerminalDeliveryID = fact.Acknowledgement.TerminalDeliveryID
	next.ReplacementIntent.RuntimeStateRevision = fact.Acknowledgement.StateRevision
	next.ReplacementIntent.AcknowledgedAt = fact.AcknowledgedAt
	if !next.ReplacementIntent.ValidFor(next) {
		return replacementTransition{}, common.ErrConflict
	}
	return replacementTransition{Next: next, Intent: cloneReplacementIntentPointer(next.ReplacementIntent)}, nil
}

func reduceReplacementTerminationFailed(current *runmodel.Run, fact replacementTerminationFailedFact) (replacementTransition, error) {
	if current.ReplacementIntent == nil || current.ReplacementIntent.RequestID != fact.RequestID {
		return replacementTransition{}, common.ErrConflict
	}
	if current.ReplacementIntent.State == runmodel.ReplacementIntentCompleted {
		intent := runmodel.CloneReplacementIntent(*current.ReplacementIntent)
		return replacementTransition{Intent: &intent, Noop: true}, nil
	}
	if !replacementIntentMayDeliver(current.ReplacementIntent) {
		return replacementTransition{}, common.ErrConflict
	}
	if current.ReplacementIntent.LastError == fact.Error {
		intent := runmodel.CloneReplacementIntent(*current.ReplacementIntent)
		return replacementTransition{Intent: &intent, Noop: true}, nil
	}
	next := runmodel.CloneRun(current)
	next.ReplacementIntent.LastError = fact.Error
	if !next.ReplacementIntent.ValidFor(next) {
		return replacementTransition{}, common.ErrConflict
	}
	return replacementTransition{Next: next, Intent: cloneReplacementIntentPointer(next.ReplacementIntent)}, nil
}

func reduceReplacementSuccessorMaterialized(current *runmodel.Run, fact replacementSuccessorMaterializedFact) (replacementTransition, error) {
	if current.ReplacementIntent == nil || current.ReplacementIntent.RequestID != fact.RequestID || !runStateIsTerminal(current.State) || current.TerminalDelivery == nil || strings.TrimSpace(current.TerminalDelivery.TerminalDeliveryID) != fact.TerminalDeliveryID {
		return replacementTransition{}, common.ErrConflict
	}
	if current.ReplacementIntent.SuccessorRunID != "" && current.ReplacementIntent.SuccessorRunID != fact.SuccessorRunID {
		return replacementTransition{}, common.ErrConflict
	}
	if current.ReplacementIntent.SuccessorRunID == fact.SuccessorRunID && current.Lineage.SuccessorRunID == fact.SuccessorRunID {
		intent := runmodel.CloneReplacementIntent(*current.ReplacementIntent)
		return replacementTransition{Intent: &intent, Noop: true}, nil
	}
	if !replacementIntentMayMaterialize(current.ReplacementIntent) {
		return replacementTransition{}, common.ErrConflict
	}
	next := runmodel.CloneRun(current)
	next.ReplacementIntent.State = runmodel.ReplacementIntentSuccessorCreated
	next.ReplacementIntent.SuccessorRunID = fact.SuccessorRunID
	next.ReplacementIntent.TerminalDeliveryID = fact.TerminalDeliveryID
	next.Lineage.SuccessorRunID = fact.SuccessorRunID
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
	if !next.ReplacementIntent.ValidFor(next) {
		return replacementTransition{}, common.ErrConflict
	}
	return replacementTransition{Next: next, Intent: cloneReplacementIntentPointer(next.ReplacementIntent)}, nil
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
