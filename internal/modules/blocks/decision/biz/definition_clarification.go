package decision

import (
	"context"
	"errors"
	"fmt"
	"strings"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

const (
	workspaceDefinitionClarificationSource = "application/workorchd"
	runtimeDefinitionClarificationSource   = "kernel/orchestrator"
	runtimeDefinitionReplacementSource     = "blocks/run"
)

func definitionClarificationTopics() []string {
	return []string{
		decisionevents.TopicOpenWorkspaceDefinitionClarification,
		decisionevents.TopicReconcileWorkspaceDefinitionClarificationCandidate,
		decisionevents.TopicResolveWorkspaceDefinitionClarificationByAdmission,
		decisionevents.TopicOpenRuntimeDefinitionClarification,
		decisionevents.TopicResolveRuntimeDefinitionClarificationByReplacement,
	}
}

func (m *Biz) handleReconcileWorkspaceDefinitionClarification(ev magicevent.Event, result magicevent.Result) {
	cmd, ok := ev.Data().(decisionevents.ReconcileWorkspaceDefinitionClarificationCandidateCommand)
	if !ok || ev.Source() != workspaceDefinitionClarificationSource || m == nil || m.tickets == nil || m.resolutions == nil {
		setDefinitionClarificationResult(result, decisionevents.ReconcileWorkspaceDefinitionClarificationCandidateResult{}, common.ErrInvalidArgument)
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
	response, err := m.openWorkspaceDefinitionClarification(ev.Context(), cmd)
	setDefinitionClarificationResult(result, response, err)
}

func (m *Biz) handleResolveWorkspaceDefinitionClarification(ev magicevent.Event, result magicevent.Result) {
	cmd, ok := ev.Data().(decisionevents.ResolveWorkspaceDefinitionClarificationByAdmissionCommand)
	if !ok || ev.Source() != workspaceDefinitionClarificationSource || m == nil || m.tickets == nil || m.resolutions == nil {
		setDefinitionClarificationResult(result, decisionevents.ResolveWorkspaceDefinitionClarificationByAdmissionResult{}, common.ErrInvalidArgument)
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
	response, err := m.openRuntimeDefinitionClarification(ev.Context(), cmd)
	setDefinitionClarificationResult(result, response, err)
}

func (m *Biz) handleResolveRuntimeDefinitionClarification(ev magicevent.Event, result magicevent.Result) {
	cmd, ok := ev.Data().(decisionevents.ResolveRuntimeDefinitionClarificationByReplacementCommand)
	if !ok || ev.Source() != runtimeDefinitionReplacementSource || m == nil || m.tickets == nil || m.resolutions == nil {
		setDefinitionClarificationResult(result, decisionevents.ResolveRuntimeDefinitionClarificationByReplacementResult{}, common.ErrInvalidArgument)
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
		if existing.WorkspaceID != cmd.WorkspaceID || existing.RunID != "" || binding == nil || binding.Class != decisionmodel.DefinitionClarificationWorkspacePreSeal || binding.OpenBindingHash != bindingHash {
			return decisionevents.OpenWorkspaceDefinitionClarificationRequestResult{}, common.ErrConflict
		}
		return decisionevents.OpenWorkspaceDefinitionClarificationRequestResult{TicketID: cmd.TicketID, WorkspaceID: cmd.WorkspaceID, CandidateID: cmd.CandidateID, CandidateRevision: cmd.CandidateRevision, Disposition: decisionevents.DefinitionClarificationDuplicate, ClarificationRequestBindingHash: bindingHash}, nil
	} else if !errors.Is(err, common.ErrNotFound) {
		return decisionevents.OpenWorkspaceDefinitionClarificationRequestResult{}, err
	}
	seed := &decisionmodel.Ticket{ID: cmd.TicketID, WorkspaceID: cmd.WorkspaceID, Reason: cmd.ViolationCode, DecisionKind: "workspace_definition_clarification", Question: workspaceDefinitionClarificationQuestion(cmd.RequiredActionKind), FailureClass: "definition_admission", FailureSignature: cmd.CanonicalSignatureHash, AllowedRecoveryActions: []string{cmd.RequiredActionKind},
		DefinitionClarification: &decisionmodel.DefinitionClarificationBinding{Class: decisionmodel.DefinitionClarificationWorkspacePreSeal, OpenBindingHash: bindingHash, RequiredActionKind: cmd.RequiredActionKind, CandidateID: cmd.CandidateID, CandidateRevision: cmd.CandidateRevision, CandidateHash: cmd.CandidateHash, ViolationOrGapCode: cmd.ViolationCode, CanonicalSignatureVersion: cmd.CanonicalSignatureVersion, CanonicalSignature: cmd.CanonicalSignature, CanonicalSignatureHash: cmd.CanonicalSignatureHash, OffendingRefs: append([]string(nil), cmd.OffendingRefs...)}}
	ticket, err := m.service.applyLifecycleInternalEvent(bound, nil, lifecycleInternalEvent{Kind: lifecycleInternalEventOpened, TicketID: seed.ID, Ticket: seed})
	if err != nil {
		return decisionevents.OpenWorkspaceDefinitionClarificationRequestResult{}, err
	}
	if err := m.ensureTicketCreatedNotification(bound, ticket); err != nil {
		return decisionevents.OpenWorkspaceDefinitionClarificationRequestResult{}, err
	}
	return decisionevents.OpenWorkspaceDefinitionClarificationRequestResult{TicketID: cmd.TicketID, WorkspaceID: cmd.WorkspaceID, CandidateID: cmd.CandidateID, CandidateRevision: cmd.CandidateRevision, Disposition: decisionevents.DefinitionClarificationCreated, ClarificationRequestBindingHash: bindingHash}, nil
}

func workspaceDefinitionClarificationQuestion(requiredAction string) string {
	if requiredAction == decisionevents.DefinitionClarificationActionFixTemplateProfile {
		return "Select or fix the Workspace Template/Profile proof contract"
	}
	return "Revise the Definition into explicit all-of semantics"
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
		if existing.WorkspaceID != cmd.WorkspaceID || existing.RunID != cmd.RunID || binding == nil || binding.Class != decisionmodel.DefinitionClarificationRuntime || binding.OpenBindingHash != bindingHash {
			return decisionevents.OpenRuntimeDefinitionClarificationRequestResult{}, common.ErrConflict
		}
		return decisionevents.OpenRuntimeDefinitionClarificationRequestResult{TicketID: cmd.TicketID, WorkspaceID: cmd.WorkspaceID, RunID: cmd.RunID, AuthoritySnapshotID: cmd.AuthoritySnapshotID, ClarificationDispatchID: cmd.ClarificationDispatchID, Disposition: decisionevents.DefinitionClarificationDuplicate, ClarificationRequestBindingHash: bindingHash}, nil
	} else if !errors.Is(err, common.ErrNotFound) {
		return decisionevents.OpenRuntimeDefinitionClarificationRequestResult{}, err
	}
	seed := &decisionmodel.Ticket{ID: cmd.TicketID, WorkspaceID: cmd.WorkspaceID, RunID: cmd.RunID, StepID: common.StepID("runtime-definition/" + cmd.ClarificationDispatchID), Reason: string(cmd.GapKind), DecisionKind: "runtime_definition_clarification", Question: "Revise the Workspace Definition and explicitly restart", FailureClass: "runtime_definition_gap", FailureSignature: cmd.CanonicalGapSignatureHash, AllowedRecoveryActions: []string{cmd.RequiredActionKind},
		DefinitionClarification: &decisionmodel.DefinitionClarificationBinding{Class: decisionmodel.DefinitionClarificationRuntime, OpenBindingHash: bindingHash, RequiredActionKind: cmd.RequiredActionKind, ViolationOrGapCode: string(cmd.GapKind), CanonicalSignatureVersion: cmd.CanonicalGapSignatureVersion, CanonicalSignature: cmd.CanonicalGapSignature, CanonicalSignatureHash: cmd.CanonicalGapSignatureHash, OffendingRefs: append([]string(nil), cmd.OffendingItemRefs...), DefinitionRevision: cmd.DefinitionRevision, StartupInputHash: cmd.StartupInputHash, AuthoritySnapshotID: cmd.AuthoritySnapshotID, ClarificationDispatchID: cmd.ClarificationDispatchID, FocusRef: cmd.FocusRef, LogicalRequestID: cmd.LogicalRequestID, SemanticDocumentHash: cmd.SemanticDocumentHash}}
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
	if oldTicket.WorkspaceID != cmd.OldRequest.WorkspaceID || oldTicket.RunID != "" || oldBinding == nil || oldBinding.Class != decisionmodel.DefinitionClarificationWorkspacePreSeal || oldBinding.OpenBindingHash != cmd.OldRequest.BindingHash() {
		return decisionevents.ReconcileWorkspaceDefinitionClarificationCandidateResult{}, common.ErrConflict
	}
	// Establish the new blocking ticket before retiring the old one. A crash at
	// either repository boundary leaves at least one invalid-candidate blocker,
	// and exact replay converges the remaining owner-local writes.
	opened, err := m.openWorkspaceDefinitionClarification(bound, cmd.NewRequest)
	if err != nil {
		return decisionevents.ReconcileWorkspaceDefinitionClarificationCandidateResult{}, err
	}
	seed := cmd.NewRequest.BindingHash()
	resolutionID := decisionevents.StableDefinitionClarificationResolutionID(cmd.OldRequest.TicketID, seed)
	bindingHash := cmd.BindingHash(resolutionID)
	result := decisionevents.ReconcileWorkspaceDefinitionClarificationCandidateResult{
		WorkspaceID: cmd.OldRequest.WorkspaceID, OldTicketID: cmd.OldRequest.TicketID, OldResolutionID: resolutionID,
		NewTicketID: opened.TicketID, NewTicketDisposition: opened.Disposition,
		NewRequestBindingHash: opened.ClarificationRequestBindingHash, ReconciliationBindingHash: bindingHash,
	}
	expected := &decisionmodel.Resolution{ID: resolutionID, TicketID: cmd.OldRequest.TicketID, Type: decisionevents.DefinitionClarificationResolutionCandidateSuperseded,
		Payload: workorcheventpayload.New(workorcheventpayload.StringField("newTicketId", string(opened.TicketID)), workorcheventpayload.StringField("newCandidateHash", cmd.NewRequest.CandidateHash))}
	if oldBinding.ResolutionID != "" {
		if oldBinding.ResolutionID != resolutionID || oldBinding.ResolutionBindingHash != bindingHash {
			return decisionevents.ReconcileWorkspaceDefinitionClarificationCandidateResult{}, common.ErrConflict
		}
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
	resolution := expected
	if existing, loadErr := m.resolutions.GetByTicketID(bound, oldTicket.ID); loadErr == nil {
		if !definitionClarificationResolutionMatches(existing, expected) {
			return decisionevents.ReconcileWorkspaceDefinitionClarificationCandidateResult{}, common.ErrConflict
		}
		resolution = existing
	} else if !errors.Is(loadErr, common.ErrNotFound) {
		return decisionevents.ReconcileWorkspaceDefinitionClarificationCandidateResult{}, loadErr
	} else if err := m.resolutions.Create(bound, expected); err != nil {
		return decisionevents.ReconcileWorkspaceDefinitionClarificationCandidateResult{}, err
	}
	updated, err := m.service.applyLifecycleInternalEvent(bound, oldTicket, lifecycleInternalEvent{
		Kind: lifecycleInternalEventResolutionObserved, TicketID: oldTicket.ID,
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
	if ticket.WorkspaceID != cmd.WorkspaceID || ticket.RunID != "" || binding == nil || binding.Class != decisionmodel.DefinitionClarificationWorkspacePreSeal || binding.CandidateID != cmd.OldCandidateID || binding.CandidateRevision != cmd.OldCandidateRevision || binding.CandidateHash != cmd.OldCandidateHash {
		return decisionevents.ResolveWorkspaceDefinitionClarificationByAdmissionResult{}, common.ErrConflict
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
	resolution := &decisionmodel.Resolution{ID: resolutionID, TicketID: cmd.TicketID, Type: decisionevents.DefinitionClarificationResolutionCandidateAdjusted, Payload: workorcheventpayload.New(workorcheventpayload.StringField("definitionRevision", cmd.DefinitionRevision), workorcheventpayload.StringField("startupInputHash", cmd.StartupInputHash))}
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
	updated, err := m.service.applyLifecycleInternalEvent(bound, ticket, lifecycleInternalEvent{
		Kind: lifecycleInternalEventResolutionObserved, TicketID: ticket.ID,
		Clarification: &definitionClarificationResolutionFact{
			ResolutionID: resolutionID, ResolutionBindingHash: resolutionHash,
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
	if ticket.WorkspaceID != cmd.WorkspaceID || ticket.RunID != cmd.PredecessorRunID || binding == nil || binding.Class != decisionmodel.DefinitionClarificationRuntime || binding.DefinitionRevision != cmd.PredecessorDefinitionRevision || binding.StartupInputHash != cmd.PredecessorStartupInputHash {
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
	resolution := &decisionmodel.Resolution{ID: resolutionID, TicketID: cmd.TicketID, Type: decisionevents.DefinitionClarificationResolutionReplacementBound, Payload: workorcheventpayload.New(workorcheventpayload.StringField("replacementIntentId", cmd.ReplacementIntentID), workorcheventpayload.StringField("replacementIntentHash", cmd.ReplacementIntentHash))}
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
	updated, err := m.service.applyLifecycleInternalEvent(bound, ticket, lifecycleInternalEvent{
		Kind: lifecycleInternalEventResolutionObserved, TicketID: ticket.ID,
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
	if !definitionClarificationResolutionMatches(resolution, expected) {
		return nil, common.ErrConflict
	}
	return resolution, nil
}

func definitionClarificationResolutionMatches(actual, expected *decisionmodel.Resolution) bool {
	if actual == nil || expected == nil || actual.ID != expected.ID || actual.TicketID != expected.TicketID || actual.Type != expected.Type || actual.ContinuationAction != "" || actual.ResumeRunID != "" || actual.SuccessorRunID != "" {
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
