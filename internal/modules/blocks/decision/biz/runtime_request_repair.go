package decision

import (
	"context"
	"errors"
	magicevent "github.com/muidea/magicCommon/event"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
	"strings"
)

func (m *Biz) handleRepairRuntimeDefinitionClarificationRequestContract(ev magicevent.Event, result magicevent.Result) {
	cmd, ok := ev.Data().(decisionevents.RepairRuntimeDefinitionClarificationRequestContractCommand)
	if !ok || ev.Source() != runtimeDefinitionClarificationSource || m == nil || m.tickets == nil || m.resolutions == nil {
		setDefinitionClarificationResult(result, decisionevents.RepairRuntimeDefinitionClarificationRequestContractResult{}, common.ErrInvalidArgument)
		return
	}
	if err := cmd.Validate(); err != nil {
		setDefinitionClarificationResult(result, decisionevents.RepairRuntimeDefinitionClarificationRequestContractResult{}, err)
		return
	}
	unlock := m.lockWorkspaceMutation(cmd.WorkspaceID)
	defer unlock()
	if err := m.rejectFencedWorkspace(ev.Context(), cmd.WorkspaceID); err != nil {
		setDefinitionClarificationResult(result, decisionevents.RepairRuntimeDefinitionClarificationRequestContractResult{}, err)
		return
	}
	response, err := m.repairRuntimeDefinitionClarificationRequestContract(ev.Context(), cmd)
	setDefinitionClarificationResult(result, response, err)
}

func requestContractRepairResolutionRecord(ticket *decisionmodel.Ticket, proofHash string) *decisionmodel.Resolution {
	id := decisionevents.StableDefinitionClarificationResolutionID(ticket.ID, proofHash)
	return &decisionmodel.Resolution{ID: id, TicketID: ticket.ID, Type: decisionevents.DefinitionClarificationResolutionRequestContractRepaired, Payload: workorcheventpayload.New(workorcheventpayload.StringField("requestContractRepairHash", proofHash))}
}
func validateRequestContractRepairResolution(ticket *decisionmodel.Ticket, resolution *decisionmodel.Resolution) error {
	binding := ticket.DefinitionClarification
	expected := requestContractRepairResolutionRecord(ticket, binding.RequestContractRepairHash)
	if binding.RequestContractRepairHash == "" || binding.ReplacementIntentID != "" || binding.ReplacementIntentHash != "" || binding.SuccessorDefinitionRevision != "" || binding.SuccessorStartupInputHash != "" || binding.ResolutionID != expected.ID || binding.ResolutionBindingHash != runtimecontract.StableRuntimeSemanticRequestRepairResolutionHash(binding.RequestContractRepairHash, ticket.ID, expected.ID) || !definitionClarificationResolutionMatches(resolution, expected) {
		return common.ErrConflict
	}
	return nil
}
func (m *Biz) repairRuntimeDefinitionClarificationRequestContract(ctx context.Context, cmd decisionevents.RepairRuntimeDefinitionClarificationRequestContractCommand) (decisionevents.RepairRuntimeDefinitionClarificationRequestContractResult, error) {
	var empty decisionevents.RepairRuntimeDefinitionClarificationRequestContractResult
	if err := cmd.Validate(); err != nil {
		return empty, err
	}
	bound, err := common.BindRuntimeWorkspaceContext(ctx, cmd.WorkspaceID)
	if err != nil {
		return empty, err
	}
	ticket, err := m.tickets.Get(bound, cmd.Proof.Request.TicketID)
	if err != nil {
		return empty, err
	}
	original := cmd.OriginalRequest()
	if ticket.CreatedNotificationPublishedAt == "" || !runtimeDefinitionClarificationTicketSelfConsistent(ticket) || !runtimeDefinitionClarificationBindingMatchesCommand(ticket.DefinitionClarification, original) || !runtimeDefinitionClarificationTicketSemanticsMatch(ticket, original) {
		return empty, common.ErrConflict
	}
	if err := m.validateResolvedDefinitionClarificationRecord(bound, ticket); err != nil {
		return empty, err
	}
	resolution := requestContractRepairResolutionRecord(ticket, cmd.Proof.Hash)
	response := decisionevents.RepairRuntimeDefinitionClarificationRequestContractResult{Disposition: decisionevents.DefinitionClarificationResolved, Resolution: runtimecontract.RuntimeSemanticRequestRepairResolution{ProofHash: cmd.Proof.Hash, TicketID: ticket.ID, ResolutionID: resolution.ID, BindingHash: runtimecontract.StableRuntimeSemanticRequestRepairResolutionHash(cmd.Proof.Hash, ticket.ID, resolution.ID)}}
	if ticket.DefinitionClarification.ResolutionID != "" {
		if ticket.DefinitionClarification.RequestContractRepairHash != cmd.Proof.Hash || ticket.DefinitionClarification.ResolutionID != resolution.ID {
			return empty, common.ErrConflict
		}
		stored, err := m.resolutions.GetByTicketID(bound, ticket.ID)
		if err != nil {
			return empty, err
		}
		if err := m.recordAndPublishResolvedTicket(bound, ticket, stored, false); err != nil {
			return empty, err
		}
		response.Disposition = decisionevents.DefinitionClarificationDuplicate
		return response, nil
	}
	resolution.ResolvedAt = newDecisionResolutionTimestamp()
	if existing, err := m.resolutions.GetByTicketID(bound, ticket.ID); err == nil {
		if !definitionClarificationResolutionMatches(existing, resolution) {
			return empty, common.ErrConflict
		}
		resolution = existing
	} else if !errors.Is(err, common.ErrNotFound) {
		return empty, err
	} else if err := m.resolutions.Create(bound, resolution); err != nil {
		return empty, err
	}
	if strings.TrimSpace(resolution.ResolvedAt) == "" {
		return empty, common.ErrConflict
	}
	updated, err := m.service.applyLifecycleInternalEvent(bound, ticket, lifecycleInternalEvent{Kind: lifecycleInternalEventResolutionObserved, TicketID: ticket.ID, ResolvedAt: resolution.ResolvedAt, Clarification: &definitionClarificationResolutionFact{ResolutionID: resolution.ID, ResolutionBindingHash: response.Resolution.BindingHash, RequestContractRepairHash: cmd.Proof.Hash}})
	if err != nil {
		return empty, err
	}
	*ticket = *updated
	if err := m.recordAndPublishResolvedTicket(bound, ticket, resolution, true); err != nil {
		return empty, err
	}
	return response, nil
}
