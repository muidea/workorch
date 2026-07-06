package decision

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"time"

	modulecommon "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/common"
	decisioncontracts "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/contracts"
	decisionevents "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/events"
	decisionModel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
	orchestratormodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type Service struct {
	tickets         repository.DecisionTicketRepository
	resolutions     repository.DecisionResolutionRepository
	evidenceRecords repository.EvidenceRecordRepository
	eventHub        events.Hub
}

func NewService(tickets repository.DecisionTicketRepository, resolutions repository.DecisionResolutionRepository, eventHub events.Hub, evidenceRecords ...repository.EvidenceRecordRepository) *Service {
	service := &Service{tickets: tickets, resolutions: resolutions, eventHub: eventHub}
	if len(evidenceRecords) > 0 {
		service.evidenceRecords = evidenceRecords[0]
	}
	return service
}

func (s *Service) CreateTicket(ctx context.Context, cmd decisioncontracts.CreateTicketCommand) error {
	if cmd.TicketID == "" || cmd.WorkspaceID == "" || cmd.RunID == "" || cmd.Reason == "" {
		return common.ErrInvalidArgument
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
		return err
	}
	if s.eventHub == nil {
		return nil
	}
	return events.PublishByContract(ctx, s.eventHub, events.Envelope{
		Version:      events.WorkorchEventVersion,
		Topic:        decisionevents.TopicTicketCreated,
		SourceModule: modulecommon.ModuleID,
		SubjectType:  "decision_ticket",
		SubjectID:    string(item.ID),
		WorkspaceID:  string(item.WorkspaceID),
		RunID:        string(item.RunID),
		StepID:       string(item.StepID),
		Payload:      events.DecisionTicketPayload(item.Reason, item.State),
	})
}

func (s *Service) EnsureTicket(ctx context.Context, cmd decisioncontracts.CreateTicketCommand) error {
	if cmd.TicketID == "" || cmd.WorkspaceID == "" || cmd.RunID == "" || cmd.Reason == "" {
		return common.ErrInvalidArgument
	}

	item, err := s.tickets.Get(ctx, cmd.TicketID)
	if err == nil {
		if item.WorkspaceID != cmd.WorkspaceID ||
			item.RunID != cmd.RunID ||
			item.StepID != cmd.StepID ||
			item.Reason != cmd.Reason ||
			item.DecisionKind != cmd.DecisionKind ||
			item.Question != cmd.Question ||
			!sameResolutionPayload(item.Context, cmd.Context) {
			return common.ErrConflict
		}
		return nil
	}
	if !errors.Is(err, common.ErrNotFound) {
		return err
	}
	return s.CreateTicket(ctx, cmd)
}

func (s *Service) ResolveTicket(ctx context.Context, cmd decisioncontracts.ResolveTicketCommand) error {
	if cmd.ResolutionID == "" || cmd.TicketID == "" || cmd.Type == "" {
		return common.ErrInvalidArgument
	}
	if !modulecommon.IsSupportedResolutionType(cmd.Type) {
		return common.ErrInvalidArgument
	}
	ctx = resolveCommandContext(ctx, cmd)

	ticket, err := s.tickets.Get(ctx, cmd.TicketID)
	if err != nil {
		return err
	}
	if !ticketMatchesResolveScope(ticket, cmd) {
		return common.ErrNotFound
	}
	if ticket.State != common.DecisionTicketStateOpen {
		return common.ErrConflict
	}

	_, err = s.resolveTicketItem(ctx, ticket, cmd)
	return err
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
		Payload:            common.RedactSensitiveMap(clonePayload(cmd.Payload)),
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
	if err := s.recordDecisionResolutionEvidence(ctx, ticket, resolution); err != nil {
		return nil, err
	}
	if err := s.publishResolvedTicketEvent(ctx, ticket, resolution); err != nil {
		return nil, err
	}
	return resolution, nil
}

func (s *Service) EnsureResolvedTicket(ctx context.Context, cmd decisioncontracts.ResolveTicketCommand) error {
	_, _, err := s.EnsureResolvedTicketAndReturn(ctx, cmd)
	return err
}

func (s *Service) EnsureResolvedTicketAndReturn(ctx context.Context, cmd decisioncontracts.ResolveTicketCommand) (*decisionModel.Ticket, *decisionModel.Resolution, error) {
	if cmd.ResolutionID == "" || cmd.TicketID == "" || cmd.Type == "" {
		return nil, nil, common.ErrInvalidArgument
	}
	if !modulecommon.IsSupportedResolutionType(cmd.Type) {
		return nil, nil, common.ErrInvalidArgument
	}
	ctx = resolveCommandContext(ctx, cmd)

	ticket, err := s.tickets.Get(ctx, cmd.TicketID)
	if err != nil {
		return nil, nil, err
	}
	if !ticketMatchesResolveScope(ticket, cmd) {
		return nil, nil, common.ErrNotFound
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
		return nil, nil, resErr
	}
	if existingResolution != nil {
		if existingResolution.ID != cmd.ResolutionID ||
			existingResolution.Type != cmd.Type ||
			!sameResolutionPayload(existingResolution.Payload, cmd.Payload) {
			return nil, nil, common.ErrConflict
		}
		if ticket.State != common.DecisionTicketStateResolved {
			ticket.State = common.DecisionTicketStateResolved
			ticket.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
			if err := s.tickets.Update(ctx, ticket); err != nil {
				return nil, nil, err
			}
			if err := s.recordDecisionResolutionEvidence(ctx, ticket, existingResolution); err != nil {
				return nil, nil, err
			}
			if err := s.publishResolvedTicketEvent(ctx, ticket, existingResolution); err != nil {
				return nil, nil, err
			}
		} else if err := s.recordDecisionResolutionEvidence(ctx, ticket, existingResolution); err != nil {
			return nil, nil, err
		}
		return ticket, existingResolution, nil
	}

	if ticket.State == common.DecisionTicketStateResolved {
		// Resolution file was deleted but ticket state is still "resolved";
		// preserve the existing strict behavior. Re-creating a resolution
		// here would be a fresh write — treat as ErrConflict to surface the
		// invariant violation rather than silently accept.
		return nil, nil, common.ErrConflict
	}
	if ticket.State != common.DecisionTicketStateOpen {
		return nil, nil, common.ErrConflict
	}
	resolution, err := s.resolveTicketItem(ctx, ticket, cmd)
	if err != nil {
		return nil, nil, err
	}
	return ticket, resolution, nil
}

func (s *Service) publishResolvedTicketEvent(ctx context.Context, ticket *decisionModel.Ticket, resolution *decisionModel.Resolution) error {
	if s == nil || s.eventHub == nil || ticket == nil || resolution == nil {
		return nil
	}
	return events.PublishByContract(ctx, s.eventHub, events.Envelope{
		Version:      events.WorkorchEventVersion,
		Topic:        decisionevents.TopicTicketResolved,
		SourceModule: modulecommon.ModuleID,
		SubjectType:  "decision_ticket",
		SubjectID:    string(ticket.ID),
		WorkspaceID:  string(ticket.WorkspaceID),
		RunID:        string(ticket.RunID),
		StepID:       string(ticket.StepID),
		Payload: events.DecisionResolutionPayload(
			ticket.Reason,
			ticket.State,
			resolution.ID,
			resolution.Type,
			resolution.Payload,
			events.DecisionResolutionPayloadData{
				ContinuationAction: resolution.ContinuationAction,
				ResumeRunID:        resolution.ResumeRunID,
				SuccessorRunID:     resolution.SuccessorRunID,
			},
		),
	})
}

func (s *Service) recordDecisionResolutionEvidence(ctx context.Context, ticket *decisionModel.Ticket, resolution *decisionModel.Resolution) error {
	if s == nil || s.evidenceRecords == nil || ticket == nil || resolution == nil || resolution.ID == "" {
		return nil
	}
	timestamp := strings.TrimSpace(ticket.UpdatedAt)
	if timestamp == "" {
		timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	}
	metadata := map[string]any{
		"ticketId":       string(ticket.ID),
		"resolutionId":   string(resolution.ID),
		"resolutionType": strings.TrimSpace(resolution.Type),
		"decisionKind":   strings.TrimSpace(ticket.DecisionKind),
	}
	if ticket.Reason != "" {
		metadata["reason"] = strings.TrimSpace(ticket.Reason)
	}
	if ticket.Question != "" {
		metadata["question"] = strings.TrimSpace(ticket.Question)
	}
	if len(resolution.Payload) > 0 {
		metadata["resolutionPayload"] = clonePayload(resolution.Payload)
	}
	if resolution.ContinuationAction != "" {
		metadata["continuationAction"] = strings.TrimSpace(resolution.ContinuationAction)
	}
	if resolution.ResumeRunID != "" {
		metadata["resumeRunId"] = string(resolution.ResumeRunID)
	}
	if resolution.SuccessorRunID != "" {
		metadata["successorRunId"] = string(resolution.SuccessorRunID)
	}
	record := &orchestratormodel.EvidenceRecord{
		ID:          orchestratormodel.StableEvidenceRecordID(string(resolution.ID), "decision-resolution", "decision"),
		WorkspaceID: ticket.WorkspaceID,
		RunID:       ticket.RunID,
		StepID:      ticket.StepID,
		Kind:        orchestratormodel.EvidenceKindDecision,
		Status:      orchestratormodel.EvidenceStatusAccepted,
		SourceType:  "decision_resolution",
		SourceRef:   string(resolution.ID),
		SubjectType: "decision_ticket",
		SubjectID:   string(ticket.ID),
		CreatedAt:   timestamp,
		UpdatedAt:   timestamp,
		Metadata:    metadata,
	}
	return repository.CreateOrUpdateEvidenceRecord(ctx, s.evidenceRecords, record)
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

func resolveCommandContext(ctx context.Context, cmd decisioncontracts.ResolveTicketCommand) context.Context {
	if cmd.WorkspaceID == "" {
		return ctx
	}
	return common.ContextWithRuntimeWorkspaceID(ctx, cmd.WorkspaceID)
}

func sameResolutionPayload(left map[string]any, right map[string]any) bool {
	if len(left) == 0 && len(right) == 0 {
		return true
	}
	return reflect.DeepEqual(left, right)
}

func clonePayload(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	out := make(map[string]any, len(input))
	for k, v := range input {
		out[k] = v
	}
	return out
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

func stringPayloadValue(payload map[string]any, key string) string {
	value, _ := payload[key].(string)
	return value
}

func stringSlicePayloadValue(payload map[string]any, key string) []string {
	switch typed := payload[key].(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		ret := make([]string, 0, len(typed))
		for _, item := range typed {
			if value, ok := item.(string); ok && strings.TrimSpace(value) != "" {
				ret = append(ret, strings.TrimSpace(value))
			}
		}
		return ret
	default:
		return nil
	}
}
