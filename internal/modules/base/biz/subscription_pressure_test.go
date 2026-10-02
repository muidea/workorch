package biz

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/muidea/magicCommon/event"
)

type blockedSubscriptionObserver struct {
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (o *blockedSubscriptionObserver) ID() string {
	o.once.Do(func() { close(o.entered) })
	<-o.release
	return "blocked"
}

func (*blockedSubscriptionObserver) Notify(event.Event, event.Result) {}

// 控制 worker 阻塞时，有界 admission 拒绝仍必须显式返回，不能被生命周期吞掉。
func TestSubscriptionControlAdmissionRejectsPressureAndCompletesAcceptedRequests(t *testing.T) {
	hub := event.NewHub(1)
	blocked := &blockedSubscriptionObserver{entered: make(chan struct{}), release: make(chan struct{})}
	var release sync.Once
	defer hub.Terminate(context.Background())
	defer release.Do(func() { close(blocked.release) })
	first := make(chan bool, 1)
	go func() { first <- hub.Subscribe("/test/blocked", blocked) == nil }()
	select {
	case <-blocked.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("控制 worker 未进入订阅")
	}
	outcomes := make(chan bool, 2)
	start := make(chan struct{})
	for _, id := range []string{"one", "two"} {
		go func() {
			<-start
			outcomes <- hub.Subscribe("/test/pressure", event.NewSimpleObserver(id, hub)) == nil
		}()
	}
	close(start)
	select {
	case accepted := <-outcomes:
		if accepted {
			t.Fatal("阻塞期间订阅报告已完成")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("控制面拒绝未返回")
	}
	release.Do(func() { close(blocked.release) })
	select {
	case accepted := <-first:
		if !accepted {
			t.Fatal("已进入控制 worker 的订阅失败")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("放行后未完成订阅")
	}
	select {
	case accepted := <-outcomes:
		if !accepted {
			t.Fatal("已排队订阅未在放行后完成")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("排队订阅未结束")
	}
}
