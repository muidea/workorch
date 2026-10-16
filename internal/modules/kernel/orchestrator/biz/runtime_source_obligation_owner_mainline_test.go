package orchestrator

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	resultmodule "github.com/muidea/workorch/internal/modules/blocks/result"
	resultbiz "github.com/muidea/workorch/internal/modules/blocks/result/biz"
	shellcommon "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/common"
	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	filerepository "github.com/muidea/workorch/internal/pkg/repository/file"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func withSharedSourceObligationBootstrap(t *testing.T, bootstrap runtimecontract.RuntimeBootstrap) runtimecontract.RuntimeBootstrap {
	t.Helper()
	item := bootstrap.CoverageItems[1]
	inventory := (runtimecontract.RuntimeSourceObligationInventory{
		SchemaVersion: runtimecontract.RuntimeSourceObligationInventorySchemaCurrent, CandidateHash: "candidate-mainline", SpecificationHash: "specification-mainline", DefinitionRevision: bootstrap.Definition.Revision, StartupInputHash: bootstrap.Definition.StartupInputHash,
		Sources: []runtimecontract.RuntimeDefinitionSource{
			{CandidateItemRef: "goal-source", ItemRef: "formal-goal", SourceScope: "goal", SourceID: "goal", Text: bootstrap.Definition.GoalDefinition},
			{CandidateItemRef: "dod-source", ItemRef: "formal-dod", SourceScope: item.SourceScope, SourceID: item.SourceID, Text: item.TargetText},
		},
		Requirements: []runtimecontract.RuntimeSourceRequirement{{ID: "source-requirement", DimensionID: "source-dimension", Required: true, SourceKind: "USER_EXPLICIT", SourceItemRefs: []string{"dod-source"}, Text: "验证实际交付行为"}},
		Obligations: []runtimecontract.RuntimeSourceObligation{
			{ID: "source-A", RequirementID: "source-requirement", DoDItemRefs: []string{"dod-source"}, Text: "构建并调用实际交付程序"},
			{ID: "source-B", RequirementID: "source-requirement", DoDItemRefs: []string{"dod-source"}, Text: "独立验证实际交付输出为 4"},
		},
		Verifications: []runtimecontract.RuntimeSourceVerification{
			{ID: "verify-source-A", ObligationID: "source-A", Declaration: "实际构建执行，不用源码标记", ProfileID: "mainline-profile", EvidenceRequirementID: item.SourceID, Scope: item.SourceScope},
			{ID: "verify-source-B", ObligationID: "source-B", Declaration: "断言真实进程响应", ProfileID: "mainline-profile", EvidenceRequirementID: item.SourceID, Scope: item.SourceScope},
		},
	}).WithHash()
	checks, err := runtimecontract.CompileRuntimeSourceObligationChecks(inventory, item)
	if err != nil {
		t.Fatal(err)
	}
	bootstrap.Definition.SourceObligations = &inventory
	bootstrap.CoverageItems[1].SourceObligationChecks = checks
	bootstrap = runtimeTestBootstrapWithBindings(bootstrap)
	if err := bootstrap.Validate(); err != nil {
		t.Fatal(err)
	}
	return bootstrap
}

func assertSharedSourceObligationOwnerObservation(t *testing.T, aggregate runtime.RuntimeAggregate, requirement runtimecontract.RuntimeValidationProofRequirement, failedB bool) {
	t.Helper()
	evidence := aggregate.Facts.Evidence.Accepted
	if failedB {
		evidence = aggregate.Facts.Evidence.Rejected
	}
	found := false
	for _, fact := range evidence {
		if fact.Kind != runtimecontract.EvidenceKindValidation || fact.Validation == nil {
			continue
		}
		for _, dispatch := range aggregate.CompletedDispatches {
			if dispatch.ID != fact.DispatchID {
				continue
			}
			for _, binding := range dispatch.CoverageBindings {
				if binding.CoverageWorkItemID != requirement.CoverageWorkItemID {
					continue
				}
				proof, ok := runtimecontract.CoverageEvidenceProofForKind(binding, runtimecontract.EvidenceKindValidation)
				if !ok || proof.ValidationProofRequirementHash != requirement.Hash {
					continue
				}
				for index, source := range requirement.SourceObligationChecks {
					native := ""
					for _, attestation := range proof.ValidationCheckAttestations {
						if attestation.RequiredCheckRef == source.CheckRef {
							if native != "" {
								t.Fatal("源 check 存在重复 owner 映射")
							}
							native = attestation.NativeCheckRef
						}
					}
					if native == "" || !sourceObservationContains(fact.Validation.ExecutedChecks, native) {
						t.Fatal("实际 owner 未执行完整逐源 check")
					}
					if failedB && index == 1 {
						if !sourceObservationContains(fact.Validation.FailedChecks, native) || sourceObservationContains(fact.Validation.PassedChecks, native) {
							t.Fatal("B 实际失败未保留 exact owner observation")
						}
					} else if !sourceObservationContains(fact.Validation.PassedChecks, native) || sourceObservationContains(fact.Validation.FailedChecks, native) {
						t.Fatal("A/B 实际通过未保留独立 owner observation")
					}
				}
				found = true
			}
		}
	}
	if !found {
		t.Fatal("未取得独立于 provider assessment 的实际 owner 逐源 observation")
	}
}

func sourceObservationContains(values []string, ref string) bool {
	for _, value := range values {
		if value == ref {
			return true
		}
	}
	return false
}

func restartSourceObligationTerminalDelivery(t *testing.T, ctx context.Context, module *Biz, completed runtime.RuntimeAggregate, firstResultOwner *resultmodule.Module, stopResources func(), fired func() bool, cut, root string) (*Biz, repository.ResultRepository) {
	t.Helper()
	if err := module.deliverRuntimeTerminal(ctx, completed); err == nil || !fired() {
		t.Fatalf("未到达真实 owner 持久化后的响应丢失切点 %s: %v", cut, err)
	}
	results := filerepository.NewResultRepository(filepath.Join(root, "results.json"))
	beforeResults, err := results.ListByRun(ctx, "run-v2")
	if err != nil || len(beforeResults) != 1 || beforeResults[0].RuntimeCloseout == nil || !reflect.DeepEqual(beforeResults[0].RuntimeCloseout.SourceObligations, completed.Facts.DefinitionSeed.SourceObligations) || len(beforeResults[0].RuntimeCloseout.RequirementOutcomes) != len(completed.Facts.DefinitionSeed.CoverageItems) {
		t.Fatalf("独立读取未取得完整逐源 closeout: %+v err=%v", beforeResults, err)
	}
	runs := filerepository.NewRunRepository(filepath.Join(root, "runs.json"))
	terminalState := common.RunStateCompleted
	if completed.Decision.State != runtimecontract.ConvergenceStateComplete {
		terminalState = common.RunStateFailed
	}
	wantRun, wantACKs := common.RunStateActive, 0
	if cut == "after_workspace_persistence" {
		wantACKs = 1
	}
	if cut == "after_run_persistence" {
		wantRun, wantACKs = terminalState, 2
	}
	persisted, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil || persisted.RuntimeState == nil || persisted.State != wantRun {
		t.Fatalf("响应未知时 Run 偏离已落盘切点: %+v err=%v", persisted, err)
	}
	if cut != "after_run_persistence" && persisted.TerminalResultID != "" || cut == "after_run_persistence" && persisted.TerminalResultID != beforeResults[0].ID {
		t.Fatal("Run Result 身份偏离 owner 已确认材料")
	}
	sealed := runtimeAggregateFromRecord(*persisted.RuntimeState)
	if err := sealed.Validate(); err != nil || sealed.TerminalMaterialSeal == nil || len(sealed.DeliveryAcknowledgements) != wantACKs {
		t.Fatalf("响应丢失后 seal/ACK 前像不完整: %v", err)
	}
	if !reflect.DeepEqual(sealed.TerminalMaterialSeal.Material.RuntimeCloseout, beforeResults[0].RuntimeCloseout) {
		t.Fatal("Result 持久材料偏离首次独立 seal")
	}
	workspaces := filerepository.NewWorkspaceRepository(filepath.Join(root, "workspaces.json"))
	workspace, err := workspaces.Get(ctx, "ws-v2")
	if err != nil {
		t.Fatal(err)
	}
	if cut == "after_result_persistence" {
		if workspace.ActiveRunID != "run-v2" || workspace.TerminalDelivery != nil {
			t.Fatal("Result 未确认时 Workspace 提前释放")
		}
	} else if workspace.ActiveRunID != "" || workspace.TerminalDelivery == nil || *workspace.TerminalDelivery != sealed.TerminalMaterialSeal.Identity {
		t.Fatal("Workspace 交接身份未在 ACK 丢失前独立持久化")
	}
	if err := firstResultOwner.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	stopResources()

	// 重建全部交付 owner 与 Hub，只从独立 file-backed 前像恢复，不重跑 provider 或 capability。
	hub := magicevent.NewHub(32)
	t.Cleanup(func() { hub.Terminate(ctx) })
	startTerminalDeliveryResourceOwners(t, ctx, hub, runs, workspaces, workspaces)
	resultOwner := resultmodule.New()
	if err := resultOwner.Configure(resultbiz.Dependencies{OwnerRepositories: testResultOwnerRepositories(results, filerepository.NewArtifactRepository(filepath.Join(root, "artifacts.json")), memory.NewResultWorkspaceDeletionRepository()), EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := resultOwner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := resultOwner.Stop(ctx); err != nil {
			t.Fatal(err)
		}
	})
	restarted := newRuntimeProjectionTestModule(t, hub)
	for range 2 {
		if err := restarted.deliverRuntimeTerminal(ctx, sealed); err != nil {
			t.Fatal(err)
		}
	}
	afterResults, err := results.ListByRun(ctx, "run-v2")
	if err != nil || len(afterResults) != 1 || !reflect.DeepEqual(beforeResults, afterResults) {
		t.Fatalf("恢复或重复交付改写／重复创建 Result: %v", err)
	}
	afterRun, err := runs.GetByWorkspaceID(ctx, "ws-v2", "run-v2")
	if err != nil || afterRun.RuntimeState == nil || afterRun.State != terminalState || afterRun.TerminalResultID != beforeResults[0].ID {
		t.Fatalf("恢复未封存 owner-derived Result 身份: %+v err=%v", afterRun, err)
	}
	after := runtimeAggregateFromRecord(*afterRun.RuntimeState)
	if err := after.Validate(); err != nil || !runtimeTerminalDeliveryFullyAcknowledged(after) || !reflect.DeepEqual(sealed.TerminalMaterialSeal, after.TerminalMaterialSeal) {
		t.Fatalf("恢复未保留原 seal 和完整有序 ACK: %v", err)
	}
	if len(after.DeliveryAcknowledgements) != 3 {
		t.Fatal("恢复没有恰好三个 owner ACK")
	}
	for index, owner := range []runtimecontract.RuntimeDeliveryOwner{runtimecontract.RuntimeDeliveryOwnerResult, runtimecontract.RuntimeDeliveryOwnerWorkspace, runtimecontract.RuntimeDeliveryOwnerRun} {
		if after.DeliveryAcknowledgements[index].Owner != owner {
			t.Fatal("恢复改变 Result→Workspace→Run ACK 顺序")
		}
	}
	workspace, err = workspaces.Get(ctx, "ws-v2")
	if err != nil || workspace.ActiveRunID != "" || workspace.TerminalDelivery == nil || !reflect.DeepEqual(workspace.TerminalDelivery, afterRun.TerminalDelivery) {
		t.Fatalf("Workspace 与 Run 的最终交接身份不一致: %+v err=%v", workspace, err)
	}
	return restarted, results
}

// provider 为脚本替身；File/Shell 执行、Run CAS 与三 owner 终态交接使用既有真实装配。
func TestDriveRuntimeSharedSourceObligationOwnerMainline(t *testing.T) {
	options := validationInputReviewMainlineOptions{Adapter: shellcommon.ValidationRecipeAdapterConjunctiveEntrypoint, ValidationCount: 1, ProtocolRef: runtimecontract.RuntimeValidationInputReviewProtocolV2, SourceObligations: true}
	baseline := runValidationInputReviewMainline(t, "consistent", "", options)
	t.Run("B actual failure", func(t *testing.T) {
		runValidationInputReviewMainline(t, "source_B_failure", "", options)
	})
	t.Run("B failure after Result persistence", func(t *testing.T) {
		runValidationInputReviewMainline(t, "source_B_failure", "after_result_persistence", options)
	})
	for _, cut := range []string{"after_result_persistence", "after_workspace_persistence", "after_run_persistence"} {
		t.Run(cut, func(t *testing.T) {
			if calls := runValidationInputReviewMainline(t, "consistent", cut, options); calls != baseline {
				t.Fatalf("source terminal recovery repeated provider consumption: %d != %d", calls, baseline)
			}
		})
	}
	t.Run("restart after review binding", func(t *testing.T) {
		if calls := runValidationInputReviewMainline(t, "consistent", "after_review_binding", options); calls != baseline {
			t.Fatalf("source restart repeated provider consumption: %d != %d", calls, baseline)
		}
	})
}
