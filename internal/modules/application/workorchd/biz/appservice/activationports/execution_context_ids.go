package activationports

import "context"

func BuildWorkspaceExecutionContextForIDs(ctx context.Context, request WorkspaceExecutionContextByIDRequest) (map[string]any, error) {
	inputs, err := ReadWorkspaceExecutionContextInputs(ctx, request.Hub, request.WorkspaceID, request.GoalID, request.DoDID)
	if err != nil {
		return nil, err
	}
	return BuildWorkspaceExecutionContext(ctx, WorkspaceExecutionContextRequest{
		Hub:                            request.Hub,
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
