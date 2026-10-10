package appservice

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
)

func TestCLIDispatcherShutdownRetainsAcceptedHandlerUntilActualExit(t *testing.T) {
	hub := magicevent.NewHub(8)
	defer hub.Terminate(context.Background())
	d := &CLICommandDispatcher{Base: basebiz.New("test.cli", hub, nil)}
	entered, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	defer unblock()
	d.subscribeTypedCLI("test.cli.active", func(_ magicevent.Event, result magicevent.Result) {
		close(entered)
		<-release
		result.Set("completed", nil)
	})
	d.subscribeTypedCLI("test.cli.fresh", func(_ magicevent.Event, result magicevent.Result) {
		t.Error("关闭准入后执行了新 handler")
		result.Set(nil, nil)
	})
	done := make(chan magicevent.Result, 1)
	callCtx, cancelCall := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelCall()
	go func() {
		done <- hub.Send(magicevent.NewEventWithContext("test.cli.active", "test", "test.cli", nil, callCtx, nil))
	}()
	select {
	case <-entered:
	case <-callCtx.Done():
		t.Fatal("真实 handler 未进入")
	}
	d.BeginShutdown()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err := d.CloseContext(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("未真实退出时应返回等待超时: %v", err)
	}
	if len(d.topics) != 2 {
		t.Fatal("未排空就清除了订阅账本")
	}
	fresh := magicevent.NewEventWithContext("test.cli.fresh", "test", "test.cli", nil, callCtx, nil)
	fresh.BindLaneKey("test.cli.fresh")
	result := hub.Send(fresh)
	if result == nil || result.Error() == nil || !errors.Is(result.Error(), basebiz.ErrLifecycleClosing) {
		t.Fatalf("新入站未明确拒绝: %v", result)
	}
	unblock()
	if result := <-done; result == nil || result.Error() != nil {
		t.Fatalf("已接纳调用未保留正常结果: %v", result)
	}
	if err := d.CloseContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(d.topics) != 0 {
		t.Fatal("重试未清理订阅")
	}
	if err := d.CloseContext(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestCLIDispatcherPartialStartAndUnsubscribeFailureRetainLedger(t *testing.T) {
	hub := &cliSubscriptionFailureHub{Hub: magicevent.NewHub(8), cause: cd.NewError(cd.Unexpected, "subscription failure")}
	defer hub.Terminate(context.Background())
	d := NewCLICommandDispatcher(&runtimecore.Session{}, hub, nil)
	hub.failSubscribe = 3
	if err := d.Run(); !errors.Is(err, hub.cause) {
		t.Fatalf("部分启动未返回原错误: %v", err)
	}
	if len(d.topics) != 2 || d.typedReady {
		t.Fatal("部分启动未保留准确成功订阅")
	}
	calls := hub.subscribeCalls
	if err := d.Run(); err == nil || hub.subscribeCalls != calls {
		t.Fatal("未清理部分启动就再次注册")
	}
	hub.failUnsubscribe = true
	if err := d.CloseContext(context.Background()); !errors.Is(err, hub.cause) {
		t.Fatalf("取消失败未返回原错误: %v", err)
	}
	if len(d.topics) != 2 {
		t.Fatal("取消失败丢失清理账本")
	}
	hub.failUnsubscribe = false
	if err := d.CloseContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	hub.failSubscribe = 0
	if err := d.Run(); err != nil {
		t.Fatalf("真实清理后不能开启下一代: %v", err)
	}
	if err := d.CloseContext(context.Background()); err != nil {
		t.Fatal(err)
	}
}

type cliSubscriptionFailureHub struct {
	magicevent.Hub
	cause                         *cd.Error
	subscribeCalls, failSubscribe int
	failUnsubscribe               bool
}

func (h *cliSubscriptionFailureHub) Subscribe(topic string, observer magicevent.Observer) *cd.Error {
	h.subscribeCalls++
	if h.subscribeCalls == h.failSubscribe {
		return h.cause
	}
	return h.Hub.Subscribe(topic, observer)
}

func (h *cliSubscriptionFailureHub) Unsubscribe(topic string, observer magicevent.Observer) *cd.Error {
	if h.failUnsubscribe {
		return h.cause
	}
	return h.Hub.Unsubscribe(topic, observer)
}

func TestCLIDispatcherUnknownSubscriptionPanicIsNotSwallowed(t *testing.T) {
	d := &CLICommandDispatcher{Base: basebiz.New("test.cli", &cliPanicSubscribeHub{}, nil)}
	defer func() {
		if value := recover(); value != "unknown subscription panic" {
			t.Fatalf("未知 panic 未原样传播: %v", value)
		}
	}()
	_ = d.runTypedCLI()
}

type cliPanicSubscribeHub struct{ magicevent.Hub }

func (*cliPanicSubscribeHub) Subscribe(string, magicevent.Observer) *cd.Error {
	panic("unknown subscription panic")
}
