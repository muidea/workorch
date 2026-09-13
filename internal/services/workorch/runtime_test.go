package workorch

import (
	"context"
	"strings"
	"testing"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
)

func TestFrameworkRuntimeServerStoppedNotificationIsChannelFreeAndIdempotent(t *testing.T) {
	runtime := &frameworkRuntime{serverStopped: make(chan struct{})}
	first := workorchdevents.ServiceStoppedData{ServiceName: "workorch", State: "failed", ErrorCode: "server_failed", ErrorMessage: "first failure", StoppedAt: time.Now().UTC()}
	second := workorchdevents.ServiceStoppedData{ServiceName: "workorch", State: "stopped", ErrorCode: "server_stopped", StoppedAt: time.Now().UTC()}
	event := magicevent.NewEventWithContext(workorchdevents.TopicWorkorchDServiceStopped, "test", workorchdevents.NotificationDestination, magicevent.NewHeader(), context.Background(), first)
	// Post observers receive nil Result; this must not panic or try to Set one.
	runtime.handleServerStopped(event, nil)
	runtime.handleServerStopped(magicevent.NewEventWithContext(workorchdevents.TopicWorkorchDServiceStopped, "test", workorchdevents.NotificationDestination, magicevent.NewHeader(), context.Background(), second), nil)
	if err := runtime.waitServer(context.Background()); err == nil || !strings.Contains(err.Error(), "first failure") {
		t.Fatalf("waitServer error = %v, want first stopped failure", err)
	}
}

func TestFrameworkRuntimeServerStopObserverHasConcreteIdentityAndIndependentLifecycle(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })

	runtime := &frameworkRuntime{serverStopped: make(chan struct{})}
	if err := runtime.subscribeServerStopped(hub); err != nil {
		t.Fatal(err)
	}
	if runtime.serverStopObserver == nil {
		t.Fatal("server stop observer was not installed")
	}
	if got := runtime.serverStopObserver.ID(); got != serverStopObserverID || hasEventHubWildcardSegment(got) {
		t.Fatalf("server stop observer ID = %q, want concrete %q", got, serverStopObserverID)
	}

	peerNotifications := make(chan struct{}, 2)
	peer := magicevent.NewSimpleObserverWithMatchID("application/workorch/server-stop-peer", workorchdevents.NotificationDestination, hub)
	peer.Subscribe(workorchdevents.TopicWorkorchDServiceStopped, func(_ magicevent.Event, _ magicevent.Result) {
		peerNotifications <- struct{}{}
	})
	t.Cleanup(func() { peer.Unsubscribe(workorchdevents.TopicWorkorchDServiceStopped) })

	first := workorchdevents.ServiceStoppedData{ServiceName: "workorch", State: "stopped", ErrorCode: "server_stopped", StoppedAt: time.Now().UTC()}
	hub.Post(magicevent.NewEventWithContext(workorchdevents.TopicWorkorchDServiceStopped, "test", workorchdevents.NotificationDestination, magicevent.NewHeader(), ctx, first))
	select {
	case <-runtime.serverStopped:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for framework runtime server-stop notification")
	}
	waitForServerStopPeerNotification(t, peerNotifications)

	runtime.serverStopObserver.Unsubscribe(workorchdevents.TopicWorkorchDServiceStopped)
	runtime.serverStopObserver = nil
	second := workorchdevents.ServiceStoppedData{ServiceName: "workorch", State: "stopped", ErrorCode: "server_stopped", StoppedAt: time.Now().UTC()}
	hub.Post(magicevent.NewEventWithContext(workorchdevents.TopicWorkorchDServiceStopped, "test", workorchdevents.NotificationDestination, magicevent.NewHeader(), ctx, second))
	waitForServerStopPeerNotification(t, peerNotifications)
}

func TestFrameworkRuntimeRejectsServerStopSubscriptionOnClosedHub(t *testing.T) {
	hub := magicevent.NewHub(16)
	hub.Terminate(context.Background())
	runtime := &frameworkRuntime{serverStopped: make(chan struct{})}
	if err := runtime.subscribeServerStopped(hub); err == nil {
		t.Fatal("closed Hub was reported as a successful server-stop subscription")
	}
	if runtime.serverStopObserver != nil {
		t.Fatal("failed subscription published an observer")
	}
}

func waitForServerStopPeerNotification(t *testing.T, notifications <-chan struct{}) {
	t.Helper()
	select {
	case <-notifications:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for server-stop peer notification")
	}
}

func hasEventHubWildcardSegment(value string) bool {
	for _, segment := range strings.Split(value, "/") {
		switch segment {
		case "#", "+", ":id":
			return true
		}
	}
	return false
}
