package soul

import (
	"context"
	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	soulevents "github.com/muidea/workorch/internal/modules/blocks/soul/pkg/events"
)

// BeginShutdown 停止新写入，保留既有事务的只读查询与校验。
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
	case soulevents.TopicSoulCommandGet, soulevents.TopicSoulCommandList, soulevents.TopicSoulCommandValidateCapability:
		drain = true
	}
	err := m.SubscribeFuncChecked(topic, func(ev magicevent.Event, result magicevent.Result) {
		if ev == nil || result == nil {
			return
		}
		leave, err := m.enterOwner(drain)
		if err != nil {
			result.Set(nil, cd.WrapError(cd.ServiceUnavailable, err, "soul owner is shutting down"))
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
