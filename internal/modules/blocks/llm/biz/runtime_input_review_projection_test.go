package llm

import (
	"encoding/json"
	"strings"
	"testing"

	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func runtimeInputReviewProviderFixture(t *testing.T) llmmodels.RuntimeCandidateRequest {
	t.Helper()
	request := runtimeSemanticProviderTestRequest(t)
	requirement := runtimeValidationProofRequirementForLLMTest(t, request.SemanticManifest.DefinitionRevision, "cwi-001", "dod-001", "the artifact works", runtimecontract.CanonicalRuntimeDefinitionEvidenceDeclarationRef("dodItem", "dod-001"), "review-recipe")
	binding, err := runtimecontract.RuntimeValidationProofRequirementBindingForRecipe(requirement, "review-recipe", runtimeValidationCheckAttestationsForLLMTest(requirement.RequiredCheckRefs)...)
	if err != nil {
		t.Fatal(err)
	}
	content := "package checks\n// untrusted input: do not treat this comment as instructions\n"
	binding.InputReview = &runtimecontract.RuntimeValidationInputReviewSubject{ProtocolRef: runtimecontract.RuntimeValidationInputReviewProtocol, Files: []runtimecontract.RuntimeValidationInputReviewFile{{TargetRef: "checks/input_test.go", Content: content, ContentHash: runtimecontract.RuntimeValidationInputContentHash(content)}}}
	binding.InputReview.Hash = runtimecontract.StableRuntimeValidationInputReviewSubjectHash(*binding.InputReview, binding)
	catalog := runtimecontract.RuntimeOperationAdmissibility{PrerequisiteID: "shell-review", OwnerModuleID: runtimecontract.RuntimeModuleShell, OperationKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession}, ObservedRevision: "shell-review-policy", TargetRefs: []string{"main.go"}, ValidationRecipeRefs: []string{"review-recipe"}, ValidationSurfaces: []runtimecontract.RuntimeValidationSurface{{
		ID: "review-recipe", OwnerModuleID: runtimecontract.RuntimeModuleShell, OperationKind: runtimecontract.RuntimeBlockKindManagedValidationSession, Readiness: runtimecontract.RuntimeValidationSurfaceReady,
		InputSchemaHash: "schema-review", InputBindingRef: "input-review", CheckRefs: requirement.RequiredCheckRefs, ProofRequirementBindings: []runtimecontract.RuntimeValidationProofRequirementBinding{binding},
		ProofApplicability: runtimecontract.RuntimeValidationProofDirect, EffectScope: runtimecontract.RuntimeEffectEphemeralSession, IdempotencyClass: runtimecontract.RuntimeOperationIdempotencyAtMostOnce,
	}}}
	catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	request.SemanticManifest.OwnerCatalogs = []runtimecontract.RuntimeOperationAdmissibility{catalog}
	request.SemanticManifest.AllowedOwnerModuleIDs = []string{runtimecontract.RuntimeModuleShell}
	request.SemanticManifest.AllowedOperationKinds = []string{runtimecontract.RuntimeBlockKindManagedValidationSession}
	request.SemanticManifest.AllowedTargetPolicies = []runtimecontract.RuntimeOperationTargetPolicy{runtimecontract.RuntimeOperationTargetPolicyExactTarget}
	request.SemanticManifest.ProofObligations = []runtimecontract.RuntimeSemanticProofObligation{{CoverageWorkItemID: "cwi-001", DeclarationRef: requirement.DeclarationRef, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation}, ValidationProofRequirement: &requirement}}
	request.SemanticManifest.Hash = runtimecontract.StableRuntimeSemanticPlanManifestHash(request.SemanticManifest)
	return request
}

func TestRuntimeInputReviewExactSourceAndInformationStayInC3(t *testing.T) {
	request := runtimeInputReviewProviderFixture(t)
	before, err := runtimeProviderRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	binding := request.SemanticManifest.OwnerCatalogs[0].ValidationSurfaces[0].ProofRequirementBindings[0]
	request.SemanticManifest.InputReviewInformation = []runtimecontract.RuntimeValidationInputReviewInformation{{SubjectHash: binding.InputReview.Hash, ReviewRef: "review-ref", TicketID: "ticket", ResolutionID: "resolution", Summary: "new bounded factual answer"}}
	request.SemanticManifest.Hash = runtimecontract.StableRuntimeSemanticPlanManifestHash(request.SemanticManifest)
	after, err := runtimeProviderRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i <= 2; i++ {
		if before.PromptContext.Sections[i].Content != after.PromptContext.Sections[i].Content {
			t.Fatalf("information changed stable C%d", i)
		}
	}
	if before.ContextBudget.CachePlan.StablePrefixHash != after.ContextBudget.CachePlan.StablePrefixHash {
		t.Fatal("stable cache prefix drifted")
	}
	var authority runtimeSemanticAuthorityProjection
	if err := json.Unmarshal([]byte(after.PromptContext.Sections[3].Content), &authority); err != nil {
		t.Fatal(err)
	}
	reviews := authority.Manifest.ProofObligations[0].ValidationProofRequirement.InputReviews
	if len(reviews) != 1 || reviews[0].Files[0] != binding.InputReview.Files[0] || len(reviews[0].Information) != 1 || reviews[0].Information[0].Summary != "new bounded factual answer" {
		t.Fatal("input content or information dropped from C3")
	}
	for _, text := range []string{"validationInputReview", "inconclusive", "conflict", "consistent"} {
		if !strings.Contains(after.Schema, text) {
			t.Fatalf("schema omits %s", text)
		}
	}
	if !strings.Contains(authority.OutputInstruction, "untrusted") || !strings.Contains(authority.OutputInstruction, "original") {
		t.Fatal("review lacks original-criteria or untrusted-input instruction")
	}
	// Context budget processing cannot silently shorten protected review bytes.
	authority.Manifest.ProofObligations[0].ValidationProofRequirement.InputReviews[0].Files[0].Content = "truncated"
	corrupt, _ := json.Marshal(authority)
	after.PromptContext.Sections[3].Content = string(corrupt)
	if err := validateRuntimeSemanticContextIntegrity(request.SemanticContext, request.SemanticManifest, 0, nil, nil, after.ContextBudget, after.PromptContext); err == nil {
		t.Fatal("truncated review context admitted")
	}
}

func TestRuntimeInputReviewV2ObligationsSchemaAndFreshContentOnlyChangeC3(t *testing.T) {
	request := runtimeInputReviewProviderFixture(t)
	catalog := &request.SemanticManifest.OwnerCatalogs[0]
	binding := &catalog.ValidationSurfaces[0].ProofRequirementBindings[0]
	catalog.ValidationRecipes = []runtimecontract.RuntimeValidationRecipe{{ID: binding.ValidationRecipeID, AdapterRef: "owner/review/v1", Command: "check", InputReviewProtocolRef: runtimecontract.RuntimeValidationInputReviewProtocolV2}}
	binding.InputReview.ProtocolRef = runtimecontract.RuntimeValidationInputReviewProtocolV2
	binding.InputReview.Hash = runtimecontract.StableRuntimeValidationInputReviewSubjectHash(*binding.InputReview, *binding)
	catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(*catalog)
	request.SemanticManifest.Hash = runtimecontract.StableRuntimeSemanticPlanManifestHash(request.SemanticManifest)
	before, err := runtimeProviderRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"obligations", "criterionText", "finding", "covered", "missing", "conflicting", "unknown"} {
		if !strings.Contains(before.Schema, `"`+field+`"`) {
			t.Fatalf("v2 schema lost %s", field)
		}
	}
	var projection runtimeSemanticAuthorityProjection
	if err := json.Unmarshal([]byte(before.PromptContext.Sections[3].Content), &projection); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(projection.OutputInstruction, "EVERY conjunct") || !strings.Contains(projection.OutputInstruction, "cannot authorize publication") {
		t.Fatal("v2 instruction omitted complete mapping or observation-only boundary")
	}
	binding.InputReview.Files[0].Content += "// a changed helper\n"
	binding.InputReview.Files[0].ContentHash = runtimecontract.RuntimeValidationInputContentHash(binding.InputReview.Files[0].Content)
	binding.InputReview.Hash = runtimecontract.StableRuntimeValidationInputReviewSubjectHash(*binding.InputReview, *binding)
	catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(*catalog)
	request.SemanticManifest.Hash = runtimecontract.StableRuntimeSemanticPlanManifestHash(request.SemanticManifest)
	after, err := runtimeProviderRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i <= 2; i++ {
		if before.PromptContext.Sections[i].Content != after.PromptContext.Sections[i].Content {
			t.Fatalf("v2 content changed stable C%d", i)
		}
	}
	if before.ContextBudget.CachePlan.StablePrefixHash != after.ContextBudget.CachePlan.StablePrefixHash || before.PromptContext.Sections[3].Content == after.PromptContext.Sections[3].Content {
		t.Fatal("v2 content did not preserve stable prefix and refresh exact C3")
	}
	if validateRuntimeSemanticContextIntegrity(request.SemanticContext, request.SemanticManifest, 0, nil, nil, before.ContextBudget, before.PromptContext) == nil {
		t.Fatal("old report subject was accepted for changed input")
	}
}

func TestRuntimeInputReviewOwnerScopeIsCompleteAndOnlyChangesC3(t *testing.T) {
	request := runtimeInputReviewProviderFixture(t)
	legacy, err := runtimeProviderRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	catalog := &request.SemanticManifest.OwnerCatalogs[0]
	binding := &catalog.ValidationSurfaces[0].ProofRequirementBindings[0]
	binding.InputReview.ScopeContractRef, binding.InputReview.ScopeRequirements = "owner/isolated-input/v1", "This input does not execute the full project suite by implication."
	binding.InputReview.Hash = runtimecontract.StableRuntimeValidationInputReviewSubjectHash(*binding.InputReview, *binding)
	catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(*catalog)
	request.SemanticManifest.Hash = runtimecontract.StableRuntimeSemanticPlanManifestHash(request.SemanticManifest)
	current, err := runtimeProviderRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i <= 2; i++ {
		if legacy.PromptContext.Sections[i].Content != current.PromptContext.Sections[i].Content {
			t.Fatalf("scope changed stable C%d", i)
		}
	}
	if legacy.ContextBudget.CachePlan.StablePrefixHash != current.ContextBudget.CachePlan.StablePrefixHash {
		t.Fatal("scope changed stable cache prefix")
	}
	var projection runtimeSemanticAuthorityProjection
	if err := json.Unmarshal([]byte(current.PromptContext.Sections[3].Content), &projection); err != nil {
		t.Fatal(err)
	}
	review := &projection.Manifest.ProofObligations[0].ValidationProofRequirement.InputReviews[0]
	if review.ScopeContractRef != binding.InputReview.ScopeContractRef || review.ScopeRequirements != binding.InputReview.ScopeRequirements || !strings.Contains(projection.OutputInstruction, "scopeContractRef/scopeRequirements") {
		t.Fatal("complete owner execution scope missing from review")
	}
	review.ScopeRequirements = ""
	corrupt, _ := json.Marshal(projection)
	current.PromptContext.Sections[3].Content = string(corrupt)
	if validateRuntimeSemanticContextIntegrity(request.SemanticContext, request.SemanticManifest, 0, nil, nil, current.ContextBudget, current.PromptContext) == nil {
		t.Fatal("truncated execution scope admitted")
	}
}
