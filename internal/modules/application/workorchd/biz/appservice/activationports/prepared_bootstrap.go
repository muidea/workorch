package activationports

import (
	"reflect"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/activationcore"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/startuppreparation"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const OriginalPreparedBootstrapVersion = "workorch-original-prepared-bootstrap/v1"

// A reproducible structural product of one original operation. Neither this
// value nor its hash is a readiness receipt, currentness proof or permission.
// Source archives, not this product, are the persistence/recovery authority.
type OriginalPreparedBootstrap struct {
	Version, FailureCode string
	Compilation          *activationcore.PreparedBootstrapCompilation
	// Original deterministic compiler diagnostic, private in-memory only. The
	// containing source archive permits exact reproduction; HTTP never emits it.
	Diagnostic []byte `json:"-"`
}

// CompileOriginalPreparedBootstrap has no owner ports, environment/config
// reads, clock or context. It never substitutes current configuration, defaults
// or an empty resource set for missing original observations. Provider selection
// here is only an archived declaration; remote authorization remains required
// by the separate preparation checklist even when this compiler succeeds.
func CompileOriginalPreparedBootstrap(w *workspace.Workspace, a workspace.WorkspacePreparationAttempt) (OriginalPreparedBootstrap, error) {
	result := OriginalPreparedBootstrap{Version: OriginalPreparedBootstrapVersion}
	fail := func(code string) (OriginalPreparedBootstrap, error) {
		result.FailureCode = code
		return result, nil
	}
	if w == nil || workspace.ValidatePreparationEvidencePersistence(w) != nil || w.StartupPreparation == nil {
		return OriginalPreparedBootstrap{}, common.ErrConflict
	}
	found := false
	for _, saved := range w.StartupPreparation.Attempts {
		if saved.RequestID == a.RequestID {
			if found || !reflect.DeepEqual(saved, a) {
				return OriginalPreparedBootstrap{}, common.ErrConflict
			}
			found = true
		}
	}
	if !found {
		return OriginalPreparedBootstrap{}, common.ErrConflict
	}
	if a.State != workspace.WorkspacePreparationBlocked || a.ExecutionInputVersion == "" {
		return fail("original_inputs_unavailable")
	}
	policy, err := CompileOriginalPreparedExecutionPolicy(w, a)
	if err != nil {
		result.Diagnostic = []byte(err.Error())
		return fail("original_policy_unavailable")
	}
	var souls *workspace.WorkspacePreparationSoulEvidence
	var skills *workspace.WorkspacePreparationSkillRegistryEvidence
	var memory *workspace.WorkspacePreparationMemoryInventoryEvidence
	var mcp *workspace.WorkspacePreparationMCPRegistryEvidence
	var provider *workspace.WorkspacePreparationProviderCacheEvidence
	for i := range w.PreparationSoulEvidence {
		if w.PreparationSoulEvidence[i].Ref == a.SoulEvidenceRef {
			souls = &w.PreparationSoulEvidence[i]
		}
	}
	for i := range w.PreparationSkillRegistryEvidence {
		if w.PreparationSkillRegistryEvidence[i].Ref == a.SkillRegistryEvidenceRef {
			skills = &w.PreparationSkillRegistryEvidence[i]
		}
	}
	for i := range w.PreparationMemoryInventoryEvidence {
		if w.PreparationMemoryInventoryEvidence[i].Ref == a.MemoryInventoryEvidenceRef {
			memory = &w.PreparationMemoryInventoryEvidence[i]
		}
	}
	for i := range w.PreparationMCPRegistryEvidence {
		if w.PreparationMCPRegistryEvidence[i].Ref == a.MCPRegistryEvidenceRef {
			mcp = &w.PreparationMCPRegistryEvidence[i]
		}
	}
	for i := range w.PreparationProviderCacheEvidence {
		if w.PreparationProviderCacheEvidence[i].Ref == a.ProviderCacheEvidenceRef {
			provider = &w.PreparationProviderCacheEvidence[i]
		}
	}
	if souls == nil {
		return fail("soul_source_unavailable")
	}
	if skills == nil || skills.Selection == nil || skills.Selection.FailureCode != "" || skills.Selection.Context == nil || skills.Selection.Context.FailureCode != "" {
		return fail("skill_source_unavailable")
	}
	if memory == nil || memory.Selection == nil {
		return fail("memory_source_unavailable")
	}
	if memory.Selection.FailureCode == "semantic_evidence_required" {
		return fail("memory_semantic_evidence_required")
	}
	if memory.Selection.FailureCode != "" || memory.Selection.Selection == nil {
		return fail("memory_source_unavailable")
	}
	if mcp == nil || mcp.Selection == nil || mcp.Selection.FailureCode != "" {
		return fail("mcp_source_unavailable")
	}
	// Do not let a missing Provider mask missing non-Provider source material.
	// This is not an authorization check and does not upgrade the blocked record.
	// Catalog availability is independent of the frozen local configuration.
	// The configuration-only preparation result is not a Runtime ceiling. This structural
	// compiler consumes only the exact declaration already independently
	// reproduced by evidence validation; it never consumes capability, budget
	// or authorization success and does not alter those judgments.
	if provider == nil || provider.Selection == nil || provider.Facts.Observation.Configuration == nil || provider.Selection.Selection.Validate() != nil || (provider.Selection.Source != "llm_owner_default" && provider.Selection.Source != "workspace_default") {
		return fail("provider_declaration_unavailable")
	}
	activation := runtimecontract.RuntimeActivationContext{
		ProviderSelection: provider.Selection.Selection,
		DialogLanguage:    policy.DialogLanguage, ExecutionContract: policy.ExecutionContract,
		EvidencePolicies: policy.EvidencePolicies,
	}
	f := souls.Facts
	for _, soul := range f.Souls {
		if soul.ID == f.DefaultLookupSoulID {
			activation.DefaultSoul = runtimeSoulContext(soul)
		}
	}
	for _, selection := range f.Selections {
		if selection.SoulID == "" {
			continue // explicit observed unbound, not missing material
		}
		for _, soul := range f.Souls {
			if soul.ID == selection.SoulID {
				activation.StageSoulBindings = append(activation.StageSoulBindings, runtimecontract.RuntimeStageSoulBinding{Stage: selection.Stage, Source: selection.Source, Soul: *runtimeSoulContext(soul)})
			}
		}
	}
	context := skills.Selection.Context.Clone()
	activation.Skills, activation.SkillContextBudget = context.Projection, context.Budget
	activation.MCPServerIDs = runtimeMCPServerIDs(mcp.Selection.SelectedServerIDs)
	activation.MCPCapabilities = runtimeMCPCapabilityContexts(mcp.Selection.Selected.Capabilities)
	activation.OperationPolicies = runtimeMCPOperationPolicies(activation.MCPCapabilities)
	// Use the same deterministic list projection/budget adapter as activation.
	// Semantic nonempty inventories were rejected above, never downgraded.
	selected, evidence, err := applyRuntimeMemorySelectionBudget(memory.Facts.Observation.Items, &MemorySelectionEvidence{Policy: "deterministic_workspace_list", RefreshSource: "workspace_inventory"})
	if err != nil {
		return fail("memory_source_unavailable")
	}
	activation.Memories, activation.MemorySelection = runtimeMemoryContexts(selected), runtimeMemorySelectionEvidence(evidence)
	activation = activation.Normalized()
	if err := activation.Validate(); err != nil {
		result.Diagnostic = []byte(err.Error())
		return fail("activation_structure_rejected")
	}
	source, err := workspace.PreparationDefinitionCompilationSource(w, a)
	if err != nil {
		return OriginalPreparedBootstrap{}, err
	}
	snapshot := startuppreparation.ExactConfirmedStartupSnapshot(source)
	manifest, err := startuppreparation.CompilePreparationManifest(snapshot)
	if err != nil {
		return OriginalPreparedBootstrap{}, err
	}
	compiled, err := activationcore.CompilePreparedBootstrap(snapshot, manifest, activation)
	if err != nil {
		result.Diagnostic = []byte(err.Error())
		return fail("bootstrap_structure_rejected")
	}
	result.Compilation = &compiled
	return result, nil
}
