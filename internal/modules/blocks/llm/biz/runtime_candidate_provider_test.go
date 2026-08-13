package llm

import (
	"context"
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"

	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeCandidateProviderClassifiesAuthenticationConfigurationAndTransientFailures(t *testing.T) {
	if got := runtimeCandidateProviderFailureClass(&ProviderHTTPStatusError{StatusCode: 401, Body: "authentication_failed"}, true); got != runtimecontract.RuntimeCandidateFailureClassProviderAuthentication {
		t.Fatalf("401 class = %s", got)
	}
	if got := runtimeCandidateProviderFailureClass(&ProviderHTTPStatusError{StatusCode: 404, Body: "model not found"}, true); got != runtimecontract.RuntimeCandidateFailureClassProviderConfiguration {
		t.Fatalf("404 class = %s", got)
	}
	if got := runtimeCandidateProviderFailureClass(&ProviderHTTPStatusError{StatusCode: 503, Body: "unavailable"}, true); got != runtimecontract.RuntimeCandidateFailureClassProviderTransient {
		t.Fatalf("503 class = %s", got)
	}
	if got := runtimeCandidateProviderFailureClass(context.DeadlineExceeded, true); got != runtimecontract.RuntimeCandidateFailureClassProviderTransient {
		t.Fatalf("deadline class = %s, want ordinary provider transport failure", got)
	}
}

func TestRuntimeCandidateProviderUsesOrchestratorPhysicalCallID(t *testing.T) {
	request := llmmodels.RuntimeCandidateRequest{DocumentKind: runtimecontract.RuntimeProviderDocumentOperations, RequestIdentity: runtimecontract.RuntimeCandidateRequestIdentity{LogicalRequestID: "candidate-request-1", Sequence: 3, PhysicalCallID: "physical-call-3"}}
	if got := newProviderRequestID(request); got != request.RequestIdentity.PhysicalCallID {
		t.Fatalf("provider request ID = %q, want persisted physical call ID %q", got, request.RequestIdentity.PhysicalCallID)
	}
}

func TestRuntimeSemanticPlanProviderUsesSingleCachePipelineAndSafeCorrection(t *testing.T) {
	base := runtimeSemanticProviderTestRequest(t)
	base.SemanticContext.PlanStep = runtimecontract.RuntimeSemanticPlanStepCoverageOrder
	base.SemanticContext.BatchID = ""
	base.SemanticContext.BatchCoverageWorkItemIDs = nil
	base.SemanticManifest.CoverageItems[0].PrerequisiteCoverageWorkItemIDs = []common.CoverageWorkItemID{"cwi-000"}
	prerequisite := runtimecontract.RuntimeCoverageItem{ID: "cwi-000", SliceID: "slice-1", SourceScope: "dodItem", SourceID: "dod-000", TargetText: "prerequisite"}
	base.SemanticManifest.CoverageItems = append(base.SemanticManifest.CoverageItems, prerequisite)
	base.SemanticContext.DefinitionSeed.CoverageItems = append(base.SemanticContext.DefinitionSeed.CoverageItems, prerequisite)
	base.SemanticContext.DefinitionSeed.Slices[0].CoverageWorkItemIDs = append(base.SemanticContext.DefinitionSeed.Slices[0].CoverageWorkItemIDs, "cwi-000")
	base.SemanticContext.DefinitionSeed.Hash = runtimecontract.StableRuntimeDefinitionSeedHash(base.SemanticContext.DefinitionSeed)
	base.SemanticManifest.DefinitionSeedHash = base.SemanticContext.DefinitionSeed.Hash
	base.SemanticManifest.Hash = runtimecontract.StableRuntimeSemanticPlanManifestHash(base.SemanticManifest)
	initial, err := runtimeProviderRequest(base)
	if err != nil {
		t.Fatal(err)
	}
	if initial.Kind != "runtime_semantic_plan" || initial.RawTextOutput || !strings.Contains(initial.Schema, `"semantic_plan"`) {
		t.Fatalf("semantic provider request = %+v", initial)
	}
	if initial.ContextBudget == nil || initial.ContextBudget.CachePlan == nil || initial.ContextBudget.CachePlan.StablePrefixHash != base.StablePrefixHash {
		t.Fatalf("semantic cache plan = %+v", initial.ContextBudget)
	}
	if got := runtimePromptSectionIDs(initial.PromptContext); !reflect.DeepEqual(got, []string{
		"C0.runtime-semantic-contract", "C1.goal-dod", "C2.capability-contract", "C3.semantic-authority", "C4.semantic-tail", "C5.acceptance-slices",
	}) {
		t.Fatalf("semantic section order = %v", got)
	}

	rejection := &runtimecontract.RuntimeOriginalDiagnostic{
		SchemaVersion: runtimecontract.RuntimeOriginalDiagnosticSchemaCurrent,
		OwnerModuleID: runtimecontract.RuntimeModuleLLM, OriginalClass: "schema", OriginalCode: "missing_target",
		OriginalMessage: "secret raw provider message must not be rendered", Signature: "semantic/missing-target",
		TargetRefs: []string{"main.go"}, FieldViolations: []runtimecontract.RuntimeFieldViolation{{Path: "$.coverageOrder", Actual: "cwi-001 before cwi-000", Required: []string{"cwi-000 before cwi-001"}}},
		SemanticViolations: []runtimecontract.RuntimeSemanticContractViolation{{Kind: runtimecontract.RuntimeSemanticContractViolationDependencyOrder, Path: "$.coverageOrder", RequiredBeforeID: "cwi-000", RequiredAfterID: "cwi-001", ActualBeforePosition: 1, ActualAfterPosition: 0}},
	}
	directive := runtimecontract.RuntimeSemanticCorrectionDirective{
		SchemaVersion: runtimecontract.RuntimeSemanticCorrectionDirectiveSchemaCurrent,
		PlanStep:      runtimecontract.RuntimeSemanticPlanStepCoverageOrder, ManifestHash: base.SemanticManifest.Hash,
		DefinitionSeedHash: base.SemanticManifest.DefinitionSeedHash, StablePrefixHash: base.StablePrefixHash,
		SourceRejectionHash: runtimecontract.StableRuntimeSemanticPlanCorrectionHash(*rejection), Attempt: 1,
		Violations: append([]runtimecontract.RuntimeSemanticContractViolation(nil), rejection.SemanticViolations...),
	}
	directive.Hash = runtimecontract.StableRuntimeSemanticCorrectionDirectiveHash(directive)
	retryRequest := base
	retryRequest.CandidateAttempt = 1
	retryRequest.PreviousSemanticRejection = rejection
	retryRequest.SemanticCorrectionDirective = &directive
	retry, err := runtimeProviderRequest(retryRequest)
	if err != nil {
		t.Fatal(err)
	}
	for idx := 0; idx < 3; idx++ {
		if initial.PromptContext.Sections[idx].Content != retry.PromptContext.Sections[idx].Content {
			t.Fatalf("semantic stable section %d drifted between attempts", idx)
		}
	}
	if retry.ContextBudget.CachePlan.StablePrefixHash != initial.ContextBudget.CachePlan.StablePrefixHash {
		t.Fatal("semantic correction changed stable prefix")
	}
	tail := retry.PromptContext.Sections[4].Content
	if strings.Contains(tail, rejection.OriginalMessage) || !strings.Contains(tail, `"missing_target"`) || !strings.Contains(tail, `"$.coverageOrder"`) || !strings.Contains(tail, `"requiredBeforeId":"cwi-000"`) {
		t.Fatalf("semantic correction tail is not provider-safe: %s", tail)
	}
}

func TestRuntimeSemanticCorrectionBoundaryClonesNestedValues(t *testing.T) {
	rejection := &runtimecontract.RuntimeOriginalDiagnostic{
		SchemaVersion: runtimecontract.RuntimeOriginalDiagnosticSchemaCurrent,
		OwnerModuleID: runtimecontract.RuntimeModuleLLM, OriginalClass: "semantic", OriginalCode: "order",
		OriginalMessage: "order rejected", Signature: "semantic/order",
		FieldViolations: []runtimecontract.RuntimeFieldViolation{{Path: "$.coverageOrder", Required: []string{"cwi-000 before cwi-001"}}},
	}
	directive := &runtimecontract.RuntimeSemanticCorrectionDirective{Violations: []runtimecontract.RuntimeSemanticContractViolation{{RequiredBeforeID: "cwi-000"}}}
	clonedRejection := cloneRuntimeOriginalDiagnostic(rejection)
	clonedDirective := cloneRuntimeSemanticCorrectionDirective(directive)
	clonedRejection.FieldViolations[0].Required[0] = "changed"
	clonedDirective.Violations[0].RequiredBeforeID = "changed"
	if rejection.FieldViolations[0].Required[0] != "cwi-000 before cwi-001" || directive.Violations[0].RequiredBeforeID != "cwi-000" {
		t.Fatal("LLM owner boundary retained caller-owned nested storage")
	}
}

func TestRuntimeSemanticProofBatchProviderProjectionOmitsUnrelatedCoverage(t *testing.T) {
	request := runtimeSemanticProviderTestRequest(t)
	second := runtimecontract.RuntimeCoverageItem{ID: "cwi-002", SliceID: "slice-2", SourceScope: "dodItem", SourceID: "dod-002", TargetText: "unrelated second requirement", Order: 2}
	items := append(append([]runtimecontract.RuntimeCoverageItem(nil), request.SemanticContext.DefinitionSeed.CoverageItems...), second)
	slices := append(append([]runtimecontract.RuntimeAcceptanceSlice(nil), request.SemanticContext.DefinitionSeed.Slices...), runtimecontract.RuntimeAcceptanceSlice{ID: "slice-2", Title: "Second", Priority: 2, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-002"}})
	seed, err := runtimecontract.CompileRuntimeDefinitionSeed(request.SemanticContext.Definition, request.SemanticContext.Activation, items, slices)
	if err != nil {
		t.Fatal(err)
	}
	request.SemanticContext.DefinitionSeed = seed
	request.SemanticContext.PlanStep = runtimecontract.RuntimeSemanticPlanStepProofBatch
	request.SemanticContext.BatchID = "batch-1"
	request.SemanticContext.BatchSequence = 0
	request.SemanticContext.BatchCoverageWorkItemIDs = []common.CoverageWorkItemID{"cwi-001"}
	request.SemanticManifest.CoverageItems = items
	request.SemanticManifest.DefinitionSeedHash = seed.Hash
	request.SemanticManifest.Hash = runtimecontract.StableRuntimeSemanticPlanManifestHash(request.SemanticManifest)
	providerRequest, err := runtimeProviderRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	authority := providerRequest.PromptContext.Sections[3].Content
	sliceProjection := providerRequest.PromptContext.Sections[5].Content
	if strings.Contains(authority, second.TargetText) || strings.Contains(sliceProjection, "cwi-002") || strings.Contains(authority, `"CoverageItems"`) || strings.Contains(authority, `"Slices"`) {
		t.Fatalf("proof batch leaked unrelated/full definition context: authority=%s slices=%s", authority, sliceProjection)
	}
	if !strings.Contains(authority, `"batchId":"batch-1"`) || !strings.Contains(authority, `"batchSequence":0`) || !strings.Contains(authority, `"coverageItems":[{"ID":"cwi-001"`) || !strings.Contains(providerRequest.Schema, `"planStep":{"const":"proof_batch"}`) {
		t.Fatalf("proof batch lost current authority: schema=%s authority=%s", providerRequest.Schema, authority)
	}
	if strings.Contains(providerRequest.Schema, `"coverageOrder"`) || !strings.Contains(authority, "Do not return coverageOrder") {
		t.Fatalf("proof batch repeated SM-3 coverage order: schema=%s authority=%s", providerRequest.Schema, authority)
	}
}

func TestRuntimeSemanticPlanProviderRejectsMixedDocumentFields(t *testing.T) {
	request := runtimeSemanticProviderTestRequest(t)
	request.DefinitionContext = request.SemanticContext.Definition
	if _, err := runtimeProviderRequest(request); err == nil || !strings.Contains(err.Error(), "must not carry operations fields") {
		t.Fatalf("mixed semantic request error = %v", err)
	}
	operations := llmmodels.RuntimeCandidateRequest{DocumentKind: runtimecontract.RuntimeProviderDocumentOperations, SemanticManifest: request.SemanticManifest}
	if _, err := runtimeProviderRequest(operations); err == nil || !strings.Contains(err.Error(), "must not carry semantic plan fields") {
		t.Fatalf("mixed operations request error = %v", err)
	}
}

func runtimeSemanticProviderTestRequest(t *testing.T) llmmodels.RuntimeCandidateRequest {
	t.Helper()
	definition := runtimecontract.RuntimeDefinitionContext{
		Revision: "definition-1", GoalID: "goal-1", GoalDefinition: "deliver a bounded artifact",
		DoDID: "dod-1", DoDDefinition: "the artifact works", CapabilityContract: []string{runtimecontract.StepKindFileWrite},
	}
	target := runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "model-1"}
	activation := runtimecontract.RuntimeActivationContext{
		ProviderSelection: runtimecontract.ProviderInferenceSelection{Target: target},
		ExecutionContract: runtimecontract.RuntimeExecutionContract{OperationContractRevision: runtimecontract.RuntimeOperationContractRevision()},
	}
	items := []runtimecontract.RuntimeCoverageItem{{ID: "cwi-001", SliceID: "slice-1", SourceScope: "dodItem", SourceID: "dod-001", TargetText: "the artifact works", Order: 1}}
	slices := []runtimecontract.RuntimeAcceptanceSlice{{ID: "slice-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-001"}}}
	seed, err := runtimecontract.CompileRuntimeDefinitionSeed(definition, activation, items, slices)
	if err != nil {
		t.Fatal(err)
	}
	catalog := runtimecontract.RuntimeOperationAdmissibility{PrerequisiteID: "semantic-file", OwnerModuleID: runtimecontract.RuntimeModuleFile, OperationKinds: []string{runtimecontract.StepKindFileWrite}, TargetRefs: []string{"main.go"}, SelectorRefs: []string{"workspace-relative-new-file-target"}, ConstraintRefs: []string{"workspace-relative-path"}, ObservedRevision: "file-revision-1"}
	catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	manifest := runtimecontract.RuntimeSemanticPlanManifest{
		SchemaVersion: runtimecontract.RuntimeSemanticPlanManifestSchemaCurrent,
		WorkspaceID:   "ws-1", RunID: "run-1", DefinitionRevision: definition.Revision, DefinitionSeedHash: seed.Hash,
		OperationContractRevision: runtimecontract.RuntimeOperationContractRevision(), CoverageItems: items,
		AllowedOwnerModuleIDs: []string{runtimecontract.RuntimeModuleFile}, AllowedOperationKinds: []string{runtimecontract.StepKindFileWrite},
		AllowedTargetPolicies: []runtimecontract.RuntimeOperationTargetPolicy{runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget},
		OwnerCatalogs:         []runtimecontract.RuntimeOperationAdmissibility{catalog},
	}
	manifest.Hash = runtimecontract.StableRuntimeSemanticPlanManifestHash(manifest)
	ceiling := &runtimecontract.ProviderCeiling{
		Selection: runtimecontract.ProviderInferenceSelection{Target: target}, ContextWindowTokens: 8192, ProviderMaxOutputTokens: 4096, EffectiveMaxOutputTokens: 2048,
		MaxInputTokens: 4096, MaxBytes: 128 * 1024, CapabilityRevision: "cap-1", ConfigurationRevision: "config-1",
	}
	context := runtimecontract.RuntimeSemanticPlanRequestContext{
		WorkspaceID: "ws-1", RunID: "run-1", Definition: definition, DefinitionSeed: seed, Activation: activation, ProviderCeiling: ceiling,
		PlanStep: runtimecontract.RuntimeSemanticPlanStepProofBatch, BatchID: "batch-1", BatchSequence: 0,
		BatchCoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-001"},
	}
	if err := context.Validate(manifest); err != nil {
		t.Fatal(err)
	}
	return llmmodels.RuntimeCandidateRequest{
		DocumentKind:     runtimecontract.RuntimeProviderDocumentSemanticPlan,
		RequestIdentity:  runtimecontract.RuntimeCandidateRequestIdentity{LogicalRequestID: "semantic-request-1", Sequence: 1, PhysicalCallID: "semantic-call-1"},
		StablePrefixHash: runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(definition, activation.ExecutionContract),
		SemanticManifest: manifest, SemanticContext: context,
	}
}

func runtimePromptSectionIDs(prompt *llmmodels.PromptContext) []string {
	if prompt == nil {
		return nil
	}
	ret := make([]string, 0, len(prompt.Sections))
	for _, section := range prompt.Sections {
		ret = append(ret, section.ID)
	}
	return ret
}

func TestRuntimeProviderRequestRejectsUnsealedManifestBeforeProviderConfiguration(t *testing.T) {
	_, err := runtimeProviderRequest(llmmodels.RuntimeCandidateRequest{DocumentKind: runtimecontract.RuntimeProviderDocumentOperations})
	if err == nil || !strings.Contains(err.Error(), "runtime candidate contract manifest") {
		t.Fatalf("provider request error = %v, want manifest preflight failure", err)
	}
}

func runtimeCandidateManifestForRequest(t *testing.T, decision runtimecontract.RuntimeDecision, context runtimecontract.RuntimeCandidateContext) runtimecontract.RuntimeCandidateContractManifest {
	t.Helper()
	decision = runtimeCandidateDecisionForSchemaTest(decision)
	manifest, err := runtimecontract.BuildRuntimeCandidateContractManifest(decision, runtimecontract.RuntimeCandidateManifestOwnerFactsFromResources(context.Resources))
	if err != nil {
		t.Fatal(err)
	}
	return manifest
}

// runtimeCandidateManifestForSchema is deliberately test-only. Production
// renderers receive the manifest carried by the sealed EventHub request and
// never compile one from a decision. Unit tests build the same canonical value
// explicitly so no legacy decision-to-schema fallback survives in production.
func runtimeCandidateManifestForSchema(t *testing.T, decision runtimecontract.RuntimeDecision, selection *runtimecontract.RuntimeTargetSelection) runtimecontract.RuntimeCandidateContractManifest {
	t.Helper()
	decision = runtimeCandidateDecisionForSchemaTest(decision)
	manifest, err := runtimecontract.BuildRuntimeCandidateContractManifest(decision, runtimecontract.RuntimeCandidateManifestOwnerFacts{TargetSelection: selection})
	if err != nil {
		t.Fatal(err)
	}
	return manifest
}

// runtimeCandidateDecisionForSchemaTest makes test-only schema fixtures obey
// the same C5 invariant as production authority: a state-changing or
// validation convergence state always carries sealed coverage and effect
// constraints. Production code never fills these fields at render time.
func runtimeCandidateDecisionForSchemaTest(decision runtimecontract.RuntimeDecision) runtimecontract.RuntimeDecision {
	contract := decision.RequiredOutput.Normalized()
	switch decision.State {
	case runtimecontract.ConvergenceStateNeedStateChange, runtimecontract.ConvergenceStateNeedValidation:
		if len(contract.CoverageWorkItemIDs) == 0 {
			contract.CoverageWorkItemIDs = []common.CoverageWorkItemID{"cwi-schema"}
		}
		policy, ok := runtimecontract.RuntimeConvergenceEffectPolicyFor(decision.State)
		if ok {
			if len(contract.AcceptedEffectScopes) == 0 {
				contract.AcceptedEffectScopes = append([]runtimecontract.RuntimeEffectScope(nil), policy.AllowedOperationEffects...)
			}
			if len(contract.RequiredEffectScopes) == 0 {
				contract.RequiredEffectScopes = append([]runtimecontract.RuntimeEffectScope(nil), policy.AllowedOperationEffects...)
			}
		}
	case runtimecontract.ConvergenceStateNeedAssessment:
		if len(contract.CoverageWorkItemIDs) == 0 {
			contract.CoverageWorkItemIDs = []common.CoverageWorkItemID{"cwi-schema"}
		}
	}
	decision.RequiredOutput = contract
	return decision
}

func runtimeCandidateSchemaForDecision(t *testing.T, decision runtimecontract.RuntimeDecision, selection *runtimecontract.RuntimeTargetSelection) string {
	t.Helper()
	return runtimeCandidateSchemaForManifest(runtimeCandidateManifestForSchema(t, decision, selection))
}

func runtimeCandidateOutputContractForDecision(t *testing.T, decision runtimecontract.RuntimeDecision, selection *runtimecontract.RuntimeTargetSelection) runtimeCandidateOutputContractProjection {
	t.Helper()
	return runtimeCandidateOutputContractForManifest(runtimeCandidateManifestForSchema(t, decision, selection))
}

func runtimeCandidateContextManifest(action runtimecontract.ConvergenceState, kinds ...string) runtimecontract.RuntimeCandidateContractManifest {
	operations := make([]runtimecontract.RuntimeCandidateOperationManifest, 0, len(kinds))
	for _, kind := range kinds {
		operations = append(operations, runtimecontract.RuntimeCandidateOperationManifest{Kind: kind})
	}
	policy, _ := runtimecontract.RuntimeConvergenceEffectPolicyFor(action)
	return runtimecontract.RuntimeCandidateContractManifest{State: action, EffectPolicy: policy, Operations: operations}
}

func TestRuntimeProviderRequestPreservesStablePrefixAndTypedContext(t *testing.T) {
	facts := runtimecontract.RuntimeFacts{
		WorkspaceID: "ws-1", RunID: "run-1", DefinitionRevision: "definition-1", EvidenceRevision: runtimecontract.InitialRuntimeEvidenceRevision,
		Focus:    runtimecontract.CoverageFocus{ID: "focus-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}},
		Coverage: []runtimecontract.RuntimeCoverageItemFact{{ID: "cwi-1", State: runtimecontract.RuntimeCoverageItemStateOpen}},
		Resources: runtimecontract.RuntimeResourceFacts{ProviderCeiling: &runtimecontract.ProviderCeiling{
			Selection:                runtimecontract.ProviderInferenceSelection{Target: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}},
			ContextWindowTokens:      128000,
			ProviderMaxOutputTokens:  16384,
			EffectiveMaxOutputTokens: 4096,
			MaxInputTokens:           92928,
			MaxBytes:                 96 * 1024,
			CapabilityRevision:       "capability-1",
			ConfigurationRevision:    "config-1",
		}, TargetSnapshots: []runtimecontract.TargetSnapshot{{
			TargetRef: "main.go", Content: "package main", ContentHash: "hash-1", ContentState: runtimecontract.SnapshotContentStateComplete,
		}}, CapabilityStates: []runtimecontract.RuntimeCapabilityStateSnapshot{{
			RequirementID: "capability_state:git", SnapshotID: "capability-state-git", AcquisitionKey: "capability-state/ws-1/run-1/capability_state:git/0", QueryReceiptRef: "capability-state-receipt/ws-1/run-1/capability_state:git/0", OwnerModuleID: runtimecontract.RuntimeModuleGit, CapabilityStateKind: "repository_state", SubjectRef: "workspace_deliverable",
			ObservedRevision: "repo-absent", ContentHash: "repo-state-hash", Disposition: runtimecontract.RuntimeCapabilityStateSequenceRequired,
			RequiredOperationSequence: []runtimecontract.RuntimeRequiredOperationVariant{{RequirementID: "repo-init", Kind: runtimecontract.StepKindGitWrite, VariantID: "init"}},
		}}},
		Activation: runtimecontract.RuntimeActivationContext{
			ProviderSelection: runtimecontract.ProviderInferenceSelection{Target: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}},
			DialogLanguage:    "zh-CN",
			DefaultSoul:       &runtimecontract.RuntimeSoulContext{ID: "soul-default", Instructions: "default-only-soul" + strings.Repeat("d", 22*1024)},
			StageSoulBindings: []runtimecontract.RuntimeStageSoulBinding{
				{Stage: "execution", Soul: runtimecontract.RuntimeSoulContext{ID: "soul-execution", Instructions: "execution-only-soul" + strings.Repeat("e", 22*1024)}},
				{Stage: "validation", Soul: runtimecontract.RuntimeSoulContext{ID: "soul-validation", Instructions: "validation-only-soul" + strings.Repeat("v", 13*1024)}},
			},
			Skills: []runtimecontract.RuntimeSkillContext{{ID: "skill-1", Instruction: "typed skill instructions"}},
			EvidencePolicies: []runtimecontract.RuntimeEvidencePolicy{{
				ID: "go-test-observable", AppliesTo: runtimecontract.StepKindShellCommand, CommandName: "go", RequiredArgs: []string{"test", "-v"},
			}},
			Continuation: runtimecontract.RuntimeContinuationContext{},
			ExecutionContract: runtimecontract.RuntimeExecutionContract{
				StateChangePolicy:       runtimecontract.RuntimeStateChangeMustProduce,
				CapabilityUsagePolicies: []runtimecontract.RuntimeCapabilityUsagePolicy{{ID: "usage-1", PreferredTools: []string{"lsp_diagnostics"}}},
				ProfileContracts: []runtimecontract.RuntimeProfileContract{{
					ID:                      "software-dev",
					AcceptanceDimensions:    []runtimecontract.RuntimeAcceptanceDimension{{ID: "runtime_entrypoint", Required: true}},
					DeliverableExpectations: []runtimecontract.RuntimeDeliverableExpectation{{ID: "version_control_snapshot", Required: true, EvidenceRole: "supplemental_evidence"}},
					EvidenceRequirements:    []runtimecontract.RuntimeProfileEvidenceRequirement{{ID: "version_control_snapshot", Scope: "deliverable", AcceptedStepKinds: []string{"git/write"}, AcceptedOperationVariants: []runtimecontract.RuntimeOperationVariantRef{{Kind: "git/write", VariantID: "commit"}}}},
				}},
				TemplateStagePolicy: &runtimecontract.RuntimeTemplateStagePolicy{SchemaVersion: runtimecontract.RuntimeTemplateStagePolicySchemaCurrent, Stages: []runtimecontract.RuntimeTemplateStage{{
					ID: "execution", SoulBinding: "execution", Entry: runtimecontract.RuntimeTemplateStageEntry{NextAction: "state_change"},
					RequiredOutputs: []runtimecontract.RuntimeTemplateStageRequiredOutput{{Kind: "state_change", EvidenceKinds: []string{"state_change"}}},
				}}},
			},
		},
	}
	selection := runtimecontract.RuntimeTargetSelection{
		PrerequisiteID: "target-selection-1", OwnerModuleID: runtimecontract.RuntimeModuleFile,
		TargetRole: "deliverable", SelectorRef: "file/workspace_deliverable",
		Disposition: runtimecontract.RuntimeTargetSelectionCreate, ProspectiveTargetRef: "initial-deliverable.txt", CandidateCount: 0, ObservedRevision: "workspace-empty-v1",
	}
	selection.SnapshotID = runtimecontract.StableRuntimeTargetSelectionSnapshotID(selection)
	facts.Resources.TargetSelection = &selection
	fileInventory := runtimecontract.RuntimeOperationAdmissibility{
		PrerequisiteID: "file-admissibility-1", OwnerModuleID: runtimecontract.RuntimeModuleFile,
		OperationKinds: []string{runtimecontract.StepKindFileWrite},
		OperationVariants: []runtimecontract.RuntimeOperationAdmissibilityVariant{{
			Kind: runtimecontract.StepKindFileWrite,
			AllowedTargetPolicies: []runtimecontract.RuntimeOperationTargetPolicy{
				runtimecontract.RuntimeOperationTargetPolicyExactTarget,
				runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope,
				runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget,
			},
			SelectorRefs:   []string{"workspace-relative-new-file-target"},
			ConstraintRefs: []string{"workspace-relative-path", "new-target-requires-owner-selector"},
		}},
		SelectorRefs:     []string{"workspace-relative-new-file-target"},
		ConstraintRefs:   []string{"workspace-relative-path", "new-target-requires-owner-selector"},
		ObservedRevision: "file-revision-1",
	}
	fileInventory.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(fileInventory)
	facts.Resources.OperationAdmissibilities = []runtimecontract.RuntimeOperationAdmissibility{fileInventory}
	bindRuntimeCandidateTestSelection(&facts, selection)
	bindRuntimeCandidateTestFocus(t, &facts, runtimecontract.ConvergenceStateNeedStateChange, []string{runtimecontract.StepKindFileWrite})
	decision, err := runtimecontract.SealRuntimeDecision(facts, runtimecontract.RuntimeDecision{
		State: runtimecontract.ConvergenceStateNeedStateChange, Focus: facts.Focus,
		RequiredOutput: runtimecontract.RequiredOutputContract{
			AcceptedKinds:        []string{runtimecontract.StepKindFileWrite},
			AcceptedEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable, runtimecontract.RuntimeEffectRemoteDurable},
			RequiredEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable, runtimecontract.RuntimeEffectRemoteDurable},
			CoverageWorkItemIDs:  facts.Focus.CoverageWorkItemIDs,
		},
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	definition := runtimecontract.RuntimeDefinitionContext{
		Revision: "definition-1", GoalID: "goal-1", GoalDefinition: "goal", DoDID: "dod-1", DoDDefinition: "dod", CapabilityContract: []string{runtimecontract.StepKindFileWrite},
		ChangeImpact: &runtimecontract.RuntimeDefinitionChangeImpact{
			Version: runtimecontract.RuntimeDefinitionChangeImpactVersionCurrent, Changed: true, Policy: "cumulative", RequiredPreExecutionAssessment: true,
			GoalChange:           runtimecontract.RuntimeDefinitionChangeSet{EffectiveDefinitionID: "goal-1"},
			DoDChange:            runtimecontract.RuntimeDefinitionChangeSet{EffectiveDefinitionID: "dod-1"},
			StrategyRequirements: []string{"identify additive constraints"},
		},
	}
	prefix := runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(definition, facts.Activation.ExecutionContract)
	rejectedPath := "main.go"
	rejectedShape := runtimecontract.RuntimeRejectedOperationShape{
		Operation:        runtimecontract.RuntimeOperation{ID: "rejected-edit", Kind: runtimecontract.StepKindFileEdit, TargetRefs: []string{rejectedPath}},
		FailureSignature: "rejected-shape-signature",
	}
	candidateContext := runtimecontract.RuntimeCandidateContextForDecision(facts, decision, []runtimecontract.RuntimeRejectedOperationShape{rejectedShape})
	if candidateContext.RequiredState != decision.State || !slices.Equal(candidateContext.RequiredEffectScopes, decision.RequiredOutput.RequiredEffectScopes) ||
		!slices.Equal(candidateContext.RequiredEvidenceKinds, decision.RequiredOutput.RequiredEvidenceKinds) {
		t.Fatalf("candidate context requirements = %+v, want sealed decision output %+v", candidateContext, decision.RequiredOutput)
	}
	manifest := runtimeCandidateManifestForRequest(t, decision, candidateContext)
	request, err := runtimeProviderRequest(llmmodels.RuntimeCandidateRequest{
		DocumentKind: runtimecontract.RuntimeProviderDocumentOperations,
		Decision:     decision, Transaction: decision.AuthorityTransaction(), StablePrefixHash: prefix,
		CandidateManifest: manifest, DefinitionContext: definition, CandidateContext: candidateContext,
	})
	if err != nil {
		t.Fatal(err)
	}
	if request.ContextBudget == nil || request.ContextBudget.CachePlan == nil || request.ContextBudget.CachePlan.StablePrefixHash != prefix ||
		request.ContextBudget.CachePlan.RenderedStablePrefixHash == "" || len(request.PromptContext.Sections) != 6 {
		t.Fatalf("provider request cache plan = %+v prompt=%+v", request.ContextBudget, request.PromptContext)
	}
	if request.ContextBudget.ActualBytes <= 0 || request.ContextBudget.ActualBytes >= 96*1024 {
		t.Fatalf("provider request actual bytes = %d, want current-stage projection below 96 KiB", request.ContextBudget.ActualBytes)
	}
	if request.PromptContext.Sections[3].Content == "" || request.PromptContext.Sections[4].Content == "" || request.Schema == "" {
		t.Fatalf("provider request lost dynamic context: %+v", request)
	}
	contractSection := request.PromptContext.Sections[0].Content
	for _, value := range []string{"one JSON object", "context.outputContract is the sole structured-output contract", "manifestHash", "sole canonical decision authority", "completion-audit information only"} {
		if !strings.Contains(contractSection, value) {
			t.Fatalf("provider request stable contract lost %q: %s", value, contractSection)
		}
	}
	for _, forbidden := range []string{`"operations"`, `"focusId"`, `"externalDependency"`, "copy that exact value into its top-level targetRefs"} {
		if strings.Contains(contractSection, forbidden) {
			t.Fatalf("provider request stable contract retained manifest-specific rule %q: %s", forbidden, contractSection)
		}
	}
	dynamicSection := request.PromptContext.Sections[3].Content
	executionTail := request.PromptContext.Sections[4].Content
	largeRefs := request.PromptContext.Sections[5].Content
	if request.PromptContext.Sections[3].ID != "C3.runtime-authority" || request.PromptContext.Sections[4].ID != "C4.execution-tail" || request.PromptContext.Sections[5].ID != "C5.large-refs" {
		t.Fatalf("runtime prompt sections = %+v", request.PromptContext.Sections)
	}
	for _, value := range []string{
		`"outputContract"`, `"manifestHash"`, `"structuredOutputInstruction"`, `"$id":"workorch.runtime_candidate.state_change"`, `"forbiddenTopLevelFields"`,
		`"operationContracts":[{"kind":"file/write"`, `"name":"content"`, `"targetCardinality":"one"`,
		`"forbiddenParameters":["input","output","payload","path","targetRef","targetRefs"]`,
		`"capabilityStates"`, `"repo-state-hash"`, `"targetSelectionInstruction"`, "empty workspace creation scope", `sealed prospective target \"initial-deliverable.txt\"`, `"evidencePolicyIds":["go-test-observable"]`,
	} {
		if !strings.Contains(dynamicSection, value) {
			t.Fatalf("provider request dynamic output contract lost %q: %s", value, dynamicSection)
		}
	}
	for _, forbidden := range []string{`"RequiredArgs"`, `"CommandName"`, `"RecipeArgs"`} {
		if strings.Contains(dynamicSection, forbidden) {
			t.Fatalf("provider request leaked Shell command-construction policy %q: %s", forbidden, dynamicSection)
		}
	}
	if !strings.Contains(request.PromptContext.Sections[1].Content, "identify additive constraints") || !strings.Contains(request.PromptContext.Sections[1].Content, "cumulative") {
		t.Fatalf("provider request lost typed definition change impact: %s", request.PromptContext.Sections[1].Content)
	}
	if !strings.Contains(request.PromptContext.Sections[3].Content, "typed skill instructions") || !strings.Contains(request.PromptContext.Sections[3].Content, "zh-CN") ||
		!strings.Contains(request.PromptContext.Sections[3].Content, "execution-only-soul") || strings.Contains(request.PromptContext.Sections[3].Content, "validation-only-soul") ||
		strings.Contains(request.PromptContext.Sections[3].Content, "default-only-soul") || strings.Contains(request.PromptContext.Sections[3].Content, "must_produce_state_change") ||
		strings.Contains(request.PromptContext.Sections[3].Content, "lsp_diagnostics") {
		t.Fatalf("provider request lost typed activation context: %s", request.PromptContext.Sections[3].Content)
	}
	for _, forbidden := range []string{"signature-old", "target-old", "RecoveryAction", `"continuation"`} {
		if strings.Contains(executionTail, forbidden) {
			t.Fatalf("C4 leaked continuation/recovery detail %q: %s", forbidden, executionTail)
		}
	}
	capabilitySection := request.PromptContext.Sections[2].Content
	if strings.Contains(capabilitySection, "signature-old") || strings.Contains(capabilitySection, "target-old") {
		t.Fatalf("volatile continuation leaked into C2 stable prefix: %s", capabilitySection)
	}
	for _, value := range []string{"must_produce_state_change", "lsp_diagnostics", "runtime_entrypoint", "version_control_snapshot", "git/write", "commit", "workorch.template_stage_policy", "state_change"} {
		if !strings.Contains(capabilitySection, value) {
			t.Fatalf("provider request lost typed profile contract %q: %s", value, capabilitySection)
		}
	}
	for idx := 0; idx < 3; idx++ {
		if strings.Contains(request.PromptContext.Sections[idx].Content, "rejected-shape-signature") {
			t.Fatalf("rejected shape leaked into stable C%d section: %s", idx, request.PromptContext.Sections[idx].Content)
		}
	}
	if !strings.Contains(executionTail, "rejectedOperationShapes") || !strings.Contains(executionTail, "rejected-shape-signature") {
		t.Fatalf("rejected shape is not rendered in volatile execution tail: %s", executionTail)
	}
	if strings.Contains(dynamicSection, "rejected-shape-signature") || strings.Contains(dynamicSection, "signature-old") || strings.Contains(dynamicSection, `"continuation":{}`) || strings.Contains(largeRefs, "package main") {
		t.Fatalf("volatile or large content leaked into C3/C5: c3=%s c5=%s", dynamicSection, largeRefs)
	}
	for _, testCase := range []struct {
		name    string
		failure runtimecontract.RuntimeCandidateFailure
		want    []string
	}{
		{
			name: "schema diagnostic",
			failure: runtimecontract.RuntimeCandidateFailure{
				Class: runtimecontract.RuntimeCandidateFailureClassSchema, Code: "candidate_schema_invalid", Signature: "candidate-schema-invalid",
				Reason: "raw decoder text must not be rendered: json: unknown field candidate",
				Diagnostic: &runtimecontract.RuntimeCandidateFailureDiagnostic{
					Path: "$.operations[0].parameters[0].name", Actual: "summary", Allowed: []string{"command", "args"}, Forbidden: []string{"summary"},
				},
			},
			want: []string{`"class":"schema"`, `"code":"candidate_schema_invalid"`, `"path":"$.operations[0].parameters[0].name"`, `"actual":"summary"`},
		},
		{
			name: "semantic manifest diagnostic",
			failure: runtimecontract.RuntimeCandidateFailure{
				Class: runtimecontract.RuntimeCandidateFailureClassContract, Code: "candidate_contract_invalid", Signature: "candidate-contract-invalid",
				Reason: "raw semantic cause must not be rendered",
				Diagnostic: &runtimecontract.RuntimeCandidateFailureDiagnostic{
					OperationID: "write", Path: "$.operations[0].targetRefs", Actual: "other.go", Required: []string{"main.go"},
				},
			},
			want: []string{`"class":"contract"`, `"code":"candidate_contract_invalid"`, `"operationId":"write"`, `"path":"$.operations[0].targetRefs"`},
		},
		{
			name: "grounded target rejection",
			failure: runtimecontract.RuntimeCandidateFailure{
				Class: runtimecontract.RuntimeCandidateFailureClassContract, Code: "candidate_target_outside_grounding", Signature: "candidate-target-outside-grounding",
				Reason: "raw owner explanation must not be rendered", TargetRefs: []string{"main.go"},
			},
			want: []string{`"code":"candidate_target_outside_grounding"`, `"targetRefs":["main.go"]`},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if diagnostic := testCase.failure.Diagnostic; diagnostic != nil {
				if diagnostic.ManifestHash == "" {
					diagnostic.ManifestHash = manifest.Hash
				}
				if diagnostic.RejectedShapeHash == "" {
					diagnostic.RejectedShapeHash = testCase.failure.Signature
				}
			}
			retryRequest, err := runtimeProviderRequest(llmmodels.RuntimeCandidateRequest{
				DocumentKind: runtimecontract.RuntimeProviderDocumentOperations,
				Decision:     decision, Transaction: decision.AuthorityTransaction(), CandidateAttempt: 1, PreviousFailure: &testCase.failure, StablePrefixHash: prefix,
				CandidateManifest: manifest, DefinitionContext: definition, CandidateContext: candidateContext,
			})
			if err != nil {
				t.Fatal(err)
			}
			retrySection := retryRequest.PromptContext.Sections[4].Content
			for _, value := range append([]string{`"candidateAttempt":1`, `"previousFailure"`, "context.outputContract.structuredOutputInstruction", "sealed manifest hash"}, testCase.want...) {
				if !strings.Contains(retrySection, value) {
					t.Fatalf("provider retry lost correction %q: %s", value, retrySection)
				}
			}
			if strings.Contains(retrySection, testCase.failure.Reason) {
				t.Fatalf("provider retry leaked raw prior failure into C4: %s", retrySection)
			}
			var tail struct {
				Retry struct {
					PreviousFailure struct {
						Diagnostic *runtimecontract.RuntimeCandidateFailureDiagnostic `json:"diagnostic"`
					} `json:"previousFailure"`
				} `json:"retry"`
			}
			if err := json.Unmarshal([]byte(retrySection), &tail); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(retrySection, `"reason"`) {
				t.Fatalf("provider retry serialized a previous-failure reason: %s", retrySection)
			}
			if testCase.failure.Diagnostic != nil {
				diagnostic := tail.Retry.PreviousFailure.Diagnostic
				want := testCase.failure.Diagnostic.Normalized()
				if diagnostic == nil || !reflect.DeepEqual(diagnostic.Normalized(), want) {
					t.Fatalf("provider retry changed typed correction delta: got=%+v want=%+v", diagnostic, want)
				}
			}
			for idx := 0; idx <= 3; idx++ {
				if retryRequest.PromptContext.Sections[idx].Content != request.PromptContext.Sections[idx].Content {
					t.Fatalf("same-transaction retry changed C%d", idx)
				}
			}
			if retryRequest.PromptContext.Sections[4].Content == request.PromptContext.Sections[4].Content || retryRequest.ContextBudget.CachePlan.StablePrefixHash != request.ContextBudget.CachePlan.StablePrefixHash || retryRequest.PromptContext.Sections[0].Content != contractSection {
				t.Fatalf("retry changed stable provider contract: initial=%+v retry=%+v", request.ContextBudget.CachePlan, retryRequest.ContextBudget.CachePlan)
			}
		})
	}
	freshContext := runtimecontract.RuntimeCandidateContextForDecision(facts, decision, nil)
	freshObservation := runtimecontract.RuntimePrerequisiteObservation{
		PrerequisiteID: "admissibility-file", Kind: runtimecontract.PrerequisiteKindOperationAdmissibility, OwnerModuleID: runtimecontract.RuntimeModuleFile, ContentHash: "admissibility-v2",
		Dependencies: []runtimecontract.RuntimeFreshnessDependency{{Domain: runtimecontract.RuntimeResourceWorkspaceTree, OwnerModuleID: runtimecontract.RuntimeModuleFile, SubjectRef: "workspace_deliverable", Revision: "tree-v2"}},
	}
	freshObservation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(freshObservation)
	freshContext.Resources.PrerequisiteObservations = []runtimecontract.RuntimePrerequisiteObservation{freshObservation}
	freshRequest, err := runtimeProviderRequest(llmmodels.RuntimeCandidateRequest{
		DocumentKind: runtimecontract.RuntimeProviderDocumentOperations,
		Decision:     decision, Transaction: decision.AuthorityTransaction(), StablePrefixHash: prefix,
		CandidateManifest: runtimeCandidateManifestForRequest(t, decision, freshContext), DefinitionContext: definition, CandidateContext: freshContext,
	})
	if err != nil {
		t.Fatal(err)
	}
	for idx := 0; idx < 3; idx++ {
		if freshRequest.PromptContext.Sections[idx].Content != request.PromptContext.Sections[idx].Content {
			t.Fatalf("freshness metadata drifted stable C%d section", idx)
		}
	}
	if !strings.Contains(freshRequest.PromptContext.Sections[3].Content, `"prerequisiteObservations"`) || !strings.Contains(freshRequest.PromptContext.Sections[3].Content, "tree-v2") {
		t.Fatalf("freshness metadata was not rendered in volatile C3: %s", freshRequest.PromptContext.Sections[3].Content)
	}
	limitedContext := runtimecontract.RuntimeCandidateContextForDecision(facts, decision, nil)
	limitedContext.Resources.ProviderCeiling = &runtimecontract.ProviderCeiling{
		Selection:                runtimecontract.ProviderInferenceSelection{Target: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}},
		ContextWindowTokens:      128000,
		ProviderMaxOutputTokens:  16384,
		EffectiveMaxOutputTokens: 4096,
		MaxInputTokens:           92928,
		MaxBytes:                 1,
		CapabilityRevision:       "capability-1",
		ConfigurationRevision:    "config-1",
	}
	if _, err := runtimeProviderRequest(llmmodels.RuntimeCandidateRequest{
		DocumentKind: runtimecontract.RuntimeProviderDocumentOperations,
		Decision:     decision, Transaction: decision.AuthorityTransaction(), StablePrefixHash: prefix,
		CandidateManifest: runtimeCandidateManifestForRequest(t, decision, limitedContext), DefinitionContext: definition, CandidateContext: limitedContext,
	}); err == nil || !strings.Contains(err.Error(), "provider ceiling") {
		t.Fatalf("runtimeProviderRequest ceiling err = %v, want typed provider ceiling failure", err)
	}
}

func TestValidateRuntimeCandidateContextIntegrityRejectsProtectedContextLoss(t *testing.T) {
	definition := runtimecontract.RuntimeDefinitionContext{Revision: "definition-1", GoalID: "goal-1", DoDID: "dod-1"}
	context := runtimecontract.RuntimeCandidateContext{
		WorkspaceID: "ws-1", RunID: "run-1", DefinitionRevision: "definition-1", EvidenceRevision: runtimecontract.InitialRuntimeEvidenceRevision,
		Focus:         runtimecontract.CoverageFocus{ID: "focus-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}},
		RequiredState: runtimecontract.ConvergenceStateNeedStateChange,
	}
	budget := &llmmodels.ProviderContextBudget{RedactionApplied: true, RedactedPaths: []string{"context.resources.targetSnapshots"}}
	prompt := &llmmodels.PromptContext{Sections: []llmmodels.PromptSection{
		{ID: "C0.runtime", Content: "contract"}, {ID: "C1.goal", Content: "definition"}, {ID: "C2.capability", Content: "capability"},
		{ID: "C3.authority", Content: "authority"}, {ID: "C4.execution", Content: "tail"},
	}}
	err := validateRuntimeCandidateContextIntegrity(definition, context, 0, nil, budget, prompt)
	if err == nil || !strings.Contains(err.Error(), "P0.contract") || !strings.Contains(err.Error(), "P4.current_snapshots") || !strings.Contains(err.Error(), "protected_context_redacted") {
		t.Fatalf("integrity error = %v", err)
	}
	if len(budget.IntegrityWarnings) < 3 {
		t.Fatalf("integrity warnings = %v", budget.IntegrityWarnings)
	}
}

func TestRuntimeCandidateRetryExplainsOperationShapeReplayWithoutChangingStablePrefix(t *testing.T) {
	failure := &runtimecontract.RuntimeCandidateFailure{
		Class:  runtimecontract.RuntimeCandidateFailureClassContract,
		Code:   "candidate_replays_rejected_shape",
		Reason: "runtime candidate replays active rejected operation shape(s)",
	}
	retry := runtimeCandidateRetry(1, failure)
	if retry == nil {
		t.Fatal("retry projection is nil")
	}
	for _, want := range []string{
		"context.rejectedOperationShapes", "context.noProgressOperationShapes", "materially different operation shape",
		"changes its declared durable targets", "Changing only an operation ID",
	} {
		if !strings.Contains(retry.Instruction, want) {
			t.Fatalf("retry instruction lost %q: %s", want, retry.Instruction)
		}
	}
	if retry := runtimeCandidateRetry(1, &runtimecontract.RuntimeCandidateFailure{
		Class: runtimecontract.RuntimeCandidateFailureClassContract, Code: "candidate_admission_invalid",
	}); retry == nil || strings.Contains(retry.Instruction, "materially different operation shape") {
		t.Fatalf("non-replay retry instruction = %+v, want no replay-specific correction", retry)
	}
}

func TestRuntimeCandidateExecutionTailOnlyRendersProviderSafeFacts(t *testing.T) {
	secretParameter := "raw-operation-payload-must-not-reach-c4"
	context := runtimecontract.RuntimeCandidateContext{
		Failure: &runtimecontract.FailureFact{
			Class: runtimecontract.FailureClassExecutionFailure, Code: "owner_fact", Signature: "owner-signature",
			Message: "raw failure message must not reach c4", TargetRefs: []string{"main.go"},
			GovernanceDependency: &runtimecontract.ExternalDependency{Kind: "external", SubjectRef: "dep-1", Reason: "raw governance reason must not reach c4"},
		},
		Evidence: runtimecontract.EvidenceLedger{Accepted: []runtimecontract.EvidenceFact{{
			ID: "evidence-1", Kind: runtimecontract.EvidenceKindValidation, SubjectRef: "main.go", FailureCode: "validation_failed",
			FailureSignature: "evidence-signature", FailureMessage: "raw evidence failure must not reach c4", Summary: "raw evidence summary must not reach c4",
			EvidenceReason: "raw evidence reason must not reach c4", TargetRefs: []string{"main.go"},
		}}},
		Assessments: []runtimecontract.RuntimeAssessmentFact{{
			ID: "assessment-1", FocusID: "focus-1", RequirementID: "requirement-1", Summary: "raw assessment summary must not reach c4",
			RequirementText: "raw assessment requirement must not reach c4", Disposition: runtimecontract.AssessmentDispositionSatisfied,
		}},
		RejectedOperationShapes: []runtimecontract.RuntimeRejectedOperationShape{{
			Operation:   runtimecontract.RuntimeOperation{ID: "op-1", Kind: runtimecontract.StepKindFileWrite, TargetRefs: []string{"main.go"}, Parameters: []runtimecontract.OperationParameter{{Name: "content", String: &secretParameter}}},
			FailureCode: "rejected_shape", FailureSignature: "shape-signature", FailureMessage: "raw rejected-shape message must not reach c4",
		}},
		Activation: runtimecontract.RuntimeActivationContext{Continuation: runtimecontract.RuntimeContinuationContext{}},
	}
	tail := buildRuntimeCandidateExecutionTailProjection(context, 0, nil, runtimecontract.RuntimeCandidateContractManifest{})
	encoded, err := json.Marshal(tail)
	if err != nil {
		t.Fatal(err)
	}
	text := string(encoded)
	for _, forbidden := range []string{
		"raw failure message", "raw governance reason", "raw evidence failure", "raw evidence summary", "raw evidence reason",
		"raw assessment summary", "raw assessment requirement", "raw rejected-shape message", secretParameter,
		"raw-recovery-action", "raw continuation reason", `"repairHints"`, `"continuation"`,
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("C4 leaked non-provider-safe detail %q: %s", forbidden, text)
		}
	}
	for _, required := range []string{`"code":"owner_fact"`, `"failureCode":"validation_failed"`, `"failureSignature":"shape-signature"`, `"kind":"file/write"`} {
		if !strings.Contains(text, required) {
			t.Fatalf("C4 lost provider-safe fact %q: %s", required, text)
		}
	}
}

func TestRuntimeTargetSelectionInstructionIsLimitedToFileScopes(t *testing.T) {
	selection := runtimecontract.RuntimeTargetSelection{
		PrerequisiteID: "target-selection-1", OwnerModuleID: runtimecontract.RuntimeModuleFile,
		TargetRole: "deliverable", SelectorRef: "file/workspace_deliverable",
		Disposition: runtimecontract.RuntimeTargetSelectionCreate, ProspectiveTargetRef: "initial-deliverable.txt", CandidateCount: 0, ObservedRevision: "workspace-empty-v1",
	}
	selection.SnapshotID = runtimecontract.StableRuntimeTargetSelectionSnapshotID(selection)
	got := runtimeTargetSelectionInstruction(&selection, runtimecontract.ConvergenceStateNeedStateChange)
	for _, required := range []string{"durable File-owner operation", `sealed prospective target "initial-deliverable.txt"`, "Git, Shell, MCP, or another owner"} {
		if !strings.Contains(got, required) {
			t.Fatalf("creation instruction lost %q: %s", required, got)
		}
	}
	if got := runtimeTargetSelectionInstruction(&selection, runtimecontract.ConvergenceStateNeedValidation); got != "" {
		t.Fatalf("validation must not receive creation instruction: %q", got)
	}
	selection.Disposition = runtimecontract.RuntimeTargetSelectionScope
	selection.TargetRefs = []string{"implementation.go", "support.go"}
	selection.CandidateTargetRefs = append([]string(nil), selection.TargetRefs...)
	selection.CandidateCount = len(selection.TargetRefs)
	selection.SnapshotID = runtimecontract.StableRuntimeTargetSelectionSnapshotID(selection)
	got = runtimeTargetSelectionInstruction(&selection, runtimecontract.ConvergenceStateNeedStateChange)
	for _, required := range []string{"bounded workspace feasibility scope, not a selected repair target", "Focus, FailureTarget, current failure, or RepairDirective", "may be a new file", "Do not use this scope to authorize Git, Shell, MCP"} {
		if !strings.Contains(got, required) {
			t.Fatalf("scope instruction lost %q: %s", required, got)
		}
	}
	selection.OwnerModuleID = runtimecontract.RuntimeModuleGit
	selection.SnapshotID = runtimecontract.StableRuntimeTargetSelectionSnapshotID(selection)
	if got := runtimeTargetSelectionInstruction(&selection, runtimecontract.ConvergenceStateNeedStateChange); got != "" {
		t.Fatalf("non-File selection must not receive creation instruction: %q", got)
	}
}

func TestRuntimeCandidateContextProjectionPreservesRepairDirectiveAuthority(t *testing.T) {
	directive := runtimecontract.RuntimeRepairDirective{
		ID: "repair-1", FocusID: "focus-1", Trigger: runtimecontract.RuntimeRepairTriggerRejectedValidation,
		RequiredState:         runtimecontract.ConvergenceStateNeedStateChange,
		RequiredEffectScopes:  []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable},
		RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange},
		AllowedOperationKinds: []string{runtimecontract.StepKindFileWrite},
		CoverageWorkItemIDs:   []common.CoverageWorkItemID{"cwi-1"}, TargetRefs: []string{"main.go"},
		CurrentFailure: &runtimecontract.FailureFact{
			Class: runtimecontract.FailureClassExecutionRejected, Message: "validation failed",
			ValidationDiagnostic: &runtimecontract.ValidationFailureDiagnostic{
				SchemaVersion: runtimecontract.ValidationFailureDiagnosticSchemaVersion,
				Class:         runtimecontract.ValidationDiagnosticBuildFailed, RecommendedEvidenceTargetType: runtimecontract.ValidationEvidenceTargetFailureSurface,
				TargetRefs: []string{"main.go"}, StderrPreview: "compile failed",
			},
		},
	}
	context := runtimecontract.RuntimeCandidateContext{
		WorkspaceID: "ws-1", RunID: "run-1", DefinitionRevision: "definition-1", EvidenceRevision: runtimecontract.InitialRuntimeEvidenceRevision,
		Focus:         runtimecontract.CoverageFocus{ID: "focus-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation}},
		RequiredState: directive.RequiredState, RequiredEffectScopes: directive.RequiredEffectScopes,
		RequiredEvidenceKinds: directive.RequiredEvidenceKinds, RepairDirective: &directive,
		AdmissibleOperationKinds: []string{runtimecontract.StepKindFileWrite},
	}
	projection := buildRuntimeCandidateContextProjection(context, runtimeCandidateContextManifest(runtimecontract.ConvergenceStateNeedStateChange, runtimecontract.StepKindFileWrite))
	if projection.RequiredState != runtimecontract.ConvergenceStateNeedStateChange || projection.RepairDirective == nil {
		t.Fatalf("projection lost repair authority: %+v", projection)
	}
	if len(projection.RequiredEffectScopes) != 1 || projection.RequiredEffectScopes[0] != runtimecontract.RuntimeEffectWorkspaceDurable {
		t.Fatalf("projection required effects = %v", projection.RequiredEffectScopes)
	}
	if len(projection.RequiredEvidenceKinds) != 1 || projection.RequiredEvidenceKinds[0] != runtimecontract.EvidenceKindStateChange {
		t.Fatalf("projection required evidence = %v", projection.RequiredEvidenceKinds)
	}
	if projection.RepairDirective == nil || projection.RepairDirective.FocusID != "focus-1" {
		t.Fatalf("projection repair directive = %+v", projection.RepairDirective)
	}
	encoded, err := json.Marshal(projection)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"ValidationDiagnostic", runtimecontract.ValidationDiagnosticBuildFailed, "compile failed", "main.go"} {
		if !strings.Contains(string(encoded), want) {
			t.Fatalf("C3 projection lost validation diagnostic %q: %s", want, encoded)
		}
	}
}

func TestRuntimeCandidateContextProjectionPreservesTargetGroundingInVolatileAuthority(t *testing.T) {
	context := runtimecontract.RuntimeCandidateContext{
		WorkspaceID: "ws-1", RunID: "run-1", DefinitionRevision: "definition-1", EvidenceRevision: "evidence-1",
		Focus: runtimecontract.CoverageFocus{
			ID: "focus-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange},
			TargetRole: "implementation", TargetRefs: []string{"implementation.go"}, TargetDiscoverySources: []string{runtimecontract.RuntimeValidationDiscoverySourceWorkspaceManifest}, RequireTargetGrounding: true,
		},
		RequiredState: runtimecontract.ConvergenceStateNeedStateChange,
		Resources: runtimecontract.RuntimeResourceFacts{
			FailureTarget: &runtimecontract.FailureTarget{Signature: "validation-failure", TargetRefs: []string{"implementation.go"}, TargetObservation: &runtimecontract.RuntimeFailureTargetObservation{
				OwnerModuleID: runtimecontract.RuntimeModuleMCP, SubjectRef: "server-a", TargetRole: "implementation", ObservedRevision: "mcp-server-v1", TargetRefs: []string{"implementation.go"},
			}},
			TargetSnapshots: []runtimecontract.TargetSnapshot{{TargetRef: "implementation.go", ContentHash: "hash-1", ContentState: runtimecontract.SnapshotContentStateComplete}, {TargetRef: "support.go", ContentHash: "hash-2", ContentState: runtimecontract.SnapshotContentStateComplete}},
			TargetSelection: &runtimecontract.RuntimeTargetSelection{PrerequisiteID: "selection-1", OwnerModuleID: runtimecontract.RuntimeModuleFile, TargetRole: "implementation", SelectorRef: "file/workspace_deliverable", Disposition: runtimecontract.RuntimeTargetSelectionScope, TargetRefs: []string{"implementation.go", "support.go"}, CandidateTargetRefs: []string{"implementation.go", "support.go"}, CandidateCount: 2, ObservedRevision: "manifest-1"},
		},
	}
	context.Resources.TargetSelection.SnapshotID = runtimecontract.StableRuntimeTargetSelectionSnapshotID(*context.Resources.TargetSelection)
	projection := buildRuntimeCandidateContextProjection(context, runtimeCandidateContextManifest(runtimecontract.ConvergenceStateNeedStateChange, runtimecontract.RuntimeBlockKindPatchApply))
	encoded, err := json.Marshal(projection)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{`"TargetRole":"implementation"`, `"TargetRefs":["implementation.go"]`, `"TargetDiscoverySources":["workspace_manifest"]`, `"RequireTargetGrounding":true`, `"failureTarget"`, `"TargetObservation"`, `"ownerModuleId":"blocks/mcp"`, `"observedRevision":"mcp-server-v1"`, `"targetSelection"`, `"scope"`, `"support.go"`} {
		if !strings.Contains(string(encoded), required) {
			t.Fatalf("volatile target-grounding context lost %s: %s", required, encoded)
		}
	}
	definition := runtimecontract.RuntimeDefinitionContext{Revision: "definition-1", GoalID: "goal-1", GoalDefinition: "goal", DoDID: "dod-1", DoDDefinition: "dod"}
	base := runtimecontract.RuntimeExecutionContract{ProfileContracts: []runtimecontract.RuntimeProfileContract{{ID: "profile"}}}
	before := runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(definition, base)
	context.Focus.TargetRefs = []string{"different.go"}
	after := runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(definition, base)
	if before != after {
		t.Fatalf("focus target grounding drifted stable C0-C2 prefix: %s != %s", before, after)
	}
}

func TestRuntimeCandidateSchemasAreActionSpecific(t *testing.T) {
	stateChange := runtimeCandidateSchema(runtimecontract.ConvergenceStateNeedStateChange)
	validation := runtimeCandidateSchema(runtimecontract.ConvergenceStateNeedValidation)
	if stateChange == validation {
		t.Fatal("state-change and validation candidate schemas must be distinct")
	}
	if !strings.Contains(stateChange, "workorch.runtime_candidate.state_change") || !strings.Contains(stateChange, "durable") {
		t.Fatalf("state-change schema = %s", stateChange)
	}
	if !strings.Contains(stateChange, "Every operation") || strings.Contains(stateChange, "At least one operation") {
		t.Fatalf("state-change schema does not render the canonical all-durable effect policy: %s", stateChange)
	}
	if !strings.Contains(validation, "workorch.runtime_candidate.validation") || !strings.Contains(validation, "non-durable") {
		t.Fatalf("validation schema = %s", validation)
	}
	catalog := runtimeCandidateContractCatalog()
	if strings.Contains(catalog, "produce_state_change or produce_validation") || strings.Contains(catalog, "produce only those missing evidence kinds") {
		t.Fatalf("candidate catalog retains conflicting action guidance: %s", catalog)
	}
	if !strings.Contains(catalog, "sole canonical decision authority") || !strings.Contains(catalog, "completion-audit information only") {
		t.Fatalf("candidate catalog lost repair authority rules: %s", catalog)
	}
	if !strings.Contains(catalog, "context.outputContract") || strings.Contains(catalog, "workorch.runtime_candidate.state_change") ||
		strings.Contains(catalog, "dependsOn") || strings.Contains(catalog, "targetRefs") {
		t.Fatalf("stable catalog retained a second operation contract: %s", catalog)
	}
}

func TestRuntimeCandidateSchemaOnlyExposesDependenciesForSealedDependentSequence(t *testing.T) {
	withoutDependency := runtimeCandidateSchemaForDecision(t, runtimecontract.RuntimeDecision{
		State:          runtimecontract.ConvergenceStateNeedStateChange,
		RequiredOutput: runtimecontract.RequiredOutputContract{AcceptedKinds: []string{runtimecontract.StepKindFileWrite}},
	}, nil)
	if runtimeOperationSchemaHasProperty(t, withoutDependency, "dependsOn") {
		t.Fatalf("ordinary candidate schema exposed an unnecessary dependency field: %s", withoutDependency)
	}

	withDependency := runtimeCandidateSchemaForDecision(t, runtimecontract.RuntimeDecision{
		State: runtimecontract.ConvergenceStateNeedStateChange,
		RequiredOutput: runtimecontract.RequiredOutputContract{
			AcceptedKinds: []string{runtimecontract.StepKindGitWrite},
			RequiredOperationSequence: []runtimecontract.RuntimeRequiredOperationVariant{
				{RequirementID: "init", Kind: runtimecontract.StepKindGitWrite, VariantID: "init"},
				{RequirementID: "commit", Kind: runtimecontract.StepKindGitWrite, VariantID: "commit", DependsOnRequirementIDs: []string{"init"}},
			},
		},
	}, nil)
	if !runtimeOperationSchemaHasProperty(t, withDependency, "dependsOn") {
		t.Fatalf("dependent sequence schema omitted operation dependency field: %s", withDependency)
	}
}

func runtimeOperationSchemaHasProperty(t *testing.T, encoded, property string) bool {
	t.Helper()
	var root map[string]any
	if err := json.Unmarshal([]byte(encoded), &root); err != nil {
		t.Fatal(err)
	}
	operations, ok := root["properties"].(map[string]any)["operations"].(map[string]any)
	if !ok {
		t.Fatalf("candidate schema has no operations: %s", encoded)
	}
	properties, ok := operations["items"].(map[string]any)["properties"].(map[string]any)
	if !ok {
		t.Fatalf("candidate schema has no operation properties: %s", encoded)
	}
	_, found := properties[property]
	return found
}

func TestRuntimeCandidateOutputContractUsesActionSpecificForbiddenTopLevelFields(t *testing.T) {
	operation := runtimeCandidateOutputContractForDecision(t, runtimecontract.RuntimeDecision{
		State:          runtimecontract.ConvergenceStateNeedStateChange,
		RequiredOutput: runtimecontract.RequiredOutputContract{AcceptedKinds: []string{runtimecontract.StepKindFileWrite}},
	}, nil)
	if !containsString(operation.ForbiddenTopLevelFields, "dependsOn") || !containsString(operation.ForbiddenTopLevelFields, "parameters") {
		t.Fatalf("operation forbidden fields = %#v", operation.ForbiddenTopLevelFields)
	}
	assessment := runtimeCandidateOutputContractForDecision(t, runtimecontract.RuntimeDecision{State: runtimecontract.ConvergenceStateNeedAssessment}, nil)
	if containsString(assessment.ForbiddenTopLevelFields, "coverageWorkItemIds") {
		t.Fatalf("assessment forbids its required top-level field: %#v", assessment.ForbiddenTopLevelFields)
	}
	governance := runtimeCandidateOutputContractForDecision(t, runtimecontract.RuntimeDecision{State: runtimecontract.ConvergenceStateNeedGovernance}, nil)
	if !containsString(governance.ForbiddenTopLevelFields, "reason") {
		t.Fatalf("governance forbidden fields = %#v", governance.ForbiddenTopLevelFields)
	}
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func TestRuntimeCandidateSchemaDerivesPatchTargetRequirementsFromOperationContract(t *testing.T) {
	schema := runtimeCandidateSchemaForDecision(t, runtimecontract.RuntimeDecision{
		State: runtimecontract.ConvergenceStateNeedStateChange,
		RequiredOutput: runtimecontract.RequiredOutputContract{
			AcceptedKinds: []string{runtimecontract.RuntimeBlockKindPatchApply},
		},
	}, nil)
	for _, value := range []string{`"const":"patch/apply"`, `"const":"patchType"`, `"targetRefs"`, `"minItems":1`, `"maxItems":1`, `"minLength":1`} {
		if !strings.Contains(schema, value) {
			t.Fatalf("descriptor-driven patch schema lost %q: %s", value, schema)
		}
	}
	if strings.Contains(runtimeCandidateSchema(runtimecontract.ConvergenceStateNeedStateChange), `"const":"patch/apply"`) {
		t.Fatal("volatile operation constraints leaked into the stable generic contract schema")
	}
}

func TestRuntimeCandidateSchemaRequiresExactlyOneSealedBindingForProofBearingKind(t *testing.T) {
	policy, ok := runtimecontract.RuntimeConvergenceEffectPolicyFor(runtimecontract.ConvergenceStateNeedStateChange)
	if !ok {
		t.Fatal("missing state-change effect policy")
	}
	manifest := runtimecontract.RuntimeCandidateContractManifest{
		State: runtimecontract.ConvergenceStateNeedStateChange, EffectPolicy: policy,
		RequiredTopLevelFields: []string{"operations"}, AllowedTopLevelFields: []string{"operations"},
		Operations: []runtimecontract.RuntimeCandidateOperationManifest{{
			Kind: runtimecontract.RuntimeBlockKindPatchApply, OwnerModuleID: runtimecontract.RuntimeModuleFile,
			AllowedOperationFields: []string{"id", "kind", "bindingRefs", "parameters", "targetRefs"},
			BindingAuthorizations:  []runtimecontract.RuntimeOperationBindingAuthorization{{VariantID: "apply", AllowedBindingRefs: []string{"binding/cwi-1"}}},
			Parameters: []runtimecontract.RuntimeOperationParameterContract{{
				Name: "patchType", Type: runtimecontract.RuntimeOperationParameterTypeString, Required: true,
			}},
			RequireTargetRefs: true,
		}},
	}
	schema := runtimeCandidateSchemaForManifest(manifest)
	for _, want := range []string{`"bindingRefs"`, `"minItems":1`, `"maxItems":1`, `"required":["bindingRefs","parameters","targetRefs"]`} {
		if !strings.Contains(schema, want) {
			t.Fatalf("proof-bearing schema lost %q: %s", want, schema)
		}
	}
	instruction := runtimeCandidateStructuredOutputInstruction(manifest)
	if !strings.Contains(instruction, "requires exactly one bindingRefs value") || strings.Contains(instruction, "Optional bindingRefs") || strings.Contains(instruction, "may omit bindingRefs") {
		t.Fatalf("proof-bearing instruction = %s", instruction)
	}
}

func TestRuntimeCandidateSchemaRequiresCanonicalTargetRefsForDurableFileOperations(t *testing.T) {
	schema := runtimeCandidateSchemaForDecision(t, runtimecontract.RuntimeDecision{
		State: runtimecontract.ConvergenceStateNeedStateChange,
		RequiredOutput: runtimecontract.RequiredOutputContract{AcceptedKinds: []string{
			runtimecontract.StepKindFileWrite,
			runtimecontract.StepKindFileEdit,
		}},
	}, nil)
	for _, kind := range []string{runtimecontract.StepKindFileWrite, runtimecontract.StepKindFileEdit} {
		if !runtimeSchemaRequiresTopLevelTargetRefs(schema, kind) {
			t.Fatalf("%s schema must require canonical top-level targetRefs", kind)
		}
	}
}

func TestRuntimeCandidateSchemaClosesNamedParameterObjects(t *testing.T) {
	schema := runtimeCandidateSchemaForDecision(t, runtimecontract.RuntimeDecision{
		State: runtimecontract.ConvergenceStateNeedStateChange,
		RequiredOutput: runtimecontract.RequiredOutputContract{
			AcceptedKinds: []string{runtimecontract.RuntimeBlockKindPatchApply, runtimecontract.StepKindShellCommand},
		},
	}, nil)
	patchParameters := runtimeSchemaParametersForKind(t, schema, runtimecontract.RuntimeBlockKindPatchApply)
	if !runtimeSchemaHasClosedNamedParameterBranch(patchParameters, "patchType", "string") {
		t.Fatalf("patch/apply parameter schema does not close patchType as string: %#v", patchParameters)
	}
	if !runtimeSchemaHasClosedNamedParameterBranch(patchParameters, "occurrence", "integer") {
		t.Fatalf("patch/apply parameter schema does not close occurrence as integer: %#v", patchParameters)
	}
	if runtimeSchemaHasNamedParameterBranch(patchParameters, "unknown") {
		t.Fatalf("patch/apply parameter schema widened to an unknown parameter: %#v", patchParameters)
	}
	if !runtimeSchemaRequiresTopLevelTargetRefs(schema, runtimecontract.RuntimeBlockKindPatchApply) {
		t.Fatal("patch/apply schema must require canonical top-level targetRefs")
	}

	shellParameters := runtimeSchemaParametersForKind(t, schema, runtimecontract.StepKindShellCommand)
	if runtimeSchemaHasNamedParameterBranch(shellParameters, "summary") {
		t.Fatalf("shell/command schema admitted summary parameter: %#v", shellParameters)
	}
	if !runtimeSchemaHasClosedNamedParameterBranch(shellParameters, "timeoutMillis", "integer") {
		t.Fatalf("shell/command parameter schema does not close timeoutMillis as integer: %#v", shellParameters)
	}
}

func TestRuntimeCandidateSchemaClosesMCPDynamicParameters(t *testing.T) {
	toolSchema := runtimeCandidateSchemaForDecision(t, runtimecontract.RuntimeDecision{
		State:          runtimecontract.ConvergenceStateNeedValidation,
		RequiredOutput: runtimecontract.RequiredOutputContract{AcceptedKinds: []string{runtimecontract.StepKindMCPToolCall}},
	}, nil)
	toolParameters := runtimeSchemaParametersForKind(t, toolSchema, runtimecontract.StepKindMCPToolCall)
	if !runtimeSchemaHasClosedNamedParameterBranch(toolParameters, "serverId", "string") {
		t.Fatalf("mcp/tool_call lost fixed serverId parameter: %#v", toolParameters)
	}
	if !runtimeSchemaHasDynamicParameterBranch(toolParameters, `^input\\..*\\S.*$`, "string") ||
		!runtimeSchemaHasDynamicParameterBranch(toolParameters, `^input\\..*\\S.*$`, "integer") ||
		!runtimeSchemaHasDynamicParameterBranch(toolParameters, `^input\\..*\\S.*$`, "boolean") {
		t.Fatalf("mcp/tool_call dynamic input schema is not closed scalar union: %#v", toolParameters)
	}
	if runtimeSchemaHasDynamicParameterBranch(toolParameters, `^input\\..*\\S.*$`, "stringList") {
		t.Fatalf("mcp/tool_call dynamic input schema wrongly permits stringList: %#v", toolParameters)
	}

	resourceSchema := runtimeCandidateSchemaForDecision(t, runtimecontract.RuntimeDecision{
		State:          runtimecontract.ConvergenceStateNeedValidation,
		RequiredOutput: runtimecontract.RequiredOutputContract{AcceptedKinds: []string{runtimecontract.StepKindMCPResourceRead}},
	}, nil)
	resourceParameters := runtimeSchemaParametersForKind(t, resourceSchema, runtimecontract.StepKindMCPResourceRead)
	if runtimeSchemaHasDynamicParameterBranch(resourceParameters, "", "string") {
		t.Fatalf("mcp/resource_read must not expose dynamic parameter fields: %#v", resourceParameters)
	}
}

func runtimeSchemaParametersForKind(t *testing.T, encoded string, kind string) map[string]any {
	t.Helper()
	var root map[string]any
	if err := json.Unmarshal([]byte(encoded), &root); err != nil {
		t.Fatal(err)
	}
	properties, ok := root["properties"].(map[string]any)
	if !ok {
		t.Fatalf("candidate schema has no root properties: %#v", root)
	}
	operations, ok := properties["operations"].(map[string]any)
	if !ok {
		t.Fatalf("candidate schema has no operations: %#v", root)
	}
	items, ok := operations["items"].(map[string]any)
	if !ok {
		t.Fatalf("candidate schema has no operation items: %#v", operations)
	}
	for _, rawCondition := range runtimeSchemaArray(items["allOf"]) {
		condition, ok := rawCondition.(map[string]any)
		if !ok {
			continue
		}
		ifPart, ok := condition["if"].(map[string]any)
		if !ok || runtimeSchemaKindConst(ifPart) != kind {
			continue
		}
		thenPart, ok := condition["then"].(map[string]any)
		if !ok {
			t.Fatalf("operation condition for %q has no then: %#v", kind, condition)
		}
		thenProperties, ok := thenPart["properties"].(map[string]any)
		if !ok {
			t.Fatalf("operation condition for %q has no properties: %#v", kind, thenPart)
		}
		parameters, ok := thenProperties["parameters"].(map[string]any)
		if !ok {
			t.Fatalf("operation condition for %q has no parameter schema: %#v", kind, thenProperties)
		}
		return parameters
	}
	t.Fatalf("candidate schema has no operation condition for %q: %#v", kind, items["allOf"])
	return nil
}

func runtimeSchemaRequiresTopLevelTargetRefs(encoded, kind string) bool {
	var root map[string]any
	if json.Unmarshal([]byte(encoded), &root) != nil {
		return false
	}
	operations, _ := root["properties"].(map[string]any)["operations"].(map[string]any)
	items, _ := operations["items"].(map[string]any)
	for _, rawCondition := range runtimeSchemaArray(items["allOf"]) {
		condition, _ := rawCondition.(map[string]any)
		ifPart, _ := condition["if"].(map[string]any)
		if runtimeSchemaKindConst(ifPart) != kind {
			continue
		}
		thenPart, _ := condition["then"].(map[string]any)
		for _, required := range runtimeSchemaStringArray(thenPart["required"]) {
			if required == "targetRefs" {
				return true
			}
		}
	}
	return false
}

func runtimeSchemaHasClosedNamedParameterBranch(parameters map[string]any, name, valueField string) bool {
	for _, branch := range runtimeSchemaParameterBranches(parameters) {
		properties, _ := branch["properties"].(map[string]any)
		nameSchema, _ := properties["name"].(map[string]any)
		if nameSchema["const"] != name || branch["additionalProperties"] != false {
			continue
		}
		if _, ok := properties[valueField].(map[string]any); !ok {
			continue
		}
		if runtimeSchemaStringArrayContains(branch["required"], "name") && runtimeSchemaStringArrayContains(branch["required"], valueField) {
			return true
		}
	}
	return false
}

func runtimeSchemaHasNamedParameterBranch(parameters map[string]any, name string) bool {
	for _, branch := range runtimeSchemaParameterBranches(parameters) {
		properties, _ := branch["properties"].(map[string]any)
		nameSchema, _ := properties["name"].(map[string]any)
		if nameSchema["const"] == name {
			return true
		}
	}
	return false
}

func runtimeSchemaHasDynamicParameterBranch(parameters map[string]any, pattern, valueField string) bool {
	for _, branch := range runtimeSchemaParameterBranches(parameters) {
		properties, _ := branch["properties"].(map[string]any)
		nameSchema, _ := properties["name"].(map[string]any)
		if nameSchema["pattern"] != pattern || branch["additionalProperties"] != false {
			continue
		}
		if _, ok := properties[valueField].(map[string]any); !ok {
			continue
		}
		if runtimeSchemaStringArrayContains(branch["required"], "name") && runtimeSchemaStringArrayContains(branch["required"], valueField) {
			return true
		}
	}
	return false
}

func runtimeSchemaParameterBranches(parameters map[string]any) []map[string]any {
	items, _ := parameters["items"].(map[string]any)
	ret := make([]map[string]any, 0)
	for _, rawBranch := range runtimeSchemaArray(items["oneOf"]) {
		if branch, ok := rawBranch.(map[string]any); ok {
			ret = append(ret, branch)
		}
	}
	return ret
}

func runtimeSchemaKindConst(condition map[string]any) string {
	properties, _ := condition["properties"].(map[string]any)
	kind, _ := properties["kind"].(map[string]any)
	value, _ := kind["const"].(string)
	return value
}

func runtimeSchemaArray(value any) []any {
	values, _ := value.([]any)
	return values
}

func runtimeSchemaStringArray(value any) []string {
	values := runtimeSchemaArray(value)
	ret := make([]string, 0, len(values))
	for _, raw := range values {
		if value, ok := raw.(string); ok {
			ret = append(ret, value)
		}
	}
	return ret
}

func runtimeSchemaStringArrayContains(value any, want string) bool {
	for _, value := range runtimeSchemaStringArray(value) {
		if value == want {
			return true
		}
	}
	return false
}

func TestRuntimeCandidateSchemaRequiresNonEmptyCanonicalPatchTarget(t *testing.T) {
	condition := runtimeNonEmptyParameterContainsSchema("targetRef", runtimecontract.RuntimeOperationParameterTypeString)
	contains, ok := condition["contains"].(map[string]any)
	if !ok {
		t.Fatalf("condition = %#v, want contains object", condition)
	}
	properties, ok := contains["properties"].(map[string]any)
	if !ok {
		t.Fatalf("contains = %#v, want properties", contains)
	}
	target, ok := properties["string"].(map[string]any)
	if !ok || target["minLength"] != 1 {
		t.Fatalf("targetRef schema = %#v, want non-empty string", target)
	}
	required, ok := contains["required"].([]string)
	if !ok || len(required) != 2 || required[0] != "name" || required[1] != "string" {
		t.Fatalf("targetRef required = %#v, want name and string", contains["required"])
	}
}

func TestRuntimeCandidateSchemaNarrowsFileOwnerScopeToDurableFileAndDeclaredGroundedKinds(t *testing.T) {
	selection := &runtimecontract.RuntimeTargetSelection{
		PrerequisiteID: "target-selection-scope", OwnerModuleID: runtimecontract.RuntimeModuleFile,
		TargetRole: "deliverable", SelectorRef: "file/workspace_deliverable",
		Disposition: runtimecontract.RuntimeTargetSelectionScope, ObservedRevision: "workspace-v1",
	}
	decision := runtimecontract.RuntimeDecision{
		State: runtimecontract.ConvergenceStateNeedStateChange,
		RequiredOutput: runtimecontract.RequiredOutputContract{AcceptedKinds: []string{
			runtimecontract.StepKindShellCommand,
			runtimecontract.RuntimeBlockKindPatchApply,
			runtimecontract.StepKindGitWrite,
		}},
	}
	manifest := runtimeCandidateManifestForSchema(t, decision, selection)
	schema := runtimeCandidateSchemaForManifest(manifest)
	if !strings.Contains(schema, `"enum":["patch/apply","shell/command"]`) {
		t.Fatalf("scope schema = %s, want durable File and descriptor-declared grounded kind", schema)
	}
	for _, forbidden := range []string{`"git/write"`} {
		if strings.Contains(schema, forbidden) {
			t.Fatalf("scope schema admitted undeclared grounded kind %s: %s", forbidden, schema)
		}
	}
	output := runtimeCandidateOutputContractForManifest(manifest)
	if len(output.OperationContracts) != 2 || output.OperationContracts[0].Kind != runtimecontract.RuntimeBlockKindPatchApply || output.OperationContracts[1].Kind != runtimecontract.StepKindShellCommand {
		t.Fatalf("scope operation contracts = %#v, want patch/apply and shell/command", output.OperationContracts)
	}
	kinds := runtimeCandidateManifestOperationKinds(manifest)
	if got, want := kinds, []string{runtimecontract.RuntimeBlockKindPatchApply, runtimecontract.StepKindShellCommand}; !slices.Equal(got, want) {
		t.Fatalf("scope admissible kinds = %v, want %v", got, want)
	}
}

func TestRuntimeCandidateRendererProjectsSealedTargetPolicy(t *testing.T) {
	decision := runtimecontract.RuntimeDecision{State: runtimecontract.ConvergenceStateNeedStateChange, RequiredOutput: runtimecontract.RequiredOutputContract{
		AcceptedKinds:        []string{runtimecontract.StepKindFileEdit},
		AcceptedEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable},
		RequiredEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable},
		CoverageWorkItemIDs:  []common.CoverageWorkItemID{"cwi-1"},
	}}
	inventory := runtimecontract.RuntimeOperationAdmissibility{
		PrerequisiteID: "operation_admissibility:file", OwnerModuleID: runtimecontract.RuntimeModuleFile,
		OperationKinds: []string{runtimecontract.StepKindFileEdit}, TargetRefs: []string{"a.go", "b.go"},
		SelectorRefs: []string{"file/workspace_deliverable"}, ObservedRevision: "tree-v1",
	}
	inventory.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(inventory)
	manifest, err := runtimecontract.BuildRuntimeCandidateContractManifest(decision, runtimecontract.RuntimeCandidateManifestOwnerFacts{
		OperationAdmissibilities: []runtimecontract.RuntimeOperationAdmissibility{inventory},
	})
	if err != nil {
		t.Fatal(err)
	}
	output := runtimeCandidateOutputContractForManifest(manifest)
	if len(output.OperationContracts) != 1 {
		t.Fatalf("operation contracts = %#v", output.OperationContracts)
	}
	contract := output.OperationContracts[0]
	if contract.OwnerModuleID != runtimecontract.RuntimeModuleFile ||
		contract.Target.Policy != runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope ||
		!slices.Equal(contract.Target.OwnerScopeTargetRefs, []string{"a.go", "b.go"}) ||
		!slices.Equal(contract.Target.AllowedTargetRefs, []string{"a.go", "b.go"}) ||
		!slices.Equal(contract.AdmissibilitySnapshotRefs, []string{inventory.SnapshotID}) ||
		!contract.RequireTargetRefs {
		t.Fatalf("sealed target policy projection = %#v", contract)
	}
	schema := string(output.Schema)
	if !strings.Contains(schema, `"enum":["a.go","b.go"]`) {
		t.Fatalf("schema must constrain targetRefs to sealed allowed set: %s", schema)
	}
	if !strings.Contains(output.StructuredOutputInstruction, "TargetPolicy=existing_owner_scope") ||
		!strings.Contains(output.StructuredOutputInstruction, "Allowed targets: a.go, b.go") {
		t.Fatalf("instruction lost sealed target policy: %s", output.StructuredOutputInstruction)
	}
}

func TestRuntimeCandidateRendererProjectsOnlyManagedValidationRecipeIDs(t *testing.T) {
	decision := runtimecontract.RuntimeDecision{State: runtimecontract.ConvergenceStateNeedValidation, RequiredOutput: runtimecontract.RequiredOutputContract{
		AcceptedKinds:         []string{runtimecontract.RuntimeBlockKindManagedValidationSession},
		AcceptedEffectScopes:  []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectNone, runtimecontract.RuntimeEffectEphemeralSession},
		RequiredEffectScopes:  []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectNone},
		RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation},
		CoverageWorkItemIDs:   []common.CoverageWorkItemID{"cwi-1"},
	}}
	manifest, err := runtimecontract.BuildRuntimeCandidateContractManifest(decision, runtimecontract.RuntimeCandidateManifestOwnerFacts{
		ValidationRecipes: []runtimecontract.RuntimeValidationRecipe{{ID: "go-test", Command: "go", Args: []string{"test", "-v", "./..."}, TimeoutMillis: 60_000}},
	})
	if err != nil {
		t.Fatal(err)
	}
	output := runtimeCandidateOutputContractForManifest(manifest)
	if len(output.OperationContracts) != 1 || !slices.Equal(output.OperationContracts[0].ValidationRecipeIDs, []string{"go-test"}) {
		t.Fatalf("validation recipe selection projection = %#v", output.OperationContracts)
	}
	schema := string(output.Schema)
	if !strings.Contains(schema, `"recipeId"`) || !strings.Contains(schema, `"enum":["go-test"]`) {
		t.Fatalf("managed recipe schema lacks sealed recipe ID enum: %s", schema)
	}
	for _, forbidden := range []string{`"command":"go"`, `"args":["test","-v","./..."]`, "test-name filter"} {
		if strings.Contains(schema, forbidden) {
			t.Fatalf("managed recipe schema leaked Shell execution detail %q: %s", forbidden, schema)
		}
	}
	if !strings.Contains(output.StructuredOutputInstruction, "recipeId must select exactly one sealed validation recipe: go-test") ||
		!strings.Contains(output.StructuredOutputInstruction, "Do not include command, args, timeoutMillis") {
		t.Fatalf("managed recipe instruction = %s", output.StructuredOutputInstruction)
	}
}

func TestRuntimeCandidateManifestDrivesSchemaInstructionAndRetryCorrection(t *testing.T) {
	cases := []struct {
		name      string
		decision  runtimecontract.RuntimeDecision
		contains  []string
		omits     []string
		dependsOn bool
	}{
		{
			name: "patch target and closed parameters",
			decision: runtimecontract.RuntimeDecision{State: runtimecontract.ConvergenceStateNeedStateChange, RequiredOutput: runtimecontract.RequiredOutputContract{
				AcceptedKinds:        []string{runtimecontract.RuntimeBlockKindPatchApply},
				AcceptedEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable}, RequiredEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable},
				CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, TargetRefs: []string{"main.go"},
			}},
			contains: []string{"patch/apply", "targetRefs cardinality=one", "targetRefs is required", "TargetPolicy=exact_target", "Allowed targets: main.go", "Forbidden parameters"},
		},
		{
			name: "shell summary remains forbidden",
			decision: runtimecontract.RuntimeDecision{State: runtimecontract.ConvergenceStateNeedValidation, RequiredOutput: runtimecontract.RequiredOutputContract{
				AcceptedKinds:        []string{runtimecontract.StepKindShellCommand},
				AcceptedEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectNone}, RequiredEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectNone},
				CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
			}},
			contains: []string{"shell/command", "command:string"},
			omits:    []string{"summary"},
		},
		{
			name: "required sequence exposes operation-local dependency",
			decision: runtimecontract.RuntimeDecision{State: runtimecontract.ConvergenceStateNeedStateChange, RequiredOutput: runtimecontract.RequiredOutputContract{
				AcceptedKinds:        []string{runtimecontract.StepKindGitWrite},
				AcceptedEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable}, RequiredEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable},
				CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, RequiredOperationSequence: []runtimecontract.RuntimeRequiredOperationVariant{
					{RequirementID: "stage", Kind: runtimecontract.StepKindGitWrite, VariantID: "add"},
					{RequirementID: "commit", Kind: runtimecontract.StepKindGitWrite, VariantID: "commit", DependsOnRequirementIDs: []string{"stage"}},
				},
			}},
			contains:  []string{"git/write", "dependsOn", "requiredOperationRef", "stage=git/write/add", "commit=git/write/commit"},
			dependsOn: true,
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			manifest, err := runtimecontract.BuildRuntimeCandidateContractManifest(testCase.decision, runtimecontract.RuntimeCandidateManifestOwnerFacts{})
			if err != nil {
				t.Fatal(err)
			}
			output := runtimeCandidateOutputContractForManifest(manifest)
			if output.ManifestHash != manifest.Hash || output.StructuredOutputInstruction == "" {
				t.Fatalf("output contract did not retain manifest identity: %+v", output)
			}
			instruction := output.StructuredOutputInstruction
			for _, want := range testCase.contains {
				if !strings.Contains(instruction, want) {
					t.Fatalf("manifest instruction lost %q: %s", want, instruction)
				}
			}
			for _, unwanted := range testCase.omits {
				if strings.Contains(instruction, unwanted) {
					t.Fatalf("manifest instruction admitted %q: %s", unwanted, instruction)
				}
			}
			if runtimeOperationSchemaHasProperty(t, string(output.Schema), "dependsOn") != testCase.dependsOn {
				t.Fatalf("schema dependency exposure = %t, want %t: %s", runtimeOperationSchemaHasProperty(t, string(output.Schema), "dependsOn"), testCase.dependsOn, output.Schema)
			}
			retry := runtimeCandidateRetryForManifest(1, &runtimecontract.RuntimeCandidateFailure{Class: runtimecontract.RuntimeCandidateFailureClassContract, Reason: `arbitrary stale error text with parameter "summary"`}, manifest)
			if retry == nil || !strings.Contains(retry.Instruction, manifest.Hash) || !strings.Contains(retry.Instruction, "context.outputContract.structuredOutputInstruction") || strings.Contains(retry.Instruction, "arbitrary stale error text") {
				t.Fatalf("retry correction was not manifest-derived: %+v", retry)
			}
		})
	}
}

func TestRuntimeCandidateSchemaKeepsExplicitSequenceDuringFileScopeNarrowing(t *testing.T) {
	selection := &runtimecontract.RuntimeTargetSelection{
		PrerequisiteID: "target-selection-scope", OwnerModuleID: runtimecontract.RuntimeModuleFile,
		TargetRole: "deliverable", SelectorRef: "file/workspace_deliverable",
		Disposition: runtimecontract.RuntimeTargetSelectionScope, ObservedRevision: "workspace-v1",
	}
	decision := runtimecontract.RuntimeDecision{
		State: runtimecontract.ConvergenceStateNeedStateChange,
		RequiredOutput: runtimecontract.RequiredOutputContract{
			AcceptedKinds: []string{runtimecontract.StepKindShellCommand, runtimecontract.RuntimeBlockKindPatchApply, runtimecontract.StepKindGitWrite},
			RequiredOperationSequence: []runtimecontract.RuntimeRequiredOperationVariant{{
				RequirementID: "initialize-repository", Kind: runtimecontract.StepKindGitWrite, VariantID: "init",
			}},
		},
	}
	manifest := runtimeCandidateManifestForSchema(t, decision, selection)
	kinds := runtimeCandidateManifestOperationKinds(manifest)
	if got, want := kinds, []string{runtimecontract.StepKindGitWrite, runtimecontract.RuntimeBlockKindPatchApply, runtimecontract.StepKindShellCommand}; !slices.Equal(got, want) {
		t.Fatalf("scope kinds = %v, want %v", got, want)
	}
	schema := runtimeCandidateSchemaForManifest(manifest)
	if !strings.Contains(schema, `"shell/command"`) || !strings.Contains(schema, `"patch/apply"`) || !strings.Contains(schema, `"git/write"`) {
		t.Fatalf("scope sequence schema = %s", schema)
	}
}

func TestRuntimeProviderRequestNarrowsFileScopeAcrossSchemaContextAndRetry(t *testing.T) {
	selection := runtimecontract.RuntimeTargetSelection{
		PrerequisiteID: "target-selection-scope", OwnerModuleID: runtimecontract.RuntimeModuleFile,
		TargetRole: "deliverable", SelectorRef: "file/workspace_deliverable",
		Disposition: runtimecontract.RuntimeTargetSelectionScope, CandidateCount: 2,
		TargetRefs: []string{"main.go", "support.go"}, CandidateTargetRefs: []string{"main.go", "support.go"}, ObservedRevision: "workspace-v1",
	}
	selection.SnapshotID = runtimecontract.StableRuntimeTargetSelectionSnapshotID(selection)
	facts := runtimecontract.RuntimeFacts{
		WorkspaceID: "ws-scope", RunID: "run-scope", DefinitionRevision: "definition-scope", EvidenceRevision: runtimecontract.InitialRuntimeEvidenceRevision,
		Focus:    runtimecontract.CoverageFocus{ID: "focus-scope", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-scope"}, RequireTargetGrounding: true, TargetRole: "deliverable"},
		Coverage: []runtimecontract.RuntimeCoverageItemFact{{ID: "cwi-scope", State: runtimecontract.RuntimeCoverageItemStateOpen}},
		Capabilities: runtimecontract.RuntimeCapabilities{StateChangeKinds: []string{
			runtimecontract.StepKindShellCommand, runtimecontract.StepKindGitWrite, runtimecontract.RuntimeBlockKindPatchApply,
		}},
		Resources: runtimecontract.RuntimeResourceFacts{
			TargetSelection: &selection,
			ProviderCeiling: &runtimecontract.ProviderCeiling{
				Selection: runtimecontract.ProviderInferenceSelection{Target: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}}, ContextWindowTokens: 128000,
				ProviderMaxOutputTokens: 16384, EffectiveMaxOutputTokens: 4096, MaxInputTokens: 92928, MaxBytes: 96 * 1024,
				CapabilityRevision: "capability-scope", ConfigurationRevision: "config-scope",
			},
		},
		Activation: runtimecontract.RuntimeActivationContext{ProviderSelection: runtimecontract.ProviderInferenceSelection{Target: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}}},
	}
	bindRuntimeCandidateTestSelection(&facts, selection)
	bindRuntimeCandidateTestFocus(t, &facts, runtimecontract.ConvergenceStateNeedStateChange, []string{
		runtimecontract.StepKindShellCommand, runtimecontract.StepKindGitWrite, runtimecontract.RuntimeBlockKindPatchApply,
	})
	decision, err := runtimecontract.SealRuntimeDecision(facts, runtimecontract.RuntimeDecision{
		State: runtimecontract.ConvergenceStateNeedStateChange, Focus: facts.Focus,
		RequiredOutput: runtimecontract.RequiredOutputContract{
			AcceptedKinds:        []string{runtimecontract.StepKindShellCommand, runtimecontract.StepKindGitWrite, runtimecontract.RuntimeBlockKindPatchApply},
			AcceptedEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable},
			RequiredEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable},
			CoverageWorkItemIDs:  facts.Focus.CoverageWorkItemIDs,
		},
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	definition := runtimecontract.RuntimeDefinitionContext{Revision: "definition-scope", GoalID: "goal-scope", GoalDefinition: "goal", DoDID: "dod-scope", DoDDefinition: "dod"}
	prefix := runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(definition, facts.Activation.ExecutionContract)
	request, err := runtimeProviderRequest(llmmodels.RuntimeCandidateRequest{
		DocumentKind: runtimecontract.RuntimeProviderDocumentOperations,
		Decision:     decision, Transaction: decision.AuthorityTransaction(), StablePrefixHash: prefix, DefinitionContext: definition,
		CandidateManifest: runtimeCandidateManifestForRequest(t, decision, runtimecontract.RuntimeCandidateContextForDecision(facts, decision, nil)), CandidateContext: runtimecontract.RuntimeCandidateContextForDecision(facts, decision, nil),
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{`"git/write"`} {
		if strings.Contains(request.Schema, forbidden) {
			t.Fatalf("scope provider schema widened to %s: %s", forbidden, request.Schema)
		}
	}
	if !strings.Contains(request.Schema, `"enum":["patch/apply","shell/command"]`) {
		t.Fatalf("scope provider schema did not retain durable File and declared target-bound operation: %s", request.Schema)
	}
	dynamic := request.PromptContext.Sections[3].Content
	for _, required := range []string{`"admissibleOperationKinds":["patch/apply","shell/command"]`, `"operationContracts":[{"kind":"patch/apply"`, `"kind":"shell/command"`, "bounded workspace feasibility scope"} {
		if !strings.Contains(dynamic, required) {
			t.Fatalf("scope provider C3 lost %q: %s", required, dynamic)
		}
	}
	for _, forbidden := range []string{`"kind":"git/write"`} {
		if strings.Contains(dynamic, forbidden) {
			t.Fatalf("scope provider C3 widened operation contracts to %s: %s", forbidden, dynamic)
		}
	}
	retryRequest, err := runtimeProviderRequest(llmmodels.RuntimeCandidateRequest{
		DocumentKind: runtimecontract.RuntimeProviderDocumentOperations,
		Decision:     decision, Transaction: decision.AuthorityTransaction(), CandidateAttempt: 1,
		PreviousFailure:  &runtimecontract.RuntimeCandidateFailure{Class: runtimecontract.RuntimeCandidateFailureClassContract, Code: "candidate_target_outside_grounding", Reason: "owner-grounded workspace scope permits only a durable File operation with a concrete target"},
		StablePrefixHash: prefix, CandidateManifest: runtimeCandidateManifestForRequest(t, decision, runtimecontract.RuntimeCandidateContextForDecision(facts, decision, nil)), DefinitionContext: definition, CandidateContext: runtimecontract.RuntimeCandidateContextForDecision(facts, decision, nil),
	})
	if err != nil {
		t.Fatal(err)
	}
	retryTail := retryRequest.PromptContext.Sections[4].Content
	if !strings.Contains(retryTail, "context.outputContract.structuredOutputInstruction") ||
		!strings.Contains(retryTail, "sealed manifest hash") || strings.Contains(retryTail, "do not use Git, MCP, Web, or SSH") {
		t.Fatalf("scope provider retry did not use manifest-bound correction: %s", retryTail)
	}
}

func bindRuntimeCandidateTestSelection(facts *runtimecontract.RuntimeFacts, selection runtimecontract.RuntimeTargetSelection) {
	if facts == nil {
		return
	}
	facts.Prerequisites.Satisfied = append(facts.Prerequisites.Satisfied, runtimecontract.RuntimePrerequisite{
		ID:            selection.PrerequisiteID,
		Kind:          runtimecontract.PrerequisiteKindTargetSelection,
		Scope:         runtimecontract.RuntimePrerequisiteScopeFocus,
		FocusID:       facts.Focus.ID,
		OwnerModuleID: selection.OwnerModuleID,
		SubjectRef:    selection.TargetRole,
	})
}

// bindRuntimeCandidateTestFocus is a test-fixture compiler for provider
// rendering tests. Production receives activation-sealed bindings and never
// creates them at request-render time. Keeping the fixture explicit makes the
// LLM tests consume the same CWI -> binding manifest path as Runtime.
func bindRuntimeCandidateTestFocus(t *testing.T, facts *runtimecontract.RuntimeFacts, state runtimecontract.ConvergenceState, operationKinds []string) {
	t.Helper()
	if facts == nil || len(facts.Focus.CoverageWorkItemIDs) == 0 {
		return
	}
	focus := facts.Focus.Normalized()
	evidenceKind := runtimecontract.EvidenceKindStateChange
	effectScopes := []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable}
	if state == runtimecontract.ConvergenceStateNeedValidation {
		evidenceKind = runtimecontract.EvidenceKindValidation
		effectScopes = []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectNone, runtimecontract.RuntimeEffectEphemeralSession}
	}
	if len(focus.RequiredEvidenceKinds) == 0 {
		focus.RequiredEvidenceKinds = []string{evidenceKind}
	}
	proofs := make([]runtimecontract.CoverageEvidenceProof, 0, len(focus.RequiredEvidenceKinds))
	for _, kind := range focus.RequiredEvidenceKinds {
		proof := runtimecontract.CoverageEvidenceProof{EvidenceKind: kind, AllowedOperationKinds: append([]string(nil), operationKinds...)}
		if kind == runtimecontract.EvidenceKindStateChange {
			proof.RequiredEffectScopes = []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable}
		} else {
			proof.RequiredEffectScopes = append([]runtimecontract.RuntimeEffectScope(nil), effectScopes...)
			proof.ValidationRecipeID = "test-provider-render-recipe"
		}
		proofs = append(proofs, proof)
	}
	bindings := make([]runtimecontract.CoverageEvidenceBinding, 0, len(focus.CoverageWorkItemIDs))
	for _, coverageID := range focus.CoverageWorkItemIDs {
		binding := runtimecontract.CoverageEvidenceBinding{
			ID: "binding/test/llm/" + focus.ID + "/" + string(coverageID), CoverageWorkItemID: coverageID,
			FocusID: focus.ID, DefinitionRevision: facts.DefinitionRevision, Proofs: proofs,
			TargetRefs: append([]string(nil), focus.TargetRefs...), DeclarationRef: "test/llm-provider-render",
		}
		if len(binding.TargetRefs) == 0 {
			binding.TargetSelectorRef = "test/provider-render-selector"
			binding.TargetSelectorOwnerRef = "test-llm"
			binding.TargetSelectorSubjectRef = focus.ID
		}
		binding.Hash = runtimecontract.StableCoverageEvidenceBindingHash(binding)
		bindings = append(bindings, binding)
	}
	focus.CoverageEvidenceBindings = runtimecontract.NormalizeCoverageEvidenceBindings(bindings)
	facts.Focus = focus
}

func TestRuntimeCandidateSchemaRestrictsVariantsToSealedAuthority(t *testing.T) {
	patchSchema := runtimeCandidateSchemaForDecision(t, runtimecontract.RuntimeDecision{
		State: runtimecontract.ConvergenceStateNeedStateChange,
		RequiredOutput: runtimecontract.RequiredOutputContract{
			AcceptedKinds: []string{runtimecontract.RuntimeBlockKindPatchApply},
			AcceptedOperationVariants: []runtimecontract.RuntimeOperationVariantRef{{
				Kind: runtimecontract.RuntimeBlockKindPatchApply, VariantID: "file_write",
			}},
		},
	}, nil)
	if !strings.Contains(patchSchema, `"const":"file_write"`) || strings.Contains(patchSchema, `"const":"text_replace"`) || strings.Contains(patchSchema, `"const":"unified_diff"`) {
		t.Fatalf("patch schema did not restrict variants to sealed file_write authority: %s", patchSchema)
	}
	if !strings.Contains(patchSchema, runtimecontract.RuntimeOperationVariantRuleID(runtimecontract.RuntimeBlockKindPatchApply, "file_write", "contract", "")) {
		t.Fatalf("patch schema did not project the sealed variant rule id: %s", patchSchema)
	}

	commitSchema := runtimeCandidateSchemaForDecision(t, runtimecontract.RuntimeDecision{
		State: runtimecontract.ConvergenceStateNeedStateChange,
		RequiredOutput: runtimecontract.RequiredOutputContract{
			AcceptedKinds: []string{runtimecontract.StepKindGitWrite},
			AcceptedOperationVariants: []runtimecontract.RuntimeOperationVariantRef{{
				Kind: runtimecontract.StepKindGitWrite, VariantID: "commit",
			}},
		},
	}, nil)
	if !strings.Contains(commitSchema, `"const":"commit"`) || strings.Contains(commitSchema, `"const":"add"`) || strings.Contains(commitSchema, `"const":"init"`) || !strings.Contains(commitSchema, `"const":"pathspecs"`) {
		t.Fatalf("commit schema did not retain only commit rule including forbidden pathspecs: %s", commitSchema)
	}

	sequenceSchema := runtimeCandidateSchemaForDecision(t, runtimecontract.RuntimeDecision{
		State: runtimecontract.ConvergenceStateNeedStateChange,
		RequiredOutput: runtimecontract.RequiredOutputContract{
			AcceptedKinds: []string{runtimecontract.StepKindGitWrite},
			AcceptedOperationVariants: []runtimecontract.RuntimeOperationVariantRef{{
				Kind: runtimecontract.StepKindGitWrite, VariantID: "commit",
			}},
			RequiredOperationSequence: []runtimecontract.RuntimeRequiredOperationVariant{
				{RequirementID: "repo-init", Kind: runtimecontract.StepKindGitWrite, VariantID: "init"},
				{RequirementID: "stage-changes", Kind: runtimecontract.StepKindGitWrite, VariantID: "add", DependsOnRequirementIDs: []string{"repo-init"}},
				{RequirementID: "create-commit", Kind: runtimecontract.StepKindGitWrite, VariantID: "commit", DependsOnRequirementIDs: []string{"stage-changes"}},
			},
		},
	}, nil)
	for _, variant := range []string{"init", "add", "commit"} {
		if !strings.Contains(sequenceSchema, `"const":"`+variant+`"`) {
			t.Fatalf("required sequence schema lost variant %q: %s", variant, sequenceSchema)
		}
	}

	unknownVariantDecision := runtimecontract.RuntimeDecision{
		State: runtimecontract.ConvergenceStateNeedStateChange,
		RequiredOutput: runtimecontract.RequiredOutputContract{
			AcceptedKinds: []string{runtimecontract.StepKindGitWrite},
			AcceptedOperationVariants: []runtimecontract.RuntimeOperationVariantRef{{
				Kind: runtimecontract.StepKindGitWrite, VariantID: "does-not-exist",
			}},
		},
	}
	if _, err := runtimecontract.BuildRuntimeCandidateContractManifest(unknownVariantDecision, runtimecontract.RuntimeCandidateManifestOwnerFacts{}); err == nil {
		t.Fatal("unknown sealed variant must fail manifest compilation instead of producing a widened schema")
	}
}

func TestRuntimeAssessmentRequestPublishesCanonicalEvidenceRefContract(t *testing.T) {
	facts := runtimecontract.RuntimeFacts{
		Activation:  runtimecontract.RuntimeActivationContext{ProviderSelection: runtimecontract.ProviderInferenceSelection{Target: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}}},
		WorkspaceID: "ws-1", RunID: "run-1", DefinitionRevision: "definition-1", EvidenceRevision: "evidence-v1-00000000000000000001",
		Focus: runtimecontract.CoverageFocus{
			ID: "focus-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
			RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}, RequireAssessment: true,
		},
		Coverage: []runtimecontract.RuntimeCoverageItemFact{{ID: "cwi-1", State: runtimecontract.RuntimeCoverageItemStateOpen}},
		Evidence: runtimecontract.EvidenceLedger{Accepted: []runtimecontract.EvidenceFact{{
			ID: "runtime-operation-1", Kind: runtimecontract.EvidenceKindStateChange, SourceRef: "workorch://runtime-operations/dispatch-1/op-1",
			EvidenceRefs: []string{"workorch://workspace/main.go"}, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
		}}},
		Capabilities: runtimecontract.RuntimeCapabilities{StateChangeKinds: []string{"file/write"}},
		Resources: runtimecontract.RuntimeResourceFacts{ProviderCeiling: &runtimecontract.ProviderCeiling{
			Selection: runtimecontract.ProviderInferenceSelection{Target: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}}, ContextWindowTokens: 128000,
			ProviderMaxOutputTokens: 16384, EffectiveMaxOutputTokens: 4096, MaxInputTokens: 92928, MaxBytes: 96 * 1024,
			CapabilityRevision: "capability-1", ConfigurationRevision: "config-1",
		}},
	}
	facts = facts.Normalized()
	decision, err := runtimecontract.SealRuntimeDecision(facts, runtimecontract.RuntimeDecision{
		State: runtimecontract.ConvergenceStateNeedAssessment, Focus: facts.Focus,
		RequiredOutput: runtimecontract.RequiredOutputContract{CoverageWorkItemIDs: facts.Focus.CoverageWorkItemIDs},
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	definition := runtimecontract.RuntimeDefinitionContext{
		Revision: "definition-1", GoalID: "goal-1", GoalDefinition: "goal", DoDID: "dod-1", DoDDefinition: "dod",
	}
	prefix := runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(definition, facts.Activation.ExecutionContract)
	context := runtimecontract.RuntimeCandidateContextForDecision(facts, decision, nil)
	projection := buildRuntimeCandidateContextProjection(context, runtimeCandidateManifestForRequest(t, decision, context))
	wantRefs := runtimecontract.AcceptedEvidenceRefsForFocus(facts)
	if strings.Join(projection.AllowedEvidenceRefs, "|") != strings.Join(wantRefs, "|") || len(wantRefs) != 3 {
		t.Fatalf("allowed evidence refs = %+v, want %+v", projection.AllowedEvidenceRefs, wantRefs)
	}
	request, err := runtimeProviderRequest(llmmodels.RuntimeCandidateRequest{
		DocumentKind: runtimecontract.RuntimeProviderDocumentOperations,
		Decision:     decision, Transaction: decision.AuthorityTransaction(), StablePrefixHash: prefix,
		CandidateManifest: runtimeCandidateManifestForRequest(t, decision, context), DefinitionContext: definition, CandidateContext: context,
	})
	if err != nil {
		t.Fatal(err)
	}
	dynamic := request.PromptContext.Sections[3].Content
	for _, ref := range wantRefs {
		if !strings.Contains(dynamic, ref) {
			t.Fatalf("assessment context lost allowed evidence ref %q: %s", ref, dynamic)
		}
	}
	schema := runtimeCandidateSchema(runtimecontract.ConvergenceStateNeedAssessment)
	for _, contract := range []string{`"required":["focusId","coverageWorkItemIds","disposition","summary"]`, `"const":"satisfied"`, `"required":["evidenceRefs"]`, `"minItems":1`} {
		if !strings.Contains(schema, contract) {
			t.Fatalf("assessment schema lost conditional evidence contract %q: %s", contract, schema)
		}
	}
	retryFailure := runtimecontract.RuntimeCandidateFailure{Class: runtimecontract.RuntimeCandidateFailureClassSchema, Reason: "missing evidenceRefs"}
	retry, err := runtimeProviderRequest(llmmodels.RuntimeCandidateRequest{
		DocumentKind: runtimecontract.RuntimeProviderDocumentOperations,
		Decision:     decision, Transaction: decision.AuthorityTransaction(), CandidateAttempt: 1, PreviousFailure: &retryFailure, StablePrefixHash: prefix,
		CandidateManifest: runtimeCandidateManifestForRequest(t, decision, context), DefinitionContext: definition, CandidateContext: context,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, ref := range wantRefs {
		if !strings.Contains(retry.PromptContext.Sections[3].Content, ref) {
			t.Fatalf("assessment retry lost allowed evidence ref %q", ref)
		}
	}
}

func TestRuntimeCandidateContextPublishesExactRejectedShapes(t *testing.T) {
	path := "main.go"
	operation := runtimecontract.RuntimeOperation{ID: "validate-1", Kind: runtimecontract.StepKindFileRead, TargetRefs: []string{path}}
	facts := runtimecontract.RuntimeFacts{
		Activation:  runtimecontract.RuntimeActivationContext{ProviderSelection: runtimecontract.ProviderInferenceSelection{Target: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}}},
		WorkspaceID: "ws-1", RunID: "run-1", DefinitionRevision: "def-1", EvidenceRevision: "evidence-v1-00000000000000000001",
		Focus:        runtimecontract.CoverageFocus{ID: "focus-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation}},
		Coverage:     []runtimecontract.RuntimeCoverageItemFact{{ID: "cwi-1", State: runtimecontract.RuntimeCoverageItemStateOpen}},
		Capabilities: runtimecontract.RuntimeCapabilities{ValidationKinds: []string{runtimecontract.StepKindFileRead}},
		Evidence: runtimecontract.EvidenceLedger{Rejected: []runtimecontract.EvidenceFact{{
			ID: "rejected-1", Kind: runtimecontract.EvidenceKindValidation, DispatchID: "dispatch-1", OperationID: "validate-1",
			CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, Revision: "evidence-v1-00000000000000000001", FailureSignature: "sig-1",
		}}},
	}.Normalized()
	decision := runtimecontract.RuntimeDecision{State: runtimecontract.ConvergenceStateNeedValidation, RequiredOutput: runtimecontract.RequiredOutputContract{
		AcceptedKinds: []string{runtimecontract.StepKindFileRead}, CoverageWorkItemIDs: facts.Focus.CoverageWorkItemIDs,
	}}
	context := runtimecontract.RuntimeCandidateContextForDecision(facts, decision, []runtimecontract.RuntimeRejectedOperationShape{{
		Operation: operation, FailureSignature: "sig-1",
	}})
	projection := buildRuntimeCandidateContextProjection(context, runtimeCandidateManifestForRequest(t, decision, context))
	if len(projection.AdmissibleOperationKinds) != 1 || len(projection.RejectedOperationShapes) != 1 || projection.RejectedOperationShapes[0].FailureSignature != "sig-1" {
		t.Fatalf("candidate admissibility projection = %+v", projection)
	}
	payload, err := json.Marshal(projection)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"rejectedOperationShapes", "validate-1", "file/read", "sig-1", "admissibleOperationKinds"} {
		if !strings.Contains(string(payload), value) {
			t.Fatalf("candidate context lost %q: %s", value, payload)
		}
	}
}
