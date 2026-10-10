package workspace

import (
	"reflect"
	"slices"
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func testGoalSpecificationCandidate() DefinitionAdmissionCandidate {
	candidate := DefinitionAdmissionCandidate{
		SchemaVersion: DefinitionAdmissionCandidateSchemaVersionCurrent, WorkspaceID: "workspace-1", CandidateID: "spec-1", CandidateRevision: 1,
		GoalInput:                   DefinitionCandidateItem{CandidateItemRef: "goal", Text: "Ship the outcome", SourceIdentity: "goal-r1"},
		DoDInputs:                   []DefinitionCandidateItem{{CandidateItemRef: "dod", Text: "The outcome is verified", SourceIdentity: "dod-r1"}},
		WorkspaceTemplateSnapshot:   DefinitionTemplateSnapshot{TemplateRef: "generic", Revision: "r1", SnapshotHash: "template-hash"},
		RuntimeProfileContractsHash: runtimecontract.StableRuntimeProfileContractsHash(nil),
		AdmissionCompilerVersion:    DefinitionAdmissionCompilerVersionCurrent, ResolvedProjectWorkspaceDirectory: "/tmp/workspace-1",
	}
	candidate.Specification = &GoalSpecification{
		Version: GoalSpecificationVersionCurrent, GoalItemRef: "goal", GoalTextHash: StableDefinitionTextHash(candidate.GoalInput.Text),
		Resources:     []GoalSpecificationResource{{ID: "repository", Ref: "workspace/repository", SnapshotHash: "snapshot-r1", Required: true, Available: true}},
		Dimensions:    []GoalSpecificationDimension{{ID: "outcome", Required: true, Activated: true, ResourceIDs: []string{"repository"}}},
		Requirements:  []GoalSpecificationRequirement{{ID: "req", DimensionID: "outcome", Required: true, SourceKind: GoalRequirementUserExplicit, SourceItemRefs: []string{"goal"}, ResourceIDs: []string{"repository"}, Text: "Deliver outcome"}},
		Obligations:   []GoalSpecificationObligation{{ID: "obligation", RequirementID: "req", DoDItemRefs: []string{"dod"}, Text: "Validate outcome"}},
		Verifications: []GoalSpecificationVerification{{ID: "verify", ObligationID: "obligation", Declaration: "Candidate proof proposal", Status: GoalVerificationUnverified}},
	}
	hash := candidate.Specification.ContentHash()
	candidate.Specification.Reviews = []GoalSpecificationCompletenessReview{
		{Kind: GoalReviewDimensionCoverage, Status: GoalReviewPass, SpecificationHash: hash, Reason: "reviewed outcome", EvidenceRefs: []string{"goal"}, RequirementIDs: currentRequiredRequirementIDsFixture(*candidate.Specification)},
		{Kind: GoalReviewNegativePath, Status: GoalReviewPass, SpecificationHash: hash, Reason: "reviewed negative paths", EvidenceRefs: []string{"goal"}, RequirementIDs: currentRequiredRequirementIDsFixture(*candidate.Specification)},
		{Kind: GoalReviewRegression, Status: GoalReviewPass, SpecificationHash: hash, Reason: "reviewed regression", EvidenceRefs: []string{"goal"}, RequirementIDs: currentRequiredRequirementIDsFixture(*candidate.Specification)},
		{Kind: GoalReviewResourceEnvironment, Status: GoalReviewPass, SpecificationHash: hash, Reason: "reviewed resources", EvidenceRefs: []string{"goal"}, RequirementIDs: currentRequiredRequirementIDsFixture(*candidate.Specification)},
		{Kind: GoalReviewTraceability, Status: GoalReviewPass, SpecificationHash: hash, Reason: "reviewed traceability", EvidenceRefs: []string{"goal"}, RequirementIDs: currentRequiredRequirementIDsFixture(*candidate.Specification)},
		{Kind: GoalReviewVerifierFeasibility, Status: GoalReviewPass, SpecificationHash: hash, Reason: "reviewed declaration", EvidenceRefs: []string{"goal"}, RequirementIDs: currentRequiredRequirementIDsFixture(*candidate.Specification)},
	}
	return candidate.WithHash()
}

func TestGoalSpecificationSealsReviewsAndPreservesNilLegacyHash(t *testing.T) {
	candidate := testGoalSpecificationCandidate()
	if err := candidate.Validate(); err != nil {
		t.Fatal(err)
	}
	legacy := candidate.Clone()
	legacy.Specification = nil
	legacy = legacy.WithHash()
	if legacy.CandidateHash == candidate.CandidateHash {
		t.Fatal("specification did not affect candidate hash")
	}
	if legacy.Specification != nil || legacy.SchemaVersion != DefinitionAdmissionCandidateSchemaVersionCurrent {
		t.Fatal("optional specification modified legacy schema")
	}
	changed := candidate.Clone()
	changed.Specification.Reviews[0].Reason = "different review"
	if changed.Specification.ContentHash() != candidate.Specification.ContentHash() {
		t.Fatal("review unexpectedly changed specification content hash")
	}
	if changed.Specification.AdmissionHash() == candidate.Specification.AdmissionHash() {
		t.Fatal("review omitted from candidate seal")
	}
	if err := changed.Validate(); err == nil {
		t.Fatal("review drift retained immutable candidate hash")
	}
	if err := legacy.Validate(); err != nil {
		t.Fatalf("legacy candidate: %v", err)
	}
}

func reviewCoverageCandidateFixture() DefinitionAdmissionCandidate {
	candidate := testGoalSpecificationCandidate()
	candidate.Specification.Requirements[0].SourceItemRefs = []string{"dod"}
	candidate.Specification.Requirements = append(candidate.Specification.Requirements,
		GoalSpecificationRequirement{ID: "independent-goal", DimensionID: "outcome", Required: true, SourceKind: GoalRequirementUserExplicit, SourceItemRefs: []string{"goal"}, Text: candidate.GoalInput.Text},
		GoalSpecificationRequirement{ID: "optional", DimensionID: "outcome", SourceKind: GoalRequirementDerived, SourceItemRefs: []string{"dod"}, Text: "可选补充"},
	)
	for i := range candidate.Specification.Reviews {
		candidate.Specification.Reviews[i].SpecificationHash = candidate.Specification.ContentHash()
		candidate.Specification.Reviews[i].RequirementIDs = currentRequiredRequirementIDsFixture(*candidate.Specification)
	}
	return candidate.WithHash()
}

func TestGoalSpecificationPASSReviewsRequireEveryRequiredRequirement(t *testing.T) {
	candidate := reviewCoverageCandidateFixture()
	base := candidate.Clone()
	for _, kind := range goalReviewKinds {
		t.Run(kind, func(t *testing.T) {
			missing := base.Clone()
			for i := range missing.Specification.Reviews {
				if missing.Specification.Reviews[i].Kind == kind {
					missing.Specification.Reviews[i].RequirementIDs = []string{"req"}
				}
			}
			missing = missing.WithHash()
			if err := missing.Validate(); err == nil {
				t.Fatal("PASS review missing a required requirement was accepted")
			}
		})
	}

	t.Run("optional cannot substitute", func(t *testing.T) {
		missing := base.Clone()
		for i := range missing.Specification.Reviews {
			missing.Specification.Reviews[i].RequirementIDs = []string{"optional"}
		}
		missing = missing.WithHash()
		if err := missing.Validate(); err == nil {
			t.Fatal("optional requirement substituted for required requirement")
		}
	})

	t.Run("all required including independent goal requirement pass", func(t *testing.T) {
		if err := base.Validate(); err != nil {
			t.Fatalf("complete required review coverage rejected: %v", err)
		}
		for _, review := range base.Specification.Reviews {
			if !reflect.DeepEqual(review.RequirementIDs, currentRequiredRequirementIDsFixture(*base.Specification)) {
				t.Fatalf("PASS review did not cover every required requirement: %+v", review.RequirementIDs)
			}
		}
	})
}

func TestGoalSpecificationReviewCloneAndOrderingAreDeterministic(t *testing.T) {
	candidate := reviewCoverageCandidateFixture()
	original := candidate.Specification.Clone()
	clone := original.Clone()
	clone.Reviews[0].RequirementIDs[0] = "changed"
	if reflect.DeepEqual(original.Reviews[0].RequirementIDs, clone.Reviews[0].RequirementIDs) {
		t.Fatal("cloned review shares requirement ID storage")
	}
	left := original.Clone()
	right := original.Clone()
	for i := range right.Reviews {
		slices.Reverse(right.Reviews[i].RequirementIDs)
	}
	if left.AdmissionHash() != right.AdmissionHash() {
		t.Fatal("requirement ordering changed deterministic admission hash")
	}
	if !reflect.DeepEqual(left.Normalized(), right.Normalized()) {
		t.Fatal("normalized review ordering is not deterministic")
	}
}

func TestGoalSpecificationCloneDetachesNestedValues(t *testing.T) {
	original := testGoalSpecificationCandidate()
	cloned := original.Clone()
	cloned.Specification.Dimensions[0].ResourceIDs[0] = "different"
	cloned.Specification.Requirements[0].SourceItemRefs[0] = "different"
	cloned.Specification.Obligations[0].DoDItemRefs[0] = "different"
	cloned.Specification.Reviews[0].EvidenceRefs[0] = "different"
	cloned.Specification.Verifications[0].Declaration = "different"
	if err := original.Validate(); err != nil {
		t.Fatalf("original changed through cloned pointer: %v", err)
	}
	if cloned.Specification.Dimensions[0].ResourceIDs[0] == original.Specification.Dimensions[0].ResourceIDs[0] || cloned.Specification.Reviews[0].EvidenceRefs[0] == original.Specification.Reviews[0].EvidenceRefs[0] {
		t.Fatal("nested specification clone shares storage")
	}
}

func TestGoalSpecificationValidateRejectsMalformedAndStaleReviews(t *testing.T) {
	original := testGoalSpecificationCandidate()
	tests := []struct {
		name   string
		mutate func(*GoalSpecification)
	}{
		{"goal text", func(s *GoalSpecification) { s.GoalTextHash = "wrong" }},
		{"version", func(s *GoalSpecification) { s.Version = 2 }},
		{"duplicate resource", func(s *GoalSpecification) { s.Resources = append(s.Resources, s.Resources[0]) }},
		{"unknown resource", func(s *GoalSpecification) { s.Requirements[0].ResourceIDs = []string{"absent"} }},
		{"duplicate source", func(s *GoalSpecification) { s.Requirements[0].SourceItemRefs = []string{"goal", "goal"} }},
		{"unknown dimension", func(s *GoalSpecification) { s.Requirements[0].DimensionID = "absent" }},
		{"source kind", func(s *GoalSpecification) { s.Requirements[0].SourceKind = "GUESS" }},
		{"unknown dod", func(s *GoalSpecification) { s.Obligations[0].DoDItemRefs = []string{"absent"} }},
		{"duplicate requirement", func(s *GoalSpecification) { s.Requirements = append(s.Requirements, s.Requirements[0]) }},
		{"resource shadows candidate", func(s *GoalSpecification) { s.Resources[0].ID = "goal" }},
		{"dimension shadows resource", func(s *GoalSpecification) { s.Dimensions[0].ID = "repository" }},
		{"obligation shadows requirement", func(s *GoalSpecification) { s.Obligations[0].ID = "req" }},
		{"verification shadows dod", func(s *GoalSpecification) { s.Verifications[0].ID = "dod" }},
		{"incomplete declared verification", func(s *GoalSpecification) { s.Verifications[0].Status = GoalVerificationDeclared }},
		{"unverified binding", func(s *GoalSpecification) { s.Verifications[0].ProfileID = "fake" }},
		{"stale review", func(s *GoalSpecification) { s.Reviews[0].SpecificationHash = "old-hash" }},
		{"duplicate review", func(s *GoalSpecification) { s.Reviews[0].Kind = s.Reviews[1].Kind }},
		{"unknown review", func(s *GoalSpecification) { s.Reviews[0].Kind = "other" }},
		{"pass without evidence", func(s *GoalSpecification) { s.Reviews[0].EvidenceRefs = nil }},
		{"unknown reviewed requirement", func(s *GoalSpecification) { s.Reviews[0].RequirementIDs = []string{"missing"} }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := original.Clone()
			tt.mutate(c.Specification)
			c = c.WithHash()
			if err := c.Validate(); err == nil {
				t.Fatal("invalid specification accepted")
			}
		})
	}
}
