package skill

import (
	"context"
	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	skillevents "github.com/muidea/workorch/internal/modules/blocks/skill/pkg/events"
)

// BeginShutdown 只关闭新写入，不提前退订既有事务所需的查询。
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

// 准入票覆盖真实 handler 返回；Send 取消不能代替完成回执。
func (m *Biz) subscribeOwner(topic string, handler magicevent.ObserverFunc) error {
	drain := topic == skillevents.TopicSkillCommandGet || topic == skillevents.TopicSkillCommandList
	err := m.SubscribeFuncChecked(topic, func(ev magicevent.Event, result magicevent.Result) {
		if ev == nil || result == nil {
			return
		}
		leave, err := m.enterOwner(drain)
		if err != nil {
			result.Set(nil, cd.WrapError(cd.ServiceUnavailable, err, "skill owner is shutting down"))
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
