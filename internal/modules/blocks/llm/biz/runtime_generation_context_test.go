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

func TestRuntimePreparationContextIsOwnerNeutralScopedAndShared(t *testing.T) {
	for _, owner := range []string{runtimecontract.RuntimeModuleShell, "blocks/document-validator"} {
		t.Run(owner, func(t *testing.T) {
			constraint := runtimecontract.RuntimeProspectiveTargetSatisfactionConstraint{SchemaVersion: runtimecontract.RuntimeProspectiveTargetSatisfactionConstraintSchemaCurrent, OwnerModuleID: owner, ProspectiveSatisfactionContractRef: "owner/prepare/v1", ValidationSurfaceID: "recipe", ValidationRecipeID: "recipe", InputSchemaHash: "schema", CatalogObservedRevision: "r1", AllOf: []runtimecontract.RuntimeProspectiveTargetClause{{ID: "input", AllOf: []runtimecontract.RuntimeProspectiveTargetPredicate{{Kind: runtimecontract.RuntimeProspectiveTargetPredicatePathExact, Value: "checks/input.spec"}}}}}
			p := runtimecontract.RuntimeValidationProofPreparation{RequirementHash: "requirement", DefinitionRevision: "definition", CoverageWorkItemID: "cwi-1", DeclarationRef: "definition/dodItem/original", ValidationRecipeID: "recipe", TargetRef: "checks/input.spec", ProspectiveTargetSatisfactionConstraint: constraint, ContentContractRef: "owner/assertions/v1", ContentRequirements: "Retain both exact original obligations.", CheckAttestations: []runtimecontract.RuntimeValidationCheckAttestationBinding{{RequiredCheckRef: "check-1", ObservationProtocolRef: "owner/behavior/v1", NativeCheckRef: "native-1"}, {RequiredCheckRef: "check-2", ObservationProtocolRef: "owner/behavior/v1", NativeCheckRef: "native-2"}}}
			raw := "完整项目上下文\r\n"
			source := runtimecontract.RuntimeSourceObligationCheck{InventoryHash: "inventory", ObligationID: "original", RequirementID: "requirement", VerificationID: "verification", Criterion: "原判据\r\n包括全部限定条件", Declaration: "通过实际项目行为验证"}
			source.CheckRef = runtimecontract.StableRuntimeSourceObligationCheckRef(p.DefinitionRevision, p.CoverageWorkItemID, source)
			p.SourceObligationChecks = []runtimecontract.RuntimeSourceObligationCheck{source}
			p.CheckAttestations[0].RequiredCheckRef = source.CheckRef
			if err := p.Validate(); err != nil {
				t.Fatal(err)
			}
			c := &runtimecontract.RuntimeValidationGenerationContext{Version: runtimecontract.RuntimeValidationGenerationContextVersion, ContractRef: "owner/project-context/v1", ObservedRevision: "r1", Outcome: "complete", Files: []runtimecontract.RuntimeValidationInputReviewFile{{TargetRef: "project/source", Content: raw, ContentHash: runtimecontract.RuntimeValidationInputContentHash(raw)}}}
			catalogs := []runtimecontract.RuntimeOperationAdmissibility{{PrerequisiteID: "catalog", OwnerModuleID: owner, ObservedRevision: "r1", SnapshotID: "snapshot", ValidationSurfaces: []runtimecontract.RuntimeValidationSurface{{ID: "recipe", GenerationContext: c, ProofRequirementPreparations: []runtimecontract.RuntimeValidationProofPreparation{p}}}}}
			focus := runtimecontract.CoverageFocus{CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, ProspectiveTargetRefs: []string{p.TargetRef}}
			preparations := runtimeCandidateValidationProofPreparations(focus, catalogs)
			contexts := runtimeValidationGenerationContexts(focus.CoverageWorkItemIDs, focus.ProspectiveTargetRefs, catalogs)
			if len(preparations) != 2 || len(contexts) != 1 || contexts[0].Context.Files[0].Content != raw || len(contexts[0].RequirementHashes) != 1 {
				t.Fatal("exact source duplicated or omitted")
			}
			for _, entry := range preparations {
				if entry.OwnerModuleID != owner || entry.CoverageWorkItemID != p.CoverageWorkItemID || entry.RequirementHash != p.RequirementHash || entry.ContentRequirements != p.ContentRequirements {
					t.Fatal("preparation lost exact ownership/source")
				}
				if entry.RequiredCheckRef == source.CheckRef && (entry.SourceObligation == nil || entry.SourceObligation.Criterion != source.Criterion || entry.SourceObligation.Declaration != source.Declaration) {
					t.Fatal("exact check-to-source text mapping lost")
				}
			}
			contexts[0].Context.Files[0].Content = "changed"
			if c.Files[0].Content != raw {
				t.Fatal("projection mutated catalog")
			}
			if len(runtimeValidationGenerationContexts([]common.CoverageWorkItemID{"other-cwi"}, nil, catalogs)) != 0 || len(runtimeValidationGenerationContexts(focus.CoverageWorkItemIDs, []string{"other-target"}, catalogs)) != 0 {
				t.Fatal("unrelated context escaped scope")
			}
			manifest := runtimecontract.RuntimeSemanticPlanManifest{OwnerFactDependencyCatalogs: catalogs}
			projection, err := runtimeSemanticManifestForProvider(manifest, runtimecontract.RuntimeSemanticPlanStepProofBatch, focus.CoverageWorkItemIDs)
			if err != nil || len(projection.ValidationGenerationContexts) != 1 {
				t.Fatalf("semantic context lost: %v", err)
			}
			order, err := runtimeSemanticManifestForProvider(manifest, runtimecontract.RuntimeSemanticPlanStepCoverageOrder, nil)
			if err != nil || len(order.ValidationGenerationContexts) != 0 {
				t.Fatal("project source polluted coverage ordering")
			}
		})
	}
	contract := runtimeCandidateContractCatalog()
	if !strings.Contains(contract, "context.validationProofPreparations") || strings.Contains(contract, "testing.T") || strings.Contains(contract, "TestWorkOrchCWI_") {
		t.Fatal("generic candidate rules assume a task-specific adapter")
	}
}

func assertRuntimeSectionCacheWire(t *testing.T, request llmmodels.ProviderRequest) {
	t.Helper()
	if err := request.ValidatePromptLayout(); err != nil {
		t.Fatal(err)
	}
	if request.PromptContext.MessageLayout != llmmodels.PromptMessageLayoutSectionsV1 {
		t.Fatal("Runtime still uses legacy joined input")
	}
	budget := *request.ContextBudget
	request.ContextBudget = &budget
	wire := responsesWireInputForRequest(&request)
	var messages []responsesInputMessage
	if json.Unmarshal(wire, &messages) != nil || len(messages) != len(request.PromptContext.Sections) || responsesPromptCacheKey(request) == "" {
		t.Fatal("Runtime wire has no exact section identity")
	}
	for i, section := range request.PromptContext.Sections {
		if messages[i].Content[0].Text != "### "+section.ID+"\n"+section.Content {
			t.Fatal("wire changed protected section")
		}
		if (i == 2) != (messages[i].Content[0].PromptCacheBreakpoint != nil) {
			t.Fatal("breakpoint includes volatile authority or excludes stable contract")
		}
	}
}

func TestRuntimeSemanticUsesIsolatedSectionCacheWithoutMovingVolatileState(t *testing.T) {
	input := runtimeSemanticProviderTestRequest(t)
	base, err := runtimeProviderRequest(input)
	if err != nil {
		t.Fatal(err)
	}
	assertRuntimeSectionCacheWire(t, base)
	before := responsesPromptCacheKey(base)
	copy := base
	value := *base.PromptContext
	copy.PromptContext = &value
	copy.PromptContext.Sections = append([]llmmodels.PromptSection(nil), base.PromptContext.Sections...)
	for _, i := range []int{3, 4, 5} {
		copy.PromptContext.Sections[i].Content += " volatile observation"
		if responsesPromptCacheKey(copy) != before {
			t.Fatal("volatile state changed cache key")
		}
	}
	copy.WorkspaceID = "unrelated-workspace"
	if responsesPromptCacheKey(copy) == before {
		t.Fatal("different Workspace reused identity")
	}
	copy.WorkspaceID = base.WorkspaceID
	copy.PromptContext.Sections[1].Content = "entirely different task"
	if responsesPromptCacheKey(copy) == before {
		t.Fatal("different project reused identity")
	}
	if reflect.DeepEqual(base.PromptContext.Sections, copy.PromptContext.Sections) {
		t.Fatal("cache probe mutated original prompt")
	}
}

func TestRuntimeCacheNamespacePreservesExactDynamicSchemaAndAdmission(t *testing.T) {
	batch := runtimeSemanticProviderTestRequest(t)
	order := runtimeSemanticProviderTestRequest(t)
	order.SemanticContext.PlanStep = runtimecontract.RuntimeSemanticPlanStepCoverageOrder
	order.SemanticContext.BatchID = ""
	order.SemanticContext.BatchCoverageWorkItemIDs = nil
	refreshSemanticProgressForTest(t, &order)
	first, err := runtimeProviderRequest(batch)
	if err != nil {
		t.Fatal(err)
	}
	second, err := runtimeProviderRequest(order)
	if err != nil {
		t.Fatal(err)
	}
	if first.Schema == second.Schema || first.PromptContext.Sections[3].Content == second.PromptContext.Sections[3].Content {
		t.Fatal("test must exercise different exact schema and volatile authority")
	}
	if responsesPromptCacheKey(first) != responsesPromptCacheKey(second) {
		t.Fatal("same owner contract and actual stable prefix split by dynamic schema")
	}
	for _, r := range []llmmodels.ProviderRequest{first, second} {
		assertRuntimeSectionCacheWire(t, r)
		for _, enabled := range []bool{false, true} {
			format := responsesTextForRequest(llmmodels.ProviderCapabilities{JSONSchemaResponse: enabled}, r)
			if enabled {
				var expected any
				if err := json.Unmarshal([]byte(r.Schema), &expected); err != nil {
					t.Fatal(err)
				}
				if format == nil || format.Format == nil || !format.Format.Strict || !reflect.DeepEqual(format.Format.Schema, expected) {
					t.Fatal("cache namespace changed exact provider schema")
				}
			} else if format != nil {
				t.Fatal("cache namespace enabled undeclared provider format")
			}
		}
	}
	// Neither a stable cache family nor the unchanged prefix can authorize a
	// stale manifest. The ordinary typed renderer still rejects it locally.
	batch.SemanticManifest.DefinitionSeedHash = "foreign-seed"
	if _, err := runtimeProviderRequest(batch); err == nil {
		t.Fatal("stable cache family bypassed exact manifest gate")
	}
	for _, change := range []func(*llmmodels.ProviderRequest){
		func(r *llmmodels.ProviderRequest) { r.WorkspaceID = "different-project" },
		func(r *llmmodels.ProviderRequest) { r.Model = "other" },
		func(r *llmmodels.ProviderRequest) { r.Config.ID = "other" },
		func(r *llmmodels.ProviderRequest) { r.ReasoningEffort = "other" },
		func(r *llmmodels.ProviderRequest) { r.SchemaFormat = "other" },
		func(r *llmmodels.ProviderRequest) { r.Config.Capabilities.JSONSchemaResponse = true },
		func(r *llmmodels.ProviderRequest) { r.RawTextOutput = true },
		func(r *llmmodels.ProviderRequest) { r.PromptContext.SchemaVersion = "claimed-unknown-contract" },
		func(r *llmmodels.ProviderRequest) { r.PromptContext.Sections[0].Content += " changed contract" },
		func(r *llmmodels.ProviderRequest) { r.PromptContext.Sections[1].Content += " different requirements" },
	} {
		changed := first
		prompt := *first.PromptContext
		prompt.Sections = append([]llmmodels.PromptSection(nil), first.PromptContext.Sections...)
		changed.PromptContext = &prompt
		change(&changed)
		if responsesPromptCacheKey(changed) == responsesPromptCacheKey(first) {
			t.Fatal("different scope/format/contract reused key")
		}
	}
}

func TestRuntimeGenerationContextIntegrityRejectsLostForgedAndForeignSource(t *testing.T) {
	semantic := runtimeSemanticProviderTestRequest(t)
	provider, err := runtimeProviderRequest(semantic)
	if err != nil {
		t.Fatal(err)
	}
	context := runtimecontract.RuntimeCandidateContext{
		WorkspaceID: semantic.SemanticContext.WorkspaceID, RunID: semantic.SemanticContext.RunID,
		RequiredState: runtimecontract.ConvergenceStateNeedStateChange,
		Focus:         runtimecontract.CoverageFocus{ID: "focus", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-001"}},
	}
	context.Resources.ProviderCeiling = semantic.SemanticContext.ProviderCeiling
	context.Resources.OperationAdmissibilities = []runtimecontract.RuntimeOperationAdmissibility{{
		PrerequisiteID: "generation", OwnerModuleID: runtimecontract.RuntimeModuleShell, ObservedRevision: "r1", SnapshotID: "catalog",
		ValidationSurfaces: []runtimecontract.RuntimeValidationSurface{{ID: "recipe",
			ProofRequirementPreparations: []runtimecontract.RuntimeValidationProofPreparation{{CoverageWorkItemID: "cwi-001", RequirementHash: "original-requirement"}},
			GenerationContext: &runtimecontract.RuntimeValidationGenerationContext{Version: runtimecontract.RuntimeValidationGenerationContextVersion,
				ContractRef: "owner/selection/v1", ObservedRevision: "r1", Outcome: "complete",
				Files: []runtimecontract.RuntimeValidationInputReviewFile{{TargetRef: "project/source.txt", Content: "原始实现\r\n", ContentHash: runtimecontract.RuntimeValidationInputContentHash("原始实现\r\n")}}},
		}},
	}}
	// This gate compares the rendered bytes to already-admitted typed facts;
	// catalog/manifest admission is tested at the owner contract boundary.
	for _, scenario := range []string{"unchanged", "missing", "changed bytes and hash", "foreign owner", "foreign requirement", "invented source"} {
		t.Run(scenario, func(t *testing.T) {
			actual := runtimeCandidateAuthorityContextProjection(context, runtimecontract.RuntimeCandidateContractManifest{})
			expected := context
			switch scenario {
			case "missing":
				actual.ValidationGenerationContexts = nil
			case "changed bytes and hash":
				file := &actual.ValidationGenerationContexts[0].Context.Files[0]
				file.Content = "a self-consistent replacement"
				file.ContentHash = runtimecontract.RuntimeValidationInputContentHash(file.Content)
			case "foreign owner":
				actual.ValidationGenerationContexts[0].OwnerModuleID = runtimecontract.RuntimeModuleFile
			case "foreign requirement":
				actual.ValidationGenerationContexts[0].RequirementHashes[0] = "another-requirement"
			case "invented source":
				expected.Resources.OperationAdmissibilities = nil
			}
			data, err := json.Marshal(struct {
				Context  runtimeCandidateAuthorityProjection `json:"context"`
				Decision struct{ FocusRef string }           `json:"decision"`
			}{Context: actual, Decision: struct{ FocusRef string }{"context.focus"}})
			if err != nil {
				t.Fatal(err)
			}
			prompt := *provider.PromptContext
			prompt.Sections = append([]llmmodels.PromptSection(nil), prompt.Sections...)
			prompt.Sections[3].Content = string(data)
			budget := *provider.ContextBudget
			err = validateRuntimeCandidateContextIntegrity(semantic.SemanticContext.Definition, expected, 0, nil, &budget, &prompt)
			if (err == nil) != (scenario == "unchanged") {
				t.Fatalf("integrity result for %s: %v", scenario, err)
			}
			manifest := semantic.SemanticManifest.Clone()
			manifest.OwnerFactDependencyCatalogs = context.Resources.OperationAdmissibilities
			projection, err := runtimeSemanticManifestForProvider(manifest, semantic.SemanticContext.PlanStep, semantic.SemanticContext.BatchCoverageWorkItemIDs)
			if err != nil {
				t.Fatal(err)
			}
			projection.ValidationGenerationContexts = actual.ValidationGenerationContexts
			if scenario == "invented source" {
				manifest.OwnerFactDependencyCatalogs = nil
			}
			data, err = json.Marshal(runtimeSemanticAuthorityProjection{ContextEncoding: runtimecontract.RuntimeSemanticContextEncoding, Manifest: projection, OutputInstruction: runtimeSemanticOutputInstruction(semantic.SemanticContext, manifest)})
			if err != nil {
				t.Fatal(err)
			}
			prompt.Sections[3].Content = string(data)
			err = validateRuntimeSemanticContextIntegrity(semantic.SemanticContext, manifest, 0, nil, nil, &budget, &prompt)
			if (err == nil) != (scenario == "unchanged") {
				t.Fatalf("semantic integrity result for %s: %v", scenario, err)
			}
		})
	}
}
