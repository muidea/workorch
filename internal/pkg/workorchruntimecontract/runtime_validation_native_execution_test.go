package workorchruntimecontract

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestValidationNativeExecutionIdentityAndLifecycle(t *testing.T) {
	observation, _ := validationExecutionConjunctionFixture()
	c := observation.ExecutionConjunction
	stage := &c.Stages[0]
	stage.NativeExecutions = []RuntimeValidationNativeExecution{{SourceRef: "source", NativeCaseRef: "native", ParentExecutableSHA256: stage.SourceBefore, Argv: []string{"actual-executable", "exact-selector"}, StartedAt: stage.StartedAt, FinishedAt: stage.FinishedAt, TerminalComplete: true, Stdout: stage.Stdout.Clone(), Stderr: stage.Stderr.Clone()}}
	if c.Validate() != nil {
		t.Fatal("complete native observation rejected", c.Validate())
	}
	encoded, err := json.Marshal(observation)
	if err != nil {
		t.Fatal(err)
	}
	var reload RuntimeValidationObservation
	if json.Unmarshal(encoded, &reload) != nil || !reflect.DeepEqual(reload, observation) {
		t.Fatal("native persistence changed original fields")
	}
	hash := StableRuntimeOperationReceiptHash(RuntimeOperationReceipt{Evidence: RuntimeOperationEvidence{Validation: &observation}})
	for _, scenario := range []string{"duplicate", "wrong-time", "timeout", "cleanup", "unfinished", "raw", "storage", "source-failure"} {
		t.Run(scenario, func(t *testing.T) {
			changed := observation.Clone()
			s := &changed.ExecutionConjunction.Stages[0]
			child := &s.NativeExecutions[0]
			switch scenario {
			case "duplicate":
				s.NativeExecutions = append(s.NativeExecutions, child.Clone())
			case "wrong-time":
				child.FinishedAt = "2026-10-12T00:00:00Z"
			case "timeout":
				child.TimedOut = true
			case "cleanup":
				child.CleanupFailed = true
			case "unfinished":
				child.TerminalComplete = false
			case "raw":
				child.Stdout.SHA256 = "tampered"
			case "storage":
				s.Stdout.Storage = &RuntimeValidationOutputStorage{Reference: "parent-stdout"}
				s.Stderr.Storage = &RuntimeValidationOutputStorage{Reference: "parent-stderr"}
			case "source-failure":
				s.NativeSourceFailure = "native-source-unavailable"
			}
			if changed.ExecutionConjunction.Validate() == nil {
				t.Fatal("invalid native execution accepted")
			}
			if StableRuntimeOperationReceiptHash(RuntimeOperationReceipt{Evidence: RuntimeOperationEvidence{Validation: &changed}}) == hash {
				t.Fatal("native mutation did not change observation identity")
			}
			if c.Validate() != nil {
				t.Fatal("clone mutation escaped into original")
			}
		})
	}
	// Noncomplete executions retain known failures/partial acquisition as
	// history; completeness is not inferred from the existence of a child.
	partial := observation.Clone()
	partial.ExecutionConjunction.Complete = false
	partial.ExecutionConjunction.Stages[0].NativeExecutions[0].ExitCode = 1
	partial.ExecutionConjunction.Stages[0].NativeExecutions[0].TerminalComplete = false
	if partial.ExecutionConjunction.Validate() != nil {
		t.Fatal("explicit incomplete native history lost")
	}
}

func TestValidationNativeExecutionReverseBindsOwnerSource(t *testing.T) {
	binding, _ := diagnosticSourceReviewFixture(t)
	observation := diagnosticExecutionFixture()
	stage := &observation.ExecutionConjunction.Stages[0]
	source := binding.InputReview.DiagnosticSources[0]
	stage.NativeExecutions = []RuntimeValidationNativeExecution{{SourceRef: source.SourceRef, NativeCaseRef: source.NativeCaseRef, ParentExecutableSHA256: stage.SourceBefore, Argv: []string{"tool"}, StartedAt: stage.StartedAt, FinishedAt: stage.FinishedAt, ExitCode: 1, TerminalComplete: true, Stdout: stage.Stdout, Stderr: stage.Stderr}}
	// Without attributed repair facts this is still an independently scoped
	// historical observation. Identity tampering is rejected even here.
	for i := range observation.ExecutionConjunction.Stages {
		observation.ExecutionConjunction.Stages[i].DiagnosticSourceRef = ""
		observation.ExecutionConjunction.Stages[i].BehaviorTargetRefs = nil
	}
	if err := ValidateRuntimeValidationDiagnosticExecution(observation, binding, nil); err != nil {
		t.Fatal(err)
	}
	changed := observation.Clone()
	changed.ExecutionConjunction.Stages[0].NativeExecutions[0].NativeCaseRef = "other"
	if ValidateRuntimeValidationDiagnosticExecution(changed, binding, nil) == nil {
		t.Fatal("changed native source accepted")
	}
}
