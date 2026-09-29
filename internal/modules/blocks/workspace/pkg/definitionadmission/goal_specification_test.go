package definitionadmission

import (
	"testing"

	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func candidateWithSpecification() workspacemodel.DefinitionAdmissionCandidate {
	candidate := candidateWithRelation("all_of")
	candidate.RuntimeProfileContracts = []runtimecontract.RuntimeProfileContract{{ID: "generic", EvidenceRequirements: []runtimecontract.RuntimeProfileEvidenceRequirement{{ID: "proof", Scope: "dodItem", RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation}, AcceptedStepKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession}, Description: "declaration"}}}}
	candidate.RuntimeProfileContractsHash = runtimecontract.StableRuntimeProfileContractsHash(candidate.RuntimeProfileContracts)
	spec := workspacemodel.GoalSpecification{
		Version: workspacemodel.GoalSpecificationVersionCurrent, GoalItemRef: "goal", GoalTextHash: workspacemodel.StableDefinitionTextHash(candidate.GoalInput.Text),
		Resources:     []workspacemodel.GoalSpecificationResource{{ID: "input", Ref: "workspace/input", SnapshotHash: "snapshot-r1", Required: true, Available: true}},
		Dimensions:    []workspacemodel.GoalSpecificationDimension{{ID: "value", Required: true, Activated: true, ResourceIDs: []string{"input"}}},
		Requirements:  []workspacemodel.GoalSpecificationRequirement{{ID: "requirement", DimensionID: "value", Required: true, SourceKind: workspacemodel.GoalRequirementUserExplicit, SourceItemRefs: []string{"goal", "dod-1"}, ResourceIDs: []string{"input"}, Text: "Deliver outcome"}},
		Obligations:   []workspacemodel.GoalSpecificationObligation{{ID: "obligation", RequirementID: "requirement", DoDItemRefs: []string{"dod-1"}, Text: "Verify outcome"}},
		Verifications: []workspacemodel.GoalSpecificationVerification{{ID: "verification", ObligationID: "obligation", Declaration: "Exact proof declaration", Status: workspacemodel.GoalVerificationDeclared, ProfileID: "generic", EvidenceRequirementID: "proof", Scope: "dodItem"}},
	}
	hash := spec.ContentHash()
	for _, kind := range []string{workspacemodel.GoalReviewDimensionCoverage, workspacemodel.GoalReviewNegativePath, workspacemodel.GoalReviewRegression, workspacemodel.GoalReviewResourceEnvironment, workspacemodel.GoalReviewTraceability, workspacemodel.GoalReviewVerifierFeasibility} {
		spec.Reviews = append(spec.Reviews, workspacemodel.GoalSpecificationCompletenessReview{Kind: kind, Status: workspacemodel.GoalReviewPass, SpecificationHash: hash, Reason: "externally reviewed", EvidenceRefs: []string{"goal"}, RequirementIDs: []string{"requirement"}})
	}
	candidate.Specification = &spec
	return candidate.WithHash()
}

func TestCompileRejectsPlaceholderProfileVerification(t *testing.T) {
	candidate := candidateWithSpecification()
	candidate.RuntimeProfileContracts[0].EvidenceRequirements[0].RequiredEvidenceKinds = nil
	candidate.RuntimeProfileContracts[0].EvidenceRequirements[0].AcceptedStepKinds = nil
	candidate.RuntimeProfileContractsHash = runtimecontract.StableRuntimeProfileContractsHash(candidate.RuntimeProfileContracts)
	candidate = candidate.WithHash()
	result, err := Compile(candidate)
	if err != nil || result.Disposition != workspacemodel.DefinitionAdmissionClarificationRequired || result.ViolationCode != workspacemodel.DefinitionAdmissionSpecificationIncomplete {
		t.Fatalf("placeholder proof declaration admitted: result=%+v err=%v", result, err)
	}
}

func TestCompileChecksVersionedGoalSpecificationBeforeAdmission(t *testing.T) {
	original := candidateWithSpecification()
	result, err := Compile(original)
	if err != nil || result.Disposition != workspacemodel.DefinitionAdmissionAdmittedAllOf {
		t.Fatalf("valid specification result=%+v err=%v", result, err)
	}
	cases := []struct {
		name, ref string
		mutate    func(*workspacemodel.GoalSpecification)
	}{
		{"missing dimension requirement", "dimension/value/requirement", func(s *workspacemodel.GoalSpecification) { s.Requirements[0].Required = false }},
		{"missing obligation", "requirement/additional/obligation", func(s *workspacemodel.GoalSpecification) {
			extra := s.Requirements[0]
			extra.ID = "additional"
			s.Requirements = append(s.Requirements, extra)
		}},
		{"missing dod mapping", "dod/dod-1", func(s *workspacemodel.GoalSpecification) { s.Obligations[0].DoDItemRefs = nil }},
		{"required obligation without dod", "obligation/extra-obligation/dod_mapping", func(s *workspacemodel.GoalSpecification) {
			s.Obligations = append(s.Obligations, workspacemodel.GoalSpecificationObligation{ID: "extra-obligation", RequirementID: "requirement", Text: "new required obligation"})
		}},
		{"unavailable resource", "resource/input", func(s *workspacemodel.GoalSpecification) { s.Resources[0].Available = false }},
		{"inactive dimension", "dimension/value", func(s *workspacemodel.GoalSpecification) { s.Dimensions[0].Activated = false }},
		{"unverified verification", "verification/verification", func(s *workspacemodel.GoalSpecification) {
			v := &s.Verifications[0]
			v.Status = workspacemodel.GoalVerificationUnverified
			v.ProfileID = ""
			v.EvidenceRequirementID = ""
			v.Scope = ""
		}},
		{"nonmatching profile", "verification/verification", func(s *workspacemodel.GoalSpecification) { s.Verifications[0].EvidenceRequirementID = "missing" }},
		{"negative-path unknown", "review/negative_path", func(s *workspacemodel.GoalSpecification) {
			for i := range s.Reviews {
				if s.Reviews[i].Kind == workspacemodel.GoalReviewNegativePath {
					s.Reviews[i].Status = workspacemodel.GoalReviewUnknown
				}
			}
		}},
		{"regression fail", "review/regression", func(s *workspacemodel.GoalSpecification) {
			for i := range s.Reviews {
				if s.Reviews[i].Kind == workspacemodel.GoalReviewRegression {
					s.Reviews[i].Status = workspacemodel.GoalReviewFail
				}
			}
		}},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			candidate := original.Clone()
			tt.mutate(candidate.Specification)
			hash := candidate.Specification.ContentHash()
			for i := range candidate.Specification.Reviews {
				candidate.Specification.Reviews[i].SpecificationHash = hash
			}
			candidate = candidate.WithHash()
			result, err := Compile(candidate)
			if err != nil {
				t.Fatalf("compile: %v", err)
			}
			if result.Disposition != workspacemodel.DefinitionAdmissionClarificationRequired || result.ViolationCode != workspacemodel.DefinitionAdmissionSpecificationIncomplete {
				t.Fatalf("unsafe admission: %+v", result)
			}
			found := false
			for _, ref := range result.OffendingRefs {
				if ref == tt.ref {
					found = true
				}
			}
			if !found {
				t.Fatalf("missing %q in %+v", tt.ref, result.OffendingRefs)
			}
			if err := result.ValidateFor(candidate); err != nil {
				t.Fatal(err)
			}
		})
	}
}
