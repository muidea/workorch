// Package biz supplies the owner-neutral EventHub and background-task base for
// framework Modules and Blocks. It is deliberately not a framework plugin and
// must not acquire business contracts or owner-specific dependencies.
package biz

import (
	"context"
	"sync"
	"time"

	"github.com/muidea/magicCommon/event"
	"github.com/muidea/magicCommon/task"
)

type Base struct {
	id                string
	eventHub          event.Hub
	simpleObserver    event.SimpleObserver
	backgroundRoutine task.BackgroundRoutine
	matchedObservers  map[string]event.SimpleObserver
	matchedMu         sync.Mutex
}

type routineTask struct {
	funcPtr func()
}

func (s *routineTask) Run() {
	s.funcPtr()
}

func New(id string, eventHub event.Hub, backgroundRoutine task.BackgroundRoutine) Base {
	return Base{
		id:                id,
		eventHub:          eventHub,
		simpleObserver:    event.NewSimpleObserver(id, eventHub),
		backgroundRoutine: backgroundRoutine,
		matchedObservers:  make(map[string]event.SimpleObserver),
	}
}

func (s *Base) ID() string {
	return s.id
}

// EventHub exposes the owner Hub only to a Biz derived from Base. Module roots
// and protocol adapters must not retain the returned Hub.
func (s *Base) EventHub() event.Hub {
	return s.eventHub
}

func (s *Base) BackgroundRoutine() task.BackgroundRoutine {
	return s.backgroundRoutine
}

func (s *Base) Subscribe(eventID string, observer event.Observer) {
	s.eventHub.Subscribe(eventID, observer)
}

func (s *Base) Unsubscribe(eventID string, observer event.Observer) {
	s.eventHub.Unsubscribe(eventID, observer)
}

func (s *Base) SubscribeFunc(eventID string, observerFunc event.ObserverFunc) {
	s.simpleObserver.Subscribe(eventID, observerFunc)
}

func (s *Base) UnsubscribeFunc(eventID string) {
	s.simpleObserver.Unsubscribe(eventID)
}

// SubscribeMatchedFunc is the owner-neutral variant for a Biz that needs a
// destination matcher different from its owner ID. The observer remains owned
// by Base so concrete Biz types never retain a parallel Hub or observer.
func (s *Base) SubscribeMatchedFunc(observerID, matchID, eventID string, observerFunc event.ObserverFunc) {
	s.matchedMu.Lock()
	observer := s.matchedObservers[observerID]
	if observer == nil {
		observer = event.NewSimpleObserverWithMatchID(observerID, matchID, s.eventHub)
		s.matchedObservers[observerID] = observer
	}
	s.matchedMu.Unlock()
	observer.Subscribe(eventID, observerFunc)
}

func (s *Base) UnsubscribeMatchedFunc(observerID, eventID string) {
	s.matchedMu.Lock()
	observer := s.matchedObservers[observerID]
	s.matchedMu.Unlock()
	if observer != nil {
		observer.Unsubscribe(eventID)
	}
}

func (s *Base) PostEvent(value event.Event) {
	s.eventHub.Post(value)
}

func (s *Base) SendEvent(value event.Event) event.Result {
	return s.eventHub.Send(value)
}

func (s *Base) SyncTask(funcPtr func()) {
	s.backgroundRoutine.SyncTask(&routineTask{funcPtr: funcPtr})
}

func (s *Base) AsyncTask(funcPtr func()) {
	s.backgroundRoutine.AsyncTask(&routineTask{funcPtr: funcPtr})
}

func (s *Base) Timer(ctx context.Context, intervalValue time.Duration, offsetValue time.Duration, funcPtr func()) {
	s.backgroundRoutine.Timer(ctx, &routineTask{funcPtr: funcPtr}, intervalValue, offsetValue)
}
