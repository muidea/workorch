package llm

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	rc "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestSemanticLocalAliasesPreserveConflictBeforeContentAndIntent(t *testing.T) {
	text := strings.Repeat("完整当前 input `a`\n", 200)
	intent := &rc.RuntimeImplementationIntent{DeclarationRef: "original", RemainingWork: "actual missing work", Reason: "original reason"}
	prev := &rc.RuntimeImplementationRoundReviewSource{Ordinal: 17, DispatchID: "dispatch-17", ReceiptSetHash: "set-17", IntentBinding: &rc.RuntimeImplementationWorkIntentBinding{Intent: intent.Clone()}, Before: []rc.TargetSnapshot{{TargetRef: "input", Content: "different preimage"}}}
	input := rc.RuntimeValidationInputReviewFile{TargetRef: "input", ContentHash: "exact-hash", Content: text}
	p := runtimeSemanticAuthorityProjection{Schema: json.RawMessage(`{}`), WorkContext: &runtimeSemanticWorkProjection{Content: &rc.RuntimeImplementationWorkContent{Snapshots: []rc.TargetSnapshot{{TargetRef: input.TargetRef, ContentHash: input.ContentHash, Content: input.Content}}}, PriorRounds: []runtimeSemanticWorkRoundProjection{{Ordinal: 17, DispatchID: "dispatch-17", ReceiptSetHash: "set-17", Intent: intent}}}, Manifest: runtimeSemanticManifestProjection{ImplementationPurpose: &runtimeSemanticImplementationWorkProjection{PreviousRound: prev, InputRevision: &runtimeInputRevisionProjection{Binding: rc.RuntimeValidationProofRequirementBinding{InputReview: &rc.RuntimeValidationInputReviewSubject{Files: []rc.RuntimeValidationInputReviewFile{input, {TargetRef: "input", ContentHash: "old-hash", Content: text + "old"}}}}}}, ValidationGenerationContexts: []runtimeValidationGenerationContextProjection{{Context: &rc.RuntimeValidationGenerationContext{Files: []rc.RuntimeValidationInputReviewFile{input}}}}}}
	before, _ := json.Marshal(runtimeSemanticAuthorityInline(p))
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	var wire runtimeSemanticAuthorityWire
	json.Unmarshal(raw, &wire)
	if len(wire.FileRefs) != 1 || len(wire.IntentRefs) != 1 || len(wire.GenerationRefs) != 1 || !strings.Contains(string(raw), "different preimage") {
		t.Fatal("did not share exact sources or discarded original preimage")
	}
	var expanded runtimeSemanticAuthorityProjection
	if err = json.Unmarshal(raw, &expanded); err != nil || !reflect.DeepEqual(p, expanded) {
		t.Fatal("exact source expansion failed", err)
	}
	after, _ := json.Marshal(runtimeSemanticAuthorityInline(p))
	if string(before) != string(after) {
		t.Fatal("owner source mutated")
	}
	for _, kind := range []string{"missing", "wrong-snapshot", "mixed", "duplicate", "foreign-round", "cycle"} {
		t.Run(kind, func(t *testing.T) {
			var w runtimeSemanticAuthorityWire
			json.Unmarshal(raw, &w)
			switch kind {
			case "missing":
				w.WorkContext.Content = nil
			case "wrong-snapshot":
				w.FileRefs[0].Snapshot = 19
			case "mixed":
				w.Manifest.ImplementationPurpose.InputRevision.Binding.InputReview.Files[0].Content = "different"
			case "duplicate":
				w.FileRefs = append(w.FileRefs, w.FileRefs[0])
			case "foreign-round":
				w.WorkContext.PriorRounds[0].DispatchID = "foreign"
			case "cycle":
				n := 0
				w.GenerationRefs[0].Snapshot = nil
				w.GenerationRefs[0].SourceGeneration = &n
			}
			bad, _ := json.Marshal(w)
			if json.Unmarshal(bad, &expanded) == nil {
				t.Fatal("untrusted local reference accepted")
			}
		})
	}
}
func TestOrdinaryOrderDoesNotRequestImplementationIntent(t *testing.T) {
	r := runtimeSemanticWorkProviderRequestForTest(t, false)
	r.SemanticContext.PlanStep = rc.RuntimeSemanticPlanStepCoverageOrder
	r.SemanticContext.BatchID = ""
	r.SemanticContext.BatchCoverageWorkItemIDs = nil
	instruction := runtimeSemanticSharedWorkOutputInstruction(r.SemanticContext, r.SemanticManifest)
	if strings.Contains(instruction, "implementationIntent") || strings.Contains(instruction, "requirementReviews") || runtimeSemanticVisibleWorkContext(r.SemanticContext, r.SemanticManifest) != nil {
		t.Fatal("ordinary ordering acquired implementation rules/body")
	}
	schema, err := rc.RuntimeSemanticPlanJSONSchemaForRequest(r.SemanticManifest, rc.RuntimeSemanticPlanStepCoverageOrder, "", 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(schema, `"implementationIntent"`) {
		t.Fatal("ordinary order schema gained intent")
	}
	purpose := runtimeSemanticWorkProviderRequestForTest(t, true)
	if !strings.Contains(runtimeSemanticSharedWorkOutputInstruction(purpose.SemanticContext, purpose.SemanticManifest), "implementationIntent") {
		t.Fatal("real purpose lost mandatory intent")
	}
}

func TestStructuralInputLocalReferenceExpandsExactOriginalBytes(t *testing.T) {
	content := strings.Repeat("original malformed input\n", 200)
	f := rc.RuntimeValidationInputReviewFile{TargetRef: "checks/input", Content: content, ContentHash: rc.RuntimeValidationInputContentHash(content)}
	p := runtimeSemanticAuthorityProjection{Schema: json.RawMessage(`{}`), WorkContext: &runtimeSemanticWorkProjection{Content: &rc.RuntimeImplementationWorkContent{Snapshots: []rc.TargetSnapshot{{TargetRef: f.TargetRef, ContentHash: f.ContentHash, Content: f.Content}}}}, Manifest: runtimeSemanticManifestProjection{ImplementationPurpose: &runtimeSemanticImplementationWorkProjection{InputRevision: &runtimeInputRevisionProjection{Observation: &runtimeInputRevisionObservationProjection{Files: []rc.RuntimeValidationInputReviewFile{f}}}}}}
	inline, _ := json.Marshal(runtimeSemanticAuthorityInline(p))
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	var w runtimeSemanticAuthorityWire
	if err := json.Unmarshal(raw, &w); err != nil || len(w.FileRefs) != 1 || len(raw) >= len(inline) {
		t.Fatal("structural input did not reuse its exact local snapshot", err)
	}
	var expanded runtimeSemanticAuthorityProjection
	if err := json.Unmarshal(raw, &expanded); err != nil || !reflect.DeepEqual(p, expanded) {
		t.Fatal("structural source expansion differs from original", err)
	}
	t.Logf("fixture C3 inline=%d bytes, physical=%d bytes; expanded source remains exact", len(inline), len(raw))
	for _, kind := range []string{"missing source", "other target", "other version", "mixed content", "duplicate"} {
		t.Run(kind, func(t *testing.T) {
			var bad runtimeSemanticAuthorityWire
			if err := json.Unmarshal(raw, &bad); err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "missing source":
				bad.WorkContext.Content.Snapshots = nil
			case "other target":
				bad.WorkContext.Content.Snapshots[0].TargetRef = "another/input"
			case "other version":
				bad.WorkContext.Content.Snapshots[0].ContentHash = "new-version"
			case "mixed content":
				bad.Manifest.ImplementationPurpose.InputRevision.Observation.Files[0].Content = "another input"
			case "duplicate":
				bad.FileRefs = append(bad.FileRefs, bad.FileRefs[0])
			}
			bytes, _ := json.Marshal(bad)
			if err := json.Unmarshal(bytes, &expanded); err == nil {
				t.Fatal("non-exact structural alias accepted")
			}
		})
	}
}

func TestGenerationAliasesRetainExactReviewAndEarlierGenerationSources(t *testing.T) {
	first := rc.RuntimeValidationInputReviewFile{TargetRef: "source-a", ContentHash: "hash-a", Content: strings.Repeat("original a\n", 100)}
	second := rc.RuntimeValidationInputReviewFile{TargetRef: "source-b", ContentHash: "hash-b", Content: strings.Repeat("original b\n", 100)}
	p := runtimeSemanticAuthorityProjection{Schema: json.RawMessage(`{}`), Manifest: runtimeSemanticManifestProjection{
		ProofObligations: []runtimeSemanticProofObligationProjection{{ValidationProofRequirement: &runtimeSemanticValidationProofRequirementProjection{InputReviews: []runtimeSemanticInputReviewProjection{{Files: []rc.RuntimeValidationInputReviewFile{first}}}}}},
		ValidationGenerationContexts: []runtimeValidationGenerationContextProjection{
			{Context: &rc.RuntimeValidationGenerationContext{Files: []rc.RuntimeValidationInputReviewFile{first, second}}},
			{Context: &rc.RuntimeValidationGenerationContext{Files: []rc.RuntimeValidationInputReviewFile{second, {TargetRef: second.TargetRef, ContentHash: "changed", Content: second.Content + "actual change"}}}},
		},
	}}
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	var w runtimeSemanticAuthorityWire
	if err := json.Unmarshal(raw, &w); err != nil {
		t.Fatal(err)
	}
	if len(w.GenerationRefs) != 2 || w.GenerationRefs[0].SourceReview == nil || w.GenerationRefs[1].SourceGeneration == nil {
		t.Fatal("exact sources were not shared")
	}
	var expanded runtimeSemanticAuthorityProjection
	if err := json.Unmarshal(raw, &expanded); err != nil || !reflect.DeepEqual(p, expanded) {
		t.Fatal("generation sources did not expand exactly", err)
	}
	for _, kind := range []string{"missing-review", "wrong-hash", "wrong-target", "forward", "mixed", "duplicate", "rule"} {
		t.Run(kind, func(t *testing.T) {
			var bad runtimeSemanticAuthorityWire
			json.Unmarshal(raw, &bad)
			switch kind {
			case "missing-review":
				bad.Manifest.ProofObligations[0].ValidationProofRequirement.InputReviews = nil
			case "wrong-hash":
				bad.Manifest.ValidationGenerationContexts[1].Context.Files[0].ContentHash = "foreign"
			case "wrong-target":
				bad.Manifest.ValidationGenerationContexts[1].Context.Files[0].TargetRef = "foreign"
			case "forward":
				n := 1
				bad.GenerationRefs[1].SourceGeneration = &n
			case "mixed":
				n := 0
				bad.GenerationRefs[0].Snapshot = &n
			case "duplicate":
				bad.GenerationRefs = append(bad.GenerationRefs, bad.GenerationRefs[1])
			case "rule":
				bad.ReferenceRule = "unknown"
			}
			value, _ := json.Marshal(bad)
			if json.Unmarshal(value, &expanded) == nil {
				t.Fatal("untrusted generation alias accepted")
			}
		})
	}
}
