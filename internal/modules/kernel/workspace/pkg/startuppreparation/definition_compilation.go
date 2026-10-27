package startuppreparation

import (
	"reflect"

	"github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/definitionadmission"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// CompilePreparedDefinition is the Provider-independent part of preparation.
// It compiles complete source obligations from the original sealed input. It
// neither constructs an activation with a fabricated provider nor initializes
// Runtime. A successful result is still insufficient for a prepared bootstrap.
func CompilePreparedDefinition(snapshot ExactConfirmedStartupSnapshot, manifest Manifest) (workspace.PreparationDefinitionCompilation, error) {
	if err := manifest.ValidateFor(snapshot); err != nil {
		return workspace.PreparationDefinitionCompilation{}, err
	}
	inventory, err := definitionadmission.CompileRuntimeSourceObligationInventory(snapshot.Candidate, snapshot.StartupInput, snapshot.ActivationAdmission)
	if err != nil || inventory == nil {
		return workspace.PreparationDefinitionCompilation{}, common.ErrConflict
	}
	return workspace.PreparationDefinitionCompilation{
		CompilerVersion: workspace.PreparationDefinitionCompilerVersion,
		SnapshotHash:    snapshot.SnapshotHash, ManifestHash: manifest.ManifestHash,
		SourceInventoryHash: inventory.Hash, ProfileContractsHash: runtimecontract.StableRuntimeProfileContractsHash(snapshot.Candidate.RuntimeProfileContracts),
		SourceCount: len(inventory.Sources), RequirementCount: len(inventory.Requirements), ObligationCount: len(inventory.Obligations), VerificationCount: len(inventory.Verifications),
	}, nil
}

func CompileOriginalPreparedDefinition(w *workspace.Workspace, a workspace.WorkspacePreparationAttempt) (workspace.PreparationDefinitionCompilation, error) {
	s, err := workspace.PreparationDefinitionCompilationSource(w, a)
	if err != nil {
		return workspace.PreparationDefinitionCompilation{}, err
	}
	snapshot := ExactConfirmedStartupSnapshot(s)
	manifest, err := CompilePreparationManifest(snapshot)
	if err != nil || manifest.ManifestHash != a.Binding.ManifestHash || manifest.PolicyVersion != a.Binding.CheckPolicyVersion || a.Binding.CompilerVersion != BootstrapCompilerVersion {
		return workspace.PreparationDefinitionCompilation{}, common.ErrConflict
	}
	compiled, err := CompilePreparedDefinition(snapshot, manifest)
	if err != nil || compiled.ValidateFor(a) != nil {
		return workspace.PreparationDefinitionCompilation{}, common.ErrConflict
	}
	return compiled, nil
}

// Content hashes are outputs, never validation credentials. Reproduce complete
// source/compiler inputs on every repository read and write; old nil history
// is not backfilled. No cached success or mutable owner I/O is used here.
func ValidatePreparedDefinitionPersistence(w *workspace.Workspace) error {
	if w == nil {
		return common.ErrInvalidArgument
	}
	if w.StartupPreparation == nil {
		return nil
	}
	for _, a := range w.StartupPreparation.Attempts {
		if a.DefinitionCompilation == nil {
			continue
		}
		expected, err := CompileOriginalPreparedDefinition(w, a)
		if err != nil || !reflect.DeepEqual(expected, *a.DefinitionCompilation) {
			return common.ErrConflict
		}
	}
	return nil
}
