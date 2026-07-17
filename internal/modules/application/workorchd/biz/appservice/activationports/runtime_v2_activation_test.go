package activationports

import (
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistrycore"
	mcpmodel "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/models"
	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	soulmodel "github.com/muidea/workorch/internal/modules/blocks/soul/pkg/models"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeV2ActivationContextUsesConcreteSelectionSnapshots(t *testing.T) {
	defaultSoul := &soulmodel.Soul{ID: "soul-default", Name: "default", SoulMD: "default instructions"}
	executionSoul := &soulmodel.Soul{ID: "soul-execution", Name: "execution", SoulMD: "execution instructions"}
	activation := RuntimeV2ActivationContext(
		runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"},
		"zh-CN",
		defaultSoul,
		map[string]*soulmodel.Soul{"execution": executionSoul},
		appmodelcore.SoulRouting{Bindings: map[string]appmodelcore.SoulRoutingBinding{"execution": {SoulID: "soul-execution", Source: "workspace_stage_binding"}}},
		[]contextregistrycore.Skill{{ID: "skill-1", Name: "skill", Instruction: "skill instructions"}},
		[]string{"mcp-1"},
		[]*mcpmodel.Capability{{ID: "cap-1", ServerID: "mcp-1", Kind: "tool", Name: "read", Enabled: true}},
		[]*memorymodel.MemoryItem{{ID: "memory-1", MemoryType: "fact", Summary: "memory summary", Content: "memory content"}},
		[]workorcheventpayload.Payload{workorcheventpayload.New(
			workorcheventpayload.StringField("id", "policy-1"),
			workorcheventpayload.StringField("appliesTo", "shell/command"),
			workorcheventpayload.StringArrayField("zeroWorkMarkers", []string{"no tests"}),
			workorcheventpayload.StringField("qualityOnZeroWork", "inconclusive"),
		)},
		map[string]any{
			"stateChangePolicy":         "must_produce_state_change",
			"maxRepairAttemptsPerFocus": 7,
			"capabilityUsagePolicies":   []any{map[string]any{"id": "usage-1", "preferredTools": []any{"lsp_diagnostics"}}},
			"validationPolicies": []any{map[string]any{
				"id": "validation-1", "scope": "workspace_profile", "appliesToEvidenceKinds": []any{"validation"},
				"discoveryRequired": true, "validationScratchRequired": true, "maxValidationOnlyRetriesPerFocus": 2,
			}},
			"profileContracts": map[string]any{
				"software-dev": map[string]any{
					"acceptanceDimensions":    []any{map[string]any{"id": "runtime_entrypoint", "required": true, "description": "runtime entrypoint is usable"}},
					"deliverableExpectations": []any{map[string]any{"id": "version_control_snapshot", "required": true, "evidenceRole": "supplemental_evidence"}},
					"evidenceRequirements": []any{map[string]any{
						"id": "version_control_snapshot", "scope": "deliverable", "evidenceRole": "supplemental_evidence",
						"requireDirectEvidence": true, "requireClaimEvidence": true, "requireExecutableBehaviorEvidence": true, "requireCoverageTargets": true,
						"acceptedStepKinds": []any{"git/write"}, "acceptedOperations": []any{"commit"}, "rejectedStepKinds": []any{"shell/read"},
					}},
				},
			},
			"templateStagePolicies": map[string]any{
				"schemaVersion": "workorch.template_stage_policy.v1",
				"stages": []any{map[string]any{
					"id": "execution", "soulBinding": "execution",
					"entry":           map[string]any{"nextAction": "state_change", "requiresDefinitionRevision": true, "requiresGapBaseline": true},
					"requiredOutputs": []any{map[string]any{"kind": "state_change", "evidenceKinds": []any{"state_change"}}},
					"completion":      map[string]any{"policy": "required_outputs_accepted"},
					"governance":      map[string]any{"decisionMode": "when_required", "approvalMode": "when_required"},
					"policyRefs":      map[string]any{"evidencePolicies": []any{"evidence-1"}, "capabilityUsagePolicies": []any{"usage-1"}},
				}},
			},
		},
		runtimecontract.RuntimeContinuationContext{},
	)
	if err := activation.Validate(); err != nil {
		t.Fatal(err)
	}
	if activation.DefaultSoul == nil || activation.DefaultSoul.Instructions != "default instructions" || len(activation.StageSoulBindings) != 1 ||
		activation.StageSoulBindings[0].Soul.Instructions != "execution instructions" || len(activation.Skills) != 1 || len(activation.MCPCapabilities) != 1 || len(activation.Memories) != 1 || len(activation.EvidencePolicies) != 1 {
		t.Fatalf("runtime activation = %+v", activation)
	}
	if activation.ExecutionContract.StateChangePolicy != "must_produce_state_change" || activation.ExecutionContract.MaxRepairAttemptsPerFocus != 7 ||
		len(activation.ExecutionContract.CapabilityUsagePolicies) != 1 || len(activation.ExecutionContract.ValidationPolicies) != 1 ||
		activation.ExecutionContract.ValidationPolicies[0].MaxValidationOnlyRetriesPerFocus != 2 || len(activation.ExecutionContract.ProfileContracts) != 1 {
		t.Fatalf("runtime execution contract = %+v", activation.ExecutionContract)
	}
	profile := activation.ExecutionContract.ProfileContracts[0]
	if profile.ID != "software-dev" || len(profile.AcceptanceDimensions) != 1 || profile.AcceptanceDimensions[0].ID != "runtime_entrypoint" || !profile.AcceptanceDimensions[0].Required ||
		len(profile.DeliverableExpectations) != 1 || profile.DeliverableExpectations[0].ID != "version_control_snapshot" || profile.DeliverableExpectations[0].EvidenceRole != "supplemental_evidence" ||
		len(profile.EvidenceRequirements) != 1 || profile.EvidenceRequirements[0].Scope != "deliverable" || !profile.EvidenceRequirements[0].RequireDirectEvidence ||
		!profile.EvidenceRequirements[0].RequireClaimEvidence || !profile.EvidenceRequirements[0].RequireExecutableBehaviorEvidence || !profile.EvidenceRequirements[0].RequireCoverageTargets ||
		len(profile.EvidenceRequirements[0].AcceptedStepKinds) != 1 || profile.EvidenceRequirements[0].AcceptedStepKinds[0] != "git/write" ||
		len(profile.EvidenceRequirements[0].AcceptedOperations) != 1 || profile.EvidenceRequirements[0].AcceptedOperations[0] != "commit" ||
		len(profile.EvidenceRequirements[0].RejectedStepKinds) != 1 || profile.EvidenceRequirements[0].RejectedStepKinds[0] != "shell/read" {
		t.Fatalf("runtime profile contract = %+v", profile)
	}
	stagePolicy := activation.ExecutionContract.TemplateStagePolicy
	if stagePolicy == nil || stagePolicy.SchemaVersion != "workorch.template_stage_policy.v1" || len(stagePolicy.Stages) != 1 ||
		stagePolicy.Stages[0].ID != "execution" || stagePolicy.Stages[0].SoulBinding != "execution" || stagePolicy.Stages[0].Entry.NextAction != "state_change" ||
		!stagePolicy.Stages[0].Entry.RequiresDefinitionRevision || !stagePolicy.Stages[0].Entry.RequiresGapBaseline || len(stagePolicy.Stages[0].RequiredOutputs) != 1 ||
		stagePolicy.Stages[0].RequiredOutputs[0].Kind != "state_change" || stagePolicy.Stages[0].Completion.Policy != "required_outputs_accepted" ||
		stagePolicy.Stages[0].Governance.DecisionMode != "when_required" || len(stagePolicy.Stages[0].PolicyRefs.EvidencePolicies) != 1 ||
		len(stagePolicy.Stages[0].PolicyRefs.CapabilityUsagePolicies) != 1 {
		t.Fatalf("runtime template stage policy = %+v", stagePolicy)
	}
}

func TestRuntimeV2DefinitionChangeImpactUsesConcreteSnapshot(t *testing.T) {
	impact := RuntimeV2DefinitionChangeImpact(map[string]any{
		"version": "workorch.definition_change_impact.v1", "changed": true,
		"policy": "workspace_goal_dod_cumulative_latest_refines_previous", "requiredPreExecutionAssessment": true,
		"goalChange": map[string]any{
			"effectiveGoalId": "goal-2", "latestGoalId": "goal-2", "previousGoalIds": []any{"goal-1"}, "allGoalIds": []any{"goal-1", "goal-2"},
			"changedInPlace": true, "allGoalsAreConstraints": true,
			"definitionRevisions": []any{map[string]any{"id": "goal-2", "revisionCount": 2, "source": "current_definition_model", "previousDefinitionHash": "hash-1"}},
		},
		"doDChange": map[string]any{
			"effectiveDoDId": "dod-2", "latestDoDId": "dod-2", "previousDoDIds": []any{"dod-1"}, "allDoDIds": []any{"dod-1", "dod-2"},
			"allDoDItemsAreAcceptanceGates": true,
			"definitionRevisions":           []any{map[string]any{"id": "dod-2", "revisionCount": float64(2), "lastEventId": "event-2", "lastOccurredAt": "2026-07-13T00:00:00Z"}},
		},
		"strategyRequirements": []any{"preserve prior constraints", "identify conflicts"},
	})
	if impact == nil || impact.Version != "workorch.definition_change_impact.v1" || !impact.RequiredPreExecutionAssessment ||
		impact.GoalChange.EffectiveDefinitionID != "goal-2" || len(impact.GoalChange.PreviousDefinitionIDs) != 1 || !impact.GoalChange.AllEntriesConstrain ||
		len(impact.GoalChange.DefinitionRevisions) != 1 || impact.GoalChange.DefinitionRevisions[0].RevisionCount != 2 ||
		impact.DoDChange.EffectiveDefinitionID != "dod-2" || !impact.DoDChange.AllEntriesConstrain || impact.DoDChange.DefinitionRevisions[0].LastEventID != "event-2" {
		t.Fatalf("runtime definition change impact = %+v", impact)
	}
	if err := impact.Validate(); err != nil {
		t.Fatal(err)
	}
}
