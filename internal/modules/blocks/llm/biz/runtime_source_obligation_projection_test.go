package llm

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func runtimeSourceObligationProviderFixture(t *testing.T) llmmodels.RuntimeCandidateRequest {
	t.Helper()
	request := runtimeInputReviewProviderFixture(t)
	context := &request.SemanticContext
	context.Activation.ExecutionContract.ProfileContracts = []runtimecontract.RuntimeProfileContract{{ID: "profile", EvidenceRequirements: []runtimecontract.RuntimeProfileEvidenceRequirement{{
		ID: "dod-001", Scope: "dodItem", RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation},
		AcceptedStepKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession}, ValidationRecipeID: "review-recipe",
	}}}}
	inventory := (runtimecontract.RuntimeSourceObligationInventory{
		SchemaVersion: runtimecontract.RuntimeSourceObligationInventorySchemaCurrent,
		CandidateHash: "candidate", SpecificationHash: "specification", DefinitionRevision: context.Definition.Revision, StartupInputHash: context.Definition.StartupInputHash,
		Sources: []runtimecontract.RuntimeDefinitionSource{
			{CandidateItemRef: "goal", ItemRef: "goal-formal", SourceScope: "goal", SourceID: "goal", Text: context.Definition.GoalDefinition},
			{CandidateItemRef: "dod", ItemRef: "dod-formal", SourceScope: "dodItem", SourceID: "dod-001", Text: "the artifact works"},
		},
		Requirements: []runtimecontract.RuntimeSourceRequirement{{ID: "requirement", DimensionID: "dimension", Required: true, SourceKind: "USER_EXPLICIT", SourceItemRefs: []string{"dod", "goal"}, Text: "源要求"}},
		Obligations: []runtimecontract.RuntimeSourceObligation{
			{ID: "A", RequirementID: "requirement", DoDItemRefs: []string{"dod"}, Text: " A 原判据\n "},
			{ID: "B", RequirementID: "requirement", DoDItemRefs: []string{"dod"}, Text: " B 独立判据\t "},
		},
		Verifications: []runtimecontract.RuntimeSourceVerification{
			{ID: "verify-A", ObligationID: "A", Declaration: " 验证 A\n ", ProfileID: "profile", EvidenceRequirementID: "dod-001", Scope: "dodItem"},
			{ID: "verify-B", ObligationID: "B", Declaration: " 验证 B\t ", ProfileID: "profile", EvidenceRequirementID: "dod-001", Scope: "dodItem"},
		},
	}).WithHash()
	sealedInventory := inventory.Clone()
	context.Definition.SourceObligations = &sealedInventory
	items := runtimecontract.NormalizeRuntimeCoverageItems(context.DefinitionSeed.CoverageItems)
	checks, err := runtimecontract.CompileRuntimeSourceObligationChecks(inventory, items[0])
	if err != nil {
		t.Fatal(err)
	}
	items[0].SourceObligationChecks = checks
	context.DefinitionSeed, err = runtimecontract.CompileRuntimeDefinitionSeedWithSourceObligations(context.Definition, context.Activation, items, context.DefinitionSeed.Slices, &inventory)
	if err != nil {
		t.Fatal(err)
	}
	requirements, err := runtimecontract.CompileRuntimeValidationProofRequirements(context.Definition.Revision, items, []runtimecontract.RuntimeDefinitionFocus{{
		ID: "focus", SliceID: items[0].SliceID, AssessmentScope: items[0].SourceScope, AssessmentID: items[0].SourceID, AssessmentText: items[0].TargetText,
		CoverageWorkItemIDs: []common.CoverageWorkItemID{items[0].ID}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation},
		AcceptedStepKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession}, EvidenceDeclarationRef: runtimecontract.CanonicalRuntimeDefinitionEvidenceDeclarationRef(items[0].SourceScope, items[0].SourceID), ValidationRecipeID: "review-recipe",
	}})
	if err != nil || len(requirements) != 1 {
		t.Fatalf("requirements: %+v %v", requirements, err)
	}
	requirement := requirements[0]
	catalog := &request.SemanticManifest.OwnerCatalogs[0]
	surface := &catalog.ValidationSurfaces[0]
	binding, err := runtimecontract.RuntimeValidationProofRequirementBindingForRecipe(requirement, "review-recipe", runtimeValidationCheckAttestationsForLLMTest(requirement.RequiredCheckRefs)...)
	if err != nil {
		t.Fatal(err)
	}
	inputReview := surface.ProofRequirementBindings[0].InputReview.Clone()
	inputReview.ProtocolRef = runtimecontract.RuntimeValidationInputReviewProtocolV2
	catalog.ValidationRecipes = []runtimecontract.RuntimeValidationRecipe{{ID: "review-recipe", AdapterRef: "owner/review/v1", Command: "check", InputReviewProtocolRef: runtimecontract.RuntimeValidationInputReviewProtocolV2}}
	binding.InputReview = &inputReview
	binding.InputReview.Hash = runtimecontract.StableRuntimeValidationInputReviewSubjectHash(*binding.InputReview, binding)
	surface.CheckRefs = requirement.RequiredCheckRefs
	surface.ProofRequirementBindings = []runtimecontract.RuntimeValidationProofRequirementBinding{binding}
	catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(*catalog)
	request.SemanticManifest.CoverageItems = items
	request.SemanticManifest.DefinitionSeedHash = context.DefinitionSeed.Hash
	request.SemanticManifest.ProofObligations[0].ValidationProofRequirement = &requirement
	request.SemanticManifest.Hash = runtimecontract.StableRuntimeSemanticPlanManifestHash(request.SemanticManifest)
	request.StablePrefixHash = runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(context.Definition, context.Activation.ExecutionContract)
	refreshSemanticProgressForTest(t, &request)
	return request
}

func TestRuntimeSourceObligationProviderPreservesExactC1AndC3(t *testing.T) {
	request := runtimeSourceObligationProviderFixture(t)
	before, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	provider, err := runtimeProviderRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	var definition runtimeCandidateDefinitionProjection
	if err := json.Unmarshal([]byte(provider.PromptContext.Sections[1].Content), &definition); err != nil {
		t.Fatal(err)
	}
	if !runtimecontract.SameRuntimeSourceObligationInventory(definition.SourceObligations, request.SemanticContext.Definition.SourceObligations) {
		t.Fatal("C1 lost exact immutable source inventory")
	}
	var authority runtimeSemanticAuthorityProjection
	if err := json.Unmarshal([]byte(provider.PromptContext.Sections[3].Content), &authority); err != nil {
		t.Fatal(err)
	}
	requirement := authority.Manifest.ProofObligations[0].ValidationProofRequirement
	want := request.SemanticManifest.ProofObligations[0].ValidationProofRequirement
	if !reflect.DeepEqual(requirement.SourceObligationChecks, runtimeSemanticSourceObligationChecksForProvider(want.SourceObligationChecks)) || len(requirement.RequiredCheckRefs) != 3 {
		t.Fatal("C3 lost independent A/B criteria, declarations or all_of checks")
	}
	if !strings.Contains(authority.OutputInstruction, "For a requiredCheckRef in sourceObligationChecks") || !strings.Contains(authority.OutputInstruction, "never substitute the shared CWI text") {
		t.Fatal("source input review instruction lost per-check original criterion")
	}
	after, _ := json.Marshal(request)
	if string(before) != string(after) {
		t.Fatal("source rendering mutated sealed request")
	}
	limited := request
	limited.SemanticContext = request.SemanticContext.Clone()
	limited.SemanticContext.ProviderCeiling.MaxBytes = 1024
	rejected, err := runtimeProviderRequest(limited)
	if err == nil || rejected.ContextBudget == nil || !rejected.ContextBudget.OverBudget {
		t.Fatal("source inventory silently bypassed provider ceiling")
	}
	if rejected.PromptContext.Sections[1].Content != provider.PromptContext.Sections[1].Content || rejected.PromptContext.Sections[3].Content != provider.PromptContext.Sections[3].Content {
		t.Fatal("budget refusal silently truncated exact source material")
	}
}

func TestRuntimeSourceObligationProviderRejectsOmittedAndChangedPreimages(t *testing.T) {
	request := runtimeSourceObligationProviderFixture(t)
	for _, name := range []string{"C1 missing B", "C1 altered criterion", "C3 missing B", "C3 wrong source", "C3 altered declaration"} {
		t.Run(name, func(t *testing.T) {
			provider, err := runtimeProviderRequest(request)
			if err != nil {
				t.Fatal(err)
			}
			if name == "C1 missing B" || name == "C1 altered criterion" {
				var projection runtimeCandidateDefinitionProjection
				if err := json.Unmarshal([]byte(provider.PromptContext.Sections[1].Content), &projection); err != nil {
					t.Fatal(err)
				}
				if name == "C1 missing B" {
					projection.SourceObligations.Obligations = projection.SourceObligations.Obligations[:1]
				} else {
					projection.SourceObligations.Obligations[0].Text = "A 原判据"
				}
				data, _ := json.Marshal(projection)
				provider.PromptContext.Sections[1].Content = string(data)
			} else {
				var projection runtimeSemanticAuthorityProjection
				if err := json.Unmarshal([]byte(provider.PromptContext.Sections[3].Content), &projection); err != nil {
					t.Fatal(err)
				}
				requirement := projection.Manifest.ProofObligations[0].ValidationProofRequirement
				switch name {
				case "C3 missing B":
					requirement.SourceObligationChecks = requirement.SourceObligationChecks[:1]
				case "C3 wrong source":
					requirement.SourceObligationChecks[1].ObligationID = "A"
				case "C3 altered declaration":
					requirement.SourceObligationChecks[0].Declaration = "验证 A"
				}
				data, _ := json.Marshal(projection)
				provider.PromptContext.Sections[3].Content = string(data)
			}
			if err := validateRuntimeSemanticContextIntegrity(request.SemanticContext, request.SemanticManifest, 0, nil, nil, provider.ContextBudget, provider.PromptContext); err == nil {
				t.Fatal("incomplete or altered exact source preimage admitted")
			}
		})
	}
}

func TestRuntimeSourceObligationProviderCorrectionPreservesC0ThroughC3(t *testing.T) {
	request := runtimeSourceObligationProviderFixture(t)
	before, err := runtimeProviderRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	violation := runtimecontract.RuntimeSemanticContractViolation{
		Kind: runtimecontract.RuntimeSemanticContractViolationProposalConstraint, Path: "$.proposals[0].target.policy", CoverageWorkItemID: "cwi-001", ProposalIndex: 0,
		Rule: runtimecontract.RuntimeSemanticConstraintAllowedValue, Actual: string(runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget), Allowed: []string{string(runtimecontract.RuntimeOperationTargetPolicyExactTarget)},
	}
	observation := runtimeSemanticCorrectionObservationForProviderTest(&runtimecontract.RuntimeOriginalDiagnostic{
		SchemaVersion: runtimecontract.RuntimeOriginalDiagnosticSchemaCurrent, OwnerModuleID: runtimecontract.RuntimeModuleLLM,
		OriginalClass: "semantic", OriginalCode: "invalid_policy", OriginalMessage: "原始失败", Signature: "semantic/invalid-policy", SemanticViolations: []runtimecontract.RuntimeSemanticContractViolation{violation},
	})
	directive := runtimecontract.RuntimeSemanticCorrectionDirective{
		SchemaVersion: runtimecontract.RuntimeSemanticCorrectionDirectiveSchemaCurrent, PlanStep: request.SemanticContext.PlanStep,
		ManifestHash: request.SemanticManifest.Hash, DefinitionSeedHash: request.SemanticManifest.DefinitionSeedHash,
		StablePrefixHash: request.StablePrefixHash, SourceObservationHash: runtimecontract.StableRuntimeSemanticCorrectionObservationHash(*observation), Attempt: 1,
		Violations: []runtimecontract.RuntimeSemanticContractViolation{violation},
	}
	directive.Hash = runtimecontract.StableRuntimeSemanticCorrectionDirectiveHash(directive)
	request.CandidateAttempt = 1
	request.SemanticCorrectionObservation = observation
	request.SemanticCorrectionDirective = &directive
	after, err := runtimeProviderRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	for n := 0; n <= 3; n++ {
		if before.PromptContext.Sections[n].Content != after.PromptContext.Sections[n].Content {
			t.Fatalf("same-contract correction changed C%d", n)
		}
	}
	if before.ContextBudget.CachePlan.StablePrefixHash != after.ContextBudget.CachePlan.StablePrefixHash || before.ContextBudget.CachePlan.RenderedStablePrefixHash != after.ContextBudget.CachePlan.RenderedStablePrefixHash || before.PromptContext.Sections[4].Content == after.PromptContext.Sections[4].Content {
		t.Fatal("correction failed to preserve exact prefix and update C4")
	}
}

func TestRuntimeSourceObligationProviderStablePrefixAcrossOwnerGeneration(t *testing.T) {
	request := runtimeSourceObligationProviderFixture(t)
	before, err := runtimeProviderRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	changed := request
	changed.SemanticManifest = request.SemanticManifest.Clone()
	changed.SemanticContext = request.SemanticContext.Clone()
	catalog := &changed.SemanticManifest.OwnerCatalogs[0]
	catalog.ObservedRevision = "new-owner-revision"
	catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(*catalog)
	changed.SemanticManifest.Hash = runtimecontract.StableRuntimeSemanticPlanManifestHash(changed.SemanticManifest)
	refreshSemanticProgressForTest(t, &changed)
	after, err := runtimeProviderRequest(changed)
	if err != nil {
		t.Fatal(err)
	}
	for n := 0; n <= 2; n++ {
		if before.PromptContext.Sections[n].Content != after.PromptContext.Sections[n].Content {
			t.Fatalf("owner generation changed C%d", n)
		}
	}
	if before.ContextBudget.CachePlan.StablePrefixHash != after.ContextBudget.CachePlan.StablePrefixHash || before.ContextBudget.CachePlan.RenderedStablePrefixHash != after.ContextBudget.CachePlan.RenderedStablePrefixHash || before.PromptContext.Sections[3].Content == after.PromptContext.Sections[3].Content {
		t.Fatal("owner generation failed to preserve stable prefix and refresh volatile authority")
	}
}
