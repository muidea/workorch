package activationports

import (
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	mcpmodel "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/models"
	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	soulmodel "github.com/muidea/workorch/internal/modules/blocks/soul/pkg/models"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
	"github.com/muidea/workorch/internal/pkg/workorchmemorycontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
	"github.com/muidea/workorch/internal/pkg/workorchskillcontract"
)

func TestRuntimeActivationContextUsesConcreteSelectionSnapshots(t *testing.T) {
	defaultSoul := &soulmodel.Soul{ID: "soul-default", Revision: 1, ContentHash: "soul-default-hash", SourceProvenance: "test", Name: "default", SoulMD: "default instructions"}
	executionSoul := &soulmodel.Soul{ID: "soul-execution", Revision: 1, ContentHash: "soul-execution-hash", SourceProvenance: "test", Name: "execution", SoulMD: "execution instructions"}
	selectedSkill := workorchmanagementcontract.Skill{
		ID: "skill-1", Name: "skill", Version: "v1", Instruction: "skill instructions",
		Source: workorchskillcontract.SourceWorkOrchRegistry, Scope: workorchskillcontract.ScopeGlobal,
	}
	selectedSkill.ContentHash = workorchskillcontract.StableContentHash(workorchskillcontract.Content{
		ID: selectedSkill.ID, Name: selectedSkill.Name, Version: selectedSkill.Version, Instruction: selectedSkill.Instruction,
	})
	activation, err := RuntimeActivationContext(
		runtimecontract.ProviderInferenceSelection{Target: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}, ReasoningEffort: "high"},
		"zh-CN",
		defaultSoul,
		map[string]*soulmodel.Soul{"execution": executionSoul},
		appmodelcore.SoulRouting{Bindings: map[string]appmodelcore.SoulRoutingBinding{"execution": {SoulID: "soul-execution", Source: "workspace_stage_binding"}}},
		[]workorchmanagementcontract.Skill{selectedSkill},
		[]string{"mcp-1"},
		[]*mcpmodel.Capability{{
			ID: "cap-1", ServerID: "mcp-1", Kind: "tool", Name: "read", Enabled: true,
			EffectScope: "none", IdempotencyClass: "safe_to_replay",
		}},
		[]*memorymodel.MemoryItem{{ID: "memory-1", MemoryType: "fact", Summary: "memory summary", Content: "memory content"}},
		&MemorySelectionEvidence{
			Policy: "test_selection", Status: "selected", CandidateMemoryIDs: []string{"memory-1"},
			SelectedMemoryIDs: []string{"memory-1"}, RetrievedMemoryIDs: []string{"memory-1"},
			SelectionScores:     []workorchmemorycontract.RecallSelectionScore{{ID: "memory-1", Score: 1, Reason: "selected_within_context_budget"}},
			RetrievedMemoryHits: []MemorySelectionHit{{ID: "memory-1", Summary: "memory summary"}},
			ContextBudget:       workorchmemorycontract.RecallContextBudget{MaxItems: 8, MaxBytes: 16384, MaxEstimatedTokens: 4096, UsedItems: 1, UsedBytes: 32, EstimatedTokens: 8},
		},
		[]workorcheventpayload.Payload{workorcheventpayload.New(
			workorcheventpayload.StringField("id", "policy-1"),
			workorcheventpayload.StringField("appliesTo", "shell/command"),
			workorcheventpayload.StringField("commandName", "go"),
			workorcheventpayload.StringArrayField("requiredArgs", []string{"test", "-v"}),
			workorcheventpayload.BoolField("recipeDeclared", true),
			workorcheventpayload.StringArrayField("recipeArgs", []string{"test", "-v", "./..."}),
			workorcheventpayload.IntField("recipeTimeoutMillis", 60_000),
			workorcheventpayload.StringArrayField("zeroWorkMarkers", []string{"no tests"}),
			workorcheventpayload.StringField("qualityOnZeroWork", "inconclusive"),
			workorcheventpayload.Field{Name: "coverageRequirements", Value: workorcheventpayload.Value{Kind: workorcheventpayload.ValueArray, Array: []workorcheventpayload.Value{{Kind: workorcheventpayload.ValueObject, Object: workorcheventpayload.New(
				workorcheventpayload.StringField("id", "*"), workorcheventpayload.StringField("scope", "dod_item"),
				workorcheventpayload.StringField("targetRole", "verification_target"), workorcheventpayload.StringArrayField("targetRefs", []string{"runtime-validation-source.txt"}),
				workorcheventpayload.StringField("targetSelectorRef", "file/workspace_deliverable"), workorcheventpayload.StringField("targetSelectorOwnerRef", "software-dev"), workorcheventpayload.StringField("targetSelectorSubjectRef", "dod_item"),
				workorcheventpayload.StringArrayField("prospectiveTargetAllOfConstraintRefs", []string{"workspace-relative-path", "new-target-requires-owner-selector"}),
				workorcheventpayload.BoolField("requireTargetGrounding", true), workorcheventpayload.BoolField("requireCoverageTargets", true),
				workorcheventpayload.StringArrayField("acceptedStepKinds", []string{"validation/managed_session"}),
				workorcheventpayload.StringField("validationRecipeId", "policy-1"),
			).Fields}}}},
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
					"coverageDependencies":    []any{map[string]any{"id": "source-refreshes-validation", "sourceScope": "deliverable", "sourceRequirementId": "source", "dependentScope": "deliverable", "dependentRequirementId": "version_control_snapshot", "triggerEvidenceKinds": []any{"state_change"}}},
					"bootstrapRecipes": []any{map[string]any{
						"id": "go-empty-workspace", "requiredGoalTerms": []any{"Go"}, "requiredDoDTerms": []any{"test"},
						"artifacts": []any{map[string]any{"id": "module", "targetRole": "module_definition", "allowedOperationKinds": []any{"file/write"}}},
					}},
					"evidenceRequirements": []any{
						map[string]any{"id": "runtime_entrypoint", "scope": "acceptanceDimension", "requiredEvidenceKinds": []any{"validation"}, "acceptedStepKinds": []any{"validation/managed_session"}},
						map[string]any{
							"id": "version_control_snapshot", "scope": "deliverable", "evidenceRole": "supplemental_evidence", "requiredEvidenceKinds": []any{"state_change"},
							"targetRole": "repository", "targetRefs": []any{".git"}, "targetDiscoverySources": []any{"workspace_manifest"}, "targetSelectorRef": "file/unique_workspace_entry", "targetSelectorOwnerRef": "profile-owner", "targetSelectorSubjectRef": "profile-subject", "targetSelectorCandidateRefs": []any{"initial-deliverable.txt"}, "requireTargetGrounding": true,
							"requireDirectEvidence": true, "requireClaimEvidence": true, "requireExecutableBehaviorEvidence": true, "requireCoverageTargets": true,
							"acceptedStepKinds": []any{"git/write", "validation/managed_session"}, "acceptedOperationVariants": []any{map[string]any{"kind": "git/write", "variantId": "commit"}},
							"requiredOwnerFacts":    []any{map[string]any{"ownerModuleId": "blocks/shell", "factKind": "selectable_validation_surface", "operationKind": "validation/managed_session"}},
							"ownerFactConvergences": []any{map[string]any{"ownerModuleId": "blocks/shell", "factKind": "selectable_validation_surface", "operationKind": "validation/managed_session", "requirePreexisting": true}},
							"rejectedStepKinds":     []any{"shell/read"},
						},
					},
				},
			},
			"templateStagePolicies": map[string]any{
				"schemaVersion": "workorch.template_stage_policy",
				"stages": []any{map[string]any{
					"id": "execution", "soulBinding": "execution",
					"requiredOutputs": []any{map[string]any{"kind": "state_change", "evidenceKinds": []any{"state_change"}}},
					"completion":      map[string]any{"policy": "required_outputs_accepted"},
					"governance":      map[string]any{"decisionMode": "when_required", "approvalMode": "when_required"},
					"policyRefs":      map[string]any{"evidencePolicies": []any{"evidence-1"}, "capabilityUsagePolicies": []any{"usage-1"}},
				}},
			},
		},
		runtimecontract.RuntimeContinuationContext{},
	)
	if err != nil {
		t.Fatalf("RuntimeActivationContext: %v", err)
	}
	if err := activation.Validate(); err != nil {
		t.Fatal(err)
	}
	if activation.DefaultSoul == nil || activation.DefaultSoul.Instructions != "default instructions" || len(activation.StageSoulBindings) != 1 ||
		activation.StageSoulBindings[0].Soul.Instructions != "execution instructions" || len(activation.Skills) != 1 || len(activation.MCPCapabilities) != 1 || len(activation.Memories) != 1 || len(activation.EvidencePolicies) != 1 {
		t.Fatalf("runtime activation = %+v", activation)
	}
	if policy := activation.EvidencePolicies[0]; policy.CommandName != "go" || len(policy.RequiredArgs) != 2 || policy.RequiredArgs[0] != "-v" || policy.RequiredArgs[1] != "test" ||
		!policy.RecipeDeclared || policy.RecipeTimeoutMillis != 60_000 || len(policy.RecipeArgs) != 3 || policy.RecipeArgs[0] != "test" || policy.RecipeArgs[1] != "-v" || policy.RecipeArgs[2] != "./..." {
		t.Fatalf("runtime activation observable evidence recipe = %+v", policy)
	}
	if policy := activation.EvidencePolicies[0]; len(policy.CoverageRequirements) != 1 || policy.CoverageRequirements[0].TargetRole != "verification_target" ||
		len(policy.CoverageRequirements[0].TargetRefs) != 1 || policy.CoverageRequirements[0].TargetRefs[0] != "runtime-validation-source.txt" ||
		!runtimecontract.SameRuntimeProspectiveTargetAllOfConstraintRefs(policy.CoverageRequirements[0].ProspectiveTargetAllOfConstraintRefs, []string{"new-target-requires-owner-selector", "workspace-relative-path"}) ||
		!policy.CoverageRequirements[0].RequireTargetGrounding || !policy.CoverageRequirements[0].RequireCoverageTargets ||
		policy.CoverageRequirements[0].ValidationRecipeID != "policy-1" {
		t.Fatalf("runtime activation coverage declaration = %+v", policy.CoverageRequirements)
	}
	if capability := activation.MCPCapabilities[0]; capability.OperationKind != runtimecontract.StepKindMCPToolCall ||
		capability.EffectScope != string(runtimecontract.RuntimeEffectNone) ||
		capability.IdempotencyClass != string(runtimecontract.RuntimeOperationIdempotencySafeToReplay) {
		t.Fatalf("sealed runtime MCP capability = %+v, want explicit non-durable replay-safe tool metadata", capability)
	}
	if len(activation.OperationPolicies) != 1 || activation.OperationPolicies[0].OwnerModuleID != runtimecontract.RuntimeModuleMCP ||
		activation.OperationPolicies[0].OperationKind != runtimecontract.StepKindMCPToolCall ||
		activation.OperationPolicies[0].EffectScope != runtimecontract.RuntimeEffectNone ||
		activation.OperationPolicies[0].IdempotencyClass != runtimecontract.RuntimeOperationIdempotencySafeToReplay {
		t.Fatalf("sealed runtime MCP operation policies = %+v", activation.OperationPolicies)
	}
	if activation.ExecutionContract.StateChangePolicy != "must_produce_state_change" || activation.ExecutionContract.MaxRepairAttemptsPerFocus != 7 ||
		len(activation.ExecutionContract.CapabilityUsagePolicies) != 1 || len(activation.ExecutionContract.ValidationPolicies) != 1 ||
		activation.ExecutionContract.ValidationPolicies[0].MaxValidationOnlyRetriesPerFocus != 2 || len(activation.ExecutionContract.ProfileContracts) != 1 {
		t.Fatalf("runtime execution contract = %+v", activation.ExecutionContract)
	}
	profile := activation.ExecutionContract.ProfileContracts[0]
	var deliverableRequirement runtimecontract.RuntimeProfileEvidenceRequirement
	if len(profile.EvidenceRequirements) == 2 {
		deliverableRequirement = profile.EvidenceRequirements[1]
	}
	if profile.ID != "software-dev" || len(profile.AcceptanceDimensions) != 1 || profile.AcceptanceDimensions[0].ID != "runtime_entrypoint" || !profile.AcceptanceDimensions[0].Required ||
		len(profile.DeliverableExpectations) != 1 || profile.DeliverableExpectations[0].ID != "version_control_snapshot" || profile.DeliverableExpectations[0].EvidenceRole != "supplemental_evidence" ||
		len(profile.EvidenceRequirements) != 2 || deliverableRequirement.Scope != "deliverable" || deliverableRequirement.TargetRole != "repository" ||
		len(deliverableRequirement.TargetRefs) != 1 || deliverableRequirement.TargetRefs[0] != ".git" ||
		len(deliverableRequirement.TargetDiscoverySources) != 1 || deliverableRequirement.TargetDiscoverySources[0] != "workspace_manifest" || deliverableRequirement.TargetSelectorRef != "file/unique_workspace_entry" || deliverableRequirement.TargetSelectorOwnerRef != "profile-owner" || deliverableRequirement.TargetSelectorSubjectRef != "profile-subject" || !runtimecontract.SameRuntimeTargetSelectorCandidateRefs(deliverableRequirement.TargetSelectorCandidateRefs, []string{"initial-deliverable.txt"}) || !deliverableRequirement.RequireTargetGrounding || !deliverableRequirement.RequireDirectEvidence ||
		!deliverableRequirement.RequireClaimEvidence || !deliverableRequirement.RequireExecutableBehaviorEvidence || !deliverableRequirement.RequireCoverageTargets ||
		len(deliverableRequirement.AcceptedStepKinds) != 2 || deliverableRequirement.AcceptedStepKinds[0] != "git/write" || deliverableRequirement.AcceptedStepKinds[1] != "validation/managed_session" ||
		len(deliverableRequirement.AcceptedOperationVariants) != 1 || deliverableRequirement.AcceptedOperationVariants[0] != (runtimecontract.RuntimeOperationVariantRef{Kind: "git/write", VariantID: "commit"}) ||
		len(deliverableRequirement.RequiredOwnerFacts) != 1 || deliverableRequirement.RequiredOwnerFacts[0] != (runtimecontract.RuntimeOwnerFactRequirement{OwnerModuleID: runtimecontract.RuntimeModuleShell, FactKind: runtimecontract.RuntimeOwnerFactSelectableValidationSurface, OperationKind: runtimecontract.RuntimeBlockKindManagedValidationSession}) ||
		len(deliverableRequirement.RejectedStepKinds) != 1 || deliverableRequirement.RejectedStepKinds[0] != "shell/read" ||
		len(profile.CoverageDependencies) != 1 || profile.CoverageDependencies[0].ID != "source-refreshes-validation" || profile.CoverageDependencies[0].SourceRequirementID != "source" || profile.CoverageDependencies[0].DependentRequirementID != "version_control_snapshot" ||
		len(profile.CoverageDependencies[0].TriggerEvidenceKinds) != 1 || profile.CoverageDependencies[0].TriggerEvidenceKinds[0] != runtimecontract.EvidenceKindStateChange ||
		len(profile.BootstrapRecipes) != 1 || profile.BootstrapRecipes[0].ID != "go-empty-workspace" || len(profile.BootstrapRecipes[0].Artifacts) != 1 || profile.BootstrapRecipes[0].Artifacts[0].TargetRole != "module_definition" {
		t.Fatalf("runtime profile contract = %+v", profile)
	}
	stagePolicy := activation.ExecutionContract.TemplateStagePolicy
	if stagePolicy == nil || stagePolicy.SchemaVersion != "workorch.template_stage_policy" || len(stagePolicy.Stages) != 1 ||
		stagePolicy.Stages[0].ID != "execution" || stagePolicy.Stages[0].SoulBinding != "execution" || len(stagePolicy.Stages[0].RequiredOutputs) != 1 ||
		stagePolicy.Stages[0].RequiredOutputs[0].Kind != "state_change" || stagePolicy.Stages[0].Completion.Policy != "required_outputs_accepted" ||
		stagePolicy.Stages[0].Governance.DecisionMode != "when_required" || len(stagePolicy.Stages[0].PolicyRefs.EvidencePolicies) != 1 ||
		len(stagePolicy.Stages[0].PolicyRefs.CapabilityUsagePolicies) != 1 {
		t.Fatalf("runtime template stage policy = %+v", stagePolicy)
	}
}

func TestRuntimeActivationContextRejectsPresealExactProspectiveTarget(t *testing.T) {
	_, err := RuntimeActivationContext(
		runtimecontract.ProviderInferenceSelection{Target: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}},
		"", nil, nil, appmodelcore.SoulRouting{}, nil, nil, nil, nil, nil, nil,
		map[string]any{"profileContracts": map[string]any{"profile": map[string]any{"evidenceRequirements": []any{map[string]any{
			"id": "deliverable", "scope": "deliverable", "prospectiveTargetRef": "legacy-target.txt",
		}}}}},
		runtimecontract.RuntimeContinuationContext{},
	)
	if err == nil || !strings.Contains(err.Error(), "pre-seal execution contract rejects prospectiveTargetRef") {
		t.Fatalf("expected pre-seal exact target rejection, got %v", err)
	}
}

func TestRuntimeActivationContextRejectsBootstrapArtifactExactTarget(t *testing.T) {
	_, err := RuntimeActivationContext(
		runtimecontract.ProviderInferenceSelection{Target: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}},
		"", nil, nil, appmodelcore.SoulRouting{}, nil, nil, nil, nil, nil, nil,
		map[string]any{"profileContracts": map[string]any{"profile": map[string]any{"bootstrapRecipes": []any{map[string]any{
			"id": "bootstrap", "requiredGoalTerms": []any{"build"}, "artifacts": []any{map[string]any{
				"id": "source", "targetRole": "implementation", "TargetRef": "main.go", "allowedOperationKinds": []any{"file/write"},
			}},
		}}}}},
		runtimecontract.RuntimeContinuationContext{},
	)
	if err == nil || !strings.Contains(err.Error(), "pre-seal bootstrap artifact rejects targetRef") {
		t.Fatalf("expected bootstrap artifact exact target rejection, got %v", err)
	}
}

func TestRuntimeActivationContextBoundsSkillHubInstructionBeforeC3(t *testing.T) {
	instruction := strings.Repeat("界", runtimecontract.RuntimeSkillInstructionMaxBytesPerSkill)
	skill := workorchmanagementcontract.Skill{
		ID: "skillhub:global:large", Name: "large", Instruction: instruction,
		Source: workorchskillcontract.SourceSkillHub, Scope: workorchskillcontract.ScopeGlobal,
	}
	skill.ContentHash = workorchskillcontract.StableContentHash(workorchskillcontract.Content{ID: skill.ID, Name: skill.Name, Instruction: skill.Instruction})
	activation, err := RuntimeActivationContext(
		runtimecontract.ProviderInferenceSelection{Target: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}},
		"zh-CN", nil, nil, appmodelcore.SoulRouting{}, []workorchmanagementcontract.Skill{skill}, nil, nil, nil, nil, nil, nil,
		runtimecontract.RuntimeContinuationContext{},
	)
	if err != nil {
		t.Fatalf("RuntimeActivationContext: %v", err)
	}
	if len(activation.Skills) != 1 || !activation.Skills[0].InstructionTruncated ||
		activation.Skills[0].OriginalInstructionBytes != len(instruction) ||
		len(activation.Skills[0].Instruction) > runtimecontract.RuntimeSkillInstructionMaxBytesPerSkill ||
		activation.SkillContextBudget == nil || activation.SkillContextBudget.TruncatedSkills != 1 ||
		activation.Skills[0].Provenance.ContentHash != skill.ContentHash {
		t.Fatalf("bounded Skill activation = %+v budget=%+v", activation.Skills, activation.SkillContextBudget)
	}
}

func TestRuntimeActivationContextDetachesSoulMCPAndProfileSnapshots(t *testing.T) {
	defaultSoul := &soulmodel.Soul{
		ID: "soul-detached", Revision: 3, ContentHash: "soul-hash-3", SourceProvenance: "test",
		SoulMD: "sealed instructions", AllowedSkillRefs: []string{"skill-original"},
	}
	stageSoul := &soulmodel.Soul{
		ID: "soul-stage-detached", Revision: 4, ContentHash: "soul-stage-hash-4", SourceProvenance: "test",
		SoulMD: "sealed stage instructions", AllowedMCPServerRefs: []string{"server-original"},
	}
	serverIDs := []string{"server-original"}
	capability := &mcpmodel.Capability{
		ID: "cap-detached", ServerID: "server-original", Kind: "tool", Name: "read", Enabled: true,
		Tags: []string{"tag-original"}, StageHints: []string{"execution"}, EffectScope: "none", IdempotencyClass: "safe_to_replay",
	}
	acceptedKinds := []any{"mcp/tool_call"}
	executionContract := map[string]any{
		"profileContracts": map[string]any{
			"detached-profile": map[string]any{
				"evidenceRequirements": []any{map[string]any{
					"id": "proof", "scope": "deliverable", "evidenceRole": "primary_state_change",
					"acceptedStepKinds": acceptedKinds,
				}},
			},
		},
	}
	activation, err := RuntimeActivationContext(
		runtimecontract.ProviderInferenceSelection{Target: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "model"}},
		"en", defaultSoul, map[string]*soulmodel.Soul{"execution": stageSoul},
		appmodelcore.SoulRouting{Bindings: map[string]appmodelcore.SoulRoutingBinding{"execution": {SoulID: string(stageSoul.ID), Source: "workspace_stage_binding"}}},
		nil, serverIDs, []*mcpmodel.Capability{capability}, nil, nil, nil, executionContract,
		runtimecontract.RuntimeContinuationContext{},
	)
	if err != nil {
		t.Fatalf("RuntimeActivationContext: %v", err)
	}

	defaultSoul.AllowedSkillRefs[0] = "skill-registry-mutated"
	stageSoul.AllowedMCPServerRefs[0] = "server-registry-mutated"
	serverIDs[0] = "server-list-mutated"
	capability.Tags[0] = "tag-catalog-mutated"
	capability.StageHints[0] = "planning"
	acceptedKinds[0] = "file/write"
	if activation.DefaultSoul.AllowedSkillRefs[0] != "skill-original" ||
		activation.StageSoulBindings[0].Soul.AllowedMCPServerRefs[0] != "server-original" ||
		activation.MCPServerIDs[0] != "server-original" ||
		activation.MCPCapabilities[0].Tags[0] != "tag-original" ||
		activation.MCPCapabilities[0].StageHints[0] != "execution" ||
		activation.ExecutionContract.ProfileContracts[0].EvidenceRequirements[0].AcceptedStepKinds[0] != "mcp/tool_call" {
		t.Fatalf("activation snapshot changed after Registry/Catalog mutation: %+v", activation)
	}

	sealed := activation.Clone()
	activation.DefaultSoul.AllowedSkillRefs[0] = "activation-mutated"
	activation.MCPCapabilities[0].Tags[0] = "activation-capability-mutated"
	activation.ExecutionContract.ProfileContracts[0].EvidenceRequirements[0].AcceptedStepKinds[0] = "activation-profile-mutated"
	if sealed.DefaultSoul.AllowedSkillRefs[0] != "skill-original" || sealed.MCPCapabilities[0].Tags[0] != "tag-original" ||
		sealed.ExecutionContract.ProfileContracts[0].EvidenceRequirements[0].AcceptedStepKinds[0] != "mcp/tool_call" {
		t.Fatalf("sealed Runtime activation clone shares mutable DTO storage: %+v", sealed)
	}
}

func TestRuntimeDefinitionChangeImpactUsesConcreteSnapshot(t *testing.T) {
	impact := RuntimeDefinitionChangeImpact(map[string]any{
		"version": "workorch.definition_change_impact", "changed": true,
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
	if impact == nil || impact.Version != "workorch.definition_change_impact" || !impact.RequiredPreExecutionAssessment ||
		impact.GoalChange.EffectiveDefinitionID != "goal-2" || len(impact.GoalChange.PreviousDefinitionIDs) != 1 || !impact.GoalChange.AllEntriesConstrain ||
		len(impact.GoalChange.DefinitionRevisions) != 1 || impact.GoalChange.DefinitionRevisions[0].RevisionCount != 2 ||
		impact.DoDChange.EffectiveDefinitionID != "dod-2" || !impact.DoDChange.AllEntriesConstrain || impact.DoDChange.DefinitionRevisions[0].LastEventID != "event-2" {
		t.Fatalf("runtime definition change impact = %+v", impact)
	}
	if err := impact.Validate(); err != nil {
		t.Fatal(err)
	}
}
