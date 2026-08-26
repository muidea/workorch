package orchestrator

import (
	"context"
	"strings"
	"testing"

	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeOperationExecutionResultUsesOnlyPersistedReceiptGovernance(t *testing.T) {
	failure := &runtimecontract.FailureFact{
		Class: runtimecontract.FailureClassExecutionFailure, Code: "approval_required",
		Message: "ordinary operation text must not create governance",
	}
	withoutGovernance := runtimeOperationExecutionResult(
		runtime.RuntimeOperationExecutionRequest{DispatchID: "dispatch-1", Operation: runtimecontract.RuntimeOperation{ID: "operation-1"}, EvidenceKind: runtimecontract.EvidenceKindStateChange},
		runtimecontract.RuntimeOperationReceipt{
			Outcome: runtimecontract.RuntimeOperationOutcomeRejected,
			Feedback: &runtimecontract.RuntimeFeedback{
				Domain: runtimecontract.RuntimeFeedbackDomainOperation, StableCategory: runtimecontract.RuntimeFeedbackCategory(runtimecontract.RuntimeOperationOutcomeRejected),
				OriginalDiagnostic: runtimecontract.RuntimeOriginalDiagnosticFromFailureFact(runtimecontract.RuntimeModuleFile, *failure),
			},
		},
	)
	if withoutGovernance.Governance != nil {
		t.Fatalf("failure text unexpectedly inferred governance: %+v", withoutGovernance)
	}
	if withoutGovernance.Failure == nil || withoutGovernance.Failure.Code != "approval_required" {
		t.Fatalf("ordinary failure was not retained for repair: %+v", withoutGovernance)
	}
	if withoutGovernance.EventID != "runtime-operation|dispatch-1|operation-1" || withoutGovernance.SourceRef != "workorch://runtime-operations/dispatch-1/operation-1" || withoutGovernance.OperationID != "operation-1" || withoutGovernance.EvidenceKind != runtimecontract.EvidenceKindStateChange {
		t.Fatalf("sealed operation identity was not projected by Orchestrator: %+v", withoutGovernance)
	}

	requirement := runtimecontract.RuntimeGovernanceRequirement{
		Kind: runtimecontract.GovernanceApproval, SubjectRef: ".env", Reason: "sensitive workspace file",
		RequestedAction: runtimecontract.RuntimeBlockKindPatchApply, RiskLevel: "high",
		AuthorizationContext: workorcheventpayload.New(workorcheventpayload.StringField("targetRef", ".env")),
	}
	withGovernance := runtimeOperationExecutionResult(
		runtime.RuntimeOperationExecutionRequest{DispatchID: "dispatch-2", Operation: runtimecontract.RuntimeOperation{ID: "operation-2"}, EvidenceKind: runtimecontract.EvidenceKindStateChange},
		runtimecontract.RuntimeOperationReceipt{Outcome: runtimecontract.RuntimeOperationOutcomeGovernanceRequired, Governance: &requirement},
	)
	if withGovernance.Governance == nil || !runtimecontract.SameRuntimeGovernanceRequirement(*withGovernance.Governance, requirement) {
		t.Fatalf("persisted typed governance was not preserved: %+v", withGovernance)
	}
}

// SM-3 concrete producers must not reconstruct owner authorization from the
// operation or process configuration. The persisted SM-2 catalog and its
// observation are both mandatory before an EventHub execute command exists.
func TestCapabilityConcreteProducersRequirePersistedOwnerCatalog(t *testing.T) {
	executor := eventRuntimeOperationExecutor{}
	ctx := context.Background()
	for name, execute := range map[string]func(runtime.RuntimeOperationExecutionRequest) error{
		"shell": func(request runtime.RuntimeOperationExecutionRequest) error {
			_, err := executor.executeShell(ctx, request, "", "")
			return err
		},
		"web": func(request runtime.RuntimeOperationExecutionRequest) error {
			_, err := executor.executeWeb(ctx, request, "", "")
			return err
		},
		"ssh": func(request runtime.RuntimeOperationExecutionRequest) error {
			_, err := executor.executeSSH(ctx, request, "", "")
			return err
		},
	} {
		t.Run(name+"/missing_catalog", func(t *testing.T) {
			if err := execute(runtime.RuntimeOperationExecutionRequest{}); err == nil || !strings.Contains(err.Error(), "persisted owner catalog") {
				t.Fatalf("missing owner catalog err = %v", err)
			}
		})
		t.Run(name+"/missing_observation", func(t *testing.T) {
			catalog := runtimecontract.RuntimeOperationAdmissibility{}
			if err := execute(runtime.RuntimeOperationExecutionRequest{OwnerCatalog: &catalog}); err == nil || !strings.Contains(err.Error(), "persisted owner catalog") {
				t.Fatalf("missing owner catalog observation err = %v", err)
			}
		})
	}
	t.Run("mcp/missing_catalog", func(t *testing.T) {
		if _, err := executor.executeMCP(ctx, runtime.RuntimeOperationExecutionRequest{}, "", ""); err == nil || !strings.Contains(err.Error(), "persisted owner catalog") {
			t.Fatalf("missing MCP owner catalog err = %v", err)
		}
	})
}

func TestConcreteProducerCarriesSealedOwnerCatalogExecutionIdentity(t *testing.T) {
	record := runtimecontract.RuntimeOperationExecutionRecord{
		OperationID: "web-1", ExecutionKey: "exec/ws/run/dispatch/web-1", Fence: 3,
		ExpectedEffectScope:     runtimecontract.RuntimeEffectNone,
		AdmissibilitySnapshotID: "operation-admissibility-web-1",
		AcquisitionBinding:      "capability-state/ws/run/web/0",
	}
	envelope := runtimeOperationExecutionEnvelope(record, nil)
	if envelope.AdmissibilitySnapshotID != record.AdmissibilitySnapshotID || envelope.AcquisitionBinding != record.AcquisitionBinding {
		t.Fatalf("concrete producer lost sealed owner catalog identity: record=%+v envelope=%+v", record, envelope)
	}
}
