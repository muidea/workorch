package decision

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	definitioncontract "github.com/muidea/workorch/internal/pkg/workorchdefinitioncontract"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

const (
	workspaceDefinitionClarificationSource = "application/workorchd"
	workspaceDefinitionFactQuerySource     = "blocks/workspace"
	runtimeDefinitionClarificationSource   = "kernel/orchestrator"
	runtimeDefinitionReplacementSource     = "blocks/run"
)

func definitionClarificationTopics() []string {
	return []string{
		decisionevents.TopicOpenWorkspaceDefinitionClarification,
		decisionevents.TopicReconcileWorkspaceDefinitionClarificationCandidate,
		decisionevents.TopicResolveWorkspaceDefinitionClarificationByAdmission,
		decisionevents.TopicQueryWorkspaceDefinitionClarificationFact,
		decisionevents.TopicOpenRuntimeDefinitionClarification,
		decisionevents.TopicResolveRuntimeDefinitionClarificationByReplacement,
		decisionevents.TopicRepairRuntimeDefinitionClarificationRequestContract,
	}
}

func (m *Biz) handleQueryWorkspaceDefinitionClarificationFact(ev magicevent.Event, result magicevent.Result) {
	cmd, ok := ev.Data().(decisionevents.QueryWorkspaceDefinitionClarificationFactCommand)
	if !ok || ev.Source() != workspaceDefinitionFactQuerySource || m == nil || m.tickets == nil || m.resolutions == nil {
		setDefinitionClarificationResult(result, decisionevents.QueryWorkspaceDefinitionClarificationFactResult{}, common.ErrInvalidArgument)
		return
	}
	unlock := m.lockWorkspaceMutation(cmd.WorkspaceID)
	defer unlock()
	if err := m.rejectFencedWorkspace(ev.Context(), cmd.WorkspaceID); err != nil {
		setDefinitionClarificationResult(result, decisionevents.QueryWorkspaceDefinitionClarificationFactResult{}, err)
		return
	}
	response, err := m.queryWorkspaceDefinitionClarificationFact(ev.Context(), cmd)
	setDefinitionClarificationResult(result, response, err)
}

func (m *Biz) handleReconcileWorkspaceDefinitionClarification(ev magicevent.Event, result magicevent.Result) {
	cmd, ok := ev.Data().(decisionevents.ReconcileWorkspaceDefinitionClarificationCandidateCommand)
	if !ok || ev.Source() != workspaceDefinitionClarificationSource || m == nil || m.tickets == nil || m.resolutions == nil {
		setDefinitionClarificationResult(result, decisionevents.ReconcileWorkspaceDefinitionClarificationCandidateResult{}, common.ErrInvalidArgument)
		return
	}
	cmd = cmd.Normalized()
	if err := cmd.Validate(); err != nil {
		setDefinitionClarificationResult(result, decisionevents.ReconcileWorkspaceDefinitionClarificationCandidateResult{}, fmt.Errorf("%w: %v", common.ErrInvalidArgument, err))
		return
	}
	unlock := m.lockWorkspaceMutation(cmd.NewRequest.WorkspaceID)
	defer unlock()
	if err := m.rejectFencedWorkspace(ev.Context(), cmd.NewRequest.WorkspaceID); err != nil {
		setDefinitionClarificationResult(result, decisionevents.ReconcileWorkspaceDefinitionClarificationCandidateResult{}, err)
		return
	}
	response, err := m.reconcileWorkspaceDefinitionClarification(ev.Context(), cmd)
	setDefinitionClarificationResult(result, response, err)
}

func (m *Biz) handleOpenWorkspaceDefinitionClarification(ev magicevent.Event, result magicevent.Result) {
	cmd, ok := ev.Data().(decisionevents.OpenWorkspaceDefinitionClarificationRequestCommand)
	if !ok || ev.Source() != workspaceDefinitionClarificationSource || m == nil || m.tickets == nil {
		setDefinitionClarificationResult(result, decisionevents.OpenWorkspaceDefinitionClarificationRequestResult{}, common.ErrInvalidArgument)
		return
	}
	unlock := m.lockWorkspaceMutation(cmd.WorkspaceID)
	defer unlock()
	if err := m.rejectFencedWorkspace(ev.Context(), cmd.WorkspaceID); err != nil {
		setDefinitionClarificationResult(result, decisionevents.OpenWorkspaceDefinitionClarificationRequestResult{}, err)
		return
	}
	response, err := m.openWorkspaceDefinitionClarification(ev.Context(), cmd)
	setDefinitionClarificationResult(result, response, err)
}

func (m *Biz) handleResolveWorkspaceDefinitionClarification(ev magicevent.Event, result magicevent.Result) {
	cmd, ok := ev.Data().(decisionevents.ResolveWorkspaceDefinitionClarificationByAdmissionCommand)
	if !ok || ev.Source() != workspaceDefinitionClarificationSource || m == nil || m.tickets == nil || m.resolutions == nil {
		setDefinitionClarificationResult(result, decisionevents.ResolveWorkspaceDefinitionClarificationByAdmissionResult{}, common.ErrInvalidArgument)
		return
	}
	unlock := m.lockWorkspaceMutation(cmd.WorkspaceID)
	defer unlock()
	if err := m.rejectFencedWorkspace(ev.Context(), cmd.WorkspaceID); err != nil {
		setDefinitionClarificationResult(result, decisionevents.ResolveWorkspaceDefinitionClarificationByAdmissionResult{}, err)
		return
	}
	response, err := m.resolveWorkspaceDefinitionClarification(ev.Context(), cmd)
	setDefinitionClarificationResult(result, response, err)
}

func (m *Biz) handleOpenRuntimeDefinitionClarification(ev magicevent.Event, result magicevent.Result) {
	cmd, ok := ev.Data().(decisionevents.OpenRuntimeDefinitionClarificationRequestCommand)
	if !ok || ev.Source() != runtimeDefinitionClarificationSource || m == nil || m.tickets == nil {
		setDefinitionClarificationResult(result, decisionevents.OpenRuntimeDefinitionClarificationRequestResult{}, common.ErrInvalidArgument)
		return
	}
	unlock := m.lockWorkspaceMutation(cmd.WorkspaceID)
	defer unlock()
	if err := m.rejectFencedWorkspace(ev.Context(), cmd.WorkspaceID); err != nil {
		setDefinitionClarificationResult(result, decisionevents.OpenRuntimeDefinitionClarificationRequestResult{}, err)
		return
	}
	response, err := m.openRuntimeDefinitionClarification(ev.Context(), cmd)
	setDefinitionClarificationResult(result, response, err)
}

func (m *Biz) handleResolveRuntimeDefinitionClarification(ev magicevent.Event, result magicevent.Result) {
	cmd, ok := ev.Data().(decisionevents.ResolveRuntimeDefinitionClarificationByReplacementCommand)
	if !ok || ev.Source() != runtimeDefinitionReplacementSource || m == nil || m.tickets == nil || m.resolutions == nil {
		setDefinitionClarificationResult(result, decisionevents.ResolveRuntimeDefinitionClarificationByReplacementResult{}, common.ErrInvalidArgument)
		return
	}
	unlock := m.lockWorkspaceMutation(cmd.WorkspaceID)
	defer unlock()
	if err := m.rejectFencedWorkspace(ev.Context(), cmd.WorkspaceID); err != nil {
		setDefinitionClarificationResult(result, decisionevents.ResolveRuntimeDefinitionClarificationByReplacementResult{}, err)
		return
	}
	response, err := m.resolveRuntimeDefinitionClarification(ev.Context(), cmd)
	setDefinitionClarificationResult(result, response, err)
}

func setDefinitionClarificationResult[T any](result magicevent.Result, response T, err error) {
	if result == nil {
		return
	}
	if err != nil {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
		return
	}
	result.Set(response, nil)
}

func (m *Biz) openWorkspaceDefinitionClarification(ctx context.Context, cmd decisionevents.OpenWorkspaceDefinitionClarificationRequestCommand) (decisionevents.OpenWorkspaceDefinitionClarificationRequestResult, error) {
	cmd = cmd.Normalized()
	if err := cmd.Validate(); err != nil {
		return decisionevents.OpenWorkspaceDefinitionClarificationRequestResult{}, fmt.Errorf("%w: %v", common.ErrInvalidArgument, err)
	}
	bound, err := common.BindRuntimeWorkspaceContext(ctx, cmd.WorkspaceID)
	if err != nil {
		return decisionevents.OpenWorkspaceDefinitionClarificationRequestResult{}, err
	}
	bindingHash := cmd.BindingHash()
	if existing, err := m.tickets.Get(bound, cmd.TicketID); err == nil {
		binding := existing.DefinitionClarification
		if !workspaceDefinitionClarificationTicketSemanticsMatch(existing, cmd) ||
			!workspaceDefinitionClarificationBindingMatchesCommand(binding, cmd) ||
			!workspaceDefinitionClarificationLifecycleBindingConsistent(existing, binding) {
			return decisionevents.OpenWorkspaceDefinitionClarificationRequestResult{}, common.ErrConflict
		}
		if err := m.validateResolvedDefinitionClarificationRecord(bound, existing); err != nil {
			return decisionevents.OpenWorkspaceDefinitionClarificationRequestResult{}, err
		}
		// Ticket creation and its audit notification marker are separate durable
		// cuts. Exact replay must finish the owner-local delivery transaction after
		// a process dies between them.
		if err := m.ensureTicketCreatedNotification(bound, existing); err != nil {
			return decisionevents.OpenWorkspaceDefinitionClarificationRequestResult{}, err
		}
		return decisionevents.OpenWorkspaceDefinitionClarificationRequestResult{TicketID: cmd.TicketID, WorkspaceID: cmd.WorkspaceID, CandidateID: cmd.CandidateID, CandidateRevision: cmd.CandidateRevision, Disposition: decisionevents.DefinitionClarificationDuplicate, ClarificationRequestBindingHash: bindingHash}, nil
	} else if !errors.Is(err, common.ErrNotFound) {
		return decisionevents.OpenWorkspaceDefinitionClarificationRequestResult{}, err
	}
	seed := workspaceDefinitionClarificationTicketForCommand(cmd)
	ticket, err := m.service.applyLifecycleInternalEvent(bound, nil, lifecycleInternalEvent{Kind: lifecycleInternalEventOpened, TicketID: seed.ID, Ticket: seed})
	if err != nil {
		return decisionevents.OpenWorkspaceDefinitionClarificationRequestResult{}, err
	}
	if err := m.ensureTicketCreatedNotification(bound, ticket); err != nil {
		return decisionevents.OpenWorkspaceDefinitionClarificationRequestResult{}, err
	}
	return decisionevents.OpenWorkspaceDefinitionClarificationRequestResult{TicketID: cmd.TicketID, WorkspaceID: cmd.WorkspaceID, CandidateID: cmd.CandidateID, CandidateRevision: cmd.CandidateRevision, Disposition: decisionevents.DefinitionClarificationCreated, ClarificationRequestBindingHash: bindingHash}, nil
}

func (m *Biz) queryWorkspaceDefinitionClarificationFact(ctx context.Context, cmd decisionevents.QueryWorkspaceDefinitionClarificationFactCommand) (decisionevents.QueryWorkspaceDefinitionClarificationFactResult, error) {
	if err := cmd.Validate(); err != nil {
		return decisionevents.QueryWorkspaceDefinitionClarificationFactResult{}, fmt.Errorf("%w: %v", common.ErrInvalidArgument, err)
	}
	bound, err := common.BindRuntimeWorkspaceContext(ctx, cmd.WorkspaceID)
	if err != nil {
		return decisionevents.QueryWorkspaceDefinitionClarificationFactResult{}, err
	}
	ticket, err := m.tickets.Get(bound, cmd.TicketID)
	if errors.Is(err, common.ErrNotFound) {
		return decisionevents.QueryWorkspaceDefinitionClarificationFactResult{Found: false}, nil
	}
	if err != nil {
		return decisionevents.QueryWorkspaceDefinitionClarificationFactResult{}, err
	}
	binding := ticket.DefinitionClarification
	if binding == nil || binding.Class != decisionmodel.DefinitionClarificationWorkspacePreSeal {
		return decisionevents.QueryWorkspaceDefinitionClarificationFactResult{}, common.ErrConflict
	}
	request := definitioncontract.WorkspaceDefinitionClarificationRequestIdentity{
		SchemaVersion: binding.SchemaVersion, TicketID: ticket.ID, WorkspaceID: ticket.WorkspaceID,
		CandidateID: binding.CandidateID, CandidateRevision: binding.CandidateRevision, CandidateHash: binding.CandidateHash,
		ViolationCode: binding.ViolationOrGapCode, OffendingRefs: append([]string(nil), binding.OffendingRefs...),
		CanonicalSignatureVersion: binding.CanonicalSignatureVersion, CanonicalSignature: binding.CanonicalSignature,
		CanonicalSignatureHash: binding.CanonicalSignatureHash, RequiredActionKind: binding.RequiredActionKind,
	}.Normalized()
	ticketRecord := definitioncontract.WorkspaceDefinitionClarificationTicketRecord{
		RequestIdentity: request, OpenBindingHash: binding.OpenBindingHash,
	}.Normalized()
	requestCommand := decisionevents.OpenWorkspaceDefinitionClarificationRequestCommand(request)
	if err := ticketRecord.Validate(); err != nil ||
		!workspaceDefinitionClarificationTicketSemanticsMatch(ticket, requestCommand) ||
		!workspaceDefinitionClarificationBindingMatchesCommand(binding, requestCommand) {
		return decisionevents.QueryWorkspaceDefinitionClarificationFactResult{}, common.ErrConflict
	}
	response := decisionevents.QueryWorkspaceDefinitionClarificationFactResult{Found: true, Ticket: &ticketRecord}
	resolutionIdentityPresent := binding.ResolutionCandidateID != "" || binding.ResolutionCandidateRevision != 0 || binding.ResolutionCandidateHash != "" || binding.AdmissionResultHash != "" || binding.DefinitionRevision != "" || binding.StartupInputHash != ""
	if !resolutionIdentityPresent {
		if ticket.State != common.DecisionTicketStateOpen || binding.ResolutionID != "" || binding.ResolutionBindingHash != "" ||
			binding.ReplacementIntentID != "" || binding.ReplacementIntentHash != "" || binding.SuccessorDefinitionRevision != "" || binding.SuccessorStartupInputHash != "" {
			return decisionevents.QueryWorkspaceDefinitionClarificationFactResult{}, common.ErrConflict
		}
		if err := response.ValidateFor(cmd); err != nil {
			return decisionevents.QueryWorkspaceDefinitionClarificationFactResult{}, common.ErrConflict
		}
		return response, nil
	}
	if ticket.State != common.DecisionTicketStateResolved || binding.ResolutionID == "" || binding.ResolutionBindingHash == "" ||
		binding.ReplacementIntentID != "" || binding.ReplacementIntentHash != "" || binding.SuccessorDefinitionRevision != "" || binding.SuccessorStartupInputHash != "" {
		return decisionevents.QueryWorkspaceDefinitionClarificationFactResult{}, common.ErrConflict
	}
	identity := definitioncontract.WorkspaceDefinitionClarificationAdmissionResolutionIdentity{
		TicketID: ticket.ID, WorkspaceID: ticket.WorkspaceID,
		OldCandidateID: binding.CandidateID, OldCandidateRevision: binding.CandidateRevision, OldCandidateHash: binding.CandidateHash,
		NewCandidateID: binding.ResolutionCandidateID, NewCandidateRevision: binding.ResolutionCandidateRevision, NewCandidateHash: binding.ResolutionCandidateHash,
		AdmissionResultHash: binding.AdmissionResultHash, DefinitionRevision: binding.DefinitionRevision, StartupInputHash: binding.StartupInputHash,
	}.Normalized()
	record := definitioncontract.WorkspaceDefinitionClarificationResolutionRecord{
		ResolutionIdentity: identity, ResolutionID: binding.ResolutionID,
		ResolutionType:        definitioncontract.DefinitionClarificationResolutionCandidateAdjusted,
		ResolutionBindingHash: binding.ResolutionBindingHash,
	}.Normalized()
	if err := record.Validate(); err != nil {
		return decisionevents.QueryWorkspaceDefinitionClarificationFactResult{}, common.ErrConflict
	}
	persistedResolution, err := m.resolutions.GetByTicketID(bound, ticket.ID)
	if err != nil {
		return decisionevents.QueryWorkspaceDefinitionClarificationFactResult{}, err
	}
	expectedResolution := &decisionmodel.Resolution{
		ID: record.ResolutionID, TicketID: ticket.ID, Type: record.ResolutionType,
		Payload: workorcheventpayload.New(
			workorcheventpayload.StringField("definitionRevision", identity.DefinitionRevision),
			workorcheventpayload.StringField("startupInputHash", identity.StartupInputHash),
		),
	}
	if !definitionClarificationResolutionMatches(persistedResolution, expectedResolution) ||
		strings.TrimSpace(persistedResolution.ResolvedAt) != strings.TrimSpace(ticket.ResolvedAt) {
		return decisionevents.QueryWorkspaceDefinitionClarificationFactResult{}, common.ErrConflict
	}
	response.Resolution = &record
	if err := response.ValidateFor(cmd); err != nil {
		return decisionevents.QueryWorkspaceDefinitionClarificationFactResult{}, common.ErrConflict
	}
	return response, nil
}

func workspaceDefinitionClarificationQuestion(requiredAction string) string {
	if requiredAction == decisionevents.DefinitionClarificationActionFixTemplateProfile {
		return "Select or fix the Workspace Template/Profile proof contract"
	}
	return "Revise the Definition into explicit all-of semantics"
}

// workspaceDefinitionClarificationTicketSemanticsMatch verifies the immutable
// outer Ticket semantics selected by Decision when it opens a Workspace
// clarification. A valid DefinitionClarification binding is not sufficient
// provenance when the enclosing aggregate was repurposed or partially edited.
func workspaceDefinitionClarificationTicketSemanticsMatch(ticket *decisionmodel.Ticket, cmd decisionevents.OpenWorkspaceDefinitionClarificationRequestCommand) bool {
	if ticket == nil {
		return false
	}
	cmd = cmd.Normalized()
	expected := workspaceDefinitionClarificationTicketForCommand(cmd)
	return ticket.ID == expected.ID &&
		ticket.WorkspaceID == expected.WorkspaceID &&
		ticket.RunID == "" &&
		ticket.StepID == "" &&
		ticket.Reason == expected.Reason &&
		ticket.DecisionKind == expected.DecisionKind &&
		ticket.Question == expected.Question &&
		ticket.Context.Equal(expected.Context) &&
		ticket.AutoGovernance.Equal(expected.AutoGovernance) &&
		ticket.FailureClass == expected.FailureClass &&
		ticket.FailureSignature == expected.FailureSignature &&
		slices.Equal(ticket.AllowedRecoveryActions, expected.AllowedRecoveryActions) &&
		ticket.RuntimeGovernanceProposal == nil &&
		workspaceDefinitionClarificationTicketStateTimeMatches(ticket)
}

// workspaceDefinitionClarificationTicketStateTimeMatches rejects an enclosing
// Ticket whose lifecycle metadata cannot have been produced by Decision's
// private reducer. Workspace Definition clarifications never use the generic
// resolution-recovered transition: their open->resolved transition always
// stamps ResolvedAt and never selects an operator ResolvedBy. Resolution
// notification publication is a later idempotent cut point, so it is optional
// only after the ticket is resolved.
func workspaceDefinitionClarificationTicketStateTimeMatches(ticket *decisionmodel.Ticket) bool {
	if ticket == nil {
		return false
	}
	switch ticket.State {
	case common.DecisionTicketStateOpen:
		return ticket.ResolvedAt == "" && ticket.ResolvedBy == "" && ticket.ResolutionNotificationPublishedAt == ""
	case common.DecisionTicketStateResolved:
		return ticket.ResolvedAt != "" && ticket.ResolvedBy == ""
	default:
		return false
	}
}

func workspaceDefinitionClarificationTicketForCommand(cmd decisionevents.OpenWorkspaceDefinitionClarificationRequestCommand) *decisionmodel.Ticket {
	cmd = cmd.Normalized()
	return &decisionmodel.Ticket{
		ID:                      cmd.TicketID,
		WorkspaceID:             cmd.WorkspaceID,
		Reason:                  cmd.ViolationCode,
		DecisionKind:            "workspace_definition_clarification",
		Question:                workspaceDefinitionClarificationQuestion(cmd.RequiredActionKind),
		FailureClass:            "definition_admission",
		FailureSignature:        cmd.CanonicalSignatureHash,
		AllowedRecoveryActions:  []string{cmd.RequiredActionKind},
		DefinitionClarification: workspaceDefinitionClarificationBindingForCommand(cmd),
	}
}

func workspaceDefinitionClarificationTicketSelfConsistent(ticket *decisionmodel.Ticket) bool {
	command, ok := workspaceDefinitionClarificationCommandFromTicket(ticket)
	if !ok {
		return false
	}
	binding := ticket.DefinitionClarification
	return workspaceDefinitionClarificationTicketSemanticsMatch(ticket, command) &&
		workspaceDefinitionClarificationBindingMatchesCommand(binding, command) &&
		workspaceDefinitionClarificationLifecycleBindingConsistent(ticket, binding)
}

func workspaceDefinitionClarificationCommandFromTicket(ticket *decisionmodel.Ticket) (decisionevents.OpenWorkspaceDefinitionClarificationRequestCommand, bool) {
	if ticket == nil || ticket.DefinitionClarification == nil {
		return decisionevents.OpenWorkspaceDefinitionClarificationRequestCommand{}, false
	}
	binding := ticket.DefinitionClarification
	command := decisionevents.OpenWorkspaceDefinitionClarificationRequestCommand(
		definitioncontract.WorkspaceDefinitionClarificationRequestIdentity{
			SchemaVersion: binding.SchemaVersion, TicketID: ticket.ID, WorkspaceID: ticket.WorkspaceID,
			CandidateID: binding.CandidateID, CandidateRevision: binding.CandidateRevision, CandidateHash: binding.CandidateHash,
			ViolationCode: binding.ViolationOrGapCode, OffendingRefs: append([]string(nil), binding.OffendingRefs...),
			CanonicalSignatureVersion: binding.CanonicalSignatureVersion, CanonicalSignature: binding.CanonicalSignature,
			CanonicalSignatureHash: binding.CanonicalSignatureHash, RequiredActionKind: binding.RequiredActionKind,
		},
	).Normalized()
	return command, command.Validate() == nil
}

// validateResolvedDefinitionClarificationRecord fences the two Decision-owned
// durable rows as one resolved aggregate. Resolution may legally precede the
// Ticket CAS while the Ticket is still open; once the Ticket says resolved,
// however, exact replay/open/preflight must prove the matching Resolution row
// and timestamp before publishing or mutating anything else.
func (m *Biz) validateResolvedDefinitionClarificationRecord(ctx context.Context, ticket *decisionmodel.Ticket) error {
	if ticket == nil || ticket.State != common.DecisionTicketStateResolved || ticket.DefinitionClarification == nil || m == nil || m.resolutions == nil {
		if ticket != nil && ticket.State != common.DecisionTicketStateResolved {
			return nil
		}
		return common.ErrConflict
	}
	binding := ticket.DefinitionClarification
	resolution, err := m.resolutions.GetByTicketID(ctx, ticket.ID)
	if err != nil {
		if errors.Is(err, common.ErrNotFound) {
			return common.ErrConflict
		}
		return err
	}
	if resolution.ID != binding.ResolutionID || resolution.TicketID != ticket.ID ||
		strings.TrimSpace(resolution.ResolvedAt) == "" ||
		strings.TrimSpace(resolution.ResolvedAt) != strings.TrimSpace(ticket.ResolvedAt) {
		return common.ErrConflict
	}

	switch binding.Class {
	case decisionmodel.DefinitionClarificationWorkspacePreSeal:
		resolutionIdentityPresent := binding.ResolutionCandidateID != "" || binding.ResolutionCandidateRevision != 0 ||
			binding.ResolutionCandidateHash != "" || binding.AdmissionResultHash != "" ||
			binding.DefinitionRevision != "" || binding.StartupInputHash != ""
		if resolutionIdentityPresent {
			identity := definitioncontract.WorkspaceDefinitionClarificationAdmissionResolutionIdentity{
				TicketID: ticket.ID, WorkspaceID: ticket.WorkspaceID,
				OldCandidateID: binding.CandidateID, OldCandidateRevision: binding.CandidateRevision, OldCandidateHash: binding.CandidateHash,
				NewCandidateID: binding.ResolutionCandidateID, NewCandidateRevision: binding.ResolutionCandidateRevision, NewCandidateHash: binding.ResolutionCandidateHash,
				AdmissionResultHash: binding.AdmissionResultHash, DefinitionRevision: binding.DefinitionRevision, StartupInputHash: binding.StartupInputHash,
			}.Normalized()
			expectedID := decisionevents.StableDefinitionClarificationResolutionID(ticket.ID, identity.AdmissionResultHash+"\x00"+identity.StartupInputHash)
			expected := &decisionmodel.Resolution{ID: expectedID, TicketID: ticket.ID, Type: decisionevents.DefinitionClarificationResolutionCandidateAdjusted,
				Payload: workorcheventpayload.New(workorcheventpayload.StringField("definitionRevision", identity.DefinitionRevision), workorcheventpayload.StringField("startupInputHash", identity.StartupInputHash))}
			if identity.Validate() != nil || binding.ResolutionBindingHash != identity.BindingHash(expectedID) ||
				!definitionClarificationResolutionMatches(resolution, expected) {
				return common.ErrConflict
			}
			return nil
		}

		oldRequest, ok := workspaceDefinitionClarificationCommandFromTicket(ticket)
		if !ok || resolution.Type != decisionevents.DefinitionClarificationResolutionCandidateSuperseded || len(resolution.Payload.Fields) != 2 {
			return common.ErrConflict
		}
		newTicketID, ticketOK := resolution.Payload.String("newTicketId")
		newCandidateHash, hashOK := resolution.Payload.String("newCandidateHash")
		if !ticketOK || !hashOK || strings.TrimSpace(newTicketID) == "" || strings.TrimSpace(newCandidateHash) == "" {
			return common.ErrConflict
		}
		successor, loadErr := m.tickets.Get(ctx, common.TicketID(newTicketID))
		if loadErr != nil {
			if errors.Is(loadErr, common.ErrNotFound) {
				return common.ErrConflict
			}
			return loadErr
		}
		newRequest, ok := workspaceDefinitionClarificationCommandFromTicket(successor)
		if !ok || newRequest.TicketID != common.TicketID(newTicketID) || newRequest.CandidateHash != newCandidateHash ||
			!workspaceDefinitionClarificationTicketSemanticsMatch(successor, newRequest) ||
			!workspaceDefinitionClarificationBindingMatchesCommand(successor.DefinitionClarification, newRequest) {
			return common.ErrConflict
		}
		reconcile := decisionevents.ReconcileWorkspaceDefinitionClarificationCandidateCommand{OldRequest: oldRequest, NewRequest: newRequest}.Normalized()
		expectedID := decisionevents.StableDefinitionClarificationResolutionID(ticket.ID, newRequest.BindingHash())
		expected := &decisionmodel.Resolution{ID: expectedID, TicketID: ticket.ID, Type: decisionevents.DefinitionClarificationResolutionCandidateSuperseded,
			Payload: workorcheventpayload.New(workorcheventpayload.StringField("newTicketId", newTicketID), workorcheventpayload.StringField("newCandidateHash", newCandidateHash))}
		if reconcile.Validate() != nil || binding.ResolutionBindingHash != reconcile.BindingHash(expectedID) ||
			!definitionClarificationResolutionMatches(resolution, expected) {
			return common.ErrConflict
		}
		return nil

	case decisionmodel.DefinitionClarificationRuntime:
		if binding.RequestContractRepairHash != "" {
			return validateRequestContractRepairResolution(ticket, resolution)
		}
		command := decisionevents.ResolveRuntimeDefinitionClarificationByReplacementCommand{
			TicketID: ticket.ID, WorkspaceID: ticket.WorkspaceID, PredecessorRunID: ticket.RunID,
			ReplacementIntentID: binding.ReplacementIntentID, ReplacementIntentHash: binding.ReplacementIntentHash,
			PredecessorDefinitionRevision: binding.DefinitionRevision, PredecessorStartupInputHash: binding.StartupInputHash,
			SuccessorDefinitionRevision: binding.SuccessorDefinitionRevision, SuccessorStartupInputHash: binding.SuccessorStartupInputHash,
		}.Normalized()
		expectedID := decisionevents.StableDefinitionClarificationResolutionID(ticket.ID, command.ReplacementIntentHash)
		expected := &decisionmodel.Resolution{ID: expectedID, TicketID: ticket.ID, Type: decisionevents.DefinitionClarificationResolutionReplacementBound,
			Payload: workorcheventpayload.New(workorcheventpayload.StringField("replacementIntentId", command.ReplacementIntentID), workorcheventpayload.StringField("replacementIntentHash", command.ReplacementIntentHash))}
		if command.Validate() != nil || binding.ResolutionBindingHash != command.BindingHash(expectedID, decisionevents.DefinitionClarificationResolved, decisionevents.RuntimeDefinitionClarificationNoConflict, "", "") ||
			!definitionClarificationResolutionMatches(resolution, expected) {
			return common.ErrConflict
		}
		return nil
	default:
		return common.ErrConflict
	}
}

func workspaceDefinitionClarificationLifecycleBindingConsistent(ticket *decisionmodel.Ticket, binding *decisionmodel.DefinitionClarificationBinding) bool {
	if ticket == nil || binding == nil {
		return false
	}
	resolutionIdentityPresent := binding.ResolutionCandidateID != "" || binding.ResolutionCandidateRevision != 0 ||
		binding.ResolutionCandidateHash != "" || binding.AdmissionResultHash != "" ||
		binding.DefinitionRevision != "" || binding.StartupInputHash != ""
	switch ticket.State {
	case common.DecisionTicketStateOpen:
		return binding.ResolutionID == "" && binding.ResolutionBindingHash == "" && !resolutionIdentityPresent
	case common.DecisionTicketStateResolved:
		if binding.ResolutionID == "" || binding.ResolutionBindingHash == "" {
			return false
		}
		if !resolutionIdentityPresent {
			// Candidate-superseded reconciliation binds only the Resolution
			// identity; its exact successor remains in the Resolution payload.
			return true
		}
		return binding.ResolutionCandidateID != "" && binding.ResolutionCandidateRevision != 0 &&
			binding.ResolutionCandidateHash != "" && binding.AdmissionResultHash != "" &&
			binding.DefinitionRevision != "" && binding.StartupInputHash != ""
	default:
		return false
	}
}

func runtimeDefinitionClarificationTicketForCommand(cmd decisionevents.OpenRuntimeDefinitionClarificationRequestCommand) *decisionmodel.Ticket {
	cmd = cmd.Normalized()
	return &decisionmodel.Ticket{
		ID: cmd.TicketID, WorkspaceID: cmd.WorkspaceID, RunID: cmd.RunID,
		StepID: common.StepID("runtime-definition/" + cmd.ClarificationDispatchID),
		Reason: string(cmd.GapKind), DecisionKind: "runtime_definition_clarification",
		Question:     "Revise the Workspace Definition and explicitly restart",
		FailureClass: "runtime_definition_gap", FailureSignature: cmd.CanonicalGapSignatureHash,
		AllowedRecoveryActions: []string{cmd.RequiredActionKind},
		DefinitionClarification: &decisionmodel.DefinitionClarificationBinding{
			Class: decisionmodel.DefinitionClarificationRuntime, SchemaVersion: cmd.SchemaVersion,
			OpenBindingHash: cmd.BindingHash(), RequiredActionKind: cmd.RequiredActionKind,
			ViolationOrGapCode: string(cmd.GapKind), CanonicalSignatureVersion: cmd.CanonicalGapSignatureVersion,
			CanonicalSignature: cmd.CanonicalGapSignature, CanonicalSignatureHash: cmd.CanonicalGapSignatureHash,
			OffendingRefs: append([]string(nil), cmd.OffendingItemRefs...), DefinitionRevision: cmd.DefinitionRevision,
			StartupInputHash: cmd.StartupInputHash, AuthoritySnapshotID: cmd.AuthoritySnapshotID,
			ClarificationDispatchID: cmd.ClarificationDispatchID, FocusRef: cmd.FocusRef,
			LogicalRequestID: cmd.LogicalRequestID, SemanticDocumentHash: cmd.SemanticDocumentHash,
		},
	}
}

func runtimeDefinitionClarificationTicketSemanticsMatch(ticket *decisionmodel.Ticket, cmd decisionevents.OpenRuntimeDefinitionClarificationRequestCommand) bool {
	if ticket == nil {
		return false
	}
	cmd = cmd.Normalized()
	expected := runtimeDefinitionClarificationTicketForCommand(cmd)
	return ticket.ID == expected.ID && ticket.WorkspaceID == expected.WorkspaceID && ticket.RunID == expected.RunID &&
		ticket.StepID == expected.StepID && ticket.Reason == expected.Reason && ticket.DecisionKind == expected.DecisionKind &&
		ticket.Question == expected.Question && ticket.Context.Equal(expected.Context) && ticket.AutoGovernance.Equal(expected.AutoGovernance) &&
		ticket.FailureClass == expected.FailureClass && ticket.FailureSignature == expected.FailureSignature &&
		slices.Equal(ticket.AllowedRecoveryActions, expected.AllowedRecoveryActions) && ticket.RuntimeGovernanceProposal == nil &&
		workspaceDefinitionClarificationTicketStateTimeMatches(ticket)
}

func runtimeDefinitionClarificationBindingMatchesCommand(binding *decisionmodel.DefinitionClarificationBinding, cmd decisionevents.OpenRuntimeDefinitionClarificationRequestCommand) bool {
	if binding == nil {
		return false
	}
	cmd = cmd.Normalized()
	return binding.Class == decisionmodel.DefinitionClarificationRuntime && binding.SchemaVersion == cmd.SchemaVersion &&
		binding.OpenBindingHash == cmd.BindingHash() && binding.RequiredActionKind == cmd.RequiredActionKind &&
		binding.ViolationOrGapCode == string(cmd.GapKind) && binding.CanonicalSignatureVersion == cmd.CanonicalGapSignatureVersion &&
		binding.CanonicalSignature == cmd.CanonicalGapSignature && binding.CanonicalSignatureHash == cmd.CanonicalGapSignatureHash &&
		slices.Equal(binding.OffendingRefs, cmd.OffendingItemRefs) && binding.DefinitionRevision == cmd.DefinitionRevision &&
		binding.StartupInputHash == cmd.StartupInputHash && binding.AuthoritySnapshotID == cmd.AuthoritySnapshotID &&
		binding.ClarificationDispatchID == cmd.ClarificationDispatchID && binding.FocusRef == cmd.FocusRef &&
		binding.LogicalRequestID == cmd.LogicalRequestID && binding.SemanticDocumentHash == cmd.SemanticDocumentHash &&
		binding.CandidateID == "" && binding.CandidateRevision == 0 && binding.CandidateHash == "" &&
		binding.ResolutionCandidateID == "" && binding.ResolutionCandidateRevision == 0 &&
		binding.ResolutionCandidateHash == "" && binding.AdmissionResultHash == ""
}

func runtimeDefinitionClarificationLifecycleBindingConsistent(ticket *decisionmodel.Ticket, binding *decisionmodel.DefinitionClarificationBinding) bool {
	if ticket == nil || binding == nil {
		return false
	}
	switch ticket.State {
	case common.DecisionTicketStateOpen:
		return binding.RequestContractRepairHash == "" && binding.ResolutionID == "" && binding.ResolutionBindingHash == "" &&
			binding.ReplacementIntentID == "" && binding.ReplacementIntentHash == "" &&
			binding.SuccessorDefinitionRevision == "" && binding.SuccessorStartupInputHash == ""
	case common.DecisionTicketStateResolved:
		if binding.RequestContractRepairHash != "" {
			return binding.ResolutionID != "" && binding.ResolutionBindingHash != "" && binding.ReplacementIntentID == "" && binding.ReplacementIntentHash == "" && binding.SuccessorDefinitionRevision == "" && binding.SuccessorStartupInputHash == ""
		}
		return binding.ResolutionID != "" && binding.ResolutionBindingHash != "" &&
			binding.ReplacementIntentID != "" && binding.ReplacementIntentHash != "" &&
			binding.SuccessorDefinitionRevision != "" && binding.SuccessorStartupInputHash != ""
	default:
		return false
	}
}

func runtimeDefinitionClarificationTicketSelfConsistent(ticket *decisionmodel.Ticket) bool {
	if ticket == nil || ticket.DefinitionClarification == nil {
		return false
	}
	binding := ticket.DefinitionClarification
	command := decisionevents.OpenRuntimeDefinitionClarificationRequestCommand{
		SchemaVersion: binding.SchemaVersion, TicketID: ticket.ID, WorkspaceID: ticket.WorkspaceID, RunID: ticket.RunID,
		DefinitionRevision: binding.DefinitionRevision, StartupInputHash: binding.StartupInputHash,
		AuthoritySnapshotID: binding.AuthoritySnapshotID, ClarificationDispatchID: binding.ClarificationDispatchID,
		FocusRef: binding.FocusRef, LogicalRequestID: binding.LogicalRequestID, SemanticDocumentHash: binding.SemanticDocumentHash,
		GapKind: decisionevents.RuntimeDefinitionGapKind(binding.ViolationOrGapCode), OffendingItemRefs: append([]string(nil), binding.OffendingRefs...),
		CanonicalGapSignatureVersion: binding.CanonicalSignatureVersion, CanonicalGapSignature: binding.CanonicalSignature,
		CanonicalGapSignatureHash: binding.CanonicalSignatureHash, RequiredActionKind: binding.RequiredActionKind,
	}.Normalized()
	return command.Validate() == nil && runtimeDefinitionClarificationTicketSemanticsMatch(ticket, command) &&
		runtimeDefinitionClarificationBindingMatchesCommand(binding, command) &&
		runtimeDefinitionClarificationLifecycleBindingConsistent(ticket, binding)
}

func (m *Biz) openRuntimeDefinitionClarification(ctx context.Context, cmd decisionevents.OpenRuntimeDefinitionClarificationRequestCommand) (decisionevents.OpenRuntimeDefinitionClarificationRequestResult, error) {
	cmd = cmd.Normalized()
	if err := cmd.Validate(); err != nil {
		return decisionevents.OpenRuntimeDefinitionClarificationRequestResult{}, fmt.Errorf("%w: %v", common.ErrInvalidArgument, err)
	}
	bound, err := common.BindRuntimeWorkspaceContext(ctx, cmd.WorkspaceID)
	if err != nil {
		return decisionevents.OpenRuntimeDefinitionClarificationRequestResult{}, err
	}
	bindingHash := cmd.BindingHash()
	if existing, err := m.tickets.Get(bound, cmd.TicketID); err == nil {
		binding := existing.DefinitionClarification
		if !runtimeDefinitionClarificationTicketSemanticsMatch(existing, cmd) ||
			!runtimeDefinitionClarificationBindingMatchesCommand(binding, cmd) ||
			!runtimeDefinitionClarificationLifecycleBindingConsistent(existing, binding) {
			return decisionevents.OpenRuntimeDefinitionClarificationRequestResult{}, common.ErrConflict
		}
		if err := m.validateResolvedDefinitionClarificationRecord(bound, existing); err != nil {
			return decisionevents.OpenRuntimeDefinitionClarificationRequestResult{}, err
		}
		if err := m.ensureTicketCreatedNotification(bound, existing); err != nil {
			return decisionevents.OpenRuntimeDefinitionClarificationRequestResult{}, err
		}
		return decisionevents.OpenRuntimeDefinitionClarificationRequestResult{TicketID: cmd.TicketID, WorkspaceID: cmd.WorkspaceID, RunID: cmd.RunID, AuthoritySnapshotID: cmd.AuthoritySnapshotID, ClarificationDispatchID: cmd.ClarificationDispatchID, Disposition: decisionevents.DefinitionClarificationDuplicate, ClarificationRequestBindingHash: bindingHash}, nil
	} else if !errors.Is(err, common.ErrNotFound) {
		return decisionevents.OpenRuntimeDefinitionClarificationRequestResult{}, err
	}
	seed := runtimeDefinitionClarificationTicketForCommand(cmd)
	ticket, err := m.service.applyLifecycleInternalEvent(bound, nil, lifecycleInternalEvent{Kind: lifecycleInternalEventOpened, TicketID: seed.ID, Ticket: seed})
	if err != nil {
		return decisionevents.OpenRuntimeDefinitionClarificationRequestResult{}, err
	}
	if err := m.ensureTicketCreatedNotification(bound, ticket); err != nil {
		return decisionevents.OpenRuntimeDefinitionClarificationRequestResult{}, err
	}
	return decisionevents.OpenRuntimeDefinitionClarificationRequestResult{TicketID: cmd.TicketID, WorkspaceID: cmd.WorkspaceID, RunID: cmd.RunID, AuthoritySnapshotID: cmd.AuthoritySnapshotID, ClarificationDispatchID: cmd.ClarificationDispatchID, Disposition: decisionevents.DefinitionClarificationCreated, ClarificationRequestBindingHash: bindingHash}, nil
}

func (m *Biz) reconcileWorkspaceDefinitionClarification(ctx context.Context, cmd decisionevents.ReconcileWorkspaceDefinitionClarificationCandidateCommand) (decisionevents.ReconcileWorkspaceDefinitionClarificationCandidateResult, error) {
	cmd = cmd.Normalized()
	if err := cmd.Validate(); err != nil {
		return decisionevents.ReconcileWorkspaceDefinitionClarificationCandidateResult{}, fmt.Errorf("%w: %v", common.ErrInvalidArgument, err)
	}
	bound, err := common.BindRuntimeWorkspaceContext(ctx, cmd.OldRequest.WorkspaceID)
	if err != nil {
		return decisionevents.ReconcileWorkspaceDefinitionClarificationCandidateResult{}, err
	}
	oldTicket, err := m.tickets.Get(bound, cmd.OldRequest.TicketID)
	if err != nil {
		return decisionevents.ReconcileWorkspaceDefinitionClarificationCandidateResult{}, err
	}
	oldBinding := oldTicket.DefinitionClarification
	if !workspaceDefinitionClarificationTicketSemanticsMatch(oldTicket, cmd.OldRequest) ||
		!workspaceDefinitionClarificationBindingMatchesCommand(oldBinding, cmd.OldRequest) ||
		!workspaceDefinitionClarificationLifecycleBindingConsistent(oldTicket, oldBinding) {
		return decisionevents.ReconcileWorkspaceDefinitionClarificationCandidateResult{}, common.ErrConflict
	}
	if err := m.validateResolvedDefinitionClarificationRecord(bound, oldTicket); err != nil {
		return decisionevents.ReconcileWorkspaceDefinitionClarificationCandidateResult{}, err
	}
	seed := cmd.NewRequest.BindingHash()
	resolutionID := decisionevents.StableDefinitionClarificationResolutionID(cmd.OldRequest.TicketID, seed)
	bindingHash := cmd.BindingHash(resolutionID)
	// Once the old ticket is durably bound, a different successor identity is
	// stale. Reject it before opening another blocking ticket; exact replay still
	// verifies or recovers the already-selected successor below.
	if oldBinding.ResolutionID != "" && (oldBinding.ResolutionID != resolutionID || oldBinding.ResolutionBindingHash != bindingHash) {
		return decisionevents.ReconcileWorkspaceDefinitionClarificationCandidateResult{}, common.ErrConflict
	}
	expected := &decisionmodel.Resolution{ID: resolutionID, TicketID: cmd.OldRequest.TicketID, ResolvedAt: newDecisionResolutionTimestamp(), Type: decisionevents.DefinitionClarificationResolutionCandidateSuperseded,
		Payload: workorcheventpayload.New(workorcheventpayload.StringField("newTicketId", string(cmd.NewRequest.TicketID)), workorcheventpayload.StringField("newCandidateHash", cmd.NewRequest.CandidateHash))}
	// Reject a conflicting pre-existing successor before selecting it durably.
	// This keeps an ID collision or forged ticket from leaving the old ticket
	// bound to a successor that exact replay can never open.
	successorFound := false
	if existing, loadErr := m.tickets.Get(bound, cmd.NewRequest.TicketID); loadErr == nil {
		successorFound = true
		if !workspaceDefinitionClarificationTicketSemanticsMatch(existing, cmd.NewRequest) ||
			!workspaceDefinitionClarificationBindingMatchesCommand(existing.DefinitionClarification, cmd.NewRequest) ||
			!workspaceDefinitionClarificationLifecycleBindingConsistent(existing, existing.DefinitionClarification) {
			return decisionevents.ReconcileWorkspaceDefinitionClarificationCandidateResult{}, common.ErrConflict
		}
		if err := m.validateResolvedDefinitionClarificationRecord(bound, existing); err != nil {
			return decisionevents.ReconcileWorkspaceDefinitionClarificationCandidateResult{}, err
		}
	} else if !errors.Is(loadErr, common.ErrNotFound) {
		return decisionevents.ReconcileWorkspaceDefinitionClarificationCandidateResult{}, loadErr
	}
	// The legal write order persists the successor before binding the old ticket.
	// A bound old ticket with no successor is corrupt owner state and must not be
	// reconstructed from a replaying caller command.
	if oldBinding.ResolutionID != "" && !successorFound {
		return decisionevents.ReconcileWorkspaceDefinitionClarificationCandidateResult{}, common.ErrConflict
	}
	// Persist the old-ticket-owned successor selection before opening the new
	// blocker. The old ticket remains open, so this cut never removes the active
	// blocker. On fresh-owner replay the resolution is the unambiguous transaction
	// preimage: exact replay completes the selected successor, while a drifted
	// successor conflicts before any new ticket write.
	var persistedResolution *decisionmodel.Resolution
	if existing, loadErr := m.resolutions.GetByTicketID(bound, oldTicket.ID); loadErr == nil {
		if !definitionClarificationResolutionMatches(existing, expected) {
			return decisionevents.ReconcileWorkspaceDefinitionClarificationCandidateResult{}, common.ErrConflict
		}
		persistedResolution = existing
	} else if !errors.Is(loadErr, common.ErrNotFound) {
		return decisionevents.ReconcileWorkspaceDefinitionClarificationCandidateResult{}, loadErr
	} else {
		// A bound old ticket without its Resolution row is corrupt owner state,
		// not a recoverable create cut. Do not let a caller-selected payload
		// reconstruct authority or create/notify a successor as a side effect.
		if oldBinding.ResolutionID != "" {
			return decisionevents.ReconcileWorkspaceDefinitionClarificationCandidateResult{}, common.ErrConflict
		}
		if createErr := m.resolutions.Create(bound, expected); createErr != nil {
			return decisionevents.ReconcileWorkspaceDefinitionClarificationCandidateResult{}, createErr
		}
		persistedResolution = expected
	}
	if strings.TrimSpace(persistedResolution.ResolvedAt) == "" {
		return decisionevents.ReconcileWorkspaceDefinitionClarificationCandidateResult{}, common.ErrConflict
	}
	// Establish the selected blocking ticket before retiring the old one. A crash
	// at either repository boundary leaves the old blocker open, and exact replay
	// converges the remaining owner-local writes from the persisted selection.
	opened, err := m.openWorkspaceDefinitionClarification(bound, cmd.NewRequest)
	if err != nil {
		return decisionevents.ReconcileWorkspaceDefinitionClarificationCandidateResult{}, err
	}
	result := decisionevents.ReconcileWorkspaceDefinitionClarificationCandidateResult{
		WorkspaceID: cmd.OldRequest.WorkspaceID, OldTicketID: cmd.OldRequest.TicketID, OldResolutionID: resolutionID,
		NewTicketID: opened.TicketID, NewTicketDisposition: opened.Disposition,
		NewRequestBindingHash: opened.ClarificationRequestBindingHash, ReconciliationBindingHash: bindingHash,
	}
	if oldBinding.ResolutionID != "" {
		resolution, matchErr := m.matchDefinitionClarificationResolution(bound, oldTicket.ID, resolutionID, expected.Type, map[string]string{"newTicketId": string(opened.TicketID), "newCandidateHash": cmd.NewRequest.CandidateHash})
		if matchErr != nil {
			return decisionevents.ReconcileWorkspaceDefinitionClarificationCandidateResult{}, matchErr
		}
		if err := m.recordAndPublishResolvedTicket(bound, oldTicket, resolution, false); err != nil {
			return decisionevents.ReconcileWorkspaceDefinitionClarificationCandidateResult{}, err
		}
		result.NewTicketDisposition = decisionevents.DefinitionClarificationDuplicate
		return result, nil
	}
	resolution := persistedResolution
	updated, err := m.service.applyLifecycleInternalEvent(bound, oldTicket, lifecycleInternalEvent{
		Kind: lifecycleInternalEventResolutionObserved, TicketID: oldTicket.ID, ResolvedAt: resolution.ResolvedAt,
		Clarification: &definitionClarificationResolutionFact{ResolutionID: resolutionID, ResolutionBindingHash: bindingHash},
	})
	if err != nil {
		return decisionevents.ReconcileWorkspaceDefinitionClarificationCandidateResult{}, err
	}
	*oldTicket = *updated
	if err := m.recordAndPublishResolvedTicket(bound, oldTicket, resolution, true); err != nil {
		return decisionevents.ReconcileWorkspaceDefinitionClarificationCandidateResult{}, err
	}
	return result, nil
}

// workspaceDefinitionClarificationBindingMatchesCommand verifies the durable
// Decision-owned opening identity field by field. OpenBindingHash remains an
// integrity fence, but is not accepted as a substitute for the persisted DTO
// fields (notably OffendingRefs) during idempotent replay/reconciliation.
func workspaceDefinitionClarificationBindingMatchesCommand(binding *decisionmodel.DefinitionClarificationBinding, cmd decisionevents.OpenWorkspaceDefinitionClarificationRequestCommand) bool {
	if binding == nil {
		return false
	}
	cmd = cmd.Normalized()
	return binding.Class == decisionmodel.DefinitionClarificationWorkspacePreSeal &&
		binding.SchemaVersion == cmd.SchemaVersion &&
		binding.OpenBindingHash == cmd.BindingHash() &&
		binding.RequiredActionKind == cmd.RequiredActionKind &&
		binding.CandidateID == cmd.CandidateID &&
		binding.CandidateRevision == cmd.CandidateRevision &&
		binding.CandidateHash == cmd.CandidateHash &&
		binding.ViolationOrGapCode == cmd.ViolationCode &&
		binding.CanonicalSignatureVersion == cmd.CanonicalSignatureVersion &&
		binding.CanonicalSignature == cmd.CanonicalSignature &&
		binding.CanonicalSignatureHash == cmd.CanonicalSignatureHash &&
		slices.Equal(binding.OffendingRefs, cmd.OffendingRefs) &&
		binding.AuthoritySnapshotID == "" &&
		binding.ClarificationDispatchID == "" &&
		binding.FocusRef == "" &&
		binding.LogicalRequestID == "" &&
		binding.SemanticDocumentHash == "" &&
		binding.ReplacementIntentID == "" &&
		binding.ReplacementIntentHash == "" &&
		binding.SuccessorDefinitionRevision == "" &&
		binding.SuccessorStartupInputHash == ""
}

func workspaceDefinitionClarificationBindingForCommand(cmd decisionevents.OpenWorkspaceDefinitionClarificationRequestCommand) *decisionmodel.DefinitionClarificationBinding {
	cmd = cmd.Normalized()
	return &decisionmodel.DefinitionClarificationBinding{
		Class:                     decisionmodel.DefinitionClarificationWorkspacePreSeal,
		SchemaVersion:             cmd.SchemaVersion,
		OpenBindingHash:           cmd.BindingHash(),
		RequiredActionKind:        cmd.RequiredActionKind,
		CandidateID:               cmd.CandidateID,
		CandidateRevision:         cmd.CandidateRevision,
		CandidateHash:             cmd.CandidateHash,
		ViolationOrGapCode:        cmd.ViolationCode,
		CanonicalSignatureVersion: cmd.CanonicalSignatureVersion,
		CanonicalSignature:        cmd.CanonicalSignature,
		CanonicalSignatureHash:    cmd.CanonicalSignatureHash,
		OffendingRefs:             append([]string(nil), cmd.OffendingRefs...),
	}
}

func (m *Biz) resolveWorkspaceDefinitionClarification(ctx context.Context, cmd decisionevents.ResolveWorkspaceDefinitionClarificationByAdmissionCommand) (decisionevents.ResolveWorkspaceDefinitionClarificationByAdmissionResult, error) {
	cmd = cmd.Normalized()
	if err := cmd.Validate(); err != nil {
		return decisionevents.ResolveWorkspaceDefinitionClarificationByAdmissionResult{}, fmt.Errorf("%w: %v", common.ErrInvalidArgument, err)
	}
	bound, err := common.BindRuntimeWorkspaceContext(ctx, cmd.WorkspaceID)
	if err != nil {
		return decisionevents.ResolveWorkspaceDefinitionClarificationByAdmissionResult{}, err
	}
	ticket, err := m.tickets.Get(bound, cmd.TicketID)
	if err != nil {
		return decisionevents.ResolveWorkspaceDefinitionClarificationByAdmissionResult{}, err
	}
	binding := ticket.DefinitionClarification
	if !workspaceDefinitionClarificationTicketSelfConsistent(ticket) ||
		binding.CandidateID != cmd.OldCandidateID || binding.CandidateRevision != cmd.OldCandidateRevision || binding.CandidateHash != cmd.OldCandidateHash {
		return decisionevents.ResolveWorkspaceDefinitionClarificationByAdmissionResult{}, common.ErrConflict
	}
	if err := m.validateResolvedDefinitionClarificationRecord(bound, ticket); err != nil {
		return decisionevents.ResolveWorkspaceDefinitionClarificationByAdmissionResult{}, err
	}
	seedHash := cmd.AdmissionResultHash + "\x00" + cmd.StartupInputHash
	resolutionID := decisionevents.StableDefinitionClarificationResolutionID(cmd.TicketID, seedHash)
	resolutionHash := cmd.BindingHash(resolutionID)
	if binding.ResolutionID != "" {
		if binding.ResolutionID != resolutionID || binding.ResolutionBindingHash != resolutionHash {
			return decisionevents.ResolveWorkspaceDefinitionClarificationByAdmissionResult{}, common.ErrConflict
		}
		resolution, err := m.matchDefinitionClarificationResolution(bound, cmd.TicketID, resolutionID, decisionevents.DefinitionClarificationResolutionCandidateAdjusted, map[string]string{"definitionRevision": cmd.DefinitionRevision, "startupInputHash": cmd.StartupInputHash})
		if err != nil {
			return decisionevents.ResolveWorkspaceDefinitionClarificationByAdmissionResult{}, err
		}
		if err := m.recordAndPublishResolvedTicket(bound, ticket, resolution, false); err != nil {
			return decisionevents.ResolveWorkspaceDefinitionClarificationByAdmissionResult{}, err
		}
		return decisionevents.ResolveWorkspaceDefinitionClarificationByAdmissionResult{TicketID: cmd.TicketID, WorkspaceID: cmd.WorkspaceID, ResolutionID: resolutionID, ResolutionType: decisionevents.DefinitionClarificationResolutionCandidateAdjusted, Disposition: decisionevents.DefinitionClarificationDuplicate, ResolutionBindingHash: resolutionHash}, nil
	}
	resolution := &decisionmodel.Resolution{ID: resolutionID, TicketID: cmd.TicketID, ResolvedAt: newDecisionResolutionTimestamp(), Type: decisionevents.DefinitionClarificationResolutionCandidateAdjusted, Payload: workorcheventpayload.New(workorcheventpayload.StringField("definitionRevision", cmd.DefinitionRevision), workorcheventpayload.StringField("startupInputHash", cmd.StartupInputHash))}
	if existing, err := m.resolutions.GetByTicketID(bound, cmd.TicketID); err == nil {
		if !definitionClarificationResolutionMatches(existing, resolution) {
			return decisionevents.ResolveWorkspaceDefinitionClarificationByAdmissionResult{}, common.ErrConflict
		}
		resolution = existing
	} else if !errors.Is(err, common.ErrNotFound) {
		return decisionevents.ResolveWorkspaceDefinitionClarificationByAdmissionResult{}, err
	} else if err := m.resolutions.Create(bound, resolution); err != nil {
		return decisionevents.ResolveWorkspaceDefinitionClarificationByAdmissionResult{}, err
	}
	if strings.TrimSpace(resolution.ResolvedAt) == "" {
		return decisionevents.ResolveWorkspaceDefinitionClarificationByAdmissionResult{}, common.ErrConflict
	}
	updated, err := m.service.applyLifecycleInternalEvent(bound, ticket, lifecycleInternalEvent{
		Kind: lifecycleInternalEventResolutionObserved, TicketID: ticket.ID, ResolvedAt: resolution.ResolvedAt,
		Clarification: &definitionClarificationResolutionFact{
			ResolutionID: resolutionID, ResolutionBindingHash: resolutionHash,
			ResolutionCandidateID: cmd.NewCandidateID, ResolutionCandidateRevision: cmd.NewCandidateRevision,
			ResolutionCandidateHash: cmd.NewCandidateHash, AdmissionResultHash: cmd.AdmissionResultHash,
			DefinitionRevision: cmd.DefinitionRevision, StartupInputHash: cmd.StartupInputHash,
		},
	})
	if err != nil {
		return decisionevents.ResolveWorkspaceDefinitionClarificationByAdmissionResult{}, err
	}
	*ticket = *updated
	if err := m.recordAndPublishResolvedTicket(bound, ticket, resolution, true); err != nil {
		return decisionevents.ResolveWorkspaceDefinitionClarificationByAdmissionResult{}, err
	}
	return decisionevents.ResolveWorkspaceDefinitionClarificationByAdmissionResult{TicketID: cmd.TicketID, WorkspaceID: cmd.WorkspaceID, ResolutionID: resolutionID, ResolutionType: decisionevents.DefinitionClarificationResolutionCandidateAdjusted, Disposition: decisionevents.DefinitionClarificationResolved, ResolutionBindingHash: resolutionHash}, nil
}

func (m *Biz) resolveRuntimeDefinitionClarification(ctx context.Context, cmd decisionevents.ResolveRuntimeDefinitionClarificationByReplacementCommand) (decisionevents.ResolveRuntimeDefinitionClarificationByReplacementResult, error) {
	cmd = cmd.Normalized()
	if err := cmd.Validate(); err != nil {
		return decisionevents.ResolveRuntimeDefinitionClarificationByReplacementResult{}, fmt.Errorf("%w: %v", common.ErrInvalidArgument, err)
	}
	bound, err := common.BindRuntimeWorkspaceContext(ctx, cmd.WorkspaceID)
	if err != nil {
		return decisionevents.ResolveRuntimeDefinitionClarificationByReplacementResult{}, err
	}
	ticket, err := m.tickets.Get(bound, cmd.TicketID)
	if err != nil {
		return decisionevents.ResolveRuntimeDefinitionClarificationByReplacementResult{}, err
	}
	binding := ticket.DefinitionClarification
	if !runtimeDefinitionClarificationTicketSelfConsistent(ticket) {
		return decisionevents.ResolveRuntimeDefinitionClarificationByReplacementResult{}, common.ErrConflict
	}
	if err := m.validateResolvedDefinitionClarificationRecord(bound, ticket); err != nil {
		return decisionevents.ResolveRuntimeDefinitionClarificationByReplacementResult{}, err
	}
	if ticket.WorkspaceID != cmd.WorkspaceID || ticket.RunID != cmd.PredecessorRunID || binding.DefinitionRevision != cmd.PredecessorDefinitionRevision || binding.StartupInputHash != cmd.PredecessorStartupInputHash {
		return runtimeClarificationConflict(cmd, decisionevents.RuntimeDefinitionClarificationScopeMismatch, "", ""), nil
	}
	if binding.ReplacementIntentID != "" {
		if binding.ReplacementIntentID == cmd.ReplacementIntentID && binding.ReplacementIntentHash == cmd.ReplacementIntentHash {
			result := runtimeClarificationResolvedResult(cmd, binding.ResolutionID, decisionevents.DefinitionClarificationDuplicate, decisionevents.RuntimeDefinitionClarificationNoConflict, "", "")
			if result.ResolutionBindingHash != binding.ResolutionBindingHash {
				return decisionevents.ResolveRuntimeDefinitionClarificationByReplacementResult{}, common.ErrConflict
			}
			resolution, err := m.matchDefinitionClarificationResolution(bound, cmd.TicketID, binding.ResolutionID, decisionevents.DefinitionClarificationResolutionReplacementBound, map[string]string{"replacementIntentId": cmd.ReplacementIntentID, "replacementIntentHash": cmd.ReplacementIntentHash})
			if err != nil {
				return decisionevents.ResolveRuntimeDefinitionClarificationByReplacementResult{}, err
			}
			if err := m.recordAndPublishResolvedTicket(bound, ticket, resolution, false); err != nil {
				return decisionevents.ResolveRuntimeDefinitionClarificationByReplacementResult{}, err
			}
			return result, nil
		}
		return runtimeClarificationConflict(cmd, decisionevents.RuntimeDefinitionClarificationTicketAlreadyBound, binding.ReplacementIntentID, binding.ReplacementIntentHash), nil
	}
	resolutionID := decisionevents.StableDefinitionClarificationResolutionID(cmd.TicketID, cmd.ReplacementIntentHash)
	result := runtimeClarificationResolvedResult(cmd, resolutionID, decisionevents.DefinitionClarificationResolved, decisionevents.RuntimeDefinitionClarificationNoConflict, "", "")
	resolution := &decisionmodel.Resolution{ID: resolutionID, TicketID: cmd.TicketID, ResolvedAt: newDecisionResolutionTimestamp(), Type: decisionevents.DefinitionClarificationResolutionReplacementBound, Payload: workorcheventpayload.New(workorcheventpayload.StringField("replacementIntentId", cmd.ReplacementIntentID), workorcheventpayload.StringField("replacementIntentHash", cmd.ReplacementIntentHash))}
	if existing, err := m.resolutions.GetByTicketID(bound, cmd.TicketID); err == nil {
		if !definitionClarificationResolutionMatches(existing, resolution) {
			return decisionevents.ResolveRuntimeDefinitionClarificationByReplacementResult{}, common.ErrConflict
		}
		resolution = existing
	} else if !errors.Is(err, common.ErrNotFound) {
		return decisionevents.ResolveRuntimeDefinitionClarificationByReplacementResult{}, err
	} else if err := m.resolutions.Create(bound, resolution); err != nil {
		return decisionevents.ResolveRuntimeDefinitionClarificationByReplacementResult{}, err
	}
	if strings.TrimSpace(resolution.ResolvedAt) == "" {
		return decisionevents.ResolveRuntimeDefinitionClarificationByReplacementResult{}, common.ErrConflict
	}
	updated, err := m.service.applyLifecycleInternalEvent(bound, ticket, lifecycleInternalEvent{
		Kind: lifecycleInternalEventResolutionObserved, TicketID: ticket.ID, ResolvedAt: resolution.ResolvedAt,
		Clarification: &definitionClarificationResolutionFact{
			ResolutionID: resolutionID, ResolutionBindingHash: result.ResolutionBindingHash,
			ReplacementIntentID: cmd.ReplacementIntentID, ReplacementIntentHash: cmd.ReplacementIntentHash,
			SuccessorDefinitionRevision: cmd.SuccessorDefinitionRevision, SuccessorStartupInputHash: cmd.SuccessorStartupInputHash,
		},
	})
	if err != nil {
		return decisionevents.ResolveRuntimeDefinitionClarificationByReplacementResult{}, err
	}
	*ticket = *updated
	if err := m.recordAndPublishResolvedTicket(bound, ticket, resolution, true); err != nil {
		return decisionevents.ResolveRuntimeDefinitionClarificationByReplacementResult{}, err
	}
	return result, nil
}

func runtimeClarificationResolvedResult(cmd decisionevents.ResolveRuntimeDefinitionClarificationByReplacementCommand, resolutionID common.ResolutionID, disposition decisionevents.DefinitionClarificationDisposition, conflict decisionevents.RuntimeDefinitionClarificationConflictKind, currentID, currentHash string) decisionevents.ResolveRuntimeDefinitionClarificationByReplacementResult {
	bindingDisposition := disposition
	if conflict == decisionevents.RuntimeDefinitionClarificationNoConflict {
		bindingDisposition = decisionevents.DefinitionClarificationResolved
	}
	return decisionevents.ResolveRuntimeDefinitionClarificationByReplacementResult{TicketID: cmd.TicketID, WorkspaceID: cmd.WorkspaceID, PredecessorRunID: cmd.PredecessorRunID, ReplacementIntentID: cmd.ReplacementIntentID, ResolutionID: resolutionID, ResolutionType: decisionevents.DefinitionClarificationResolutionReplacementBound, Disposition: disposition, ConflictKind: conflict, CurrentReplacementIntentID: strings.TrimSpace(currentID), CurrentReplacementIntentHash: strings.TrimSpace(currentHash), ResolutionBindingHash: cmd.BindingHash(resolutionID, bindingDisposition, conflict, currentID, currentHash)}
}

func runtimeClarificationConflict(cmd decisionevents.ResolveRuntimeDefinitionClarificationByReplacementCommand, conflict decisionevents.RuntimeDefinitionClarificationConflictKind, currentID, currentHash string) decisionevents.ResolveRuntimeDefinitionClarificationByReplacementResult {
	return runtimeClarificationResolvedResult(cmd, "", decisionevents.DefinitionClarificationStale, conflict, currentID, currentHash)
}

func (m *Biz) matchDefinitionClarificationResolution(ctx context.Context, ticketID common.TicketID, resolutionID common.ResolutionID, resolutionType string, fields map[string]string) (*decisionmodel.Resolution, error) {
	resolution, err := m.resolutions.GetByTicketID(ctx, ticketID)
	if err != nil {
		return nil, err
	}
	expected := &decisionmodel.Resolution{ID: resolutionID, TicketID: ticketID, Type: resolutionType}
	payloadFields := make([]workorcheventpayload.Field, 0, len(fields))
	for name, value := range fields {
		payloadFields = append(payloadFields, workorcheventpayload.StringField(name, value))
	}
	expected.Payload = workorcheventpayload.New(payloadFields...)
	if !definitionClarificationResolutionMatches(resolution, expected) || strings.TrimSpace(resolution.ResolvedAt) == "" {
		return nil, common.ErrConflict
	}
	return resolution, nil
}

func definitionClarificationResolutionMatches(actual, expected *decisionmodel.Resolution) bool {
	if actual == nil || expected == nil || strings.TrimSpace(actual.ResolvedAt) == "" || actual.ID != expected.ID || actual.TicketID != expected.TicketID || actual.Type != expected.Type || actual.ContinuationAction != "" || actual.ResumeRunID != "" || actual.SuccessorRunID != "" {
		return false
	}
	if len(actual.Payload.Fields) != len(expected.Payload.Fields) {
		return false
	}
	for _, field := range expected.Payload.Fields {
		value, ok := expected.Payload.String(field.Name)
		actualValue, actualOK := actual.Payload.String(field.Name)
		if !ok || !actualOK || actualValue != value {
			return false
		}
	}
	return true
}
