package activationports

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/activationcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/workspaceports"
	"github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/definitionadmission"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// CompileWorkspaceExecutionRestartBootstrap is the application's narrow
// restart compiler. It only reads owner snapshots and produces an immutable
// Runtime bootstrap; it never creates, starts, continues, supersedes, or
// releases a Run/Workspace slot.
func CompileWorkspaceExecutionRestartBootstrap(ctx context.Context, request ActivateWorkspaceExecutionViewRequest) (*runtimecontract.RuntimeBootstrap, error) {
	if request.Hub == nil || strings.TrimSpace(request.WorkspaceID) == "" {
		return nil, common.ErrInvalidArgument
	}
	workspaceID := common.WorkspaceID(strings.TrimSpace(request.WorkspaceID))
	workspace, err := workspaceports.ReadWorkspaceRecord(ctx, request.Hub, workspaceID)
	if err != nil {
		return nil, err
	}
	ctx, err = common.BindRuntimeWorkspaceContext(ctx, workspace.ID)
	if err != nil {
		return nil, err
	}
	goal, dod, err := selectWorkspaceExecutionDefinitions(ctx, request.Hub, workspace.ID)
	if err != nil {
		return nil, err
	}
	admissionSnapshot, err := EnsureWorkspaceDefinitionAdmission(ctx, request.Hub, workspace, goal, dod)
	if err != nil {
		return nil, err
	}
	workspace = admissionSnapshot.Workspace
	activationBuild, err := workspaceRuntimeActivation(ctx, request, workspace, admissionSnapshot.Template, goal, dod)
	if err != nil {
		return nil, err
	}
	if workspace.StartupInput == nil || workspace.ActivationAdmission == nil {
		return nil, common.ErrConflict
	}
	inventory, err := definitionadmission.CompileRuntimeSourceObligationInventory(admissionSnapshot.Candidate, *workspace.StartupInput, *workspace.ActivationAdmission)
	if err != nil {
		return nil, err
	}
	bootstrap, err := activationcore.RuntimeBootstrapWithSourceObligations(
		activationBuild.EffectiveGoal,
		activationBuild.EffectiveDoD,
		activationBuild.DefinitionRevision,
		workspaceStartupInputHash(workspace, activationBuild.DefinitionRevision),
		activationBuild.DefinitionChangeImpact,
		activationBuild.RuntimeActivation,
		inventory,
	)
	if err != nil {
		return nil, err
	}
	if err := validateRuntimeBootstrapStartupSeal(workspace, admissionSnapshot.Template, admissionSnapshot.Candidate, bootstrap); err != nil {
		return nil, err
	}
	return bootstrap, nil
}

func validateRuntimeBootstrapStartupSeal(workspace *workspacemodel.Workspace, template *workspacemodel.WorkspaceTemplate, candidate workspacemodel.DefinitionAdmissionCandidate, bootstrap *runtimecontract.RuntimeBootstrap) error {
	if workspace == nil || workspace.StartupInput == nil || workspace.ActivationAdmission == nil || template == nil || bootstrap == nil {
		return common.ErrInvalidArgument
	}
	startup := workspace.StartupInput.Normalized()
	admission := workspace.ActivationAdmission.Normalized()
	if err := validateRuntimeExecutionLimitsStartupSeal(candidate, bootstrap.Activation.ExecutionContract.ExecutionLimits); err != nil {
		return err
	}
	if startup.ValidateCandidate(candidate) != nil || admission.ValidateCandidate(startup, candidate) != nil || admission.State != workspacemodel.WorkspaceActivationEligible {
		return fmt.Errorf("%w: Runtime bootstrap does not bind an eligible Workspace Definition admission", common.ErrConflict)
	}
	expectedInventory, err := definitionadmission.CompileRuntimeSourceObligationInventory(candidate, startup, admission)
	if err != nil || !reflect.DeepEqual(expectedInventory, bootstrap.DefinitionSeed.SourceObligations) {
		return fmt.Errorf("%w: Runtime bootstrap source obligations drifted from exact eligible specification", common.ErrConflict)
	}
	if startup.WorkspaceTemplateRef != strings.TrimSpace(template.ID) || startup.TemplateSnapshotHash != workspaceTemplateSnapshotHash(template) {
		return fmt.Errorf("%w: Runtime bootstrap Template snapshot drifted after Definition admission", common.ErrConflict)
	}
	expectedEvidencePolicies, err := compileRuntimeEvidencePolicies("Workspace "+strings.TrimSpace(string(workspace.ID)), workspace.EvidencePolicies)
	if err != nil {
		return fmt.Errorf("%w: admitted Workspace evidence policies are invalid: %v", common.ErrConflict, err)
	}
	if bootstrap.Definition.Revision != startup.DefinitionRevision || bootstrap.Definition.StartupInputHash != startup.StartupInputHash ||
		runtimecontract.StableRuntimeProfileContractsHash(bootstrap.Activation.ExecutionContract.ProfileContracts) != startup.RuntimeProfileContractsHash ||
		!runtimecontract.SameRuntimeEvidencePolicies(bootstrap.Activation.EvidencePolicies, expectedEvidencePolicies) {
		return fmt.Errorf("%w: Runtime bootstrap definition/profile/evidence-policy snapshot drifted from Workspace startup input", common.ErrConflict)
	}
	return nil
}

func workspaceStartupInputHash(workspace *workspacemodel.Workspace, definitionRevision string) string {
	if workspace == nil || workspace.StartupInput == nil || workspace.ActivationAdmission == nil {
		return ""
	}
	startup := workspace.StartupInput.Normalized()
	admission := workspace.ActivationAdmission.Normalized()
	if startup.DefinitionRevision != strings.TrimSpace(definitionRevision) || admission.State != workspacemodel.WorkspaceActivationEligible || admission.DefinitionRevision != startup.DefinitionRevision || admission.StartupInputHash != startup.StartupInputHash {
		return ""
	}
	return startup.StartupInputHash
}

// workspaceRuntimeActivation is the compiler-local projection from immutable
// owner snapshots to an activation context.  It deliberately has no Run or
// Workspace lifecycle side effect; lifecycle admission belongs to the
// Workspace and Run owners after the compiler returns its sealed bootstrap.
func workspaceRuntimeActivation(ctx context.Context, request ActivateWorkspaceExecutionViewRequest, workspace *workspacemodel.Workspace, template *workspacemodel.WorkspaceTemplate, goal *workspacemodel.Goal, dod *workspacemodel.DoD) (workspaceRuntimeActivationBuild, error) {
	return buildWorkspaceRuntimeActivation(ctx, workspaceRuntimeActivationRequest{
		Hub: request.Hub,

		Workspace:                      workspace,
		Template:                       template,
		Goal:                           goal,
		DoD:                            dod,
		SystemDefaultSoulID:            common.SoulID(strings.TrimSpace(string(request.SystemDefaultSoulID))),
		DefaultDialogLanguage:          strings.TrimSpace(request.DefaultDialogLanguage),
		DefaultEmbeddingProviderLoader: request.DefaultEmbeddingProviderLoader,
		SkillHubCatalog:                request.SkillHubCatalog,
	})
}
