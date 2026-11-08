package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
)

func TestNativeAssertionBodyPreservesBytesAndBindsReceipt(t *testing.T) {
	body := "expected 15, got 13\r\n        exact 原文"
	digest := sha256.Sum256([]byte(strings.TrimSpace(body)))
	o := RuntimeValidationObservation{FailedChecks: []string{"native"}, FailureDetailProtocolRef: RuntimeValidationFailureDetailProtocol, CheckFailures: []RuntimeValidationCheckFailure{{CheckRef: "native", TargetRef: "input.go", Line: 5, DiagnosticHash: hex.EncodeToString(digest[:]), ComparisonHash: strings.Repeat("a", 64), AssertionText: body}}}
	if err := o.ValidateFailureDetails(); err != nil {
		t.Fatal(err)
	}
	if ProjectRuntimeValidationExecutionFeedback(&o).FailureDetailsIncomplete {
		t.Fatal("ordinary native assertion requires a private shape marker")
	}
	raw, _ := json.Marshal(o)
	var restored RuntimeValidationObservation
	if err := json.Unmarshal(raw, &restored); err != nil || restored.CheckFailures[0].AssertionText != body {
		t.Fatal("raw body changed", err)
	}
	hash := StableRuntimeOperationReceiptHash(RuntimeOperationReceipt{Evidence: RuntimeOperationEvidence{Validation: &o}})
	restored.CheckFailures[0].AssertionText += " "
	// DiagnosticHash is the established trimmed diagnostic identity; the receipt
	// independently binds the exact body, including native whitespace.
	if restored.ValidateFailureDetails() != nil || hash == StableRuntimeOperationReceiptHash(RuntimeOperationReceipt{Evidence: RuntimeOperationEvidence{Validation: &restored}}) {
		t.Fatal("receipt did not bind exact native bytes")
	}
	for name, change := range map[string]func(*RuntimeValidationObservation){
		"wrong digest": func(o *RuntimeValidationObservation) { o.CheckFailures[0].AssertionText = "other" },
		"utf8":         func(o *RuntimeValidationObservation) { o.CheckFailures[0].AssertionText = string([]byte{0xff}) },
		"oversize": func(o *RuntimeValidationObservation) {
			o.CheckFailures[0].AssertionText = strings.Repeat("a", RuntimeValidationAssertionMaxBytes+1)
		},
	} {
		t.Run(name, func(t *testing.T) {
			bad := o.Clone()
			change(&bad)
			if bad.ValidateFailureDetails() == nil {
				t.Fatal("invalid native body admitted")
			}
		})
	}
	missing := o.Clone()
	missing.CheckFailures[0].AssertionText = ""
	if !ProjectRuntimeValidationExecutionFeedback(&missing).FailureDetailsIncomplete {
		t.Fatal("hash alone appeared complete")
	}
}
