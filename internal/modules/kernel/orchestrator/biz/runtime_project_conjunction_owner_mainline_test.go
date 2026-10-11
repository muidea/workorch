package orchestrator

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"reflect"
	"strings"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	shellcommon "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/common"
	shellevents "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/events"
	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// provider 为脚本替身；准备、评审输入、dispatch、File/Shell 执行、文件型 Run CAS、
// 原回执查询与 Workspace 目录读取均使用现有真实 owner。不是公开 authoring 或 fresh-process 验收。
func TestDriveRuntimeProjectConjunctionOwnerMainline(t *testing.T) {
	options := validationInputReviewMainlineOptions{Adapter: shellcommon.ValidationRecipeAdapterProjectConjunction, ValidationCount: 1, ProtocolRef: runtimecontract.RuntimeValidationInputReviewProtocolV2, SourceObligations: true}
	baseline := runValidationInputReviewMainline(t, "consistent", "", options)
	t.Run("original-project-failure", func(t *testing.T) {
		runValidationInputReviewMainline(t, "project_history_failure", "", options)
	})
	t.Run("restart-after-review-binding", func(t *testing.T) {
		if calls := runValidationInputReviewMainline(t, "consistent", "after_review_binding", options); calls != baseline {
			t.Fatalf("restart repeated provider consumption: %d != %d", calls, baseline)
		}
	})
}

func assertProjectConjunctionPersistedOwnerOutputs(t *testing.T, ctx context.Context, hub magicevent.Hub, module *Biz, record runtimecontract.RuntimeStateRecord, success bool) {
	t.Helper()
	before, err := module.loadRuntime(ctx, record.Facts.WorkspaceID, record.Facts.RunID)
	if err != nil || before.Validate() != nil {
		t.Fatalf("original persisted aggregate invalid: %v", err)
	}
	count := 0
	for _, receipt := range record.OperationReceipts {
		observation := receipt.Evidence.Validation
		if observation == nil || observation.ObservationProtocolRef != "shell/go-test-json/project-conjunction-behavior/v1" {
			continue
		}
		count++
		if strings.Contains(receipt.Evidence.Summary, "stdout=") || strings.Contains(receipt.Evidence.Summary, "stderr=") || strings.Contains(receipt.Evidence.Summary, "原工程判据失败") {
			t.Fatal("original raw output leaked into general Runtime summary")
		}
		conjunction := observation.ExecutionConjunction
		if conjunction == nil || conjunction.Validate() != nil || conjunction.Complete != success {
			t.Fatal("persisted owner receipt lost actual all-of facts")
		}
		if success {
			if len(conjunction.Stages) != 4 || receipt.Outcome != runtimecontract.RuntimeOperationOutcomeAccepted {
				t.Fatal("accepted original receipt omitted a required stage")
			}
		} else if len(conjunction.Stages) != 2 || !conjunction.Stages[1].KnownFailure || conjunction.Stages[1].ExitCode == 0 || receipt.Outcome != runtimecontract.RuntimeOperationOutcomeRejected || len(observation.PassedChecks) != 0 {
			t.Fatal("original project failure disappeared or supplied current CWI proof")
		}
		var dispatchID string
		var original *runtime.RuntimeOperationExecutionRequest
		for _, dispatch := range record.CompletedDispatches {
			for _, execution := range dispatch.OperationExecutions {
				if execution.ExecutionKey == receipt.ExecutionKey && execution.Fence == receipt.Fence && execution.OwnerReceiptHash == runtimecontract.StableRuntimeOperationReceiptHash(receipt) {
					dispatchID = dispatch.ID
					for _, catalog := range dispatch.OperationAdmissibilities {
						if catalog.SnapshotID != execution.AdmissibilitySnapshotID || catalog.AcquisitionBinding != execution.AcquisitionBinding || catalog.OwnerModuleID != runtimecontract.RuntimeModuleShell {
							continue
						}
						for _, observed := range dispatch.OperationAdmissibilityObservations {
							if observed.ContentHash != catalog.SnapshotID || observed.PrerequisiteID != catalog.PrerequisiteID || observed.OwnerModuleID != catalog.OwnerModuleID {
								continue
							}
							for _, entry := range dispatch.Candidate.Validation.Operations {
								if entry.Operation.ID != receipt.OperationID {
									continue
								}
								for _, surface := range catalog.ValidationSurfaces {
									for _, binding := range surface.ProofRequirementBindings {
										if binding.ValidationRecipeID != observation.RecipeID {
											continue
										}
										catalogCopy, observationCopy := catalog.Clone(), observed.Normalized()
										original = &runtime.RuntimeOperationExecutionRequest{WorkspaceID: record.Facts.WorkspaceID, RunID: record.Facts.RunID, DispatchID: dispatch.ID, Transaction: dispatch.Transaction, Operation: entry.Operation, Execution: execution, EvidenceKind: runtimecontract.EvidenceKindValidation, TargetRole: dispatch.FocusTargetRole, EvidencePolicies: record.Facts.Activation.EvidencePolicies, ValidationRecipes: dispatch.ValidationRecipes, ValidationScopeTargetRefs: catalog.TargetRefs, ValidationProofBinding: binding.Clone(), OwnerCatalog: &catalogCopy, OwnerCatalogObservation: &observationCopy}
									}
								}
							}
						}
					}
					for _, binding := range dispatch.CoverageBindings {
						proof, found := runtimecontract.CoverageEvidenceProofForKind(binding, runtimecontract.EvidenceKindValidation)
						if !found {
							continue
						}
						for _, check := range proof.ValidationCheckAttestations {
							if check.ExecutionStagesHash != conjunction.RequiredStagesHash || success && !runtimecontract.RuntimeValidationAttestationPassed(*observation, check) {
								t.Fatal("actual receipt did not satisfy exact sealed stage mapping")
							}
						}
					}
				}
			}
		}
		if dispatchID == "" || original == nil {
			t.Fatal("receipt has no original persisted execution authority")
		}
		duplicate, err := (eventRuntimeOperationExecutor{hub: hub}).ExecuteRuntimeOperation(ctx, *original)
		if err != nil || !runtimecontract.SameRuntimeOperationReceipt(duplicate.Receipt, receipt) {
			t.Fatalf("actual owner duplicate did not replay original receipt: %v", err)
		}
		facts := record.Facts.Evidence.Accepted
		if !success {
			facts = record.Facts.Evidence.Rejected
		}
		matched := false
		for _, fact := range facts {
			if fact.DispatchID == dispatchID && fact.OperationID == receipt.OperationID && reflect.DeepEqual(fact.Validation, observation) {
				matched = true
			}
		}
		if !matched {
			t.Fatal("judged EvidenceFact lost original conjunction/storage identity")
		}
		for _, stage := range conjunction.Stages {
			for _, stream := range []string{"stdout", "stderr"} {
				identity := stage.Stdout
				if stream == "stderr" {
					identity = stage.Stderr
				}
				command := shellevents.QueryRuntimeValidationRawOutputCommand{WorkspaceID: record.Facts.WorkspaceID, RunID: record.Facts.RunID, DispatchID: dispatchID, OperationID: receipt.OperationID, ExecutionKey: receipt.ExecutionKey, Fence: receipt.Fence, SourceReceiptHash: runtimecontract.StableRuntimeOperationReceiptHash(receipt), StageID: stage.Identity.ID, Stream: stream}
				response := hub.Send(magicevent.NewEventWithContext(shellevents.TopicQueryRuntimeValidationRawOutput, ModuleID, runtimecontract.RuntimeModuleShell, magicevent.NewHeader(), ctx, command))
				if response == nil {
					t.Fatal("missing actual owner raw response")
				}
				data, err := response.Get()
				value, ok := data.(shellevents.QueryRuntimeValidationRawOutputResult)
				if err != nil || !ok || value.ValidateFor(command) != nil || !reflect.DeepEqual(value.Identity, identity) {
					t.Fatalf("actual raw identity mismatch: %v", err)
				}
				digest := sha256.Sum256(value.Raw)
				if int64(len(value.Raw)) != identity.Bytes || hex.EncodeToString(digest[:]) != identity.SHA256 {
					t.Fatal("actual stage output bytes/hash were not preserved")
				}
				command.Fence++
				if rejected := hub.Send(magicevent.NewEventWithContext(shellevents.TopicQueryRuntimeValidationRawOutput, ModuleID, runtimecontract.RuntimeModuleShell, magicevent.NewHeader(), ctx, command)); rejected == nil || rejected.Error() == nil {
					t.Fatal("wrong original fence read raw output")
				}
			}
		}
	}
	if count != 1 {
		t.Fatalf("expected one actual conjunction receipt, got %d", count)
	}
	after, err := module.loadRuntime(ctx, record.Facts.WorkspaceID, record.Facts.RunID)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("raw queries changed persisted Runtime authority or budget")
	}
}
