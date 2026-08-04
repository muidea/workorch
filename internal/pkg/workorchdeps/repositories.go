package workorchdeps

import (
	cd "github.com/muidea/magicCommon/def"
	"github.com/muidea/magicCommon/framework/plugin/initiator"
	runtimecommon "github.com/muidea/workorch/internal/initiators/workorchruntime/pkg/common"
)

func ResolveWorkspaceRepositories(consumer string) (runtimecommon.WorkspaceRepositories, *cd.Error) {
	provider, err := resolveRepositoryProvider(consumer)
	if err != nil {
		return runtimecommon.WorkspaceRepositories{}, err
	}
	return provider.Workspace(), nil
}

func ResolveDecisionRepositories(consumer string) (runtimecommon.DecisionRepositories, *cd.Error) {
	provider, err := resolveRepositoryProvider(consumer)
	if err != nil {
		return runtimecommon.DecisionRepositories{}, err
	}
	return provider.Decision(), nil
}

func ResolveApprovalRepositories(consumer string) (runtimecommon.ApprovalRepositories, *cd.Error) {
	provider, err := resolveRepositoryProvider(consumer)
	if err != nil {
		return runtimecommon.ApprovalRepositories{}, err
	}
	return provider.Approval(), nil
}

func ResolveEventStreamRepositories(consumer string) (runtimecommon.EventStreamRepositories, *cd.Error) {
	provider, err := resolveRepositoryProvider(consumer)
	if err != nil {
		return runtimecommon.EventStreamRepositories{}, err
	}
	return provider.EventStream(), nil
}

func ResolveGovernanceRepositories(consumer string) (runtimecommon.GovernanceRepositories, *cd.Error) {
	provider, err := resolveRepositoryProvider(consumer)
	if err != nil {
		return runtimecommon.GovernanceRepositories{}, err
	}
	return provider.Governance(), nil
}

func ResolveMCPRepositories(consumer string) (runtimecommon.MCPRepositories, *cd.Error) {
	provider, err := resolveRepositoryProvider(consumer)
	if err != nil {
		return runtimecommon.MCPRepositories{}, err
	}
	return provider.MCP(), nil
}

func ResolveMemoryRepositories(consumer string) (runtimecommon.MemoryRepositories, *cd.Error) {
	provider, err := resolveRepositoryProvider(consumer)
	if err != nil {
		return runtimecommon.MemoryRepositories{}, err
	}
	return provider.Memory(), nil
}

func ResolveOrchestratorRepositories(consumer string) (runtimecommon.OrchestratorRepositories, *cd.Error) {
	provider, err := resolveRepositoryProvider(consumer)
	if err != nil {
		return runtimecommon.OrchestratorRepositories{}, err
	}
	return provider.Orchestrator(), nil
}

func ResolveResultRepositories(consumer string) (runtimecommon.ResultRepositories, *cd.Error) {
	provider, err := resolveRepositoryProvider(consumer)
	if err != nil {
		return runtimecommon.ResultRepositories{}, err
	}
	return provider.Result(), nil
}

func ResolveRunRepositories(consumer string) (runtimecommon.RunRepositories, *cd.Error) {
	provider, err := resolveRepositoryProvider(consumer)
	if err != nil {
		return runtimecommon.RunRepositories{}, err
	}
	return provider.Run(), nil
}

func ResolveSkillRepositories(consumer string) (runtimecommon.SkillRepositories, *cd.Error) {
	provider, err := resolveRepositoryProvider(consumer)
	if err != nil {
		return runtimecommon.SkillRepositories{}, err
	}
	return provider.Skill(), nil
}

func ResolveSoulRepositories(consumer string) (runtimecommon.SoulRepositories, *cd.Error) {
	provider, err := resolveRepositoryProvider(consumer)
	if err != nil {
		return runtimecommon.SoulRepositories{}, err
	}
	return provider.Soul(), nil
}

func resolveRepositoryProvider(consumer string) (runtimecommon.RepositoryProvider, *cd.Error) {
	var helper runtimecommon.Provider
	runtime, err := initiator.GetEntity(runtimecommon.InitiatorID, helper)
	if err != nil {
		return nil, err
	}
	provider := runtime.RepositoryProvider()
	if provider == nil {
		return nil, cd.NewError(cd.IllegalParam, "invalid argument: missing workorch repository provider for "+consumer)
	}
	return provider, nil
}
