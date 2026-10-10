package memory

import (
	"context"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	memoryevents "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/events"
)

// BeginShutdown 拒绝新资源修改，保留查询与已接纳 Run 的 recall 交付。
func (m *Biz) BeginShutdown(context.Context) {
	if m != nil {
		m.lifecycle.BeginShutdown()
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
	case memoryevents.TopicMemoryCommandGet, memoryevents.TopicMemoryCommandListByWorkspace, memoryevents.TopicMemoryCommandSearch, memoryevents.TopicMemoryRecallTraceCommandList, memoryevents.TopicMemoryRecallTraceCommandUpsert:
		drain = true
	}
	// Recall 交付仍由原 handler 校验 exact Run source、scope、不可变内容及删除 fence。
	err := m.SubscribeFuncChecked(topic, func(ev magicevent.Event, result magicevent.Result) {
		if ev == nil || result == nil {
			return
		}
		leave, err := m.enterOwner(drain)
		if err != nil {
			result.Set(nil, cd.WrapError(cd.ServiceUnavailable, err, "memory owner is shutting down"))
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
