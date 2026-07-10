package runtimecore

import (
	"context"
	"errors"
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/bootstrap"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func TestSessionRequiredEventHubReturnsBorrowedExportHub(t *testing.T) {
	hub := events.NewMemoryHub()
	runtime := bootstrap.NewWithBackend("memory", "")
	t.Cleanup(func() { _ = runtime.Shutdown(context.Background()) })
	exports := runtime.Exports()
	exports.Infrastructure.EventHub = hub
	session, err := NewSessionFromExports(exports, nil)
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

func TestValidateRuntimeExportsNamesMissingCoverageWorkItems(t *testing.T) {
	runtime := bootstrap.NewWithBackend("memory", "")
	t.Cleanup(func() { _ = runtime.Shutdown(context.Background()) })
	exports := runtime.Exports()
	exports.Repositories.CoverageWorkItems = nil

	err := ValidateRuntimeExports(exports)
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("ValidateRuntimeExports error = %v, want ErrInvalidArgument", err)
	}
	if got := err.Error(); got != "invalid argument: missing runtime export Repositories.CoverageWorkItems" {
		t.Fatalf("ValidateRuntimeExports error = %q", got)
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
