package workorchruntimecontract

import (
	"fmt"
	"github.com/muidea/workorch/internal/pkg/common"
)

// Frozen compiler paths before invocation-local validation reuse.
// referenceWorkManifestCompiler derives one CWI's
// current work vocabulary from immutable Definition plus a privately persisted
// scope reservation. It does not select a next round or grant a dispatch.
func referenceWorkManifestCompiler(workspace common.WorkspaceID, run common.RunID, definition RuntimeDefinitionContext, seed RuntimeDefinitionSeed, focuses []RuntimeDefinitionFocus, execution RuntimeExecutionContract, catalogs []RuntimeOperationAdmissibility, round RuntimeImplementationWorkRound) (RuntimeSemanticPlanManifest, error) {
	if err := round.Validate(); err != nil {
		return RuntimeSemanticPlanManifest{}, err
	}
	if round.WorkingBinding != nil || round.DispatchID != "" || round.ScopeObservation == nil || round.WorkspaceID != workspace || round.RunID != run || round.StartupInputHash != definition.StartupInputHash || round.DefinitionSeedHash != seed.Hash {
		return RuntimeSemanticPlanManifest{}, fmt.Errorf("semantic work compiler requires an unbound same-startup scope reservation")
	}
	sources := RuntimeImplementationWorkScopeSources{WorkspaceID: workspace, RunID: run, AuthoritySnapshotID: round.ScopeRequest.AuthoritySnapshotID, Seed: seed, Focuses: focuses, Profiles: execution.ProfileContracts, CoverageWorkItemID: round.CoverageWorkItemID, ManifestBinding: round.ScopeRequest.ManifestBinding}
	if err := referenceWorkScopeAgainstSources(round.ScopeRequest, sources); err != nil {
		return RuntimeSemanticPlanManifest{}, err
	}
	if err := definition.Validate(); err != nil {
		return RuntimeSemanticPlanManifest{}, err
	}
	if err := execution.Validate(); err != nil {
		return RuntimeSemanticPlanManifest{}, err
	}
	item, found := runtimeCoverageItemForID(seed.CoverageItems, round.CoverageWorkItemID)
	if !found {
		return RuntimeSemanticPlanManifest{}, fmt.Errorf("semantic work CWI is outside sealed Definition")
	}
	var source RuntimeDefinitionFocus
	for _, focus := range focuses {
		if len(focus.CoverageWorkItemIDs) == 1 && focus.CoverageWorkItemIDs[0] == item.ID {
			source = focus
		}
	}
	obligation, err := compileRuntimeSemanticDefinitionProofObligation(definition.Revision, item, source)
	if err != nil {
		return RuntimeSemanticPlanManifest{}, err
	}
	if err := validateRuntimeSemanticOwnerCatalogCollection(catalogs); err != nil {
		return RuntimeSemanticPlanManifest{}, err
	}
	policy := round.ScopeRequest.Policy
	var ownerCatalog RuntimeOperationAdmissibility
	for _, catalog := range catalogs {
		if catalog.OwnerModuleID == policy.OwnerModuleID {
			ownerCatalog = catalog
		}
	}
	if ownerCatalog.OwnerModuleID == "" || ownerCatalog.ObservedRevision != round.ScopeObservation.Selection.ObservedRevision {
		return RuntimeSemanticPlanManifest{}, fmt.Errorf("semantic work has no fresh resource-owner catalog")
	}
	kinds := []string{}
	for _, operation := range policy.AllowedOperations {
		if !containsCanonicalOperationKind(definition.CapabilityContract, operation.Kind) || !runtimeOperationKindsContain(ownerCatalog.OperationKinds, operation.Kind) {
			return RuntimeSemanticPlanManifest{}, fmt.Errorf("semantic work operation is outside sealed capability/catalog")
		}
		kinds = append(kinds, operation.Kind)
	}
	inputCatalogs := cloneRuntimeStateRawSlice(catalogs, cloneRuntimeOperationAdmissibilityRaw)
	work := RuntimeSemanticImplementationWork{RoundHash: round.Hash, Ordinal: round.Ordinal, PrecedingReceiptSetHash: round.PrecedingReceiptSetHash, PolicyHash: round.PolicyHash, Scope: round.ScopeObservation.Clone(), MutationInputCatalogs: inputCatalogs}
	work.ExcludedTargetRefs = runtimeValidationInputMutationExclusions(policy.OwnerModuleID, RuntimeCandidateManifestOwnerFacts{OperationAdmissibilities: inputCatalogs}, RequiredOutputContract{}, nil)
	obligation.ImplementationWork = &work
	// Settled external prerequisite CWIs do not become provider ordering input.
	item.PrerequisiteCoverageWorkItemIDs = nil
	manifest := RuntimeSemanticPlanManifest{SchemaVersion: RuntimeSemanticPlanManifestSchemaCurrent, WorkspaceID: workspace, RunID: run,
		DefinitionRevision: definition.Revision, DefinitionSeedHash: seed.Hash, OperationContractRevision: execution.OperationContractRevision,
		CoverageItems: []RuntimeCoverageItem{item}, AllowedOwnerModuleIDs: []string{policy.OwnerModuleID}, AllowedOperationKinds: canonicalizeOperationKinds(kinds),
		AllowedTargetPolicies: append([]RuntimeOperationTargetPolicy(nil), policy.AllowedTargetPolicies...), ProofObligations: []RuntimeSemanticProofObligation{obligation}, OwnerCatalogs: []RuntimeOperationAdmissibility{ownerCatalog.Clone()}}
	manifest = manifest.Normalized()
	manifest.Hash = StableRuntimeSemanticPlanManifestHash(manifest)
	if err := manifest.Validate(); err != nil {
		return RuntimeSemanticPlanManifest{}, err
	}
	return manifest, nil
}

// referenceWorkPurposeCompiler 复用原 semantic 请求合同与预算。
// Preview 不是 effect round；达到工作轮次上限仍可选择原 terminal proof。
func referenceWorkPurposeCompiler(workspace common.WorkspaceID, run common.RunID, definition RuntimeDefinitionContext, seed RuntimeDefinitionSeed, focuses []RuntimeDefinitionFocus, execution RuntimeExecutionContract, catalogs []RuntimeOperationAdmissibility, preview RuntimeImplementationWorkScopePreview, rounds []RuntimeImplementationWorkRound) (RuntimeSemanticPlanManifest, error) {
	if preview.Observation == nil {
		return RuntimeSemanticPlanManifest{}, fmt.Errorf("implementation purpose requires complete fresh scope preview")
	}
	ordinal, preceding := 1, ""
	for _, prior := range rounds {
		if prior.CoverageWorkItemID != preview.Request.CoverageWorkItemID {
			continue
		}
		if prior.CompletedReceiptSetHash == "" || prior.FailedReceiptHash != "" {
			return RuntimeSemanticPlanManifest{}, fmt.Errorf("implementation purpose cannot bypass incomplete prior work")
		}
		ordinal, preceding = prior.Ordinal+1, prior.CompletedReceiptSetHash
	}
	// 复用词汇编译器的来源校验；这个局部 value 从不入账、从不授 dispatch。
	vocabularySource := RuntimeImplementationWorkRound{WorkspaceID: workspace, RunID: run, StartupInputHash: definition.StartupInputHash, DefinitionSeedHash: seed.Hash, CoverageWorkItemID: preview.Request.CoverageWorkItemID, PolicyHash: StableRuntimeImplementationWorkPolicyHash(preview.Request.Policy), Ordinal: 1, ScopeRequest: preview.Request.Clone(), ScopeObservation: preview.Observation}
	vocabularySource.Hash = StableRuntimeImplementationWorkRoundHash(vocabularySource)
	manifest, err := referenceWorkManifestCompiler(workspace, run, definition, seed, focuses, execution, catalogs, vocabularySource)
	if err != nil {
		return RuntimeSemanticPlanManifest{}, err
	}
	work := manifest.ProofObligations[0].ImplementationWork
	work.PurposeChoiceOnly, work.EffectAvailable = true, ordinal <= preview.Request.Policy.MaxRoundsPerCWI
	work.Ordinal, work.PrecedingReceiptSetHash = ordinal, preceding
	work.RoundHash = implementationWorkRawHash("implementation-purpose-preview:", preview)
	manifest.Hash = StableRuntimeSemanticPlanManifestHash(manifest)
	if err := manifest.Validate(); err != nil {
		return RuntimeSemanticPlanManifest{}, err
	}
	return manifest, nil
}
