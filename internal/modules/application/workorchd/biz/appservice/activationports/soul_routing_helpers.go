package activationports

import (
	"context"
	"strings"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistryports"
	soulmodel "github.com/muidea/workorch/internal/modules/blocks/soul/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type soulRecordLoader struct {
	ctx   context.Context
	hub   magicevent.Hub
	cache map[common.SoulID]*soulmodel.Soul
}

func newSoulRecordLoader(ctx context.Context, hub magicevent.Hub) *soulRecordLoader {
	return &soulRecordLoader{
		ctx:   ctx,
		hub:   hub,
		cache: map[common.SoulID]*soulmodel.Soul{},
	}
}

func (l *soulRecordLoader) Load(id common.SoulID) (*soulmodel.Soul, error) {
	if id == "" {
		return nil, nil
	}
	if item, ok := l.cache[id]; ok {
		return item, nil
	}
	item, err := contextregistryports.ReadSoulRecord(l.ctx, l.hub, id)
	if err != nil {
		return nil, err
	}
	l.cache[id] = item
	return item, nil
}

type soulRoutingDefaults struct {
	requiresSoulLookup   bool
	defaultSoulID        common.SoulID
	defaultSource        string
	currentDefaultSoulID common.SoulID
}

func workspaceSoulRoutingDefaults(request WorkspaceSoulRoutingRequest, runStageBindings map[string]common.SoulID) soulRoutingDefaults {
	workspace := request.Workspace
	runDefaultSoulID := common.SoulID(strings.TrimSpace(request.RunDefaultSoulID))
	workspaceDefaultSoulID := workspace.DefaultSoulID
	systemDefaultSoulID := request.SystemDefaultSoulID
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
	return soulRoutingDefaults{
		requiresSoulLookup:   runDefaultSoulID != "" || workspaceDefaultSoulID != "" || systemDefaultSoulID != "" || len(workspace.SoulBindings) > 0 || len(runStageBindings) > 0,
		defaultSoulID:        defaultSoulID,
		defaultSource:        defaultSource,
		currentDefaultSoulID: currentDefaultSoulID,
	}
}
