package decision

import (
	"context"
	"errors"
	"testing"

	cd "github.com/muidea/magicCommon/def"
	"github.com/muidea/magicCommon/event"
)

type rejectedLifecycleHub struct {
	event.Hub
	rejection *cd.Error
}

func (h *rejectedLifecycleHub) Unsubscribe(topic string, observer event.Observer) *cd.Error {
	if h.rejection != nil {
		return h.rejection
	}
	return h.Hub.Unsubscribe(topic, observer)
}
func TestModuleTeardownFailurePreservesOwnerAndFrameworkCause(t *testing.T) {
	deps := decisionModuleContractDependencies()
	hub := &rejectedLifecycleHub{Hub: deps.EventHub}
	deps.EventHub = hub
	defer hub.Terminate(context.Background())
	module := New()
	if err := module.Configure(deps); err != nil {
		t.Fatal(err)
	}
	if err := module.Start(); err != nil {
		t.Fatal(err)
	}
	owner := module.bizPtr
	module.BeginShutdown(context.Background())
	if err := module.Quiesce(context.Background()); err != nil {
		t.Fatal(err)
	}
	rejection := cd.NewError(cd.ResourceExhausted, "unsubscribe rejected")
	hub.rejection = rejection
	func() {
		defer func() {
			err, ok := recover().(error)
			if !ok || !errors.Is(err, rejection) {
				t.Fatalf("guard cause = %v", err)
			}
		}()
		module.Teardown(context.Background())
	}()
	if module.bizPtr != owner {
		t.Fatal("failed Teardown discarded owner")
	}
	if err := module.Configure(deps); err == nil {
		t.Fatal("Configure replaced unreleased owner")
	}
	hub.rejection = nil
	module.Teardown(context.Background())
	if module.bizPtr != nil {
		t.Fatal("successful retry retained owner")
	}
	module.Teardown(context.Background())
}
