package llm

import (
	"strings"
	"testing"

	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeV2ProviderRequestPreservesStablePrefixAndTypedContext(t *testing.T) {
	facts := runtimecontract.RuntimeFacts{
		WorkspaceID: "ws-1", RunID: "run-1", DefinitionRevision: "definition-1", EvidenceRevision: runtimecontract.InitialRuntimeEvidenceRevision,
		Focus: runtimecontract.CoverageFocus{ID: "focus-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}},
		Resources: runtimecontract.RuntimeResourceFacts{ProviderCeiling: &runtimecontract.ProviderCeiling{ProviderID: "provider-1", MaxBytes: 1 << 20, Revision: "config-1"}, TargetSnapshots: []runtimecontract.TargetSnapshot{{
			TargetRef: "main.go", Content: "package main", ContentHash: "hash-1", ContentState: runtimecontract.SnapshotContentStateComplete,
		}}},
		Activation: runtimecontract.RuntimeActivationContext{
			DialogLanguage: "zh-CN", Skills: []runtimecontract.RuntimeSkillContext{{ID: "skill-1", Instruction: "typed skill instructions"}},
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
					EvidenceRequirements:    []runtimecontract.RuntimeProfileEvidenceRequirement{{ID: "version_control_snapshot", Scope: "deliverable", AcceptedStepKinds: []string{"git/write"}, AcceptedOperations: []string{"commit"}}},
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
		RequiredOutput: runtimecontract.RequiredOutputContract{Kind: runtimecontract.RequiredOutputKindStateChange, AcceptedKinds: []string{"file/edit"}, CoverageWorkItemIDs: facts.Focus.CoverageWorkItemIDs},
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
	request, err := runtimeV2ProviderRequest(llmmodels.RuntimeCandidateRequest{
		Decision: decision, Transaction: decision.AuthorityTransaction(), StablePrefixHash: prefix,
		DefinitionContext: definition, CandidateContext: runtimecontract.RuntimeCandidateContextFromFacts(facts),
	})
	if err != nil {
		t.Fatal(err)
	}
	if request.ContextBudget == nil || request.ContextBudget.CachePlan == nil || request.ContextBudget.CachePlan.StablePrefixHash != prefix ||
		request.ContextBudget.CachePlan.RenderedStablePrefixHash != prefix || len(request.PromptContext.Sections) != 4 {
		t.Fatalf("provider request cache plan = %+v prompt=%+v", request.ContextBudget, request.PromptContext)
	}
	if request.PromptContext.Sections[3].Content == "" || request.Schema == "" {
		t.Fatalf("provider request lost dynamic context: %+v", request)
	}
	if !strings.Contains(request.PromptContext.Sections[1].Content, "identify additive constraints") || !strings.Contains(request.PromptContext.Sections[1].Content, "cumulative") {
		t.Fatalf("provider request lost typed definition change impact: %s", request.PromptContext.Sections[1].Content)
	}
	if !strings.Contains(request.PromptContext.Sections[3].Content, "typed skill instructions") || !strings.Contains(request.PromptContext.Sections[3].Content, "zh-CN") ||
		!strings.Contains(request.PromptContext.Sections[3].Content, "must_produce_state_change") || !strings.Contains(request.PromptContext.Sections[3].Content, "lsp_diagnostics") ||
		!strings.Contains(request.PromptContext.Sections[3].Content, "signature-old") || !strings.Contains(request.PromptContext.Sections[3].Content, "target-old") {
		t.Fatalf("provider request lost typed activation context: %s", request.PromptContext.Sections[3].Content)
	}
	capabilitySection := request.PromptContext.Sections[2].Content
	if strings.Contains(capabilitySection, "signature-old") || strings.Contains(capabilitySection, "target-old") {
		t.Fatalf("volatile continuation leaked into C2 stable prefix: %s", capabilitySection)
	}
	for _, value := range []string{"runtime_entrypoint", "version_control_snapshot", "git/write", "commit", "workorch.template_stage_policy.v1", "state_change"} {
		if !strings.Contains(capabilitySection, value) {
			t.Fatalf("provider request lost typed profile contract %q: %s", value, capabilitySection)
		}
	}
	limitedContext := runtimecontract.RuntimeCandidateContextFromFacts(facts)
	limitedContext.Resources.ProviderCeiling = &runtimecontract.ProviderCeiling{ProviderID: "provider-1", MaxBytes: 1, Revision: "config-1"}
	if _, err := runtimeV2ProviderRequest(llmmodels.RuntimeCandidateRequest{
		Decision: decision, Transaction: decision.AuthorityTransaction(), StablePrefixHash: prefix,
		DefinitionContext: definition, CandidateContext: limitedContext,
	}); err == nil || !strings.Contains(err.Error(), "provider ceiling") {
		t.Fatalf("runtimeV2ProviderRequest ceiling err = %v, want typed provider ceiling failure", err)
	}
}
