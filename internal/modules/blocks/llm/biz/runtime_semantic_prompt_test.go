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

func TestCompactSemanticPlanPromptSchemaRemovesProseMetadata(t *testing.T) {
	raw := []byte(`{"type":"object","title":"large","description":"prose","properties":{"x":{"type":"string","description":"field prose","examples":["value"]}}}`)
	compact := string(compactSemanticPlanPromptSchema(raw))
	if strings.Contains(compact, "description") || strings.Contains(compact, "examples") || strings.Contains(compact, "title") {
		t.Fatalf("compact schema retained prose metadata: %s", compact)
	}
	if !strings.Contains(compact, `"properties"`) || !strings.Contains(compact, `"type":"string"`) {
		t.Fatalf("compact schema lost structural metadata: %s", compact)
	}
}

func runtimeSemanticWorkProviderRequestForTest(t *testing.T, purpose bool, revisions ...*runtimecontract.RuntimeValidationInputRevision) llmmodels.RuntimeCandidateRequest {
	t.Helper()
	request := runtimeSemanticProviderTestRequest(t)
	ctx := &request.SemanticContext
	policy := runtimecontract.RuntimeImplementationWorkPolicy{ProtocolRef: runtimecontract.RuntimeImplementationWorkProtocolV1, OwnerModuleID: runtimecontract.RuntimeModuleFile,
		AllowedOperations: []runtimecontract.RuntimeOperationVariantRef{{Kind: runtimecontract.StepKindFileWrite}}, AllowedTargetPolicies: []runtimecontract.RuntimeOperationTargetPolicy{runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope},
		TargetSelectorRef: "work-scope", TargetSelectorOwnerRef: "profile/work", TargetSelectorSubjectRef: "implementation", TargetSelectorCandidateRefs: []string{"main.go"},
		MaxRoundsPerCWI: 2, MaxTargetsPerRound: 1, RequireFreshObservation: true, RequireFreshValidation: true, ProtectValidationInputs: true}
	var revision *runtimecontract.RuntimeValidationInputRevision
	if len(revisions) > 0 {
		revision = revisions[0].Clone()
		policy.InputRevisionProtocolRef = runtimecontract.RuntimeValidationInputRevisionProtocolV1
		policy.AllowedOperations = []runtimecontract.RuntimeOperationVariantRef{{Kind: runtimecontract.StepKindFileEdit}}
		ctx.Definition.CapabilityContract = []string{runtimecontract.StepKindFileEdit}
	}
	ctx.Activation.ExecutionContract.ProfileContracts = []runtimecontract.RuntimeProfileContract{{ID: "work", EvidenceRequirements: []runtimecontract.RuntimeProfileEvidenceRequirement{{ID: "*", Scope: "dodItem", RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation}, AcceptedStepKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession}, ImplementationWorkPolicy: &policy}}}}
	item := ctx.DefinitionSeed.CoverageItems[0]
	focus := runtimecontract.RuntimeDefinitionFocus{ID: "focus-work", SliceID: item.SliceID, AssessmentScope: item.SourceScope, AssessmentID: item.SourceID, AssessmentText: item.TargetText,
		CoverageWorkItemIDs: []common.CoverageWorkItemID{item.ID}, EvidenceDeclarationRef: "definition/dodItem/" + item.SourceID, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation}, AcceptedStepKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession}, ImplementationWorkPolicy: &policy}
	catalog := request.SemanticManifest.OwnerCatalogs[0].Clone()
	if revision != nil {
		catalog.OperationKinds = []string{runtimecontract.StepKindFileEdit}
	}
	catalog.SelectorRefs = []string{"work-scope"}
	catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	observation := runtimecontract.RuntimePrerequisiteObservation{PrerequisiteID: "manifest-work", Kind: runtimecontract.PrerequisiteKindManifest, OwnerModuleID: runtimecontract.RuntimeModuleFile, ContentHash: catalog.ObservedRevision,
		Dependencies: []runtimecontract.RuntimeFreshnessDependency{{OwnerModuleID: runtimecontract.RuntimeModuleFile, Domain: runtimecontract.RuntimeResourceWorkspaceTree, SubjectRef: "workspace_deliverable", Revision: catalog.ObservedRevision}}}
	observation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(observation)
	scopeRequest, err := runtimecontract.CompileRuntimeImplementationWorkScopeRequest(ctx.WorkspaceID, ctx.RunID, "authority-work", ctx.DefinitionSeed, []runtimecontract.RuntimeDefinitionFocus{focus}, ctx.Activation.ExecutionContract.ProfileContracts, item.ID,
		runtimecontract.RuntimeWorkspaceManifestBinding{Manifest: runtimecontract.WorkspaceManifest{Revision: catalog.ObservedRevision, TargetRefs: []string{"main.go"}}, Observation: observation})
	if err != nil {
		t.Fatal(err)
	}
	if revision != nil {
		revision.WorkspaceID, revision.RunID, revision.DefinitionSeedHash = ctx.WorkspaceID, ctx.RunID, ctx.DefinitionSeed.Hash
		revision.Binding.CoverageWorkItemID = item.ID
		revision.Binding.InputReview.Files[0].TargetRef = "main.go"
		revision.InputTargetRefs = []string{"main.go"}
		for i := range revision.Report.Checks {
			revision.Report.Checks[i].InputTargetRefs = []string{"main.go"}
		}
		revision.Binding.InputReview.Hash = runtimecontract.StableRuntimeValidationInputReviewSubjectHash(*revision.Binding.InputReview, revision.Binding)
		revision.Report.SubjectHash = revision.Binding.InputReview.Hash
		sources := runtimecontract.RuntimeImplementationWorkScopeSources{WorkspaceID: ctx.WorkspaceID, RunID: ctx.RunID, AuthoritySnapshotID: "authority-work", Seed: ctx.DefinitionSeed, Focuses: []runtimecontract.RuntimeDefinitionFocus{focus}, Profiles: ctx.Activation.ExecutionContract.ProfileContracts, CoverageWorkItemID: item.ID, ManifestBinding: scopeRequest.ManifestBinding, InputRevision: revision}
		scopeRequest, err = sources.Compile()
		if err != nil {
			t.Fatal(err)
		}
	}
	selection := runtimecontract.RuntimeTargetSelection{PrerequisiteID: scopeRequest.RequestID, OwnerModuleID: policy.OwnerModuleID, TargetRole: "implementation_work", SelectorRef: policy.TargetSelectorRef, SelectorOwnerRef: policy.TargetSelectorOwnerRef, SelectorSubjectRef: policy.TargetSelectorSubjectRef, SelectorCandidateRefs: policy.TargetSelectorCandidateRefs,
		Disposition: runtimecontract.RuntimeTargetSelectionSelected, TargetRefs: []string{"main.go"}, CandidateTargetRefs: []string{"main.go"}, CandidateCount: 1, ObservedRevision: catalog.ObservedRevision}
	selection.SnapshotID = runtimecontract.StableRuntimeTargetSelectionSnapshotID(selection)
	observation.PrerequisiteID, observation.Kind, observation.ContentHash = scopeRequest.RequestID, runtimecontract.PrerequisiteKindTargetSelection, selection.SnapshotID
	observation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(observation)
	scope := runtimecontract.RuntimeImplementationWorkScopeObservation{Request: scopeRequest, Selection: selection, Observation: observation}
	var manifest runtimecontract.RuntimeSemanticPlanManifest
	if purpose {
		manifest, err = runtimecontract.CompileRuntimeSemanticImplementationPurposeManifest(ctx.WorkspaceID, ctx.RunID, ctx.Definition, ctx.DefinitionSeed, []runtimecontract.RuntimeDefinitionFocus{focus}, ctx.Activation.ExecutionContract, []runtimecontract.RuntimeOperationAdmissibility{catalog}, runtimecontract.RuntimeImplementationWorkScopePreview{Request: scopeRequest, Observation: &scope}, nil)
		ctx.PlanStep, ctx.BatchID, ctx.BatchSequence, ctx.BatchCoverageWorkItemIDs = runtimecontract.RuntimeSemanticPlanStepCoverageOrder, "", 0, nil
	} else {
		round := runtimecontract.RuntimeImplementationWorkRound{WorkspaceID: ctx.WorkspaceID, RunID: ctx.RunID, StartupInputHash: ctx.Definition.StartupInputHash, DefinitionSeedHash: ctx.DefinitionSeed.Hash, CoverageWorkItemID: item.ID, PolicyHash: runtimecontract.StableRuntimeImplementationWorkPolicyHash(policy), Ordinal: 1, ScopeRequest: scopeRequest, ScopeObservation: &scope}
		round.Hash = runtimecontract.StableRuntimeImplementationWorkRoundHash(round)
		manifest, err = runtimecontract.CompileRuntimeSemanticPlanManifestForImplementationWork(ctx.WorkspaceID, ctx.RunID, ctx.Definition, ctx.DefinitionSeed, []runtimecontract.RuntimeDefinitionFocus{focus}, ctx.Activation.ExecutionContract, []runtimecontract.RuntimeOperationAdmissibility{catalog}, round)
	}
	if err != nil {
		t.Fatal(err)
	}
	request.SemanticManifest = manifest
	ctx.CoverageProgress, err = runtimecontract.BuildRuntimeSemanticCoverageProgress(*ctx, manifest, runtimecontract.RuntimeFacts{WorkspaceID: ctx.WorkspaceID, RunID: ctx.RunID, DefinitionRevision: ctx.Definition.Revision, DefinitionSeed: ctx.DefinitionSeed, EvidenceRevision: "evidence-1", Coverage: runtimecontract.InitialRuntimeCoverageFacts(ctx.DefinitionSeed.CoverageItems)}, 1)
	if err != nil {
		t.Fatal(err)
	}
	ctx.ExecutionHistory, err = runtimecontract.BuildRuntimeSemanticExecutionHistory(*ctx, manifest, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	budget, err := runtimecontract.BuildRuntimeExecutionBudgetView(ctx.WorkspaceID, ctx.RunID, 1, runtimecontract.RuntimeExecutionBudgetLedger{Policy: runtimecontract.DefaultRuntimeExecutionBudgetPolicy()}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	ctx.ExecutionBudget, err = runtimecontract.BuildRuntimeSemanticExecutionBudget(*ctx, manifest, budget, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	request.StablePrefixHash = runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(ctx.Definition, ctx.Activation.ExecutionContract)
	return request
}

func TestRuntimeSemanticWorkPurposeAndProofProviderContracts(t *testing.T) {
	for _, purpose := range []bool{true, false} {
		t.Run(map[bool]string{true: "purpose", false: "proof"}[purpose], func(t *testing.T) {
			request := runtimeSemanticWorkProviderRequestForTest(t, purpose)
			before, _ := json.Marshal(request)
			provider, err := runtimeSemanticPlanProviderRequest(request)
			if err != nil {
				t.Fatal(err)
			}
			var authority runtimeSemanticAuthorityProjection
			if err := json.Unmarshal([]byte(provider.PromptContext.Sections[3].Content), &authority); err != nil {
				t.Fatal(err)
			}
			var work *runtimeSemanticImplementationWorkProjection
			if purpose {
				work = authority.Manifest.ImplementationPurpose
			} else {
				work = authority.Manifest.ProofObligations[0].ImplementationWork
			}
			if work == nil || work.TerminalEvidenceCredit || !reflect.DeepEqual(work.Scope.TargetRefs, []string{"main.go"}) || !reflect.DeepEqual(work.Policy, request.SemanticManifest.ProofObligations[0].ImplementationWork.Scope.Request.Policy) {
				t.Fatal("provider lost original no-credit work scope/policy")
			}
			if provider.PromptContext.Sections[0].Content != runtimeSemanticPlanContractCatalog() || !strings.Contains(authority.OutputInstruction, "implementation") {
				t.Fatal("work purpose lost complete C0 or dynamic instruction")
			}
			for _, field := range []string{"main.go", "authority-work", work.RoundHash, work.Scope.SnapshotID} {
				if strings.Contains(provider.PromptContext.Sections[0].Content, field) {
					t.Fatal("volatile work source entered C0")
				}
			}
			tamperedData, _ := json.Marshal(provider.PromptContext)
			var tampered llmmodels.PromptContext
			if err := json.Unmarshal(tamperedData, &tampered); err != nil {
				t.Fatal(err)
			}
			if purpose {
				authority.Manifest.ImplementationPurpose = nil
			} else {
				authority.Manifest.ProofObligations[0].ImplementationWork = nil
			}
			data, _ := json.Marshal(authority)
			tampered.Sections[3].Content = string(data)
			if err := validateRuntimeSemanticContextIntegrity(request.SemanticContext, request.SemanticManifest, 0, nil, nil, provider.ContextBudget, &tampered); err == nil {
				t.Fatal("missing work C3 preimage passed integrity")
			}
			limited := request
			limited.SemanticContext = request.SemanticContext.Clone()
			limited.SemanticContext.ProviderCeiling.MaxBytes = 1024
			if rejected, err := runtimeSemanticPlanProviderRequest(limited); err == nil || !rejected.ContextBudget.OverBudget {
				t.Fatal("work render silently bypassed sealed ceiling")
			}
			after, _ := json.Marshal(request)
			if string(before) != string(after) {
				t.Fatal("provider work rendering mutated original input")
			}
		})
	}
}

func TestRuntimeSemanticWorkPurposeNativeAndPromptRemainEquivalent(t *testing.T) {
	request := runtimeSemanticWorkProviderRequestForTest(t, true)
	provider, err := runtimeSemanticPlanProviderRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	manifest := request.SemanticManifest
	document := runtimecontract.RuntimeSemanticPlanDocument{SchemaVersion: runtimecontract.RuntimeSemanticPlanDocumentSchemaCurrent, ManifestHash: manifest.Hash, DefinitionSeedHash: manifest.DefinitionSeedHash,
		PlanStep: runtimecontract.RuntimeSemanticPlanStepCoverageOrder, CoverageOrder: []common.CoverageWorkItemID{manifest.CoverageItems[0].ID}, ImplementationPurpose: runtimecontract.RuntimeImplementationWorkPurposeEffect}
	payload, err := runtimecontract.EncodeRuntimeSemanticPlanPayloadWithManifest(manifest, document)
	if err != nil {
		t.Fatal(err)
	}
	for _, enabled := range []bool{false, true} {
		transport := responsesTextForRequest(llmmodels.ProviderCapabilities{JSONSchemaResponse: enabled}, provider)
		if enabled && (transport == nil || transport.Format == nil || transport.Format.Type != "json_schema" || !transport.Format.Strict) {
			t.Fatal("native purpose schema was not dispatched")
		}
		if !enabled && transport != nil {
			t.Fatal("prompt mode gained independent native schema")
		}
		decoded, err := runtimecontract.DecodeRuntimeSemanticPlanPayloadWithManifest(manifest, payload)
		if err != nil || decoded.ImplementationPurpose != document.ImplementationPurpose {
			t.Fatalf("mode %t lost sole purpose: %v", enabled, err)
		}
		for name, mutate := range map[string]func(map[string]any){
			"missing":          func(v map[string]any) { delete(v, "implementationPurpose") },
			"null":             func(v map[string]any) { v["implementationPurpose"] = nil },
			"invalid":          func(v map[string]any) { v["implementationPurpose"] = "next_action" },
			"second authority": func(v map[string]any) { v["nextAction"] = "file/write" },
		} {
			var invalid map[string]any
			if err := json.Unmarshal(payload, &invalid); err != nil {
				t.Fatal(err)
			}
			mutate(invalid)
			raw, _ := json.Marshal(invalid)
			if _, err := runtimecontract.DecodeRuntimeSemanticPlanPayloadWithManifest(manifest, raw); err == nil {
				t.Fatalf("mode %t accepted %s", enabled, name)
			}
		}
	}
	// ordinal/history 都只进入 C3；同一 sealed Profile 的稳定 sections 逐字不变。
	for _, ordinal := range []int{2, 3} {
		changed := request
		changed.SemanticManifest = request.SemanticManifest.Clone()
		work := changed.SemanticManifest.ProofObligations[0].ImplementationWork
		work.Ordinal, work.PrecedingReceiptSetHash = ordinal, "exact-prior-collective-hash"
		work.EffectAvailable = ordinal <= work.Scope.Request.Policy.MaxRoundsPerCWI
		changed.SemanticManifest.Hash = runtimecontract.StableRuntimeSemanticPlanManifestHash(changed.SemanticManifest)
		changed.SemanticContext = request.SemanticContext.Clone()
		refreshSemanticProgressForTest(t, &changed)
		next, err := runtimeSemanticPlanProviderRequest(changed)
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 3; i++ {
			if next.PromptContext.Sections[i].Content != provider.PromptContext.Sections[i].Content {
				t.Fatalf("ordinal %d drifted C%d", ordinal, i)
			}
		}
		if next.ContextBudget.CachePlan.RenderedStablePrefixHash != provider.ContextBudget.CachePlan.RenderedStablePrefixHash || next.ContextBudget.CachePlan.StablePrefixHash != provider.ContextBudget.CachePlan.StablePrefixHash {
			t.Fatal("ordinal drifted stable prefix")
		}
		if ordinal == 3 {
			var schema map[string]any
			if err := json.Unmarshal([]byte(next.Schema), &schema); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(next.Schema, string(runtimecontract.RuntimeImplementationWorkPurposeEffect)) {
				t.Fatal("exhausted rounds retained effect enum")
			}
			document.ManifestHash, document.ImplementationPurpose = changed.SemanticManifest.Hash, runtimecontract.RuntimeImplementationWorkPurposeTerminal
			raw, err := runtimecontract.EncodeRuntimeSemanticPlanPayloadWithManifest(changed.SemanticManifest, document)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := runtimecontract.DecodeRuntimeSemanticPlanPayloadWithManifest(changed.SemanticManifest, raw); err != nil {
				t.Fatal(err)
			}
			document.ImplementationPurpose = runtimecontract.RuntimeImplementationWorkPurposeEffect
			if _, err := runtimecontract.EncodeRuntimeSemanticPlanPayloadWithManifest(changed.SemanticManifest, document); err == nil {
				t.Fatal("exhausted rounds granted new effect intent")
			}
		}
	}
}

func TestRuntimeSemanticContractRequiresCompleteCompactOutput(t *testing.T) {
	contract := runtimeSemanticPlanContractCatalog()
	for _, required := range []string{"exactly one complete compact JSON object", "finish", "unknown fields", "unrequested proposals", "preface", "suffix", "markdown fences", "repeated Goal", "RequiredOwnerFacts", "postcondition, not a current owner fact", "same ownerCatalog fact", "prospective/create-only authority cannot be used for an existing owner target", "prerequisiteClaims", "ownerModuleId", "factKind", "operationKind", "never call a requirement undeclared"} {
		if !strings.Contains(contract, required) {
			t.Fatalf("runtime semantic contract lost %q: %s", required, contract)
		}
	}
}

func TestRuntimeSemanticWorkRenderingRejectsLegacyAndSeparatesSelectorDomains(t *testing.T) {
	request := runtimeSemanticWorkProviderRequestForTest(t, false)
	// Unlike the old fixture, Profile observation and capability proposal selector
	// domains deliberately have no common identity.
	catalog := &request.SemanticManifest.OwnerCatalogs[0]
	catalog.SelectorRefs = []string{"native-existing-target"}
	catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(*catalog)
	request.SemanticManifest.Hash = runtimecontract.StableRuntimeSemanticPlanManifestHash(request.SemanticManifest)
	refreshSemanticProgressForTest(t, &request)
	expected := runtimecontract.RuntimeExpectedDocumentContract{SchemaVersion: runtimecontract.RuntimeExpectedDocumentSchemaCurrent, DocumentKind: runtimecontract.RuntimeProviderDocumentSemanticPlan, PlanStep: request.SemanticContext.PlanStep, ManifestHash: request.SemanticManifest.Hash, DefinitionSeedHash: request.SemanticManifest.DefinitionSeedHash, LogicalRequestID: "work-render-version", StablePrefixHash: request.StablePrefixHash, MaxOutputTokens: 4096, MaxRecoveryAttempts: 1, BatchID: request.SemanticContext.BatchID, BatchSequence: request.SemanticContext.BatchSequence, BatchCoverageWorkItemIDs: request.SemanticContext.BatchCoverageWorkItemIDs}
	expected.Hash = runtimecontract.StableRuntimeExpectedDocumentHash(expected)
	request.ExpectedDocument = &expected
	if _, err := runtimeSemanticPlanProviderRequest(request); err == nil {
		t.Fatal("legacy sealed work request was silently re-rendered")
	}
	legacyHash := expected.Hash
	expected.SemanticRenderContractRef = runtimecontract.RuntimeSemanticRenderContractV2
	expected.Hash = runtimecontract.StableRuntimeExpectedDocumentHash(expected)
	if expected.Hash == legacyHash {
		t.Fatal("renderer upgrade reused old expected-document identity")
	}
	before, _ := json.Marshal(request)
	provider, err := runtimeSemanticPlanProviderRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	instruction := runtimeSemanticBaseOutputInstruction(request.SemanticContext, request.SemanticManifest)
	if strings.Contains(instruction, "requiredEffectScopes") || !strings.Contains(instruction, "derived internally") || !strings.Contains(instruction, "Profile targetSelectorRef") {
		t.Fatal("work instruction and wire contract diverged")
	}
	if strings.Contains(provider.Schema, `"enum":["work-scope"]`) {
		t.Fatal("Profile selector was imposed on capability proposal")
	}
	after, _ := json.Marshal(request)
	if string(before) != string(after) {
		t.Fatal("rendering mutated workspace input")
	}
}
