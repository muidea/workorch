package result

import (
	"context"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	resultevents "github.com/muidea/workorch/internal/modules/blocks/result/pkg/events"
)

// BeginShutdown 关闭新删除和恢复调度，保留终态交付、查询及在途依赖。
func (m *Biz) BeginShutdown(context.Context) {
	if m == nil {
		return
	}
	m.shutdownMu.Lock()
	m.lifecycle.BeginShutdown()
	cancel := m.notificationRecoveryCancel
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
func (m *Biz) subscribeOwner(topic string, handler magicevent.ObserverFunc) error {
	drain := false
	switch topic {
	case resultevents.TopicResultCommandGet, resultevents.TopicResultCommandListByWorkspace, resultevents.TopicResultCommandListByRun,
		resultevents.TopicArtifactCommandListByWorkspace, resultevents.TopicArtifactCommandListByRun, resultevents.TopicArtifactCommandListByResult,
		resultevents.TopicArtifactCommandReadContent, resultevents.TopicResultFactTerminalDelivery:
		drain = true
	}
	// 原 terminal handler 继续校验 exact Orchestrator source 与 sealed material。
	err := m.SubscribeFuncChecked(topic, func(ev magicevent.Event, result magicevent.Result) {
		if ev == nil || result == nil {
			return
		}
		leave, err := m.enterOwner(drain)
		if err != nil {
			result.Set(nil, cd.WrapError(cd.ServiceUnavailable, err, "result owner is shutting down"))
			return
		}
		defer leave()
		handler(ev, result)
	})
	if err != nil {
		return err
	}
	m.topics = append(m.topics, topic)
	return nil
}
