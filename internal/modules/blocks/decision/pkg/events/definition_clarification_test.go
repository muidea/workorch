package events

import (
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func workspaceClarificationRequestForTest(revision uint64, candidateHash, signature string) OpenWorkspaceDefinitionClarificationRequestCommand {
	cmd := OpenWorkspaceDefinitionClarificationRequestCommand{
		SchemaVersion: DefinitionClarificationSchemaVersionCurrent, WorkspaceID: "workspace-1", CandidateID: "workspace-definition", CandidateRevision: revision,
		CandidateHash: candidateHash, ViolationCode: "definition_one_of_unsupported", OffendingRefs: []string{"relation-1"},
		CanonicalSignatureVersion: "v1", CanonicalSignature: signature, RequiredActionKind: DefinitionClarificationActionRewriteAllOf,
	}
	cmd.CanonicalSignatureHash = CanonicalDefinitionClarificationSignatureHash("workspace-signature", cmd.CanonicalSignatureVersion, cmd.CanonicalSignature)
	cmd.TicketID = StableWorkspaceDefinitionClarificationTicketID(cmd)
	return cmd
}

func TestReconcileWorkspaceDefinitionClarificationCandidateBindsExactRevisionPair(t *testing.T) {
	cmd := ReconcileWorkspaceDefinitionClarificationCandidateCommand{
		OldRequest: workspaceClarificationRequestForTest(1, "candidate-hash-1", "one-of:a"),
		NewRequest: workspaceClarificationRequestForTest(2, "candidate-hash-2", "one-of:b"),
	}
	if err := cmd.Validate(); err != nil {
		t.Fatalf("valid reconciliation: %v", err)
	}
	first := cmd.BindingHash("resolution-1")
	if first == "" || first != cmd.BindingHash("resolution-1") {
		t.Fatalf("reconciliation hash is not stable: %q", first)
	}
	stale := cmd
	stale.NewRequest.CandidateRevision = stale.OldRequest.CandidateRevision
	stale.NewRequest.TicketID = StableWorkspaceDefinitionClarificationTicketID(stale.NewRequest)
	if err := stale.Validate(); err == nil {
		t.Fatal("non-advancing invalid candidate reconciliation was accepted")
	}
}

func TestWorkspaceDefinitionClarificationResultsValidateDeterministicBindings(t *testing.T) {
	openCommand := workspaceClarificationRequestForTest(1, "candidate-hash-1", "one-of:a")
	opened := OpenWorkspaceDefinitionClarificationRequestResult{
		TicketID: openCommand.TicketID, WorkspaceID: openCommand.WorkspaceID, CandidateID: openCommand.CandidateID, CandidateRevision: openCommand.CandidateRevision,
		Disposition: DefinitionClarificationCreated, ClarificationRequestBindingHash: openCommand.BindingHash(),
	}
	if err := opened.ValidateFor(openCommand); err != nil {
		t.Fatalf("valid open result: %v", err)
	}
	forgedOpen := opened
	forgedOpen.ClarificationRequestBindingHash = "forged"
	if err := forgedOpen.ValidateFor(openCommand); err == nil {
		t.Fatal("forged open result binding was accepted")
	}

	newCommand := workspaceClarificationRequestForTest(2, "candidate-hash-2", "one-of:b")
	reconcileCommand := ReconcileWorkspaceDefinitionClarificationCandidateCommand{OldRequest: openCommand, NewRequest: newCommand}
	reconciliationResolutionID := StableDefinitionClarificationResolutionID(openCommand.TicketID, newCommand.BindingHash())
	reconciled := ReconcileWorkspaceDefinitionClarificationCandidateResult{
		WorkspaceID: openCommand.WorkspaceID, OldTicketID: openCommand.TicketID, OldResolutionID: reconciliationResolutionID,
		NewTicketID: newCommand.TicketID, NewTicketDisposition: DefinitionClarificationCreated,
		NewRequestBindingHash: newCommand.BindingHash(), ReconciliationBindingHash: reconcileCommand.BindingHash(reconciliationResolutionID),
	}
	if err := reconciled.ValidateFor(reconcileCommand); err != nil {
		t.Fatalf("valid reconciliation result: %v", err)
	}
	forgedReconciliation := reconciled
	forgedReconciliation.NewTicketID = "unrelated-ticket"
	if err := forgedReconciliation.ValidateFor(reconcileCommand); err == nil {
		t.Fatal("mismatched reconciliation result was accepted")
	}

	resolveCommand := ResolveWorkspaceDefinitionClarificationByAdmissionCommand{
		TicketID: openCommand.TicketID, WorkspaceID: openCommand.WorkspaceID,
		OldCandidateID: openCommand.CandidateID, OldCandidateRevision: openCommand.CandidateRevision, OldCandidateHash: openCommand.CandidateHash,
		NewCandidateID: newCommand.CandidateID, NewCandidateRevision: newCommand.CandidateRevision, NewCandidateHash: newCommand.CandidateHash,
		AdmissionResultHash: "admission-result-hash", DefinitionRevision: "definition-r2", StartupInputHash: "startup-r2",
	}
	seed := resolveCommand.AdmissionResultHash + "\x00" + resolveCommand.StartupInputHash
	resolutionID := StableDefinitionClarificationResolutionID(resolveCommand.TicketID, seed)
	resolved := ResolveWorkspaceDefinitionClarificationByAdmissionResult{
		TicketID: resolveCommand.TicketID, WorkspaceID: resolveCommand.WorkspaceID, ResolutionID: resolutionID,
		ResolutionType: DefinitionClarificationResolutionCandidateAdjusted, Disposition: DefinitionClarificationResolved,
		ResolutionBindingHash: resolveCommand.BindingHash(resolutionID),
	}
	if err := resolved.ValidateFor(resolveCommand); err != nil {
		t.Fatalf("valid resolution result: %v", err)
	}
	forgedResolution := resolved
	forgedResolution.ResolutionID = "forged-resolution"
	if err := forgedResolution.ValidateFor(resolveCommand); err == nil {
		t.Fatal("mismatched resolution result was accepted")
	}
}

func runtimeReplacementResolutionCommandForTest() ResolveRuntimeDefinitionClarificationByReplacementCommand {
	return ResolveRuntimeDefinitionClarificationByReplacementCommand{
		TicketID: "runtime-definition-clarification:ticket-1", WorkspaceID: "workspace-1", PredecessorRunID: "run-1",
		ReplacementIntentID: "runtime-definition-replacement/restart-1", PredecessorDefinitionRevision: "definition-1",
		PredecessorStartupInputHash: "startup-old", SuccessorDefinitionRevision: "definition-2",
		SuccessorStartupInputHash: "startup-new", ReplacementIntentHash: "replacement-hash-1",
	}
}

func TestRuntimeReplacementResolutionResultValidatesExactDispositionAndBinding(t *testing.T) {
	cmd := runtimeReplacementResolutionCommandForTest()
	resolutionID := common.ResolutionID("resolution-1")
	resolved := ResolveRuntimeDefinitionClarificationByReplacementResult{
		TicketID: cmd.TicketID, WorkspaceID: cmd.WorkspaceID, PredecessorRunID: cmd.PredecessorRunID,
		ReplacementIntentID: cmd.ReplacementIntentID, ResolutionID: resolutionID,
		ResolutionType: DefinitionClarificationResolutionReplacementBound, Disposition: DefinitionClarificationDuplicate,
		ConflictKind:          RuntimeDefinitionClarificationNoConflict,
		ResolutionBindingHash: cmd.BindingHash(resolutionID, DefinitionClarificationResolved, RuntimeDefinitionClarificationNoConflict, "", ""),
	}
	if err := resolved.ValidateFor(cmd); err != nil {
		t.Fatalf("valid duplicate resolution rejected: %v", err)
	}

	stale := ResolveRuntimeDefinitionClarificationByReplacementResult{
		TicketID: cmd.TicketID, WorkspaceID: cmd.WorkspaceID, PredecessorRunID: cmd.PredecessorRunID,
		ReplacementIntentID: cmd.ReplacementIntentID, ResolutionType: DefinitionClarificationResolutionReplacementBound,
		Disposition: DefinitionClarificationStale, ConflictKind: RuntimeDefinitionClarificationTicketAlreadyBound,
		CurrentReplacementIntentID: "runtime-definition-replacement/restart-current", CurrentReplacementIntentHash: "replacement-hash-current",
	}
	stale.ResolutionBindingHash = cmd.BindingHash("", stale.Disposition, stale.ConflictKind, stale.CurrentReplacementIntentID, stale.CurrentReplacementIntentHash)
	if err := stale.ValidateFor(cmd); err != nil {
		t.Fatalf("valid stale resolution rejected: %v", err)
	}

	forged := stale
	forged.ResolutionBindingHash = "forged"
	if err := forged.ValidateFor(cmd); err == nil {
		t.Fatal("forged stale resolution binding was accepted")
	}
	forged = resolved
	forged.CurrentReplacementIntentID = "unexpected-current-intent"
	if err := forged.ValidateFor(cmd); err == nil {
		t.Fatal("successful resolution carrying stale identity was accepted")
	}
	unchanged := cmd
	unchanged.SuccessorDefinitionRevision = unchanged.PredecessorDefinitionRevision
	if err := unchanged.Validate(); err == nil {
		t.Fatal("runtime clarification accepted a replacement without a new Definition revision")
	}
}
