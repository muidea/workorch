package orchestrator

import (
	"context"
	"fmt"

	magicevent "github.com/muidea/magicCommon/event"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
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
	bindOwnerCommandLane(event, runevents.Contracts(), runevents.TopicRunCommandApplyGovernanceWait, "run", string(runID), workspaceID, runID)
	response, err := runLifecycleCommandResponse(m.EventHub().Send(event), runevents.TopicRunCommandApplyGovernanceWait)
	if err != nil {
		return err
	}
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
	bindOwnerCommandLane(event, runevents.Contracts(), runevents.TopicRunCommandApplyContinuationEligible, "run", string(runID), workspaceID, runID)
	response, err := runLifecycleCommandResponse(m.EventHub().Send(event), runevents.TopicRunCommandApplyContinuationEligible)
	if err != nil {
		return err
	}
	_ = response
	return nil
}

// applyRunRuntimeStateIncompatible delivers an immutable SM-4 observation to
// the Run owner. It does not attempt to write a failure into an aggregate the
// current binary cannot load, and it never schedules LLM, operation, or
// governance work for that record.
func (m *Biz) applyRunRuntimeStateIncompatible(
	ctx context.Context,
	item runevents.RuntimeRecoveryInventoryItem,
	schemaErr *runtimecontract.RuntimeStateSchemaVersionError,
) error {
	if m == nil || m.EventHub() == nil || schemaErr == nil || item.WorkspaceID == "" || item.RunID == "" || item.RuntimeStateRef == "" {
		return common.ErrInvalidArgument
	}
	feedback := runtimecontract.RuntimeFeedback{
		Domain:         runtimecontract.RuntimeFeedbackDomainDrive,
		StableCategory: runtimecontract.RuntimeFeedbackCategoryStateSchemaUnsupported,
		WorkspaceID:    item.WorkspaceID,
		RunID:          item.RunID,
		OriginalDiagnostic: &runtimecontract.RuntimeOriginalDiagnostic{
			SchemaVersion:    runtimecontract.RuntimeOriginalDiagnosticSchemaCurrent,
			OwnerModuleID:    ModuleID,
			OriginalClass:    "runtime_state_schema",
			OriginalCode:     "unsupported_runtime_state_schema",
			OriginalMessage:  "runtime state schema is unsupported by this binary",
			Signature:        fmt.Sprintf("runtime-state-schema/%d/%d/%s", schemaErr.Actual, schemaErr.Supported, item.RuntimeStateRef),
			ObservedRevision: item.RuntimeStateRef,
		},
	}
	cmd := runevents.ApplyRuntimeStateIncompatibleFactCommand{
		WorkspaceID:            item.WorkspaceID,
		RunID:                  item.RunID,
		SourceOwner:            ModuleID,
		CausationID:            fmt.Sprintf("runtime-state-incompatible/%s/%d/%d", item.RuntimeStateRef, schemaErr.Actual, schemaErr.Supported),
		RuntimeStateRef:        item.RuntimeStateRef,
		ActualSchemaVersion:    schemaErr.Actual,
		SupportedSchemaVersion: schemaErr.Supported,
		Feedback:               feedback,
	}.Normalized()
	if err := cmd.Validate(); err != nil {
		return err
	}
	event := magicevent.NewEventWithContext(
		runevents.TopicRunCommandApplyRuntimeStateIncompatible,
		ModuleID,
		runevents.CommandDestination,
		magicevent.NewHeader(),
		ctx,
		cmd,
	)
	bindOwnerCommandLane(event, runevents.Contracts(), runevents.TopicRunCommandApplyRuntimeStateIncompatible, "run", string(item.RunID), item.WorkspaceID, item.RunID)
	_, err := runLifecycleCommandResponse(m.EventHub().Send(event), runevents.TopicRunCommandApplyRuntimeStateIncompatible)
	return err
}

func runtimeGovernanceWaitCausationID(authoritySnapshotID, dispatchID string, ticketID common.TicketID, approvalID common.ApprovalID) string {
	return fmt.Sprintf("runtime-governance-wait/%s/%s/%s/%s", authoritySnapshotID, dispatchID, ticketID, approvalID)
}

func runtimeContinuationCausationID(authoritySnapshotID, dispatchID string, resolutionID common.ResolutionID) string {
	return fmt.Sprintf("runtime-governance-continuation/%s/%s/%s", authoritySnapshotID, dispatchID, resolutionID)
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

func governanceWaitFromDriveResult(result orchestratorevents.DriveRuntimeResult) (authoritySnapshotID, definitionRevision, evidenceRevision, dispatchID string, ticketID common.TicketID, approvalID common.ApprovalID) {
	return result.AuthoritySnapshotID, result.DefinitionRevision, result.EvidenceRevision, result.DispatchID, result.BlockingTicketID, ""
}

func continuationFromGovernanceCommit(
	resolution runtimecontract.RuntimeGovernanceResolution,
	response orchestratorevents.CommitRuntimeGovernanceResolutionResult,
	aggregate runtime.RuntimeAggregate,
) (authoritySnapshotID, definitionRevision, evidenceRevision, dispatchID string, resolutionID common.ResolutionID, ticketID common.TicketID, approvalID common.ApprovalID, stateRevision uint64) {
	// The continuation is bound to the authority/dispatch that opened the
	// governance request. Commit may already have sealed a successor authority,
	// so using aggregate.Decision.AuthoritySnapshot here would lose the binding
	// that Run must verify against GovernanceResolutions.
	return resolution.AuthoritySnapshotID, aggregate.Facts.DefinitionRevision, aggregate.Facts.EvidenceRevision, resolution.DispatchID, resolution.ResolutionID, resolution.TicketID, "", aggregate.StateRevision
}
