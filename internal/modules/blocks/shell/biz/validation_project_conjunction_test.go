package shell

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

func shellProjectTranscript(events ...shellGoTestJSONEvent) []byte {
	var body bytes.Buffer
	for _, event := range events {
		encoded, _ := json.Marshal(event)
		body.Write(encoded)
		body.WriteByte('\n')
	}
	return body.Bytes()
}

func shellProjectPassingTranscript() []byte {
	return shellProjectTranscript(
		shellGoTestJSONEvent{Action: "start", Package: "fixture"},
		shellGoTestJSONEvent{Action: "run", Package: "fixture", Test: "TestActual"},
		shellGoTestJSONEvent{Action: "pass", Package: "fixture", Test: "TestActual"},
		shellGoTestJSONEvent{Action: "pass", Package: "fixture"},
	)
}

func TestProjectConjunctionJSONRequiresEveryPackageAndTestTerminal(t *testing.T) {
	pass := shellProjectPassingTranscript()
	for _, scenario := range []struct {
		name     string
		body     []byte
		complete bool
		want     string
		failed   bool
	}{
		{name: "actual", body: pass, complete: true},
		{name: "empty", complete: true, want: "validation_project_test_transcript_incomplete"},
		{name: "no-tests", body: shellProjectTranscript(shellGoTestJSONEvent{Action: "start", Package: "fixture"}, shellGoTestJSONEvent{Action: "skip", Package: "fixture"}), complete: true, want: "validation_project_test_empty"},
		{name: "skip-only", body: shellProjectTranscript(shellGoTestJSONEvent{Action: "start", Package: "fixture"}, shellGoTestJSONEvent{Action: "run", Package: "fixture", Test: "TestSkipped"}, shellGoTestJSONEvent{Action: "skip", Package: "fixture", Test: "TestSkipped"}, shellGoTestJSONEvent{Action: "pass", Package: "fixture"}), complete: true, want: "validation_project_test_skipped"},
		{name: "unfinished-package", body: append(append([]byte(nil), pass...), shellProjectTranscript(shellGoTestJSONEvent{Action: "start", Package: "neighbor"})...), complete: true, want: "validation_project_test_transcript_incomplete"},
		{name: "unfinished-test", body: shellProjectTranscript(shellGoTestJSONEvent{Action: "start", Package: "fixture"}, shellGoTestJSONEvent{Action: "run", Package: "fixture", Test: "TestActual"}, shellGoTestJSONEvent{Action: "pass", Package: "fixture"}), complete: true, want: "validation_project_test_transcript_incomplete"},
		{name: "unstarted-pass", body: shellProjectTranscript(shellGoTestJSONEvent{Action: "pass", Package: "fixture", Test: "TestForged"}), complete: true, want: "validation_project_test_transcript_incomplete"},
		{name: "duplicate-package-terminal", body: append(append([]byte(nil), pass...), shellProjectTranscript(shellGoTestJSONEvent{Action: "pass", Package: "fixture"})...), complete: true, want: "validation_project_test_transcript_incomplete"},
		{name: "raw-child-marker", body: []byte("WORKORCH_CHECK_PASSED=TestActual\nPASS\n"), complete: true, want: "validation_project_test_transcript_incomplete"},
		{name: "truncated-pass", body: pass, want: "validation_project_test_transcript_incomplete"},
		{name: "failure-before-incomplete", body: shellProjectTranscript(shellGoTestJSONEvent{Action: "fail", Package: "fixture", Test: "TestActual"}), want: "validation_project_test_failed", failed: true},
		{name: "build-failure-before-incomplete", body: shellProjectTranscript(shellGoTestJSONEvent{Action: "build-fail", ImportPath: "neighbor"}), want: "validation_project_test_failed", failed: true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			_, _, _, terminal, failed, code := shellProjectJSONCompletion(scenario.body, scenario.complete)
			if code != scenario.want || failed != scenario.failed || (scenario.want == "" && !terminal) {
				t.Fatalf("terminal=%v failed=%v code=%s，期望 %s", terminal, failed, code, scenario.want)
			}
		})
	}
}

func TestProjectConjunctionRawBytesPreservedAndCapacityExplicit(t *testing.T) {
	raw := []byte("  token=not-a-real-secret\r\n\x00\xff\n")
	buffer := newShellProjectRawBuffer()
	_, _ = buffer.Write(raw)
	fact := buffer.fact()
	digest := sha256.Sum256(raw)
	if !bytes.Equal(fact.Raw, raw) || !fact.Complete || fact.Bytes != int64(len(raw)) || fact.SHA256 != hex.EncodeToString(digest[:]) || !shellProjectRawFactValid(fact) {
		t.Fatal("原 bytes 或身份被清洗/摘要替代")
	}
	large := bytes.Repeat([]byte("z"), MaxCommandOutputBytes+17)
	buffer = newShellProjectRawBuffer()
	_, _ = buffer.Write(large)
	fact = buffer.fact()
	digest = sha256.Sum256(large)
	if fact.Complete || fact.Bytes != int64(len(large)) || fact.SavedBytes != MaxCommandOutputBytes || fact.OmittedReason == "" || fact.SHA256 != hex.EncodeToString(digest[:]) || shellProjectRawFactValid(fact) {
		t.Fatal("超界输出被误标完整")
	}
}

func TestProjectConjunctionFactBindsExactArgvScopeAndSourceSet(t *testing.T) {
	specs := shellProjectFixedStages([]string{"test", "-json", "-count=1", "-run", "^TestExact$", "./workorch_validation/v4/exact"}, "workorch_validation/v4/exact")
	raw := newShellProjectRawBuffer().fact()
	var stages []shellProjectStageFact
	for _, spec := range specs {
		stages = append(stages, shellProjectStageFact{ID: spec.ID, Scope: spec.Scope, Argv: append([]string{"go"}, spec.Args...), Started: true, ExitCode: 0, Stdout: raw, Stderr: raw, TerminalComplete: true, Tests: 1, PassedTests: 1, SourceBefore: "source-a", SourceAfter: "source-a"})
	}
	baseline := sealShellProjectConjunction(stages, specs, []string{"delivery.go"})
	if !baseline.Complete || baseline.CommitmentHash == "" {
		t.Fatal("完整合取事实无法封存")
	}
	for _, mutation := range []string{"native-only", "wrong-argv", "wrong-scope", "failed", "timed-out", "cleanup", "raw-identity", "source-change", "cross-stage-source-change", "no-tests", "unfinished", "missing-behavior"} {
		t.Run(mutation, func(t *testing.T) {
			changed := append([]shellProjectStageFact(nil), stages...)
			behavior := []string{"delivery.go"}
			switch mutation {
			case "native-only":
				changed = changed[3:]
			case "wrong-argv":
				changed[1].Argv = []string{"go", "test", "-run", "TestExact", "./..."}
			case "wrong-scope":
				changed[1].Scope = "reviewed-package:exact"
			case "failed":
				changed[1].KnownFailure = true
			case "timed-out":
				changed[2].TimedOut = true
			case "cleanup":
				changed[0].CleanupFailed = true
			case "raw-identity":
				changed[1].Stdout.SHA256 = "forged"
			case "source-change":
				changed[1].SourceAfter = "source-b"
			case "cross-stage-source-change":
				changed[2].SourceBefore, changed[2].SourceAfter = "source-b", "source-b"
			case "no-tests":
				changed[2].Tests, changed[2].PassedTests = 0, 0
			case "unfinished":
				changed[2].TerminalComplete = false
			case "missing-behavior":
				behavior = nil
			}
			fact := sealShellProjectConjunction(changed, specs, behavior)
			if fact.Complete || fact.FailureCode == "" || fact.CommitmentHash == baseline.CommitmentHash {
				t.Fatal("native-only/弱事实混充复合验证")
			}
		})
	}
	if !reflect.DeepEqual(stages[1].Argv, append([]string{"go"}, specs[1].Args...)) {
		t.Fatal("seal 修改原执行事实")
	}
}

// 真实 Go 负例，由主测试窗口执行。旧输入/相邻 CWI 的失败不得因当前
// exact CWI 可通过而省略；这里只校准独立 stage helper，不宣称 Runtime 接线。
func TestProjectConjunctionRealGoRejectsProjectFailuresSkipsAndEmpty(t *testing.T) {
	for _, scenario := range []string{"pass", "old-input-failure", "neighbor-compile", "skip", "empty", "race"} {
		t.Run(scenario, func(t *testing.T) {
			root := t.TempDir()
			writeNativeStructureSource(t, root, "go.mod", "module fixture\ngo 1.26\n")
			writeNativeStructureSource(t, root, "delivery.go", "package fixture\nfunc Value() int { return 4 }\n")
			body := "package fixture\nimport \"testing\"\nfunc TestActual(t *testing.T) { if Value()!=4 {t.Fatal(\"wrong\")} }\n"
			spec := shellProjectFixedStages(nil, "")[1]
			switch scenario {
			case "old-input-failure":
				writeNativeStructureSource(t, root, "workorch_validation/v3/old_test.go", "package old\nimport \"testing\"\nfunc TestOld(t *testing.T){t.Fatal(\"original failure\")}\n")
			case "neighbor-compile":
				writeNativeStructureSource(t, root, "workorch_validation/v4/neighbor/input_test.go", "package neighbor\nfunc broken(\n")
			case "skip":
				body = strings.Replace(body, "if Value()!=4 {t.Fatal(\"wrong\")}", "t.Skip(\"not executed\")", 1)
			case "empty":
				body = "package fixture\n"
			case "race":
				spec = shellProjectFixedStages(nil, "")[2]
				body = "package fixture\nimport (\"testing\";\"sync\")\nfunc TestRace(t *testing.T){var n int; var wg sync.WaitGroup; wg.Add(2); for range 2 {go func(){defer wg.Done(); for range 10000 {n++}}()}; wg.Wait(); _=n}\n"
			}
			writeNativeStructureSource(t, root, "delivery_test.go", body)
			ctx, cancel := context.WithTimeout(context.Background(), DefaultCommandTimeout)
			defer cancel()
			env := shellBehaviorEnvironment(NewRuntimeAdapter().filteredEnv())
			fact, err := runShellProjectStage(ctx, root, env, spec)
			if scenario == "pass" {
				if err != nil || fact.FailureCode != "" || fact.Tests == 0 || !fact.TerminalComplete || !fact.Stdout.Complete {
					t.Fatalf("真实测试未完成：%+v err=%v", fact, err)
				}
			} else if err == nil || fact.FailureCode == "" {
				t.Fatalf("真实负例被接受：%+v", fact)
			}
			if scenario == "old-input-failure" || scenario == "neighbor-compile" || scenario == "race" {
				if !fact.KnownFailure || fact.ExitCode == 0 {
					t.Fatalf("已知失败被降为未决：%+v", fact)
				}
			}
		})
	}
}

func TestProjectConjunctionDeadlineCannotBeExtendedOrRenewed(t *testing.T) {
	for _, duration := range []time.Duration{0, 2 * DefaultCommandTimeout, -time.Second} {
		ctx := context.Background()
		cancel := func() {}
		if duration != 0 {
			ctx, cancel = context.WithTimeout(ctx, duration)
		}
		fact, err := runShellProjectStage(ctx, t.TempDir(), nil, shellProjectFixedStages(nil, "")[0])
		cancel()
		if err == nil || fact.Started || fact.FailureCode == "" {
			t.Fatal("无 deadline/期限提升/过期 context 仍启动进程")
		}
	}
}

func TestProjectConjunctionSourceSetIncludesNeighborsAndFixtures(t *testing.T) {
	root := t.TempDir()
	writeNativeStructureSource(t, root, "go.mod", "module fixture\ngo 1.26\n")
	before, err := shellProjectSourceSetHash(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, ref := range []string{"workorch_validation/v4/neighbor/input_test.go", "workorch_validation/v4/neighbor/fixture.txt", "go.work.sum"} {
		writeNativeStructureSource(t, root, ref, "original bytes")
		after, err := shellProjectSourceSetHash(root)
		if err != nil || after == before {
			t.Fatalf("遗漏 source/member：%s %v", ref, err)
		}
		before = after
	}
}
