package workorchruntimecontract

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func semanticInputReviewFixture(t *testing.T) (RuntimeSemanticPlanManifest, RuntimeSemanticPlanDocument, []RuntimeSemanticProofFeasibility, RuntimeValidationProofRequirementBinding) {
	t.Helper()
	manifest, document, _, _ := semanticIndependentProofSetFixture(t)
	var binding RuntimeValidationProofRequirementBinding
	for i := range manifest.OwnerCatalogs {
		catalog := &manifest.OwnerCatalogs[i]
		if catalog.OwnerModuleID != RuntimeModuleShell {
			continue
		}
		binding = catalog.ValidationSurfaces[0].ProofRequirementBindings[0].Clone()
		content := "package checks\n// Exact input bytes, not an assertion of compliance.\n"
		binding.InputReview = &RuntimeValidationInputReviewSubject{ProtocolRef: RuntimeValidationInputReviewProtocol,
			Files: []RuntimeValidationInputReviewFile{{TargetRef: "checks/input_test.go", Content: content, ContentHash: RuntimeValidationInputContentHash(content)}}}
		binding.InputReview.Hash = StableRuntimeValidationInputReviewSubjectHash(*binding.InputReview, binding)
		catalog.ValidationSurfaces[0].ProofRequirementBindings[0] = binding.Clone()
		catalog.SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(*catalog)
	}
	manifest.Hash = StableRuntimeSemanticPlanManifestHash(manifest)
	if err := manifest.Validate(); err != nil {
		t.Fatal(err)
	}
	document.ManifestHash = manifest.Hash
	review := &RuntimeValidationInputReview{SubjectHash: binding.InputReview.Hash}
	for _, check := range binding.RequiredCheckRefs {
		review.Checks = append(review.Checks, RuntimeValidationInputCheckReview{RequiredCheckRef: check, Conclusion: RuntimeValidationInputConsistent,
			Reason: "The input preserves the original requirement without a skip or waiver.", InputTargetRefs: []string{"checks/input_test.go"}})
	}
	document.Proposals[1].ValidationInputReview = review
	facts := []RuntimeSemanticProofFeasibility{
		semanticFeasibilityForTest(t, manifest, document.Proposals[0], []string{"main.go"}, "", nil),
		semanticFeasibilityForTest(t, manifest, document.Proposals[1], []string{"main.go"}, "recipe/validate", binding.RequiredCheckRefs),
	}
	return manifest, document, facts, binding
}

func TestValidationInputReviewSeparatesCompleteConclusionFromBindingAuthority(t *testing.T) {
	for _, conclusion := range []string{RuntimeValidationInputConsistent, RuntimeValidationInputConflict, RuntimeValidationInputInconclusive} {
		t.Run(conclusion, func(t *testing.T) {
			manifest, document, facts, _ := semanticInputReviewFixture(t)
			document.Proposals[1].ValidationInputReview.Checks[0].Conclusion = conclusion
			facts[1].ProposalHash = StableRuntimeSemanticProofProposalHash(document.Proposals[1])
			if err := document.ValidateAgainstManifest(manifest); err != nil {
				t.Fatalf("complete observation rejected as malformed: %v", err)
			}
			focuses, _, err := CompileRuntimeSemanticBindings(manifest, runtimeSemanticBindingTestRequestID, document, facts)
			if conclusion == RuntimeValidationInputConsistent {
				if err != nil || len(focuses) == 0 {
					t.Fatalf("consistent review blocked: %v", err)
				}
			} else if err == nil || len(focuses) != 0 {
				t.Fatal("non-consistent report became proof authority")
			}
		})
	}
}

func TestValidationInputReviewRejectsIncompleteUnboundAndWeakenedReports(t *testing.T) {
	cases := map[string]func(*RuntimeValidationInputReview){
		"missing":        func(r *RuntimeValidationInputReview) { r.Checks = nil },
		"partial all-of": func(r *RuntimeValidationInputReview) { r.Checks = r.Checks[:1] },
		"duplicate":      func(r *RuntimeValidationInputReview) { r.Checks[1] = r.Checks[0] },
		"unknown check":  func(r *RuntimeValidationInputReview) { r.Checks[0].RequiredCheckRef = "replacement criterion" },
		"approval":       func(r *RuntimeValidationInputReview) { r.Checks[0].Conclusion = "approved" },
		"empty reason":   func(r *RuntimeValidationInputReview) { r.Checks[0].Reason = " " },
		"unknown input":  func(r *RuntimeValidationInputReview) { r.Checks[0].InputTargetRefs = []string{"replacement.go"} },
		"repeated input": func(r *RuntimeValidationInputReview) {
			r.Checks[0].InputTargetRefs = []string{"checks/input_test.go", "checks/input_test.go"}
		},
		"stale content identity": func(r *RuntimeValidationInputReview) { r.SubjectHash += "stale" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			manifest, document, _, _ := semanticInputReviewFixture(t)
			mutate(document.Proposals[1].ValidationInputReview)
			if err := document.ValidateAgainstManifest(manifest); err == nil {
				t.Fatal("invalid report admitted")
			}
		})
	}
	manifest, document, _, _ := semanticInputReviewFixture(t)
	document.Proposals[1].ValidationInputReview = nil
	if err := document.ValidateAgainstManifest(manifest); err == nil {
		t.Fatal("required review omitted")
	}
}

func TestValidationInputReviewSubjectExactPreimageAndClone(t *testing.T) {
	_, document, _, binding := semanticInputReviewFixture(t)
	for name, mutate := range map[string]func(*RuntimeValidationProofRequirementBinding){
		"content":        func(b *RuntimeValidationProofRequirementBinding) { b.InputReview.Files[0].Content += "// drift" },
		"target":         func(b *RuntimeValidationProofRequirementBinding) { b.InputReview.Files[0].TargetRef = "other_test.go" },
		"native mapping": func(b *RuntimeValidationProofRequirementBinding) { b.CheckAttestations[0].NativeCheckRef += "Other" },
		"criterion":      func(b *RuntimeValidationProofRequirementBinding) { b.DefinitionRevision += "Other" },
		"capacity": func(b *RuntimeValidationProofRequirementBinding) {
			b.InputReview.Files[0].Content = strings.Repeat("a", RuntimeValidationInputReviewMaxBytes+1)
			b.InputReview.Files[0].ContentHash = RuntimeValidationInputContentHash(b.InputReview.Files[0].Content)
			b.InputReview.Hash = StableRuntimeValidationInputReviewSubjectHash(*b.InputReview, *b)
		},
	} {
		t.Run(name, func(t *testing.T) {
			clone := binding.Clone()
			mutate(&clone)
			if err := clone.InputReview.ValidateAgainstBinding(clone); err == nil {
				t.Fatal("drift admitted")
			}
			if err := binding.InputReview.ValidateAgainstBinding(binding); err != nil {
				t.Fatalf("clone aliased source: %v", err)
			}
		})
	}
	clone := document.Clone()
	for i := range clone.Proposals {
		if clone.Proposals[i].ValidationInputReview != nil {
			clone.Proposals[i].ValidationInputReview.Checks[0].InputTargetRefs[0] = "other"
		}
	}
	if reflect.DeepEqual(clone, document) || document.Proposals[1].ValidationInputReview.Checks[0].InputTargetRefs[0] == "other" {
		t.Fatal("report clone shares targets")
	}
}

func TestValidationInputReviewPayloadHasNoAuthorityEscape(t *testing.T) {
	_, document, _, _ := semanticInputReviewFixture(t)
	raw, err := json.Marshal(document.Proposals[1].ValidationInputReview)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateRuntimeValidationInputReviewPayload("$.validationInputReview", raw); err != nil {
		t.Fatal(err)
	}
	for _, payload := range []string{"null", `{}`, `{"subjectHash":"id","checks":null}`,
		strings.Replace(string(raw), `"subjectHash":`, `"approved":true,"subjectHash":`, 1),
		strings.Replace(string(raw), `"conclusion":`, `"replacementDoD":"pass","conclusion":`, 1)} {
		if err := validateRuntimeValidationInputReviewPayload("$.validationInputReview", []byte(payload)); err == nil {
			t.Fatalf("authority field or empty report accepted: %s", payload)
		}
	}
}

func TestValidationInputReviewInformationOnlyBindsExactCurrentSubject(t *testing.T) {
	manifest, _, _, binding := semanticInputReviewFixture(t)
	info := RuntimeValidationInputReviewInformation{SubjectHash: binding.InputReview.Hash, ReviewRef: "review", TicketID: "ticket", ResolutionID: "resolution", Summary: "bounded factual response"}
	manifest.InputReviewInformation = []RuntimeValidationInputReviewInformation{info}
	manifest.Hash = StableRuntimeSemanticPlanManifestHash(manifest)
	if err := manifest.Validate(); err != nil {
		t.Fatal(err)
	}
	clone := manifest.Clone()
	clone.InputReviewInformation[0].Summary += " changed"
	if StableRuntimeSemanticPlanManifestHash(clone) == manifest.Hash {
		t.Fatal("information omitted from manifest identity")
	}
	clone.InputReviewInformation[0].SubjectHash += "stale"
	clone.Hash = StableRuntimeSemanticPlanManifestHash(clone)
	if err := clone.Validate(); err == nil {
		t.Fatal("old content information rebound to new subject")
	}
}

func TestValidationInputReviewCatalogRejectsMixedReadVersions(t *testing.T) {
	manifest, _, _, _ := semanticInputReviewFixture(t)
	var catalog RuntimeOperationAdmissibility
	for _, owner := range manifest.OwnerCatalogs {
		if owner.OwnerModuleID == RuntimeModuleShell {
			catalog = owner.Clone()
		}
	}
	second := catalog.ValidationSurfaces[0].Clone()
	second.ID = "recipe/other"
	binding := &second.ProofRequirementBindings[0]
	binding.ValidationRecipeID = second.ID
	binding.InputReview.Files[0].Content += "// concurrent modification"
	binding.InputReview.Files[0].ContentHash = RuntimeValidationInputContentHash(binding.InputReview.Files[0].Content)
	binding.InputReview.Hash = StableRuntimeValidationInputReviewSubjectHash(*binding.InputReview, *binding)
	catalog.ValidationSurfaces = append(catalog.ValidationSurfaces, second)
	catalog.ValidationRecipeRefs = append(catalog.ValidationRecipeRefs, second.ID)
	catalog.SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	if err := catalog.Validate(); err == nil || !strings.Contains(err.Error(), "conflicting review input versions") {
		t.Fatalf("mixed owner read versions admitted: %v", err)
	}
}
