package governance

import (
	"context"
	"errors"
	"testing"

	governanceevents "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/events"
	governancemodel "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
)

func TestModuleIdentityConfigureStartStopContract(t *testing.T) {
	module := New()
	if module.ID() != ModuleID {
		t.Fatalf("ID = %q, want %q", module.ID(), ModuleID)
	}
	if err := module.Start(); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("Start before Configure err = %v, want invalid argument", err)
	}

	if err := module.Configure(governanceModuleContractDependencies()); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if module.bizPtr == nil || module.traces == nil || module.eventHub == nil {
		t.Fatalf("module not fully configured: %+v", module)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	firstSubscriptionCount := len(module.unsub)
	if firstSubscriptionCount == 0 {
		t.Fatal("Start did not subscribe governance commands")
	}
	if err := module.Start(); err != nil {
		t.Fatalf("second Start: %v", err)
	}
	if len(module.unsub) != firstSubscriptionCount {
		t.Fatalf("second Start subscription count = %d, want %d", len(module.unsub), firstSubscriptionCount)
	}
	if err := module.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if module.bizPtr != nil ||
		module.traces != nil ||
		module.eventHub != nil ||
		len(module.unsub) != 0 {
		t.Fatalf("module was not cleared after Stop: %+v", module)
	}
	if err := module.Stop(context.Background()); err != nil {
		t.Fatalf("second Stop: %v", err)
	}
}

func TestModuleConfigureRequiresCoreDependencies(t *testing.T) {
	base := governanceModuleContractDependencies()
	tests := []struct {
		name string
		deps Dependencies
	}{
		{name: "missing traces", deps: func() Dependencies { d := base; d.AutoGovernanceTraces = nil; return d }()},
		{name: "missing event hub", deps: func() Dependencies { d := base; d.EventHub = nil; return d }()},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := New().Configure(tc.deps); !errors.Is(err, common.ErrInvalidArgument) {
				t.Fatalf("Configure err = %v, want invalid argument", err)
			}
		})
	}
}

func TestModuleListTracesCommandUsesRepositoryQueryContract(t *testing.T) {
	ctx := context.Background()
	hub := events.NewMemoryHub()
	traces := memory.NewAutoGovernanceTraceRepository()
	if err := traces.Create(ctx, &governancemodel.AutoGovernanceTrace{
		ID:          "trace-owner-a",
		WorkspaceID: "ws-governance-owner-a",
		RunID:       "run-governance-owner-a",
		StepID:      "step-governance-owner-a",
		ItemType:    "decision",
		ItemID:      "ticket-a",
		Status:      "resolved",
	}); err != nil {
		t.Fatalf("create trace a: %v", err)
	}
	if err := traces.Create(ctx, &governancemodel.AutoGovernanceTrace{
		ID:          "trace-owner-b",
		WorkspaceID: "ws-governance-owner-b",
		RunID:       "run-governance-owner-b",
		StepID:      "step-governance-owner-b",
		ItemType:    "approval",
		ItemID:      "approval-b",
		Status:      "blocked",
	}); err != nil {
		t.Fatalf("create trace b: %v", err)
	}
	module := New()
	deps := governanceModuleContractDependencies()
	deps.AutoGovernanceTraces = traces
	deps.EventHub = hub
	if err := module.Configure(deps); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		if err := module.Stop(ctx); err != nil {
			t.Errorf("Stop: %v", err)
		}
	})

	err := hub.Publish(ctx, events.Envelope{
		Topic:       governanceevents.TopicGovernanceAutoCommandListTraces,
		WorkspaceID: "ws-governance-owner-a",
		RunID:       "run-governance-owner-a",
		StepID:      "step-governance-owner-a",
		Payload: map[string]any{
			events.PayloadKeyQuery: repository.AutoGovernanceTraceQuery{
				WorkspaceID: "ws-governance-owner-a",
				RunID:       "run-governance-owner-a",
				ItemType:    "decision",
				Status:      "resolved",
			},
		},
	})
	response, ok := events.ResponseEnvelope(err)
	if !ok {
		t.Fatalf("list traces err = %v, want response", err)
	}
	items, ok := response.Payload[events.PayloadKeyItems].([]*governancemodel.AutoGovernanceTrace)
	if !ok || len(items) != 1 || items[0].ID != "trace-owner-a" {
		t.Fatalf("items = %+v, want scoped governance trace", response.Payload[events.PayloadKeyItems])
	}
}

func governanceModuleContractDependencies() Dependencies {
	return Dependencies{
		AutoGovernanceTraces: memory.NewAutoGovernanceTraceRepository(),
		EventHub:             events.NewMemoryHub(),
		LoadConfig: func() (workorchconfig.Config, error) {
			return workorchconfig.Config{}, nil
		},
	}
}
