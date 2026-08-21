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
