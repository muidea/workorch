package runtimecore

import (
	"errors"
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/bootstrap"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func TestSessionRequiredEventHubReturnsBorrowedExportHub(t *testing.T) {
	hub := events.NewMemoryHub()
	session, err := NewSessionFromExports(bootstrap.Exports{
		Infrastructure: bootstrap.InfrastructureExports{
			EventHub: hub,
		},
	}, nil)
	if err != nil {
		t.Fatalf("NewSessionFromExports: %v", err)
	}

	got, err := session.RequiredEventHub()
	if err != nil {
		t.Fatalf("RequiredEventHub: %v", err)
	}
	if got != hub {
		t.Fatalf("RequiredEventHub returned different hub")
	}
}

func TestSessionRequiredEventHubFailsFastWithoutHub(t *testing.T) {
	session := &Session{}
	if _, err := session.RequiredEventHub(); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("empty session RequiredEventHub error = %v, want ErrInvalidArgument", err)
	}
	if _, err := (*Session)(nil).RequiredEventHub(); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("nil session RequiredEventHub error = %v, want ErrInvalidArgument", err)
	}
}
