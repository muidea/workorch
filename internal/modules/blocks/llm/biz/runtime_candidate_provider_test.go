package llm

import (
	"context"
	"encoding/json"
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

func TestRuntimeV2ProviderRequestPreservesStablePrefixAndTypedContext(t *testing.T) {
	facts := runtimecontract.RuntimeFacts{
		WorkspaceID: "ws-1", RunID: "run-1", DefinitionRevision: "definition-1", EvidenceRevision: runtimecontract.InitialRuntimeEvidenceRevision,
		Focus:    runtimecontract.CoverageFocus{ID: "focus-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}},
		Coverage: []runtimecontract.RuntimeCoverageItemFact{{ID: "cwi-1", State: runtimecontract.RuntimeCoverageItemStateOpen}},
		Resources: runtimecontract.RuntimeResourceFacts{ProviderCeiling: &runtimecontract.ProviderCeiling{
			Target:                   runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"},
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
			ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"},
			DialogLanguage: "zh-CN",
			DefaultSoul:    &runtimecontract.RuntimeSoulContext{ID: "soul-default", Instructions: "default-only-soul" + strings.Repeat("d", 22*1024)},
			StageSoulBindings: []runtimecontract.RuntimeStageSoulBinding{
				{Stage: "execution", Soul: runtimecontract.RuntimeSoulContext{ID: "soul-execution", Instructions: "execution-only-soul" + strings.Repeat("e", 22*1024)}},
				{Stage: "validation", Soul: runtimecontract.RuntimeSoulContext{ID: "soul-validation", Instructions: "validation-only-soul" + strings.Repeat("v", 13*1024)}},
			},
			Skills: []runtimecontract.RuntimeSkillContext{{ID: "skill-1", Instruction: "typed skill instructions"}},
			Continuation: runtimecontract.RuntimeContinuationContext{
				Replan: &runtimecontract.RuntimeReplanContinuation{
					PredecessorRunID: "run-old", TicketID: "ticket-old", ResolutionID: "resolution-old", ResolutionType: "replan", RecoveryAction: "replan",
					Failure: &runtimecontract.RuntimeContinuationFailure{Class: "validation", Signature: "signature-old", TargetRefs: []string{"target-old"}},
				},
			},
			ExecutionContract: runtimecontract.RuntimeExecutionContract{
				StateChangePolicy:       runtimecontract.RuntimeStateChangeMustProduce,
				CapabilityUsagePolicies: []runtimecontract.RuntimeCapabilityUsagePolicy{{ID: "usage-1", PreferredTools: []string{"lsp_diagnostics"}}},
				ProfileContracts: []runtimecontract.RuntimeProfileContract{{
					ID:                      "software-dev",
					AcceptanceDimensions:    []runtimecontract.RuntimeAcceptanceDimension{{ID: "runtime_entrypoint", Required: true}},
					DeliverableExpectations: []runtimecontract.RuntimeDeliverableExpectation{{ID: "version_control_snapshot", Required: true, EvidenceRole: "supplemental_evidence"}},
					EvidenceRequirements:    []runtimecontract.RuntimeProfileEvidenceRequirement{{ID: "version_control_snapshot", Scope: "deliverable", AcceptedStepKinds: []string{"git/write"}, AcceptedOperationVariants: []runtimecontract.RuntimeOperationVariantRef{{Kind: "git/write", VariantID: "commit"}}}},
				}},
				TemplateStagePolicy: &runtimecontract.RuntimeTemplateStagePolicy{SchemaVersion: runtimecontract.RuntimeTemplateStagePolicySchemaV1, Stages: []runtimecontract.RuntimeTemplateStage{{
					ID: "execution", SoulBinding: "execution", Entry: runtimecontract.RuntimeTemplateStageEntry{NextAction: "state_change"},
					RequiredOutputs: []runtimecontract.RuntimeTemplateStageRequiredOutput{{Kind: "state_change", EvidenceKinds: []string{"state_change"}}},
				}}},
			},
		},
	}
	decision, err := runtimecontract.SealRuntimeDecision(facts, runtimecontract.RuntimeDecision{
		State: runtimecontract.ConvergenceStateNeedStateChange, Action: runtimecontract.RuntimeActionProduceStateChange, Focus: facts.Focus,
		RequiredOutput: runtimecontract.RequiredOutputContract{
			Kind: runtimecontract.RequiredOutputKindStateChange, AcceptedKinds: []string{"file/edit"},
			RequiredOperationSequence: []runtimecontract.RuntimeRequiredOperationVariant{{RequirementID: "repo-init", Kind: runtimecontract.StepKindGitWrite, VariantID: "init"}},
			AcceptedEffectScopes:      []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable, runtimecontract.RuntimeEffectRemoteDurable},
			RequiredEffectScopes:      []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable, runtimecontract.RuntimeEffectRemoteDurable},
			CoverageWorkItemIDs:       facts.Focus.CoverageWorkItemIDs,
		},
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	definition := runtimecontract.RuntimeDefinitionContext{
		Revision: "definition-1", GoalID: "goal-1", GoalDefinition: "goal", DoDID: "dod-1", DoDDefinition: "dod", CapabilityContract: []string{"file/edit"},
		ChangeImpact: &runtimecontract.RuntimeDefinitionChangeImpact{
			Version: runtimecontract.RuntimeDefinitionChangeImpactVersionV1, Changed: true, Policy: "cumulative", RequiredPreExecutionAssessment: true,
			GoalChange:           runtimecontract.RuntimeDefinitionChangeSet{EffectiveDefinitionID: "goal-1"},
			DoDChange:            runtimecontract.RuntimeDefinitionChangeSet{EffectiveDefinitionID: "dod-1"},
			StrategyRequirements: []string{"identify additive constraints"},
		},
	}
	prefix := runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(definition, facts.Activation.ExecutionContract)
	rejectedPath := "main.go"
	rejectedShape := runtimecontract.RuntimeRejectedOperationShape{
		Operation:        runtimecontract.RuntimeOperation{ID: "rejected-edit", Kind: runtimecontract.StepKindFileEdit, TargetRefs: []string{rejectedPath}, Parameters: []runtimecontract.OperationParameter{{Name: "path", String: &rejectedPath}}},
		FailureSignature: "rejected-shape-signature",
	}
	candidateContext := runtimecontract.RuntimeCandidateContextForDecision(facts, decision, []runtimecontract.RuntimeRejectedOperationShape{rejectedShape})
	request, err := runtimeV2ProviderRequest(llmmodels.RuntimeCandidateRequest{
		Decision: decision, Transaction: decision.AuthorityTransaction(), StablePrefixHash: prefix,
		DefinitionContext: definition, CandidateContext: candidateContext,
	})
	if err != nil {
		t.Fatal(err)
	}
	if request.ContextBudget == nil || request.ContextBudget.CachePlan == nil || request.ContextBudget.CachePlan.StablePrefixHash != prefix ||
		request.ContextBudget.CachePlan.RenderedStablePrefixHash != prefix || len(request.PromptContext.Sections) != 4 {
		t.Fatalf("provider request cache plan = %+v prompt=%+v", request.ContextBudget, request.PromptContext)
	}
	if request.ContextBudget.ActualBytes <= 0 || request.ContextBudget.ActualBytes >= 96*1024 {
		t.Fatalf("provider request actual bytes = %d, want current-stage projection below 96 KiB", request.ContextBudget.ActualBytes)
	}
	if request.PromptContext.Sections[3].Content == "" || request.Schema == "" {
		t.Fatalf("provider request lost dynamic context: %+v", request)
	}
	contractSection := request.PromptContext.Sections[0].Content
	for _, value := range []string{`"operations"`, `"focusId"`, `"externalDependency"`, "candidate", "candidates", "executionGraph"} {
		if !strings.Contains(contractSection, value) {
			t.Fatalf("provider request stable contract lost %q: %s", value, contractSection)
		}
	}
	dynamicSection := request.PromptContext.Sections[3].Content
	for _, value := range []string{
		`"outputContract"`, `"requiredTopLevelExample":"{\"operations\":[...]}"`, `"$id":"workorch.runtime_candidate.state_change.v2"`, `"forbiddenTopLevelFields"`,
		`"operationContracts":[{"kind":"file/edit"`, `"name":"path"`, `"name":"oldText"`, `"name":"newText"`,
		`"forbiddenParameters":["filePath","fileContent","targetRef","targetRefs"]`,
		`"RequirementID":"repo-init"`, `"capabilityStates"`, `"repo-state-hash"`,
	} {
		if !strings.Contains(dynamicSection, value) {
			t.Fatalf("provider request dynamic output contract lost %q: %s", value, dynamicSection)
		}
	}
	if !strings.Contains(request.PromptContext.Sections[1].Content, "identify additive constraints") || !strings.Contains(request.PromptContext.Sections[1].Content, "cumulative") {
		t.Fatalf("provider request lost typed definition change impact: %s", request.PromptContext.Sections[1].Content)
	}
	if !strings.Contains(request.PromptContext.Sections[3].Content, "typed skill instructions") || !strings.Contains(request.PromptContext.Sections[3].Content, "zh-CN") ||
		!strings.Contains(request.PromptContext.Sections[3].Content, "execution-only-soul") || strings.Contains(request.PromptContext.Sections[3].Content, "validation-only-soul") ||
		strings.Contains(request.PromptContext.Sections[3].Content, "default-only-soul") || strings.Contains(request.PromptContext.Sections[3].Content, "must_produce_state_change") ||
		strings.Contains(request.PromptContext.Sections[3].Content, "lsp_diagnostics") ||
		!strings.Contains(request.PromptContext.Sections[3].Content, "signature-old") || !strings.Contains(request.PromptContext.Sections[3].Content, "target-old") {
		t.Fatalf("provider request lost typed activation context: %s", request.PromptContext.Sections[3].Content)
	}
	capabilitySection := request.PromptContext.Sections[2].Content
	if strings.Contains(capabilitySection, "signature-old") || strings.Contains(capabilitySection, "target-old") {
		t.Fatalf("volatile continuation leaked into C2 stable prefix: %s", capabilitySection)
	}
	for _, value := range []string{"must_produce_state_change", "lsp_diagnostics", "runtime_entrypoint", "version_control_snapshot", "git/write", "commit", "workorch.template_stage_policy.v1", "state_change"} {
		if !strings.Contains(capabilitySection, value) {
			t.Fatalf("provider request lost typed profile contract %q: %s", value, capabilitySection)
		}
	}
	for idx := 0; idx < 3; idx++ {
		if strings.Contains(request.PromptContext.Sections[idx].Content, "rejected-shape-signature") {
			t.Fatalf("rejected shape leaked into stable C%d section: %s", idx, request.PromptContext.Sections[idx].Content)
		}
	}
	if !strings.Contains(request.PromptContext.Sections[3].Content, "rejectedOperationShapes") || !strings.Contains(request.PromptContext.Sections[3].Content, "rejected-shape-signature") {
		t.Fatalf("rejected shape is not rendered in volatile candidate context: %s", request.PromptContext.Sections[3].Content)
	}
	retryFailure := runtimecontract.RuntimeCandidateFailure{Class: runtimecontract.RuntimeCandidateFailureClassSchema, Reason: `json: unknown field "candidate"`}
	retryRequest, err := runtimeV2ProviderRequest(llmmodels.RuntimeCandidateRequest{
		Decision: decision, Transaction: decision.AuthorityTransaction(), CandidateAttempt: 1, PreviousFailure: &retryFailure, StablePrefixHash: prefix,
		DefinitionContext: definition, CandidateContext: candidateContext,
	})
	if err != nil {
		t.Fatal(err)
	}
	retrySection := retryRequest.PromptContext.Sections[3].Content
	for _, value := range []string{`"candidateAttempt":1`, `"previousFailure"`, `unknown field \"candidate\"`, "do not repeat any wrapper", "executionGraph"} {
		if !strings.Contains(retrySection, value) {
			t.Fatalf("provider retry lost correction %q: %s", value, retrySection)
		}
	}
	if retryRequest.ContextBudget.CachePlan.StablePrefixHash != request.ContextBudget.CachePlan.StablePrefixHash || retryRequest.PromptContext.Sections[0].Content != contractSection {
		t.Fatalf("retry changed stable provider contract: initial=%+v retry=%+v", request.ContextBudget.CachePlan, retryRequest.ContextBudget.CachePlan)
	}
	freshContext := runtimecontract.RuntimeCandidateContextFromFacts(facts)
	freshObservation := runtimecontract.RuntimePrerequisiteObservation{
		PrerequisiteID: "admissibility-file", Kind: runtimecontract.PrerequisiteKindOperationAdmissibility, OwnerModuleID: runtimecontract.RuntimeModuleFile, ContentHash: "admissibility-v2",
		Dependencies: []runtimecontract.RuntimeFreshnessDependency{{Domain: runtimecontract.RuntimeResourceWorkspaceTree, OwnerModuleID: runtimecontract.RuntimeModuleFile, SubjectRef: "workspace_deliverable", Revision: "tree-v2"}},
	}
	freshObservation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(freshObservation)
	freshContext.Resources.PrerequisiteObservations = []runtimecontract.RuntimePrerequisiteObservation{freshObservation}
	freshRequest, err := runtimeV2ProviderRequest(llmmodels.RuntimeCandidateRequest{
		Decision: decision, Transaction: decision.AuthorityTransaction(), StablePrefixHash: prefix,
		DefinitionContext: definition, CandidateContext: freshContext,
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
	limitedContext := runtimecontract.RuntimeCandidateContextFromFacts(facts)
	limitedContext.Resources.ProviderCeiling = &runtimecontract.ProviderCeiling{
		Target:                   runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"},
		ContextWindowTokens:      128000,
		ProviderMaxOutputTokens:  16384,
		EffectiveMaxOutputTokens: 4096,
		MaxInputTokens:           92928,
		MaxBytes:                 1,
		CapabilityRevision:       "capability-1",
		ConfigurationRevision:    "config-1",
	}
	if _, err := runtimeV2ProviderRequest(llmmodels.RuntimeCandidateRequest{
		Decision: decision, Transaction: decision.AuthorityTransaction(), StablePrefixHash: prefix,
		DefinitionContext: definition, CandidateContext: limitedContext,
	}); err == nil || !strings.Contains(err.Error(), "provider ceiling") {
		t.Fatalf("runtimeV2ProviderRequest ceiling err = %v, want typed provider ceiling failure", err)
	}
}

func TestRuntimeV2CandidateContextProjectionPreservesRepairDirectiveAuthority(t *testing.T) {
	directive := runtimecontract.RuntimeRepairDirective{
		ID: "repair-1", FocusID: "focus-1", Trigger: runtimecontract.RuntimeRepairTriggerRejectedValidation,
		RequiredAction:        runtimecontract.RuntimeActionProduceStateChange,
		RequiredEffectScopes:  []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable},
		RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange},
		AllowedOperationKinds: []string{runtimecontract.StepKindFileWrite},
		CoverageWorkItemIDs:   []common.CoverageWorkItemID{"cwi-1"}, TargetRefs: []string{"main.go"},
		CurrentFailure: &runtimecontract.FailureFact{Class: runtimecontract.FailureClassExecutionRejected, Message: "validation failed"},
	}
	context := runtimecontract.RuntimeCandidateContext{
		WorkspaceID: "ws-1", RunID: "run-1", DefinitionRevision: "definition-1", EvidenceRevision: runtimecontract.InitialRuntimeEvidenceRevision,
		Focus:          runtimecontract.CoverageFocus{ID: "focus-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation}},
		RequiredAction: directive.RequiredAction, RequiredEffectScopes: directive.RequiredEffectScopes,
		RequiredEvidenceKinds: directive.RequiredEvidenceKinds, RepairDirective: &directive,
		AdmissibleOperationKinds: []string{runtimecontract.StepKindFileWrite},
	}
	projection := runtimeV2CandidateContextProjection(context, runtimecontract.RuntimeActionProduceStateChange)
	if projection.RequiredAction != runtimecontract.RuntimeActionProduceStateChange || projection.RepairDirective == nil {
		t.Fatalf("projection lost repair authority: %+v", projection)
	}
	if len(projection.RequiredEffectScopes) != 1 || projection.RequiredEffectScopes[0] != runtimecontract.RuntimeEffectWorkspaceDurable {
		t.Fatalf("projection required effects = %v", projection.RequiredEffectScopes)
	}
	if len(projection.RequiredEvidenceKinds) != 1 || projection.RequiredEvidenceKinds[0] != runtimecontract.EvidenceKindStateChange {
		t.Fatalf("projection required evidence = %v", projection.RequiredEvidenceKinds)
	}
	hints := projection.RepairHints
	if len(hints) == 0 || hints[0].Code != "obey_repair_directive" {
		t.Fatalf("projection repair hints = %+v", hints)
	}
}

func TestRuntimeV2CandidateContextProjectionPreservesTargetGroundingInVolatileAuthority(t *testing.T) {
	context := runtimecontract.RuntimeCandidateContext{
		WorkspaceID: "ws-1", RunID: "run-1", DefinitionRevision: "definition-1", EvidenceRevision: "evidence-1",
		Focus: runtimecontract.CoverageFocus{
			ID: "focus-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange},
			TargetRole: "implementation", TargetRefs: []string{"implementation.go"}, TargetDiscoverySources: []string{runtimecontract.RuntimeValidationDiscoverySourceWorkspaceManifest}, RequireTargetGrounding: true,
		},
		RequiredAction: runtimecontract.RuntimeActionProduceStateChange,
		Resources: runtimecontract.RuntimeResourceFacts{
			FailureTarget: &runtimecontract.FailureTarget{Signature: "validation-failure", TargetRefs: []string{"implementation.go"}, TargetObservation: &runtimecontract.RuntimeFailureTargetObservation{
				OwnerModuleID: runtimecontract.RuntimeModuleMCP, SubjectRef: "server-a", TargetRole: "implementation", ObservedRevision: "mcp-server-v1", TargetRefs: []string{"implementation.go"},
			}},
			TargetSnapshots: []runtimecontract.TargetSnapshot{{TargetRef: "implementation.go", ContentHash: "hash-1", ContentState: runtimecontract.SnapshotContentStateComplete}},
			TargetSelection: &runtimecontract.RuntimeTargetSelection{PrerequisiteID: "selection-1", OwnerModuleID: runtimecontract.RuntimeModuleFile, TargetRole: "implementation", Disposition: runtimecontract.RuntimeTargetSelectionSelected, TargetRefs: []string{"implementation.go"}, CandidateTargetRefs: []string{"implementation.go"}, CandidateCount: 1, ObservedRevision: "manifest-1"},
		},
	}
	context.Resources.TargetSelection.SnapshotID = runtimecontract.StableRuntimeTargetSelectionSnapshotID(*context.Resources.TargetSelection)
	projection := runtimeV2CandidateContextProjection(context, runtimecontract.RuntimeActionProduceStateChange)
	encoded, err := json.Marshal(projection)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{`"TargetRole":"implementation"`, `"TargetRefs":["implementation.go"]`, `"TargetDiscoverySources":["workspace_manifest"]`, `"RequireTargetGrounding":true`, `"failureTarget"`, `"TargetObservation"`, `"OwnerModuleID":"blocks/mcp"`, `"ObservedRevision":"mcp-server-v1"`, `"targetSelection"`, `"selected"`} {
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

func TestRuntimeV2CandidateSchemasAreActionSpecific(t *testing.T) {
	stateChange := runtimeV2CandidateSchema(runtimecontract.RuntimeActionProduceStateChange)
	validation := runtimeV2CandidateSchema(runtimecontract.RuntimeActionProduceValidation)
	if stateChange == validation {
		t.Fatal("state-change and validation candidate schemas must be distinct")
	}
	if !strings.Contains(stateChange, "workorch.runtime_candidate.state_change.v2") || !strings.Contains(stateChange, "durable") {
		t.Fatalf("state-change schema = %s", stateChange)
	}
	if !strings.Contains(validation, "workorch.runtime_candidate.validation.v2") || !strings.Contains(validation, "non-durable") {
		t.Fatalf("validation schema = %s", validation)
	}
	catalog := runtimeV2CandidateContractCatalog()
	if strings.Contains(catalog, "produce_state_change or produce_validation") || strings.Contains(catalog, "produce only those missing evidence kinds") {
		t.Fatalf("candidate catalog retains conflicting action guidance: %s", catalog)
	}
	if !strings.Contains(catalog, "sole next-action authority") || !strings.Contains(catalog, "completion-audit information only") {
		t.Fatalf("candidate catalog lost repair authority rules: %s", catalog)
	}
}

func TestRuntimeV2CandidateSchemaDerivesPatchTargetRequirementsFromOperationContract(t *testing.T) {
	schema := runtimeV2CandidateSchemaForDecision(runtimecontract.RuntimeDecision{
		Action: runtimecontract.RuntimeActionProduceStateChange,
		RequiredOutput: runtimecontract.RequiredOutputContract{
			AcceptedKinds: []string{runtimecontract.RuntimeBlockKindPatchApply},
		},
	})
	for _, value := range []string{`"const":"patch/apply"`, `"const":"patchType"`, `"const":"targetRef"`, `"const":"path"`, `"targetRefs"`, `"minItems":1`} {
		if !strings.Contains(schema, value) {
			t.Fatalf("descriptor-driven patch schema lost %q: %s", value, schema)
		}
	}
	if strings.Contains(runtimeV2CandidateSchema(runtimecontract.RuntimeActionProduceStateChange), `"const":"patch/apply"`) {
		t.Fatal("volatile operation constraints leaked into the stable generic contract schema")
	}
}

func TestRuntimeV2CandidateSchemaRestrictsVariantsToSealedAuthority(t *testing.T) {
	patchSchema := runtimeV2CandidateSchemaForDecision(runtimecontract.RuntimeDecision{
		Action: runtimecontract.RuntimeActionProduceStateChange,
		RequiredOutput: runtimecontract.RequiredOutputContract{
			AcceptedKinds: []string{runtimecontract.RuntimeBlockKindPatchApply},
			AcceptedOperationVariants: []runtimecontract.RuntimeOperationVariantRef{{
				Kind: runtimecontract.RuntimeBlockKindPatchApply, VariantID: "file_write",
			}},
		},
	})
	if !strings.Contains(patchSchema, `"const":"file_write"`) || strings.Contains(patchSchema, `"const":"text_replace"`) || strings.Contains(patchSchema, `"const":"unified_diff"`) {
		t.Fatalf("patch schema did not restrict variants to sealed file_write authority: %s", patchSchema)
	}
	if !strings.Contains(patchSchema, runtimecontract.RuntimeOperationVariantRuleID(runtimecontract.RuntimeBlockKindPatchApply, "file_write", "contract", "")) {
		t.Fatalf("patch schema did not project the sealed variant rule id: %s", patchSchema)
	}

	commitSchema := runtimeV2CandidateSchemaForDecision(runtimecontract.RuntimeDecision{
		Action: runtimecontract.RuntimeActionProduceStateChange,
		RequiredOutput: runtimecontract.RequiredOutputContract{
			AcceptedKinds: []string{runtimecontract.StepKindGitWrite},
			AcceptedOperationVariants: []runtimecontract.RuntimeOperationVariantRef{{
				Kind: runtimecontract.StepKindGitWrite, VariantID: "commit",
			}},
		},
	})
	if !strings.Contains(commitSchema, `"const":"commit"`) || strings.Contains(commitSchema, `"const":"add"`) || strings.Contains(commitSchema, `"const":"init"`) || !strings.Contains(commitSchema, `"const":"pathspecs"`) {
		t.Fatalf("commit schema did not retain only commit rule including forbidden pathspecs: %s", commitSchema)
	}

	sequenceSchema := runtimeV2CandidateSchemaForDecision(runtimecontract.RuntimeDecision{
		Action: runtimecontract.RuntimeActionProduceStateChange,
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
	})
	for _, variant := range []string{"init", "add", "commit"} {
		if !strings.Contains(sequenceSchema, `"const":"`+variant+`"`) {
			t.Fatalf("required sequence schema lost variant %q: %s", variant, sequenceSchema)
		}
	}

	unknownVariantSchema := runtimeV2CandidateSchemaForDecision(runtimecontract.RuntimeDecision{
		Action: runtimecontract.RuntimeActionProduceStateChange,
		RequiredOutput: runtimecontract.RequiredOutputContract{
			AcceptedKinds: []string{runtimecontract.StepKindGitWrite},
			AcceptedOperationVariants: []runtimecontract.RuntimeOperationVariantRef{{
				Kind: runtimecontract.StepKindGitWrite, VariantID: "does-not-exist",
			}},
		},
	})
	if !strings.Contains(unknownVariantSchema, `"oneOf":[]`) {
		t.Fatalf("unknown sealed variant widened provider schema instead of failing closed: %s", unknownVariantSchema)
	}
}

func TestRuntimeV2AssessmentRequestPublishesCanonicalEvidenceRefContract(t *testing.T) {
	facts := runtimecontract.RuntimeFacts{
		Activation:  runtimecontract.RuntimeActivationContext{ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}},
		WorkspaceID: "ws-1", RunID: "run-1", DefinitionRevision: "definition-1", EvidenceRevision: "evidence-v2-00000000000000000001",
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
			Target: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}, ContextWindowTokens: 128000,
			ProviderMaxOutputTokens: 16384, EffectiveMaxOutputTokens: 4096, MaxInputTokens: 92928, MaxBytes: 96 * 1024,
			CapabilityRevision: "capability-1", ConfigurationRevision: "config-1",
		}},
	}
	facts = facts.Normalized()
	decision, err := runtimecontract.SealRuntimeDecision(facts, runtimecontract.RuntimeDecision{
		State: runtimecontract.ConvergenceStateNeedAssessment, Action: runtimecontract.RuntimeActionProduceAssessment, Focus: facts.Focus,
		RequiredOutput: runtimecontract.RequiredOutputContract{Kind: runtimecontract.RequiredOutputKindAssessment, CoverageWorkItemIDs: facts.Focus.CoverageWorkItemIDs},
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	definition := runtimecontract.RuntimeDefinitionContext{
		Revision: "definition-1", GoalID: "goal-1", GoalDefinition: "goal", DoDID: "dod-1", DoDDefinition: "dod",
	}
	prefix := runtimecontract.StableRuntimeCandidatePrefixHashWithExecutionContract(definition, facts.Activation.ExecutionContract)
	context := runtimecontract.RuntimeCandidateContextFromFacts(facts)
	projection := runtimeV2CandidateContextProjection(context, decision.Action)
	wantRefs := runtimecontract.AcceptedEvidenceRefsForFocus(facts)
	if strings.Join(projection.AllowedEvidenceRefs, "|") != strings.Join(wantRefs, "|") || len(wantRefs) != 3 {
		t.Fatalf("allowed evidence refs = %+v, want %+v", projection.AllowedEvidenceRefs, wantRefs)
	}
	request, err := runtimeV2ProviderRequest(llmmodels.RuntimeCandidateRequest{
		Decision: decision, Transaction: decision.AuthorityTransaction(), StablePrefixHash: prefix,
		DefinitionContext: definition, CandidateContext: context,
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
	schema := runtimeV2CandidateSchema(runtimecontract.RuntimeActionProduceAssessment)
	for _, contract := range []string{`"required":["focusId","coverageWorkItemIds","disposition","summary"]`, `"const":"satisfied"`, `"required":["evidenceRefs"]`, `"minItems":1`} {
		if !strings.Contains(schema, contract) {
			t.Fatalf("assessment schema lost conditional evidence contract %q: %s", contract, schema)
		}
	}
	retryFailure := runtimecontract.RuntimeCandidateFailure{Class: runtimecontract.RuntimeCandidateFailureClassSchema, Reason: "missing evidenceRefs"}
	retry, err := runtimeV2ProviderRequest(llmmodels.RuntimeCandidateRequest{
		Decision: decision, Transaction: decision.AuthorityTransaction(), CandidateAttempt: 1, PreviousFailure: &retryFailure, StablePrefixHash: prefix,
		DefinitionContext: definition, CandidateContext: context,
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

func TestRuntimeV2CandidateContextPublishesExactRejectedShapes(t *testing.T) {
	path := "main.go"
	operation := runtimecontract.RuntimeOperation{ID: "validate-1", Kind: runtimecontract.StepKindFileRead, TargetRefs: []string{path}, Parameters: []runtimecontract.OperationParameter{{Name: "path", String: &path}}}
	facts := runtimecontract.RuntimeFacts{
		Activation:  runtimecontract.RuntimeActivationContext{ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}},
		WorkspaceID: "ws-1", RunID: "run-1", DefinitionRevision: "def-1", EvidenceRevision: "evidence-v2-00000000000000000001",
		Focus:        runtimecontract.CoverageFocus{ID: "focus-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation}},
		Coverage:     []runtimecontract.RuntimeCoverageItemFact{{ID: "cwi-1", State: runtimecontract.RuntimeCoverageItemStateOpen}},
		Capabilities: runtimecontract.RuntimeCapabilities{ValidationKinds: []string{runtimecontract.StepKindFileRead}},
		Evidence: runtimecontract.EvidenceLedger{Rejected: []runtimecontract.EvidenceFact{{
			ID: "rejected-1", Kind: runtimecontract.EvidenceKindValidation, DispatchID: "dispatch-1", OperationID: "validate-1",
			CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, Revision: "evidence-v2-00000000000000000001", FailureSignature: "sig-1",
		}}},
	}.Normalized()
	decision := runtimecontract.RuntimeDecision{Action: runtimecontract.RuntimeActionProduceValidation, RequiredOutput: runtimecontract.RequiredOutputContract{
		Kind: runtimecontract.RequiredOutputKindValidation, AcceptedKinds: []string{runtimecontract.StepKindFileRead}, CoverageWorkItemIDs: facts.Focus.CoverageWorkItemIDs,
	}}
	context := runtimecontract.RuntimeCandidateContextForDecision(facts, decision, []runtimecontract.RuntimeRejectedOperationShape{{
		Operation: operation, FailureSignature: "sig-1",
	}})
	projection := runtimeV2CandidateContextProjection(context, decision.Action)
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
