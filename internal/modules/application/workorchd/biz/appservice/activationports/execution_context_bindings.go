package activationports

import (
	"context"
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	soulmodel "github.com/muidea/workorch/internal/modules/blocks/soul/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type workspaceRuntimeActivationBindings struct {
	defaultSoul       *soulmodel.Soul
	stageSouls        map[string]*soulmodel.Soul
	memories          []*memorymodel.MemoryItem
	memorySelection   *MemorySelectionEvidence
	governanceHistory []runtimecontract.RuntimeGovernanceHistoryEntry
	soulRouting       appmodelcore.SoulRouting
}

func loadWorkspaceRuntimeActivationBindings(ctx context.Context, request workspaceRuntimeActivationRequest, effectiveGoal *workspacemodel.Goal, effectiveDoD *workspacemodel.DoD) (workspaceRuntimeActivationBindings, error) {
	workspace := request.Workspace
	ret := workspaceRuntimeActivationBindings{}
	err := readmodelcore.RunTasks(
		func() error {
			result, err := ResolveWorkspaceSoulRouting(ctx, WorkspaceSoulRoutingRequest{
				Hub:                 request.Hub,
				Workspace:           workspace,
				SystemDefaultSoulID: common.SoulID(strings.TrimSpace(string(request.SystemDefaultSoulID))),
			})
			if err != nil {
				return err
			}
			ret.defaultSoul = result.DefaultSoul
			ret.stageSouls = result.StageSouls
			ret.soulRouting = result.Routing
			return nil
		},
		func() error {
			memories, evidence, err := SelectedWorkspaceMemoryItems(ctx, WorkspaceMemorySelectionRequest{
				WorkspaceID: workspace.ID,
				Hub:         request.Hub,

				Goal:                           effectiveGoal,
				DoD:                            effectiveDoD,
				DefaultEmbeddingProviderLoader: request.DefaultEmbeddingProviderLoader,
			})
			if err != nil {
				return err
			}
			ret.memories = memories
			ret.memorySelection = evidence
			return nil
		},
		func() error {
			ret.governanceHistory = WorkspaceGovernanceHistory(ctx, request.Hub, workspace.ID, effectiveGoal.ID, effectiveDoD.ID, 10)
			return nil
		},
	)
	return ret, err
}
