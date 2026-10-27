package run

import (
	"fmt"
	"strings"
	"testing"
	"time"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func receiptFixture() WorkspaceExecutionIntentReceipt {
	b := receiptBootstrapFixture()
	hash, _ := ExecutionIntentPayloadHash("ws", "request", b)
	return WorkspaceExecutionIntentReceipt{Version: WorkspaceExecutionIntentReceiptVersion, PayloadHash: hash, RecordedAt: time.Now().UTC(), Admission: runtimecontract.RuntimeExecutionIntentAdmission{
		WorkspaceID: "ws", RunID: "run-001", RequestID: "request", Kind: runtimecontract.RuntimeExecutionIntentRestart,
		Outcome: runtimecontract.RuntimeExecutionIntentReconciliationRequested, Reason: runtimecontract.RuntimeExecutionIntentReasonNewRunActivated,
	}}
}

func receiptBootstrapFixture() runtimecontract.RuntimeBootstrap {
	b := runtimecontract.RuntimeBootstrap{Definition: runtimecontract.RuntimeDefinitionContext{GoalID: "goal", DoDID: "dod", Revision: "def", StartupInputHash: "startup"}}
	return b.Clone()
}

func receiptSourceFixture(r WorkspaceExecutionIntentReceipt) *Run {
	b := receiptBootstrapFixture()
	return &Run{ID: r.Admission.RunID, WorkspaceID: r.Admission.WorkspaceID, ActivationRequestID: r.Admission.RequestID, GoalID: "goal", DoDID: "dod", DefinitionRevision: "def", StartupInputHash: "startup", RuntimeBootstrap: &b}
}

func TestExecutionReceiptValidationAndImmutablePrefix(t *testing.T) {
	r := receiptFixture()
	item := receiptSourceFixture(r)
	next := CloneRun(item)
	next.ExecutionIntentReceipts = []WorkspaceExecutionIntentReceipt{r}
	if r.Validate() != nil || ValidateExecutionIntentReceiptTransition(item, next, true) != nil {
		t.Fatal("initial CAS receipt rejected")
	}
	if ValidateExecutionIntentReceiptTransition(item, next, false) == nil || ValidateExecutionIntentReceiptTransition(next, item, true) == nil {
		t.Fatal("ordinary update appended or CAS erased receipt")
	}
	for name, mutate := range map[string]func(*WorkspaceExecutionIntentReceipt){
		"version": func(r *WorkspaceExecutionIntentReceipt) { r.Version = "v0" },
		"hash":    func(r *WorkspaceExecutionIntentReceipt) { r.PayloadHash = strings.Repeat("A", 64) },
		"time":    func(r *WorkspaceExecutionIntentReceipt) { r.RecordedAt = time.Time{} },
		"scope":   func(r *WorkspaceExecutionIntentReceipt) { r.Admission.WorkspaceID = "other" },
		"run":     func(r *WorkspaceExecutionIntentReceipt) { r.Admission.RunID = "other" },
		"request": func(r *WorkspaceExecutionIntentReceipt) { r.Admission.RequestID = " request" },
		"replay": func(r *WorkspaceExecutionIntentReceipt) {
			r.Admission.Reason = runtimecontract.RuntimeExecutionIntentReasonCurrentRunContinuationAccepted
		},
	} {
		t.Run(name, func(t *testing.T) {
			changed := CloneRun(next)
			mutate(&changed.ExecutionIntentReceipts[0])
			if ValidateExecutionIntentReceiptTransition(next, changed, true) == nil {
				t.Fatal("history replacement accepted")
			}
		})
	}
	cloned := CloneRun(next)
	cloned.ExecutionIntentReceipts[0].PayloadHash = strings.Repeat("b", 64)
	if next.ExecutionIntentReceipts[0] != r {
		t.Fatal("clone aliases receipt")
	}
	next.ExecutionIntentReceipts = append(next.ExecutionIntentReceipts, r)
	if ValidateExecutionIntentReceipts(next) == nil {
		t.Fatal("duplicate request accepted")
	}
	next.ExecutionIntentReceipts = nil
	for i := 0; i <= WorkspaceExecutionIntentReceiptLimit; i++ {
		r.Admission.RequestID = fmt.Sprint(i)
		next.ExecutionIntentReceipts = append(next.ExecutionIntentReceipts, r)
	}
	if ValidateExecutionIntentReceipts(next) == nil {
		t.Fatal("unbounded receipt history accepted")
	}
}

func TestExecutionReceiptCompleteInventoryRejectsAmbiguityAndPreservesUnknown(t *testing.T) {
	r := receiptFixture()
	item := receiptSourceFixture(r)
	item.ExecutionIntentReceipts = []WorkspaceExecutionIntentReceipt{r}
	got, err := CompileExecutionIntentReceipt("ws", "request", []*Run{item})
	if err != nil || got == nil || *got != r {
		t.Fatal("original receipt missing")
	}
	got.PayloadHash = "caller mutation"
	if item.ExecutionIntentReceipts[0] != r {
		t.Fatal("compiler aliases stored value")
	}
	for _, items := range [][]*Run{nil, {&Run{ID: "legacy", WorkspaceID: "ws"}}} {
		if got, err := CompileExecutionIntentReceipt("ws", "request", items); err != nil || got != nil {
			t.Fatal("missing receipt did not stay unknown")
		}
	}
	duplicate := CloneRun(item)
	duplicate.ID = "run-002"
	duplicate.ExecutionIntentReceipts[0].Admission.RunID = duplicate.ID
	for _, items := range [][]*Run{{nil}, {item, duplicate}, {&Run{WorkspaceID: "other"}}} {
		if _, err := CompileExecutionIntentReceipt("ws", "request", items); err == nil {
			t.Fatal("incomplete, ambiguous or cross-scope inventory accepted")
		}
	}
}

func TestExecutionReceiptReloadReprovesOriginalPayloadAndSource(t *testing.T) {
	r := receiptFixture()
	item := receiptSourceFixture(r)
	item.ExecutionIntentReceipts = []WorkspaceExecutionIntentReceipt{r}
	if ValidateRunRuntimeBootstrapsRaw(item) != nil {
		t.Fatal("valid original source rejected")
	}
	for _, mutation := range []string{"activation", "source", "payload", "goal", "missing_source", "invented_request"} {
		next := CloneRun(item)
		switch mutation {
		case "activation":
			next.ActivationRequestID = "other"
		case "source":
			next.RuntimeBootstrap.Definition.GoalDefinition = "changed original source"
		case "payload":
			next.ExecutionIntentReceipts[0].PayloadHash = strings.Repeat("b", 64)
		case "goal":
			next.GoalID = "other"
		case "missing_source":
			next.RuntimeBootstrap = nil
		case "invented_request":
			next.ExecutionIntentReceipts[0].Admission.RequestID = "other"
			next.ExecutionIntentReceipts[0].PayloadHash, _ = ExecutionIntentPayloadHash("ws", "other", *next.RuntimeBootstrap)
		}
		if ValidateRunRuntimeBootstrapsRaw(next) == nil {
			t.Fatalf("%s drift survived raw reload", mutation)
		}
	}
}
