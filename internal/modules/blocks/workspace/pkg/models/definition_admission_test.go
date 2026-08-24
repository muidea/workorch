package workspace

import (
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestDefinitionAdmissionCandidateHashIsPermutationStable(t *testing.T) {
	base := DefinitionAdmissionCandidate{
		SchemaVersion: DefinitionAdmissionCandidateSchemaVersionCurrent, WorkspaceID: "workspace-1", CandidateID: "candidate-1", CandidateRevision: 1,
		GoalInput:                   DefinitionCandidateItem{CandidateItemRef: "goal", Text: "goal", SourceIdentity: "user"},
		DoDInputs:                   []DefinitionCandidateItem{{CandidateItemRef: "dod-b", Text: "B", SourceIdentity: "user"}, {CandidateItemRef: "dod-a", Text: "A", SourceIdentity: "user"}},
		WorkspaceTemplateSnapshot:   DefinitionTemplateSnapshot{TemplateRef: "template", Revision: "r1", SnapshotHash: "h1"},
		AdmissionPolicyBindings:     []DefinitionAdmissionPolicyBinding{{PolicyRef: "b", Revision: "1", SnapshotHash: "b1"}, {PolicyRef: "a", Revision: "1", SnapshotHash: "a1"}},
		RuntimeProfileContractsHash: runtimecontract.StableRuntimeProfileContractsHash(nil),
		AdmissionCompilerVersion:    DefinitionAdmissionCompilerVersionCurrent, ResolvedProjectWorkspaceDirectory: "/tmp/workspace-1",
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
	startup := WorkspaceStartupInputContract{WorkspaceID: "workspace-1", DefinitionRevision: "definition-1", GoalDefinitionRef: "goal", GoalTextHash: "goal-hash", DoDDefinitionRefs: []string{"dod"}, DoDItemSetHash: "dod-hash", WorkspaceTemplateRef: "template", TemplateRevision: "r1", TemplateSnapshotHash: "template-hash", RuntimeProfileContractsHash: runtimecontract.StableRuntimeProfileContractsHash(nil), ResolvedProjectWorkspaceDirectory: "/tmp/workspace-1", CandidateItemToItemRefMappings: []CandidateItemToItemRefMapping{{CandidateItemRef: "goal", ItemRef: "goal"}, {CandidateItemRef: "dod", ItemRef: "dod"}}}.WithHash()
	pending := WorkspaceActivationAdmission{WorkspaceID: "workspace-1", CandidateID: "candidate-1", CandidateRevision: 1, CandidateHash: "candidate-hash", DefinitionRevision: startup.DefinitionRevision, StartupInputHash: startup.StartupInputHash, State: WorkspaceActivationPendingClarificationResolution, BlockingTicketID: "ticket-1"}
	if err := pending.Validate(startup); err != nil {
		t.Fatalf("pending admission: %v", err)
	}
	pending.State = WorkspaceActivationEligible
	if err := pending.Validate(startup); err == nil {
		t.Fatal("eligible clarified admission accepted without resolution acknowledgement")
	}
}

func TestDefinitionAdmissionCandidateHashSealsOwnerFactAcquisitionSelector(t *testing.T) {
	requirement := runtimecontract.RuntimeOwnerFactRequirement{
		OwnerModuleID: runtimecontract.RuntimeModuleShell,
		FactKind:      runtimecontract.RuntimeOwnerFactSelectableValidationSurface,
		OperationKind: runtimecontract.RuntimeBlockKindManagedValidationSession,
	}
	sourceRef := "profile/software-dev/validation-preparation"
	candidate := (DefinitionAdmissionCandidate{
		SchemaVersion: DefinitionAdmissionCandidateSchemaVersionCurrent, WorkspaceID: "workspace-selector", CandidateID: "candidate-selector", CandidateRevision: 1,
		GoalInput:                 DefinitionCandidateItem{CandidateItemRef: "goal", Text: "goal", SourceIdentity: "goal:goal-1@goal-r1"},
		DoDInputs:                 []DefinitionCandidateItem{{CandidateItemRef: "dod", Text: "done", SourceIdentity: "dod:dod-1@dod-r1"}},
		WorkspaceTemplateSnapshot: DefinitionTemplateSnapshot{TemplateRef: "software-dev", Revision: "r1", SnapshotHash: "template-hash"},
		RequiredOwnerFactBindings: []DefinitionRequiredOwnerFactBinding{{SourceRef: sourceRef, Requirement: requirement}},
		OwnerFactConvergenceBindings: []DefinitionOwnerFactConvergenceBinding{{SourceRef: sourceRef, Convergence: runtimecontract.RuntimeOwnerFactConvergence{
			Requirement: requirement, MissingTrigger: runtimecontract.RuntimeOwnerFactMissingTrigger,
			AcquisitionOwnerModuleID: runtimecontract.RuntimeModuleFile, AcquisitionOperationKind: runtimecontract.StepKindFileWrite,
			AcquisitionTargetPolicy: runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget, AcquisitionTargetSelectorRef: "workspace-relative-new-file-target",
			TargetSelectorRef: "file/workspace_deliverable", TargetSelectorOwnerRef: "software-dev", TargetSelectorSubjectRef: "validation-preparation",
			ReobserveOwnerModuleID: runtimecontract.RuntimeModuleShell, ReobserveFactKind: runtimecontract.RuntimeOwnerFactSelectableValidationSurface,
			MaxAcquisitionAttempts: 1,
		}}},
		RuntimeProfileContractsHash: runtimecontract.StableRuntimeProfileContractsHash(nil),
		AdmissionCompilerVersion:    DefinitionAdmissionCompilerVersionCurrent, ResolvedProjectWorkspaceDirectory: "/tmp/workspace-selector",
		ProposedRelationInputs: []DefinitionProposedRelationInput{{RelationRef: "terminal-all-of", RelationKind: "all_of", SourceItemRefs: []string{"dod"}, TargetItemRefs: []string{"goal"}}},
	}).WithHash()
	if err := candidate.Validate(); err != nil {
		t.Fatalf("current candidate: %v", err)
	}

	tests := []struct {
		name   string
		tamper func(*runtimecontract.RuntimeOwnerFactConvergence)
	}{
		{name: "operation variant", tamper: func(value *runtimecontract.RuntimeOwnerFactConvergence) {
			value.AcquisitionOperationVariantID = "unexpected"
		}},
		{name: "target policy", tamper: func(value *runtimecontract.RuntimeOwnerFactConvergence) {
			value.AcquisitionTargetPolicy = runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope
		}},
		{name: "operation selector", tamper: func(value *runtimecontract.RuntimeOwnerFactConvergence) {
			value.AcquisitionTargetSelectorRef = "file/unique_workspace_entry"
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tampered := candidate.Clone()
			test.tamper(&tampered.OwnerFactConvergenceBindings[0].Convergence)
			if err := tampered.Validate(); err == nil {
				t.Fatal("tampered acquisition selector retained the sealed candidate hash")
			}
		})
	}
}

func TestWorkspaceActivationAdmissionPersistenceRequiresCurrentExactCandidate(t *testing.T) {
	candidate := (DefinitionAdmissionCandidate{
		SchemaVersion: DefinitionAdmissionCandidateSchemaVersionCurrent, WorkspaceID: "workspace-current", CandidateID: "candidate-current", CandidateRevision: 1,
		GoalInput:                   DefinitionCandidateItem{CandidateItemRef: "goal", Text: "goal", SourceIdentity: "goal:goal-1@goal-r1"},
		DoDInputs:                   []DefinitionCandidateItem{{CandidateItemRef: "dod", Text: "done", SourceIdentity: "dod:dod-1@dod-r1"}},
		WorkspaceTemplateSnapshot:   DefinitionTemplateSnapshot{TemplateRef: "template", Revision: "r1", SnapshotHash: "template-hash"},
		RuntimeProfileContractsHash: runtimecontract.StableRuntimeProfileContractsHash(nil), AdmissionCompilerVersion: DefinitionAdmissionCompilerVersionCurrent,
		ResolvedProjectWorkspaceDirectory: "/tmp/workspace-current",
		ProposedRelationInputs:            []DefinitionProposedRelationInput{{RelationRef: "terminal-all-of", RelationKind: "all_of", SourceItemRefs: []string{"dod"}, TargetItemRefs: []string{"goal"}}},
	}).WithHash()
	startup := (WorkspaceStartupInputContract{
		WorkspaceID: candidate.WorkspaceID, DefinitionRevision: candidate.GoalInput.SourceIdentity + "|" + candidate.DoDInputs[0].SourceIdentity,
		GoalDefinitionRef: "goal-item", GoalTextHash: StableDefinitionTextHash(candidate.GoalInput.Text),
		DoDDefinitionRefs: []string{"dod-item"}, DoDItemSetHash: StableDefinitionStringSetHash([]string{candidate.DoDInputs[0].Text}),
		WorkspaceTemplateRef: candidate.WorkspaceTemplateSnapshot.TemplateRef, TemplateRevision: candidate.WorkspaceTemplateSnapshot.Revision,
		TemplateSnapshotHash: candidate.WorkspaceTemplateSnapshot.SnapshotHash, RuntimeProfileContractsHash: candidate.RuntimeProfileContractsHash,
		ResolvedProjectWorkspaceDirectory: candidate.ResolvedProjectWorkspaceDirectory,
		CandidateItemToItemRefMappings:    []CandidateItemToItemRefMapping{{CandidateItemRef: "goal", ItemRef: "goal-item"}, {CandidateItemRef: "dod", ItemRef: "dod-item"}},
	}).WithHash()
	admission := WorkspaceActivationAdmission{
		WorkspaceID: candidate.WorkspaceID, CandidateID: candidate.CandidateID, CandidateRevision: candidate.CandidateRevision, CandidateHash: candidate.CandidateHash,
		DefinitionRevision: startup.DefinitionRevision, StartupInputHash: startup.StartupInputHash, State: WorkspaceActivationEligible,
	}
	workspace := &Workspace{ID: candidate.WorkspaceID, DefinitionAdmissionCandidates: []DefinitionAdmissionCandidate{candidate}, StartupInput: &startup, ActivationAdmission: &admission}
	if _, present, err := workspace.ValidateActivationAdmissionPersistence(); err != nil || !present {
		t.Fatalf("current admission persistence present=%t err=%v", present, err)
	}
	missing := &Workspace{ID: candidate.WorkspaceID, StartupInput: &startup, ActivationAdmission: &admission}
	if _, _, err := missing.ValidateActivationAdmissionPersistence(); err == nil {
		t.Fatal("activation admission accepted a missing candidate")
	}
	duplicate := &Workspace{ID: candidate.WorkspaceID, DefinitionAdmissionCandidates: []DefinitionAdmissionCandidate{candidate, candidate.Clone()}, StartupInput: &startup, ActivationAdmission: &admission}
	if _, _, err := duplicate.ValidateActivationAdmissionPersistence(); err == nil {
		t.Fatal("activation admission accepted a duplicated candidate identity")
	}
	drifted := candidate.Clone()
	drifted.ResolvedProjectWorkspaceDirectory = "/tmp/drifted"
	driftedWorkspace := &Workspace{ID: candidate.WorkspaceID, DefinitionAdmissionCandidates: []DefinitionAdmissionCandidate{drifted}, StartupInput: &startup, ActivationAdmission: &admission}
	if _, _, err := driftedWorkspace.ValidateActivationAdmissionPersistence(); err == nil {
		t.Fatal("activation admission accepted candidate payload drift under the sealed hash")
	}

	for _, test := range []struct {
		name   string
		mutate func(*DefinitionAdmissionCandidate)
	}{
		{name: "schema", mutate: func(value *DefinitionAdmissionCandidate) { value.SchemaVersion-- }},
		{name: "compiler", mutate: func(value *DefinitionAdmissionCandidate) {
			value.AdmissionCompilerVersion = "workorch-definition-admission-v2"
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			invalid := candidate.Clone()
			test.mutate(&invalid)
			invalid = invalid.WithHash()
			invalidAdmission := admission
			invalidAdmission.CandidateHash = invalid.CandidateHash
			invalidWorkspace := &Workspace{ID: candidate.WorkspaceID, DefinitionAdmissionCandidates: []DefinitionAdmissionCandidate{invalid}, StartupInput: &startup, ActivationAdmission: &invalidAdmission}
			if _, _, err := invalidWorkspace.ValidateActivationAdmissionPersistence(); err == nil {
				t.Fatal("non-current candidate shape was accepted by activation admission")
			}
		})
	}
}
