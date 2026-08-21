package workorchruntimecontract

import (
	"reflect"
	"testing"
)

func formalCloseoutFixture(t *testing.T, terminal TerminalOutcomeKind, assessment RuntimeRequirementAssessmentState) RuntimeResultCloseout {
	t.Helper()
	revisions := (RuntimeOwnerRevisionSet{DefinitionRevision: "definition-1", Entries: []RuntimeOwnerRevisionRef{{
		OwnerKind: "capability", OwnerInstanceID: "blocks/file", ResourceKind: "workspace_tree", ResourceID: "workspace-1", Revision: "tree-2", DependencyHash: "dependency-hash-1",
	}}}).WithHash()
	outcome := RuntimeRequirementOutcome{
		RequirementRef: "requirement-1", SourceItemRef: "dod-item-1", CoverageWorkItemID: "cwi-1", DefinitionRevision: "definition-1",
		CoverageState: RuntimeRequirementCovered, AssessmentState: assessment, OwnerRevisionSetHash: revisions.SetHash,
		EvidenceItems:        []RuntimeRequirementEvidenceItem{{EvidenceRef: "evidence-1", BindingRef: "binding-1", State: RuntimeRequirementEvidenceAccepted, OwnerRevisionSetHash: revisions.SetHash}},
		AcceptedEvidenceRefs: []string{"evidence-1"},
	}
	if assessment == RuntimeRequirementIndeterminate {
		outcome.AssessmentReasonCode = "required_observation_unavailable"
		outcome.MissingEvidenceKinds = []string{"validation"}
	}
	closeout := RuntimeResultCloseout{
		SchemaVersion: RuntimeResultCloseoutSchemaVersionCurrent, CloseoutID: "closeout-1", StartupInputHash: "startup-1", DefinitionRevision: "definition-1", RuntimeStateRevision: 4,
		TerminalOutcome: TerminalOutcome{Kind: terminal, Code: string(terminal), Summary: "sealed"}, RequirementOutcomes: []RuntimeRequirementOutcome{outcome},
		AcceptedEvidenceRefs: []string{"evidence-1"}, OwnerRevisionSet: revisions,
		RecoveryPolicy: DefaultRuntimeRecoveryPolicy(),
	}
	return closeout.WithCloseoutHash()
}

func TestRuntimeResultCloseoutFormalHashAndEvidenceUnion(t *testing.T) {
	closeout := formalCloseoutFixture(t, TerminalOutcomeKindCompleted, RuntimeRequirementSatisfied)
	if err := closeout.Validate(); err != nil {
		t.Fatalf("formal closeout: %v", err)
	}
	mutated := closeout
	mutated.AcceptedEvidenceRefs = []string{"evidence-other"}
	mutated = mutated.WithCloseoutHash()
	if err := mutated.Validate(); err == nil {
		t.Fatal("accepted evidence union drift was accepted")
	}
	mutated = closeout
	mutated.RequirementOutcomes[0].AssessmentReasonCode = "mutated"
	if err := mutated.Validate(); err == nil {
		t.Fatal("formal closeout hash drift was accepted")
	}
}

func TestRuntimeResultCloseoutIndeterminateCannotCompleteSuccessfully(t *testing.T) {
	closeout := formalCloseoutFixture(t, TerminalOutcomeKindFailed, RuntimeRequirementIndeterminate)
	if err := closeout.Validate(); err != nil {
		t.Fatalf("failed indeterminate closeout: %v", err)
	}
	closeout.TerminalOutcome = TerminalOutcome{Kind: TerminalOutcomeKindCompleted, Code: "completed", Summary: "sealed"}
	closeout = closeout.WithCloseoutHash()
	if err := closeout.Validate(); err == nil {
		t.Fatal("indeterminate requirement was accepted as completed")
	}
}

func TestRuntimeResultCloseoutNormalizationIsDetachedAndEvidenceAuthorityIsExact(t *testing.T) {
	closeout := formalCloseoutFixture(t, TerminalOutcomeKindCompleted, RuntimeRequirementSatisfied)
	original := closeout
	original.RequirementOutcomes = append([]RuntimeRequirementOutcome(nil), closeout.RequirementOutcomes...)
	original.RequirementOutcomes[0].EvidenceItems = append([]RuntimeRequirementEvidenceItem(nil), closeout.RequirementOutcomes[0].EvidenceItems...)
	original.OwnerRevisionSet.Entries = append([]RuntimeOwnerRevisionRef(nil), closeout.OwnerRevisionSet.Entries...)

	normalized := closeout.Normalized()
	normalized.RequirementOutcomes[0].EvidenceItems[0].BindingRef = "mutated-binding"
	normalized.OwnerRevisionSet.Entries[0].Revision = "mutated-revision"
	if !reflect.DeepEqual(closeout, original) {
		t.Fatal("runtime closeout normalization mutated its input backing slices")
	}

	for _, mutate := range []func(*RuntimeResultCloseout){
		func(value *RuntimeResultCloseout) { value.RequirementOutcomes[0].EvidenceItems[0].BindingRef = "" },
		func(value *RuntimeResultCloseout) {
			value.RequirementOutcomes[0].EvidenceItems[0].OwnerRevisionSetHash = "owner-set-other"
		},
		func(value *RuntimeResultCloseout) { value.OwnerRevisionSet.Entries = nil },
	} {
		invalid := closeout.Normalized()
		mutate(&invalid)
		invalid = invalid.WithCloseoutHash()
		if err := invalid.Validate(); err == nil {
			t.Fatal("invalid accepted evidence authority was admitted")
		}
	}
}
