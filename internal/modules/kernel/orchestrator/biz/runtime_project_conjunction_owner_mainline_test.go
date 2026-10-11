package orchestrator

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"reflect"
	"strings"
	"sync"
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
	var failureBaseline int64
	var failureOnce sync.Once
	failureCalls := func(t *testing.T) int64 {
		t.Helper()
		failureOnce.Do(func() { failureBaseline = runValidationInputReviewMainline(t, "project_history_failure", "", options) })
		return failureBaseline
	}
	t.Run("original-project-failure", func(t *testing.T) { failureCalls(t) })
	t.Run("restart-after-review-binding", func(t *testing.T) {
		if calls := runValidationInputReviewMainline(t, "consistent", "after_review_binding", options); calls != baseline {
			t.Fatalf("restart repeated provider consumption: %d != %d", calls, baseline)
		}
	})
	t.Run("receipt-reply-loss", func(t *testing.T) {
		if calls := runValidationInputReviewMainline(t, "consistent", "after_project_receipt", options); calls != baseline {
			t.Fatalf("原 receipt 恢复重复 provider 消费: %d != %d", calls, baseline)
		}
	})
	t.Run("failed-receipt-reply-loss", func(t *testing.T) {
		expected := failureCalls(t)
		if calls := runValidationInputReviewMainline(t, "project_history_failure", "after_project_receipt", options); calls != expected {
			t.Fatalf("原 rejected receipt 恢复重复 provider 消费: %d != %d", calls, expected)
		}
	})
}

func assertProjectReceiptReplyLossBoundary(t *testing.T, aggregate runtime.RuntimeAggregate, success bool) runtimecontract.RuntimeOperationReceipt {
	t.Helper()
	if aggregate.Validate() != nil || aggregate.ActiveDispatch == nil || len(aggregate.ActiveDispatch.OperationExecutions) != 1 {
		t.Fatal("receipt 响应丢失前像不完整")
	}
	execution := aggregate.ActiveDispatch.OperationExecutions[0]
	receipt, found := runtimecontract.RuntimeOperationReceiptForExecutionKeyFence(aggregate.OperationReceipts, execution.ExecutionKey, execution.Fence)
	if !found || execution.State != runtimecontract.OperationExecutionOwnerCommitted || execution.OwnerReceiptHash != runtimecontract.StableRuntimeOperationReceiptHash(receipt) || receipt.Evidence.Validation == nil || receipt.Evidence.Validation.ExecutionConjunction == nil {
		t.Fatal("actual v6 receipt 尚未持久化或缺少原 owner 合取")
	}
	conjunction := receipt.Evidence.Validation.ExecutionConjunction
	if success {
		if receipt.Outcome != runtimecontract.RuntimeOperationOutcomeAccepted || !conjunction.Complete || len(conjunction.Stages) != 4 {
			t.Fatal("成功 receipt 丢失完整四阶段事实")
		}
	} else if receipt.Outcome != runtimecontract.RuntimeOperationOutcomeRejected || conjunction.Complete || len(conjunction.Stages) != 2 || !conjunction.Stages[1].KnownFailure || conjunction.Stages[1].ExitCode == 0 || len(receipt.Evidence.Validation.PassedChecks) != 0 {
		t.Fatal("失败 receipt 丢失原工程判据或产生当前 proof")
	}
	for _, facts := range [][]runtimecontract.EvidenceFact{aggregate.Facts.Evidence.Accepted, aggregate.Facts.Evidence.Rejected} {
		for _, fact := range facts {
			if fact.DispatchID == aggregate.ActiveDispatch.ID && fact.OperationID == execution.OperationID {
				t.Fatal("receipt 响应丢失切点已经授予 SM-2 evidence")
			}
		}
	}
	for _, coverage := range aggregate.Facts.Coverage {
		for _, binding := range aggregate.ActiveDispatch.CoverageBindings {
			if coverage.ID == binding.CoverageWorkItemID && (coverage.State == runtimecontract.RuntimeCoverageItemStateSatisfied || coverage.CWIConvergenceState == runtimecontract.RuntimeCWIConvergenceStateSettled || len(coverage.EvidenceRefs) != 0) {
				t.Fatal("receipt 尚未裁决却提前授予 CWI credit")
			}
		}
	}
	assertProjectReceiptBudgetConsumption(t, aggregate, receipt)
	if execution.EvidenceFactID != "" || aggregate.Facts.Recovery.OperationRecoveryAttempts != 0 {
		t.Fatal("receipt 持久化切点错误推进 evidence 或 recovery budget")
	}
	return receipt
}

func assertProjectReceiptBudgetConsumption(t *testing.T, aggregate runtime.RuntimeAggregate, receipt runtimecontract.RuntimeOperationReceipt) {
	t.Helper()
	allocations, consumptions := 0, 0
	for _, allocation := range aggregate.ExecutionBudget.Allocations {
		if allocation.Dimension == runtimecontract.RuntimeBudgetDimensionCapabilityOperation && allocation.LogicalConsumptionID == receipt.ExecutionKey {
			allocations++
		}
	}
	for _, consumption := range aggregate.ExecutionBudget.Consumptions {
		if consumption.LogicalConsumptionID != receipt.ExecutionKey {
			continue
		}
		consumptions++
		if consumption.Owner != runtimecontract.RuntimeModuleShell || consumption.OwnerResultHash != runtimecontract.StableRuntimeOperationReceiptHash(receipt) || consumption.ConfirmedAmount != 1 || consumption.UnknownAmount != 0 {
			t.Fatal("原 receipt 的实际 operation 消费绑定错误")
		}
	}
	if allocations != 1 || consumptions != 1 {
		t.Fatalf("原 receipt 重复分配或扣费: allocations=%d consumptions=%d", allocations, consumptions)
	}
}

func assertProjectReceiptPreservedAfterRecovery(t *testing.T, before, after runtime.RuntimeAggregate, original runtimecontract.RuntimeOperationReceipt) {
	t.Helper()
	replayed, found := runtimecontract.RuntimeOperationReceiptForExecutionKeyFence(after.OperationReceipts, original.ExecutionKey, original.Fence)
	if !found || !runtimecontract.SameRuntimeOperationReceipt(original, replayed) || len(before.OperationReceipts) != len(after.OperationReceipts) {
		t.Fatal("恢复替换或重复生成原 owner receipt")
	}
	assertProjectReceiptBudgetConsumption(t, after, replayed)
	if !reflect.DeepEqual(before.Facts.Recovery, after.Facts.Recovery) || !reflect.DeepEqual(before.OperationRecoveryAuthorizations, after.OperationRecoveryAuthorizations) {
		t.Fatal("原 committed receipt 恢复错误消费 recovery budget")
	}
	for _, consumption := range before.ExecutionBudget.Consumptions {
		found := false
		for _, current := range after.ExecutionBudget.Consumptions {
			if current.ConsumptionID == consumption.ConsumptionID {
				found = reflect.DeepEqual(consumption, current)
			}
		}
		if !found {
			t.Fatal("恢复改写或清除既有 confirmed／unknown 消费事实")
		}
	}
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
