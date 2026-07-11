package runtimecore

import (
	"context"
	"errors"
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/bootstrap"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestValidateRuntimeExportsNamesMissingCommandHub(t *testing.T) {
	runtime := bootstrap.NewWithBackend("memory", "")
	t.Cleanup(func() { _ = runtime.Shutdown(context.Background()) })
	exports := runtime.Exports()
	exports.Infrastructure.CommandHub = nil

	err := ValidateRuntimeExports(exports)
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("ValidateRuntimeExports error = %v, want ErrInvalidArgument", err)
	}
	if got := err.Error(); got != "invalid argument: missing runtime export Infrastructure.CommandHub" {
		t.Fatalf("ValidateRuntimeExports error = %q", got)
	}
}
