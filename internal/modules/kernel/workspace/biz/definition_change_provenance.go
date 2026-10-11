package workspace

import (
	"context"
	"fmt"

	magicevent "github.com/muidea/magicCommon/event"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	workorchevents "github.com/muidea/workorch/internal/pkg/events"
)

// 原 Decision 版本由接收 owner 查询；跨 owner 观察不是批准或原子启动屏障。
func (m *Biz) observeDefinitionChangeProposal(ctx context.Context, cmd workspaceevents.PrepareDefinitionChangeCommand) (decisionmodel.ContractChangeProposal, error) {
	if m == nil || m.EventHub() == nil || ctx == nil || cmd.Validate() != nil {
		return decisionmodel.ContractChangeProposal{}, common.ErrInvalidArgument
	}
	bound, err := common.BindRuntimeWorkspaceContext(ctx, cmd.WorkspaceID)
	if err != nil {
		return decisionmodel.ContractChangeProposal{}, err
	}
	ev := magicevent.NewEventWithContext(decisionevents.TopicDecisionCommandGetTicket, ModuleID, decisionevents.CommandDestination, magicevent.NewHeader(), bound,
		decisionevents.GetTicketCommand{WorkspaceID: cmd.WorkspaceID, TicketID: cmd.TicketID})
	ev.BindLaneKey(workorchevents.EventHubLaneKeyFor(decisionevents.Contracts(), ev.ID(), eventcontract.LaneScope{WorkspaceID: string(cmd.WorkspaceID), SubjectType: "decision_ticket", SubjectID: string(cmd.TicketID)}))
	response := m.SendEvent(ev)
	if response == nil {
		return decisionmodel.ContractChangeProposal{}, common.ErrConflict
	}
	if err := response.Error(); err != nil {
		return decisionmodel.ContractChangeProposal{}, err
	}
	data, resultErr := response.Get()
	if resultErr != nil {
		return decisionmodel.ContractChangeProposal{}, resultErr
	}
	result, ok := data.(decisionevents.GetTicketResult)
	if !ok || result.Ticket == nil || result.Ticket.ValidateContractChange() != nil || result.Ticket.WorkspaceID != cmd.WorkspaceID || result.Ticket.ID != cmd.TicketID || result.Ticket.RunID != "" {
		return decisionmodel.ContractChangeProposal{}, common.ErrConflict
	}
	latest := result.Ticket.ContractChangeRevisions[len(result.Ticket.ContractChangeRevisions)-1]
	if latest.Revision != cmd.ProposalRevision || latest.Hash != cmd.ProposalHash {
		return decisionmodel.ContractChangeProposal{}, fmt.Errorf("%w: candidate requires current persisted proposal", common.ErrConflict)
	}
	// 已有 Run 即使投影 ActiveRunID 为空也不可进入零 Run 准备合同。
	runEvent := magicevent.NewEventWithContext(runevents.TopicRunCommandListByWorkspace, ModuleID, runevents.CommandDestination, magicevent.NewHeader(), bound, runevents.ListRunsByWorkspaceCommand{WorkspaceID: cmd.WorkspaceID})
	runEvent.BindLaneKey(workorchevents.EventHubLaneKeyFor(runevents.Contracts(), runEvent.ID(), eventcontract.LaneScope{WorkspaceID: string(cmd.WorkspaceID)}))
	response = m.SendEvent(runEvent)
	if response == nil {
		return decisionmodel.ContractChangeProposal{}, common.ErrConflict
	}
	if err := response.Error(); err != nil {
		return decisionmodel.ContractChangeProposal{}, err
	}
	data, resultErr = response.Get()
	if resultErr != nil {
		return decisionmodel.ContractChangeProposal{}, resultErr
	}
	runs, ok := data.(runevents.ListRunsByWorkspaceResult)
	if !ok || len(runs.Runs) != 0 {
		return decisionmodel.ContractChangeProposal{}, common.ErrConflict
	}
	return latest.Proposal.Clone(), nil
}

// 只观察新 reserve 的实际来源。已接纳 permit 的结算不依赖之后变化的提案。
func (m *Biz) observeDefinitionChangeAuthoring(ctx context.Context, event definitionSpecificationInternalEvent) (string, error) {
	inputHash := ""
	if event.DeriveReserve != nil {
		inputHash = event.DeriveReserve.InputHash
	} else if event.AuthoringSpecReserve != nil {
		inputHash = event.AuthoringSpecReserve.InputHash
	} else if event.PreparationDiagnostic != nil {
		inputHash = event.PreparationDiagnostic.Diagnostic.InputHash
	}
	if inputHash == "" {
		return "", nil
	}
	bound, err := common.BindRuntimeWorkspaceContext(ctx, event.WorkspaceID)
	if err != nil {
		return "", err
	}
	w, err := m.workspaces.Get(bound, event.WorkspaceID)
	if err != nil {
		return "", err
	}
	c := w.DefinitionChangeCandidate
	if c == nil || c.Record.InputHash != inputHash {
		return "", nil
	}
	if c.Validate(w) != nil || workspaceDeletionRejectsMutation(w) != nil {
		return "", common.ErrConflict
	}
	_, err = m.observeDefinitionChangeProposal(ctx, workspaceevents.PrepareDefinitionChangeCommand{
		WorkspaceID: w.ID, RequestID: c.RequestID, ExpectedWorkspaceRevision: c.ExpectedWorkspaceRevision,
		TicketID: c.TicketID, ProposalRevision: c.ProposalRevision, ProposalHash: c.ProposalHash,
	})
	if err != nil {
		return "", err
	}
	return inputHash, nil
}

func (m *Biz) queryDefinitionChange(ctx context.Context, cmd workspaceevents.QueryDefinitionChangeCommand) (workspaceevents.QueryDefinitionChangeResult, error) {
	if m == nil || m.manager == nil || m.workspaces == nil || ctx == nil || cmd.WorkspaceID == "" {
		return workspaceevents.QueryDefinitionChangeResult{}, common.ErrInvalidArgument
	}
	m.manager.definitionMu.Lock()
	defer m.manager.definitionMu.Unlock()
	bound, err := common.BindRuntimeWorkspaceContext(ctx, cmd.WorkspaceID)
	if err != nil {
		return workspaceevents.QueryDefinitionChangeResult{}, err
	}
	w, err := m.workspaces.Get(bound, cmd.WorkspaceID)
	if err != nil {
		return workspaceevents.QueryDefinitionChangeResult{}, err
	}
	if err := workspaceDeletionRejectsMutation(w); err != nil {
		return workspaceevents.QueryDefinitionChangeResult{}, err
	}
	if err := validateWorkspaceAuthoringMappingOutcomes(w); err != nil {
		return workspaceevents.QueryDefinitionChangeResult{}, err
	}
	result := workspaceevents.QueryDefinitionChangeResult{WorkspaceID: w.ID, WorkspaceRevision: w.Revision}
	if c := w.DefinitionChangeCandidate; c != nil {
		if c.Validate(w) != nil {
			return workspaceevents.QueryDefinitionChangeResult{}, common.ErrConflict
		}
		value := definitionChangePreparationResult(w, *c).Candidate
		result.Candidate = &value
		if identity, err := c.ReviewedIdentity(w); err == nil {
			result.Reviewed = &identity
		}
	}
	return result, result.ValidateFor(cmd)
}
