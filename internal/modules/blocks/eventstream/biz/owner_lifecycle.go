package eventstream

import (
	"context"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	eventstreamevents "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
)

type ownerSubscription struct {
	topic      string
	observerID string
}

func (s *Biz) BeginShutdown(context.Context) {
	if s != nil {
		s.lifecycle.BeginShutdown()
	}
}
func (s *Biz) Quiesce(ctx context.Context) error {
	if s == nil {
		return nil
	}
	return s.lifecycle.Quiesce(ctx)
}
func (s *Biz) enterOwner(drain bool) (func(), error) {
	s.shutdownMu.Lock()
	defer s.shutdownMu.Unlock()
	if s.releasing || s.released {
		return nil, basebiz.ErrLifecycleClosing
	}
	if drain {
		return s.lifecycle.EnterDrain()
	}
	return s.lifecycle.Enter()
}
func (s *Biz) subscribeOwnerCommand(topic string, handler magicevent.ObserverFunc) error {
	drain := topic == eventstreamevents.TopicEventStreamCommandList || topic == eventstreamevents.TopicEventStreamCommandHighWatermarks || topic == eventstreamevents.TopicEventStreamCommandRecordApprovalResolution
	err := s.SubscribeFuncChecked(topic, func(ev magicevent.Event, result magicevent.Result) {
		if ev == nil || result == nil {
			return
		}
		leave, err := s.enterOwner(drain)
		if err != nil {
			result.Set(nil, cd.WrapError(cd.ServiceUnavailable, err, "eventstream owner is shutting down"))
			return
		}
		defer leave()
		handler(ev, result)
	})
	if err != nil {
		return err
	}
	s.subscriptions = append(s.subscriptions, ownerSubscription{topic: topic})
	return nil
}
func (s *Biz) subscribeOwnerNotification(observerID, matchID, topic string, handler magicevent.ObserverFunc) error {
	err := s.SubscribeMatchedFuncChecked(observerID, matchID, topic, func(ev magicevent.Event, result magicevent.Result) {
		if ev == nil {
			return
		}
		leave, err := s.enterOwner(true)
		if err != nil {
			if result != nil {
				result.Set(nil, cd.WrapError(cd.ServiceUnavailable, err, "eventstream owner is released"))
			}
			return
		}
		defer leave()
		handler(ev, result)
	})
	if err != nil {
		return err
	}
	s.subscriptions = append(s.subscriptions, ownerSubscription{topic: topic, observerID: observerID})
	return nil
}
func (s *Biz) stopChecked(ctx context.Context) error {
	if s == nil {
		return nil
	}
	s.BeginShutdown(ctx)
	if err := s.Quiesce(ctx); err != nil {
		return err
	}
	s.shutdownMu.Lock()
	if s.releasing {
		s.shutdownMu.Unlock()
		return common.ErrConflict
	}
	s.releasing = true
	s.shutdownMu.Unlock()
	defer func() { s.shutdownMu.Lock(); s.releasing = false; s.shutdownMu.Unlock() }()
	if err := s.Quiesce(ctx); err != nil {
		return err
	}
	for len(s.subscriptions) > 0 {
		if err := ctx.Err(); err != nil {
			return err
		}
		last := len(s.subscriptions) - 1
		entry := s.subscriptions[last]
		var err error
		if entry.observerID != "" {
			err = s.UnsubscribeMatchedFuncChecked(entry.observerID, entry.topic)
		} else {
			err = s.UnsubscribeFuncChecked(entry.topic)
		}
		if err != nil {
			return err
		}
		s.subscriptions = s.subscriptions[:last]
	}
	s.repo = nil
	s.workspaceDeletions = nil
	s.workspacePurger = nil
	s.revisions = nil
	s.started = false
	s.shutdownMu.Lock()
	s.released = true
	s.shutdownMu.Unlock()
	return nil
}
