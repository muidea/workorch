package web

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	magictask "github.com/muidea/magicCommon/task"
	approvalevents "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/events"
	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	webbiz "github.com/muidea/workorch/internal/modules/blocks/web/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/web/pkg/common"
	webmodels "github.com/muidea/workorch/internal/modules/blocks/web/pkg/models"
	webproviderconfig "github.com/muidea/workorch/internal/modules/blocks/web/pkg/providerconfig"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/runtimeevent"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
)

const ModuleID = modulecommon.ModuleID
const StepKindSearch = modulecommon.StepKindSearch
const StepKindFetch = modulecommon.StepKindFetch

type Dependencies struct {
	EventHub   magicevent.Hub
	Background magictask.BackgroundRoutine
}

type Module struct {
	runtimeAdapter *webbiz.RuntimeAdapter
	executor       *runtimeevent.Executor
	eventHub       magicevent.Hub
	background     magictask.BackgroundRoutine
}

func New() *Module { return &Module{} }

func (m *Module) ID() string { return ModuleID }

func (m *Module) Configure(deps Dependencies) error {
	m.eventHub = deps.EventHub
	m.background = deps.Background
	m.runtimeAdapter = webbiz.NewRuntimeAdapterFromConfigLoaderWithRevision(loadProviderConfigsFromWorkorchConfig, workorchconfig.Revision, webApprovalGetter{hub: deps.EventHub})
	return nil
}

func (m *Module) Start() error {
	if m == nil || m.runtimeAdapter == nil || m.eventHub == nil || m.background == nil {
		return common.ErrInvalidArgument
	}
	if m.executor != nil {
		return nil
	}
	m.executor = runtimeevent.NewExecutor(ModuleID, m.eventHub, m.background, m.runtimeAdapter, StepKindSearch, StepKindFetch)
	return m.executor.Run()
}

func (m *Module) Stop(ctx context.Context) error {
	_ = ctx
	if m.executor != nil {
		m.executor.Teardown()
	}
	m.executor = nil
	m.runtimeAdapter = nil
	m.eventHub = nil
	m.background = nil
	return nil
}

func loadProviderConfigsFromWorkorchConfig() (string, map[string]webmodels.ProviderConfig, error) {
	cfg, err := workorchconfig.Current()
	if err != nil {
		return "", nil, err
	}
	return webproviderconfig.LoadProviderConfigs(cfg.WebSearch.DefaultProvider, cfg.WebSearchProviderValueSets())
}

type webApprovalGetter struct {
	hub magicevent.Hub
}

func (g webApprovalGetter) Get(ctx context.Context, id common.ApprovalID) (*approvalmodel.Approval, error) {
	return g.GetForRun(ctx, id, "")
}

func (g webApprovalGetter) GetForRun(ctx context.Context, id common.ApprovalID, runID common.RunID) (*approvalmodel.Approval, error) {
	if g.hub == nil || id == "" {
		return nil, common.ErrInvalidArgument
	}
	ev := magicevent.NewEventWithContext(
		approvalevents.TopicApprovalCommandGet,
		ModuleID,
		approvalevents.CommandDestination,
		magicevent.NewHeader(),
		ctx,
		approvalevents.GetApprovalCommand{RunID: runID, ApprovalID: id},
	)
	result := g.hub.Send(ev)
	if result == nil {
		return nil, common.ErrInvalidArgument
	}
	if err := result.Error(); err != nil {
		return nil, err
	}
	data, err := result.Get()
	if err != nil {
		return nil, err
	}
	response, ok := data.(approvalevents.ApprovalCommandResult)
	if !ok || response.Approval == nil {
		return nil, common.ErrNotFound
	}
	return response.Approval, nil
}
