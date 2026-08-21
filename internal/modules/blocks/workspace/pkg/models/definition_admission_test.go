package workspace

import "testing"

func TestDefinitionAdmissionCandidateHashIsPermutationStable(t *testing.T) {
	base := DefinitionAdmissionCandidate{
		SchemaVersion: DefinitionAdmissionCandidateSchemaVersionCurrent, WorkspaceID: "workspace-1", CandidateID: "candidate-1", CandidateRevision: 1,
		GoalInput:                 DefinitionCandidateItem{CandidateItemRef: "goal", Text: "goal", SourceIdentity: "user"},
		DoDInputs:                 []DefinitionCandidateItem{{CandidateItemRef: "dod-b", Text: "B", SourceIdentity: "user"}, {CandidateItemRef: "dod-a", Text: "A", SourceIdentity: "user"}},
		WorkspaceTemplateSnapshot: DefinitionTemplateSnapshot{TemplateRef: "template", Revision: "r1", SnapshotHash: "h1"},
		AdmissionPolicyBindings:   []DefinitionAdmissionPolicyBinding{{PolicyRef: "b", Revision: "1", SnapshotHash: "b1"}, {PolicyRef: "a", Revision: "1", SnapshotHash: "a1"}},
		AdmissionCompilerVersion:  DefinitionAdmissionCompilerVersionCurrent, ResolvedProjectWorkspaceDirectory: "/tmp/workspace-1",
		ProposedRelationInputs: []DefinitionProposedRelationInput{{RelationRef: "r1", RelationKind: "all_of", SourceItemRefs: []string{"dod-b", "dod-a"}, TargetItemRefs: []string{"goal"}}},
	}
	first := base.WithHash()
	base.DoDInputs[0], base.DoDInputs[1] = base.DoDInputs[1], base.DoDInputs[0]
	base.AdmissionPolicyBindings[0], base.AdmissionPolicyBindings[1] = base.AdmissionPolicyBindings[1], base.AdmissionPolicyBindings[0]
	base.ProposedRelationInputs[0].SourceItemRefs[0], base.ProposedRelationInputs[0].SourceItemRefs[1] = base.ProposedRelationInputs[0].SourceItemRefs[1], base.ProposedRelationInputs[0].SourceItemRefs[0]
	second := base.WithHash()
	if first.CandidateHash != second.CandidateHash {
		t.Fatalf("candidate hash drift: %q != %q", first.CandidateHash, second.CandidateHash)
	}
	if err := first.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
}

func TestWorkspaceActivationAdmissionRequiresClarificationAcknowledgement(t *testing.T) {
	startup := WorkspaceStartupInputContract{WorkspaceID: "workspace-1", DefinitionRevision: "definition-1", GoalDefinitionRef: "goal", GoalTextHash: "goal-hash", DoDDefinitionRefs: []string{"dod"}, DoDItemSetHash: "dod-hash", WorkspaceTemplateRef: "template", TemplateRevision: "r1", TemplateSnapshotHash: "template-hash", ResolvedProjectWorkspaceDirectory: "/tmp/workspace-1", CandidateItemToItemRefMappings: []CandidateItemToItemRefMapping{{CandidateItemRef: "goal", ItemRef: "goal"}, {CandidateItemRef: "dod", ItemRef: "dod"}}}.WithHash()
	pending := WorkspaceActivationAdmission{WorkspaceID: "workspace-1", CandidateID: "candidate-1", CandidateRevision: 1, CandidateHash: "candidate-hash", DefinitionRevision: startup.DefinitionRevision, StartupInputHash: startup.StartupInputHash, State: WorkspaceActivationPendingClarificationResolution, BlockingTicketID: "ticket-1"}
	if err := pending.Validate(startup); err != nil {
		t.Fatalf("pending admission: %v", err)
	}
	pending.State = WorkspaceActivationEligible
	if err := pending.Validate(startup); err == nil {
		t.Fatal("eligible clarified admission accepted without resolution acknowledgement")
	}
}
