package preparationcompiler

import (
	"reflect"

	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/startuppreparation"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/definitionconfiguration"
	"github.com/muidea/workorch/internal/pkg/runtimeprojection"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// A pure structural compilation, not a Runtime activation, owner resource
// verdict or complete PreparedBootstrap. No provider/model is invented here.
type PreparedExecutionPolicy struct {
	InputVersion               string
	SnapshotHash, ManifestHash string
	DialogLanguage             string
	ExecutionContract          runtimecontract.RuntimeExecutionContract
	EvidencePolicies           []runtimecontract.RuntimeEvidencePolicy
}

// CompileOriginalPreparedExecutionPolicy consumes only the complete original
// archive after independent persistence/source validation. It deliberately has
// no context, config loader, Hub or activation port. Missing history fails
// closed instead of borrowing a newer process or Workspace configuration.
func CompileOriginalPreparedExecutionPolicy(w *workspace.Workspace, a workspace.WorkspacePreparationAttempt) (PreparedExecutionPolicy, error) {
	if workspace.ValidatePreparationEvidencePersistence(w) != nil {
		return PreparedExecutionPolicy{}, common.ErrConflict
	}
	return compileOriginalPreparedExecutionPolicy(w, a)
}

// A private, call-local compilation of the complete original source. It is
// neither retained across calls nor exported as a validation credential.
type originalPreparedSource struct {
	snapshot   startuppreparation.ExactConfirmedStartupSnapshot
	manifest   startuppreparation.Manifest
	definition workspace.PreparationDefinitionCompilation
}

func compileOriginalPreparedSource(w *workspace.Workspace, a workspace.WorkspacePreparationAttempt) (originalPreparedSource, error) {
	fail := func() (originalPreparedSource, error) { return originalPreparedSource{}, common.ErrConflict }
	if a.ExecutionInputVersion != workspace.PreparationExecutionInputsVersion || w.StartupPreparation == nil {
		return fail()
	}
	found := false
	for _, saved := range w.StartupPreparation.Attempts {
		if saved.RequestID == a.RequestID {
			if found || !reflect.DeepEqual(saved, a) {
				return fail()
			}
			found = true
		}
	}
	if !found {
		return fail()
	}
	return compileOriginalPreparedDefinitionSource(w, a)
}

func compileOriginalPreparedDefinitionSource(w *workspace.Workspace, a workspace.WorkspacePreparationAttempt) (originalPreparedSource, error) {
	fail := func() (originalPreparedSource, error) { return originalPreparedSource{}, common.ErrConflict }
	source, err := workspace.PreparationDefinitionCompilationSource(w, a)
	if err != nil {
		return originalPreparedSource{}, err
	}
	snapshot := startuppreparation.ExactConfirmedStartupSnapshot(source)
	manifest, err := startuppreparation.CompilePreparationManifest(snapshot)
	if err != nil || manifest.ManifestHash != a.Binding.ManifestHash || manifest.PolicyVersion != a.Binding.CheckPolicyVersion || a.Binding.CompilerVersion != startuppreparation.BootstrapCompilerVersion {
		return fail()
	}
	definition, err := startuppreparation.CompilePreparedDefinition(snapshot, manifest)
	if err != nil || definition.ValidateFor(a) != nil {
		return fail()
	}
	return originalPreparedSource{snapshot: snapshot, manifest: manifest, definition: definition}, nil
}

func compileOriginalPreparedExecutionPolicy(w *workspace.Workspace, a workspace.WorkspacePreparationAttempt) (PreparedExecutionPolicy, error) {
	source, err := compileOriginalPreparedSource(w, a)
	if err != nil {
		return PreparedExecutionPolicy{}, err
	}
	return source.policy(w, a)
}

func (s originalPreparedSource) policy(w *workspace.Workspace, a workspace.WorkspacePreparationAttempt) (PreparedExecutionPolicy, error) {
	for _, e := range w.PreparationSealEvidence {
		if e.Ref == a.SealEvidenceRef && e.ExecutionInputs != nil {
			original, err := workspace.OriginalPreparationTemplate(w, a)
			if err != nil {
				return PreparedExecutionPolicy{}, err
			}
			return CompilePreparedExecutionPolicy(s.snapshot, s.manifest, e.ExecutionInputs.Clone(), original)
		}
	}
	return PreparedExecutionPolicy{}, common.ErrConflict
}

// Like CompilePreparedBootstrap, this detached compiler validates structure
// but does not authenticate caller inputs or grant readiness. Production
// historical consumers must use CompileOriginalPreparedExecutionPolicy.
func CompilePreparedExecutionPolicy(snapshot startuppreparation.ExactConfirmedStartupSnapshot, manifest startuppreparation.Manifest, inputs workspace.PreparationExecutionInputs, originalTemplate workspace.WorkspaceTemplate) (PreparedExecutionPolicy, error) {
	if manifest.ValidateFor(snapshot) != nil || inputs.Validate() != nil || snapshot.WorkspaceRevision == ^uint64(0) || inputs.Process.WorkspaceID != snapshot.WorkspaceID || inputs.Process.WorkspaceRevision != snapshot.WorkspaceRevision+1 || inputs.Template.ID != snapshot.StartupInput.WorkspaceTemplateRef || inputs.Configuration.Hash() != snapshot.AuthoringConfigurationHash {
		return PreparedExecutionPolicy{}, common.ErrConflict
	}
	w := &workspace.Workspace{ID: snapshot.WorkspaceID, TemplateID: inputs.Template.ID, DefaultSoulID: inputs.Configuration.DefaultSoulID, SoulBindings: inputs.Configuration.Clone().SoulBindings, EvidencePolicies: inputs.Configuration.Clone().EvidencePolicies, ContextHints: inputs.Configuration.ContextHints.Clone(), DefinitionAdmissionCandidates: []workspace.DefinitionAdmissionCandidate{snapshot.Candidate.Clone()}, StartupInput: &snapshot.StartupInput, ActivationAdmission: &snapshot.ActivationAdmission}
	// The source configuration is original owner material, while this operation's
	// declaration snapshot is current and retained in ExecutionInputs.
	originalTemplate = originalTemplate.Clone()
	w.DefinitionAuthoring = &workspace.DefinitionAuthoringRecord{TemplateSnapshot: originalTemplate, Input: workspace.DefinitionAuthoringInput{GoalText: snapshot.Candidate.GoalInput.Text}}
	current, err := workspace.WorkspaceForCurrentTemplate(w, &inputs.Template)
	if err != nil {
		return PreparedExecutionPolicy{}, err
	}
	w = current
	contract := executionContractHints(w.ContextHints.Map(), inputs.Template.ContextHints.Map())
	p := inputs.Process.Before
	// Same exact configuration projection as applyConfiguredRuntimeRepairLimit;
	// no call to it, since it reads today's process configuration.
	contract["maxRepairAttemptsPerFocus"] = p.MaxRuntimeRepairAttempts
	contract["candidateDocumentRecovery"] = map[string]any{"maxAttempts": p.MaxDocumentRecoveryAttempts}
	if err := runtimeprojection.ValidateRuntimeSourceTargetDeclarations(contract, nil); err != nil {
		return PreparedExecutionPolicy{}, err
	}
	execution := runtimeprojection.RuntimeExecutionContract(contract)
	compiled, err := definitionconfiguration.CompileSourceForItems(w, &inputs.Template, snapshot.Candidate.DoDInputs)
	if err != nil {
		return PreparedExecutionPolicy{}, err
	}
	execution.ProfileContracts = compiled.ProfileContracts
	execution.ExecutionLimits = compiled.ExecutionLimits
	if execution.Validate() != nil {
		return PreparedExecutionPolicy{}, common.ErrConflict
	}
	policies, err := runtimeprojection.CompileRuntimeEvidencePolicies("Workspace", w.EvidencePolicies)
	if err != nil {
		return PreparedExecutionPolicy{}, err
	}
	return PreparedExecutionPolicy{InputVersion: inputs.Version, SnapshotHash: snapshot.SnapshotHash, ManifestHash: manifest.ManifestHash, DialogLanguage: p.DialogLanguage(), ExecutionContract: execution, EvidencePolicies: policies}, nil
}
