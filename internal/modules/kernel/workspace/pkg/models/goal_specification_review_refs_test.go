package workspace

import (
	"reflect"
	"slices"
	"testing"
)

func TestReviewEvidenceRefsExactlyMatchOwnerProvenance(t *testing.T) {
	record, mapping, _ := fullSizeMappingFixture(t)
	d := record.DerivationRequests[0]
	spec, err := CompileDefinitionAuthoringMapping(d, mapping)
	if err != nil {
		t.Fatal(err)
	}
	spec.Resources = []GoalSpecificationResource{{ID: "resource", Ref: "file:input", SnapshotHash: "snapshot", Required: true, Available: true}}
	goal := DefinitionCandidateItem{CandidateItemRef: DefinitionAuthoringGoalItemRef, Text: d.Input.GoalText}
	items, err := d.Proposal.CandidateItems(d.Input, &d.Template)
	if err != nil {
		t.Fatal(err)
	}
	before := spec.Clone()
	refs, err := spec.ReviewEvidenceRefs(goal, items)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(spec, before) || !slices.IsSorted(refs) {
		t.Fatal("projection changed proposal or ref ordering")
	}
	want := 1 + len(items) + len(spec.Resources) + len(spec.Dimensions) + len(spec.Requirements) + len(spec.Obligations) + len(spec.Verifications)
	if len(refs) != want {
		t.Fatalf("missing provenance namespace: %d != %d", len(refs), want)
	}
	for i, ref := range refs {
		if i > 0 && refs[i-1] == ref {
			t.Fatal("ambiguous provenance ref")
		}
		candidate := spec.Clone()
		required, err := candidate.RequiredReviewRequirementIDs(GoalReviewTraceability)
		if err != nil {
			t.Fatal(err)
		}
		for _, kind := range goalReviewKinds {
			candidate.Reviews = append(candidate.Reviews, GoalSpecificationCompletenessReview{Kind: kind, Status: GoalReviewPass, SpecificationHash: spec.ContentHash(), Reason: "引用原来源或规格对象作独立审查依据；不是运行证据。", EvidenceRefs: []string{ref}, RequirementIDs: required})
		}
		if err := candidate.Validate(goal, items); err != nil {
			t.Fatalf("projected ref rejected by owner %s: %v", ref, err)
		}
	}
	for _, ref := range []string{spec.ContentHash(), d.Input.InputHash(), WorkspaceTemplateSnapshotHash(&d.Template), d.ResourceSnapshotHash, "runtime-profile-contracts:candidate-prefix-ecbf982cb2efe649c3b3"} {
		if slices.Contains(refs, ref) {
			t.Fatalf("hash or fabricated ref gained provenance: %s", ref)
		}
		candidate := spec.Clone()
		for _, kind := range goalReviewKinds {
			candidate.Reviews = append(candidate.Reviews, GoalSpecificationCompletenessReview{Kind: kind, Status: GoalReviewUnknown, SpecificationHash: spec.ContentHash(), EvidenceRefs: []string{ref}})
		}
		if err := candidate.Validate(goal, items); err == nil {
			t.Fatalf("owner accepted non-provenance ref %s", ref)
		}
	}
	withReview := spec.Clone()
	withReview.Reviews = []GoalSpecificationCompletenessReview{{Kind: GoalReviewTraceability}}
	if _, err := withReview.ReviewEvidenceRefs(goal, items); err == nil {
		t.Fatal("projection accepted reviewed input")
	}
	broken := spec.Clone()
	broken.Obligations[0].ID = broken.Requirements[0].ID
	if _, err := broken.ReviewEvidenceRefs(goal, items); err == nil {
		t.Fatal("projection accepted ambiguous object identity")
	}
}
