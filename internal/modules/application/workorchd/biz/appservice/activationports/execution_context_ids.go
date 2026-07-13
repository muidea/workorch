package activationports

import "context"

func BuildWorkspaceRuntimeActivationForIDs(ctx context.Context, request WorkspaceRuntimeActivationByIDRequest) (WorkspaceRuntimeActivationBuild, error) {
	inputs, err := ReadWorkspaceRuntimeActivationInputs(ctx, request.Hub, request.WorkspaceID, request.GoalID, request.DoDID)
	if err != nil {
		return WorkspaceRuntimeActivationBuild{}, err
	}
	return BuildWorkspaceRuntimeActivation(ctx, WorkspaceRuntimeActivationRequest{
		Hub: request.Hub,

		Workspace:                      inputs.Workspace,
		Goal:                           inputs.Goal,
		DoD:                            inputs.DoD,
		Command:                        request.Command,
		SystemDefaultSoulID:            request.SystemDefaultSoulID,
		DefaultDialogLanguage:          request.DefaultDialogLanguage,
		DefaultEmbeddingProviderLoader: request.DefaultEmbeddingProviderLoader,
		SkillHubCatalog:                request.SkillHubCatalog,
	})
}
