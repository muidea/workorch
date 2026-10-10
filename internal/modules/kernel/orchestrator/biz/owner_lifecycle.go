package orchestrator

import (
	"context"
	"sync"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
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
	m.ownerLifecycle.BeginShutdown()
	m.driveMu.Lock()
	if !m.runtimeLeaseTimerClosing {
		m.runtimeLeaseTimerClosing = true
		m.runtimeLeaseTimerEpoch++
	}
	cancel, timerCancel := m.lifecycleCancel, m.runtimeLeaseTimerCancel
	if m.runtimeDriveGateSettlementWake.Timer != nil {
		m.runtimeDriveGateSettlementWake.Timer.Stop()
	}
	m.driveMu.Unlock()
	m.shutdownMu.Unlock()
	if cancel != nil {
		cancel()
	}
	if timerCancel != nil {
		timerCancel()
	}
}

func (m *Biz) Quiesce(ctx context.Context) error {
	if m == nil {
		return nil
	}
	if err := m.ownerLifecycle.Quiesce(ctx); err != nil {
		return err
	}
	if err := m.lockLifecycleForStop(ctx); err != nil {
		return err
	}
	m.lifecycleMu.Unlock()
	return nil
}

func (m *Biz) enterOwner(drain bool) (func(), error) {
	m.shutdownMu.Lock()
	defer m.shutdownMu.Unlock()
	if m.releasing || m.released {
		return nil, basebiz.ErrLifecycleClosing
	}
	if drain {
		return m.ownerLifecycle.EnterDrain()
	}
	return m.ownerLifecycle.Enter()
}

func (m *Biz) subscribeOwner(topic string, handler magicevent.ObserverFunc, matched bool) error {
	drain := false
	switch topic {
	case orchestratorevents.TopicOrchestratorCommandListToolCallsByRun,
		orchestratorevents.TopicOrchestratorCommandListPatchesByRun,
		orchestratorevents.TopicOrchestratorCommandListValidationResultsByRun,
		orchestratorevents.TopicOrchestratorCommandListCoverageWorkItemsByRun,
		orchestratorevents.TopicOrchestratorCommandListArtifactContinuationsByRun,
		orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt,
		orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt,
		orchestratorevents.TopicOrchestratorCommitRuntimeGovernanceTicket,
		orchestratorevents.TopicOrchestratorCommitRuntimeGovernanceApproval,
		orchestratorevents.TopicOrchestratorCommandRecordDecisionEvidence,
		orchestratorevents.TopicOrchestratorCommandRecordApprovalEvidence:
		drain = true
	}
	wrapped := func(ev magicevent.Event, result magicevent.Result) {
		if ev == nil || (!matched && result == nil) {
			return
		}
		leave, err := m.enterOwner(drain)
		if err != nil {
			if result != nil {
				result.Set(nil, cd.WrapError(cd.ServiceUnavailable, err, "orchestrator owner is shutting down"))
			}
			return
		}
		defer leave()
		handler(ev, result)
	}
	var err error
	if matched {
		err = m.SubscribeMatchedFuncChecked(ModuleID+"/run-drive", runevents.NotificationDestination, topic, wrapped)
	} else {
		err = m.SubscribeFuncChecked(topic, wrapped)
	}
	if err != nil {
		return err
	}
	m.subscriptions = append(m.subscriptions, ownerSubscription{topic: topic, matched: matched})
	return nil
}

// 提交前持票，accepted 队列工作真实结束后才释放；拒绝不遗留票。
func (m *Biz) scheduleOwnerWork(submit func(func()) error, callback func()) error {
	leave, err := m.enterOwner(false)
	if err != nil {
		return err
	}
	var once sync.Once
	finish := func() { once.Do(leave) }
	if err := submit(func() { defer finish(); callback() }); err != nil {
		finish()
		return err
	}
	return nil
}
