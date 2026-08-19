package decision

import (
	"context"
	"strings"
	"time"

	decisionModel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

// lifecycleInternalEvent is Decision's private state-machine input. Public
// commands provide facts; only this owner-local reducer chooses ticket and
// proposal states.
type lifecycleInternalEvent struct {
	Kind          lifecycleInternalEventKind
	TicketID      common.TicketID
	Proposal      *decisionModel.RuntimeGovernanceResolutionProposal
	ResolvedBy    string
	FailureClass  string
	FailureReason string
}

type lifecycleInternalEventKind string

const (
	lifecycleInternalEventResolutionObserved              lifecycleInternalEventKind = "resolution_observed"
	lifecycleInternalEventResolutionRecovered             lifecycleInternalEventKind = "resolution_recovered"
	lifecycleInternalEventProposalPrepared                lifecycleInternalEventKind = "proposal_prepared"
	lifecycleInternalEventProposalCommitted               lifecycleInternalEventKind = "proposal_committed"
	lifecycleInternalEventProposalRejected                lifecycleInternalEventKind = "proposal_rejected"
	lifecycleInternalEventCreatedNotificationPublished    lifecycleInternalEventKind = "created_notification_published"
	lifecycleInternalEventResolutionNotificationPublished lifecycleInternalEventKind = "resolution_notification_published"
	lifecycleInternalEventProposalEvidenceRecorded        lifecycleInternalEventKind = "proposal_evidence_recorded"
	lifecycleInternalEventProposalNotificationPublished   lifecycleInternalEventKind = "proposal_notification_published"
	lifecycleInternalEventAggregateDeletionRequested      lifecycleInternalEventKind = "aggregate_deletion_requested"
)

func (s *Service) applyLifecycleInternalEvent(ctx context.Context, current *decisionModel.Ticket, event lifecycleInternalEvent) (*decisionModel.Ticket, error) {
	if s == nil || current == nil || event.TicketID == "" || current.ID != event.TicketID {
		return nil, common.ErrInvalidArgument
	}
	next, err := reduceDecisionLifecycle(current, event)
	if err != nil {
		return nil, err
	}
	if err := s.tickets.Update(ctx, next); err != nil {
		return nil, err
	}
	return next, nil
}

func reduceDecisionLifecycle(current *decisionModel.Ticket, event lifecycleInternalEvent) (*decisionModel.Ticket, error) {
	if current == nil || current.ID == "" || current.ID != event.TicketID {
		return nil, common.ErrInvalidArgument
	}
	next := decisionModel.CloneTicket(current)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	switch event.Kind {
	case lifecycleInternalEventResolutionObserved:
		if current.State != common.DecisionTicketStateOpen || current.DecisionKind == "runtime_governance" {
			return nil, common.ErrConflict
		}
		next.State = common.DecisionTicketStateResolved
		next.ResolvedAt = now
		next.ResolvedBy = strings.TrimSpace(event.ResolvedBy)
	case lifecycleInternalEventResolutionRecovered:
		if current.State != common.DecisionTicketStateOpen {
			return nil, common.ErrConflict
		}
		next.State = common.DecisionTicketStateResolved
	case lifecycleInternalEventProposalPrepared:
		if current.State != common.DecisionTicketStateOpen || current.DecisionKind != "runtime_governance" || current.RuntimeGovernanceProposal != nil || event.Proposal == nil {
			return nil, common.ErrConflict
		}
		proposal := event.Proposal.Clone()
		proposal.State = decisionModel.RuntimeGovernanceResolutionProposalPending
		proposal.CreatedAt = now
		proposal.UpdatedAt = now
		next.RuntimeGovernanceProposal = &proposal
	case lifecycleInternalEventProposalCommitted:
		if current.DecisionKind != "runtime_governance" || (current.State != common.DecisionTicketStateOpen && current.State != common.DecisionTicketStateResolved) || current.RuntimeGovernanceProposal == nil {
			return nil, common.ErrConflict
		}
		next.State = common.DecisionTicketStateResolved
		next.ResolvedAt = nonEmptyString(next.ResolvedAt, now)
		next.ResolvedBy = nonEmptyString(next.ResolvedBy, strings.TrimSpace(event.ResolvedBy))
		next.RuntimeGovernanceProposal.State = decisionModel.RuntimeGovernanceResolutionProposalCommitted
		next.RuntimeGovernanceProposal.FailureClass = ""
		next.RuntimeGovernanceProposal.FailureReason = ""
		next.RuntimeGovernanceProposal.UpdatedAt = now
	case lifecycleInternalEventProposalRejected:
		if current.DecisionKind != "runtime_governance" || current.State != common.DecisionTicketStateOpen || current.RuntimeGovernanceProposal == nil {
			return nil, common.ErrConflict
		}
		next.RuntimeGovernanceProposal.State = decisionModel.RuntimeGovernanceResolutionProposalRejected
		next.RuntimeGovernanceProposal.FailureClass = strings.TrimSpace(event.FailureClass)
		next.RuntimeGovernanceProposal.FailureReason = strings.TrimSpace(event.FailureReason)
		next.RuntimeGovernanceProposal.UpdatedAt = now
	case lifecycleInternalEventCreatedNotificationPublished:
		if current.CreatedNotificationPublishedAt != "" {
			return nil, common.ErrConflict
		}
		next.CreatedNotificationPublishedAt = now
	case lifecycleInternalEventResolutionNotificationPublished:
		if current.DecisionKind == "runtime_governance" || current.State != common.DecisionTicketStateResolved || current.ResolutionNotificationPublishedAt != "" {
			return nil, common.ErrConflict
		}
		next.ResolutionNotificationPublishedAt = now
	case lifecycleInternalEventProposalEvidenceRecorded:
		if current.DecisionKind != "runtime_governance" || current.State != common.DecisionTicketStateResolved || current.RuntimeGovernanceProposal == nil ||
			current.RuntimeGovernanceProposal.State != decisionModel.RuntimeGovernanceResolutionProposalCommitted || current.RuntimeGovernanceProposal.EvidenceRecordedAt != "" {
			return nil, common.ErrConflict
		}
		next.RuntimeGovernanceProposal.EvidenceRecordedAt = now
		next.RuntimeGovernanceProposal.UpdatedAt = now
	case lifecycleInternalEventProposalNotificationPublished:
		if current.DecisionKind != "runtime_governance" || current.State != common.DecisionTicketStateResolved || current.RuntimeGovernanceProposal == nil ||
			current.RuntimeGovernanceProposal.State != decisionModel.RuntimeGovernanceResolutionProposalCommitted || current.RuntimeGovernanceProposal.ResolutionNotificationPublishedAt != "" {
			return nil, common.ErrConflict
		}
		next.RuntimeGovernanceProposal.ResolutionNotificationPublishedAt = now
		next.RuntimeGovernanceProposal.UpdatedAt = now
	case lifecycleInternalEventAggregateDeletionRequested:
		if current.DeletionRequestedAt != "" {
			return nil, common.ErrConflict
		}
		next.DeletionRequestedAt = now
	default:
		return nil, common.ErrInvalidArgument
	}
	next.UpdatedAt = now
	return next, nil
}
