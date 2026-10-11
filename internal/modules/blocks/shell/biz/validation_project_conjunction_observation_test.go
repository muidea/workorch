package shell

import (
	"reflect"
	"testing"
	"time"
)

func TestProjectConjunctionObservationPreservesActualOwnerFacts(t *testing.T) {
	specs := shellProjectFixedStages([]string{"test", "-json", "-count=1", "-run", "^TestExact$", "./workorch_validation/v4/exact"}, "workorch_validation/v4/exact")
	raw := newShellProjectRawBuffer()
	_, _ = raw.Write([]byte("原始输出\r\n\x00\xff"))
	source, err := shellProjectSourceSetHash(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var stages []shellProjectStageFact
	for index, spec := range specs {
		start := time.Date(2026, 10, 11, 0, 0, index*2, 0, time.UTC)
		stages = append(stages, shellProjectStageFact{ID: spec.ID, Scope: spec.Scope, Argv: append([]string{"go"}, spec.Args...), Started: true, StartedAt: start.Format(time.RFC3339Nano), FinishedAt: start.Add(time.Second).Format(time.RFC3339Nano), ExitCode: 0, Stdout: raw.fact(), Stderr: newShellProjectRawBuffer().fact(), SourceBefore: source, SourceAfter: source, TerminalComplete: true, Tests: 1, PassedTests: 1})
	}
	fact := sealShellProjectConjunction(stages, specs, []string{"delivery.go"})
	observation, err := shellProjectConjunctionObservation(fact, specs)
	if err != nil || !observation.Complete || len(observation.Stages) != 4 || observation.Stages[1].Stdout.SHA256 != stages[1].Stdout.SHA256 || !reflect.DeepEqual(observation.Stages[3].Argv, stages[3].Argv) {
		t.Fatalf("实际执行映射丢失事实：%v", err)
	}
	observation.Stages[0].Argv[0] = "caller"
	observation.BehaviorTargetRefs[0] = "caller"
	if fact.Stages[0].Argv[0] != "go" || fact.BehaviorTargetRefs[0] != "delivery.go" {
		t.Fatal("typed 映射共享 owner 原事实")
	}
	for _, scenario := range []string{"wrong-argv", "wrong-stage", "raw-hash", "raw-bytes", "partial"} {
		changed := fact
		changed.Stages = append([]shellProjectStageFact(nil), fact.Stages...)
		switch scenario {
		case "wrong-argv":
			changed.Stages[1].Argv = []string{"go", "test", "./subset"}
		case "wrong-stage":
			changed.Stages[1].Scope = "subset"
		case "raw-hash":
			changed.Stages[1].Stdout.SHA256 = source
		case "raw-bytes":
			changed.Stages[1].Stdout.Raw = []byte("replaced")
		case "partial":
			changed.Complete = false
			changed.Stages = changed.Stages[:2]
			changed.Stages[1].KnownFailure = true
			changed.Stages[1].ExitCode = 1
			changed.Stages[1].FailureCode = "known-failure"
		}
		converted, err := shellProjectConjunctionObservation(changed, specs)
		if scenario == "partial" {
			if err != nil || converted.Complete || !converted.Stages[1].KnownFailure || converted.Stages[1].ExitCode != 1 {
				t.Fatal("部分已知失败被丢失或假称完整")
			}
		} else if err == nil {
			t.Fatalf("非法实际事实映射未拒绝：%s", scenario)
		}
	}
}
