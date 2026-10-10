package definitionadmission

import (
	"testing"

	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func candidateWithRelation(kind string) workspacemodel.DefinitionAdmissionCandidate {
	return (workspacemodel.DefinitionAdmissionCandidate{
		SchemaVersion: workspacemodel.DefinitionAdmissionCandidateSchemaVersionCurrent,
		WorkspaceID:   "workspace-1", CandidateID: "candidate-1", CandidateRevision: 1,
		GoalInput:                         workspacemodel.DefinitionCandidateItem{CandidateItemRef: "goal", Text: "ship the outcome", SourceIdentity: "user"},
		DoDInputs:                         []workspacemodel.DefinitionCandidateItem{{CandidateItemRef: "dod-1", Text: "evidence exists", SourceIdentity: "user"}},
		WorkspaceTemplateSnapshot:         workspacemodel.DefinitionTemplateSnapshot{TemplateRef: "generic", Revision: "template-r1", SnapshotHash: "template-hash"},
		RuntimeProfileContractsHash:       runtimecontract.StableRuntimeProfileContractsHash(nil),
		AdmissionCompilerVersion:          workspacemodel.DefinitionAdmissionCompilerVersionCurrent,
		ResolvedProjectWorkspaceDirectory: "/tmp/workspace-1",
		ProposedRelationInputs:            []workspacemodel.DefinitionProposedRelationInput{{RelationRef: "relation-1", RelationKind: kind, SourceItemRefs: []string{"dod-1"}, TargetItemRefs: []string{"goal"}}},
	}).WithHash()
}

func TestCompileRejectsOneOfWithoutCallingExternalOwner(t *testing.T) {
	candidate := candidateWithRelation("one_of")
	result, err := Compile(candidate)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if result.Disposition != workspacemodel.DefinitionAdmissionClarificationRequired || result.ViolationCode != workspacemodel.DefinitionAdmissionOneOfUnsupported {
		t.Fatalf("result = %+v", result)
	}
	if err := result.ValidateFor(candidate); err != nil {
		t.Fatalf("validate result: %v", err)
	}
}

func TestCompileAdmitsAllOfDeterministically(t *testing.T) {
	candidate := candidateWithRelation("all_of")
	first, err := Compile(candidate)
	if err != nil {
		t.Fatalf("compile first: %v", err)
	}
	second, err := Compile(candidate)
	if err != nil {
		t.Fatalf("compile second: %v", err)
	}
	if first.Disposition != workspacemodel.DefinitionAdmissionAdmittedAllOf || first.AdmissionResultHash != second.AdmissionResultHash {
		t.Fatalf("non-deterministic admission: %+v / %+v", first, second)
	}
}

func TestCompileRejectsCandidateHashDriftInsteadOfResealingCallerInput(t *testing.T) {
	candidate := candidateWithRelation("all_of")
	candidate.ResolvedProjectWorkspaceDirectory = "/tmp/tampered"
	if _, err := Compile(candidate); err == nil {
		t.Fatal("compiler silently resealed a mutated immutable candidate")
	}
}

func TestCompileRequiresExactStaticOwnerFactConvergenceBeforeAdmission(t *testing.T) {
	requirement := runtimecontract.RuntimeOwnerFactRequirement{
		OwnerModuleID: runtimecontract.RuntimeModuleShell, FactKind: runtimecontract.RuntimeOwnerFactSelectableValidationSurface,
		OperationKind: runtimecontract.RuntimeBlockKindManagedValidationSession,
	}
	convergence := runtimecontract.RuntimeOwnerFactConvergence{
		Requirement: requirement, MissingTrigger: runtimecontract.RuntimeOwnerFactMissingTrigger,
		AcquisitionOwnerModuleID: runtimecontract.RuntimeModuleFile, AcquisitionOperationKind: runtimecontract.StepKindFileWrite,
		AcquisitionTargetPolicy: runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget, AcquisitionTargetSelectorRef: "workspace-relative-new-file-target",
		ProspectiveSatisfactionContractRef: "shell/selectable-validation-surface/prospective-workspace-manifest/v1",
		TargetSelectorRef:                  "file/workspace_deliverable", TargetSelectorOwnerRef: "software-dev", TargetSelectorSubjectRef: "validation-preparation",
		ReobserveOwnerModuleID: runtimecontract.RuntimeModuleShell, ReobserveFactKind: runtimecontract.RuntimeOwnerFactSelectableValidationSurface,
		MaxAcquisitionAttempts: 1,
	}
	profile := runtimecontract.RuntimeProfileContract{ID: "software-dev", EvidenceRequirements: []runtimecontract.RuntimeProfileEvidenceRequirement{{
		ID: "validation", Scope: "dodItem", AcceptedStepKinds: []string{runtimecontract.StepKindFileWrite, runtimecontract.RuntimeBlockKindManagedValidationSession},
		TargetSelectorRef: "file/workspace_deliverable", TargetSelectorOwnerRef: "software-dev", TargetSelectorSubjectRef: "validation-preparation",
		RequiredOwnerFacts: []runtimecontract.RuntimeOwnerFactRequirement{requirement}, OwnerFactConvergences: []runtimecontract.RuntimeOwnerFactConvergence{convergence},
	}}}
	candidate := candidateWithRelation("all_of")
	candidate.RuntimeProfileContracts = []runtimecontract.RuntimeProfileContract{profile}
	candidate.RuntimeProfileContractsHash = runtimecontract.StableRuntimeProfileContractsHash(candidate.RuntimeProfileContracts)
	candidate.RequiredOwnerFactBindings, _ = CompileProfileOwnerFactBindings(candidate.RuntimeProfileContracts)
	candidate = candidate.WithHash()

	missing, err := Compile(candidate)
	if err != nil {
		t.Fatalf("compile missing convergence: %v", err)
	}
	if missing.Disposition != workspacemodel.DefinitionAdmissionClarificationRequired || missing.ViolationCode != workspacemodel.DefinitionAdmissionProfilePrerequisiteUnresolved || len(missing.OffendingRefs) != 1 {
		t.Fatalf("missing convergence result = %+v", missing)
	}

	_, candidate.OwnerFactConvergenceBindings = CompileProfileOwnerFactBindings(candidate.RuntimeProfileContracts)
	candidate = candidate.WithHash()
	admitted, err := Compile(candidate)
	if err != nil || admitted.Disposition != workspacemodel.DefinitionAdmissionAdmittedAllOf {
		t.Fatalf("exact convergence admission = %+v err=%v", admitted, err)
	}

	selectorDrift := candidate.Clone()
	selectorDrift.RuntimeProfileContracts[0].EvidenceRequirements[0].OwnerFactConvergences[0].TargetSelectorSubjectRef = "different-definition-subject"
	selectorDrift.RuntimeProfileContractsHash = runtimecontract.StableRuntimeProfileContractsHash(selectorDrift.RuntimeProfileContracts)
	selectorDrift.RequiredOwnerFactBindings, selectorDrift.OwnerFactConvergenceBindings = CompileProfileOwnerFactBindings(selectorDrift.RuntimeProfileContracts)
	selectorDrift = selectorDrift.WithHash()
	selectorDriftResult, err := Compile(selectorDrift)
	if err != nil || selectorDriftResult.Disposition != workspacemodel.DefinitionAdmissionClarificationRequired || selectorDriftResult.ViolationCode != workspacemodel.DefinitionAdmissionProfileContractInvalid {
		t.Fatalf("pre-seal selector drift result = %+v err=%v", selectorDriftResult, err)
	}

	drifted := candidate.Clone()
	drifted.OwnerFactConvergenceBindings[0].Convergence.TargetSelectorSubjectRef = "caller-overridden-subject"
	drifted = drifted.WithHash()
	driftedResult, err := Compile(drifted)
	if err != nil || driftedResult.Disposition != workspacemodel.DefinitionAdmissionClarificationRequired || driftedResult.ViolationCode != workspacemodel.DefinitionAdmissionProfilePrerequisiteUnresolved {
		t.Fatalf("snapshot-derived manifest drift result = %+v err=%v", driftedResult, err)
	}

	incomplete := candidate.Clone()
	incomplete.OwnerFactConvergenceBindings[0].Convergence.AcquisitionTargetSelectorRef = ""
	incomplete = incomplete.WithHash()
	blocked, err := Compile(incomplete)
	if err != nil || blocked.Disposition != workspacemodel.DefinitionAdmissionClarificationRequired || blocked.ViolationCode != workspacemodel.DefinitionAdmissionProfilePrerequisiteUnresolved {
		t.Fatalf("incomplete acquisition operation selector result = %+v err=%v", blocked, err)
	}

	candidate.OwnerFactConvergenceBindings = append(candidate.OwnerFactConvergenceBindings, candidate.OwnerFactConvergenceBindings[0])
	candidate = candidate.WithHash()
	duplicate, err := Compile(candidate)
	if err != nil {
		t.Fatalf("compile duplicate convergence: %v", err)
	}
	if duplicate.Disposition != workspacemodel.DefinitionAdmissionClarificationRequired || duplicate.ViolationCode != workspacemodel.DefinitionAdmissionProfilePrerequisiteUnresolved {
		t.Fatalf("duplicate convergence result = %+v", duplicate)
	}
}

func TestCompileBlocksUnprovableRequiredProfileCWI(t *testing.T) {
	candidate := candidateWithRelation("all_of")
	candidate.RuntimeProfileContracts = []runtimecontract.RuntimeProfileContract{{
		ID:                      "software-dev",
		DeliverableExpectations: []runtimecontract.RuntimeDeliverableExpectation{{ID: "coverage_map", Required: true, EvidenceRole: "supplemental_evidence"}},
	}}
	candidate.RuntimeProfileContractsHash = runtimecontract.StableRuntimeProfileContractsHash(candidate.RuntimeProfileContracts)
	candidate = candidate.WithHash()

	result, err := Compile(candidate)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if result.Disposition != workspacemodel.DefinitionAdmissionClarificationRequired ||
		result.ViolationCode != workspacemodel.DefinitionAdmissionProfileContractInvalid ||
		len(result.OffendingRefs) != 1 || result.OffendingRefs[0] != "profile/software-dev/deliverable/coverage_map" {
		t.Fatalf("unprovable Profile admission = %+v", result)
	}
}
