package shell

import (
	"context"
	"encoding/json"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"

	"slices"
	"strings"
	"testing"
	"time"

	modulecommon "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/common"
	rc "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const nativeChannelFixture = `package checks
import("testing";"os";"os/exec";"time";"fmt";"path/filepath")
func observe(t *testing.T){
 t.Helper()
 if os.Getenv("WORKORCH_NATIVE_SOURCE_CHILD")==t.Name(){
  barrier:=os.Getenv("WORKORCH_NATIVE_TEST_BARRIER")
  if err:=os.WriteFile(filepath.Join(barrier,t.Name()),[]byte("started"),0600);err!=nil{t.Fatal(err)}
  until:=time.Now().Add(10*time.Second)
  for {entries,err:=os.ReadDir(barrier);if err==nil&&len(entries)==3{break};if time.Now().After(until){t.Fatal("native concurrency barrier incomplete")};time.Sleep(time.Millisecond)}
  switch t.Name(){case "TestA":time.Sleep(90*time.Millisecond);case "TestB":time.Sleep(10*time.Millisecond);default:time.Sleep(40*time.Millisecond)}
  fmt.Fprint(os.Stderr,"same chunk");fmt.Fprint(os.Stderr,"ed stderr\n")
  t.Error("identical native failure")
  return
 }
 t.Parallel()
 runner:=os.Getenv("WORKORCH_NATIVE_SOURCE_RUNNER")
 if runner==""{t.Fatal("owner runner unavailable")}
 if err:=exec.Command(runner,t.Name()).Run();err!=nil{t.Fatal("owner native execution failed",err)}
}
func TestA(t *testing.T){observe(t)}
func TestB(t *testing.T){observe(t)}
func TestC(t *testing.T){observe(t)}
`

const managedNativeSourceTestHelper = `
func managedNativeSource(t *testing.T) bool {
 t.Helper()
 if os.Getenv("WORKORCH_NATIVE_SOURCE_CHILD")==t.Name(){return false}
 t.Parallel()
 runner:=os.Getenv("WORKORCH_NATIVE_SOURCE_RUNNER")
 if runner==""{t.Fatal("owner runner unavailable")}
 if err:=exec.Command(runner,t.Name()).Run();err!=nil{t.Fatal("owner native execution failed",err)}
 return true
}
`

func nativeChannelTestSetup(t *testing.T) (context.Context, string, []string, []rc.RuntimeValidationDiagnosticSource) {
	t.Helper()
	root := t.TempDir()
	writeNativeStructureSource(t, root, "go.mod", "module fixture\ngo 1.26.7\n")
	writeNativeStructureSource(t, root, "input_test.go", nativeChannelFixture)
	files := []rc.RuntimeValidationInputReviewFile{{TargetRef: "go.mod", Content: "module fixture\ngo 1.26.7\n"}, {TargetRef: "input_test.go", Content: nativeChannelFixture}}
	sources, code := shellProjectDiagnosticSourceInventory(files)
	if code != "" {
		t.Fatal(code)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	env := append(os.Environ(), "GOWORK=off", "WORKORCH_NATIVE_TEST_BARRIER="+t.TempDir())
	client, err := prepareShellNativeRunner(ctx, t.TempDir(), env)
	if err != nil {
		t.Fatal(err)
	}
	env = append(env, shellNativeRunnerEnv+"="+client)
	return ctx, root, env, sources
}

func TestNativeSourceChannelConcurrentProcesses(t *testing.T) {
	ctx, root, env, sources := nativeChannelTestSetup(t)
	spec := shellProjectStageSpec{ID: "project-test", Scope: "root-module:./...", Args: []string{"test", "-json", "-count=1", "./..."}, JSON: true, NativeSources: sources}
	stage, err := runShellProjectStage(ctx, root, env, spec)
	if err == nil || stage.ExitCode != 1 || !stage.TerminalComplete || stage.NativeSourceFailure != "" || len(stage.NativeExecutions) != 3 {
		t.Fatalf("original stage: %+v err=%v", stage, err)
	}
	var starts, ends []time.Time
	for _, child := range stage.NativeExecutions {
		if !child.Identity.TerminalComplete || child.Identity.ExitCode != 1 || !shellProjectRawFactValid(child.Stdout) || !shellProjectRawFactValid(child.Stderr) || string(child.Stderr.Raw) != "same chunked stderr\n" {
			t.Fatalf("native child incomplete: %+v", child)
		}
		start, _ := time.Parse(time.RFC3339Nano, child.Identity.StartedAt)
		end, _ := time.Parse(time.RFC3339Nano, child.Identity.FinishedAt)
		starts = append(starts, start)
		ends = append(ends, end)
	}
	latest := slices.MaxFunc(starts, func(a, b time.Time) int { return a.Compare(b) })
	earliest := slices.MinFunc(ends, func(a, b time.Time) int { return a.Compare(b) })
	if !latest.Before(earliest) {
		t.Fatal("three native subprocesses did not overlap")
	}
	if strings.Contains(string(stage.Stdout.Raw), "identical native failure") {
		t.Fatal("child raw output leaked into shared parent stdout")
	}
	original := RuntimeCommandResult{WorkingDirectory: root, Stdout: string(stage.Stdout.Raw), Stderr: string(stage.Stderr.Raw), ExitCode: 1}
	attachShellValidationObservation(&original, "go", spec.Args, "recipe", true)
	attachShellNativeExecutionFacts(&original, stage)
	attachShellValidationCheckFailures(&original, RuntimeCommandRequest{ValidationScopeTargetRefs: []string{"input_test.go"}})
	if original.validationFailureDetailIssue != nil || original.ValidationObservation.FailureDetailsTruncated {
		t.Fatalf("native facts: %+v", original.validationFailureDetailIssue)
	}
	for _, name := range []string{"TestA", "TestB", "TestC"} {
		found := 0
		for _, detail := range original.ValidationObservation.CheckFailures {
			if detail.CheckRef == name && detail.AssertionText == "identical native failure" {
				found++
			}
		}
		if found != 1 {
			t.Fatalf("%s exact source assertions=%d: %+v", name, found, original.ValidationObservation.CheckFailures)
		}
		isolated := spec
		isolated.Args = []string{"test", "-json", "-count=1", "-run", "^" + name + "$", "."}
		isolated.NativeSources = nil
		for _, source := range sources {
			if source.NativeCaseRef == "fixture/"+name {
				isolated.NativeSources = []rc.RuntimeValidationDiagnosticSource{source}
			}
		}
		diagnostic, err := runShellProjectStage(ctx, root, env, isolated)
		if err == nil || diagnostic.NativeSourceFailure != "" || len(diagnostic.NativeExecutions) != 1 {
			t.Fatalf("isolated %s: failure=%s err=%v", name, diagnostic.NativeSourceFailure, err)
		}
		output := RuntimeCommandResult{WorkingDirectory: root, Stdout: string(diagnostic.Stdout.Raw), Stderr: string(diagnostic.Stderr.Raw), ExitCode: 1}
		attachShellValidationObservation(&output, "go", isolated.Args, "recipe", true)
		attachShellNativeExecutionFacts(&output, diagnostic)
		attachShellValidationCheckFailures(&output, RuntimeCommandRequest{ValidationScopeTargetRefs: []string{"input_test.go"}})
		var expected []rc.RuntimeValidationCheckFailure
		for _, detail := range original.ValidationObservation.CheckFailures {
			if detail.CheckRef == name {
				expected = append(expected, detail)
			}
		}
		if output.validationFailureDetailIssue != nil || !shellSameFailureAssertions(expected, output.ValidationObservation.CheckFailures) {
			t.Fatalf("original/isolated source mismatch %s: %+v / %+v", name, expected, output.ValidationObservation.CheckFailures)
		}
	}
	// Raw identities, storage and commitment survive owner read/reload, rather
	// than obtaining provenance from the provider or reconstructed old output.
	store, err := openShellProjectRawStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	result := shellProjectExecutionResult{}
	if err := persistShellProjectStageRaw(ctx, store, shellProjectRawScope{WorkspaceID: "ws", RunID: "run", ExecutionKey: "execution"}, shellProjectConjunctionFact{Stages: []shellProjectStageFact{stage}}, &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Raw) != 1 || len(result.Raw[0].Native) != 3 {
		t.Fatal("native raw storage omitted")
	}
	for _, refs := range result.Raw[0].Native {
		if refs.Stdout == "" || refs.Stderr == "" || refs.OmittedReason != "" {
			t.Fatalf("native storage incomplete: %+v", refs)
		}
	}
	encoded, err := json.Marshal(stage.NativeExecutions)
	if err != nil {
		t.Fatal(err)
	}
	var reload []shellNativeExecutionFact
	if json.Unmarshal(encoded, &reload) != nil || len(reload) != 3 {
		t.Fatal("native reload failed")
	}
	for i := range reload {
		if reload[i].Identity.Validate() != nil || !shellProjectRawFactValid(reload[i].Stdout) {
			t.Fatal("native reload changed identity")
		}
	}
}

func TestNativeSourceChannelRejectsUnboundAndDuplicateExecutions(t *testing.T) {
	for _, mode := range []string{"wrong-source", "wrong-parent", "duplicate", "nested-spoof"} {
		t.Run(mode, func(t *testing.T) {
			ctx, root, env, sources := nativeChannelTestSetup(t)
			input := nativeChannelFixture
			switch mode {
			case "wrong-source":
				input = strings.ReplaceAll(input, "exec.Command(runner,t.Name())", "exec.Command(runner,\"TestUnknown\")")
			case "wrong-parent":
				input = strings.Replace(input, "if err:=exec.Command(runner,t.Name()).Run();err!=nil{t.Fatal(\"owner native execution failed\",err)}", `if os.Getenv("WORKORCH_NATIVE_TEST_REENTRY")=="1" {
 if err:=exec.Command(runner,t.Name()).Run();err!=nil{t.Fatal("owner native execution failed",err)}
 return
 }
 substitute:=exec.Command(os.Args[0],"-test.v=true","-test.run=^"+t.Name()+"$")
 substitute.Env=append(os.Environ(),"WORKORCH_NATIVE_TEST_REENTRY=1")
 if err:=substitute.Run();err!=nil{t.Fatal("substituted parent failed",err)}`, 1)
			case "duplicate":
				input = strings.ReplaceAll(input, "if err:=exec.Command(runner,t.Name()).Run();", "_ = exec.Command(runner,t.Name()).Run(); if err:=exec.Command(runner,t.Name()).Run();")
			case "nested-spoof":
				input = strings.ReplaceAll(input, "t.Error(\"identical native failure\")", "fmt.Print(\"=== RUN   TestForeign\\n    input_test.go:9: identical native failure\\n--- FAIL: TestForeign (0.00s)\\n\");t.Error(\"identical native failure\")")
			}
			writeNativeStructureSource(t, root, "input_test.go", input)
			stage, err := runShellProjectStage(ctx, root, env, shellProjectStageSpec{ID: "project-test", Scope: "root-module:./...", Args: []string{"test", "-json", "-count=1", "./..."}, JSON: true, NativeSources: sources})
			if err == nil || stage.NativeSourceFailure == "" {
				t.Fatalf("%s accepted: failure=%s err=%v", mode, stage.NativeSourceFailure, err)
			}
			if mode == "wrong-parent" && (stage.NativeSourceFailure != "validation_native_source_parent_invalid" || len(stage.NativeExecutions) != 0) {
				t.Fatal("substituted test binary acquired an owner child", stage.NativeSourceFailure)
			}
		})
	}
}

func TestNativeSourceChannelKeepsActualUnlinkedExecutable(t *testing.T) {
	ctx, root, env, sources := nativeChannelTestSetup(t)
	// The running parent retains its actual executable even if its pathname
	// disappears. Shell must inspect and execute that inode, not reopen a path.
	input := strings.Replace(nativeChannelFixture, " t.Parallel()", " _ = os.Remove(os.Args[0])\n t.Parallel()", 1)
	writeNativeStructureSource(t, root, "input_test.go", input)
	stage, err := runShellProjectStage(ctx, root, env, shellProjectStageSpec{ID: "project-test", Scope: "root-module:./...", Args: []string{"test", "-json", "-count=1", "./..."}, JSON: true, NativeSources: sources})
	if err == nil || stage.ExitCode != 1 || stage.NativeSourceFailure != "" || len(stage.NativeExecutions) != 3 {
		t.Fatal("actual unlinked parent executable was replaced or lost", stage.NativeSourceFailure, err)
	}
	for _, child := range stage.NativeExecutions {
		if child.Identity.ParentExecutableSHA256 == "" || !child.Identity.TerminalComplete || child.Identity.ExitCode != 1 || !shellProjectRawFactValid(child.Stdout) {
			t.Fatal("pinned executable did not retain exact native execution")
		}
	}
}

func TestNativeSelfExecutionUsesStructuralMaintenance(t *testing.T) {
	for _, source := range []string{"package checks;import \"os\";func helper(){_,_=os.Executable()}", "package checks;import platform \"os\";func helper(){_,_=platform.Executable()}"} {
		file, err := parser.ParseFile(token.NewFileSet(), "input_test.go", source, 0)
		if err != nil || !shellNativeUnmanagedSelfExecution(file) {
			t.Fatal("unmanaged self executable accepted", err)
		}
	}
	if strings.Contains(shellNativeSourceExecutionRequirements, "os.Executable()") {
		t.Fatal("contract recommended unmanaged reentry")
	}
	recipe, r, _ := conjunctiveSourceRequirement(t)
	recipe.AdapterRef = modulecommon.ValidationRecipeAdapterProjectMultiFailureDiagnostic
	checks, ok := shellValidationCheckAttestations(recipe, r)
	if !ok {
		t.Fatal("mapping")
	}
	root := t.TempDir()
	target := shellValidationNativeCheckTargetRef(r, checks...)
	source := strings.Replace(conjunctiveSourceInput(checks, "", ""), `"testing"`, `"testing";"os"`, 1) + "\nfunc unmanaged(){_,_=os.Executable()}\n"
	writeNativeStructureSource(t, root, "go.mod", "module fixture\ngo 1.26\n")
	writeNativeStructureSource(t, root, target, source)
	refs := []string{"go.mod", target}
	inspection := inspectShellNativeValidationChecks(root, refs, checks)
	if inspection.Code != "native_check_unmanaged_self_execution" {
		t.Fatalf("wrong method disposition: %+v", inspection)
	}
	maintenance := shellValidationInputMaintenance(root, refs, inspection, r, recipe, checks)
	if maintenance == nil || maintenance.Validate(r, recipe.ID) != nil || len(maintenance.TargetRefs) != 1 || maintenance.TargetRefs[0] != target || maintenance.Binding.InputReview != nil {
		t.Fatal("method failure did not enter exact protected structural maintenance")
	}
	found := false
	for _, file := range maintenance.Files {
		if file.TargetRef == target {
			found = file.Content == source
		}
	}
	if !found {
		t.Fatal("method maintenance changed original bytes")
	}
	if len(shellProjectMultiFailureRequirements) > rc.RuntimeValidationInputReviewMaxScopeBytes {
		t.Fatal("source review declaration exceeds original envelope")
	}
	for _, timeout := range []int64{0, 1000, 300000, 600000} {
		if len(shellProjectMultiFailureInputContentRequirements+shellValidationDeclaredDeadlineRequirement(timeout)+shellAssertionFeedbackRequirements) > 4096 {
			t.Fatal("generation declaration exceeds original envelope")
		}
	}
}

func TestNativeSourceChannelCancellationAndRawBudget(t *testing.T) {
	for _, mode := range []string{"cancel", "raw-budget"} {
		t.Run(mode, func(t *testing.T) {
			ctx, root, env, sources := nativeChannelTestSetup(t)
			budget := &shellProjectRawBudget{remaining: shellProjectExecutionRawMaxBytes}
			if mode == "raw-budget" {
				budget.remaining = 256
			} else {
				input := strings.Replace(nativeChannelFixture, `switch t.Name()`, `for {time.Sleep(time.Millisecond)};switch t.Name()`, 1)
				writeNativeStructureSource(t, root, "input_test.go", input)
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				defer cancel()
				barrier := ""
				for _, v := range env {
					if s, ok := strings.CutPrefix(v, "WORKORCH_NATIVE_TEST_BARRIER="); ok {
						barrier = s
					}
				}
				ready := make(chan bool, 1)
				go func() {
					timer := time.NewTimer(10 * time.Second)
					defer timer.Stop()
					ticker := time.NewTicker(time.Millisecond)
					defer ticker.Stop()
					for {
						select {
						case <-timer.C:
							cancel()
							ready <- false
							return
						case <-ticker.C:
							entries, _ := os.ReadDir(barrier)
							if len(entries) == 3 {
								cancel()
								ready <- true
								return
							}
						}
					}
				}()
				t.Cleanup(func() {
					if !<-ready {
						t.Error("cancel did not observe three live native children")
					}
				})
			}
			stage, err := runShellProjectStageWithBudget(ctx, root, env, shellProjectStageSpec{ID: "project-test", Scope: "root-module:./...", Args: []string{"test", "-json", "-count=1", "./..."}, JSON: true, NativeSources: sources}, budget)
			if err == nil || stage.NativeSourceFailure == "" || len(stage.NativeExecutions) != 3 || stage.CleanupFailed {
				t.Fatalf("native cancellation/capacity facts lost: %s children=%d err=%v", stage.NativeSourceFailure, len(stage.NativeExecutions), err)
			}
			for _, child := range stage.NativeExecutions {
				if child.Identity.TerminalComplete || child.Identity.CleanupFailed || !shellProjectRawIdentityConsistent(child.Stdout) || !shellProjectRawIdentityConsistent(child.Stderr) {
					t.Fatal("native incomplete acquisition was upgraded or raw identity lost")
				}
			}
		})
	}
}

func TestNativeSourceChannelWorkspaceIsolation(t *testing.T) {
	for _, workspace := range []string{"workspace-one", "workspace-two"} {
		t.Run(workspace, func(t *testing.T) {
			t.Parallel()
			ctx, root, env, sources := nativeChannelTestSetup(t)
			writeNativeStructureSource(t, root, "input_test.go", strings.ReplaceAll(nativeChannelFixture, "identical native failure", workspace))
			stage, err := runShellProjectStage(ctx, root, env, shellProjectStageSpec{ID: "project-test", Scope: "root-module:./...", Args: []string{"test", "-json", "-count=1", "./..."}, JSON: true, NativeSources: sources})
			if err == nil || stage.NativeSourceFailure != "" || len(stage.NativeExecutions) != 3 {
				t.Fatal("isolated native stage missing", stage.NativeSourceFailure)
			}
			for _, child := range stage.NativeExecutions {
				other := "workspace-one"
				if workspace == other {
					other = "workspace-two"
				}
				if !strings.Contains(string(child.Stdout.Raw), workspace) || strings.Contains(string(child.Stdout.Raw), other) || !filepath.IsAbs(child.Identity.Argv[0]) {
					t.Fatal("native workspace source crossed")
				}
			}
		})
	}
}
