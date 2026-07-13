package web

import (
	"context"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	approvalevents "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/events"
	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	webbiz "github.com/muidea/workorch/internal/modules/blocks/web/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/web/pkg/common"
	webmodels "github.com/muidea/workorch/internal/modules/blocks/web/pkg/models"
	webproviderconfig "github.com/muidea/workorch/internal/modules/blocks/web/pkg/providerconfig"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
)

const ModuleID = modulecommon.ModuleID
const StepKindSearch = modulecommon.StepKindSearch
const StepKindFetch = modulecommon.StepKindFetch

type Dependencies struct {
	EventHub magicevent.Hub
}

type Module struct {
	runtimeAdapter     *webbiz.RuntimeAdapter
	capabilityObserver magicevent.SimpleObserver
	eventHub           magicevent.Hub
}

func New() *Module { return &Module{} }

func (m *Module) ID() string { return ModuleID }

func (m *Module) Configure(deps Dependencies) error {
	m.eventHub = deps.EventHub
	m.runtimeAdapter = webbiz.NewRuntimeAdapterFromConfigLoaderWithRevision(loadProviderConfigsFromWorkorchConfig, workorchconfig.Revision, webApprovalGetter{hub: deps.EventHub})
	return nil
}

func (m *Module) Start() error {
	if m == nil || m.runtimeAdapter == nil || m.eventHub == nil {
		return common.ErrInvalidArgument
	}
	if m.capabilityObserver != nil {
		return nil
	}
	m.capabilityObserver = magicevent.NewSimpleObserver(ModuleID, m.eventHub)
	m.capabilityObserver.Subscribe(orchestratorevents.TopicOrchestratorAcquireRuntimeCapabilities, m.handleRuntimeCapabilities)
	m.capabilityObserver.Subscribe(orchestratorevents.TopicOrchestratorExecuteRuntimeOperation, m.handleRuntimeOperation)
	return nil
}

func (m *Module) Stop(ctx context.Context) error {
	_ = ctx
	if m.capabilityObserver != nil {
		m.capabilityObserver.Unsubscribe(orchestratorevents.TopicOrchestratorExecuteRuntimeOperation)
		m.capabilityObserver.Unsubscribe(orchestratorevents.TopicOrchestratorAcquireRuntimeCapabilities)
	}
	m.capabilityObserver = nil
	m.runtimeAdapter = nil
	m.eventHub = nil
	return nil
}

func (m *Module) handleRuntimeOperation(event magicevent.Event, result magicevent.Result) {
	command, ok := event.Data().(orchestratorevents.ExecuteRuntimeOperationCommand)
	if !ok || m.runtimeAdapter == nil {
		setCapabilityError(result, common.ErrInvalidArgument)
		return
	}
	response, err := orchestratorevents.ExecuteRuntimeOperation(event.Context(), command, m.runtimeAdapter)
	if err != nil {
		setCapabilityError(result, err)
		return
	}
	result.Set(response, nil)
}

func (m *Module) handleRuntimeCapabilities(event magicevent.Event, result magicevent.Result) {
	command, ok := event.Data().(orchestratorevents.AcquireRuntimeCapabilitiesCommand)
	if !ok {
		setCapabilityError(result, common.ErrInvalidArgument)
		return
	}
	response, err := orchestratorevents.ResolveRuntimeCapabilities(command, ModuleID, StepKindSearch, StepKindFetch)
	if err != nil {
		setCapabilityError(result, err)
		return
	}
	result.Set(response, nil)
}

func setCapabilityError(result magicevent.Result, err error) {
	if result != nil {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
	}
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
