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
