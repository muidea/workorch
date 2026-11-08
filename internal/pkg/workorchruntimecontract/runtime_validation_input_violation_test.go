package workorchruntimecontract

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestInputReviewConstraintSurvivesStrictDecodeCorrectionAndReload(t *testing.T) {
	manifest, document, _ := semanticInputReviewV2Fixture(t)
	valid, err := EncodeRuntimeSemanticPlanPayloadWithManifest(manifest, document)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, path, actual string
		mutate             func(*runtimeValidationInputReviewPayload)
	}{
		{"duplicate", ".checks[0].obligations[1].criterionText", "duplicate_criterion_text", func(r *runtimeValidationInputReviewPayload) {
			r.Checks[0].Obligations = append(r.Checks[0].Obligations, r.Checks[0].Obligations[0])
		}},
		{"false conclusion", ".checks[0].conclusion", "contradictory_conclusion", func(r *runtimeValidationInputReviewPayload) {
			r.Checks[0].Obligations[0].Finding = RuntimeValidationInputMissing
		}},
		{"foreign excerpt", ".checks[1].obligations[0].criterionText", "foreign_criterion_excerpt", func(r *runtimeValidationInputReviewPayload) {
			r.Checks[1].Obligations[0].CriterionText = "foreign criterion"
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var wire runtimeSemanticPlanPayload
			if err := json.Unmarshal(valid, &wire); err != nil {
				t.Fatal(err)
			}
			proposalIndex := -1
			for index, proposal := range wire.Proposals {
				if proposal.ValidationInputReview != nil {
					proposalIndex = index
					break
				}
			}
			if proposalIndex < 0 {
				t.Fatal("fixture has no input review")
			}
			tc.mutate(wire.Proposals[proposalIndex].ValidationInputReview)
			raw, err := json.Marshal(wire)
			if err != nil {
				t.Fatal(err)
			}
			before := append([]byte(nil), raw...)
			for range 2 {
				_, err := DecodeRuntimeSemanticPlanPayloadWithManifest(manifest, raw)
				var diagnostic *RuntimeCandidatePayloadDiagnostic
				if !errors.As(err, &diagnostic) {
					t.Fatalf("missing typed failure: %v", err)
				}
				if diagnostic.Path != fmt.Sprintf("$.proposals[%d].validationInputReview", proposalIndex)+tc.path || diagnostic.Actual != tc.actual || len(diagnostic.SemanticViolations) != 1 {
					t.Fatalf("imprecise diagnostic: %+v", diagnostic)
				}
				violation := diagnostic.SemanticViolations[0]
				if violation.ValidateAgainstManifest(manifest) != nil || len(violation.Required) == 0 {
					t.Fatal("unbound correction fact")
				}
				if tc.name == "duplicate" && !strings.Contains(strings.Join(violation.Required, " "), "obligations[0]") {
					t.Fatal("lost first occurrence witness")
				}
				rejection := RuntimeOriginalDiagnostic{SchemaVersion: RuntimeOriginalDiagnosticSchemaCurrent, OwnerModuleID: RuntimeModuleOrchestrator, OriginalClass: "semantic", OriginalCode: "semantic_plan_invalid", OriginalMessage: diagnostic.Error(), Signature: "semantic-plan/invalid-document", SemanticViolations: diagnostic.SemanticViolations}
				observation := semanticCorrectionObservationForTest(rejection)
				directive := RuntimeSemanticCorrectionDirective{SchemaVersion: RuntimeSemanticCorrectionDirectiveSchemaCurrent, PlanStep: RuntimeSemanticPlanStepProofBatch, ManifestHash: manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash, StablePrefixHash: "frozen-prefix", SourceObservationHash: StableRuntimeSemanticCorrectionObservationHash(observation), Attempt: 1, Violations: diagnostic.SemanticViolations}
				directive.Hash = StableRuntimeSemanticCorrectionDirectiveHash(directive)
				stored, _ := json.Marshal(directive)
				var restored RuntimeSemanticCorrectionDirective
				if json.Unmarshal(stored, &restored) != nil || restored.ValidateAgainstObservation(manifest, observation) != nil || !reflect.DeepEqual(restored.Normalized(), directive.Normalized()) {
					t.Fatal("exact correction lost on reload")
				}
				stale := restored.Clone()
				stale.Violations[0].Path += "stale"
				stale.Hash = StableRuntimeSemanticCorrectionDirectiveHash(stale)
				if stale.ValidateAgainstObservation(manifest, observation) == nil {
					t.Fatal("stale witness admitted")
				}
			}
			if !bytes.Equal(before, raw) {
				t.Fatal("repaired original output")
			}
		})
	}
}

func TestJointReviewSingleExcerptPreservesRealMissingFinding(t *testing.T) {
	_, binding, report := compositionFixture(t)
	check := &report.Checks[0]
	check.Obligations[0].ProofConsumerRefs = []string{RuntimeValidationInputConsumer}
	other := check.Obligations[0]
	other.ProofConsumerRefs = []string{"delivery", "rounds"}
	check.Obligations[0].Finding = RuntimeValidationInputMissing
	other.Finding = RuntimeValidationInputCovered
	check.Conclusion = RuntimeValidationInputConflict
	check.Obligations = append(check.Obligations, other)
	var duplicate *runtimeValidationInputViolation
	if err := report.ValidateAgainstBinding(binding); !errors.As(err, &duplicate) || duplicate.Actual != "duplicate_criterion_text" {
		t.Fatalf("per-consumer duplicate accepted: %v", err)
	}
	// This is a separate synthetic legal provider report, not an output repair.
	legal := report.Clone()
	legal.Checks[0].Obligations = legal.Checks[0].Obligations[:1]
	legal.Checks[0].Obligations[0].ProofConsumerRefs = RuntimeValidationProofConsumerRefs(*binding.InputReview)
	legal.Checks[0].Obligations[0].Reason = "Declared future rules map to source; ordinary behavioral assertions are still missing."
	if err := legal.ValidateAgainstBinding(binding); err != nil {
		t.Fatal(err)
	}
	if RuntimeValidationInputReviewConclusion(legal) != RuntimeValidationInputConflict || ValidateRuntimeSemanticInputReviewConsistency(RuntimeSemanticPlanDocument{Proposals: []RuntimeSemanticProofProposal{{ValidationInputReview: &legal}}}) == nil {
		t.Fatal("structural correction granted proof authority")
	}
	legal.Checks[0].Obligations[0].ProofConsumerRefs = []string{RuntimeValidationInputConsumer}
	var missing *runtimeValidationInputViolation
	if err := legal.ValidateAgainstBinding(binding); !errors.As(err, &missing) || missing.Actual != "missing_proof_consumer" {
		t.Fatalf("missing consumer not precisely rejected: %v", err)
	}
}
