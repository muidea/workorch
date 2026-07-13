package cli

import (
	"context"
	"testing"

	workorchport "github.com/muidea/workorch/internal/pkg/workorchport"
)

type runtimeV1RetirementBackendStub struct {
	workorchport.Backend
	options workorchport.RuntimeV1RetirementOptions
}

func (b *runtimeV1RetirementBackendStub) RetireRuntimeV1(_ context.Context, options workorchport.RuntimeV1RetirementOptions) (workorchport.RuntimeV1RetirementResult, error) {
	b.options = options
	return workorchport.RuntimeV1RetirementResult{DryRun: options.DryRun}, nil
}

func TestExecuteRuntimeV1RetirementDefaultsToDryRun(t *testing.T) {
	backend := &runtimeV1RetirementBackendStub{}
	result, err := executeRuntimeV1Retirement(context.Background(), backend, false)
	if err != nil || !result.DryRun || !backend.options.DryRun {
		t.Fatalf("result=%+v options=%+v err=%v", result, backend.options, err)
	}
}

func TestExecuteRuntimeV1RetirementApplyRequiresExplicitFlag(t *testing.T) {
	backend := &runtimeV1RetirementBackendStub{}
	result, err := executeRuntimeV1Retirement(context.Background(), backend, true)
	if err != nil || result.DryRun || backend.options.DryRun {
		t.Fatalf("result=%+v options=%+v err=%v", result, backend.options, err)
	}
}

func TestExecuteRuntimeV1RetirementRejectsBackendWithoutMaintenanceCapability(t *testing.T) {
	backend := &runtimeV1RetirementBackendStub{}
	if _, err := executeRuntimeV1Retirement(context.Background(), backend.Backend, false); err == nil {
		t.Fatal("executeRuntimeV1Retirement returned nil error")
	}
}
