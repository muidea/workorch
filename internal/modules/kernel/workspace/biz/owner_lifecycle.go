package workspace

import (
	"context"
	"sync"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
)

type ownerSubscription struct {
	topic   string
	matched bool
}

func (m *Biz) BeginShutdown(context.Context) {
	if m == nil {
		return
	}
	m.shutdownMu.Lock()
	m.lifecycle.BeginShutdown()
	m.runProjectionStateMu.Lock()
	cancel := m.runProjectionCancel
	m.runProjectionStateMu.Unlock()
	m.shutdownMu.Unlock()
	if cancel != nil {
		cancel()
	}
}
func (m *Biz) Quiesce(ctx context.Context) error {
	if m == nil {
		return nil
	}
	return m.lifecycle.Quiesce(ctx)
}
func (m *Biz) enterOwner(drain bool) (func(), error) {
	m.shutdownMu.Lock()
	defer m.shutdownMu.Unlock()
	if m.releasing || m.released {
		return nil, basebiz.ErrLifecycleClosing
	}
	if drain {
		return m.lifecycle.EnterDrain()
	}
	return m.lifecycle.Enter()
}
func (m *Biz) subscribeOwner(topic string, handler magicevent.ObserverFunc, matched bool) error {
	drain := false
	switch topic {
	case workspaceevents.TopicWorkspaceCommandGet, workspaceevents.TopicWorkspaceCommandList,
		workspaceevents.TopicWorkspaceCommandGetGoal, workspaceevents.TopicWorkspaceCommandListGoalsByWS,
		workspaceevents.TopicWorkspaceCommandGetDoD, workspaceevents.TopicWorkspaceCommandListDoDsByWS,
		workspaceevents.TopicWorkspaceTemplateCommandGet, workspaceevents.TopicWorkspaceTemplateCommandList,
		workspaceevents.TopicWorkspaceTemplateCommandQueryBuiltinDefaults, workspaceevents.TopicWorkspaceFactTerminalDelivery,
		workspaceevents.TopicWorkspaceDerivationComplete, workspaceevents.TopicWorkspaceAuthoringSpecificationComplete,
		workspaceevents.TopicWorkspaceSpecificationComplete, workspaceevents.TopicWorkspacePreparationDiagnosticRecord,
		workspaceevents.TopicWorkspaceDefinitionChangeQuery,
		workspaceevents.TopicWorkspaceExactConfirmedStartupQuery,
		workspaceevents.TopicWorkspacePreparationStateQuery,
		workspaceevents.TopicWorkspaceDefinitionAuthoringQuery, workspaceevents.TopicWorkspaceAuthoringDiagnosticQuery,
		workspaceevents.TopicWorkspaceAuthoringSuccessAuditQuery, workspaceevents.TopicWorkspacePreparationDiagnosticQuery,
		workspaceevents.TopicWorkspaceAuthoringResourceScopeQuery, runevents.TopicRunStateChanged, runevents.TopicRunRuntimeCommandsReady:
		drain = true
	}
	wrapped := func(ev magicevent.Event, result magicevent.Result) {
		if ev == nil || (!matched && result == nil) {
			return
		}
		leave, err := m.enterOwner(drain)
		if err != nil {
			if result != nil {
				result.Set(nil, cd.WrapError(cd.ServiceUnavailable, err, "workspace owner is shutting down"))
			}
			return
		}
		defer leave()
		handler(ev, result)
	}
	var err error
	if matched {
		err = m.SubscribeMatchedFuncChecked(ModuleID+"/run-projection", runevents.NotificationDestination, topic, wrapped)
	} else {
		err = m.SubscribeFuncChecked(topic, wrapped)
	}
	if err != nil {
		return err
	}
	m.subscriptions = append(m.subscriptions, ownerSubscription{topic: topic, matched: matched})
	return nil
}

// 已接纳 AsyncFunction 持票到真实执行结束；提交失败才由提交方释放。
func (m *Biz) scheduleProjectionWork(fn func()) error {
	leave, err := m.enterOwner(false)
	if err != nil {
		return err
	}
	var once sync.Once
	finish := func() { once.Do(leave) }
	if err := m.BackgroundRoutine().AsyncFunction(func() { defer finish(); fn() }); err != nil {
		finish()
		return err
	}
	return nil
}
