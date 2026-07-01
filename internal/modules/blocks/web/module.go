package web

import (
	"context"

	magictask "github.com/muidea/magicCommon/task"
	"github.com/muidea/workorch/internal/pkg/runtimeevent"
	webbiz "github.com/muidea/workorch/internal/modules/blocks/web/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/web/pkg/common"
	webmodels "github.com/muidea/workorch/internal/modules/blocks/web/pkg/models"
	webproviderconfig "github.com/muidea/workorch/internal/modules/blocks/web/pkg/providerconfig"
	approvalevents "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/events"
	approvalmodel "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
)

const ModuleID = modulecommon.ModuleID
const StepKindSearch = modulecommon.StepKindSearch
const StepKindFetch = modulecommon.StepKindFetch

type Dependencies struct {
	EventHub   events.Hub
	Background magictask.BackgroundRoutine
}

type Module struct {
	runtimeAdapter *webbiz.RuntimeAdapter
	executor       *runtimeevent.Executor
	eventHub       events.Hub
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
	hub events.Hub
}

func (g webApprovalGetter) Get(ctx context.Context, id common.ApprovalID) (*approvalmodel.Approval, error) {
	return g.GetForRun(ctx, id, "")
}

func (g webApprovalGetter) GetForRun(ctx context.Context, id common.ApprovalID, runID common.RunID) (*approvalmodel.Approval, error) {
	if g.hub == nil || id == "" {
		return nil, common.ErrInvalidArgument
	}
	err := g.hub.Publish(ctx, events.Envelope{
		Topic:        approvalevents.TopicApprovalCommandGet,
		SourceModule: ModuleID,
		SubjectType:  "approval",
		SubjectID:    string(id),
		RunID:        string(runID),
		Payload: map[string]any{
			events.PayloadKeyApprovalID: string(id),
			events.PayloadKeyRunID:      string(runID),
		},
	})
	if response, ok := events.ResponseEnvelope(err); ok {
		item, ok := response.Payload[events.PayloadKeyApproval].(*approvalmodel.Approval)
		if !ok || item == nil {
			return nil, common.ErrNotFound
		}
		return item, nil
	}
	if err != nil {
		return nil, err
	}
	return nil, common.ErrNotFound
}
