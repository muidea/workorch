package orchestrator

import (
	"context"
	"fmt"

	magicevent "github.com/muidea/magicCommon/event"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	runtimev2 "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtimev2"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// RUNTIME-SMO: Orchestrator publishes immutable lifecycle facts and sends the
// corresponding Run owner lifecycle commands. It never assigns Run.State.

const orchestratorLifecycleSource = "kernel/orchestrator"

func (m *Biz) applyRunGovernanceWait(
	ctx context.Context,
	workspaceID common.WorkspaceID,
	runID common.RunID,
	authoritySnapshotID string,
	definitionRevision string,
	evidenceRevision string,
	dispatchID string,
	ticketID common.TicketID,
	approvalID common.ApprovalID,
	stateRevision uint64,
) error {
	if m == nil || m.EventHub() == nil || workspaceID == "" || runID == "" {
		return common.ErrInvalidArgument
	}
	cmd := runevents.ApplyRuntimeGovernanceWaitCommand{
		WorkspaceID:         workspaceID,
		RunID:               runID,
		SourceOwner:         orchestratorLifecycleSource,
		CausationID:         runtimeGovernanceWaitCausationID(authoritySnapshotID, dispatchID, ticketID, approvalID),
		AuthoritySnapshotID: authoritySnapshotID,
		DefinitionRevision:  definitionRevision,
		EvidenceRevision:    evidenceRevision,
		DispatchID:          dispatchID,
		BlockingTicketID:    ticketID,
		BlockingApprovalID:  approvalID,
	}.Normalized()
	if err := cmd.Validate(); err != nil {
		return err
	}
	event := magicevent.NewEventWithContext(
		runevents.TopicRunCommandApplyGovernanceWait,
		ModuleID,
		runevents.CommandDestination,
		magicevent.NewHeader(),
		ctx,
		cmd,
	)
	bindOwnerCommandLane(event, runevents.TopicRunCommandApplyGovernanceWait, "run", string(runID), workspaceID, runID)
	response, err := runLifecycleCommandResponse(m.EventHub().Send(event), runevents.TopicRunCommandApplyGovernanceWait)
	if err != nil {
		return err
	}
	m.postGovernanceWaitRecorded(ctx, cmd, stateRevision)
	_ = response
	return nil
}

func (m *Biz) applyRunContinuationEligible(
	ctx context.Context,
	workspaceID common.WorkspaceID,
	runID common.RunID,
	authoritySnapshotID string,
	definitionRevision string,
	evidenceRevision string,
	dispatchID string,
	resolutionID common.ResolutionID,
	ticketID common.TicketID,
	approvalID common.ApprovalID,
	stateRevision uint64,
) error {
	if m == nil || m.EventHub() == nil || workspaceID == "" || runID == "" {
		return common.ErrInvalidArgument
	}
	cmd := runevents.ApplyRuntimeContinuationEligibleCommand{
		WorkspaceID:         workspaceID,
		RunID:               runID,
		SourceOwner:         orchestratorLifecycleSource,
		CausationID:         runtimeContinuationCausationID(authoritySnapshotID, dispatchID, resolutionID),
		AuthoritySnapshotID: authoritySnapshotID,
		DefinitionRevision:  definitionRevision,
		EvidenceRevision:    evidenceRevision,
		DispatchID:          dispatchID,
		ResolutionID:        resolutionID,
		TicketID:            ticketID,
		ApprovalID:          approvalID,
	}.Normalized()
	if err := cmd.Validate(); err != nil {
		return err
	}
	event := magicevent.NewEventWithContext(
		runevents.TopicRunCommandApplyContinuationEligible,
		ModuleID,
		runevents.CommandDestination,
		magicevent.NewHeader(),
		ctx,
		cmd,
	)
	bindOwnerCommandLane(event, runevents.TopicRunCommandApplyContinuationEligible, "run", string(runID), workspaceID, runID)
	response, err := runLifecycleCommandResponse(m.EventHub().Send(event), runevents.TopicRunCommandApplyContinuationEligible)
	if err != nil {
		return err
	}
	m.postContinuationEligible(ctx, cmd, stateRevision)
	_ = response
	return nil
}

func runtimeGovernanceWaitCausationID(authoritySnapshotID, dispatchID string, ticketID common.TicketID, approvalID common.ApprovalID) string {
	return fmt.Sprintf("runtime-governance-wait/%s/%s/%s/%s", authoritySnapshotID, dispatchID, ticketID, approvalID)
}

func runtimeContinuationCausationID(authoritySnapshotID, dispatchID string, resolutionID common.ResolutionID) string {
	return fmt.Sprintf("runtime-governance-continuation/%s/%s/%s", authoritySnapshotID, dispatchID, resolutionID)
}

func (m *Biz) postGovernanceWaitRecorded(ctx context.Context, cmd runevents.ApplyRuntimeGovernanceWaitCommand, stateRevision uint64) {
	if m == nil || m.EventHub() == nil {
		return
	}
	data := orchestratorevents.RuntimeGovernanceWaitRecordedData{
		WorkspaceID:         cmd.WorkspaceID,
		RunID:               cmd.RunID,
		SourceOwner:         cmd.SourceOwner,
		CausationID:         cmd.CausationID,
		AuthoritySnapshotID: cmd.AuthoritySnapshotID,
		DefinitionRevision:  cmd.DefinitionRevision,
		EvidenceRevision:    cmd.EvidenceRevision,
		DispatchID:          cmd.DispatchID,
		BlockingTicketID:    cmd.BlockingTicketID,
		BlockingApprovalID:  cmd.BlockingApprovalID,
		StateRevision:       stateRevision,
	}.Normalized()
	if err := data.Validate(); err != nil {
		return
	}
	event := magicevent.NewEventWithContext(
		orchestratorevents.TopicOrchestratorRuntimeGovernanceWaitRecorded,
		ModuleID,
		orchestratorevents.NotificationDestination,
		magicevent.NewHeader(),
		ctx,
		data,
	)
	m.PostEvent(event)
}

func (m *Biz) postContinuationEligible(ctx context.Context, cmd runevents.ApplyRuntimeContinuationEligibleCommand, stateRevision uint64) {
	if m == nil || m.EventHub() == nil {
		return
	}
	data := orchestratorevents.RuntimeContinuationEligibleData{
		WorkspaceID:         cmd.WorkspaceID,
		RunID:               cmd.RunID,
		SourceOwner:         cmd.SourceOwner,
		CausationID:         cmd.CausationID,
		AuthoritySnapshotID: cmd.AuthoritySnapshotID,
		DefinitionRevision:  cmd.DefinitionRevision,
		EvidenceRevision:    cmd.EvidenceRevision,
		DispatchID:          cmd.DispatchID,
		ResolutionID:        cmd.ResolutionID,
		TicketID:            cmd.TicketID,
		ApprovalID:          cmd.ApprovalID,
		StateRevision:       stateRevision,
	}.Normalized()
	if err := data.Validate(); err != nil {
		return
	}
	event := magicevent.NewEventWithContext(
		orchestratorevents.TopicOrchestratorRuntimeContinuationEligible,
		ModuleID,
		orchestratorevents.NotificationDestination,
		magicevent.NewHeader(),
		ctx,
		data,
	)
	m.PostEvent(event)
}

func runLifecycleCommandResponse(result magicevent.Result, topic string) (runevents.RunLifecycleCommandResult, error) {
	if result == nil {
		return runevents.RunLifecycleCommandResult{}, common.ErrInvalidArgument
	}
	if err := prerequisiteResultError(result); err != nil {
		return runevents.RunLifecycleCommandResult{}, err
	}
	data, magicErr := result.Get()
	if magicErr != nil {
		return runevents.RunLifecycleCommandResult{}, prerequisiteMagicError(magicErr)
	}
	response, ok := data.(runevents.RunLifecycleCommandResult)
	if !ok {
		return runevents.RunLifecycleCommandResult{}, fmt.Errorf("run owner returned invalid lifecycle result for %s", topic)
	}
	return response, nil
}

func governanceWaitFromDriveResult(result orchestratorevents.DriveRuntimeV2Result) (authoritySnapshotID, definitionRevision, evidenceRevision, dispatchID string, ticketID common.TicketID, approvalID common.ApprovalID) {
	return result.AuthoritySnapshotID, result.DefinitionRevision, result.EvidenceRevision, result.DispatchID, result.BlockingTicketID, ""
}

func continuationFromGovernanceCommit(
	resolution runtimecontract.RuntimeGovernanceResolution,
	response orchestratorevents.CommitRuntimeGovernanceResolutionResult,
	aggregate runtimev2.RuntimeAggregate,
) (authoritySnapshotID, definitionRevision, evidenceRevision, dispatchID string, resolutionID common.ResolutionID, ticketID common.TicketID, approvalID common.ApprovalID, stateRevision uint64) {
	// The continuation is bound to the authority/dispatch that opened the
	// governance request. Commit may already have sealed a successor authority,
	// so using aggregate.Decision.AuthoritySnapshot here would lose the binding
	// that Run must verify against GovernanceResolutions.
	return resolution.AuthoritySnapshotID, aggregate.Facts.DefinitionRevision, aggregate.Facts.EvidenceRevision, resolution.DispatchID, resolution.ResolutionID, resolution.TicketID, "", aggregate.StateRevision
}
