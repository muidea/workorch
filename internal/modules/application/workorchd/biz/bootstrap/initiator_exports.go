package bootstrap

import (
	"strings"

	"github.com/muidea/workorch/internal/pkg/workorchconfig"
	"github.com/muidea/workorch/internal/pkg/workorchdeps"
)

func ExportsFromInitiator(consumer string) (Exports, error) {
	consumer = strings.TrimSpace(consumer)
	if consumer == "" {
		consumer = "application/workorchd bootstrap"
	}
	systemRepos, err := workorchdeps.ResolveSystemRepositories(consumer)
	if err != nil {
		return Exports{}, err
	}
	workspaceRepos, err := workorchdeps.ResolveWorkspaceRepositories(consumer)
	if err != nil {
		return Exports{}, err
	}
	memoryRepos, err := workorchdeps.ResolveMemoryRepositories(consumer)
	if err != nil {
		return Exports{}, err
	}
	governanceRepos, err := workorchdeps.ResolveGovernanceRepositories(consumer)
	if err != nil {
		return Exports{}, err
	}
	skillRepos, err := workorchdeps.ResolveSkillRepositories(consumer)
	if err != nil {
		return Exports{}, err
	}
	mcpRepos, err := workorchdeps.ResolveMCPRepositories(consumer)
	if err != nil {
		return Exports{}, err
	}
	resultRepos, err := workorchdeps.ResolveResultRepositories(consumer)
	if err != nil {
		return Exports{}, err
	}
	eventStreamRepos, err := workorchdeps.ResolveEventStreamRepositories(consumer)
	if err != nil {
		return Exports{}, err
	}
	runRepos, err := workorchdeps.ResolveRunRepositories(consumer)
	if err != nil {
		return Exports{}, err
	}
	orchestratorRepos, err := workorchdeps.ResolveOrchestratorRepositories(consumer)
	if err != nil {
		return Exports{}, err
	}
	decisionRepos, err := workorchdeps.ResolveDecisionRepositories(consumer)
	if err != nil {
		return Exports{}, err
	}
	approvalRepos, err := workorchdeps.ResolveApprovalRepositories(consumer)
	if err != nil {
		return Exports{}, err
	}
	eventHub, err := workorchdeps.ResolveEventHub(consumer)
	if err != nil {
		return Exports{}, err
	}
	return Exports{
		Repositories: RepositoryExports{
			Souls:                 systemRepos.Souls,
			WorkspaceTemplates:    systemRepos.Templates,
			MemoryItems:           memoryRepos.MemoryItems,
			MemoryRecallTraces:    memoryRepos.MemoryRecallTraces,
			AutoGovernanceTraces:  governanceRepos.AutoGovernanceTraces,
			Skills:                skillRepos.Skills,
			MCPServers:            mcpRepos.Servers,
			MCPCapabilities:       mcpRepos.Capabilities,
			Workspaces:            workspaceRepos.Workspaces,
			Goals:                 workspaceRepos.Goals,
			DoDs:                  workspaceRepos.DoDs,
			Results:               resultRepos.Results,
			Artifacts:             resultRepos.Artifacts,
			ArtifactContinuations: orchestratorRepos.ArtifactContinuations,
			EventRecords:          eventStreamRepos.EventRecords,
			Runs:                  runRepos.Runs,
			Steps:                 runRepos.Steps,
			ToolCalls:             orchestratorRepos.ToolCalls,
			Patches:               orchestratorRepos.Patches,
			ValidationResults:     orchestratorRepos.ValidationResults,
			RuntimeCheckpoints:    runRepos.Checkpoints,
			Tickets:               decisionRepos.Tickets,
			Resolutions:           decisionRepos.Resolutions,
			Approvals:             approvalRepos.Approvals,
		},
		Infrastructure: InfrastructureExports{
			EventHub:   eventHub,
			LoadConfig: workorchconfig.Current,
		},
	}, nil
}
