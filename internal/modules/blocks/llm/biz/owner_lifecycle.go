package llm

import (
	"context"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
)

type ownerSubscription struct {
	topic   string
	matched bool
}

// BeginShutdown 关闭新请求，保留在途 provider 调用、审计及冻结配置。
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

func (m *Biz) subscribeOwner(topic string, handler magicevent.ObserverFunc, matched bool) error {
	// 普通 catalog/selection/ceiling 可触发 refresh；cache-only 观察不加入该路径。
	drain := topic == llmevents.TopicLLMCommandGetActiveConfiguration || topic == llmevents.TopicObserveProviderCatalogCache
	wrapped := func(ev magicevent.Event, result magicevent.Result) {
		if ev == nil || result == nil {
			return
		}
		leave, err := m.enterOwner(drain)
		if err != nil {
			result.Set(nil, cd.WrapError(cd.ServiceUnavailable, err, "llm owner is shutting down"))
			return
		}
		defer leave()
		handler(ev, result)
	}
	var err error
	if matched {
		err = m.SubscribeMatchedFuncChecked(ModuleID+"/prerequisite", orchestratorevents.PrerequisiteCommandDestination, topic, wrapped)
	} else {
		err = m.SubscribeFuncChecked(topic, wrapped)
	}
	if err != nil {
		return err
	}
	m.subscriptions = append(m.subscriptions, ownerSubscription{topic: topic, matched: matched})
	return nil
}
