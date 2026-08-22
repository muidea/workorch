package governance

import (
	"context"
	"errors"
	"strings"

	governancemodel "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type governanceTraceInternalEventKind string

const (
	governanceTraceInternalEventEvaluated           governanceTraceInternalEventKind = "evaluated"
	governanceTraceInternalEventEscalated           governanceTraceInternalEventKind = "escalated"
	governanceTraceInternalEventResolutionStarted   governanceTraceInternalEventKind = "resolution_started"
	governanceTraceInternalEventResolutionBlocked   governanceTraceInternalEventKind = "resolution_blocked"
	governanceTraceInternalEventResolutionCompleted governanceTraceInternalEventKind = "resolution_completed"
)

type governanceTraceEscalatedFact struct {
	PolicyBlocked string
	OccurredAt    string
}

type governanceTraceResolutionStartedFact struct {
	Action        string
	PolicyMatched string
	BudgetAfter   int
	Reason        string
	OccurredAt    string
}

type governanceTraceBlockedFact struct {
	PolicyBlocked string
	Reason        string
	OccurredAt    string
}

type governanceTraceResolvedFact struct {
	Action     string
	Reason     string
	OccurredAt string
}

// governanceTraceInternalEvent is the sole Governance-owner status mutation
// surface. Ticket and Approval owner facts are validated before being reduced
// to one of these private events.
type governanceTraceInternalEvent struct {
	Kind        governanceTraceInternalEventKind
	WorkspaceID common.WorkspaceID
	TraceID     common.AutoGovernanceTraceID

	Evaluated           *governancemodel.AutoGovernanceTrace
	Escalated           *governanceTraceEscalatedFact
	ResolutionStarted   *governanceTraceResolutionStartedFact
	ResolutionBlocked   *governanceTraceBlockedFact
	ResolutionCompleted *governanceTraceResolvedFact
}

type governanceTraceTransition struct {
	Next *governancemodel.AutoGovernanceTrace
	Noop bool
}

func normalizeGovernanceTraceInternalEvent(event governanceTraceInternalEvent) (governanceTraceInternalEvent, error) {
	if event.WorkspaceID == "" || event.TraceID == "" {
		return governanceTraceInternalEvent{}, common.ErrInvalidArgument
	}
	payloadCount := 0
	for _, present := range []bool{event.Evaluated != nil, event.Escalated != nil, event.ResolutionStarted != nil, event.ResolutionBlocked != nil, event.ResolutionCompleted != nil} {
		if present {
			payloadCount++
		}
	}
	if payloadCount != 1 {
		return governanceTraceInternalEvent{}, common.ErrInvalidArgument
	}
	switch event.Kind {
	case governanceTraceInternalEventEvaluated:
		if event.Evaluated == nil || event.Evaluated.ID != event.TraceID || event.Evaluated.WorkspaceID != event.WorkspaceID || event.Evaluated.RunID == "" || strings.TrimSpace(event.Evaluated.ItemType) == "" || strings.TrimSpace(event.Evaluated.ItemID) == "" || strings.TrimSpace(event.Evaluated.OccurredAt) == "" {
			return governanceTraceInternalEvent{}, common.ErrInvalidArgument
		}
		trace := governancemodel.CloneAutoGovernanceTrace(event.Evaluated)
		trace.Status = ""
		event.Evaluated = trace
	case governanceTraceInternalEventEscalated:
		if event.Escalated == nil || strings.TrimSpace(event.Escalated.OccurredAt) == "" {
			return governanceTraceInternalEvent{}, common.ErrInvalidArgument
		}
		fact := *event.Escalated
		fact.PolicyBlocked = strings.TrimSpace(fact.PolicyBlocked)
		fact.OccurredAt = strings.TrimSpace(fact.OccurredAt)
		event.Escalated = &fact
	case governanceTraceInternalEventResolutionStarted:
		if event.ResolutionStarted == nil || strings.TrimSpace(event.ResolutionStarted.Action) == "" || strings.TrimSpace(event.ResolutionStarted.OccurredAt) == "" {
			return governanceTraceInternalEvent{}, common.ErrInvalidArgument
		}
		fact := *event.ResolutionStarted
		fact.Action = strings.TrimSpace(fact.Action)
		fact.PolicyMatched = strings.TrimSpace(fact.PolicyMatched)
		fact.Reason = strings.TrimSpace(fact.Reason)
		fact.OccurredAt = strings.TrimSpace(fact.OccurredAt)
		event.ResolutionStarted = &fact
	case governanceTraceInternalEventResolutionBlocked:
		if event.ResolutionBlocked == nil || strings.TrimSpace(event.ResolutionBlocked.PolicyBlocked) == "" || strings.TrimSpace(event.ResolutionBlocked.Reason) == "" || strings.TrimSpace(event.ResolutionBlocked.OccurredAt) == "" {
			return governanceTraceInternalEvent{}, common.ErrInvalidArgument
		}
		fact := *event.ResolutionBlocked
		fact.PolicyBlocked = strings.TrimSpace(fact.PolicyBlocked)
		fact.Reason = strings.TrimSpace(fact.Reason)
		fact.OccurredAt = strings.TrimSpace(fact.OccurredAt)
		event.ResolutionBlocked = &fact
	case governanceTraceInternalEventResolutionCompleted:
		if event.ResolutionCompleted == nil || strings.TrimSpace(event.ResolutionCompleted.Action) == "" || strings.TrimSpace(event.ResolutionCompleted.OccurredAt) == "" {
			return governanceTraceInternalEvent{}, common.ErrInvalidArgument
		}
		fact := *event.ResolutionCompleted
		fact.Action = strings.TrimSpace(fact.Action)
		fact.Reason = strings.TrimSpace(fact.Reason)
		fact.OccurredAt = strings.TrimSpace(fact.OccurredAt)
		event.ResolutionCompleted = &fact
	default:
		return governanceTraceInternalEvent{}, common.ErrInvalidArgument
	}
	return event, nil
}

func reduceGovernanceTraceInternalEvent(current *governancemodel.AutoGovernanceTrace, event governanceTraceInternalEvent) (governanceTraceTransition, error) {
	if current != nil && (current.ID != event.TraceID || current.WorkspaceID != event.WorkspaceID) {
		return governanceTraceTransition{}, common.ErrConflict
	}
	switch event.Kind {
	case governanceTraceInternalEventEvaluated:
		return reduceGovernanceTraceEvaluated(current, *event.Evaluated)
	case governanceTraceInternalEventEscalated:
		return reduceGovernanceTraceEscalated(current, *event.Escalated)
	case governanceTraceInternalEventResolutionStarted:
		return reduceGovernanceTraceResolutionStarted(current, *event.ResolutionStarted)
	case governanceTraceInternalEventResolutionBlocked:
		return reduceGovernanceTraceBlocked(current, *event.ResolutionBlocked)
	case governanceTraceInternalEventResolutionCompleted:
		return reduceGovernanceTraceResolved(current, *event.ResolutionCompleted)
	default:
		return governanceTraceTransition{}, common.ErrInvalidArgument
	}
}

func reduceGovernanceTraceEvaluated(current *governancemodel.AutoGovernanceTrace, observed governancemodel.AutoGovernanceTrace) (governanceTraceTransition, error) {
	if current != nil && !sameGovernanceTraceIdentity(current, &observed) {
		return governanceTraceTransition{}, common.ErrConflict
	}
	if current != nil && (strings.EqualFold(current.Status, autoGovernanceStatusResolved) || strings.EqualFold(current.Status, autoGovernanceStatusBlocked)) {
		return governanceTraceTransition{Next: governancemodel.CloneAutoGovernanceTrace(current), Noop: true}, nil
	}
	next := governancemodel.CloneAutoGovernanceTrace(&observed)
	next.Status = autoGovernanceStatusEvaluated
	return governanceTraceTransition{Next: next}, nil
}

func reduceGovernanceTraceEscalated(current *governancemodel.AutoGovernanceTrace, fact governanceTraceEscalatedFact) (governanceTraceTransition, error) {
	if current == nil || (!strings.EqualFold(current.Status, autoGovernanceStatusEvaluated) && !strings.EqualFold(current.Status, autoGovernanceStatusEscalated)) {
		return governanceTraceTransition{}, common.ErrConflict
	}
	if strings.EqualFold(current.Status, autoGovernanceStatusEscalated) && current.PolicyBlocked == fact.PolicyBlocked {
		return governanceTraceTransition{Next: governancemodel.CloneAutoGovernanceTrace(current), Noop: true}, nil
	}
	next := governancemodel.CloneAutoGovernanceTrace(current)
	next.Status = autoGovernanceStatusEscalated
	next.PolicyBlocked = fact.PolicyBlocked
	next.OccurredAt = fact.OccurredAt
	return governanceTraceTransition{Next: next}, nil
}

func reduceGovernanceTraceResolutionStarted(current *governancemodel.AutoGovernanceTrace, fact governanceTraceResolutionStartedFact) (governanceTraceTransition, error) {
	if current == nil || (!strings.EqualFold(current.Status, autoGovernanceStatusEvaluated) && !strings.EqualFold(current.Status, autoGovernanceStatusResolving)) {
		return governanceTraceTransition{}, common.ErrConflict
	}
	next := governancemodel.CloneAutoGovernanceTrace(current)
	next.Status = autoGovernanceStatusResolving
	next.Action = fact.Action
	next.PolicyMatched = fact.PolicyMatched
	next.PolicyBlocked = ""
	next.BudgetAfter = fact.BudgetAfter
	next.Reason = fact.Reason
	next.OccurredAt = fact.OccurredAt
	return governanceTraceTransition{Next: next}, nil
}

func reduceGovernanceTraceBlocked(current *governancemodel.AutoGovernanceTrace, fact governanceTraceBlockedFact) (governanceTraceTransition, error) {
	if current == nil || strings.EqualFold(current.Status, autoGovernanceStatusResolved) {
		return governanceTraceTransition{}, common.ErrConflict
	}
	if strings.EqualFold(current.Status, autoGovernanceStatusBlocked) && current.PolicyBlocked == fact.PolicyBlocked && current.Reason == fact.Reason {
		return governanceTraceTransition{Next: governancemodel.CloneAutoGovernanceTrace(current), Noop: true}, nil
	}
	if !strings.EqualFold(current.Status, autoGovernanceStatusEvaluated) && !strings.EqualFold(current.Status, autoGovernanceStatusResolving) {
		return governanceTraceTransition{}, common.ErrConflict
	}
	next := governancemodel.CloneAutoGovernanceTrace(current)
	next.Status = autoGovernanceStatusBlocked
	next.Action = "escalate"
	next.PolicyMatched = ""
	next.PolicyBlocked = fact.PolicyBlocked
	next.Reason = fact.Reason
	next.OccurredAt = fact.OccurredAt
	return governanceTraceTransition{Next: next}, nil
}

func reduceGovernanceTraceResolved(current *governancemodel.AutoGovernanceTrace, fact governanceTraceResolvedFact) (governanceTraceTransition, error) {
	if current == nil || strings.EqualFold(current.Status, autoGovernanceStatusBlocked) {
		return governanceTraceTransition{}, common.ErrConflict
	}
	if strings.EqualFold(current.Status, autoGovernanceStatusResolved) {
		if current.Action != fact.Action {
			return governanceTraceTransition{}, common.ErrConflict
		}
		return governanceTraceTransition{Next: governancemodel.CloneAutoGovernanceTrace(current), Noop: true}, nil
	}
	if !strings.EqualFold(current.Status, autoGovernanceStatusEvaluated) && !strings.EqualFold(current.Status, autoGovernanceStatusResolving) {
		return governanceTraceTransition{}, common.ErrConflict
	}
	next := governancemodel.CloneAutoGovernanceTrace(current)
	next.Status = autoGovernanceStatusResolved
	next.Action = fact.Action
	next.PolicyBlocked = ""
	if next.Reason == "" {
		next.Reason = fact.Reason
	}
	next.OccurredAt = fact.OccurredAt
	return governanceTraceTransition{Next: next}, nil
}

func sameGovernanceTraceIdentity(left, right *governancemodel.AutoGovernanceTrace) bool {
	return left != nil && right != nil && left.ID == right.ID && left.WorkspaceID == right.WorkspaceID && left.RunID == right.RunID && left.StepID == right.StepID && strings.EqualFold(strings.TrimSpace(left.ItemType), strings.TrimSpace(right.ItemType)) && strings.TrimSpace(left.ItemID) == strings.TrimSpace(right.ItemID)
}

func (s *Service) submitGovernanceTraceInternalEvent(ctx context.Context, event governanceTraceInternalEvent) (*governancemodel.AutoGovernanceTrace, bool, error) {
	if s == nil || s.traces == nil {
		return nil, false, common.ErrInvalidArgument
	}
	event, err := normalizeGovernanceTraceInternalEvent(event)
	if err != nil {
		return nil, false, err
	}
	bound, err := common.BindRuntimeWorkspaceContext(ctx, event.WorkspaceID)
	if err != nil {
		return nil, false, err
	}
	s.traceMu.Lock()
	defer s.traceMu.Unlock()
	current, err := s.traces.Get(bound, event.TraceID)
	if err != nil && !errors.Is(err, common.ErrNotFound) {
		return nil, false, err
	}
	if errors.Is(err, common.ErrNotFound) {
		current = nil
	}
	transition, err := reduceGovernanceTraceInternalEvent(current, event)
	if err != nil {
		return nil, false, err
	}
	if transition.Next == nil {
		return nil, false, common.ErrInvalidArgument
	}
	if transition.Noop {
		return governancemodel.CloneAutoGovernanceTrace(transition.Next), false, nil
	}
	if err := s.traces.Upsert(bound, transition.Next); err != nil {
		return nil, false, err
	}
	return governancemodel.CloneAutoGovernanceTrace(transition.Next), true, nil
}

func (s *Service) persistEvaluatedGovernanceTrace(ctx context.Context, trace *governancemodel.AutoGovernanceTrace) (*governancemodel.AutoGovernanceTrace, error) {
	if trace == nil {
		return nil, common.ErrInvalidArgument
	}
	value, _, err := s.submitGovernanceTraceInternalEvent(ctx, governanceTraceInternalEvent{
		Kind: governanceTraceInternalEventEvaluated, WorkspaceID: trace.WorkspaceID, TraceID: trace.ID, Evaluated: trace,
	})
	return value, err
}

func (s *Service) escalateGovernanceTrace(ctx context.Context, trace *governancemodel.AutoGovernanceTrace, policyBlocked string) (*governancemodel.AutoGovernanceTrace, error) {
	if trace == nil {
		return nil, common.ErrInvalidArgument
	}
	value, _, err := s.submitGovernanceTraceInternalEvent(ctx, governanceTraceInternalEvent{
		Kind: governanceTraceInternalEventEscalated, WorkspaceID: trace.WorkspaceID, TraceID: trace.ID,
		Escalated: &governanceTraceEscalatedFact{PolicyBlocked: policyBlocked, OccurredAt: nowUTC()},
	})
	return value, err
}

func (s *Service) startGovernanceTraceResolution(ctx context.Context, trace *governancemodel.AutoGovernanceTrace, action string, policyMatched string, budgetAfter int, reason string) (*governancemodel.AutoGovernanceTrace, error) {
	if trace == nil {
		return nil, common.ErrInvalidArgument
	}
	value, _, err := s.submitGovernanceTraceInternalEvent(ctx, governanceTraceInternalEvent{
		Kind: governanceTraceInternalEventResolutionStarted, WorkspaceID: trace.WorkspaceID, TraceID: trace.ID,
		ResolutionStarted: &governanceTraceResolutionStartedFact{Action: action, PolicyMatched: policyMatched, BudgetAfter: budgetAfter, Reason: reason, OccurredAt: nowUTC()},
	})
	return value, err
}

func (s *Service) blockGovernanceTrace(ctx context.Context, trace *governancemodel.AutoGovernanceTrace, policyBlocked string, reason string) (*governancemodel.AutoGovernanceTrace, error) {
	if trace == nil {
		return nil, common.ErrInvalidArgument
	}
	value, _, err := s.submitGovernanceTraceInternalEvent(ctx, governanceTraceInternalEvent{
		Kind: governanceTraceInternalEventResolutionBlocked, WorkspaceID: trace.WorkspaceID, TraceID: trace.ID,
		ResolutionBlocked: &governanceTraceBlockedFact{PolicyBlocked: policyBlocked, Reason: reason, OccurredAt: nowUTC()},
	})
	return value, err
}

func (s *Service) resolveGovernanceTrace(ctx context.Context, trace *governancemodel.AutoGovernanceTrace, action string, fallbackReason string) (*governancemodel.AutoGovernanceTrace, error) {
	if trace == nil {
		return nil, common.ErrInvalidArgument
	}
	value, _, err := s.submitGovernanceTraceInternalEvent(ctx, governanceTraceInternalEvent{
		Kind: governanceTraceInternalEventResolutionCompleted, WorkspaceID: trace.WorkspaceID, TraceID: trace.ID,
		ResolutionCompleted: &governanceTraceResolvedFact{Action: action, Reason: fallbackReason, OccurredAt: nowUTC()},
	})
	return value, err
}
