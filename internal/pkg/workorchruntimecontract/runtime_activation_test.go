package workorchruntimecontract

import (
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchmemorycontract"
	"github.com/muidea/workorch/internal/pkg/workorchskillcontract"
)

func TestRuntimeActivationContextCloneAndEqualityPreserveTypedSelections(t *testing.T) {
	skills, skillBudget := sealedRuntimeSkillFixture(t, "skill-go", "执行 Go 验证")
	activation := RuntimeActivationContext{
		ProviderSelection:  ProviderInferenceSelection{Target: ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}, ReasoningEffort: "high"},
		DialogLanguage:     "zh-CN",
		DefaultSoul:        &RuntimeSoulContext{ID: "soul-default", Name: "默认 Soul", Instructions: "遵守当前阶段合同", AllowedToolKinds: []string{"shell/command"}},
		StageSoulBindings:  []RuntimeStageSoulBinding{{Stage: "execution", Source: "workspace_stage_binding", Soul: RuntimeSoulContext{ID: "soul-execution", Instructions: "执行阶段指令"}}},
		Skills:             skills,
		SkillContextBudget: skillBudget,
		MCPServerIDs:       []common.MCPServerID{"mcp-main"},
		MCPCapabilities: []RuntimeMCPCapabilityContext{{
			ID: "cap-read", ServerID: "mcp-main", Kind: "tool", OperationKind: StepKindMCPToolCall, Name: "read", Enabled: true,
			EffectScope: string(RuntimeEffectNone), IdempotencyClass: string(RuntimeOperationIdempotencySafeToReplay),
		}},
		Memories: []RuntimeMemoryContext{{ID: "memory-1", MemoryType: "fact", Summary: "历史约束", Content: "保持 typed contract"}},
		MemorySelection: &workorchmemorycontract.RecallSelectionEvidence{
			Policy: "test_selection", Status: "selected", CandidateMemoryIDs: []common.MemoryID{"memory-1"},
			SelectedMemoryIDs: []common.MemoryID{"memory-1"}, RetrievedMemoryIDs: []common.MemoryID{"memory-1"},
			SelectionScores:     []workorchmemorycontract.RecallSelectionScore{{ID: "memory-1", Score: 1, Reason: "selected_within_context_budget"}},
			RetrievedMemoryHits: []workorchmemorycontract.RecallHit{{ID: "memory-1", Summary: "历史约束"}},
			ContextBudget:       workorchmemorycontract.RecallContextBudget{MaxItems: 8, MaxBytes: 16384, MaxEstimatedTokens: 4096, UsedItems: 1, UsedBytes: 32, EstimatedTokens: 8},
		},
		EvidencePolicies: []RuntimeEvidencePolicy{{ID: "policy-1", AppliesTo: "shell/command", ZeroWorkMarkers: []string{"no tests"}, QualityOnZeroWork: "inconclusive"}},
		Continuation: RuntimeContinuationContext{
			GovernanceHistory: []RuntimeGovernanceHistoryEntry{{
				TicketID: "ticket-1", RunID: "run-old", StepID: "step-old", ResolutionID: "resolution-1", ResolutionType: "replan", RecoveryAction: "replan",
				Failure: &RuntimeContinuationFailure{Class: "validation", Signature: "sig-1", TargetRefs: []string{"target-a"}},
			}},
		},
		ExecutionContract: RuntimeExecutionContract{
			StateChangePolicy:       RuntimeStateChangeMustProduce,
			CapabilityUsagePolicies: []RuntimeCapabilityUsagePolicy{{ID: "usage-1", PreferredTools: []string{"lsp_diagnostics"}}},
			ProfileContracts: []RuntimeProfileContract{{
				ID:                      "software-dev",
				AcceptanceDimensions:    []RuntimeAcceptanceDimension{{ID: "runtime_entrypoint", Required: true}},
				DeliverableExpectations: []RuntimeDeliverableExpectation{{ID: "version_control_snapshot", Required: true, EvidenceRole: "supplemental_evidence"}},
				EvidenceRequirements:    []RuntimeProfileEvidenceRequirement{{ID: "version_control_snapshot", Scope: "deliverable", AcceptedStepKinds: []string{"git/write"}, AcceptedOperationVariants: []RuntimeOperationVariantRef{{Kind: "git/write", VariantID: "commit"}}}},
			}},
			TemplateStagePolicy: &RuntimeTemplateStagePolicy{SchemaVersion: RuntimeTemplateStagePolicySchemaCurrent, Stages: []RuntimeTemplateStage{{
				ID: "execution", SoulBinding: "execution",
				RequiredOutputs: []RuntimeTemplateStageRequiredOutput{{Kind: "state_change", EvidenceKinds: []string{"state_change"}}},
			}}},
		},
	}
	if err := activation.Validate(); err != nil {
		t.Fatal(err)
	}
	cloned := activation.Clone()
	if !SameRuntimeActivationContext(activation, cloned) {
		t.Fatalf("activation clone drifted: source=%+v clone=%+v", activation, cloned)
	}
	cloned.ProviderSelection.ReasoningEffort = "low"
	if SameRuntimeActivationContext(activation, cloned) {
		t.Fatal("provider reasoning effort drift was not detected")
	}
	cloned = activation.Clone()
	cloned.Skills[0].Instruction = "mutated"
	if SameRuntimeActivationContext(activation, cloned) {
		t.Fatal("skill instruction drift was not detected")
	}
	cloned = activation.Clone()
	cloned.ExecutionContract.CapabilityUsagePolicies[0].UsageMode = "required"
	if SameRuntimeActivationContext(activation, cloned) {
		t.Fatal("execution contract drift was not detected")
	}
	cloned = activation.Clone()
	cloned.ExecutionContract.ProfileContracts[0].EvidenceRequirements[0].AcceptedOperationVariants[0].VariantID = "push"
	if SameRuntimeActivationContext(activation, cloned) {
		t.Fatal("profile contract drift was not detected")
	}
	cloned = activation.Clone()
	cloned.ExecutionContract.TemplateStagePolicy.Stages[0].RequiredOutputs[0].Kind = "validation"
	if SameRuntimeActivationContext(activation, cloned) {
		t.Fatal("template stage policy drift was not detected")
	}
	cloned = activation.Clone()
	cloned.Continuation.GovernanceHistory[0].Failure.TargetRefs[0] = "mutated"
	if SameRuntimeActivationContext(activation, cloned) {
		t.Fatal("continuation drift was not detected")
	}
	for name, mutate := range map[string]func(*RuntimeActivationContext){
		"operation kind": func(value *RuntimeActivationContext) {
			value.MCPCapabilities[0].OperationKind = StepKindMCPResourceRead
		},
		"effect scope": func(value *RuntimeActivationContext) {
			value.MCPCapabilities[0].EffectScope = string(RuntimeEffectEphemeralSession)
		},
		"idempotency": func(value *RuntimeActivationContext) {
			value.MCPCapabilities[0].IdempotencyClass = string(RuntimeOperationIdempotencyAtMostOnce)
		},
	} {
		t.Run(name, func(t *testing.T) {
			changed := activation.Clone()
			mutate(&changed)
			if SameRuntimeActivationContext(activation, changed) {
				t.Fatalf("MCP %s drift was not detected", name)
			}
		})
	}
}

func sealedRuntimeSkillFixture(t *testing.T, id, instruction string) ([]RuntimeSkillContext, *RuntimeSkillContextBudget) {
	t.Helper()
	contentHash := workorchskillcontract.StableContentHash(workorchskillcontract.Content{ID: id, Name: "Go", Instruction: instruction, RequiredToolKinds: []string{"shell/command"}})
	items, budget, err := SealRuntimeSkillContexts([]RuntimeSkillContext{{
		ID: id, Name: "Go", Instruction: instruction, RequiredToolKinds: []string{"shell/command"},
		Provenance: workorchskillcontract.Provenance{Source: workorchskillcontract.SourceWorkOrchRegistry, Scope: workorchskillcontract.ScopeGlobal, ContentHash: contentHash},
	}})
	if err != nil {
		t.Fatalf("seal runtime Skill fixture: %v", err)
	}
	return items, budget
}

func TestRuntimeContinuationContextRetainsBoundedGovernanceHistoryAfterDefinitionChange(t *testing.T) {
	context := RuntimeContinuationContext{
		DefinitionChangeSupersedesPrior: true,
		GovernanceHistory: []RuntimeGovernanceHistoryEntry{{
			TicketID: "ticket-1", RunID: "run-old", ResolutionID: "resolution-1", ResolutionType: "replan",
		}},
	}.Normalized()
	if len(context.GovernanceHistory) != 1 {
		t.Fatalf("continuation = %+v, want bounded governance history retained", context)
	}
}
