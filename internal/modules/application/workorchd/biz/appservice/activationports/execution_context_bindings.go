package activationports

import (
	"context"
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	memorymodel "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/models"
	soulmodel "github.com/muidea/workorch/internal/modules/kernel/soul/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type workspaceExecutionContextBindings struct {
	defaultSoul       *soulmodel.Soul
	memories          []*memorymodel.MemoryItem
	retrievalEvidence map[string]any
	governanceHistory []map[string]any
	soulRouting       appmodelcore.SoulRouting
}

func loadWorkspaceExecutionContextBindings(ctx context.Context, request WorkspaceExecutionContextRequest, effectiveGoal *workspacemodel.Goal, effectiveDoD *workspacemodel.DoD, memoryIDs []string) (workspaceExecutionContextBindings, error) {
	workspace := request.Workspace
	command := request.Command
	ret := workspaceExecutionContextBindings{}
	err := readmodelcore.RunTasks(
		func() error {
			result, err := ResolveWorkspaceSoulRouting(ctx, WorkspaceSoulRoutingRequest{
				Hub:                 request.Hub,
				Workspace:           workspace,
				RunDefaultSoulID:    command.SoulID,
				RunSoulBindings:     command.SoulBindings,
				SystemDefaultSoulID: common.SoulID(strings.TrimSpace(string(request.SystemDefaultSoulID))),
			})
			if err != nil {
				return err
			}
			ret.defaultSoul = result.DefaultSoul
			ret.soulRouting = result.Routing
			return nil
		},
		func() error {
			memories, retrievalEvidence, err := SelectedWorkspaceMemoryItems(ctx, WorkspaceMemorySelectionRequest{
				WorkspaceID:                    workspace.ID,
				Hub:                            request.Hub,
				IDs:                            memoryIDs,
				Goal:                           effectiveGoal,
				DoD:                            effectiveDoD,
				DefaultEmbeddingProviderLoader: request.DefaultEmbeddingProviderLoader,
			})
			if err != nil {
				return err
			}
			ret.memories = memories
			ret.retrievalEvidence = retrievalEvidence
			return nil
		},
		func() error {
			ret.governanceHistory = WorkspaceGovernanceHistory(ctx, request.Hub, workspace.ID, effectiveGoal.ID, effectiveDoD.ID, 10)
			return nil
		},
	)
	return ret, err
}
