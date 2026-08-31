package orchestrator

import (
	"context"
	"strings"
	"testing"

	fileevents "github.com/muidea/workorch/internal/modules/blocks/file/pkg/events"
	gitevents "github.com/muidea/workorch/internal/modules/blocks/git/pkg/events"
	mcpevents "github.com/muidea/workorch/internal/modules/blocks/mcp/pkg/events"
	shellevents "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/events"
	sshevents "github.com/muidea/workorch/internal/modules/blocks/ssh/pkg/events"
	webevents "github.com/muidea/workorch/internal/modules/blocks/web/pkg/events"
	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeOperationExecutionEventBindsExactExecutionLane(t *testing.T) {
	base := runtime.RuntimeOperationExecutionRequest{
		WorkspaceID: "ws-lane", RunID: "run-lane",
		Execution: runtimecontract.RuntimeOperationExecutionRecord{ExecutionKey: "exec/ws-lane/run-lane/dispatch-1/operation-1", Fence: 1},
	}
	tests := []struct {
		name        string
		contracts   []eventcontract.Contract
		topic       string
		destination string
		laneOwner   string
	}{
		{name: "file_list", contracts: fileevents.Contracts(), topic: fileevents.TopicExecuteFileListOperation, destination: runtimecontract.RuntimeModuleFile, laneOwner: "file"},
		{name: "file_read", contracts: fileevents.Contracts(), topic: fileevents.TopicExecuteFileReadOperation, destination: runtimecontract.RuntimeModuleFile, laneOwner: "file"},
		{name: "file_edit", contracts: fileevents.Contracts(), topic: fileevents.TopicExecuteFileEditOperation, destination: runtimecontract.RuntimeModuleFile, laneOwner: "file"},
		{name: "file_write", contracts: fileevents.Contracts(), topic: fileevents.TopicExecuteFileWriteOperation, destination: runtimecontract.RuntimeModuleFile, laneOwner: "file"},
		{name: "patch_apply", contracts: fileevents.Contracts(), topic: fileevents.TopicExecutePatchApplyOperation, destination: runtimecontract.RuntimeModuleFile, laneOwner: "file"},
		{name: "patch_preview", contracts: fileevents.Contracts(), topic: fileevents.TopicExecutePatchPreviewOperation, destination: runtimecontract.RuntimeModuleFile, laneOwner: "file"},
		{name: "git", contracts: gitevents.Contracts(), topic: gitevents.TopicExecuteOperation, destination: runtimecontract.RuntimeModuleGit, laneOwner: "git"},
		{name: "shell", contracts: shellevents.Contracts(), topic: shellevents.TopicExecuteOperation, destination: runtimecontract.RuntimeModuleShell, laneOwner: "shell"},
		{name: "ssh", contracts: sshevents.Contracts(), topic: sshevents.TopicExecuteOperation, destination: runtimecontract.RuntimeModuleSSH, laneOwner: "ssh"},
		{name: "web", contracts: webevents.Contracts(), topic: webevents.TopicExecuteOperation, destination: runtimecontract.RuntimeModuleWeb, laneOwner: "web"},
		{name: "mcp", contracts: mcpevents.Contracts(), topic: mcpevents.TopicExecuteOperation, destination: runtimecontract.RuntimeModuleMCP, laneOwner: "mcp"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			contractFound := false
			for _, contract := range test.contracts {
				if contract.Topic == test.topic {
					contractFound = true
					if contract.LanePolicy != eventcontract.LanePolicyExecute {
						t.Fatalf("owner contract lane policy = %q, want %q", contract.LanePolicy, eventcontract.LanePolicyExecute)
					}
					if contract.Metadata.EffectClass != eventcontract.EffectClassExternalSideEffect {
						t.Fatalf("owner execute effect class = %q, want %q", contract.Metadata.EffectClass, eventcontract.EffectClassExternalSideEffect)
					}
				}
			}
			if !contractFound {
				t.Fatalf("execute topic %q is absent from receiver contracts", test.topic)
			}

			event := runtimeOperationExecutionEvent(context.Background(), test.contracts, test.topic, test.destination, base, struct{}{})
			want := "exec/block/" + test.laneOwner + "/workspace/ws-lane/step/" + base.Execution.ExecutionKey
			if event.LaneKey() != want {
				t.Fatalf("operation execution lane = %q, want %q", event.LaneKey(), want)
			}

			retry := base
			retry.Execution.Fence = 2
			retryEvent := runtimeOperationExecutionEvent(context.Background(), test.contracts, test.topic, test.destination, retry, struct{}{})
			if retryEvent.LaneKey() != event.LaneKey() {
				t.Fatalf("same execution key with a new fence changed lane: first=%q retry=%q", event.LaneKey(), retryEvent.LaneKey())
			}

			other := base
			other.Execution.ExecutionKey = "exec/ws-lane/run-lane/dispatch-1/operation-2"
			otherEvent := runtimeOperationExecutionEvent(context.Background(), test.contracts, test.topic, test.destination, other, struct{}{})
			if otherEvent.LaneKey() == event.LaneKey() {
				t.Fatal("distinct execution keys shared one capability execution lane")
			}
		})
	}
}

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
		IdempotencyClass:        runtimecontract.RuntimeOperationIdempotencyAtMostOnce,
		AdmissibilitySnapshotID: "operation-admissibility-web-1",
		AcquisitionBinding:      "capability-state/ws/run/web/0",
	}
	record.AuthorityHash = runtimecontract.StableRuntimeOperationExecutionAuthorityHash(record)
	envelope := runtimeOperationExecutionEnvelope(record, nil)
	if envelope.AdmissibilitySnapshotID != record.AdmissibilitySnapshotID || envelope.AcquisitionBinding != record.AcquisitionBinding ||
		envelope.IdempotencyClass != record.IdempotencyClass || envelope.AuthorityHash != record.AuthorityHash {
		t.Fatalf("concrete producer lost sealed owner catalog identity: record=%+v envelope=%+v", record, envelope)
	}
}
