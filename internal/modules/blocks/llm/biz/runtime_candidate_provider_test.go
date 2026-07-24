package llm

import (
	"context"
	"encoding/json"
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
	selection := runtimecontract.RuntimeTargetSelection{
		PrerequisiteID: "target-selection-1", OwnerModuleID: runtimecontract.RuntimeModuleFile,
		TargetRole: "deliverable", SelectorRef: "file/workspace_deliverable",
		Disposition: runtimecontract.RuntimeTargetSelectionCreate, ProspectiveTargetRef: "initial-deliverable.txt", CandidateCount: 0, ObservedRevision: "workspace-empty-v1",
	}
	selection.SnapshotID = runtimecontract.StableRuntimeTargetSelectionSnapshotID(selection)
	facts.Resources.TargetSelection = &selection
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
	if candidateContext.RequiredAction != decision.Action || !slices.Equal(candidateContext.RequiredEffectScopes, decision.RequiredOutput.RequiredEffectScopes) ||
		!slices.Equal(candidateContext.RequiredEvidenceKinds, decision.RequiredOutput.RequiredEvidenceKinds) {
		t.Fatalf("candidate context requirements = %+v, want sealed decision output %+v", candidateContext, decision.RequiredOutput)
	}
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
	for _, value := range []string{`"operations"`, `"focusId"`, `"externalDependency"`, "candidate", "candidates", "executionGraph", "durable-effect witnesses", "copy that exact value into its top-level targetRefs"} {
		if !strings.Contains(contractSection, value) {
			t.Fatalf("provider request stable contract lost %q: %s", value, contractSection)
		}
	}
	dynamicSection := request.PromptContext.Sections[3].Content
	for _, value := range []string{
		`"outputContract"`, `"requiredTopLevelExample":"{\"operations\":[...]}"`, `"$id":"workorch.runtime_candidate.state_change.v2"`, `"forbiddenTopLevelFields"`,
		`"operationContracts":[{"kind":"file/edit"`, `"name":"path"`, `"name":"oldText"`, `"name":"newText"`,
		`"forbiddenParameters":["filePath","fileContent","targetRef","targetRefs"]`,
		`"RequirementID":"repo-init"`, `"capabilityStates"`, `"repo-state-hash"`, `"targetSelectionInstruction"`, "empty workspace creation scope", `sealed prospective target \"initial-deliverable.txt\"`,
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
	freshContext := runtimecontract.RuntimeCandidateContextForDecision(facts, decision, nil)
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
	limitedContext := runtimecontract.RuntimeCandidateContextForDecision(facts, decision, nil)
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

func TestRuntimeV2CandidateRetryExplainsOperationShapeReplayWithoutChangingStablePrefix(t *testing.T) {
	failure := &runtimecontract.RuntimeCandidateFailure{
		Class:  runtimecontract.RuntimeCandidateFailureClassContract,
		Code:   "candidate_replays_rejected_shape",
		Reason: "runtime candidate replays active rejected operation shape(s)",
	}
	retry := runtimeV2CandidateRetry(1, failure)
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
	if retry := runtimeV2CandidateRetry(1, &runtimecontract.RuntimeCandidateFailure{
		Class: runtimecontract.RuntimeCandidateFailureClassContract, Code: "candidate_admission_invalid",
	}); retry == nil || strings.Contains(retry.Instruction, "materially different operation shape") {
		t.Fatalf("non-replay retry instruction = %+v, want no replay-specific correction", retry)
	}
}

func TestRuntimeV2CandidateRetryExplainsCanonicalTargetRefConsistency(t *testing.T) {
	retry := runtimeV2CandidateRetry(1, &runtimecontract.RuntimeCandidateFailure{
		Class:  runtimecontract.RuntimeCandidateFailureClassContract,
		Reason: `runtime candidate operation "op-write": kind "patch/apply" targetRefs must contain targetRef "new/manager.go"`,
	})
	if retry == nil {
		t.Fatal("retry projection is nil")
	}
	for _, want := range []string{"non-empty canonical targetRef or path", "top-level targetRefs", "even when the file does not yet exist"} {
		if !strings.Contains(retry.Instruction, want) {
			t.Fatalf("retry instruction lost %q: %s", want, retry.Instruction)
		}
	}
}

func TestRuntimeV2TargetSelectionInstructionIsLimitedToFileScopes(t *testing.T) {
	selection := runtimecontract.RuntimeTargetSelection{
		PrerequisiteID: "target-selection-1", OwnerModuleID: runtimecontract.RuntimeModuleFile,
		TargetRole: "deliverable", SelectorRef: "file/workspace_deliverable",
		Disposition: runtimecontract.RuntimeTargetSelectionCreate, ProspectiveTargetRef: "initial-deliverable.txt", CandidateCount: 0, ObservedRevision: "workspace-empty-v1",
	}
	selection.SnapshotID = runtimecontract.StableRuntimeTargetSelectionSnapshotID(selection)
	got := runtimeV2TargetSelectionInstruction(&selection, runtimecontract.RuntimeActionProduceStateChange)
	for _, required := range []string{"durable File-owner operation", `sealed prospective target "initial-deliverable.txt"`, "Git, Shell, MCP, or another owner"} {
		if !strings.Contains(got, required) {
			t.Fatalf("creation instruction lost %q: %s", required, got)
		}
	}
	if got := runtimeV2TargetSelectionInstruction(&selection, runtimecontract.RuntimeActionProduceValidation); got != "" {
		t.Fatalf("validation must not receive creation instruction: %q", got)
	}
	selection.Disposition = runtimecontract.RuntimeTargetSelectionScope
	selection.TargetRefs = []string{"implementation.go", "support.go"}
	selection.CandidateTargetRefs = append([]string(nil), selection.TargetRefs...)
	selection.CandidateCount = len(selection.TargetRefs)
	selection.SnapshotID = runtimecontract.StableRuntimeTargetSelectionSnapshotID(selection)
	got = runtimeV2TargetSelectionInstruction(&selection, runtimecontract.RuntimeActionProduceStateChange)
	for _, required := range []string{"bounded workspace feasibility scope, not a selected repair target", "Focus, FailureTarget, current failure, or RepairDirective", "may be a new file", "Do not use this scope to authorize Git, Shell, MCP"} {
		if !strings.Contains(got, required) {
			t.Fatalf("scope instruction lost %q: %s", required, got)
		}
	}
	selection.OwnerModuleID = runtimecontract.RuntimeModuleGit
	selection.SnapshotID = runtimecontract.StableRuntimeTargetSelectionSnapshotID(selection)
	if got := runtimeV2TargetSelectionInstruction(&selection, runtimecontract.RuntimeActionProduceStateChange); got != "" {
		t.Fatalf("non-File selection must not receive creation instruction: %q", got)
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
			TargetSnapshots: []runtimecontract.TargetSnapshot{{TargetRef: "implementation.go", ContentHash: "hash-1", ContentState: runtimecontract.SnapshotContentStateComplete}, {TargetRef: "support.go", ContentHash: "hash-2", ContentState: runtimecontract.SnapshotContentStateComplete}},
			TargetSelection: &runtimecontract.RuntimeTargetSelection{PrerequisiteID: "selection-1", OwnerModuleID: runtimecontract.RuntimeModuleFile, TargetRole: "implementation", SelectorRef: "file/workspace_deliverable", Disposition: runtimecontract.RuntimeTargetSelectionScope, TargetRefs: []string{"implementation.go", "support.go"}, CandidateTargetRefs: []string{"implementation.go", "support.go"}, CandidateCount: 2, ObservedRevision: "manifest-1"},
		},
	}
	context.Resources.TargetSelection.SnapshotID = runtimecontract.StableRuntimeTargetSelectionSnapshotID(*context.Resources.TargetSelection)
	projection := runtimeV2CandidateContextProjection(context, runtimecontract.RuntimeActionProduceStateChange)
	encoded, err := json.Marshal(projection)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{`"TargetRole":"implementation"`, `"TargetRefs":["implementation.go"]`, `"TargetDiscoverySources":["workspace_manifest"]`, `"RequireTargetGrounding":true`, `"failureTarget"`, `"TargetObservation"`, `"OwnerModuleID":"blocks/mcp"`, `"ObservedRevision":"mcp-server-v1"`, `"targetSelection"`, `"scope"`, `"support.go"`} {
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
	for _, value := range []string{`"const":"patch/apply"`, `"const":"patchType"`, `"const":"targetRef"`, `"const":"path"`, `"targetRefs"`, `"minItems":1`, `"minLength":1`} {
		if !strings.Contains(schema, value) {
			t.Fatalf("descriptor-driven patch schema lost %q: %s", value, schema)
		}
	}
	if strings.Contains(runtimeV2CandidateSchema(runtimecontract.RuntimeActionProduceStateChange), `"const":"patch/apply"`) {
		t.Fatal("volatile operation constraints leaked into the stable generic contract schema")
	}
}

func TestRuntimeV2CandidateSchemaRequiresCanonicalTargetRefsForDurableFileOperations(t *testing.T) {
	schema := runtimeV2CandidateSchemaForDecision(runtimecontract.RuntimeDecision{
		Action: runtimecontract.RuntimeActionProduceStateChange,
		RequiredOutput: runtimecontract.RequiredOutputContract{AcceptedKinds: []string{
			runtimecontract.StepKindFileWrite,
			runtimecontract.StepKindFileEdit,
		}},
	})
	for _, kind := range []string{runtimecontract.StepKindFileWrite, runtimecontract.StepKindFileEdit} {
		if !runtimeV2SchemaRequiresTopLevelTargetRefs(schema, kind) {
			t.Fatalf("%s schema must require canonical top-level targetRefs", kind)
		}
	}
}

func TestRuntimeV2CandidateSchemaClosesNamedParameterObjects(t *testing.T) {
	schema := runtimeV2CandidateSchemaForDecision(runtimecontract.RuntimeDecision{
		Action: runtimecontract.RuntimeActionProduceStateChange,
		RequiredOutput: runtimecontract.RequiredOutputContract{
			AcceptedKinds: []string{runtimecontract.RuntimeBlockKindPatchApply, runtimecontract.StepKindShellCommand},
		},
	})
	patchParameters := runtimeV2SchemaParametersForKind(t, schema, runtimecontract.RuntimeBlockKindPatchApply)
	if !runtimeV2SchemaHasClosedNamedParameterBranch(patchParameters, "patchType", "string") {
		t.Fatalf("patch/apply parameter schema does not close patchType as string: %#v", patchParameters)
	}
	if !runtimeV2SchemaHasClosedNamedParameterBranch(patchParameters, "occurrence", "integer") {
		t.Fatalf("patch/apply parameter schema does not close occurrence as integer: %#v", patchParameters)
	}
	if runtimeV2SchemaHasNamedParameterBranch(patchParameters, "unknown") {
		t.Fatalf("patch/apply parameter schema widened to an unknown parameter: %#v", patchParameters)
	}
	if !runtimeV2SchemaRequiresTopLevelTargetRefs(schema, runtimecontract.RuntimeBlockKindPatchApply) {
		t.Fatal("patch/apply schema must require canonical top-level targetRefs")
	}

	shellParameters := runtimeV2SchemaParametersForKind(t, schema, runtimecontract.StepKindShellCommand)
	if runtimeV2SchemaHasNamedParameterBranch(shellParameters, "summary") {
		t.Fatalf("shell/command schema admitted summary parameter: %#v", shellParameters)
	}
	if !runtimeV2SchemaHasClosedNamedParameterBranch(shellParameters, "timeoutMillis", "integer") {
		t.Fatalf("shell/command parameter schema does not close timeoutMillis as integer: %#v", shellParameters)
	}
}

func TestRuntimeV2CandidateSchemaClosesMCPDynamicParameters(t *testing.T) {
	toolSchema := runtimeV2CandidateSchemaForDecision(runtimecontract.RuntimeDecision{
		Action:         runtimecontract.RuntimeActionProduceValidation,
		RequiredOutput: runtimecontract.RequiredOutputContract{AcceptedKinds: []string{runtimecontract.StepKindMCPToolCall}},
	})
	toolParameters := runtimeV2SchemaParametersForKind(t, toolSchema, runtimecontract.StepKindMCPToolCall)
	if !runtimeV2SchemaHasClosedNamedParameterBranch(toolParameters, "serverId", "string") {
		t.Fatalf("mcp/tool_call lost fixed serverId parameter: %#v", toolParameters)
	}
	if !runtimeV2SchemaHasDynamicParameterBranch(toolParameters, `^input\\..*\\S.*$`, "string") ||
		!runtimeV2SchemaHasDynamicParameterBranch(toolParameters, `^input\\..*\\S.*$`, "integer") ||
		!runtimeV2SchemaHasDynamicParameterBranch(toolParameters, `^input\\..*\\S.*$`, "boolean") {
		t.Fatalf("mcp/tool_call dynamic input schema is not closed scalar union: %#v", toolParameters)
	}
	if runtimeV2SchemaHasDynamicParameterBranch(toolParameters, `^input\\..*\\S.*$`, "stringList") {
		t.Fatalf("mcp/tool_call dynamic input schema wrongly permits stringList: %#v", toolParameters)
	}

	resourceSchema := runtimeV2CandidateSchemaForDecision(runtimecontract.RuntimeDecision{
		Action:         runtimecontract.RuntimeActionProduceValidation,
		RequiredOutput: runtimecontract.RequiredOutputContract{AcceptedKinds: []string{runtimecontract.StepKindMCPResourceRead}},
	})
	resourceParameters := runtimeV2SchemaParametersForKind(t, resourceSchema, runtimecontract.StepKindMCPResourceRead)
	if runtimeV2SchemaHasDynamicParameterBranch(resourceParameters, "", "string") {
		t.Fatalf("mcp/resource_read must not expose dynamic parameter fields: %#v", resourceParameters)
	}
}

func runtimeV2SchemaParametersForKind(t *testing.T, encoded string, kind string) map[string]any {
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
	for _, rawCondition := range runtimeV2SchemaArray(items["allOf"]) {
		condition, ok := rawCondition.(map[string]any)
		if !ok {
			continue
		}
		ifPart, ok := condition["if"].(map[string]any)
		if !ok || runtimeV2SchemaKindConst(ifPart) != kind {
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

func runtimeV2SchemaRequiresTopLevelTargetRefs(encoded, kind string) bool {
	var root map[string]any
	if json.Unmarshal([]byte(encoded), &root) != nil {
		return false
	}
	operations, _ := root["properties"].(map[string]any)["operations"].(map[string]any)
	items, _ := operations["items"].(map[string]any)
	for _, rawCondition := range runtimeV2SchemaArray(items["allOf"]) {
		condition, _ := rawCondition.(map[string]any)
		ifPart, _ := condition["if"].(map[string]any)
		if runtimeV2SchemaKindConst(ifPart) != kind {
			continue
		}
		thenPart, _ := condition["then"].(map[string]any)
		for _, required := range runtimeV2SchemaStringArray(thenPart["required"]) {
			if required == "targetRefs" {
				return true
			}
		}
	}
	return false
}

func runtimeV2SchemaHasClosedNamedParameterBranch(parameters map[string]any, name, valueField string) bool {
	for _, branch := range runtimeV2SchemaParameterBranches(parameters) {
		properties, _ := branch["properties"].(map[string]any)
		nameSchema, _ := properties["name"].(map[string]any)
		if nameSchema["const"] != name || branch["additionalProperties"] != false {
			continue
		}
		if _, ok := properties[valueField].(map[string]any); !ok {
			continue
		}
		if runtimeV2SchemaStringArrayContains(branch["required"], "name") && runtimeV2SchemaStringArrayContains(branch["required"], valueField) {
			return true
		}
	}
	return false
}

func runtimeV2SchemaHasNamedParameterBranch(parameters map[string]any, name string) bool {
	for _, branch := range runtimeV2SchemaParameterBranches(parameters) {
		properties, _ := branch["properties"].(map[string]any)
		nameSchema, _ := properties["name"].(map[string]any)
		if nameSchema["const"] == name {
			return true
		}
	}
	return false
}

func runtimeV2SchemaHasDynamicParameterBranch(parameters map[string]any, pattern, valueField string) bool {
	for _, branch := range runtimeV2SchemaParameterBranches(parameters) {
		properties, _ := branch["properties"].(map[string]any)
		nameSchema, _ := properties["name"].(map[string]any)
		if nameSchema["pattern"] != pattern || branch["additionalProperties"] != false {
			continue
		}
		if _, ok := properties[valueField].(map[string]any); !ok {
			continue
		}
		if runtimeV2SchemaStringArrayContains(branch["required"], "name") && runtimeV2SchemaStringArrayContains(branch["required"], valueField) {
			return true
		}
	}
	return false
}

func runtimeV2SchemaParameterBranches(parameters map[string]any) []map[string]any {
	items, _ := parameters["items"].(map[string]any)
	ret := make([]map[string]any, 0)
	for _, rawBranch := range runtimeV2SchemaArray(items["oneOf"]) {
		if branch, ok := rawBranch.(map[string]any); ok {
			ret = append(ret, branch)
		}
	}
	return ret
}

func runtimeV2SchemaKindConst(condition map[string]any) string {
	properties, _ := condition["properties"].(map[string]any)
	kind, _ := properties["kind"].(map[string]any)
	value, _ := kind["const"].(string)
	return value
}

func runtimeV2SchemaArray(value any) []any {
	values, _ := value.([]any)
	return values
}

func runtimeV2SchemaStringArray(value any) []string {
	values := runtimeV2SchemaArray(value)
	ret := make([]string, 0, len(values))
	for _, raw := range values {
		if value, ok := raw.(string); ok {
			ret = append(ret, value)
		}
	}
	return ret
}

func runtimeV2SchemaStringArrayContains(value any, want string) bool {
	for _, value := range runtimeV2SchemaStringArray(value) {
		if value == want {
			return true
		}
	}
	return false
}

func TestRuntimeV2CandidateSchemaRequiresNonEmptyCanonicalPatchTarget(t *testing.T) {
	condition := runtimeV2NonEmptyParameterContainsSchema("targetRef", runtimecontract.RuntimeOperationParameterTypeString)
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

func TestRuntimeV2CandidateSchemaNarrowsFileOwnerScopeToDurableFileKinds(t *testing.T) {
	selection := &runtimecontract.RuntimeTargetSelection{
		PrerequisiteID: "target-selection-scope", OwnerModuleID: runtimecontract.RuntimeModuleFile,
		TargetRole: "deliverable", SelectorRef: "file/workspace_deliverable",
		Disposition: runtimecontract.RuntimeTargetSelectionScope, ObservedRevision: "workspace-v1",
	}
	decision := runtimecontract.RuntimeDecision{
		Action: runtimecontract.RuntimeActionProduceStateChange,
		RequiredOutput: runtimecontract.RequiredOutputContract{AcceptedKinds: []string{
			runtimecontract.StepKindShellCommand,
			runtimecontract.RuntimeBlockKindPatchApply,
			runtimecontract.StepKindGitWrite,
		}},
	}
	schema := runtimeV2CandidateSchemaForDecisionWithTargetSelection(decision, selection)
	if !strings.Contains(schema, `"enum":["patch/apply"]`) {
		t.Fatalf("scope schema = %s, want only durable File kind", schema)
	}
	for _, forbidden := range []string{`"shell/command"`, `"git/write"`} {
		if strings.Contains(schema, forbidden) {
			t.Fatalf("scope schema admitted non-File kind %s: %s", forbidden, schema)
		}
	}
	output := runtimeV2CandidateOutputContract(decision, selection)
	if len(output.OperationContracts) != 1 || output.OperationContracts[0].Kind != runtimecontract.RuntimeBlockKindPatchApply {
		t.Fatalf("scope operation contracts = %#v, want patch/apply only", output.OperationContracts)
	}
	kinds := runtimeV2CandidateAllowedOperationKinds(decision.Action, decision.RequiredOutput.AcceptedKinds, nil, selection)
	if len(kinds) != 1 || kinds[0] != runtimecontract.RuntimeBlockKindPatchApply {
		t.Fatalf("scope admissible kinds = %v, want patch/apply only", kinds)
	}
}

func TestRuntimeV2CandidateRetryExplainsFileOwnerScopeRestriction(t *testing.T) {
	retry := runtimeV2CandidateRetry(1, &runtimecontract.RuntimeCandidateFailure{
		Class:  runtimecontract.RuntimeCandidateFailureClassContract,
		Code:   "candidate_target_outside_grounding",
		Reason: "owner-grounded workspace scope permits only a durable File operation with a concrete target",
	})
	if retry == nil {
		t.Fatal("retry projection is nil")
	}
	for _, want := range []string{"target-selection scope", "durable File-owner operation", "do not use Shell, Git, MCP, Web, or SSH"} {
		if !strings.Contains(retry.Instruction, want) {
			t.Fatalf("scope retry instruction lost %q: %s", want, retry.Instruction)
		}
	}
}

func TestRuntimeV2CandidateRetryExplainsSealedTargetAndClosedParameterContract(t *testing.T) {
	targetRetry := runtimeV2CandidateRetry(1, &runtimecontract.RuntimeCandidateFailure{
		Class: runtimecontract.RuntimeCandidateFailureClassContract, Code: "candidate_target_outside_grounding",
		Reason: "runtime state-changing candidate does not target the sealed focus, failure, or repair target set",
	})
	if targetRetry == nil {
		t.Fatal("sealed-target retry projection is nil")
	}
	for _, want := range []string{"sealed concrete target set", "context.resources.failureTarget", "top-level targetRefs", "build, test, read"} {
		if !strings.Contains(targetRetry.Instruction, want) {
			t.Fatalf("sealed-target retry instruction lost %q: %s", want, targetRetry.Instruction)
		}
	}
	parameterRetry := runtimeV2CandidateRetry(1, &runtimecontract.RuntimeCandidateFailure{
		Class:  runtimecontract.RuntimeCandidateFailureClassContract,
		Reason: `runtime candidate operation "op-1": kind "shell/command" does not accept parameter "summary"; allowed parameters: command, args`,
	})
	if parameterRetry == nil {
		t.Fatal("parameter retry projection is nil")
	}
	for _, want := range []string{"Remove every rejected parameter", "exact parameter names", "summary, narrative, explanation, metadata", "never a legal substitute"} {
		if !strings.Contains(parameterRetry.Instruction, want) {
			t.Fatalf("parameter retry instruction lost %q: %s", want, parameterRetry.Instruction)
		}
	}
}

func TestRuntimeV2CandidateSchemaKeepsExplicitSequenceDuringFileScopeNarrowing(t *testing.T) {
	selection := &runtimecontract.RuntimeTargetSelection{
		PrerequisiteID: "target-selection-scope", OwnerModuleID: runtimecontract.RuntimeModuleFile,
		TargetRole: "deliverable", SelectorRef: "file/workspace_deliverable",
		Disposition: runtimecontract.RuntimeTargetSelectionScope, ObservedRevision: "workspace-v1",
	}
	decision := runtimecontract.RuntimeDecision{
		Action: runtimecontract.RuntimeActionProduceStateChange,
		RequiredOutput: runtimecontract.RequiredOutputContract{
			AcceptedKinds: []string{runtimecontract.StepKindShellCommand, runtimecontract.RuntimeBlockKindPatchApply, runtimecontract.StepKindGitWrite},
			RequiredOperationSequence: []runtimecontract.RuntimeRequiredOperationVariant{{
				RequirementID: "initialize-repository", Kind: runtimecontract.StepKindGitWrite, VariantID: "init",
			}},
		},
	}
	kinds := runtimeV2CandidateAllowedOperationKinds(decision.Action, decision.RequiredOutput.AcceptedKinds, decision.RequiredOutput.RequiredOperationSequence, selection)
	if got, want := kinds, []string{runtimecontract.RuntimeBlockKindPatchApply, runtimecontract.StepKindGitWrite}; !slices.Equal(got, want) {
		t.Fatalf("scope kinds = %v, want %v", got, want)
	}
	schema := runtimeV2CandidateSchemaForDecisionWithTargetSelection(decision, selection)
	if strings.Contains(schema, `"shell/command"`) || !strings.Contains(schema, `"patch/apply"`) || !strings.Contains(schema, `"git/write"`) {
		t.Fatalf("scope sequence schema = %s", schema)
	}
}

func TestRuntimeV2ProviderRequestNarrowsFileScopeAcrossSchemaContextAndRetry(t *testing.T) {
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
				Target: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}, ContextWindowTokens: 128000,
				ProviderMaxOutputTokens: 16384, EffectiveMaxOutputTokens: 4096, MaxInputTokens: 92928, MaxBytes: 96 * 1024,
				CapabilityRevision: "capability-scope", ConfigurationRevision: "config-scope",
			},
		},
		Activation: runtimecontract.RuntimeActivationContext{ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}},
	}
	decision, err := runtimecontract.SealRuntimeDecision(facts, runtimecontract.RuntimeDecision{
		State: runtimecontract.ConvergenceStateNeedStateChange, Action: runtimecontract.RuntimeActionProduceStateChange, Focus: facts.Focus,
		RequiredOutput: runtimecontract.RequiredOutputContract{
			Kind:                 runtimecontract.RequiredOutputKindStateChange,
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
	request, err := runtimeV2ProviderRequest(llmmodels.RuntimeCandidateRequest{
		Decision: decision, Transaction: decision.AuthorityTransaction(), StablePrefixHash: prefix, DefinitionContext: definition,
		CandidateContext: runtimecontract.RuntimeCandidateContextForDecision(facts, decision, nil),
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{`"shell/command"`, `"git/write"`} {
		if strings.Contains(request.Schema, forbidden) {
			t.Fatalf("scope provider schema widened to %s: %s", forbidden, request.Schema)
		}
	}
	if !strings.Contains(request.Schema, `"enum":["patch/apply"]`) {
		t.Fatalf("scope provider schema did not retain durable File operation: %s", request.Schema)
	}
	dynamic := request.PromptContext.Sections[3].Content
	for _, required := range []string{`"admissibleOperationKinds":["patch/apply"]`, `"operationContracts":[{"kind":"patch/apply"`, "bounded workspace feasibility scope"} {
		if !strings.Contains(dynamic, required) {
			t.Fatalf("scope provider C3 lost %q: %s", required, dynamic)
		}
	}
	for _, forbidden := range []string{`"kind":"shell/command"`, `"kind":"git/write"`} {
		if strings.Contains(dynamic, forbidden) {
			t.Fatalf("scope provider C3 widened operation contracts to %s: %s", forbidden, dynamic)
		}
	}
	retryRequest, err := runtimeV2ProviderRequest(llmmodels.RuntimeCandidateRequest{
		Decision: decision, Transaction: decision.AuthorityTransaction(), CandidateAttempt: 1,
		PreviousFailure:  &runtimecontract.RuntimeCandidateFailure{Class: runtimecontract.RuntimeCandidateFailureClassContract, Code: "candidate_target_outside_grounding", Reason: "owner-grounded workspace scope permits only a durable File operation with a concrete target"},
		StablePrefixHash: prefix, DefinitionContext: definition, CandidateContext: runtimecontract.RuntimeCandidateContextForDecision(facts, decision, nil),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(retryRequest.PromptContext.Sections[3].Content, "do not use Shell, Git, MCP, Web, or SSH") {
		t.Fatalf("scope provider retry lost owner-bound correction: %s", retryRequest.PromptContext.Sections[3].Content)
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
	context := runtimecontract.RuntimeCandidateContextForDecision(facts, decision, nil)
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
