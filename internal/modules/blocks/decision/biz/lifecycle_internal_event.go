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
	Kind           lifecycleInternalEventKind
	TicketID       common.TicketID
	Ticket         *decisionModel.Ticket
	Proposal       *decisionModel.RuntimeGovernanceResolutionProposal
	ContractChange *decisionModel.ContractChangeRevision
	Clarification  *definitionClarificationResolutionFact
	ResolvedAt     string
	ResolvedBy     string
	FailureClass   string
	FailureReason  string
}

// definitionClarificationResolutionFact is the bounded Decision-owner input
// needed to resolve a clarification binding. It carries no target ticket
// state; binding progress and open->resolved are committed together below.
type definitionClarificationResolutionFact struct {
	RequestContractRepairHash   string
	ResolutionID                common.ResolutionID
	ResolutionBindingHash       string
	ResolutionCandidateID       string
	ResolutionCandidateRevision uint64
	ResolutionCandidateHash     string
	AdmissionResultHash         string
	DefinitionRevision          string
	StartupInputHash            string
	ReplacementIntentID         string
	ReplacementIntentHash       string
	SuccessorDefinitionRevision string
	SuccessorStartupInputHash   string
}

type lifecycleInternalEventKind string

const (
	lifecycleInternalEventContractChangeRegistered        lifecycleInternalEventKind = "contract_change_registered"
	lifecycleInternalEventOpened                          lifecycleInternalEventKind = "opened"
	lifecycleInternalEventResolutionObserved              lifecycleInternalEventKind = "resolution_observed"
	lifecycleInternalEventResolutionRecovered             lifecycleInternalEventKind = "resolution_recovered"
	lifecycleInternalEventProposalPrepared                lifecycleInternalEventKind = "proposal_prepared"
	lifecycleInternalEventProposalCommitted               lifecycleInternalEventKind = "proposal_committed"
	lifecycleInternalEventProposalRejected                lifecycleInternalEventKind = "proposal_rejected"
	lifecycleInternalEventCreatedNotificationPublished    lifecycleInternalEventKind = "created_notification_published"
	lifecycleInternalEventResolutionNotificationPublished lifecycleInternalEventKind = "resolution_notification_published"
	lifecycleInternalEventProposalEvidenceRecorded        lifecycleInternalEventKind = "proposal_evidence_recorded"
	lifecycleInternalEventProposalNotificationPublished   lifecycleInternalEventKind = "proposal_notification_published"
)

func (s *mutationService) applyLifecycleInternalEvent(ctx context.Context, current *decisionModel.Ticket, event lifecycleInternalEvent) (*decisionModel.Ticket, error) {
	if s == nil || s.tickets == nil || event.TicketID == "" {
		return nil, common.ErrInvalidArgument
	}
	workspaceID := common.WorkspaceID("")
	if current != nil {
		if current.ID != event.TicketID || event.Kind == lifecycleInternalEventOpened || event.Ticket != nil {
			return nil, common.ErrInvalidArgument
		}
		workspaceID = current.WorkspaceID
	} else {
		if event.Kind != lifecycleInternalEventOpened || event.Ticket == nil || event.Ticket.ID != event.TicketID {
			return nil, common.ErrInvalidArgument
		}
		workspaceID = event.Ticket.WorkspaceID
	}
	bound, err := common.BindRuntimeWorkspaceContext(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	next, err := reduceDecisionLifecycle(current, event)
	if err != nil {
		return nil, err
	}
	if event.Kind == lifecycleInternalEventOpened {
		if err := s.tickets.Create(bound, next); err != nil {
			return nil, err
		}
		return next, nil
	}
	if err := s.tickets.Update(bound, next); err != nil {
		return nil, err
	}
	return next, nil
}

func reduceDecisionLifecycle(current *decisionModel.Ticket, event lifecycleInternalEvent) (*decisionModel.Ticket, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if event.Kind == lifecycleInternalEventOpened {
		if current != nil || event.Ticket == nil || event.Ticket.ID == "" || event.Ticket.ID != event.TicketID || event.Ticket.WorkspaceID == "" || strings.TrimSpace(event.Ticket.Reason) == "" {
			return nil, common.ErrInvalidArgument
		}
		seed := event.Ticket
		if seed.State != "" || seed.RuntimeGovernanceProposal != nil || seed.ResolvedAt != "" || seed.ResolvedBy != "" || seed.CreatedAt != "" || seed.UpdatedAt != "" || seed.CreatedNotificationPublishedAt != "" || seed.ResolutionNotificationPublishedAt != "" || event.Proposal != nil || event.Clarification != nil {
			return nil, common.ErrInvalidArgument
		}
		if seed.DefinitionClarification != nil && definitionClarificationBindingHasResolution(seed.DefinitionClarification) {
			return nil, common.ErrInvalidArgument
		}
		next := decisionModel.CloneTicket(seed)
		next.State = common.DecisionTicketStateOpen
		next.CreatedAt = now
		next.UpdatedAt = now
		if next.IsContractChange() {
			if len(next.ContractChangeRevisions) != 1 || next.ContractChangeRevisions[0].RegisteredAt != "" || event.ContractChange != nil {
				return nil, common.ErrInvalidArgument
			}
			next.ContractChangeRevisions[0].RegisteredAt = now
			if err := next.ValidateContractChange(); err != nil {
				return nil, err
			}
		}
		return next, nil
	}
	if current == nil || current.ID == "" || current.ID != event.TicketID {
		return nil, common.ErrInvalidArgument
	}
	if current.IsContractChange() {
		if current.ValidateContractChange() != nil || event.Kind != lifecycleInternalEventContractChangeRegistered || event.ContractChange == nil ||
			event.Proposal != nil || event.Clarification != nil || event.ResolvedAt != "" || event.ResolvedBy != "" || event.FailureClass != "" || event.FailureReason != "" {
			return nil, common.ErrConflict
		}
		next := decisionModel.CloneTicket(current)
		revision := *event.ContractChange
		if revision.RegisteredAt != "" || revision.Revision != uint64(len(next.ContractChangeRevisions)+1) {
			return nil, common.ErrConflict
		}
		revision.Proposal = revision.Proposal.Clone()
		revision.RegisteredAt = now
		next.ContractChangeRevisions = append(next.ContractChangeRevisions, revision)
		next.UpdatedAt = now
		if err := next.ValidateContractChange(); err != nil {
			return nil, err
		}
		return next, nil
	}
	if event.ContractChange != nil || event.Kind == lifecycleInternalEventContractChangeRegistered {
		return nil, common.ErrInvalidArgument
	}
	next := decisionModel.CloneTicket(current)
	switch event.Kind {
	case lifecycleInternalEventResolutionObserved:
		if current.State != common.DecisionTicketStateOpen || current.DecisionKind == "runtime_governance" {
			return nil, common.ErrConflict
		}
		resolvedAt := strings.TrimSpace(event.ResolvedAt)
		if resolvedAt == "" {
			return nil, common.ErrInvalidArgument
		}
		if current.DefinitionClarification == nil {
			if event.Clarification != nil {
				return nil, common.ErrInvalidArgument
			}
		} else {
			binding, err := reduceDefinitionClarificationResolution(current.DefinitionClarification, event.Clarification)
			if err != nil {
				return nil, err
			}
			next.DefinitionClarification = binding
		}
		next.State = common.DecisionTicketStateResolved
		next.ResolvedAt = resolvedAt
		next.ResolvedBy = strings.TrimSpace(event.ResolvedBy)
	case lifecycleInternalEventResolutionRecovered:
		resolvedAt := strings.TrimSpace(event.ResolvedAt)
		if current.State != common.DecisionTicketStateOpen || resolvedAt == "" {
			return nil, common.ErrConflict
		}
		next.State = common.DecisionTicketStateResolved
		next.ResolvedAt = resolvedAt
	case lifecycleInternalEventProposalPrepared:
		if current.State != common.DecisionTicketStateOpen || current.DecisionKind != "runtime_governance" || current.RuntimeGovernanceProposal != nil || event.Proposal == nil {
			return nil, common.ErrConflict
		}
		if event.Proposal.State != "" || strings.TrimSpace(event.Proposal.FailureClass) != "" || strings.TrimSpace(event.Proposal.FailureReason) != "" ||
			strings.TrimSpace(event.Proposal.CreatedAt) != "" || strings.TrimSpace(event.Proposal.UpdatedAt) != "" ||
			strings.TrimSpace(event.Proposal.EvidenceRecordedAt) != "" || strings.TrimSpace(event.Proposal.ResolutionNotificationPublishedAt) != "" {
			return nil, common.ErrInvalidArgument
		}
		proposal := event.Proposal.Clone()
		proposal.State = decisionModel.RuntimeGovernanceResolutionProposalPending
		proposal.CreatedAt = now
		proposal.UpdatedAt = now
		next.RuntimeGovernanceProposal = &proposal
	case lifecycleInternalEventProposalCommitted:
		resolvedAt := strings.TrimSpace(event.ResolvedAt)
		if current.DecisionKind != "runtime_governance" || (current.State != common.DecisionTicketStateOpen && current.State != common.DecisionTicketStateResolved) || current.RuntimeGovernanceProposal == nil || resolvedAt == "" || (current.ResolvedAt != "" && current.ResolvedAt != resolvedAt) {
			return nil, common.ErrConflict
		}
		next.State = common.DecisionTicketStateResolved
		next.ResolvedAt = resolvedAt
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
	default:
		return nil, common.ErrInvalidArgument
	}
	next.UpdatedAt = now
	return next, nil
}

func definitionClarificationBindingHasResolution(binding *decisionModel.DefinitionClarificationBinding) bool {
	return binding != nil && (binding.RequestContractRepairHash != "" || binding.ResolutionID != "" || strings.TrimSpace(binding.ResolutionBindingHash) != "" ||
		strings.TrimSpace(binding.ResolutionCandidateID) != "" || binding.ResolutionCandidateRevision != 0 ||
		strings.TrimSpace(binding.ResolutionCandidateHash) != "" || strings.TrimSpace(binding.AdmissionResultHash) != "" ||
		strings.TrimSpace(binding.ReplacementIntentID) != "" || strings.TrimSpace(binding.ReplacementIntentHash) != "" ||
		strings.TrimSpace(binding.SuccessorDefinitionRevision) != "" || strings.TrimSpace(binding.SuccessorStartupInputHash) != "")
}

func reduceDefinitionClarificationResolution(current *decisionModel.DefinitionClarificationBinding, fact *definitionClarificationResolutionFact) (*decisionModel.DefinitionClarificationBinding, error) {
	if current == nil || fact == nil || fact.ResolutionID == "" || strings.TrimSpace(fact.ResolutionBindingHash) == "" || definitionClarificationBindingHasResolution(current) {
		return nil, common.ErrConflict
	}
	next := current.Clone()
	next.ResolutionID = fact.ResolutionID
	next.ResolutionBindingHash = strings.TrimSpace(fact.ResolutionBindingHash)
	switch current.Class {
	case decisionModel.DefinitionClarificationWorkspacePreSeal:
		if fact.RequestContractRepairHash != "" {
			return nil, common.ErrInvalidArgument
		}
		if fact.ReplacementIntentID != "" || fact.ReplacementIntentHash != "" || fact.SuccessorDefinitionRevision != "" || fact.SuccessorStartupInputHash != "" ||
			((fact.DefinitionRevision == "") != (fact.StartupInputHash == "")) {
			return nil, common.ErrInvalidArgument
		}
		resolutionIdentityPresent := strings.TrimSpace(fact.ResolutionCandidateID) != "" || fact.ResolutionCandidateRevision != 0 ||
			strings.TrimSpace(fact.ResolutionCandidateHash) != "" || strings.TrimSpace(fact.AdmissionResultHash) != ""
		if resolutionIdentityPresent {
			if strings.TrimSpace(fact.ResolutionCandidateID) == "" || fact.ResolutionCandidateID != current.CandidateID ||
				fact.ResolutionCandidateRevision <= current.CandidateRevision || strings.TrimSpace(fact.ResolutionCandidateHash) == "" ||
				strings.TrimSpace(fact.AdmissionResultHash) == "" || strings.TrimSpace(fact.DefinitionRevision) == "" || strings.TrimSpace(fact.StartupInputHash) == "" {
				return nil, common.ErrInvalidArgument
			}
		} else if fact.DefinitionRevision != "" || fact.StartupInputHash != "" {
			return nil, common.ErrInvalidArgument
		}
		next.ResolutionCandidateID = strings.TrimSpace(fact.ResolutionCandidateID)
		next.ResolutionCandidateRevision = fact.ResolutionCandidateRevision
		next.ResolutionCandidateHash = strings.TrimSpace(fact.ResolutionCandidateHash)
		next.AdmissionResultHash = strings.TrimSpace(fact.AdmissionResultHash)
		next.DefinitionRevision = strings.TrimSpace(fact.DefinitionRevision)
		next.StartupInputHash = strings.TrimSpace(fact.StartupInputHash)
	case decisionModel.DefinitionClarificationRuntime:
		if fact.RequestContractRepairHash != "" {
			if fact.ReplacementIntentID != "" || fact.ReplacementIntentHash != "" || fact.SuccessorDefinitionRevision != "" || fact.SuccessorStartupInputHash != "" || fact.DefinitionRevision != "" || fact.StartupInputHash != "" || fact.ResolutionCandidateID != "" || fact.ResolutionCandidateRevision != 0 || fact.ResolutionCandidateHash != "" || fact.AdmissionResultHash != "" {
				return nil, common.ErrInvalidArgument
			}
			next.RequestContractRepairHash = fact.RequestContractRepairHash
			return next, nil
		}
		if strings.TrimSpace(fact.ReplacementIntentID) == "" || strings.TrimSpace(fact.ReplacementIntentHash) == "" || strings.TrimSpace(fact.SuccessorDefinitionRevision) == "" || strings.TrimSpace(fact.SuccessorStartupInputHash) == "" || fact.DefinitionRevision != "" || fact.StartupInputHash != "" {
			return nil, common.ErrInvalidArgument
		}
		next.ReplacementIntentID = strings.TrimSpace(fact.ReplacementIntentID)
		next.ReplacementIntentHash = strings.TrimSpace(fact.ReplacementIntentHash)
		next.SuccessorDefinitionRevision = strings.TrimSpace(fact.SuccessorDefinitionRevision)
		next.SuccessorStartupInputHash = strings.TrimSpace(fact.SuccessorStartupInputHash)
	default:
		return nil, common.ErrInvalidArgument
	}
	return next, nil
}
