package shell

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// 使用真实 Go、真实 managed copy 和 owner instrumentation；尚不开放公开
// adapter，也不把本测试当作 authoring/activation/fullflow 或 fresh-process 验收。
func TestProjectConjunctionOwnerRunnerKeepsProjectAndCWIAllOf(t *testing.T) {
	for _, scenario := range []string{"pass", "old-failure", "neighbor-compile", "skip", "final-behavior-fails", "behavior-unobserved", "missing-mapping", "source-change", "race"} {
		t.Run(scenario, func(t *testing.T) {
			root := t.TempDir()
			recipe := isolatedInputRecipe()
			requirement := shellValidationProofRequirementForTest(t, recipe.ID, "project-cwi")
			checks, ok := shellValidationCheckAttestations(recipe, requirement)
			if !ok {
				t.Fatal("缺少 exact native mapping")
			}
			target := shellValidationNativeCheckTargetRef(requirement, checks...)
			body := `if delivery.Value()!=4 {t.Fatal("wrong delivery")}`
			switch scenario {
			case "skip":
				body = `t.Skip("not executed"); if delivery.Value()!=4 {t.Fatal("wrong delivery")}`
			case "behavior-unobserved":
				body = `if false {t.Fatal(delivery.Value())}`
			case "source-change":
				body = `if delivery.Value()!=4 {t.Fatal("wrong delivery")}; if err:=os.WriteFile("../../../delivery/value.go",[]byte("package delivery\nfunc Value() int{return 9}\n"),0600);err!=nil{t.Fatal(err)}`
			case "final-behavior-fails":
				body = `if delivery.Value()!=4 {t.Fatal("wrong delivery")}; if strings.Contains(os.Getenv("GOFLAGS"),"-cover") {t.Fatal("exact behavior failure")}`
			}
			imports := `"testing"; delivery "fixture/delivery"`
			if scenario == "source-change" || scenario == "final-behavior-fails" {
				imports += `; "os"`
			}
			if scenario == "final-behavior-fails" {
				imports += `; "strings"`
			}
			files := map[string]string{
				"go.mod":                 "module fixture\ngo 1.26\n",
				"delivery/value.go":      "package delivery\nfunc Value() int{return 4}\n",
				"delivery/value_test.go": "package delivery\nimport \"testing\"\nfunc TestProduction(t *testing.T){if Value()!=4{t.Fatal(\"wrong\")}}\n",
				target:                   fmt.Sprintf("package checks\nimport (%s)\nfunc %s(t *testing.T){%s}\n", imports, checks[0].NativeCheckRef, body),
			}
			if scenario == "old-failure" {
				files["workorch_validation/v3/old_test.go"] = "package old\nimport \"testing\"\nfunc TestOld(t *testing.T){t.Fatal(\"original failure\")}\n"
			}
			if scenario == "neighbor-compile" {
				files["workorch_validation/v4/neighbor/input_test.go"] = "package neighbor\nfunc broken(\n"
			}
			if scenario == "race" {
				files["delivery/value_test.go"] = "package delivery\nimport (\"testing\";\"sync\")\nfunc TestRace(t *testing.T){var n int;var wg sync.WaitGroup;wg.Add(2);for range 2 {go func(){defer wg.Done();for range 10000 {n++}}()};wg.Wait();_=n}\n"
			}
			var refs []string
			for ref, content := range files {
				writeNativeStructureSource(t, root, ref, content)
				refs = append(refs, ref)
			}
			request := RuntimeCommandRequest{WorkspaceDeliverableDirectory: root, ValidationRecipeID: recipe.ID, Command: recipe.Command, Args: recipe.Args, TimeoutMillis: 60000, TargetRefs: []string{target}, ValidationScopeTargetRefs: refs, ValidationCheckAttestations: checks, ManagedValidation: runtimecontract.RuntimeManagedValidation{RuntimeManaged: true, Kind: runtimecontract.RuntimeBlockKindManagedValidationSession}}
			if err := instrumentManagedValidationNativeChecks(&request); err != nil {
				t.Fatal(err)
			}
			if scenario == "missing-mapping" {
				request.ValidationCheckAttestations = nil
			}
			copied, err := prepareManagedValidationSandbox(&request, root)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := cleanupManagedValidationScratch(request); err != nil {
					t.Error(err)
				}
			})
			adapter := NewRuntimeAdapter()
			env := managedValidationEnvironment(adapter.filteredEnv(), request.ValidationScratchDirectory)
			ctx, cancel := context.WithTimeout(context.Background(), DefaultCommandTimeout)
			defer cancel()
			result, fact, err := runShellProjectConjunction(ctx, copied, env, request)
			if scenario == "pass" {
				if err != nil || !fact.Complete || len(fact.Stages) != 4 || result.Evidence.Quality != common.EvidenceQualityValid || result.ValidationObservation == nil || !runtimecontract.RuntimeValidationCheckPassed(*result.ValidationObservation, checks[0].NativeCheckRef) {
					t.Fatalf("真实合取未完成：%+v err=%v", fact, err)
				}
				if !reflect.DeepEqual(fact.BehaviorTargetRefs, []string{"delivery/value.go"}) || result.ValidationObservation.ProofClass != "" {
					t.Fatal("private producer 补造 Runtime proof 或丢失实际行为")
				}
			} else if err == nil || fact.Complete || result.Evidence.Quality == common.EvidenceQualityValid {
				t.Fatalf("合取反例被接受：%s %+v", scenario, fact)
			}
			if scenario == "old-failure" || scenario == "neighbor-compile" || scenario == "race" || scenario == "final-behavior-fails" {
				last := fact.Stages[len(fact.Stages)-1]
				if !last.KnownFailure || last.ExitCode == 0 || result.ExitCode == 0 {
					t.Fatal("已知工程失败被改成未决的 CWI 结果")
				}
			}
			if scenario == "final-behavior-fails" {
				if len(fact.Stages) != 4 || fact.Stages[0].FailureCode != "" || fact.Stages[1].FailureCode != "" || fact.Stages[2].FailureCode != "" || result.ValidationObservation == nil || !reflect.DeepEqual(result.ValidationObservation.FailedChecks, []string{checks[0].NativeCheckRef}) {
					t.Fatal("前三阶段成功替代了最后 exact CWI 行为失败")
				}
			}
			for _, stage := range fact.Stages {
				if !shellProjectRawFactValid(stage.Stdout) || !shellProjectRawFactValid(stage.Stderr) {
					t.Fatal("stage 原文输出丢失 bytes/hash")
				}
			}
			for ref, want := range files {
				got, err := os.ReadFile(filepath.Join(root, ref))
				if err != nil || string(got) != want {
					t.Fatalf("owner 改写权威 workspace：%s", ref)
				}
			}
		})
	}
}

func TestProjectConjunctionOwnerRunnerRejectsUnboundedContextBeforePreparation(t *testing.T) {
	for _, duration := range []time.Duration{0, 2 * DefaultCommandTimeout, -time.Second} {
		ctx, cancel := context.Background(), func() {}
		if duration != 0 {
			ctx, cancel = context.WithTimeout(ctx, duration)
		}
		result, fact, err := runShellProjectConjunction(ctx, t.TempDir(), nil, RuntimeCommandRequest{})
		cancel()
		if err == nil || fact.Complete || len(fact.Stages) != 0 || result.FailureCode != "validation_project_deadline_invalid" {
			t.Fatal("无效 operation deadline 仍进入准备或实际进程执行")
		}
	}
	for _, request := range []RuntimeCommandRequest{{TimeoutMillis: -1}, {TimeoutMillis: 999}, {TimeoutMillis: 120000}, {TimeoutMillis: 1000}} {
		ctx, cancel := context.WithTimeout(context.Background(), DefaultCommandTimeout)
		result, fact, err := runShellProjectConjunction(ctx, t.TempDir(), nil, request)
		cancel()
		if err == nil || len(fact.Stages) != 0 || result.FailureCode != "validation_project_deadline_invalid" {
			t.Fatal("声明期限提升或 context 超出精确 request 总期限仍被接受")
		}
	}
}
