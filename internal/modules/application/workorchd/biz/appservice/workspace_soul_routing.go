package appservice

import (
	"context"
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/bootstrap"
	soulmodel "github.com/muidea/workorch/internal/modules/kernel/soul/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func resolveWorkspaceSoulRouting(ctx context.Context, exports bootstrap.Exports, workspace *workspacemodel.Workspace, cmd ActivateWorkspaceExecutionCommand) (*soulmodel.Soul, SoulRouting, []common.SoulID, error) {
	if workspace == nil {
		return nil, SoulRouting{}, nil, common.ErrInvalidArgument
	}
	runStageBindings, err := normalizeSoulBindings(cmd.SoulBindings)
	if err != nil {
		return nil, SoulRouting{}, nil, err
	}
	systemDefaultSoulID, err := configuredSystemDefaultSoulID(ctx)
	if err != nil {
		return nil, SoulRouting{}, nil, err
	}
	if exports.Repositories.Souls == nil {
		if strings.TrimSpace(cmd.SoulID) != "" || workspace.DefaultSoulID != "" || systemDefaultSoulID != "" || len(workspace.SoulBindings) > 0 || len(runStageBindings) > 0 {
			return nil, SoulRouting{}, nil, common.ErrInvalidArgument
		}
		return nil, unboundSoulRouting("soul repository is not configured"), nil, nil
	}

	souls := map[common.SoulID]*soulmodel.Soul{}
	loadSoul := func(id common.SoulID) (*soulmodel.Soul, error) {
		if id == "" {
			return nil, nil
		}
		if item, ok := souls[id]; ok {
			return item, nil
		}
		item, err := exports.Repositories.Souls.Get(ctx, id)
		if err != nil {
			return nil, err
		}
		souls[id] = item
		return item, nil
	}

	runDefaultSoulID := common.SoulID(strings.TrimSpace(cmd.SoulID))
	workspaceDefaultSoulID := workspace.DefaultSoulID
	hasStageBindings := len(runStageBindings) > 0 || len(workspace.SoulBindings) > 0
	defaultSoulID := runDefaultSoulID
	defaultSource := "run_default"
	if defaultSoulID == "" {
		defaultSoulID = workspaceDefaultSoulID
		defaultSource = "workspace_default"
	}
	if defaultSoulID == "" {
		defaultSoulID = systemDefaultSoulID
		defaultSource = "system_default"
	}
	currentDefaultSoulID := defaultSoulID
	if runDefaultSoulID == "" && workspaceDefaultSoulID == "" && hasStageBindings {
		currentDefaultSoulID = ""
	}
	defaultSoul, err := loadSoul(currentDefaultSoulID)
	if err != nil {
		return nil, SoulRouting{}, nil, err
	}

	bindings := map[string]SoulRoutingBinding{}
	effectiveSoulIDs := make([]common.SoulID, 0, len(soulBindingStages)+1)
	for _, stage := range soulBindingStages {
		soulID, source := stageSoulBinding(stage, runStageBindings, workspace.SoulBindings, defaultSoulID, defaultSource)
		if soulID == "" {
			continue
		}
		soul, err := loadSoul(soulID)
		if err != nil {
			return nil, SoulRouting{}, nil, err
		}
		bindings[stage] = SoulRoutingBinding{
			SoulID: string(soulID),
			Source: source,
			Soul:   soulContext(soul),
		}
		effectiveSoulIDs = appendSoulIDOnce(effectiveSoulIDs, soulID)
	}
	if defaultSoulID != "" {
		effectiveSoulIDs = appendSoulIDOnce(effectiveSoulIDs, defaultSoulID)
	}
	if len(bindings) == 0 && defaultSoulID == "" {
		return nil, unboundSoulRouting("no run, workspace, or system soul configured"), nil, nil
	}
	return defaultSoul, SoulRouting{
		Status:                 "resolved",
		PriorityPolicy:         cloneStrings(soulRoutingPriorityPolicy),
		EffectiveDefaultSoulID: string(defaultSoulID),
		Bindings:               bindings,
	}, effectiveSoulIDs, nil
}

func stageSoulBinding(stage string, runStageBindings map[string]common.SoulID, workspaceStageBindings map[string]common.SoulID, defaultSoulID common.SoulID, defaultSource string) (common.SoulID, string) {
	if value := runStageBindings[stage]; value != "" {
		return value, "run_stage_override"
	}
	if value := workspaceStageBindings[stage]; value != "" {
		return value, "workspace_stage_binding"
	}
	if defaultSoulID != "" {
		return defaultSoulID, defaultSource
	}
	return "", ""
}

func unboundSoulRouting(reason string) SoulRouting {
	return SoulRouting{
		Status:         "unbound",
		Reason:         reason,
		PriorityPolicy: cloneStrings(soulRoutingPriorityPolicy),
		Bindings:       map[string]SoulRoutingBinding{},
	}
}

func appendSoulIDOnce(input []common.SoulID, value common.SoulID) []common.SoulID {
	if value == "" {
		return input
	}
	for _, item := range input {
		if item == value {
			return input
		}
	}
	return append(input, value)
}

func configuredSystemDefaultSoulID(ctx context.Context) (common.SoulID, error) {
	cfg, err := loadConfigFromContext(ctx)
	if err != nil {
		return "", err
	}
	return common.SoulID(strings.TrimSpace(cfg.Runtime.DefaultSoulID)), nil
}
