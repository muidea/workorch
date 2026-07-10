package activationports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/activationcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	soulmodel "github.com/muidea/workorch/internal/modules/kernel/soul/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type WorkspaceSoulRoutingRequest struct {
	Hub                 magicevent.Hub
	Workspace           *workspacemodel.Workspace
	RunDefaultSoulID    string
	RunSoulBindings     map[string]string
	SystemDefaultSoulID common.SoulID
}

type WorkspaceSoulRoutingResult struct {
	DefaultSoul      *soulmodel.Soul
	Routing          appmodelcore.SoulRouting
	EffectiveSoulIDs []common.SoulID
}

func ResolveWorkspaceSoulRouting(ctx context.Context, request WorkspaceSoulRoutingRequest) (WorkspaceSoulRoutingResult, error) {
	workspace := request.Workspace
	if workspace == nil {
		return WorkspaceSoulRoutingResult{}, common.ErrInvalidArgument
	}
	runStageBindings, err := appmodelcore.NormalizeSoulBindings(request.RunSoulBindings)
	if err != nil {
		return WorkspaceSoulRoutingResult{}, err
	}
	defaults := workspaceSoulRoutingDefaults(request, runStageBindings)
	if request.Hub == nil {
		if defaults.requiresSoulLookup {
			return WorkspaceSoulRoutingResult{}, common.ErrInvalidArgument
		}
		return WorkspaceSoulRoutingResult{Routing: appmodelcore.UnboundSoulRouting("soul event hub is not configured")}, nil
	}

	loader := newSoulRecordLoader(ctx, request.Hub)
	defaultSoul, err := loader.Load(defaults.currentDefaultSoulID)
	if err != nil {
		return WorkspaceSoulRoutingResult{}, err
	}

	bindings := map[string]appmodelcore.SoulRoutingBinding{}
	effectiveSoulIDs := make([]common.SoulID, 0, len(appmodelcore.SoulBindingStages())+1)
	for _, stage := range appmodelcore.SoulBindingStages() {
		soulID, source := appmodelcore.StageSoulBinding(stage, runStageBindings, workspace.SoulBindings, defaults.defaultSoulID, defaults.defaultSource)
		if soulID == "" {
			continue
		}
		soul, err := loader.Load(soulID)
		if err != nil {
			return WorkspaceSoulRoutingResult{}, err
		}
		bindings[stage] = appmodelcore.SoulRoutingBinding{
			SoulID: string(soulID),
			Source: source,
			Soul:   activationcore.SoulContext(soul),
		}
		effectiveSoulIDs = appmodelcore.AppendSoulIDOnce(effectiveSoulIDs, soulID)
	}
	if defaults.defaultSoulID != "" {
		effectiveSoulIDs = appmodelcore.AppendSoulIDOnce(effectiveSoulIDs, defaults.defaultSoulID)
	}
	if len(bindings) == 0 && defaults.defaultSoulID == "" {
		return WorkspaceSoulRoutingResult{Routing: appmodelcore.UnboundSoulRouting("no run, workspace, or system soul configured")}, nil
	}
	return WorkspaceSoulRoutingResult{
		DefaultSoul: defaultSoul,
		Routing: appmodelcore.SoulRouting{
			Status:                 "resolved",
			PriorityPolicy:         appmodelcore.SoulRoutingPriorityPolicy(),
			EffectiveDefaultSoulID: string(defaults.defaultSoulID),
			Bindings:               bindings,
		},
		EffectiveSoulIDs: effectiveSoulIDs,
	}, nil
}
