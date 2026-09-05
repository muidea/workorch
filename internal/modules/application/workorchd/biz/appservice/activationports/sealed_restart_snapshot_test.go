package activationports

import (
	"context"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	mcpmodel "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/models"
	runbiz "github.com/muidea/workorch/internal/modules/blocks/run/biz"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	soulmodel "github.com/muidea/workorch/internal/modules/blocks/soul/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
	"github.com/muidea/workorch/internal/pkg/workorchrepo"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
	"github.com/muidea/workorch/internal/pkg/workorchskillcontract"
)

func TestRestartSealsSoulMCPAndProfileSnapshotsAcrossOwnerRestart(t *testing.T) {
	defaultSoul := &soulmodel.Soul{
		ID: "soul-restart", Revision: 3, ContentHash: "soul-hash-3", SourceProvenance: "registry",
		SoulMD: "sealed instructions", AllowedSkillRefs: []string{"skill-original"},
	}
	stageSoul := &soulmodel.Soul{
		ID: "soul-stage-restart", Revision: 4, ContentHash: "soul-stage-hash-4", SourceProvenance: "registry",
		SoulMD: "sealed stage instructions", AllowedMCPServerRefs: []string{"server-original"},
	}
	serverIDs := []string{"server-original"}
	capability := &mcpmodel.Capability{
		ID: "cap-restart", ServerID: "server-original", Kind: "tool", Name: "read", Enabled: true,
		Tags: []string{"tag-original"}, StageHints: []string{"execution"}, EffectScope: "none", IdempotencyClass: "safe_to_replay",
	}
	selectedSkills := []workorchmanagementcontract.Skill{
		{
			ID: "registry-skill", Name: "registry skill", Version: "v1", Instruction: "registry instruction",
			TriggerRules: []string{"registry-original"}, Source: workorchskillcontract.SourceWorkOrchRegistry, Scope: workorchskillcontract.ScopeGlobal,
		},
		{
			ID: "skillhub:workspace:catalog-skill", Name: "catalog skill", Version: "v1", Instruction: "catalog instruction",
			TriggerRules: []string{"catalog-original"}, Source: workorchskillcontract.SourceSkillHub, Scope: workorchskillcontract.ScopeWorkspace,
			WorkspaceID: "ws-sealed-restart",
		},
	}
	for idx := range selectedSkills {
		skill := &selectedSkills[idx]
		skill.ContentHash = workorchskillcontract.StableContentHash(workorchskillcontract.Content{
			ID: skill.ID, Name: skill.Name, Version: skill.Version, Instruction: skill.Instruction, TriggerRules: skill.TriggerRules,
		})
	}
	acceptedKinds := []any{"mcp/tool_call"}
	executionContract := map[string]any{
		"stateChangePolicy": "allowed_but_not_required",
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
		selectedSkills, serverIDs, []*mcpmodel.Capability{capability}, nil, nil, nil, executionContract,
		runtimecontract.RuntimeContinuationContext{},
	)
	if err != nil {
		t.Fatalf("build Runtime activation: %v", err)
	}

	bootstrap := sealedRestartSnapshotBootstrap(t, activation)
	replayBootstrap := bootstrap.Clone()
	command := runevents.WorkspaceExecutionIntentCommand{
		WorkspaceID: "ws-sealed-restart", RequestID: "restart-sealed-snapshot", Kind: runtimecontract.RuntimeExecutionIntentRestart,
		GoalID: "goal-1", DoDID: "dod-1", DefinitionRevision: "def-1", StartupInputHash: "startup-input-sealed",
		Bootstrap: &bootstrap,
	}
	dataRoot := t.TempDir()
	firstOwner, firstHub := startSealedSnapshotRunOwner(t, dataRoot)
	first := sendSealedSnapshotRestart(t, firstHub, command)
	stopSealedSnapshotRunOwner(t, firstOwner, firstHub)

	// Simulate Registry/Catalog DTO reuse and caller mutation after the restart
	// command has been admitted. None of these mutable sources may remain aliased
	// with the Run owner's sealed bootstrap.
	defaultSoul.SoulMD = "registry-updated instructions"
	defaultSoul.AllowedSkillRefs[0] = "skill-registry-updated"
	stageSoul.AllowedMCPServerRefs[0] = "server-registry-updated"
	serverIDs[0] = "server-list-updated"
	capability.Tags[0] = "tag-catalog-updated"
	capability.StageHints[0] = "planning"
	selectedSkills[0].Instruction = "registry-updated instruction"
	selectedSkills[0].TriggerRules[0] = "registry-updated"
	selectedSkills[1].Instruction = "catalog-updated instruction"
	selectedSkills[1].TriggerRules[0] = "catalog-updated"
	acceptedKinds[0] = "file/write"
	bootstrap.Activation.DefaultSoul.AllowedSkillRefs[0] = "caller-mutated"
	bootstrap.Activation.Skills[0].Instruction = "caller-skill-mutated"
	bootstrap.Activation.MCPCapabilities[0].Tags[0] = "caller-capability-mutated"
	bootstrap.Activation.ExecutionContract.ProfileContracts[0].EvidenceRequirements[0].AcceptedStepKinds[0] = "caller/profile-mutated"

	secondOwner, secondHub := startSealedSnapshotRunOwner(t, dataRoot)
	t.Cleanup(func() { stopSealedSnapshotRunOwner(t, secondOwner, secondHub) })
	replay := command
	replay.Bootstrap = &replayBootstrap
	duplicate := sendSealedSnapshotRestart(t, secondHub, replay)
	if duplicate.RunID != first.RunID || duplicate.Outcome != runtimecontract.RuntimeExecutionIntentReconciliationRequested || duplicate.Reason != runtimecontract.RuntimeExecutionIntentReasonCurrentRunContinuationAccepted {
		t.Fatalf("restart replay = %+v, first=%+v", duplicate, first)
	}

	stored, err := workorchrepo.NewWorkspaceScopedFileRepositorySet(dataRoot).RunOwner.Runs().GetByWorkspaceID(
		common.ContextWithRuntimeWorkspaceID(context.Background(), command.WorkspaceID), command.WorkspaceID, first.RunID,
	)
	if err != nil {
		t.Fatalf("read sealed Run after owner restart: %v", err)
	}
	if stored.RuntimeBootstrap == nil ||
		stored.RuntimeBootstrap.Activation.DefaultSoul.Instructions != "sealed instructions" ||
		stored.RuntimeBootstrap.Activation.DefaultSoul.AllowedSkillRefs[0] != "skill-original" ||
		stored.RuntimeBootstrap.Activation.StageSoulBindings[0].Soul.AllowedMCPServerRefs[0] != "server-original" ||
		stored.RuntimeBootstrap.Activation.MCPServerIDs[0] != "server-original" ||
		stored.RuntimeBootstrap.Activation.MCPCapabilities[0].Tags[0] != "tag-original" ||
		stored.RuntimeBootstrap.Activation.MCPCapabilities[0].StageHints[0] != "execution" ||
		stored.RuntimeBootstrap.Activation.ExecutionContract.ProfileContracts[0].EvidenceRequirements[0].AcceptedStepKinds[0] != "mcp/tool_call" {
		t.Fatalf("sealed activation drifted after owner restart: %+v", stored.RuntimeBootstrap)
	}
	sealedSkills := make(map[string]runtimecontract.RuntimeSkillContext, len(stored.RuntimeBootstrap.Activation.Skills))
	for _, skill := range stored.RuntimeBootstrap.Activation.Skills {
		sealedSkills[skill.ID] = skill
	}
	if sealedSkills["registry-skill"].Instruction != "registry instruction" ||
		sealedSkills["registry-skill"].TriggerRules[0] != "registry-original" ||
		sealedSkills["skillhub:workspace:catalog-skill"].Instruction != "catalog instruction" ||
		sealedSkills["skillhub:workspace:catalog-skill"].TriggerRules[0] != "catalog-original" {
		t.Fatalf("sealed Skill snapshots drifted after Registry/Catalog updates: %+v", sealedSkills)
	}
}

func sealedRestartSnapshotBootstrap(t *testing.T, activation runtimecontract.RuntimeActivationContext) runtimecontract.RuntimeBootstrap {
	t.Helper()
	definition := runtimecontract.RuntimeDefinitionContext{
		StartupInputHash: "startup-input-sealed", Revision: "def-1", GoalID: "goal-1", GoalDefinition: "finish goal",
		DoDID: "dod-1", DoDDefinition: "produce evidence", CapabilityContract: []string{"mcp/tool_call"},
	}
	item := runtimecontract.RuntimeCoverageItem{ID: "cwi-1", SliceID: "slice-1", SourceScope: "dod", SourceID: "dod-1", TargetText: "produce evidence", Order: 1}
	slice := runtimecontract.RuntimeAcceptanceSlice{
		ID: "slice-1", Title: "proof", Scope: "dod", Priority: 1, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
		RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}, CompletionPolicy: runtimecontract.RuntimeAcceptanceSliceCompletionAll,
		ResultPolicy: runtimecontract.RuntimeAcceptanceSliceResultAllowed,
	}
	seed, err := runtimecontract.CompileRuntimeDefinitionSeed(definition, activation, []runtimecontract.RuntimeCoverageItem{item}, []runtimecontract.RuntimeAcceptanceSlice{slice})
	if err != nil {
		t.Fatalf("compile sealed definition seed: %v", err)
	}
	bootstrap := runtimecontract.RuntimeBootstrap{
		Definition: definition, Activation: activation, DefinitionSeed: seed,
		DefinitionFocuses: []runtimecontract.RuntimeDefinitionFocus{{
			ID: "focus-1", SliceID: "slice-1",
			AssessmentScope: item.SourceScope, AssessmentID: item.SourceID, AssessmentText: item.TargetText,
			CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange},
			EvidenceDeclarationRef: runtimecontract.CanonicalRuntimeDefinitionEvidenceDeclarationRef(item.SourceScope, item.SourceID),
		}},
		Slices: []runtimecontract.RuntimeAcceptanceSlice{slice}, CoverageItems: []runtimecontract.RuntimeCoverageItem{item},
		ConvergencePolicy: runtimecontract.DefaultRuntimeConvergencePolicy(), RecoveryPolicy: runtimecontract.DefaultRuntimeRecoveryPolicy(),
	}
	bootstrap.ExecutionBudgetPolicy = runtimecontract.RuntimeExecutionBudgetPolicyForRuntimePolicies(bootstrap.ConvergencePolicy, bootstrap.RecoveryPolicy)
	if err := bootstrap.Validate(); err != nil {
		t.Fatalf("validate sealed restart bootstrap: %v", err)
	}
	return bootstrap.Clone()
}

func startSealedSnapshotRunOwner(t *testing.T, dataRoot string) (*runbiz.Biz, magicevent.Hub) {
	t.Helper()
	hub := magicevent.NewHub(32)
	owner := runbiz.New()
	if err := owner.Configure(runbiz.Dependencies{
		OwnerRepositories: workorchrepo.NewWorkspaceScopedFileRepositorySet(dataRoot).RunOwner,
		EventHub:          hub,
	}); err != nil {
		t.Fatalf("configure Run owner: %v", err)
	}
	if err := owner.Start(); err != nil {
		t.Fatalf("start Run owner: %v", err)
	}
	return owner, hub
}

func stopSealedSnapshotRunOwner(t *testing.T, owner *runbiz.Biz, hub magicevent.Hub) {
	t.Helper()
	if err := owner.Stop(context.Background()); err != nil {
		t.Fatalf("stop Run owner: %v", err)
	}
	hub.Terminate(context.Background())
}

func sendSealedSnapshotRestart(t *testing.T, hub magicevent.Hub, command runevents.WorkspaceExecutionIntentCommand) runevents.WorkspaceExecutionIntentResult {
	t.Helper()
	ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), command.WorkspaceID)
	event := magicevent.NewEventWithContext(
		runevents.TopicRunCommandWorkspaceExecutionIntent, "blocks/workspace", runevents.CommandDestination,
		magicevent.NewHeader(), ctx, command,
	)
	event.BindLaneKey("state/workspace/" + string(command.WorkspaceID))
	response := hub.Send(event)
	if response == nil {
		t.Fatal("restart response is nil")
	}
	if err := response.Error(); err != nil {
		t.Fatalf("restart owner error: %v", err)
	}
	data, err := response.Get()
	if err != nil {
		t.Fatalf("read restart response: %v", err)
	}
	result, ok := data.(runevents.WorkspaceExecutionIntentResult)
	if !ok {
		t.Fatalf("restart response type = %T", data)
	}
	return result
}
