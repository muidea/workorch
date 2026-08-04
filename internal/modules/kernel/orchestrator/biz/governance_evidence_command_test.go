package orchestrator

// Owner Biz evidence tests.

import (
	"context"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	approvalbiz "github.com/muidea/workorch/internal/modules/blocks/approval/biz"
	approvalcontracts "github.com/muidea/workorch/internal/modules/blocks/approval/biz/contracts"
	approvalevents "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/events"
	decisionbiz "github.com/muidea/workorch/internal/modules/blocks/decision/biz"
	decisioncontracts "github.com/muidea/workorch/internal/modules/blocks/decision/biz/contracts"
	decisioncommon "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/common"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	memoryrepo "github.com/muidea/workorch/internal/pkg/repository/memory"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestDecisionEvidenceCommandPersistsThroughOrchestratorConsumer(t *testing.T) {
	ctx := context.Background()
	evidenceRepo := memoryrepo.NewEvidenceRecordRepository()
	deps := moduleContractDependencies()
	deps.EvidenceRecords = evidenceRepo
	installEmptyRecoveryInventory(t, deps.CommandHub)
	module := New()
	if err := module.Configure(deps); err != nil {
		t.Fatalf("configure orchestrator: %v", err)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("start orchestrator: %v", err)
	}
	t.Cleanup(func() {
		_ = module.Stop(ctx)
		deps.CommandHub.Terminate(ctx)
	})

	tickets := memoryrepo.NewDecisionTicketRepository()
	resolutions := memoryrepo.NewDecisionResolutionRepository()
	service := decisionbiz.NewService(tickets, resolutions)
	create := decisioncontracts.CreateTicketCommand{
		TicketID:     "ticket-pairwise-evidence",
		WorkspaceID:  "ws-pairwise-evidence",
		RunID:        "run-pairwise-evidence",
		StepID:       "step-pairwise-evidence",
		Reason:       "need operator decision",
		DecisionKind: "operator_input",
		Question:     "which path should continue",
	}
	if err := service.CreateTicket(ctx, create); err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	resolve := decisioncontracts.ResolveTicketCommand{
		ResolutionID: "resolution-pairwise-evidence",
		TicketID:     create.TicketID,
		WorkspaceID:  create.WorkspaceID,
		RunID:        create.RunID,
		StepID:       create.StepID,
		Type:         decisioncommon.ResolutionTypeProvideMissingInfo,
		Payload: workorcheventpayload.New(
			workorcheventpayload.StringField("operator", "tester"),
			workorcheventpayload.BoolField("retry", true),
			workorcheventpayload.StringArrayField("targets", []string{"step-a", "step-b"}),
		),
		ContinuationAction: "resume",
		ResumeRunID:        create.RunID,
	}
	if err := service.EnsureResolvedTicket(ctx, resolve); err != nil {
		t.Fatalf("resolve ticket: %v", err)
	}
	if err := service.EnsureResolvedTicket(ctx, resolve); err != nil {
		t.Fatalf("resolve ticket idempotently: %v", err)
	}
	if _, err := module.recordDecisionEvidence(ctx, decisionevents.RecordDecisionEvidenceCommand{
		EvidenceRecordID: "evidence-decision-pairwise", WorkspaceID: create.WorkspaceID, RunID: create.RunID, StepID: create.StepID,
		TicketID: create.TicketID, ResolutionID: resolve.ResolutionID, ResolutionType: resolve.Type, DecisionKind: create.DecisionKind,
		Reason: create.Reason, Question: create.Question, ResolutionPayload: resolve.Payload.Clone().Fields,
		ContinuationAction: resolve.ContinuationAction, ResumeRunID: resolve.ResumeRunID,
	}); err != nil {
		t.Fatalf("record decision evidence: %v", err)
	}

	records, err := evidenceRepo.ListByRun(common.ContextWithRuntimeWorkspaceID(ctx, create.WorkspaceID), create.RunID)
	if err != nil {
		t.Fatalf("list decision evidence: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("decision evidence count = %d, want 1", len(records))
	}
	record := records[0]
	if record.Kind != runtimemodel.EvidenceKindDecision || record.Status != runtimemodel.EvidenceStatusAccepted || record.SubjectID != string(create.TicketID) {
		t.Fatalf("decision evidence = %+v, want accepted decision record", record)
	}
	payload, ok := record.Metadata.Object("resolutionPayload")
	operator, _ := payload.String("operator")
	retry, _ := payload.Bool("retry")
	if !ok || operator != "tester" || !retry {
		t.Fatalf("decision evidence payload = %#v, want concrete fields restored", payload)
	}
}

func TestApprovalEvidenceCommandPersistsThroughOrchestratorConsumer(t *testing.T) {
	ctx := context.Background()
	evidenceRepo := memoryrepo.NewEvidenceRecordRepository()
	deps := moduleContractDependencies()
	deps.EvidenceRecords = evidenceRepo
	installEmptyRecoveryInventory(t, deps.CommandHub)
	module := New()
	if err := module.Configure(deps); err != nil {
		t.Fatalf("configure orchestrator: %v", err)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("start orchestrator: %v", err)
	}
	t.Cleanup(func() {
		_ = module.Stop(ctx)
		deps.CommandHub.Terminate(ctx)
	})

	approvals := memoryrepo.NewApprovalRepository()
	service := approvalbiz.NewService(approvals)
	create := approvalcontracts.CreateApprovalCommand{
		ApprovalID:  "approval-pairwise-evidence",
		WorkspaceID: "ws-pairwise-evidence",
		RunID:       "run-pairwise-evidence",
		StepID:      "step-pairwise-evidence",
		Reason:      "need authorization",
		Action:      "continue runtime action",
		Context: workorcheventpayload.New(
			workorcheventpayload.StringField("scope", "runtime"),
			workorcheventpayload.IntField("risk", 2),
		),
	}
	if err := service.CreateApproval(ctx, create); err != nil {
		t.Fatalf("create approval: %v", err)
	}
	resolve := approvalcontracts.ResolveApprovalCommand{
		ApprovalID:  create.ApprovalID,
		WorkspaceID: create.WorkspaceID,
		RunID:       create.RunID,
		Context: workorcheventpayload.New(
			workorcheventpayload.StringField("resolvedBy", "tester"),
		),
	}
	approved, err := service.EnsureApprovedAndReturn(ctx, resolve)
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	if _, err := service.EnsureApprovedAndReturn(ctx, resolve); err != nil {
		t.Fatalf("approve idempotently: %v", err)
	}
	if _, err := module.recordApprovalEvidence(ctx, approvalevents.RecordApprovalEvidenceCommand{
		EvidenceRecordID: "evidence-approval-pairwise", WorkspaceID: create.WorkspaceID, RunID: create.RunID, StepID: create.StepID,
		ApprovalID: create.ApprovalID, ApprovalState: common.ApprovalStateApproved, Reason: create.Reason, Action: create.Action,
		Context: approved.Context.Clone().Fields,
	}); err != nil {
		t.Fatalf("record approval evidence: %v", err)
	}

	records, err := evidenceRepo.ListByRun(common.ContextWithRuntimeWorkspaceID(ctx, create.WorkspaceID), create.RunID)
	if err != nil {
		t.Fatalf("list approval evidence: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("approval evidence count = %d, want 1", len(records))
	}
	record := records[0]
	if record.Kind != runtimemodel.EvidenceKindApproval || record.Status != runtimemodel.EvidenceStatusAccepted || record.SubjectID != string(create.ApprovalID) {
		t.Fatalf("approval evidence = %+v, want accepted approval record", record)
	}
	metadataContext, ok := record.Metadata.Object("context")
	scope, _ := metadataContext.String("scope")
	resolvedBy, _ := metadataContext.String("resolvedBy")
	if !ok || scope != "runtime" || resolvedBy != "tester" {
		t.Fatalf("approval evidence context = %#v, want merged concrete context", metadataContext)
	}
}

func installEmptyRecoveryInventory(t *testing.T, hub magicevent.Hub) {
	t.Helper()
	observer := magicevent.NewSimpleObserver(runevents.CommandDestination, hub)
	observer.Subscribe(runevents.TopicRunCommandRuntimeRecoveryInventory, func(_ magicevent.Event, result magicevent.Result) {
		if result != nil {
			result.Set(runevents.QueryRuntimeRecoveryInventoryResult{}, nil)
		}
	})
	t.Cleanup(func() { observer.Unsubscribe(runevents.TopicRunCommandRuntimeRecoveryInventory) })
}
