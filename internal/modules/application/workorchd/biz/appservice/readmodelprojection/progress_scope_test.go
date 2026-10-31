package readmodelprojection

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeProgressKeepsWholeRunValidationWhenFocusAdvances(t *testing.T) {
	state := runtimecontract.RuntimeStateRecord{Facts: runtimecontract.RuntimeFacts{
		Focus: runtimecontract.CoverageFocus{ID: "focus-b", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-b"}},
		Evidence: runtimecontract.EvidenceLedger{
			Accepted: []runtimecontract.EvidenceFact{
				{ID: "implementation", Kind: runtimecontract.EvidenceKindStateChange, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-a"}},
				{ID: "proved-a", Kind: runtimecontract.EvidenceKindValidation, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-a"}},
				{ID: "uncredited-aux", Kind: runtimecontract.EvidenceKindValidation},
			},
			Rejected: []runtimecontract.EvidenceFact{{ID: "failed-a", Kind: runtimecontract.EvidenceKindValidation, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-a"}}},
		},
	}}
	before, _ := json.Marshal(state)
	progress := mapRuntimeProgress(state)
	want := &projectioncontract.RuntimeEvidenceCounts{Accepted: 3, AcceptedValidation: 2, Rejected: 1, RejectedValidation: 1}
	if !reflect.DeepEqual(progress.RunEvidence, want) || progress.AcceptedValidationEvidenceCount != 0 {
		t.Fatalf("focus/Run counters disagree with their scope: %+v", progress)
	}
	text := runtimeProgressSummary(&progress)
	for _, fragment := range []string{"whole Run:", "validation accepted 2 rejected 1", "current focus:", "CWI-credited validation 0"} {
		if !strings.Contains(text, fragment) {
			t.Fatalf("summary omitted %q: %s", fragment, text)
		}
	}
	after, _ := json.Marshal(state)
	if string(before) != string(after) {
		t.Fatal("read model mutated Runtime authority")
	}
	clone := projectioncontract.CloneRuntimeProgress(&progress)
	clone.RunEvidence.AcceptedValidation = 100
	if progress.RunEvidence.AcceptedValidation != 2 {
		t.Fatal("run totals alias their clone")
	}
	legacy := projectioncontract.RuntimeProgress{AcceptedValidationEvidenceCount: 2}
	if text := runtimeProgressSummary(&legacy); strings.Contains(text, "validation accepted 0 rejected 0") {
		t.Fatal("legacy missing totals became zero progress")
	}
}

func TestTerminalReadinessCauseProjectsWithoutFailureFactOrAuthorityMutation(t *testing.T) {
	diagnostic := &runtimecontract.RuntimeOriginalDiagnostic{SchemaVersion: runtimecontract.RuntimeOriginalDiagnosticSchemaCurrent, OwnerModuleID: runtimecontract.RuntimeModuleShell,
		OriginalClass: "validation_readiness", OriginalCode: "native_check_missing", OriginalMessage: "missing exact native check", Signature: "readiness/check", TargetRefs: []string{"checks/cwi.test"}}
	state := runtimecontract.RuntimeStateRecord{Decision: runtimecontract.RuntimeDecision{State: runtimecontract.ConvergenceStateFailed,
		Terminal: &runtimecontract.RuntimeTerminalDecision{Code: "owner_fact_convergence_exhausted", OriginalDiagnostic: diagnostic}}}
	progress := mapRuntimeProgress(state)
	audit := mapRuntimeAudit(&state)
	for _, got := range []*runtimecontract.RuntimeOriginalDiagnostic{progress.OriginalDiagnostic, audit.Terminal.OriginalDiagnostic} {
		if !reflect.DeepEqual(got, diagnostic) || got == diagnostic {
			t.Fatal("terminal projection lost/aliased cause")
		}
	}
	if progress.TerminalCode != "owner_fact_convergence_exhausted" || state.Facts.Failure != nil || audit.Failure != nil {
		t.Fatal("projection rebuilt a FailureFact")
	}
	clone := projectioncontract.CloneRuntimeAudit(audit)
	clone.Terminal.OriginalDiagnostic.TargetRefs[0] = "other"
	if audit.Terminal.OriginalDiagnostic.TargetRefs[0] != "checks/cwi.test" {
		t.Fatal("terminal audit clone aliased cause")
	}
}

func TestRuntimeImplementationProgressSeparatesReceiptsRoundsAndValidation(t *testing.T) {
	receipt := runtimecontract.RuntimeOperationReceipt{OperationID: "operation", ExecutionKey: "execution", Fence: 1, Outcome: runtimecontract.RuntimeOperationOutcomeAccepted, Changed: true}
	state := runtimecontract.RuntimeStateRecord{
		ImplementationWork:  runtimecontract.RuntimeImplementationWorkLedger{Rounds: []runtimecontract.RuntimeImplementationWorkRound{{DispatchID: "work-dispatch", CompletedReceiptSetHash: "complete"}, {DispatchID: "pending-dispatch", OutcomeUnknownExecutionKey: "unknown-execution", OutcomeUnknownFence: 1}}},
		CompletedDispatches: []runtimecontract.RuntimeDispatchRecord{{ID: "work-dispatch", OperationExecutions: []runtimecontract.RuntimeOperationExecutionRecord{{OperationID: receipt.OperationID, ExecutionKey: receipt.ExecutionKey, Fence: 1, State: runtimecontract.OperationExecutionEvidenceCommitted, OwnerReceiptHash: runtimecontract.StableRuntimeOperationReceiptHash(receipt)}}}},
		ActiveDispatch:      &runtimecontract.RuntimeDispatchRecord{ID: "pending-dispatch", OperationExecutions: []runtimecontract.RuntimeOperationExecutionRecord{{OperationID: "unknown-operation", State: runtimecontract.OperationExecutionOutcomeUnknown}}},
		OperationReceipts:   []runtimecontract.RuntimeOperationReceipt{receipt},
	}
	before, _ := json.Marshal(state)
	progress := mapRuntimeProgress(state)
	want := &projectioncontract.RuntimeImplementationProgress{ReservedRounds: 2, CompletedRounds: 1, CommittedReceipts: 1, UnknownExecutions: 1}
	if !reflect.DeepEqual(progress.ImplementationWork, want) || progress.RunEvidence.AcceptedValidation != 0 || progress.SatisfiedCWICount != 0 {
		t.Fatalf("implementation became acceptance progress: %+v", progress)
	}
	for _, fragment := range []string{"rounds completed 1/2", "Runtime receipts 1", "unknown executions 1", "work is not validation credit"} {
		if !strings.Contains(runtimeProgressSummary(&progress), fragment) {
			t.Fatalf("summary missing %q", fragment)
		}
	}
	state.StateRevision++
	if !reflect.DeepEqual(mapRuntimeProgress(state).ImplementationWork, want) {
		t.Fatal("scheduling revision fabricated progress")
	}
	state.StateRevision--
	after, _ := json.Marshal(state)
	if string(before) != string(after) {
		t.Fatal("projection mutated state")
	}
	clone := projectioncontract.CloneRuntimeProgress(&progress)
	clone.ImplementationWork.CompletedRounds = 99
	if progress.ImplementationWork.CompletedRounds != 1 {
		t.Fatal("workspace progress clones alias")
	}
	state.ActiveDispatch.OperationExecutions[0].State = runtimecontract.OperationExecutionOwnerCommitted
	if runtimeImplementationProgress(state).UnknownExecutions != 1 {
		t.Fatal("late known receipt erased the original unknown round")
	}
	state.OperationReceipts[0].AfterHash = "receipt-drift"
	if runtimeImplementationProgress(state).CommittedReceipts != 0 {
		t.Fatal("receipt count trusted a mismatching body")
	}
}

func TestRuntimeProgressTerminalAcknowledgementsRequireExactIdentity(t *testing.T) {
	state := runtimecontract.RuntimeStateRecord{Decision: runtimecontract.RuntimeDecision{
		State:             runtimecontract.ConvergenceStateFailed,
		AuthoritySnapshot: runtimecontract.AuthoritySnapshot{ID: "authority", WorkspaceID: "workspace", RunID: "run"},
		Terminal:          &runtimecontract.RuntimeTerminalDecision{Code: "original-failure", Summary: "original failure"},
	}}
	outcome, found, err := runtimecontract.DeriveRuntimeTerminalOutcome(state.Decision, nil)
	if err != nil || !found {
		t.Fatal(err)
	}
	deliveryID := runtimecontract.StableTerminalDeliveryID("workspace", "run", "authority", outcome)
	for i, owner := range []runtimecontract.RuntimeDeliveryOwner{runtimecontract.RuntimeDeliveryOwnerResult, runtimecontract.RuntimeDeliveryOwnerWorkspace, runtimecontract.RuntimeDeliveryOwnerRun} {
		ack := runtimecontract.RuntimeDeliveryAcknowledgement{WorkspaceID: "workspace", RunID: "run", TerminalDeliveryID: deliveryID, Owner: owner,
			AuthoritySnapshotID: "authority", CausationID: "terminal-delivery/" + deliveryID, Outcome: outcome}
		switch owner {
		case runtimecontract.RuntimeDeliveryOwnerResult:
			ack.ResultID = runtimecontract.RuntimeTerminalResultID("workspace", "run")
			ack.AcknowledgedState = runtimecontract.RuntimeDeliveryAcknowledgedResultRecorded
		case runtimecontract.RuntimeDeliveryOwnerWorkspace:
			ack.AcknowledgedState = runtimecontract.RuntimeDeliveryAcknowledgedWorkspaceReleased
		case runtimecontract.RuntimeDeliveryOwnerRun:
			ack.AcknowledgedState = runtimecontract.RuntimeDeliveryAcknowledgedRunTerminal
		}
		state.DeliveryAcknowledgements = append(state.DeliveryAcknowledgements, ack)
		progress := mapRuntimeProgress(state)
		if len(progress.TerminalAcknowledgedOwners) != i+1 {
			t.Fatal("exact persisted ACK omitted")
		}
		clone := projectioncontract.CloneRuntimeProgress(&progress)
		clone.TerminalAcknowledgedOwners[0] = "other-owner"
		if progress.TerminalAcknowledgedOwners[0] == "other-owner" {
			t.Fatal("ACK projection clone aliases")
		}
	}
	state.DeliveryAcknowledgements[2].RunID = "another-run"
	if len(mapRuntimeProgress(state).TerminalAcknowledgedOwners) != 0 {
		t.Fatal("cross-run ACK became completion")
	}
}
