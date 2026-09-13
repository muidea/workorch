package events

import (
	"context"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
)

// Exercise the application-selected EventHub using virtual time and public APIs.
// Required contract: accepted changes wait for acknowledgement; rejected changes
// return an explicit error and preserve both local and Hub subscription state.
func TestEventHubSubscriptionBackpressure(t *testing.T) {
	for _, operation := range []string{"subscribe", "unsubscribe"} {
		t.Run(operation, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				hub := magicevent.NewHubWithOptions(16, magicevent.WithHubActionChanSize(1))
				defer hub.Terminate(context.Background())
				const topic, owner = "/diagnostic/target", "diagnostic-owner"
				target := magicevent.NewSimpleObserver(owner, hub)
				handler := func(_ magicevent.Event, result magicevent.Result) { result.Set("observed", nil) }
				if operation == "unsubscribe" {
					if err := target.Subscribe(topic, handler); err != nil {
						t.Fatal(err)
					}
				}
				release := holdSubscriptionController(t, hub)
				defer release()
				done := make(chan *cd.Error, 1)
				go func() {
					if operation == "subscribe" {
						done <- target.Subscribe(topic, handler)
					} else {
						done <- target.Unsubscribe(topic)
					}
				}()
				synctest.Wait()
				time.Sleep(20 * time.Millisecond)
				synctest.Wait()
				select {
				case err := <-done:
					if err == nil || err.Code != cd.ResourceExhausted {
						t.Errorf("%s rejection must return ResourceExhausted: %v", operation, err)
					}
				default:
					t.Error("unadmitted subscription change did not return its bounded rejection")
				}
				release()
				assertRoute := func(wantPresent bool) {
					t.Helper()
					result := hub.Send(magicevent.NewEvent(topic, "diagnostic-caller", owner, nil, nil))
					if result == nil {
						t.Fatal("missing synchronous result")
					}
					value, err := result.Get()
					if wantPresent {
						if err != nil || value != "observed" {
							t.Fatalf("required observer is not callable: value=%v error=%v", value, err)
						}
					} else if err == nil || !strings.Contains(err.Error(), "missing observer") {
						t.Fatalf("absent observer still routes: value=%v error=%v", value, err)
					}
				}
				assertRoute(operation == "unsubscribe")
				// A new explicit caller attempt after the rejected command must be
				// possible. This is a framework test, not a business retry policy.
				if operation == "subscribe" {
					if err := target.Subscribe(topic, handler); err != nil {
						t.Fatal(err)
					}
					assertRoute(true)
					if err := target.Subscribe(topic, handler); err == nil || err.Code != cd.Duplicated {
						t.Fatalf("duplicate subscription error = %v", err)
					}
					assertRoute(true)
				} else {
					if err := target.Unsubscribe(topic); err != nil {
						t.Fatal(err)
					}
					assertRoute(false)
					if err := target.Unsubscribe(topic); err != nil {
						t.Fatal(err)
					}
					assertRoute(false)
				}
			})
		})
	}
}

type subscriptionControllerBlocker struct {
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (b *subscriptionControllerBlocker) ID() string {
	b.once.Do(func() { close(b.entered) })
	<-b.release
	return "diagnostic-blocker"
}

func (*subscriptionControllerBlocker) Notify(_ magicevent.Event, result magicevent.Result) {
	if result != nil {
		result.Set("blocker", nil)
	}
}

// Public Observer identity supplies a controlled pause in the controller. This
// is a fault-injection fixture, not a claim that WorkOrch IDs block in production.
func holdSubscriptionController(t *testing.T, hub magicevent.Hub) func() {
	t.Helper()
	b := &subscriptionControllerBlocker{entered: make(chan struct{}), release: make(chan struct{})}
	controllerDone := make(chan *cd.Error, 1)
	go func() {
		controllerDone <- hub.Subscribe("/diagnostic/control", b)
	}()
	<-b.entered
	queuedDone := make(chan *cd.Error, 1)
	go func() {
		queuedDone <- hub.Subscribe("/diagnostic/queued", magicevent.NewSimpleObserver("queued-peer", hub))
	}()
	synctest.Wait()
	var once sync.Once
	return func() {
		once.Do(func() {
			close(b.release)
			if err := <-controllerDone; err != nil {
				t.Error(err)
			}
			if err := <-queuedDone; err != nil {
				t.Error(err)
			}
		})
	}
}
