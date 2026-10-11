package decision

import (
	"context"
	"reflect"

	magicevent "github.com/muidea/magicCommon/event"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	workorchevents "github.com/muidea/workorch/internal/pkg/events"
)

// FinalCandidate 仍是提案材料，但必须来自 Workspace 持久六审结果，不能 caller 自证。
func (m *Biz) verifyContractChangeFinalCandidate(ctx context.Context, cmd decisionevents.RegisterContractChangeProposalCommand) error {
	if cmd.Proposal.FinalCandidate == nil {
		return nil
	}
	if m == nil || m.EventHub() == nil || ctx == nil || cmd.Proposal.RunID != "" || cmd.ExpectedRevision == 0 {
		return common.ErrConflict
	}
	ev := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceDefinitionChangeQuery, ModuleID, workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, workspaceevents.QueryDefinitionChangeCommand{WorkspaceID: cmd.Proposal.WorkspaceID})
	ev.BindLaneKey(workorchevents.EventHubLaneKeyFor(workspaceevents.Contracts(), ev.ID(), eventcontract.LaneScope{WorkspaceID: string(cmd.Proposal.WorkspaceID)}))
	response := m.SendEvent(ev)
	if response == nil {
		return common.ErrConflict
	}
	if err := response.Error(); err != nil {
		return err
	}
	data, resultErr := response.Get()
	if resultErr != nil {
		return resultErr
	}
	view, ok := data.(workspaceevents.QueryDefinitionChangeResult)
	if !ok || view.ValidateFor(workspaceevents.QueryDefinitionChangeCommand{WorkspaceID: cmd.Proposal.WorkspaceID}) != nil || view.Candidate == nil || view.Reviewed == nil {
		return common.ErrConflict
	}
	c, reviewed := view.Candidate, view.Reviewed
	if c.TicketID != cmd.TicketID || c.ProposalRevision != cmd.ExpectedRevision || c.ProposalHash != cmd.ExpectedHash || reviewed.ID != cmd.Proposal.FinalCandidate.ID || reviewed.Revision != cmd.Proposal.FinalCandidate.Revision || reviewed.Hash != cmd.Proposal.FinalCandidate.Hash {
		return common.ErrConflict
	}
	bound, err := common.BindRuntimeWorkspaceContext(ctx, cmd.Proposal.WorkspaceID)
	if err != nil {
		return err
	}
	ticket, err := m.tickets.Get(bound, cmd.TicketID)
	if err != nil || ticket.ValidateContractChange() != nil {
		return common.ErrConflict
	}
	previous := ticket.ContractChangeRevisions[len(ticket.ContractChangeRevisions)-1]
	if previous.Revision != cmd.ExpectedRevision || previous.Hash != cmd.ExpectedHash {
		return common.ErrConflict
	}
	before, after := previous.Proposal.Clone(), cmd.Proposal.Clone()
	before.FinalCandidate, after.FinalCandidate = nil, nil
	if !reflect.DeepEqual(before, after) {
		return common.ErrConflict
	}
	return nil
}
