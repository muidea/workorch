package workorchdeps

import (
	"fmt"

	cd "github.com/muidea/magicCommon/def"
	"github.com/muidea/magicCommon/framework/plugin/initiator"
	runtimecommon "github.com/muidea/workorch/internal/initiators/workorchruntime/pkg/common"
)

// RepositoryConsumerID is the owner identity presented when a component
// resolves its repository family. The resolver validates the exact
// consumer-to-family binding before it discovers the repository Initiator.
type RepositoryConsumerID string

type repositoryFamily string

const (
	repositoryFamilyWorkspace    repositoryFamily = "workspace"
	repositoryFamilyDecision     repositoryFamily = "decision"
	repositoryFamilyApproval     repositoryFamily = "approval"
	repositoryFamilyEventStream  repositoryFamily = "eventstream"
	repositoryFamilyGovernance   repositoryFamily = "governance"
	repositoryFamilyMCP          repositoryFamily = "mcp"
	repositoryFamilyMemory       repositoryFamily = "memory"
	repositoryFamilyOrchestrator repositoryFamily = "orchestrator"
	repositoryFamilyResult       repositoryFamily = "result"
	repositoryFamilyRun          repositoryFamily = "run"
	repositoryFamilySkill        repositoryFamily = "skill"
	repositoryFamilySoul         repositoryFamily = "soul"
)

const (
	repositoryConsumerWorkspace    RepositoryConsumerID = "blocks/workspace"
	repositoryConsumerDecision     RepositoryConsumerID = "blocks/decision"
	repositoryConsumerApproval     RepositoryConsumerID = "blocks/approval"
	repositoryConsumerEventStream  RepositoryConsumerID = "blocks/eventstream"
	repositoryConsumerGovernance   RepositoryConsumerID = "kernel/governance"
	repositoryConsumerMCP          RepositoryConsumerID = "blocks/mcp-registry"
	repositoryConsumerMemory       RepositoryConsumerID = "blocks/memory"
	repositoryConsumerOrchestrator RepositoryConsumerID = "kernel/orchestrator"
	repositoryConsumerResult       RepositoryConsumerID = "blocks/result"
	repositoryConsumerRun          RepositoryConsumerID = "blocks/run"
	repositoryConsumerSkill        RepositoryConsumerID = "blocks/skill"
	repositoryConsumerSoul         RepositoryConsumerID = "blocks/soul"
)

func ResolveWorkspaceRepositories(consumer RepositoryConsumerID) (runtimecommon.WorkspaceRepositories, *cd.Error) {
	provider, err := resolveRepositoryProvider(consumer, repositoryFamilyWorkspace)
	if err != nil {
		return runtimecommon.WorkspaceRepositories{}, err
	}
	return provider.Workspace(), nil
}

func ResolveDecisionRepositories(consumer RepositoryConsumerID) (runtimecommon.DecisionRepositories, *cd.Error) {
	provider, err := resolveRepositoryProvider(consumer, repositoryFamilyDecision)
	if err != nil {
		return runtimecommon.DecisionRepositories{}, err
	}
	return provider.Decision(), nil
}

func ResolveApprovalRepositories(consumer RepositoryConsumerID) (runtimecommon.ApprovalRepositories, *cd.Error) {
	provider, err := resolveRepositoryProvider(consumer, repositoryFamilyApproval)
	if err != nil {
		return runtimecommon.ApprovalRepositories{}, err
	}
	return provider.Approval(), nil
}

func ResolveEventStreamRepositories(consumer RepositoryConsumerID) (runtimecommon.EventStreamRepositories, *cd.Error) {
	provider, err := resolveRepositoryProvider(consumer, repositoryFamilyEventStream)
	if err != nil {
		return runtimecommon.EventStreamRepositories{}, err
	}
	return provider.EventStream(), nil
}

func ResolveGovernanceRepositories(consumer RepositoryConsumerID) (runtimecommon.GovernanceRepositories, *cd.Error) {
	provider, err := resolveRepositoryProvider(consumer, repositoryFamilyGovernance)
	if err != nil {
		return runtimecommon.GovernanceRepositories{}, err
	}
	return provider.Governance(), nil
}

func ResolveMCPRepositories(consumer RepositoryConsumerID) (runtimecommon.MCPRepositories, *cd.Error) {
	provider, err := resolveRepositoryProvider(consumer, repositoryFamilyMCP)
	if err != nil {
		return runtimecommon.MCPRepositories{}, err
	}
	return provider.MCP(), nil
}

func ResolveMemoryRepositories(consumer RepositoryConsumerID) (runtimecommon.MemoryRepositories, *cd.Error) {
	provider, err := resolveRepositoryProvider(consumer, repositoryFamilyMemory)
	if err != nil {
		return runtimecommon.MemoryRepositories{}, err
	}
	return provider.Memory(), nil
}

func ResolveOrchestratorRepositories(consumer RepositoryConsumerID) (runtimecommon.OrchestratorRepositories, *cd.Error) {
	provider, err := resolveRepositoryProvider(consumer, repositoryFamilyOrchestrator)
	if err != nil {
		return runtimecommon.OrchestratorRepositories{}, err
	}
	return provider.Orchestrator(), nil
}

func ResolveResultRepositories(consumer RepositoryConsumerID) (runtimecommon.ResultRepositories, *cd.Error) {
	provider, err := resolveRepositoryProvider(consumer, repositoryFamilyResult)
	if err != nil {
		return runtimecommon.ResultRepositories{}, err
	}
	return provider.Result(), nil
}

func ResolveRunRepositories(consumer RepositoryConsumerID) (runtimecommon.RunRepositories, *cd.Error) {
	provider, err := resolveRepositoryProvider(consumer, repositoryFamilyRun)
	if err != nil {
		return runtimecommon.RunRepositories{}, err
	}
	return provider.Run(), nil
}

func ResolveSkillRepositories(consumer RepositoryConsumerID) (runtimecommon.SkillRepositories, *cd.Error) {
	provider, err := resolveRepositoryProvider(consumer, repositoryFamilySkill)
	if err != nil {
		return runtimecommon.SkillRepositories{}, err
	}
	return provider.Skill(), nil
}

func ResolveSoulRepositories(consumer RepositoryConsumerID) (runtimecommon.SoulRepositories, *cd.Error) {
	provider, err := resolveRepositoryProvider(consumer, repositoryFamilySoul)
	if err != nil {
		return runtimecommon.SoulRepositories{}, err
	}
	return provider.Soul(), nil
}

func resolveRepositoryProvider(consumer RepositoryConsumerID, requestedFamily repositoryFamily) (runtimecommon.RepositoryProvider, *cd.Error) {
	if err := validateRepositoryAccess(consumer, requestedFamily); err != nil {
		return nil, err
	}
	var helper runtimecommon.Provider
	runtime, err := initiator.GetEntity(runtimecommon.InitiatorID, helper)
	if err != nil {
		return nil, err
	}
	provider := runtime.RepositoryProvider()
	if provider == nil {
		return nil, cd.NewError(cd.IllegalParam, fmt.Sprintf("invalid argument: missing workorch repository provider for %s", consumer))
	}
	return provider, nil
}

func validateRepositoryAccess(consumer RepositoryConsumerID, requestedFamily repositoryFamily) *cd.Error {
	assignedFamily, found := repositoryFamilyForConsumer(consumer)
	if !found || assignedFamily != requestedFamily {
		return cd.NewError(cd.IllegalParam, fmt.Sprintf("invalid argument: repository consumer %q is not authorized for %s repositories", consumer, requestedFamily))
	}
	return nil
}

func repositoryFamilyForConsumer(consumer RepositoryConsumerID) (repositoryFamily, bool) {
	switch consumer {
	case repositoryConsumerWorkspace:
		return repositoryFamilyWorkspace, true
	case repositoryConsumerDecision:
		return repositoryFamilyDecision, true
	case repositoryConsumerApproval:
		return repositoryFamilyApproval, true
	case repositoryConsumerEventStream:
		return repositoryFamilyEventStream, true
	case repositoryConsumerGovernance:
		return repositoryFamilyGovernance, true
	case repositoryConsumerMCP:
		return repositoryFamilyMCP, true
	case repositoryConsumerMemory:
		return repositoryFamilyMemory, true
	case repositoryConsumerOrchestrator:
		return repositoryFamilyOrchestrator, true
	case repositoryConsumerResult:
		return repositoryFamilyResult, true
	case repositoryConsumerRun:
		return repositoryFamilyRun, true
	case repositoryConsumerSkill:
		return repositoryFamilySkill, true
	case repositoryConsumerSoul:
		return repositoryFamilySoul, true
	default:
		return "", false
	}
}
