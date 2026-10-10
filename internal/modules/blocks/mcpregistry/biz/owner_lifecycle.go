package mcp

import (
	"context"
	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	mcpevents "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/events"
)

// BeginShutdown 停止注册变更，保留既有执行所需的一致快照读取。
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
	case mcpevents.TopicMCPCommandGetServer, mcpevents.TopicMCPCommandListServers, mcpevents.TopicMCPCommandGetCapability, mcpevents.TopicMCPCommandListCapabilities, mcpevents.TopicMCPCommandReadRegistrySnapshot, mcpevents.TopicMCPCommandReadSelectionSnapshot:
		drain = true
	}
	err := m.SubscribeFuncChecked(topic, func(ev magicevent.Event, result magicevent.Result) {
		if ev == nil || result == nil {
			return
		}
		leave, err := m.enterOwner(drain)
		if err != nil {
			result.Set(nil, cd.WrapError(cd.ServiceUnavailable, err, "mcp registry owner is shutting down"))
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
