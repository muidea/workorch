package llm

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"

	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeInputReviewSelectableProtocolsHaveCompleteOrderIndependentInstructions(t *testing.T) {
	request := runtimeInputReviewProviderFixture(t)
	requirement := request.SemanticManifest.ProofObligations[0].ValidationProofRequirement
	requirement.ValidationRecipeID = ""
	requirement.Hash = runtimecontract.StableRuntimeValidationProofRequirementHash(*requirement)
	catalog := &request.SemanticManifest.OwnerCatalogs[0]
	original := catalog.ValidationSurfaces[0].Clone()
	catalog.ValidationSurfaces = nil
	catalog.ValidationRecipes = nil
	catalog.ValidationRecipeRefs = nil
	for i, protocol := range []string{runtimecontract.RuntimeValidationInputReviewProtocol, runtimecontract.RuntimeValidationInputReviewProtocol, runtimecontract.RuntimeValidationInputReviewProtocol} {
		id := []string{"recipe-v1", "recipe-v2", "recipe-v3"}[i]
		binding, err := runtimecontract.RuntimeValidationProofRequirementBindingForRecipe(*requirement, id, runtimeValidationCheckAttestationsForLLMTest(requirement.RequiredCheckRefs)...)
		if err != nil {
			t.Fatal(err)
		}
		subject := original.ProofRequirementBindings[0].InputReview.Clone()
		subject.ProtocolRef = protocol
		// A scope on the first choice must not suppress a later choice's
		// diagnostic-source or protocol instructions.
		if i == 0 {
			subject.ScopeContractRef, subject.ScopeRequirements = "owner/scope/v1", "Only the supplied inputs are observed."
		}
		if i == 2 {
			subject.DiagnosticSourceContractRef = runtimecontract.RuntimeValidationDiagnosticSourceReviewV1
			subject.DiagnosticSources = []runtimecontract.RuntimeValidationDiagnosticSource{{SourceRef: "source-v3", NativeCaseRef: "owner/TestCase", InputTargetRef: subject.Files[0].TargetRef}}
		}
		binding.InputReview = &subject
		binding.InputReview.Hash = runtimecontract.StableRuntimeValidationInputReviewSubjectHash(subject, binding)
		surface := original.Clone()
		surface.ID, surface.ProofRequirementBindings = id, []runtimecontract.RuntimeValidationProofRequirementBinding{binding}
		catalog.ValidationSurfaces = append(catalog.ValidationSurfaces, surface)
		catalog.ValidationRecipes = append(catalog.ValidationRecipes, runtimecontract.RuntimeValidationRecipe{ID: id, AdapterRef: "owner/review/v1", Command: "check", InputReviewProtocolRef: protocol})
		catalog.ValidationRecipeRefs = append(catalog.ValidationRecipeRefs, id)
	}
	catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(*catalog)
	request.SemanticManifest.Hash = runtimecontract.StableRuntimeSemanticPlanManifestHash(request.SemanticManifest)
	refreshSemanticProgressForTest(t, &request)
	provider, err := runtimeProviderRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	var projection runtimeSemanticAuthorityProjection
	if err := json.Unmarshal([]byte(provider.PromptContext.Sections[3].Content), &projection); err != nil {
		t.Fatal(err)
	}
	instruction := projection.OutputInstruction
	for _, required := range []string{"For validation/input-consistency", "one independent report for EVERY exact sourceRef", "scopeContractRef/scopeRequirements"} {
		if strings.Count(instruction, required) != 1 {
			t.Fatalf("missing or duplicated selected-contract explanation: %s", required)
		}
	}
	reviews := projection.Manifest.ProofObligations[0].ValidationProofRequirement.InputReviews
	if len(reviews) != 3 {
		t.Fatal("selectable recipe projections missing")
	}
	for _, review := range reviews {
		if !reflect.DeepEqual(review.RequiredProofConsumerRefs, []string{runtimecontract.RuntimeValidationInputConsumer}) {
			t.Fatal("current required consumers missing")
		}
	}

	slices.Reverse(catalog.ValidationSurfaces)
	if got := runtimeSemanticOutputInstruction(request.SemanticContext, request.SemanticManifest); got != instruction {
		t.Fatal("selectable surface order changed review instructions")
	}
	context := request.SemanticContext.Clone()
	context.BatchCoverageWorkItemIDs = []common.CoverageWorkItemID{"unrelated"}
	if strings.Contains(runtimeSemanticOutputInstruction(context, request.SemanticManifest), "Independent input-consistency review") {
		t.Fatal("another CWI's review contract leaked into current batch")
	}
	context.PlanStep = runtimecontract.RuntimeSemanticPlanStepCoverageOrder
	if runtimeSemanticInputObligationsInstruction(context, request.SemanticManifest) != "" {
		t.Fatal("coverage order received validation review instructions")
	}
}

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
	catalog.ValidationRecipes = []runtimecontract.RuntimeValidationRecipe{{ID: "review-recipe", AdapterRef: "owner/review", Command: "check", InputReviewProtocolRef: runtimecontract.RuntimeValidationInputReviewProtocol}}
	catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	request.SemanticManifest.OwnerCatalogs = []runtimecontract.RuntimeOperationAdmissibility{catalog}
	request.SemanticManifest.AllowedOwnerModuleIDs = []string{runtimecontract.RuntimeModuleShell}
	request.SemanticManifest.AllowedOperationKinds = []string{runtimecontract.RuntimeBlockKindManagedValidationSession}
	request.SemanticManifest.AllowedTargetPolicies = []runtimecontract.RuntimeOperationTargetPolicy{runtimecontract.RuntimeOperationTargetPolicyExactTarget}
	request.SemanticManifest.ProofObligations = []runtimecontract.RuntimeSemanticProofObligation{{CoverageWorkItemID: "cwi-001", DeclarationRef: requirement.DeclarationRef, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation}, ValidationProofRequirement: &requirement}}
	request.SemanticManifest.Hash = runtimecontract.StableRuntimeSemanticPlanManifestHash(request.SemanticManifest)
	refreshSemanticProgressForTest(t, &request)
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
	refreshSemanticProgressForTest(t, &request)
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
	catalog.ValidationRecipes = []runtimecontract.RuntimeValidationRecipe{{ID: binding.ValidationRecipeID, AdapterRef: "owner/review/v1", Command: "check", InputReviewProtocolRef: runtimecontract.RuntimeValidationInputReviewProtocol}}
	binding.InputReview.ProtocolRef = runtimecontract.RuntimeValidationInputReviewProtocol
	binding.InputReview.Hash = runtimecontract.StableRuntimeValidationInputReviewSubjectHash(*binding.InputReview, *binding)
	catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(*catalog)
	request.SemanticManifest.Hash = runtimecontract.StableRuntimeSemanticPlanManifestHash(request.SemanticManifest)
	refreshSemanticProgressForTest(t, &request)
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
	refreshSemanticProgressForTest(t, &request)
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
	refreshSemanticProgressForTest(t, &request)
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
