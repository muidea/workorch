package runtime

import (
	rc "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
	"testing"
)

func TestReconciliationCommittedTransitionDoesNotRecompileHistory(t *testing.T) {
	// Source is deliberately unavailable here. It belongs to the original
	// admission; the current transition must preserve its formal result exactly.
	request := rc.RuntimeCWIReconciliationRequest{RequestID: "request", Generation: 1}
	before := RuntimeAggregate{Reconciliation: rc.RuntimeCWIReconciliationLedger{Request: &request, RequestHash: "request-hash"}}
	next := before.CloneRaw()
	next.Reconciliation.History = append(next.Reconciliation.History, rc.RuntimeCWIReconciliationGeneration{Request: request, RequestHash: "request-hash", Superseded: true})
	next.Reconciliation.Request = nil
	next.Reconciliation.RequestHash = ""
	if err := validateReconciliationExecutionTransition(before, next); err != nil {
		t.Fatal(err)
	}
	restored := next.CloneRaw()
	if err := validateReconciliationExecutionTransition(next, restored); err != nil {
		t.Fatal(err)
	}
	restored.Reconciliation.History[0].RequestHash = "rewritten"
	if err := validateReconciliationExecutionTransition(next, restored); err == nil {
		t.Fatal("historical result rewrite accepted")
	}
}

func TestReconciliationNewExecutionUseRequiresOriginalSource(t *testing.T) {
	request := rc.RuntimeCWIReconciliationRequest{RequestID: "request", Bindings: []rc.RuntimeCWIReconciliationBindingRef{{BindingID: "binding"}}}
	before := RuntimeAggregate{Reconciliation: rc.RuntimeCWIReconciliationLedger{Request: &request}}
	next := before.CloneRaw()
	next.Reconciliation.Batch = &rc.RuntimeCWIReconciliationBatch{Observations: []rc.RuntimeCWIReconciliationObservation{{BindingID: "binding", ValidationExecution: &rc.RuntimeValidationExecutionUse{SourceReceiptHash: "forged", SourceScopeHash: "forged"}}}}
	if err := validateReconciliationExecutionTransition(before, next); err == nil {
		t.Fatal("new self-hashed execution gained authority")
	}
}
