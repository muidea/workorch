package decision

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	decisioncontracts "github.com/muidea/workorch/internal/modules/blocks/decision/biz/contracts"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/common"
	decisionModel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

type Service struct {
	tickets     repository.DecisionTicketRepository
	resolutions repository.DecisionResolutionRepository
}

// NewService only retains Decision owner repositories.
func NewService(tickets repository.DecisionTicketRepository, resolutions repository.DecisionResolutionRepository) *Service {
	return &Service{tickets: tickets, resolutions: resolutions}
}

func (s *Service) CreateTicket(ctx context.Context, cmd decisioncontracts.CreateTicketCommand) error {
	_, err := s.CreateTicketAndReturn(ctx, cmd)
	return err
}

// CreateTicketAndReturn persists a new ticket. Event publication is owned by
// Biz so this repository service remains independent of framework resources.
func (s *Service) CreateTicketAndReturn(ctx context.Context, cmd decisioncontracts.CreateTicketCommand) (*decisionModel.Ticket, error) {
	if cmd.TicketID == "" || cmd.WorkspaceID == "" || cmd.RunID == "" || cmd.Reason == "" {
		return nil, common.ErrInvalidArgument
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	item := &decisionModel.Ticket{
		ID:                     cmd.TicketID,
		WorkspaceID:            cmd.WorkspaceID,
		RunID:                  cmd.RunID,
		StepID:                 cmd.StepID,
		Reason:                 cmd.Reason,
		DecisionKind:           cmd.DecisionKind,
		Question:               cmd.Question,
		Context:                clonePayload(cmd.Context),
		State:                  common.DecisionTicketStateOpen,
		AutoGovernance:         clonePayload(cmd.AutoGovernance),
		FailureClass:           nonEmptyString(cmd.FailureClass, stringPayloadValue(cmd.Context, "failureClass")),
		FailureSignature:       nonEmptyString(cmd.FailureSignature, stringPayloadValue(cmd.Context, "failureSignature")),
		AllowedRecoveryActions: nonEmptyStringSlice(cmd.AllowedRecoveryActions, stringSlicePayloadValue(cmd.Context, "allowedRecoveryActions")),
		CreatedAt:              now,
		UpdatedAt:              now,
	}
	if err := s.tickets.Create(ctx, item); err != nil {
		return nil, err
	}
	return item, nil
}

func (s *Service) EnsureTicket(ctx context.Context, cmd decisioncontracts.CreateTicketCommand) error {
	_, _, err := s.EnsureTicketAndReturn(ctx, cmd)
	return err
}

// EnsureTicketAndReturn reports whether this invocation created the ticket.
func (s *Service) EnsureTicketAndReturn(ctx context.Context, cmd decisioncontracts.CreateTicketCommand) (*decisionModel.Ticket, bool, error) {
	if cmd.TicketID == "" || cmd.WorkspaceID == "" || cmd.RunID == "" || cmd.Reason == "" {
		return nil, false, common.ErrInvalidArgument
	}

	item, err := s.tickets.Get(ctx, cmd.TicketID)
	if err == nil {
		if item.WorkspaceID != cmd.WorkspaceID ||
			item.RunID != cmd.RunID ||
			item.StepID != cmd.StepID ||
			item.Reason != cmd.Reason ||
			item.DecisionKind != cmd.DecisionKind ||
			item.Question != cmd.Question ||
			!sameResolutionPayload(item.Context, cmd.Context) ||
			!sameResolutionPayload(item.AutoGovernance, cmd.AutoGovernance) ||
			item.FailureClass != nonEmptyString(cmd.FailureClass, stringPayloadValue(cmd.Context, "failureClass")) ||
			item.FailureSignature != nonEmptyString(cmd.FailureSignature, stringPayloadValue(cmd.Context, "failureSignature")) ||
			!slices.Equal(item.AllowedRecoveryActions, nonEmptyStringSlice(cmd.AllowedRecoveryActions, stringSlicePayloadValue(cmd.Context, "allowedRecoveryActions"))) {
			return nil, false, common.ErrConflict
		}
		return item, false, nil
	}
	if !errors.Is(err, common.ErrNotFound) {
		return nil, false, err
	}
	item, err = s.CreateTicketAndReturn(ctx, cmd)
	return item, err == nil, err
}

func (s *Service) ResolveTicket(ctx context.Context, cmd decisioncontracts.ResolveTicketCommand) error {
	_, _, err := s.ResolveTicketAndReturn(ctx, cmd)
	return err
}

func (s *Service) ResolveTicketAndReturn(ctx context.Context, cmd decisioncontracts.ResolveTicketCommand) (*decisionModel.Ticket, *decisionModel.Resolution, error) {
	if cmd.ResolutionID == "" || cmd.TicketID == "" || cmd.Type == "" {
		return nil, nil, common.ErrInvalidArgument
	}
	if !modulecommon.IsSupportedResolutionType(cmd.Type) {
		return nil, nil, common.ErrInvalidArgument
	}
	ctx, err := resolveCommandContext(ctx, cmd)
	if err != nil {
		return nil, nil, err
	}

	ticket, err := s.tickets.Get(ctx, cmd.TicketID)
	if err != nil {
		return nil, nil, err
	}
	if !ticketMatchesResolveScope(ticket, cmd) {
		return nil, nil, common.ErrNotFound
	}
	if ticket.DecisionKind == "runtime_governance" {
		return nil, nil, common.ErrConflict
	}
	if err := validateResolutionTypeForTicket(ticket, cmd.Type); err != nil {
		return nil, nil, err
	}
	if ticket.State != common.DecisionTicketStateOpen {
		return nil, nil, common.ErrConflict
	}

	resolution, err := s.resolveTicketItem(ctx, ticket, cmd)
	return ticket, resolution, err
}

func (s *Service) resolveTicketItem(ctx context.Context, ticket *decisionModel.Ticket, cmd decisioncontracts.ResolveTicketCommand) (*decisionModel.Resolution, error) {
	if ticket == nil || ticket.ID == "" {
		return nil, common.ErrInvalidArgument
	}
	if ticket.State != common.DecisionTicketStateOpen {
		return nil, common.ErrConflict
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	ticket.UpdatedAt = now
	ticket.ResolvedAt = now
	ticket.ResolvedBy = nonEmptyString(stringPayloadValue(cmd.Payload, "resolvedBy"), stringPayloadValue(cmd.Payload, "actor"))
	resolution := &decisionModel.Resolution{
		ID:                 cmd.ResolutionID,
		TicketID:           cmd.TicketID,
		Type:               cmd.Type,
		Payload:            common.RedactSensitivePayload(cmd.Payload),
		ContinuationAction: nonEmptyString(cmd.ContinuationAction, stringPayloadValue(cmd.Payload, "continuationAction")),
		ResumeRunID:        nonEmptyRunID(cmd.ResumeRunID, common.RunID(stringPayloadValue(cmd.Payload, "resumeRunId"))),
		SuccessorRunID:     nonEmptyRunID(cmd.SuccessorRunID, common.RunID(stringPayloadValue(cmd.Payload, "successorRunId"))),
	}
	if err := s.resolutions.Create(ctx, resolution); err != nil {
		return nil, err
	}

	ticket.State = common.DecisionTicketStateResolved
	if err := s.tickets.Update(ctx, ticket); err != nil {
		return nil, err
	}
	return resolution, nil
}

func (s *Service) EnsureResolvedTicket(ctx context.Context, cmd decisioncontracts.ResolveTicketCommand) error {
	_, _, err := s.EnsureResolvedTicketAndReturn(ctx, cmd)
	return err
}

func (s *Service) EnsureResolvedTicketAndReturn(ctx context.Context, cmd decisioncontracts.ResolveTicketCommand) (*decisionModel.Ticket, *decisionModel.Resolution, error) {
	ticket, resolution, _, err := s.EnsureResolvedTicketAndReturnWithTransition(ctx, cmd)
	return ticket, resolution, err
}

// EnsureResolvedTicketAndReturnWithTransition reports whether the ticket
// transitioned to resolved. Evidence is required for every successful call,
// while the resolved notification is emitted only for a transition.
func (s *Service) EnsureResolvedTicketAndReturnWithTransition(ctx context.Context, cmd decisioncontracts.ResolveTicketCommand) (*decisionModel.Ticket, *decisionModel.Resolution, bool, error) {
	if cmd.ResolutionID == "" || cmd.TicketID == "" || cmd.Type == "" {
		return nil, nil, false, common.ErrInvalidArgument
	}
	if !modulecommon.IsSupportedResolutionType(cmd.Type) {
		return nil, nil, false, common.ErrInvalidArgument
	}
	ctx, err := resolveCommandContext(ctx, cmd)
	if err != nil {
		return nil, nil, false, err
	}

	ticket, err := s.tickets.Get(ctx, cmd.TicketID)
	if err != nil {
		return nil, nil, false, err
	}
	if !ticketMatchesResolveScope(ticket, cmd) {
		return nil, nil, false, common.ErrNotFound
	}
	if ticket.DecisionKind == "runtime_governance" {
		return nil, nil, false, common.ErrConflict
	}
	if err := validateResolutionTypeForTicket(ticket, cmd.Type); err != nil {
		return nil, nil, false, err
	}

	// Orphan resolution recovery: a resolution file may already exist for this
	// ticket even when ticket.State is still "open" (e.g. an operator manually
	// reset the run back to blocked_on_decision without deleting the
	// resolution, or an external fixture wrote state=open while leaving the
	// resolution on disk). Production code never reverts a resolved ticket to
	// open, so this mismatch is recoverable. When a persisted resolution
	// matches the new request, accept it idempotently and sync the ticket
	// state to "resolved" so the run orchestrator's ResumeRun check at
	// runtime_lifecycle.go (ticket.State == Resolved) passes. Because the
	// current owner state is transitioning from open -> resolved again, we
	// must publish TopicTicketResolved so workorchd projections, SSE, and TUI
	// stay consistent with the repaired owner state.
	existingResolution, resErr := s.resolutions.GetByTicketID(ctx, cmd.TicketID)
	if resErr != nil && !errors.Is(resErr, common.ErrNotFound) {
		return nil, nil, false, resErr
	}
	if existingResolution != nil {
		if existingResolution.ID != cmd.ResolutionID ||
			existingResolution.Type != cmd.Type ||
			!sameResolutionPayload(existingResolution.Payload, cmd.Payload) {
			return nil, nil, false, common.ErrConflict
		}
		transitioned := ticket.State != common.DecisionTicketStateResolved
		if ticket.State != common.DecisionTicketStateResolved {
			ticket.State = common.DecisionTicketStateResolved
			ticket.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
			if err := s.tickets.Update(ctx, ticket); err != nil {
				return nil, nil, false, err
			}
		}
		return ticket, existingResolution, transitioned, nil
	}

	if ticket.State == common.DecisionTicketStateResolved {
		// Resolution file was deleted but ticket state is still "resolved";
		// preserve the existing strict behavior. Re-creating a resolution
		// here would be a fresh write — treat as ErrConflict to surface the
		// invariant violation rather than silently accept.
		return nil, nil, false, common.ErrConflict
	}
	if ticket.State != common.DecisionTicketStateOpen {
		return nil, nil, false, common.ErrConflict
	}
	resolution, err := s.resolveTicketItem(ctx, ticket, cmd)
	if err != nil {
		return nil, nil, false, err
	}
	return ticket, resolution, true, nil
}

func ticketMatchesResolveScope(ticket *decisionModel.Ticket, cmd decisioncontracts.ResolveTicketCommand) bool {
	if ticket == nil {
		return false
	}
	if cmd.WorkspaceID != "" && ticket.WorkspaceID != cmd.WorkspaceID {
		return false
	}
	if cmd.RunID != "" && ticket.RunID != cmd.RunID {
		return false
	}
	if cmd.StepID != "" && ticket.StepID != cmd.StepID {
		return false
	}
	return true
}

func resolveCommandContext(ctx context.Context, cmd decisioncontracts.ResolveTicketCommand) (context.Context, error) {
	if cmd.WorkspaceID == "" {
		return ctx, nil
	}
	return common.BindRuntimeWorkspaceContext(ctx, cmd.WorkspaceID)
}

func sameResolutionPayload(left workorcheventpayload.Payload, right workorcheventpayload.Payload) bool {
	return left.Equal(right)
}

func clonePayload(input workorcheventpayload.Payload) workorcheventpayload.Payload {
	return input.Clone()
}

func nonEmptyString(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func nonEmptyRunID(values ...common.RunID) common.RunID {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func nonEmptyStringSlice(values ...[]string) []string {
	for _, value := range values {
		if len(value) > 0 {
			return append([]string(nil), value...)
		}
	}
	return nil
}

func stringPayloadValue(payload workorcheventpayload.Payload, key string) string {
	value, _ := payload.String(key)
	return value
}

func stringSlicePayloadValue(payload workorcheventpayload.Payload, key string) []string {
	values, ok := payload.StringSlice(key)
	if !ok {
		return nil
	}
	ret := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			ret = append(ret, value)
		}
	}
	return ret
}

func validateResolutionTypeForTicket(ticket *decisionModel.Ticket, resolutionType string) error {
	if ticket == nil || !modulecommon.IsSupportedResolutionType(resolutionType) {
		return common.ErrInvalidArgument
	}
	allowed := ticket.AllowedRecoveryActions
	if ticket.DecisionKind == "runtime_governance" && len(allowed) == 0 {
		return fmt.Errorf("%w: runtime_governance ticket has no allowed recovery actions", common.ErrInvalidArgument)
	}
	if len(allowed) == 0 {
		return nil
	}
	if slices.Contains(allowed, resolutionType) {
		return nil
	}
	return modulecommon.NewResolutionActionConflict(ticket.ID, resolutionType, allowed)
}

// EnsureRuntimeGovernanceResolutionProposal persists a Runtime proposal
// while leaving the ticket open. The caller must submit the resulting typed
// resolution to Orchestrator before it can finalize the Decision resolution.
//
// A replay of the exact already committed proposal returns that persisted
// proposal as well. This is required after process recreation: the Decision
// owner may have durably finalized its ticket while the caller still needs to
// join Orchestrator's persisted, idempotent continuation lane. It must not
// create a second Decision resolution or accept changed user input.
func (s *Service) EnsureRuntimeGovernanceResolutionProposal(ctx context.Context, cmd decisioncontracts.ResolveTicketCommand) (*decisionModel.Ticket, *decisionModel.RuntimeGovernanceResolutionProposal, error) {
	if s == nil || cmd.ResolutionID == "" || cmd.TicketID == "" || cmd.Type == "" {
		return nil, nil, common.ErrInvalidArgument
	}
	ctx, err := resolveCommandContext(ctx, cmd)
	if err != nil {
		return nil, nil, err
	}
	ticket, err := s.tickets.Get(ctx, cmd.TicketID)
	if err != nil {
		return nil, nil, err
	}
	if !ticketMatchesResolveScope(ticket, cmd) {
		return nil, nil, common.ErrNotFound
	}
	if ticket.DecisionKind != "runtime_governance" {
		return nil, nil, common.ErrInvalidArgument
	}
	if err := validateResolutionTypeForTicket(ticket, cmd.Type); err != nil {
		return nil, nil, err
	}
	proposal, err := runtimeGovernanceProposalFromCommand(cmd)
	if err != nil {
		return nil, nil, err
	}
	resolution, resolutionErr := s.resolutions.GetByTicketID(ctx, cmd.TicketID)
	if resolutionErr != nil && !errors.Is(resolutionErr, common.ErrNotFound) {
		return nil, nil, resolutionErr
	}
	if ticket.State == common.DecisionTicketStateResolved {
		if resolution == nil || ticket.RuntimeGovernanceProposal == nil ||
			ticket.RuntimeGovernanceProposal.State != decisionModel.RuntimeGovernanceResolutionProposalCommitted ||
			!sameRuntimeGovernanceProposal(*ticket.RuntimeGovernanceProposal, proposal) ||
			resolution.ID != cmd.ResolutionID || resolution.Type != cmd.Type ||
			!sameResolutionPayload(resolution.Payload, common.RedactSensitivePayload(cmd.Payload)) {
			return nil, nil, common.ErrConflict
		}
		ret := ticket.RuntimeGovernanceProposal.Clone()
		return ticket, &ret, nil
	}
	if ticket.State != common.DecisionTicketStateOpen || resolution != nil {
		return nil, nil, common.ErrConflict
	}
	if existing := ticket.RuntimeGovernanceProposal; existing != nil {
		if !sameRuntimeGovernanceProposal(*existing, proposal) {
			return nil, nil, common.ErrConflict
		}
		switch existing.State {
		case decisionModel.RuntimeGovernanceResolutionProposalPending:
			ret := existing.Clone()
			return ticket, &ret, nil
		case decisionModel.RuntimeGovernanceResolutionProposalCommitted,
			decisionModel.RuntimeGovernanceResolutionProposalRejected:
			return nil, nil, common.ErrConflict
		default:
			return nil, nil, common.ErrConflict
		}
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	proposal.CreatedAt = now
	proposal.UpdatedAt = now
	ticket.RuntimeGovernanceProposal = &proposal
	if err := s.tickets.Update(ctx, ticket); err != nil {
		return nil, nil, err
	}
	ret := proposal.Clone()
	return ticket, &ret, nil
}

// FinalizeRuntimeGovernanceResolutionProposal creates the public resolution
// and marks its ticket resolved only after the matching Runtime aggregate
// has committed. It is safe to retry after any owner-local write failure.
func (s *Service) FinalizeRuntimeGovernanceResolutionProposal(ctx context.Context, cmd decisioncontracts.ResolveTicketCommand) (*decisionModel.Ticket, *decisionModel.Resolution, error) {
	ticket, resolution, _, err := s.FinalizeRuntimeGovernanceResolutionProposalWithTransition(ctx, cmd)
	return ticket, resolution, err
}

// FinalizeRuntimeGovernanceResolutionProposalWithTransition reports whether
// the public ticket moved from open to resolved during this invocation.
func (s *Service) FinalizeRuntimeGovernanceResolutionProposalWithTransition(ctx context.Context, cmd decisioncontracts.ResolveTicketCommand) (*decisionModel.Ticket, *decisionModel.Resolution, bool, error) {
	if s == nil || cmd.ResolutionID == "" || cmd.TicketID == "" || cmd.Type == "" {
		return nil, nil, false, common.ErrInvalidArgument
	}
	ctx, err := resolveCommandContext(ctx, cmd)
	if err != nil {
		return nil, nil, false, err
	}
	ticket, err := s.tickets.Get(ctx, cmd.TicketID)
	if err != nil {
		return nil, nil, false, err
	}
	if !ticketMatchesResolveScope(ticket, cmd) || ticket.DecisionKind != "runtime_governance" {
		return nil, nil, false, common.ErrNotFound
	}
	proposal, err := runtimeGovernanceProposalFromCommand(cmd)
	if err != nil {
		return nil, nil, false, err
	}
	if ticket.RuntimeGovernanceProposal == nil || !sameRuntimeGovernanceProposal(*ticket.RuntimeGovernanceProposal, proposal) {
		return nil, nil, false, common.ErrConflict
	}
	if ticket.RuntimeGovernanceProposal.State == decisionModel.RuntimeGovernanceResolutionProposalRejected {
		return nil, nil, false, common.ErrConflict
	}
	resolution, err := s.resolutions.GetByTicketID(ctx, cmd.TicketID)
	if err != nil && !errors.Is(err, common.ErrNotFound) {
		return nil, nil, false, err
	}
	if resolution != nil {
		if resolution.ID != cmd.ResolutionID || resolution.Type != cmd.Type || !sameResolutionPayload(resolution.Payload, common.RedactSensitivePayload(cmd.Payload)) {
			return nil, nil, false, common.ErrConflict
		}
	} else {
		resolution = resolutionFromResolveCommand(cmd)
		if err := s.resolutions.Create(ctx, resolution); err != nil {
			return nil, nil, false, err
		}
	}
	if ticket.State != common.DecisionTicketStateOpen && ticket.State != common.DecisionTicketStateResolved {
		return nil, nil, false, common.ErrConflict
	}
	transitioned := ticket.State == common.DecisionTicketStateOpen
	now := time.Now().UTC().Format(time.RFC3339Nano)
	ticket.State = common.DecisionTicketStateResolved
	ticket.ResolvedAt = nonEmptyString(ticket.ResolvedAt, now)
	ticket.ResolvedBy = nonEmptyString(ticket.ResolvedBy, stringPayloadValue(cmd.Payload, "resolvedBy"), stringPayloadValue(cmd.Payload, "actor"))
	ticket.RuntimeGovernanceProposal.State = decisionModel.RuntimeGovernanceResolutionProposalCommitted
	ticket.RuntimeGovernanceProposal.FailureClass = ""
	ticket.RuntimeGovernanceProposal.FailureReason = ""
	ticket.RuntimeGovernanceProposal.UpdatedAt = now
	if err := s.tickets.Update(ctx, ticket); err != nil {
		return nil, nil, false, err
	}
	return ticket, resolution, transitioned, nil
}

func (s *Service) RejectRuntimeGovernanceResolutionProposal(ctx context.Context, cmd decisioncontracts.ResolveTicketCommand, failureClass string, failureReason string) error {
	if s == nil || cmd.ResolutionID == "" || cmd.TicketID == "" || cmd.Type == "" {
		return common.ErrInvalidArgument
	}
	ctx, err := resolveCommandContext(ctx, cmd)
	if err != nil {
		return err
	}
	ticket, err := s.tickets.Get(ctx, cmd.TicketID)
	if err != nil {
		return err
	}
	proposal, err := runtimeGovernanceProposalFromCommand(cmd)
	if err != nil {
		return err
	}
	if ticket.DecisionKind != "runtime_governance" || ticket.State != common.DecisionTicketStateOpen || ticket.RuntimeGovernanceProposal == nil || !sameRuntimeGovernanceProposal(*ticket.RuntimeGovernanceProposal, proposal) {
		return common.ErrConflict
	}
	ticket.RuntimeGovernanceProposal.State = decisionModel.RuntimeGovernanceResolutionProposalRejected
	ticket.RuntimeGovernanceProposal.FailureClass = strings.TrimSpace(failureClass)
	ticket.RuntimeGovernanceProposal.FailureReason = strings.TrimSpace(failureReason)
	ticket.RuntimeGovernanceProposal.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	return s.tickets.Update(ctx, ticket)
}

func runtimeGovernanceProposalFromCommand(cmd decisioncontracts.ResolveTicketCommand) (decisionModel.RuntimeGovernanceResolutionProposal, error) {
	payload := common.RedactSensitivePayload(cmd.Payload)
	hash, err := runtimeGovernanceProposalPayloadHash(cmd, payload)
	if err != nil {
		return decisionModel.RuntimeGovernanceResolutionProposal{}, err
	}
	return decisionModel.RuntimeGovernanceResolutionProposal{
		ResolutionID:       cmd.ResolutionID,
		ResolutionType:     cmd.Type,
		Payload:            payload,
		ContinuationAction: nonEmptyString(cmd.ContinuationAction, stringPayloadValue(payload, "continuationAction")),
		ResumeRunID:        nonEmptyRunID(cmd.ResumeRunID, common.RunID(stringPayloadValue(payload, "resumeRunId"))),
		SuccessorRunID:     nonEmptyRunID(cmd.SuccessorRunID, common.RunID(stringPayloadValue(payload, "successorRunId"))),
		PayloadHash:        hash,
		State:              decisionModel.RuntimeGovernanceResolutionProposalPending,
	}, nil
}

func runtimeGovernanceProposalPayloadHash(cmd decisioncontracts.ResolveTicketCommand, payload workorcheventpayload.Payload) (string, error) {
	encoded, err := json.Marshal(struct {
		ResolutionID       common.ResolutionID
		ResolutionType     string
		Payload            workorcheventpayload.Payload
		ContinuationAction string
		ResumeRunID        common.RunID
		SuccessorRunID     common.RunID
	}{
		ResolutionID: cmd.ResolutionID, ResolutionType: cmd.Type, Payload: payload,
		ContinuationAction: nonEmptyString(cmd.ContinuationAction, stringPayloadValue(payload, "continuationAction")),
		ResumeRunID:        nonEmptyRunID(cmd.ResumeRunID, common.RunID(stringPayloadValue(payload, "resumeRunId"))),
		SuccessorRunID:     nonEmptyRunID(cmd.SuccessorRunID, common.RunID(stringPayloadValue(payload, "successorRunId"))),
	})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

func sameRuntimeGovernanceProposal(left decisionModel.RuntimeGovernanceResolutionProposal, right decisionModel.RuntimeGovernanceResolutionProposal) bool {
	return left.ResolutionID == right.ResolutionID &&
		left.ResolutionType == right.ResolutionType &&
		left.PayloadHash == right.PayloadHash &&
		left.ContinuationAction == right.ContinuationAction &&
		left.ResumeRunID == right.ResumeRunID &&
		left.SuccessorRunID == right.SuccessorRunID &&
		left.Payload.Equal(right.Payload)
}

func resolutionFromResolveCommand(cmd decisioncontracts.ResolveTicketCommand) *decisionModel.Resolution {
	return &decisionModel.Resolution{
		ID:                 cmd.ResolutionID,
		TicketID:           cmd.TicketID,
		Type:               cmd.Type,
		Payload:            common.RedactSensitivePayload(cmd.Payload),
		ContinuationAction: nonEmptyString(cmd.ContinuationAction, stringPayloadValue(cmd.Payload, "continuationAction")),
		ResumeRunID:        nonEmptyRunID(cmd.ResumeRunID, common.RunID(stringPayloadValue(cmd.Payload, "resumeRunId"))),
		SuccessorRunID:     nonEmptyRunID(cmd.SuccessorRunID, common.RunID(stringPayloadValue(cmd.Payload, "successorRunId"))),
	}
}
