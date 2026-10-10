// Package biz supplies the owner-neutral EventHub and background-task base for
// framework Modules and Blocks. It is deliberately not a framework plugin and
// must not acquire business contracts or owner-specific dependencies.
package biz

import (
	"context"
	"sync"
	"time"

	cd "github.com/muidea/magicCommon/def"
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

// 无返回值的必需订阅失败通过生命周期 guard 向上传播。
// 可恢复错误使用对应 Checked 方法；不能把部分注册标记为完整启动。
func (s *Base) Subscribe(eventID string, observer event.Observer) {
	if err := s.SubscribeChecked(eventID, observer); err != nil {
		panic(err)
	}
}

func (s *Base) SubscribeChecked(eventID string, observer event.Observer) error {
	if s == nil || s.eventHub == nil {
		return cd.NewError(cd.ServiceUnavailable, "event hub is required")
	}
	return cd.ToStdError(s.eventHub.Subscribe(eventID, observer))
}

func (s *Base) Unsubscribe(eventID string, observer event.Observer) {
	if err := s.UnsubscribeChecked(eventID, observer); err != nil {
		panic(err)
	}
}

func (s *Base) UnsubscribeChecked(eventID string, observer event.Observer) error {
	if s == nil || s.eventHub == nil {
		return cd.NewError(cd.ServiceUnavailable, "event hub is required")
	}
	return cd.ToStdError(s.eventHub.Unsubscribe(eventID, observer))
}

func (s *Base) SubscribeFunc(eventID string, observerFunc event.ObserverFunc) {
	if err := s.SubscribeFuncChecked(eventID, observerFunc); err != nil {
		panic(err)
	}
}

func (s *Base) SubscribeFuncChecked(eventID string, observerFunc event.ObserverFunc) error {
	if s == nil || s.simpleObserver == nil {
		return cd.NewError(cd.ServiceUnavailable, "simple observer is required")
	}
	return cd.ToStdError(s.simpleObserver.Subscribe(eventID, observerFunc))
}

func (s *Base) UnsubscribeFunc(eventID string) {
	if err := s.UnsubscribeFuncChecked(eventID); err != nil {
		panic(err)
	}
}

func (s *Base) UnsubscribeFuncChecked(eventID string) error {
	if s == nil || s.simpleObserver == nil {
		return cd.NewError(cd.ServiceUnavailable, "simple observer is required")
	}
	return cd.ToStdError(s.simpleObserver.Unsubscribe(eventID))
}

// SubscribeMatchedFunc is the owner-neutral variant for a Biz that needs a
// destination matcher different from its owner ID. The observer remains owned
// by Base so concrete Biz types never retain a parallel Hub or observer.
func (s *Base) SubscribeMatchedFunc(observerID, matchID, eventID string, observerFunc event.ObserverFunc) {
	if err := s.SubscribeMatchedFuncChecked(observerID, matchID, eventID, observerFunc); err != nil {
		panic(err)
	}
}

func (s *Base) SubscribeMatchedFuncChecked(observerID, matchID, eventID string, observerFunc event.ObserverFunc) error {
	if s == nil || s.eventHub == nil {
		return cd.NewError(cd.ServiceUnavailable, "event hub is required")
	}
	s.matchedMu.Lock()
	if s.matchedObservers == nil {
		s.matchedObservers = make(map[string]event.SimpleObserver)
	}
	observer := s.matchedObservers[observerID]
	if observer == nil {
		observer = event.NewSimpleObserverWithMatchID(observerID, matchID, s.eventHub)
		s.matchedObservers[observerID] = observer
	}
	s.matchedMu.Unlock()
	return cd.ToStdError(observer.Subscribe(eventID, observerFunc))
}

func (s *Base) UnsubscribeMatchedFunc(observerID, eventID string) {
	if err := s.UnsubscribeMatchedFuncChecked(observerID, eventID); err != nil {
		panic(err)
	}
}

func (s *Base) UnsubscribeMatchedFuncChecked(observerID, eventID string) error {
	if s == nil {
		return cd.NewError(cd.ServiceUnavailable, "base is required")
	}
	s.matchedMu.Lock()
	observer := s.matchedObservers[observerID]
	s.matchedMu.Unlock()
	if observer == nil {
		return nil
	}
	// 取消失败保留 observer 和 callback，供 owner 保留依赖并重试。
	return cd.ToStdError(observer.Unsubscribe(eventID))
}

func (s *Base) PostEvent(value event.Event) {
	s.eventHub.Post(value)
}

func (s *Base) SendEvent(value event.Event) event.Result {
	return s.eventHub.Send(value)
}

// SyncTask 返回提交或执行错误，包括 callback panic。
func (s *Base) SyncTask(funcPtr func()) error {
	if err := s.checkTask(funcPtr); err != nil {
		return err
	}
	return s.backgroundRoutine.SyncTask(&routineTask{funcPtr: funcPtr})
}

// SyncTaskWithTimeOut 只约束入队后的完成等待。超时不取消已接纳任务，
// 也不证明任务退出；BackgroundRoutine 仍追踪其真实停机。-1 表示无限等待。
func (s *Base) SyncTaskWithTimeOut(funcPtr func(), timeout time.Duration) error {
	if err := s.checkTask(funcPtr); err != nil {
		return err
	}
	return s.backgroundRoutine.SyncTaskWithTimeOut(&routineTask{funcPtr: funcPtr}, timeout)
}

// AsyncTask 只返回提交结果；后续 callback 错误由 BackgroundRoutine 观测。
func (s *Base) AsyncTask(funcPtr func()) error {
	if err := s.checkTask(funcPtr); err != nil {
		return err
	}
	return s.backgroundRoutine.AsyncTask(&routineTask{funcPtr: funcPtr})
}

// AsyncTaskContext 使用实际任务 API，只约束入队。
// 已接纳 callback 的取消与清理由执行方负责。
func (s *Base) AsyncTaskContext(ctx context.Context, funcPtr func()) error {
	if err := s.checkTask(funcPtr); err != nil {
		return err
	}
	return s.backgroundRoutine.AsyncTaskContext(ctx, &routineTask{funcPtr: funcPtr})
}

// Timer 成功仅表示注册成功。取消 ctx 停止调度，但 BackgroundRoutine
// 仍追踪已接纳 callback，不能据此宣告真实排空。
func (s *Base) Timer(ctx context.Context, intervalValue time.Duration, offsetValue time.Duration, funcPtr func()) error {
	if err := s.checkTask(funcPtr); err != nil {
		return err
	}
	return s.backgroundRoutine.Timer(ctx, &routineTask{funcPtr: funcPtr}, intervalValue, offsetValue)
}

func (s *Base) checkTask(funcPtr func()) error {
	if funcPtr == nil {
		return cd.NewError(cd.IllegalParam, "task callback is required")
	}
	if s == nil || s.backgroundRoutine == nil {
		return cd.NewError(cd.ServiceUnavailable, "background routine is required")
	}
	return nil
}
