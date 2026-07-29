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
	request := llmmodels.RuntimeCandidateRequest{LogicalRequestID: "candidate-request-1", RequestSequence: 3, PhysicalCallID: "physical-call-3"}
	if got := newProviderRequestID(request); got != request.PhysicalCallID {
		t.Fatalf("provider request ID = %q, want persisted physical call ID %q", got, request.PhysicalCallID)
	}
}

func TestRuntimeV2ProviderRequestRejectsUnsealedManifestBeforeProviderConfiguration(t *testing.T) {
	_, err := runtimeV2ProviderRequest(llmmodels.RuntimeCandidateRequest{})
	if err == nil || !strings.Contains(err.Error(), "runtime candidate contract manifest") {
		t.Fatalf("provider request error = %v, want manifest preflight failure", err)
	}
}

func runtimeCandidateManifestForRequest(t *testing.T, decision runtimecontract.RuntimeDecision, context runtimecontract.RuntimeCandidateContext) runtimecontract.RuntimeCandidateContractManifest {
	t.Helper()
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
	if decision.RequiredOutput.Kind == runtimecontract.RequiredOutputKindNone {
		switch decision.Action {
		case runtimecontract.RuntimeActionProduceStateChange:
			decision.RequiredOutput.Kind = runtimecontract.RequiredOutputKindStateChange
		case runtimecontract.RuntimeActionProduceValidation:
			decision.RequiredOutput.Kind = runtimecontract.RequiredOutputKindValidation
		case runtimecontract.RuntimeActionProduceAssessment:
			decision.RequiredOutput.Kind = runtimecontract.RequiredOutputKindAssessment
		case runtimecontract.RuntimeActionRequestGovernance:
			decision.RequiredOutput.Kind = runtimecontract.RequiredOutputKindGovernance
		}
	}
	manifest, err := runtimecontract.BuildRuntimeCandidateContractManifest(decision, runtimecontract.RuntimeCandidateManifestOwnerFacts{TargetSelection: selection})
	if err != nil {
		t.Fatal(err)
	}
	return manifest
}

func runtimeCandidateSchemaForDecision(t *testing.T, decision runtimecontract.RuntimeDecision, selection *runtimecontract.RuntimeTargetSelection) string {
	t.Helper()
	return runtimeV2CandidateSchemaForManifest(runtimeCandidateManifestForSchema(t, decision, selection))
}

func runtimeCandidateOutputContractForDecision(t *testing.T, decision runtimecontract.RuntimeDecision, selection *runtimecontract.RuntimeTargetSelection) runtimeCandidateOutputContractProjection {
	t.Helper()
	return runtimeV2CandidateOutputContractForManifest(runtimeCandidateManifestForSchema(t, decision, selection))
}

func runtimeCandidateContextManifest(action runtimecontract.RuntimeAction, kinds ...string) runtimecontract.RuntimeCandidateContractManifest {
	operations := make([]runtimecontract.RuntimeCandidateOperationManifest, 0, len(kinds))
	for _, kind := range kinds {
		operations = append(operations, runtimecontract.RuntimeCandidateOperationManifest{Kind: kind})
	}
	policy, _ := runtimecontract.RuntimeActionEffectPolicyFor(action)
	return runtimecontract.RuntimeCandidateContractManifest{Action: action, EffectPolicy: policy, Operations: operations}
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
			EvidencePolicies: []runtimecontract.RuntimeEvidencePolicy{{
				ID: "go-test-observable", AppliesTo: runtimecontract.StepKindShellCommand, CommandName: "go", RequiredArgs: []string{"test", "-v"},
			}},
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
	bindRuntimeCandidateTestSelection(&facts, selection)
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
		Operation:        runtimecontract.RuntimeOperation{ID: "rejected-edit", Kind: runtimecontract.StepKindFileEdit, TargetRefs: []string{rejectedPath}},
		FailureSignature: "rejected-shape-signature",
	}
	candidateContext := runtimecontract.RuntimeCandidateContextForDecision(facts, decision, []runtimecontract.RuntimeRejectedOperationShape{rejectedShape})
	if candidateContext.RequiredAction != decision.Action || !slices.Equal(candidateContext.RequiredEffectScopes, decision.RequiredOutput.RequiredEffectScopes) ||
		!slices.Equal(candidateContext.RequiredEvidenceKinds, decision.RequiredOutput.RequiredEvidenceKinds) {
		t.Fatalf("candidate context requirements = %+v, want sealed decision output %+v", candidateContext, decision.RequiredOutput)
	}
	manifest := runtimeCandidateManifestForRequest(t, decision, candidateContext)
	request, err := runtimeV2ProviderRequest(llmmodels.RuntimeCandidateRequest{
		Decision: decision, Transaction: decision.AuthorityTransaction(), StablePrefixHash: prefix,
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
	for _, value := range []string{"one JSON object", "context.outputContract is the sole structured-output contract", "manifestHash", "sole next-action authority", "completion-audit information only"} {
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
		`"outputContract"`, `"manifestHash"`, `"structuredOutputInstruction"`, `"$id":"workorch.runtime_candidate.state_change.v2"`, `"forbiddenTopLevelFields"`,
		`"operationContracts":[{"kind":"file/edit"`, `"name":"oldText"`, `"name":"newText"`, `"targetCardinality":"one"`,
		`"forbiddenParameters":["filePath","fileContent","path","targetRef","targetRefs"]`,
		`"RequirementID":"repo-init"`, `"capabilityStates"`, `"repo-state-hash"`, `"targetSelectionInstruction"`, "empty workspace creation scope", `sealed prospective target \"initial-deliverable.txt\"`, `"RequiredArgs":["-v","test"]`,
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
				if len(diagnostic.Objectives) == 0 {
					diagnostic.Objectives = runtimecontract.RuntimeCandidateCorrectionObjectivesForFailure(&testCase.failure)
				}
			}
			retryRequest, err := runtimeV2ProviderRequest(llmmodels.RuntimeCandidateRequest{
				Decision: decision, Transaction: decision.AuthorityTransaction(), CandidateAttempt: 1, PreviousFailure: &testCase.failure, StablePrefixHash: prefix,
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
	freshRequest, err := runtimeV2ProviderRequest(llmmodels.RuntimeCandidateRequest{
		Decision: decision, Transaction: decision.AuthorityTransaction(), StablePrefixHash: prefix,
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
		CandidateManifest: runtimeCandidateManifestForRequest(t, decision, limitedContext), DefinitionContext: definition, CandidateContext: limitedContext,
	}); err == nil || !strings.Contains(err.Error(), "provider ceiling") {
		t.Fatalf("runtimeV2ProviderRequest ceiling err = %v, want typed provider ceiling failure", err)
	}
}

func TestValidateRuntimeCandidateContextIntegrityRejectsProtectedContextLoss(t *testing.T) {
	definition := runtimecontract.RuntimeDefinitionContext{Revision: "definition-1", GoalID: "goal-1", DoDID: "dod-1"}
	context := runtimecontract.RuntimeCandidateContext{
		WorkspaceID: "ws-1", RunID: "run-1", DefinitionRevision: "definition-1", EvidenceRevision: runtimecontract.InitialRuntimeEvidenceRevision,
		Focus:          runtimecontract.CoverageFocus{ID: "focus-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}},
		RequiredAction: runtimecontract.RuntimeActionProduceStateChange,
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

func TestRuntimeV2CandidateExecutionTailOnlyRendersProviderSafeFacts(t *testing.T) {
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
		Activation: runtimecontract.RuntimeActivationContext{Continuation: runtimecontract.RuntimeContinuationContext{
			Replan: &runtimecontract.RuntimeReplanContinuation{PredecessorRunID: "run-prior", RecoveryAction: "raw-recovery-action-must-not-reach-c4", Reason: "raw continuation reason must not reach c4"},
		}},
	}
	tail := runtimeV2CandidateExecutionTailProjection(context, 0, nil, nil, runtimecontract.RuntimeCandidateContractManifest{})
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
	projection := runtimeV2CandidateContextProjection(context, runtimeCandidateContextManifest(runtimecontract.RuntimeActionProduceStateChange, runtimecontract.StepKindFileWrite))
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
	projection := runtimeV2CandidateContextProjection(context, runtimeCandidateContextManifest(runtimecontract.RuntimeActionProduceStateChange, runtimecontract.RuntimeBlockKindPatchApply))
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
	if !strings.Contains(stateChange, "Every operation") || strings.Contains(stateChange, "At least one operation") {
		t.Fatalf("state-change schema does not render the canonical all-durable effect policy: %s", stateChange)
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
	if !strings.Contains(catalog, "context.outputContract") || strings.Contains(catalog, "workorch.runtime_candidate.state_change.v2") ||
		strings.Contains(catalog, "dependsOn") || strings.Contains(catalog, "targetRefs") {
		t.Fatalf("stable catalog retained a second operation contract: %s", catalog)
	}
}

func TestRuntimeV2CandidateSchemaOnlyExposesDependenciesForSealedDependentSequence(t *testing.T) {
	withoutDependency := runtimeCandidateSchemaForDecision(t, runtimecontract.RuntimeDecision{
		Action:         runtimecontract.RuntimeActionProduceStateChange,
		RequiredOutput: runtimecontract.RequiredOutputContract{AcceptedKinds: []string{runtimecontract.StepKindFileWrite}},
	}, nil)
	if runtimeV2OperationSchemaHasProperty(t, withoutDependency, "dependsOn") {
		t.Fatalf("ordinary candidate schema exposed an unnecessary dependency field: %s", withoutDependency)
	}

	withDependency := runtimeCandidateSchemaForDecision(t, runtimecontract.RuntimeDecision{
		Action: runtimecontract.RuntimeActionProduceStateChange,
		RequiredOutput: runtimecontract.RequiredOutputContract{
			AcceptedKinds: []string{runtimecontract.StepKindGitWrite},
			RequiredOperationSequence: []runtimecontract.RuntimeRequiredOperationVariant{
				{RequirementID: "init", Kind: runtimecontract.StepKindGitWrite, VariantID: "init"},
				{RequirementID: "commit", Kind: runtimecontract.StepKindGitWrite, VariantID: "commit", DependsOnRequirementIDs: []string{"init"}},
			},
		},
	}, nil)
	if !runtimeV2OperationSchemaHasProperty(t, withDependency, "dependsOn") {
		t.Fatalf("dependent sequence schema omitted operation dependency field: %s", withDependency)
	}
}

func runtimeV2OperationSchemaHasProperty(t *testing.T, encoded, property string) bool {
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

func TestRuntimeV2CandidateOutputContractUsesActionSpecificForbiddenTopLevelFields(t *testing.T) {
	operation := runtimeCandidateOutputContractForDecision(t, runtimecontract.RuntimeDecision{
		Action:         runtimecontract.RuntimeActionProduceStateChange,
		RequiredOutput: runtimecontract.RequiredOutputContract{AcceptedKinds: []string{runtimecontract.StepKindFileWrite}},
	}, nil)
	if !containsString(operation.ForbiddenTopLevelFields, "dependsOn") || !containsString(operation.ForbiddenTopLevelFields, "parameters") {
		t.Fatalf("operation forbidden fields = %#v", operation.ForbiddenTopLevelFields)
	}
	assessment := runtimeCandidateOutputContractForDecision(t, runtimecontract.RuntimeDecision{Action: runtimecontract.RuntimeActionProduceAssessment}, nil)
	if containsString(assessment.ForbiddenTopLevelFields, "coverageWorkItemIds") {
		t.Fatalf("assessment forbids its required top-level field: %#v", assessment.ForbiddenTopLevelFields)
	}
	governance := runtimeCandidateOutputContractForDecision(t, runtimecontract.RuntimeDecision{Action: runtimecontract.RuntimeActionRequestGovernance}, nil)
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

func TestRuntimeV2CandidateSchemaDerivesPatchTargetRequirementsFromOperationContract(t *testing.T) {
	schema := runtimeCandidateSchemaForDecision(t, runtimecontract.RuntimeDecision{
		Action: runtimecontract.RuntimeActionProduceStateChange,
		RequiredOutput: runtimecontract.RequiredOutputContract{
			AcceptedKinds: []string{runtimecontract.RuntimeBlockKindPatchApply},
		},
	}, nil)
	for _, value := range []string{`"const":"patch/apply"`, `"const":"patchType"`, `"targetRefs"`, `"minItems":1`, `"maxItems":1`, `"minLength":1`} {
		if !strings.Contains(schema, value) {
			t.Fatalf("descriptor-driven patch schema lost %q: %s", value, schema)
		}
	}
	if strings.Contains(runtimeV2CandidateSchema(runtimecontract.RuntimeActionProduceStateChange), `"const":"patch/apply"`) {
		t.Fatal("volatile operation constraints leaked into the stable generic contract schema")
	}
}

func TestRuntimeV2CandidateSchemaRequiresCanonicalTargetRefsForDurableFileOperations(t *testing.T) {
	schema := runtimeCandidateSchemaForDecision(t, runtimecontract.RuntimeDecision{
		Action: runtimecontract.RuntimeActionProduceStateChange,
		RequiredOutput: runtimecontract.RequiredOutputContract{AcceptedKinds: []string{
			runtimecontract.StepKindFileWrite,
			runtimecontract.StepKindFileEdit,
		}},
	}, nil)
	for _, kind := range []string{runtimecontract.StepKindFileWrite, runtimecontract.StepKindFileEdit} {
		if !runtimeV2SchemaRequiresTopLevelTargetRefs(schema, kind) {
			t.Fatalf("%s schema must require canonical top-level targetRefs", kind)
		}
	}
}

func TestRuntimeV2CandidateSchemaClosesNamedParameterObjects(t *testing.T) {
	schema := runtimeCandidateSchemaForDecision(t, runtimecontract.RuntimeDecision{
		Action: runtimecontract.RuntimeActionProduceStateChange,
		RequiredOutput: runtimecontract.RequiredOutputContract{
			AcceptedKinds: []string{runtimecontract.RuntimeBlockKindPatchApply, runtimecontract.StepKindShellCommand},
		},
	}, nil)
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
	toolSchema := runtimeCandidateSchemaForDecision(t, runtimecontract.RuntimeDecision{
		Action:         runtimecontract.RuntimeActionProduceValidation,
		RequiredOutput: runtimecontract.RequiredOutputContract{AcceptedKinds: []string{runtimecontract.StepKindMCPToolCall}},
	}, nil)
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

	resourceSchema := runtimeCandidateSchemaForDecision(t, runtimecontract.RuntimeDecision{
		Action:         runtimecontract.RuntimeActionProduceValidation,
		RequiredOutput: runtimecontract.RequiredOutputContract{AcceptedKinds: []string{runtimecontract.StepKindMCPResourceRead}},
	}, nil)
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

func TestRuntimeV2CandidateSchemaNarrowsFileOwnerScopeToDurableFileAndDeclaredGroundedKinds(t *testing.T) {
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
	manifest := runtimeCandidateManifestForSchema(t, decision, selection)
	schema := runtimeV2CandidateSchemaForManifest(manifest)
	if !strings.Contains(schema, `"enum":["patch/apply","shell/command"]`) {
		t.Fatalf("scope schema = %s, want durable File and descriptor-declared grounded kind", schema)
	}
	for _, forbidden := range []string{`"git/write"`} {
		if strings.Contains(schema, forbidden) {
			t.Fatalf("scope schema admitted undeclared grounded kind %s: %s", forbidden, schema)
		}
	}
	output := runtimeV2CandidateOutputContractForManifest(manifest)
	if len(output.OperationContracts) != 2 || output.OperationContracts[0].Kind != runtimecontract.RuntimeBlockKindPatchApply || output.OperationContracts[1].Kind != runtimecontract.StepKindShellCommand {
		t.Fatalf("scope operation contracts = %#v, want patch/apply and shell/command", output.OperationContracts)
	}
	kinds := runtimeV2CandidateManifestOperationKinds(manifest)
	if got, want := kinds, []string{runtimecontract.RuntimeBlockKindPatchApply, runtimecontract.StepKindShellCommand}; !slices.Equal(got, want) {
		t.Fatalf("scope admissible kinds = %v, want %v", got, want)
	}
}

func TestRuntimeV2CandidateRendererProjectsSealedTargetPolicy(t *testing.T) {
	decision := runtimecontract.RuntimeDecision{Action: runtimecontract.RuntimeActionProduceStateChange, RequiredOutput: runtimecontract.RequiredOutputContract{
		Kind: runtimecontract.RequiredOutputKindStateChange, AcceptedKinds: []string{runtimecontract.StepKindFileEdit},
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
	output := runtimeV2CandidateOutputContractForManifest(manifest)
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

func TestRuntimeV2CandidateManifestDrivesSchemaInstructionAndRetryCorrection(t *testing.T) {
	cases := []struct {
		name      string
		decision  runtimecontract.RuntimeDecision
		contains  []string
		omits     []string
		dependsOn bool
	}{
		{
			name: "patch target and closed parameters",
			decision: runtimecontract.RuntimeDecision{Action: runtimecontract.RuntimeActionProduceStateChange, RequiredOutput: runtimecontract.RequiredOutputContract{
				Kind: runtimecontract.RequiredOutputKindStateChange, AcceptedKinds: []string{runtimecontract.RuntimeBlockKindPatchApply},
				AcceptedEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable}, RequiredEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable},
				CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, TargetRefs: []string{"main.go"},
			}},
			contains: []string{"patch/apply", "targetRefs cardinality=one", "targetRefs is required", "TargetPolicy=exact_target", "Allowed targets: main.go", "Forbidden parameters"},
		},
		{
			name: "shell summary remains forbidden",
			decision: runtimecontract.RuntimeDecision{Action: runtimecontract.RuntimeActionProduceValidation, RequiredOutput: runtimecontract.RequiredOutputContract{
				Kind: runtimecontract.RequiredOutputKindValidation, AcceptedKinds: []string{runtimecontract.StepKindShellCommand},
				AcceptedEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectNone}, RequiredEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectNone},
				CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
			}},
			contains: []string{"shell/command", "command:string"},
			omits:    []string{"summary"},
		},
		{
			name: "required sequence exposes operation-local dependency",
			decision: runtimecontract.RuntimeDecision{Action: runtimecontract.RuntimeActionProduceStateChange, RequiredOutput: runtimecontract.RequiredOutputContract{
				Kind: runtimecontract.RequiredOutputKindStateChange, AcceptedKinds: []string{runtimecontract.StepKindGitWrite},
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
			output := runtimeV2CandidateOutputContractForManifest(manifest)
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
			if runtimeV2OperationSchemaHasProperty(t, string(output.Schema), "dependsOn") != testCase.dependsOn {
				t.Fatalf("schema dependency exposure = %t, want %t: %s", runtimeV2OperationSchemaHasProperty(t, string(output.Schema), "dependsOn"), testCase.dependsOn, output.Schema)
			}
			retry := runtimeV2CandidateRetryForManifest(1, &runtimecontract.RuntimeCandidateFailure{Class: runtimecontract.RuntimeCandidateFailureClassContract, Reason: `arbitrary stale error text with parameter "summary"`}, manifest)
			if retry == nil || !strings.Contains(retry.Instruction, manifest.Hash) || !strings.Contains(retry.Instruction, "context.outputContract.structuredOutputInstruction") || strings.Contains(retry.Instruction, "arbitrary stale error text") {
				t.Fatalf("retry correction was not manifest-derived: %+v", retry)
			}
		})
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
	manifest := runtimeCandidateManifestForSchema(t, decision, selection)
	kinds := runtimeV2CandidateManifestOperationKinds(manifest)
	if got, want := kinds, []string{runtimecontract.StepKindGitWrite, runtimecontract.RuntimeBlockKindPatchApply, runtimecontract.StepKindShellCommand}; !slices.Equal(got, want) {
		t.Fatalf("scope kinds = %v, want %v", got, want)
	}
	schema := runtimeV2CandidateSchemaForManifest(manifest)
	if !strings.Contains(schema, `"shell/command"`) || !strings.Contains(schema, `"patch/apply"`) || !strings.Contains(schema, `"git/write"`) {
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
	bindRuntimeCandidateTestSelection(&facts, selection)
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
	retryRequest, err := runtimeV2ProviderRequest(llmmodels.RuntimeCandidateRequest{
		Decision: decision, Transaction: decision.AuthorityTransaction(), CandidateAttempt: 1,
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

func TestRuntimeV2CandidateSchemaRestrictsVariantsToSealedAuthority(t *testing.T) {
	patchSchema := runtimeCandidateSchemaForDecision(t, runtimecontract.RuntimeDecision{
		Action: runtimecontract.RuntimeActionProduceStateChange,
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
		Action: runtimecontract.RuntimeActionProduceStateChange,
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
	}, nil)
	for _, variant := range []string{"init", "add", "commit"} {
		if !strings.Contains(sequenceSchema, `"const":"`+variant+`"`) {
			t.Fatalf("required sequence schema lost variant %q: %s", variant, sequenceSchema)
		}
	}

	unknownVariantDecision := runtimecontract.RuntimeDecision{
		Action: runtimecontract.RuntimeActionProduceStateChange,
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
	projection := runtimeV2CandidateContextProjection(context, runtimeCandidateManifestForRequest(t, decision, context))
	wantRefs := runtimecontract.AcceptedEvidenceRefsForFocus(facts)
	if strings.Join(projection.AllowedEvidenceRefs, "|") != strings.Join(wantRefs, "|") || len(wantRefs) != 3 {
		t.Fatalf("allowed evidence refs = %+v, want %+v", projection.AllowedEvidenceRefs, wantRefs)
	}
	request, err := runtimeV2ProviderRequest(llmmodels.RuntimeCandidateRequest{
		Decision: decision, Transaction: decision.AuthorityTransaction(), StablePrefixHash: prefix,
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
	schema := runtimeV2CandidateSchema(runtimecontract.RuntimeActionProduceAssessment)
	for _, contract := range []string{`"required":["focusId","coverageWorkItemIds","disposition","summary"]`, `"const":"satisfied"`, `"required":["evidenceRefs"]`, `"minItems":1`} {
		if !strings.Contains(schema, contract) {
			t.Fatalf("assessment schema lost conditional evidence contract %q: %s", contract, schema)
		}
	}
	retryFailure := runtimecontract.RuntimeCandidateFailure{Class: runtimecontract.RuntimeCandidateFailureClassSchema, Reason: "missing evidenceRefs"}
	retry, err := runtimeV2ProviderRequest(llmmodels.RuntimeCandidateRequest{
		Decision: decision, Transaction: decision.AuthorityTransaction(), CandidateAttempt: 1, PreviousFailure: &retryFailure, StablePrefixHash: prefix,
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

func TestRuntimeV2CandidateContextPublishesExactRejectedShapes(t *testing.T) {
	path := "main.go"
	operation := runtimecontract.RuntimeOperation{ID: "validate-1", Kind: runtimecontract.StepKindFileRead, TargetRefs: []string{path}}
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
	projection := runtimeV2CandidateContextProjection(context, runtimeCandidateManifestForRequest(t, decision, context))
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
