package run

import (
	"context"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/pkg/common"
)

// enterOwnerCommand 区分新工作准入与 owner 验证后的收尾入口。
// 原 typed scope/source/revision/reducer 校验继续执行；EnterDrain 不授予业务权威。
func (m *Biz) enterOwnerCommand(drain bool) (func(), error) {
	if m == nil {
		return nil, common.ErrInvalidArgument
	}
	m.commandMu.RLock()
	defer m.commandMu.RUnlock()
	if m.stopping || m.runs == nil || m.EventHub() == nil {
		return nil, common.ErrConflict
	}
	if drain {
		return m.lifecycle.EnterDrain()
	}
	return m.lifecycle.Enter()
}

// Base 必需订阅的失败合同是明确的 *cd.Error panic，只转换该错误。
// 未知 panic 必须原样交给 framework guard；可能 panic 的 Hub 操作不得持有 commandMu。
func checkedOwnerSubscription(fn func()) (err error) {
	defer func() {
		if value := recover(); value != nil {
			if declared, ok := value.(*cd.Error); ok {
				err = declared
				return
			}
			panic(value)
		}
	}()
	fn()
	return nil
}

func (m *Biz) subscribeOwnerTopic(topic string, handler magicevent.ObserverFunc) error {
	m.commandMu.RLock()
	for _, existing := range m.topics {
		if existing == topic {
			m.commandMu.RUnlock()
			return nil
		}
	}
	m.commandMu.RUnlock()
	// 跟踪到 observer 真实返回，包含 response clone / Set。
	// 外层只记账，不授予业务准入：typed handler 仍区分 fresh/drain 并验证消息。
	tracked := func(ev magicevent.Event, result magicevent.Result) {
		leave, err := m.enterOwnerCommand(true)
		if err != nil {
			if result != nil {
				result.Set(nil, cd.WrapError(cd.Unexpected, err, "run owner is released"))
			}
			return
		}
		defer leave()
		handler(ev, result)
	}
	if err := checkedOwnerSubscription(func() { m.SubscribeFunc(topic, tracked) }); err != nil {
		return err
	}
	m.commandMu.Lock()
	m.topics = append(m.topics, topic)
	m.commandMu.Unlock()
	return nil
}

// BeginShutdown 非阻塞关闭新的 Start/Execute/recovery 准入，
// 不等待 handler、不退订、不清依赖，也不忘记已接受的任务。
func (m *Biz) BeginShutdown() {
	if m == nil {
		return
	}
	m.lifecycle.BeginShutdown()
	m.replacementRecoveryMu.Lock()
	cancel := m.replacementRecoveryCancel
	m.replacementRecoveryMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// Quiesce 只证明本 owner 的真实 handler/task 完成；不关闭已验证的
// drain 入口，也不替代应用层共享 task/Hub 排空屏障。
func (m *Biz) Quiesce(ctx context.Context) error {
	if m == nil {
		return nil
	}
	return m.lifecycle.Quiesce(ctx)
}
