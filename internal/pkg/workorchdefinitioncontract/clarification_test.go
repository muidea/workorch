package workorchdefinitioncontract

import "testing"

func workspaceClarificationCommandForTest(revision uint64, candidateHash, signature string) WorkspaceDefinitionClarificationRequestIdentity {
	command := WorkspaceDefinitionClarificationRequestIdentity{
		SchemaVersion: DefinitionClarificationSchemaVersionCurrent, WorkspaceID: "ws-1", CandidateID: "candidate-1", CandidateRevision: revision,
		CandidateHash: candidateHash, ViolationCode: "definition_one_of_unsupported", OffendingRefs: []string{" dod:item-1 ", "dod:item-1"},
		CanonicalSignatureVersion: "v1", CanonicalSignature: signature, RequiredActionKind: DefinitionClarificationActionRewriteAllOf,
	}
	command.CanonicalSignatureHash = CanonicalDefinitionClarificationSignatureHash("workspace-signature", command.CanonicalSignatureVersion, command.CanonicalSignature)
	command.TicketID = StableWorkspaceDefinitionClarificationTicketID(command)
	return command
}

func TestWorkspaceClarificationDTOValidatesExactCommandResultPair(t *testing.T) {
	command := workspaceClarificationCommandForTest(1, "candidate-hash-1", "one-of:item-1")
	result := WorkspaceDefinitionClarificationTicketBinding{
		TicketID: command.TicketID, WorkspaceID: command.WorkspaceID, CandidateID: command.CandidateID,
		CandidateRevision: command.CandidateRevision, Disposition: DefinitionClarificationCreated,
		ClarificationRequestBindingHash: command.BindingHash(),
	}
	if err := result.ValidateFor(command); err != nil {
		t.Fatalf("valid command/result pair: %v", err)
	}
	forged := result
	forged.ClarificationRequestBindingHash = "forged"
	if err := forged.ValidateFor(command); err == nil {
		t.Fatal("forged Decision result binding was accepted")
	}
}

func TestWorkspaceClarificationDTOExactCommandComparisonIncludesNormalizedOffendingRefs(t *testing.T) {
	expected := workspaceClarificationCommandForTest(1, "candidate-hash-1", "one-of:item-1")
	equivalent := expected
	equivalent.OffendingRefs = []string{"dod:item-1", " dod:item-1 "}
	if !equivalent.EqualNormalized(expected) {
		t.Fatal("normalization-equivalent offending refs did not compare equal")
	}
	forged := expected
	forged.OffendingRefs = []string{"dod:item-forged"}
	// Ticket identity deliberately remains unchanged: exact DTO matching must
	// still reject the forged field rather than trusting a ticket ID alone.
	if forged.TicketID != expected.TicketID {
		t.Fatal("test fixture unexpectedly changed ticket identity")
	}
	if forged.EqualNormalized(expected) {
		t.Fatal("forged offending refs compared equal")
	}
}

func TestWorkspaceClarificationDTOValidatesExactResolutionPair(t *testing.T) {
	oldRequest := workspaceClarificationCommandForTest(1, "candidate-hash-1", "one-of:item-1")
	newRequest := workspaceClarificationCommandForTest(2, "candidate-hash-2", "all-of:item-1")
	command := WorkspaceDefinitionClarificationAdmissionResolutionIdentity{
		TicketID: oldRequest.TicketID, WorkspaceID: oldRequest.WorkspaceID,
		OldCandidateID: oldRequest.CandidateID, OldCandidateRevision: oldRequest.CandidateRevision, OldCandidateHash: oldRequest.CandidateHash,
		NewCandidateID: newRequest.CandidateID, NewCandidateRevision: newRequest.CandidateRevision, NewCandidateHash: newRequest.CandidateHash,
		AdmissionResultHash: "admission-hash-2", DefinitionRevision: "definition-2", StartupInputHash: "startup-2",
	}
	resolutionID := StableDefinitionClarificationResolutionID(command.TicketID, command.AdmissionResultHash+"\x00"+command.StartupInputHash)
	result := WorkspaceDefinitionClarificationResolutionBinding{
		TicketID: command.TicketID, WorkspaceID: command.WorkspaceID, ResolutionID: resolutionID,
		ResolutionType: DefinitionClarificationResolutionCandidateAdjusted, Disposition: DefinitionClarificationResolved,
		ResolutionBindingHash: command.BindingHash(resolutionID),
	}
	if err := result.ValidateFor(command); err != nil {
		t.Fatalf("valid resolution pair: %v", err)
	}
	forged := result
	forged.ResolutionID = "forged"
	if err := forged.ValidateFor(command); err == nil {
		t.Fatal("forged resolution identity was accepted")
	}
}

func TestWorkspaceClarificationResolutionRequiresOneCandidateLineageAndIncreasingRevision(t *testing.T) {
	oldRequest := workspaceClarificationCommandForTest(1, "candidate-hash-1", "one-of:item-1")
	command := WorkspaceDefinitionClarificationAdmissionResolutionIdentity{
		TicketID: oldRequest.TicketID, WorkspaceID: oldRequest.WorkspaceID,
		OldCandidateID: oldRequest.CandidateID, OldCandidateRevision: oldRequest.CandidateRevision, OldCandidateHash: oldRequest.CandidateHash,
		NewCandidateID: oldRequest.CandidateID, NewCandidateRevision: oldRequest.CandidateRevision + 1, NewCandidateHash: "candidate-hash-2",
		AdmissionResultHash: "admission-hash-2", DefinitionRevision: "definition-2", StartupInputHash: "startup-2",
	}
	if err := command.Validate(); err != nil {
		t.Fatalf("valid same-lineage successor: %v", err)
	}
	changedLineage := command
	changedLineage.NewCandidateID = "candidate-forged"
	if err := changedLineage.Validate(); err == nil {
		t.Fatal("different candidate lineage was accepted")
	}
	nonIncreasing := command
	nonIncreasing.NewCandidateRevision = command.OldCandidateRevision
	if err := nonIncreasing.Validate(); err == nil {
		t.Fatal("non-increasing candidate revision was accepted")
	}
}
