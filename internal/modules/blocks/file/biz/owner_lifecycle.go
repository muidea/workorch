package file

import (
	"context"
	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	fileevents "github.com/muidea/workorch/internal/modules/blocks/file/pkg/events"
)

type ownerSubscription struct {
	topic   string
	matched bool
}

// BeginShutdown 关闭新执行，不释放已接纳工作的 adapter 和 receipt 通路。
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
	drain := false
	switch topic {
	case fileevents.TopicAcquireAuthoringResources, fileevents.TopicVerifyAuthoringResource, fileevents.TopicAcquireWorkspaceManifest, fileevents.TopicQueryRuntimeCWIReconciliation,
		fileevents.TopicExecuteFileListOperation, fileevents.TopicExecuteFileReadOperation, fileevents.TopicExecuteFileEditOperation, fileevents.TopicExecuteFileWriteOperation, fileevents.TopicExecutePatchApplyOperation, fileevents.TopicExecutePatchPreviewOperation:
		drain = true
	}
	// Execute drain 只供 sealed 校验及 exact receipt 重放；未命中须另取 fresh。
	wrapped := func(ev magicevent.Event, result magicevent.Result) {
		if ev == nil || result == nil {
			return
		}
		leave, err := m.enterOwner(drain)
		if err != nil {
			result.Set(nil, cd.WrapError(cd.ServiceUnavailable, err, "file owner is shutting down"))
			return
		}
		defer leave()
		handler(ev, result)
	}
	var err error
	if matched {
		err = m.SubscribeMatchedFuncChecked(ModuleID+"/prerequisite", fileevents.PrerequisiteCommandDestination, topic, wrapped)
	} else {
		err = m.SubscribeFuncChecked(topic, wrapped)
	}
	if err != nil {
		return err
	}
	m.subscriptions = append(m.subscriptions, ownerSubscription{topic: topic, matched: matched})
	return nil
}
