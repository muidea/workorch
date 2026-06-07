package workorchdeps

import (
	cd "github.com/muidea/magicCommon/def"
	runtimecommon "github.com/muidea/workorch/internal/initiators/workorchruntime/pkg/common"
)

type SystemRepositories = runtimecommon.SystemRepositories
type WorkspaceRepositories = runtimecommon.WorkspaceRepositories
type DecisionRepositories = runtimecommon.DecisionRepositories
type ApprovalRepositories = runtimecommon.ApprovalRepositories
type EventStreamRepositories = runtimecommon.EventStreamRepositories
type GovernanceRepositories = runtimecommon.GovernanceRepositories
type MCPRepositories = runtimecommon.MCPRepositories
type MemoryRepositories = runtimecommon.MemoryRepositories
type OrchestratorRepositories = runtimecommon.OrchestratorRepositories
type ResultRepositories = runtimecommon.ResultRepositories
type RunRepositories = runtimecommon.RunRepositories
type SkillRepositories = runtimecommon.SkillRepositories
type SoulRepositories = runtimecommon.SoulRepositories

func ResolveSystemRepositories(consumer string) (SystemRepositories, *cd.Error) {
	provider, err := resolveRepositoryProvider(consumer)
	if err != nil {
		return SystemRepositories{}, err
	}
	return provider.System(), nil
}

func ResolveWorkspaceRepositories(consumer string) (WorkspaceRepositories, *cd.Error) {
	provider, err := resolveRepositoryProvider(consumer)
	if err != nil {
		return WorkspaceRepositories{}, err
	}
	return provider.Workspace(), nil
}

func ResolveDecisionRepositories(consumer string) (DecisionRepositories, *cd.Error) {
	provider, err := resolveRepositoryProvider(consumer)
	if err != nil {
		return DecisionRepositories{}, err
	}
	return provider.Decision(), nil
}

func ResolveApprovalRepositories(consumer string) (ApprovalRepositories, *cd.Error) {
	provider, err := resolveRepositoryProvider(consumer)
	if err != nil {
		return ApprovalRepositories{}, err
	}
	return provider.Approval(), nil
}

func ResolveEventStreamRepositories(consumer string) (EventStreamRepositories, *cd.Error) {
	provider, err := resolveRepositoryProvider(consumer)
	if err != nil {
		return EventStreamRepositories{}, err
	}
	return provider.EventStream(), nil
}

func ResolveGovernanceRepositories(consumer string) (GovernanceRepositories, *cd.Error) {
	provider, err := resolveRepositoryProvider(consumer)
	if err != nil {
		return GovernanceRepositories{}, err
	}
	return provider.Governance(), nil
}

func ResolveMCPRepositories(consumer string) (MCPRepositories, *cd.Error) {
	provider, err := resolveRepositoryProvider(consumer)
	if err != nil {
		return MCPRepositories{}, err
	}
	return provider.MCP(), nil
}

func ResolveMemoryRepositories(consumer string) (MemoryRepositories, *cd.Error) {
	provider, err := resolveRepositoryProvider(consumer)
	if err != nil {
		return MemoryRepositories{}, err
	}
	return provider.Memory(), nil
}

func ResolveOrchestratorRepositories(consumer string) (OrchestratorRepositories, *cd.Error) {
	provider, err := resolveRepositoryProvider(consumer)
	if err != nil {
		return OrchestratorRepositories{}, err
	}
	return provider.Orchestrator(), nil
}

func ResolveResultRepositories(consumer string) (ResultRepositories, *cd.Error) {
	provider, err := resolveRepositoryProvider(consumer)
	if err != nil {
		return ResultRepositories{}, err
	}
	return provider.Result(), nil
}

func ResolveRunRepositories(consumer string) (RunRepositories, *cd.Error) {
	provider, err := resolveRepositoryProvider(consumer)
	if err != nil {
		return RunRepositories{}, err
	}
	return provider.Run(), nil
}

func ResolveSkillRepositories(consumer string) (SkillRepositories, *cd.Error) {
	provider, err := resolveRepositoryProvider(consumer)
	if err != nil {
		return SkillRepositories{}, err
	}
	return provider.Skill(), nil
}

func ResolveSoulRepositories(consumer string) (SoulRepositories, *cd.Error) {
	provider, err := resolveRepositoryProvider(consumer)
	if err != nil {
		return SoulRepositories{}, err
	}
	return provider.Soul(), nil
}

func resolveRepositoryProvider(consumer string) (runtimecommon.RepositoryProvider, *cd.Error) {
	runtime, err := ResolveRuntimeHelper(consumer)
	if err != nil {
		return nil, err
	}
	provider := runtime.RepositoryProvider()
	if provider == nil {
		return nil, cd.NewError(cd.IllegalParam, "invalid argument: missing workorch repository provider for "+consumer)
	}
	return provider, nil
}
