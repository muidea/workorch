package events

import (
	"strings"
	"testing"
	"time"

	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestExecutionReceiptQueryContractPreservesUnknownAndExactIdentity(t *testing.T) {
	c := QueryWorkspaceExecutionIntentReceiptCommand{WorkspaceID: "ws", RequestID: "request", PayloadHash: strings.Repeat("a", 64)}
	r := QueryWorkspaceExecutionIntentReceiptResult{WorkspaceID: c.WorkspaceID, RequestID: c.RequestID, PayloadHash: c.PayloadHash, ObservedAt: time.Now().UTC()}
	if r.ValidateFor(c) != nil || r.Clone().Receipt != nil {
		t.Fatal("unknown is not a valid explicit read result")
	}
	r.Receipt = &runmodel.WorkspaceExecutionIntentReceipt{Version: runmodel.WorkspaceExecutionIntentReceiptVersion, PayloadHash: c.PayloadHash, RecordedAt: time.Now().UTC(), Admission: runtimecontract.RuntimeExecutionIntentAdmission{
		WorkspaceID: "ws", RunID: "run-001", RequestID: "request", Kind: runtimecontract.RuntimeExecutionIntentRestart,
		Outcome: runtimecontract.RuntimeExecutionIntentReconciliationRequested, Reason: runtimecontract.RuntimeExecutionIntentReasonNewRunActivated,
	}}
	if r.ValidateFor(c) != nil {
		t.Fatal("exact original receipt rejected")
	}
	for name, mutate := range map[string]func(*QueryWorkspaceExecutionIntentReceiptResult){
		"workspace":          func(v *QueryWorkspaceExecutionIntentReceiptResult) { v.WorkspaceID = "other" },
		"request":            func(v *QueryWorkspaceExecutionIntentReceiptResult) { v.RequestID = "other" },
		"hash":               func(v *QueryWorkspaceExecutionIntentReceiptResult) { v.PayloadHash = strings.Repeat("b", 64) },
		"time":               func(v *QueryWorkspaceExecutionIntentReceiptResult) { v.ObservedAt = time.Time{} },
		"original_workspace": func(v *QueryWorkspaceExecutionIntentReceiptResult) { v.Receipt.Admission.WorkspaceID = "other" },
		"original_request":   func(v *QueryWorkspaceExecutionIntentReceiptResult) { v.Receipt.Admission.RequestID = "other" },
		"original_hash":      func(v *QueryWorkspaceExecutionIntentReceiptResult) { v.Receipt.PayloadHash = strings.Repeat("b", 64) },
		"original_time":      func(v *QueryWorkspaceExecutionIntentReceiptResult) { v.Receipt.RecordedAt = time.Time{} },
	} {
		t.Run(name, func(t *testing.T) {
			bad := r.Clone()
			mutate(&bad)
			if bad.ValidateFor(c) == nil {
				t.Fatal("drifted original receipt accepted")
			}
			if r.ValidateFor(c) != nil {
				t.Fatal("clone aliases original receipt")
			}
		})
	}
	for _, request := range []string{"", " request", "request\n", string([]byte{0xff}), strings.Repeat("r", 257)} {
		bad := c
		bad.RequestID = request
		if bad.Validate() == nil {
			t.Fatal("invalid query request accepted")
		}
	}
	for _, hash := range []string{"", "sha256:" + c.PayloadHash, strings.Repeat("A", 64), strings.Repeat("a", 63)} {
		bad := c
		bad.PayloadHash = hash
		if bad.Validate() == nil {
			t.Fatal("noncanonical payload hash accepted")
		}
	}
}
