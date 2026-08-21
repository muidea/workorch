package definitionadmissioncore

import (
	"testing"

	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
)

func candidateWithRelation(kind string) workspacemodel.DefinitionAdmissionCandidate {
	return (workspacemodel.DefinitionAdmissionCandidate{
		SchemaVersion: workspacemodel.DefinitionAdmissionCandidateSchemaVersionCurrent,
		WorkspaceID:   "workspace-1", CandidateID: "candidate-1", CandidateRevision: 1,
		GoalInput:                         workspacemodel.DefinitionCandidateItem{CandidateItemRef: "goal", Text: "ship the outcome", SourceIdentity: "user"},
		DoDInputs:                         []workspacemodel.DefinitionCandidateItem{{CandidateItemRef: "dod-1", Text: "evidence exists", SourceIdentity: "user"}},
		WorkspaceTemplateSnapshot:         workspacemodel.DefinitionTemplateSnapshot{TemplateRef: "generic", Revision: "template-r1", SnapshotHash: "template-hash"},
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
