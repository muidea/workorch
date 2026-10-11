package shell

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func projectExecutionFixture(t *testing.T, failed bool) (RuntimeCommandRequest, runtimecontract.RuntimeOperationExecutionContext) {
	t.Helper()
	root, internal := t.TempDir(), t.TempDir()
	recipe := isolatedInputRecipe()
	requirement := shellValidationProofRequirementForTest(t, recipe.ID, "execution-cwi")
	checks, ok := shellValidationCheckAttestations(recipe, requirement)
	if !ok {
		t.Fatal("缺少 exact mapping")
	}
	target := shellValidationNativeCheckTargetRef(requirement, checks...)
	files := map[string]string{
		"go.mod":            "module fixture\ngo 1.26\n",
		"delivery/value.go": "package delivery\nfunc Value() int {return 4}\n",
		target:              fmt.Sprintf("package checks\nimport (\"testing\"; delivery \"fixture/delivery\")\nfunc %s(t *testing.T){if delivery.Value()!=4{t.Fatal(\"wrong\")}}\n", checks[0].NativeCheckRef),
	}
	if failed {
		files["workorch_validation/v3/old_test.go"] = "package old\nimport \"testing\"\nfunc TestOld(t *testing.T){t.Fatal(\"原已知失败\")}\n"
	}
	var refs []string
	for ref, body := range files {
		writeNativeStructureSource(t, root, ref, body)
		refs = append(refs, ref)
	}
	request := RuntimeCommandRequest{WorkspaceID: "ws", RunID: "run", StepID: "dispatch/op", WorkspaceDeliverableDirectory: root, WorkspaceInternalDirectory: internal,
		ValidationRecipeID: recipe.ID, Command: recipe.Command, Args: recipe.Args, TimeoutMillis: 60000, TargetRefs: []string{target}, ValidationScopeTargetRefs: refs, ValidationCheckAttestations: checks,
		ManagedValidation: runtimecontract.RuntimeManagedValidation{RuntimeManaged: true, Kind: runtimecontract.RuntimeBlockKindManagedValidationSession}}
	if err := instrumentManagedValidationNativeChecks(&request); err != nil {
		t.Fatal(err)
	}
	execution := runtimecontract.RuntimeOperationExecutionContext{WorkspaceID: request.WorkspaceID, RunID: request.RunID, DispatchID: "dispatch", OperationID: "op", WorkspaceDeliverableDirectory: root, WorkspaceInternalDirectory: internal,
		Execution: runtimecontract.RuntimeOperationExecutionEnvelope{ExecutionKey: runtimecontract.StableRuntimeOperationExecutionKey(request.WorkspaceID, request.RunID, "dispatch", "op"), Fence: 1, OperationHash: "private-fixture-hash", AuthorityHash: "private-fixture-authority", ExpectedEffectScope: runtimecontract.RuntimeEffectEphemeralSession, IdempotencyClass: runtimecontract.RuntimeOperationIdempotencyAtMostOnce}}
	return request, execution
}

// 真实执行封套与 fresh-store 读取，不代替公开 seal/receipt 或 fresh-process 验收。
func TestProjectExecutionEnvelopePreservesFactsRawScopeAndCleanup(t *testing.T) {
	for _, failed := range []bool{false, true} {
		t.Run(fmt.Sprintf("known_failure=%t", failed), func(t *testing.T) {
			request, execution := projectExecutionFixture(t, failed)
			result, err := NewRuntimeAdapter().executeShellProjectConjunction(context.Background(), request, execution)
			if failed {
				if err == nil || result.Conjunction == nil || result.Conjunction.Complete || len(result.Conjunction.Stages) != 2 || !result.Conjunction.Stages[1].KnownFailure || result.Conjunction.Stages[1].ExitCode == 0 || result.Result.ExitCode == 0 {
					t.Fatalf("原已知失败丢失：%+v err=%v", result, err)
				}
			} else if err != nil || result.Conjunction == nil || !result.Conjunction.Complete || len(result.Conjunction.Stages) != 4 || result.Result.Evidence.Quality != common.EvidenceQualityValid {
				t.Fatalf("真实完整封套失败：%+v err=%v", result, err)
			}
			if result.Conjunction.Validate() != nil || !result.Result.ManagedValidation.ResourcesReleased || len(result.Raw) != len(result.Conjunction.Stages) || result.FinishedAt.After(result.Deadline) || result.Deadline.After(result.StartedAt.Add(DefaultCommandTimeout)) {
				t.Fatal("合取事实、清理或原 deadline 不成立")
			}
			for _, resource := range result.Result.ValidationResources {
				if resource.Status != "released" || resource.Kind != "scratch_dir" {
					t.Fatalf("scratch 资源未释放：%+v", resource)
				}
				if _, err := os.Stat(resource.Path); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("scratch 仅报告清理而实际尚存")
				}
			}
			store, err := openShellProjectRawStore(request.WorkspaceInternalDirectory)
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			for index, refs := range result.Raw {
				for stream, ref := range map[string]string{"stdout": refs.Stdout, "stderr": refs.Stderr} {
					scope := shellProjectRawScope{WorkspaceID: request.WorkspaceID, RunID: request.RunID, ExecutionKey: execution.Execution.ExecutionKey, StageID: refs.StageID, Stream: stream}
					raw, err := store.Read(context.Background(), scope, ref)
					identity := result.Conjunction.Stages[index].Stdout
					if stream == "stderr" {
						identity = result.Conjunction.Stages[index].Stderr
					}
					if identity.Storage == nil || identity.Storage.Reference != ref || identity.Storage.OmittedReason != "" {
						t.Fatal("持久执行事实未绑定原 owner raw 引用")
					}
					identity.Storage = nil
					if err != nil || shellProjectOutputIdentity(raw) != identity {
						t.Fatalf("原执行输出身份丢失：%v", err)
					}
					scope.ExecutionKey = runtimecontract.StableRuntimeOperationExecutionKey(request.WorkspaceID, request.RunID, "other", "op")
					if _, err := store.Read(context.Background(), scope, ref); err == nil {
						t.Fatal("原文引用跨 execution 读取")
					}
				}
			}
			if len(result.Result.validationCopiedInputs) == 0 {
				t.Fatal("执行封套丢失实际 copied input dependencies")
			}
			if _, err := os.Stat(filepath.Join(request.WorkspaceDeliverableDirectory, "delivery/value.go")); err != nil {
				t.Fatal("清理删除了权威 workspace")
			}
		})
	}
}

func TestProjectExecutionEnvelopeKeepsEarlierParentDeadlineAndExplicitRawOmission(t *testing.T) {
	request, execution := projectExecutionFixture(t, false)
	deadline := time.Now().Add(100 * time.Millisecond)
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()
	result, err := NewRuntimeAdapter().executeShellProjectConjunction(ctx, request, execution)
	if err == nil || !result.Deadline.Equal(deadline) || result.Conjunction != nil && result.Conjunction.Complete || result.Result.Evidence.Quality == common.EvidenceQualityValid {
		t.Fatal("较早 parent deadline 被重置或超期完成 proof")
	}
	for _, refs := range result.Raw {
		if (refs.Stdout == "" || refs.Stderr == "") && refs.OmittedReason == "" {
			t.Fatal("期限内未保存原文却未显式报告 omission")
		}
	}
	for _, resource := range result.Result.ValidationResources {
		if resource.Status != "released" {
			t.Fatal("取消后未真实清理 scratch")
		}
		if _, err := os.Stat(resource.Path); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("取消后 scratch 仍存在")
		}
	}
}

func TestProjectExecutionEnvelopeRejectsInvalidScopeAndBudgetBeforeScratch(t *testing.T) {
	request, execution := projectExecutionFixture(t, false)
	for _, scenario := range []string{"workspace", "run", "execution", "directory", "internal", "effect", "scratch", "timeout", "cancelled"} {
		t.Run(scenario, func(t *testing.T) {
			changed, scope := request, execution
			ctx := context.Background()
			switch scenario {
			case "workspace":
				changed.WorkspaceID = "other"
			case "run":
				changed.RunID = "other"
			case "execution":
				scope.Execution.ExecutionKey = "other"
			case "directory":
				changed.WorkspaceDeliverableDirectory = t.TempDir()
			case "internal":
				changed.WorkspaceInternalDirectory = t.TempDir()
			case "effect":
				scope.Execution.ExpectedEffectScope = runtimecontract.RuntimeEffectWorkspaceDurable
			case "scratch":
				changed.ValidationScratchDirectory = t.TempDir()
			case "timeout":
				changed.TimeoutMillis = 120000
			case "cancelled":
				cancelled, cancel := context.WithCancel(ctx)
				cancel()
				ctx = cancelled
			}
			result, err := NewRuntimeAdapter().executeShellProjectConjunction(ctx, changed, scope)
			if err == nil || result.Conjunction != nil || len(result.Raw) != 0 || len(result.Result.ValidationResources) != 0 {
				t.Fatal("非法 scope/budget 仍进入实际执行")
			}
		})
	}
}

func TestProjectExecutionCompletionKeepsKnownFailureAndRejectsCleanupDeadlineOrRawGap(t *testing.T) {
	for _, scenario := range []string{"cleanup", "deadline", "raw-gap", "known-failure"} {
		t.Run(scenario, func(t *testing.T) {
			known := errors.New("原已知进程失败")
			cleanup := errors.New("真实清理失败")
			started := time.Now().UTC()
			source, err := shellProjectSourceSetHash(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			specs := shellProjectFixedStages([]string{"test", "-json", "-count=1", "./exact"}, "exact")
			var stages []shellProjectStageFact
			var refs []shellProjectStageRawReferences
			for index, spec := range specs {
				at := started.Add(time.Duration(index) * time.Millisecond)
				stages = append(stages, shellProjectStageFact{ID: spec.ID, Scope: spec.Scope, Argv: append([]string{"go"}, spec.Args...), Started: true, StartedAt: at.Format(time.RFC3339Nano), FinishedAt: at.Format(time.RFC3339Nano), Stdout: newShellProjectRawBuffer().fact(), Stderr: newShellProjectRawBuffer().fact(), SourceBefore: source, SourceAfter: source, TerminalComplete: true, Tests: 1, PassedTests: 1})
				refs = append(refs, shellProjectStageRawReferences{StageID: spec.ID, Stdout: source, Stderr: source})
			}
			conjunction, err := shellProjectConjunctionObservation(sealShellProjectConjunction(stages, specs, []string{"delivery.go"}), specs)
			if err != nil || !conjunction.Complete {
				t.Fatal("反例前像不是完整有效合取")
			}
			result := shellProjectExecutionResult{StartedAt: started, FinishedAt: time.Now(), Conjunction: &conjunction, Raw: refs,
				Result: RuntimeCommandResult{Status: "completed", ExitCode: 0, EvidenceQuality: string(common.EvidenceQualityValid), ValidationObservation: &runtimecontract.RuntimeValidationObservation{ProofClass: runtimecontract.RuntimeValidationProofExecuted}}}
			var executeErr, cleanupErr, contextErr error
			switch scenario {
			case "cleanup":
				cleanupErr = cleanup
			case "deadline":
				contextErr = context.DeadlineExceeded
			case "raw-gap":
				result.Raw[2].Stderr = ""
			case "known-failure":
				executeErr, cleanupErr, contextErr = known, cleanup, context.DeadlineExceeded
				result.Result.ExitCode, result.Result.FailureCode = 7, "original_known_failure"
			}
			err = finishShellProjectExecution(&result, executeErr, cleanupErr, contextErr)
			if err == nil || result.Conjunction.Complete || result.Result.Evidence.Quality == common.EvidenceQualityValid || result.Result.ValidationObservation.ProofClass != "" {
				t.Fatal("未完整 cleanup/deadline/raw 仍完成 proof")
			}
			if err := result.Conjunction.Validate(); err != nil {
				t.Fatalf("失败合取丢失可保全的原执行事实：%v", err)
			}
			if scenario == "raw-gap" {
				output := result.Conjunction.Stages[2].Stderr
				if !output.Complete || output.Storage == nil || output.Storage.Reference != "" || output.Storage.OmittedReason != "shell_stage_raw_storage_unavailable" {
					t.Fatal("持久引用缺失被隐藏或改写为接收容量不足")
				}
				if result.Conjunction.Stages[2].Stdout.Storage.Reference != source {
					t.Fatal("缺失 stderr 丢弃了已保存 stdout 的原引用")
				}
			}
			if scenario == "known-failure" && (!errors.Is(err, known) || result.Result.ExitCode != 7 || result.Result.FailureCode != "original_known_failure") {
				t.Fatal("已知失败被 cleanup/timeout 覆盖")
			}
			if scenario == "cleanup" && !errors.Is(err, cleanup) || scenario == "deadline" && !errors.Is(err, context.DeadlineExceeded) {
				t.Fatal("真实失败原因丢失")
			}
		})
	}
}

func TestProjectStageExpiredBeforeStartKeepsActualEmptyOutputIdentity(t *testing.T) {
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	stage, err := runShellProjectStage(ctx, t.TempDir(), nil, shellProjectFixedStages(nil, "exact")[0])
	if err == nil || stage.Started || !stage.TimedOut || !reflect.DeepEqual(stage.Stdout, newShellProjectRawBuffer().fact()) || !reflect.DeepEqual(stage.Stderr, newShellProjectRawBuffer().fact()) {
		t.Fatal("启动前超时补造进程或丢失实际空 bytes 身份")
	}
}
