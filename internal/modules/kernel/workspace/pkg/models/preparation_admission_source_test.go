package workspace

import (
	"reflect"
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// Compare the local compilation product with a second, independent complete
// source validation. Repeated calls must observe changed raw ledgers, and the
// product must not alias the persisted candidate or another compilation.
func TestPreparationAdmissionSourceCompilationIsExactAndCallLocal(t *testing.T) {
	record, journal := confirmedAuthoringSourceFixture(t)
	goal, items, specification, ref, err := ConfirmedAuthoringAdmissionSource(&record, journal, record.Input.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	candidate := (DefinitionAdmissionCandidate{
		SchemaVersion: DefinitionAdmissionCandidateSchemaVersionCurrent, WorkspaceID: record.Input.WorkspaceID,
		CandidateID: "exact-preparation", CandidateRevision: 1, GoalInput: goal, DoDInputs: items,
		WorkspaceTemplateSnapshot:   DefinitionTemplateSnapshot{TemplateRef: "template", Revision: "r1", SnapshotHash: record.TemplateSnapshotHash},
		RuntimeProfileContractsHash: runtimecontract.StableRuntimeProfileContractsHash(nil),
		AdmissionCompilerVersion:    DefinitionAdmissionCompilerVersionCurrent, ResolvedProjectWorkspaceDirectory: "/tmp/exact-preparation",
		Specification: &specification, AuthoringRef: &ref,
	}).WithHash()
	startup := WorkspaceStartupInputContract{
		WorkspaceID: candidate.WorkspaceID, DefinitionRevision: goal.SourceIdentity + "|" + items[0].SourceIdentity,
		GoalDefinitionRef: "goal-item", GoalTextHash: StableDefinitionTextHash(goal.Text), DoDItemSetHash: StableDefinitionStringSetHash(journal.DoD.Items),
		WorkspaceTemplateRef: "template", TemplateRevision: "r1", TemplateSnapshotHash: record.TemplateSnapshotHash,
		RuntimeProfileContractsHash: candidate.RuntimeProfileContractsHash, SpecificationHash: specification.AdmissionHash(),
		ResolvedProjectWorkspaceDirectory: candidate.ResolvedProjectWorkspaceDirectory,
		CandidateItemToItemRefMappings:    []CandidateItemToItemRefMapping{{CandidateItemRef: goal.CandidateItemRef, ItemRef: "goal-item"}},
	}
	for _, item := range items {
		formal := "formal-" + item.CandidateItemRef
		startup.DoDDefinitionRefs = append(startup.DoDDefinitionRefs, formal)
		startup.CandidateItemToItemRefMappings = append(startup.CandidateItemToItemRefMappings, CandidateItemToItemRefMapping{CandidateItemRef: item.CandidateItemRef, ItemRef: formal})
	}
	startup = startup.WithHash()
	admission := WorkspaceActivationAdmission{WorkspaceID: candidate.WorkspaceID, CandidateID: candidate.CandidateID, CandidateRevision: 1, CandidateHash: candidate.CandidateHash, DefinitionRevision: startup.DefinitionRevision, StartupInputHash: startup.StartupInputHash, State: WorkspaceActivationEligible}
	w := &Workspace{ID: candidate.WorkspaceID, DefinitionAuthoring: &record, DefinitionAuthoringConfirmation: &journal, DefinitionAdmissionCandidates: []DefinitionAdmissionCandidate{candidate}, StartupInput: &startup, ActivationAdmission: &admission}
	_, present, compiled, err := w.compileActivationAdmissionPersistence(false)
	if err != nil || !present || compiled == nil || compiled.ref != ref || !reflect.DeepEqual(compiled.items, items) {
		t.Fatalf("exact source lost: present=%t err=%v", present, err)
	}
	compiled.items[0].Text += "detached mutation"
	_, _, second, err := w.compileActivationAdmissionPersistence(false)
	if err != nil || second == nil || !reflect.DeepEqual(second.items, items) {
		t.Fatal("local product aliases source/next call", err)
	}
	record.SpecificationRequests[len(record.SpecificationRequests)-1].Review.Status = GoalReviewUnknown
	if _, _, _, err := w.compileActivationAdmissionPersistence(false); err == nil {
		t.Fatal("changed ledger reused prior success")
	}
	if _, _, err := w.ValidateActivationAdmissionPersistence(); err == nil {
		t.Fatal("public gate lost independent full validation")
	}
}
