package orchestrator

// Owner Biz evidence tests.

import (
	"context"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	approvalevents "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/events"
	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
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

	command := decisionevents.RecordDecisionEvidenceCommand{
		EvidenceRecordID: "evidence-decision-pairwise",
		WorkspaceID:      "ws-pairwise-evidence",
		RunID:            "run-pairwise-evidence",
		StepID:           "step-pairwise-evidence",
		TicketID:         "ticket-pairwise-evidence",
		ResolutionID:     "resolution-pairwise-evidence",
		ResolutionType:   decisioncommon.ResolutionTypeProvideMissingInfo,
		DecisionKind:     "operator_input",
		Reason:           "need operator decision",
		Question:         "which path should continue",
		ResolutionPayload: workorcheventpayload.New(
			workorcheventpayload.StringField("operator", "tester"),
			workorcheventpayload.BoolField("retry", true),
			workorcheventpayload.StringArrayField("targets", []string{"step-a", "step-b"}),
		).Fields,
		ContinuationAction: "resume",
		ResumeRunID:        "run-pairwise-evidence",
	}
	if _, err := module.recordDecisionEvidence(ctx, command); err != nil {
		t.Fatalf("record decision evidence: %v", err)
	}

	records, err := evidenceRepo.ListByRun(common.ContextWithRuntimeWorkspaceID(ctx, command.WorkspaceID), command.RunID)
	if err != nil {
		t.Fatalf("list decision evidence: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("decision evidence count = %d, want 1", len(records))
	}
	record := records[0]
	if record.Kind != runtimemodel.EvidenceKindDecision || record.Status != runtimemodel.EvidenceStatusAccepted || record.SubjectID != string(command.TicketID) {
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

	command := approvalevents.RecordApprovalEvidenceCommand{
		EvidenceRecordID: "evidence-approval-pairwise",
		WorkspaceID:      "ws-pairwise-evidence",
		RunID:            "run-pairwise-evidence",
		StepID:           "step-pairwise-evidence",
		ApprovalID:       "approval-pairwise-evidence",
		ApprovalState:    common.ApprovalStateApproved,
		ResolutionID:     "resolution-approval-pairwise-evidence",
		Principal: approvalmodel.ApprovalResolutionPrincipal{
			Kind: approvalmodel.ApprovalResolutionPrincipalHumanOperator, SubjectID: "tester",
		},
		Reason: "need authorization",
		Action: "continue runtime action",
		Context: workorcheventpayload.New(
			workorcheventpayload.StringField("scope", "runtime"),
			workorcheventpayload.IntField("risk", 2),
		).Fields,
		ResolutionContext: workorcheventpayload.New(
			workorcheventpayload.StringField("resolvedBy", "tester"),
		).Fields,
	}
	if _, err := module.recordApprovalEvidence(ctx, command); err != nil {
		t.Fatalf("record approval evidence: %v", err)
	}

	records, err := evidenceRepo.ListByRun(common.ContextWithRuntimeWorkspaceID(ctx, command.WorkspaceID), command.RunID)
	if err != nil {
		t.Fatalf("list approval evidence: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("approval evidence count = %d, want 1", len(records))
	}
	record := records[0]
	if record.Kind != runtimemodel.EvidenceKindApproval || record.Status != runtimemodel.EvidenceStatusAccepted || record.SubjectID != string(command.ApprovalID) {
		t.Fatalf("approval evidence = %+v, want accepted approval record", record)
	}
	metadataContext, ok := record.Metadata.Object("context")
	scope, _ := metadataContext.String("scope")
	metadataResolution, resolutionOK := record.Metadata.Object("resolutionContext")
	resolvedBy, _ := metadataResolution.String("resolvedBy")
	if !ok || !resolutionOK || scope != "runtime" || resolvedBy != "tester" {
		t.Fatalf("approval evidence contexts = target:%#v resolution:%#v, want separate immutable target and resolution metadata", metadataContext, metadataResolution)
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
