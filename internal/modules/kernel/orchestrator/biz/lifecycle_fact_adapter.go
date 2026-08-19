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
	cmd := runevents.RuntimeGovernanceWaitFact{
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
		runevents.TopicRunFactGovernanceWait,
		ModuleID,
		runevents.CommandDestination,
		magicevent.NewHeader(),
		ctx,
		cmd,
	)
	bindOwnerCommandLane(event, runevents.Contracts(), runevents.TopicRunFactGovernanceWait, "run", string(runID), workspaceID, runID)
	response, err := runLifecycleCommandResponse(m.EventHub().Send(event), runevents.TopicRunFactGovernanceWait)
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
	cmd := runevents.RuntimeContinuationEligibleFact{
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
		runevents.TopicRunFactContinuationEligible,
		ModuleID,
		runevents.CommandDestination,
		magicevent.NewHeader(),
		ctx,
		cmd,
	)
	bindOwnerCommandLane(event, runevents.Contracts(), runevents.TopicRunFactContinuationEligible, "run", string(runID), workspaceID, runID)
	response, err := runLifecycleCommandResponse(m.EventHub().Send(event), runevents.TopicRunFactContinuationEligible)
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
	workspaceID common.WorkspaceID,
	runID common.RunID,
	runtimeStateRef string,
	incompatibility runtimecontract.RuntimeStateIncompatibility,
	diagnostic error,
) error {
	if m == nil || m.EventHub() == nil || workspaceID == "" || runID == "" || runtimeStateRef == "" || incompatibility.Validate() != nil {
		return common.ErrInvalidArgument
	}
	incompatibility = incompatibility.Normalized()
	category := runtimecontract.RuntimeFeedbackCategoryStateSchemaUnsupported
	originalClass := "runtime_state_schema"
	originalCode := "unsupported_runtime_state_schema"
	originalMessage := "runtime state schema is unsupported by this binary"
	if incompatibility.Reason == runtimecontract.RuntimeStateIncompatibilityReasonCWILedgerInvalid {
		category = runtimecontract.RuntimeFeedbackCategoryStateCWILedgerInvalid
		originalClass = "runtime_cwi_ledger"
		originalCode = "invalid_runtime_cwi_ledger"
		originalMessage = "runtime state CWI ledger is invalid and cannot be resumed"
	}
	if diagnostic != nil {
		originalMessage = diagnostic.Error()
	}
	feedback := runtimecontract.RuntimeFeedback{
		Domain:         runtimecontract.RuntimeFeedbackDomainDrive,
		StableCategory: category,
		WorkspaceID:    workspaceID,
		RunID:          runID,
		OriginalDiagnostic: &runtimecontract.RuntimeOriginalDiagnostic{
			SchemaVersion:    runtimecontract.RuntimeOriginalDiagnosticSchemaCurrent,
			OwnerModuleID:    ModuleID,
			OriginalClass:    originalClass,
			OriginalCode:     originalCode,
			OriginalMessage:  originalMessage,
			Signature:        fmt.Sprintf("runtime-state-incompatible/%s/%d/%d/%s", incompatibility.Reason, incompatibility.ActualSchemaVersion, incompatibility.SupportedSchemaVersion, runtimeStateRef),
			ObservedRevision: runtimeStateRef,
		},
	}
	cmd := runevents.RuntimeStateIncompatibleFact{
		WorkspaceID:     workspaceID,
		RunID:           runID,
		SourceOwner:     ModuleID,
		CausationID:     fmt.Sprintf("runtime-state-incompatible/%s/%s/%d/%d", runtimeStateRef, incompatibility.Reason, incompatibility.ActualSchemaVersion, incompatibility.SupportedSchemaVersion),
		RuntimeStateRef: runtimeStateRef,
		Incompatibility: incompatibility,
		Feedback:        feedback,
	}.Normalized()
	if err := cmd.Validate(); err != nil {
		return err
	}
	event := magicevent.NewEventWithContext(
		runevents.TopicRunFactRuntimeStateIncompatible,
		ModuleID,
		runevents.CommandDestination,
		magicevent.NewHeader(),
		ctx,
		cmd,
	)
	bindOwnerCommandLane(event, runevents.Contracts(), runevents.TopicRunFactRuntimeStateIncompatible, "run", string(runID), workspaceID, runID)
	_, err := runLifecycleCommandResponse(m.EventHub().Send(event), runevents.TopicRunFactRuntimeStateIncompatible)
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
	return result.AuthoritySnapshotID, result.DefinitionRevision, result.EvidenceRevision, result.DispatchID, result.BlockingTicketID, result.BlockingApprovalID
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
	return resolution.AuthoritySnapshotID, aggregate.Facts.DefinitionRevision, aggregate.Facts.EvidenceRevision, resolution.DispatchID, resolution.ResolutionID, resolution.TicketID, resolution.ApprovalID, aggregate.StateRevision
}
