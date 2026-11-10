package llm

import (
	"encoding/json"
	"reflect"
	"testing"

	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	rc "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestInputReviewExecutionMethodIsExactDynamicFrozenInput(t *testing.T) {
	request := runtimeInputReviewProviderFixture(t)
	before, err := runtimeProviderRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	catalog := &request.SemanticManifest.OwnerCatalogs[0]
	binding := &catalog.ValidationSurfaces[0].ProofRequirementBindings[0]
	identity := rc.RuntimeValidationExecutionStageIdentity{ID: "ordinary", Scope: "whole", RequireTests: true}
	binding.InputReview.ExecutionPlan = &rc.RuntimeValidationExecutionPlan{Stages: []rc.RuntimeValidationPlannedStage{{Identity: identity, Command: "owner-tool", Args: []string{"all"}, Instrumentation: []string{"independent_observation"}, RequiredCheckRefs: binding.RequiredCheckRefs}}}
	for i := range binding.CheckAttestations {
		binding.CheckAttestations[i].ExecutionStagesHash = rc.StableRuntimeValidationExecutionStagesHash([]rc.RuntimeValidationExecutionStageIdentity{identity})
	}
	binding.InputReview.Hash = rc.StableRuntimeValidationInputReviewSubjectHash(*binding.InputReview, *binding)
	catalog.SnapshotID = rc.StableRuntimeOperationAdmissibilitySnapshotID(*catalog)
	request.SemanticManifest.Hash = rc.StableRuntimeSemanticPlanManifestHash(request.SemanticManifest)
	refreshSemanticProgressForTest(t, &request)
	current, err := runtimeProviderRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if current.PromptContext.Sections[i].Content != before.PromptContext.Sections[i].Content {
			t.Fatalf("method observation changed stable C%d", i)
		}
	}
	var projection runtimeSemanticAuthorityProjection
	if err := json.Unmarshal([]byte(current.PromptContext.Sections[3].Content), &projection); err != nil {
		t.Fatal(err)
	}
	method := projection.Manifest.ProofObligations[0].ValidationProofRequirement.InputReviews[0].ExecutionPlan
	if !reflect.DeepEqual(method, binding.InputReview.ExecutionPlan) {
		t.Fatal("method lost declared command, instrumentation or complete check membership")
	}
	raw, _ := json.Marshal(request)
	var frozen llmmodels.RuntimeCandidateRequest
	if err := json.Unmarshal(raw, &frozen); err != nil {
		t.Fatal(err)
	}
	replay, err := runtimeProviderRequest(frozen)
	if err != nil || !reflect.DeepEqual(current.PromptContext.Sections, replay.PromptContext.Sections) {
		t.Fatal("frozen method changed during recovery", err)
	}
	method.Stages[0].Args[0] = "another method"
	corrupt, _ := json.Marshal(projection)
	replay.PromptContext.Sections[3].Content = string(corrupt)
	if validateRuntimeSemanticContextIntegrity(request.SemanticContext, request.SemanticManifest, 0, nil, nil, replay.ContextBudget, replay.PromptContext) == nil {
		t.Fatal("rendered method drift admitted")
	}
	if binding.InputReview.ExecutionPlan.Stages[0].Args[0] != "all" {
		t.Fatal("provider projection aliased owner method")
	}
}

// Method declarations must survive the whole maintenance handoff, not just
// the initial review. Purpose selection, work planning and operation generation
// all consume the original subject; none may substitute today's method.
func TestInputReviewExecutionMethodSurvivesMaintenanceHandoff(t *testing.T) {
	v := inputRevisionProviderFixture(t)
	b := &v.Binding
	s := b.InputReview
	identity := rc.RuntimeValidationExecutionStageIdentity{ID: "ordinary", Scope: "whole", RequireTests: true}
	s.ExecutionPlan = &rc.RuntimeValidationExecutionPlan{TimeoutMillis: s.Recipe.TimeoutMillis, Stages: []rc.RuntimeValidationPlannedStage{{Identity: identity, Command: "owner-tool", Args: []string{"all"}, Instrumentation: []string{"independent_observation"}, RequiredCheckRefs: b.RequiredCheckRefs}}}
	for i := range b.CheckAttestations {
		b.CheckAttestations[i].ExecutionStagesHash = rc.StableRuntimeValidationExecutionStagesHash([]rc.RuntimeValidationExecutionStageIdentity{identity})
	}
	s.Hash = rc.StableRuntimeValidationInputReviewSubjectHash(*s, *b)
	v.Report.SubjectHash = s.Hash
	if err := v.Validate(); err != nil {
		t.Fatal(err)
	}
	want := s.ExecutionPlan.Clone()
	for _, purpose := range []bool{true, false} {
		t.Run(map[bool]string{true: "purpose", false: "work"}[purpose], func(t *testing.T) {
			request := runtimeSemanticWorkProviderRequestForTest(t, purpose, v)
			provider, err := runtimeSemanticPlanProviderRequest(request)
			if err != nil {
				t.Fatal(err)
			}
			encoded, err := json.Marshal(request)
			if err != nil {
				t.Fatal(err)
			}
			var frozen llmmodels.RuntimeCandidateRequest
			if err := json.Unmarshal(encoded, &frozen); err != nil {
				t.Fatal(err)
			}
			recovered, err := runtimeSemanticPlanProviderRequest(frozen)
			if err != nil || !reflect.DeepEqual(provider.PromptContext.Sections, recovered.PromptContext.Sections) {
				t.Fatal("maintenance recovery changed frozen method", err)
			}
			var authority runtimeSemanticAuthorityProjection
			if err := json.Unmarshal([]byte(provider.PromptContext.Sections[3].Content), &authority); err != nil {
				t.Fatal(err)
			}
			work := authority.Manifest.ImplementationPurpose
			if !purpose {
				work = authority.Manifest.ProofObligations[0].ImplementationWork
			}
			method := work.InputRevision.Binding.InputReview.ExecutionPlan
			if method == nil || !reflect.DeepEqual(*method, want) {
				t.Fatal("maintenance lost original stage/check mapping or instrumentation")
			}
			method.Stages[0].Args[0] = "substituted"
			corrupt, _ := json.Marshal(authority)
			provider.PromptContext.Sections[3].Content = string(corrupt)
			if validateRuntimeSemanticContextIntegrity(request.SemanticContext, request.SemanticManifest, 0, nil, nil, provider.ContextBudget, provider.PromptContext) == nil {
				t.Fatal("maintenance accepted rendered method substitution")
			}
		})
	}
	operations := runtimeCandidateOperationContractsForManifest([]rc.RuntimeCandidateOperationManifest{{Kind: rc.StepKindFileEdit, InputRevision: v}}, rc.RuntimeCandidateWireContract{})
	method := operations[0].InputRevision.Binding.InputReview.ExecutionPlan
	if method == nil || !reflect.DeepEqual(*method, want) {
		t.Fatal("operation generation lost original method")
	}
	method.Stages[0].Args[0] = "caller mutation"
	if !reflect.DeepEqual(*v.Binding.InputReview.ExecutionPlan, want) {
		t.Fatal("maintenance projection aliased owner method")
	}
}
