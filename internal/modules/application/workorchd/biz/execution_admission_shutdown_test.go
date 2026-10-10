package biz

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
)

type shutdownSubscriptionFailureHub struct {
	magicevent.Hub
	fail  bool
	cause *cd.Error
}

func (h *shutdownSubscriptionFailureHub) Unsubscribe(topic string, observer magicevent.Observer) *cd.Error {
	if h.fail {
		return h.cause
	}
	return h.Hub.Unsubscribe(topic, observer)
}

func TestApplicationStopRetainsSubscriptionLedgerOnFailure(t *testing.T) {
	hub := &shutdownSubscriptionFailureHub{Hub: magicevent.NewHub(8), cause: cd.NewError(cd.Unexpected, "unsubscribe failed")}
	defer hub.Terminate(context.Background())
	b := &Biz{Base: basebiz.New(ModuleID, hub, nil), admissions: newExecutionAdmissionLifecycle()}
	if err := b.subscribeCommand("test.shutdown.command", func(_ magicevent.Event, result magicevent.Result) {
		result.Set("retained", nil)
	}); err != nil {
		t.Fatal(err)
	}
	hub.fail = true
	if err := b.Stop(context.Background()); !errors.Is(err, hub.cause) {
		t.Fatalf("取消失败未向上返回原错误: %v", err)
	}
	if len(b.subscriptions) != 1 {
		t.Fatal("取消失败丢失了订阅清理入口")
	}
	result := hub.Send(magicevent.NewEvent("test.shutdown.command", "test", ModuleID, nil, nil))
	if result == nil || result.Error() != nil {
		t.Fatalf("取消失败后原订阅不可达: %v", result)
	}
	hub.fail = false
	if err := b.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(b.subscriptions) != 0 {
		t.Fatal("成功重试未移除已释放订阅")
	}
	if err := b.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestExecutionAdmissionWaitContextRetainsAcceptedHandlersForRetry(t *testing.T) {
	lifecycle := newExecutionAdmissionLifecycle()
	if !lifecycle.begin() {
		t.Fatal("未接纳初始 handler")
	}
	lifecycle.stop()
	if lifecycle.begin() {
		t.Fatal("关门后接纳了新 handler")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err := lifecycle.waitContext(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("真实 handler 未退出时应返回 deadline: %v", err)
	}
	select {
	case <-lifecycle.drained:
		t.Fatal("等待超时被错误标记为真实排空")
	default:
	}
	lifecycle.end()
	if err := lifecycle.waitContext(context.Background()); err != nil {
		t.Fatalf("真实退出后的重试失败: %v", err)
	}
	if err := lifecycle.waitContext(ctx); err != nil {
		t.Fatalf("已完成排空不应被旧等待预算覆盖: %v", err)
	}
}

func TestAuthoringWaitContextRetriesWithoutExtendingSettlementWindow(t *testing.T) {
	lifecycle := newAuthoringTaskLifecycle()
	_, release, err := lifecycle.accept(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	lifecycle.beginStop(ctx)
	if err := lifecycle.waitContext(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("未真实退出时应返回等待预算: %v", err)
	}
	select {
	case <-lifecycle.drainCtx.Done():
	case <-time.After(time.Second):
		t.Fatal("settlement 窗口未按首次预算关闭")
	}
	retryCtx, retryCancel := context.WithTimeout(context.Background(), time.Second)
	defer retryCancel()
	waiting := make(chan error, 1)
	go func() { waiting <- lifecycle.waitContext(retryCtx) }()
	select {
	case err := <-waiting:
		t.Fatalf("settlement root 取消被当成真实 handler 退出: %v", err)
	case <-time.After(10 * time.Millisecond):
	}
	release()
	if err := <-waiting; err != nil {
		t.Fatalf("真实退出后排空重试失败: %v", err)
	}
	if _, _, err := lifecycle.accept(context.Background()); err == nil {
		t.Fatal("重试等待重新开放了 authoring 准入")
	}
}

func TestExecutionAdmissionCloseAndConcurrentEntryDrain(t *testing.T) {
	for iteration := 0; iteration < 50; iteration++ {
		lifecycle := newExecutionAdmissionLifecycle()
		start := make(chan struct{})
		var workers sync.WaitGroup
		for range 32 {
			workers.Go(func() {
				<-start
				if lifecycle.begin() {
					defer lifecycle.end()
				}
			})
		}
		close(start)
		lifecycle.stop()
		workers.Wait()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		err := lifecycle.waitContext(ctx)
		cancel()
		if err != nil {
			t.Fatalf("第 %d 次并发入站未真实排空: %v", iteration, err)
		}
		if lifecycle.begin() {
			t.Fatal("排空后重新开放了入站")
		}
	}
}
