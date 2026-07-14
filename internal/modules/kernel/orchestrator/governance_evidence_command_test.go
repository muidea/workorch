package orchestrator

import (
	"context"
	"testing"

	approvalbiz "github.com/muidea/workorch/internal/modules/blocks/approval/biz"
	approvalcontracts "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/contracts"
	decisionbiz "github.com/muidea/workorch/internal/modules/blocks/decision/biz"
	decisioncommon "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/common"
	decisioncontracts "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/contracts"
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
	service := decisionbiz.NewService(tickets, resolutions, deps.CommandHub)
	create := decisioncontracts.CreateTicketCommand{
		TicketID:     "ticket-pairwise-evidence",
		WorkspaceID:  "ws-pairwise-evidence",
		RunID:        "run-pairwise-evidence",
		StepID:       "step-pairwise-evidence",
		Reason:       "need operator decision",
		DecisionKind: "runtime_governance",
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
	service := approvalbiz.NewService(approvals, deps.CommandHub)
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
	if _, err := service.EnsureApprovedAndReturn(ctx, resolve); err != nil {
		t.Fatalf("approve: %v", err)
	}
	if _, err := service.EnsureApprovedAndReturn(ctx, resolve); err != nil {
		t.Fatalf("approve idempotently: %v", err)
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
