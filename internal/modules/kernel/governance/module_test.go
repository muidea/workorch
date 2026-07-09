package governance

import (
	"context"
	"errors"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	governanceevents "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/events"
	governancemodel "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
)

func sendGovernanceTestCommand(ctx context.Context, rawHub magicevent.Hub, topic string, command any) (governanceevents.GovernanceCommandResult, error) {
	ev := magicevent.NewEventWithContext(topic, "governance/module_test", governanceevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	result := rawHub.Send(ev)
	if result == nil {
		return governanceevents.GovernanceCommandResult{}, common.ErrInvalidArgument
	}
	if err := result.Error(); err != nil {
		if err.Cause != nil {
			return governanceevents.GovernanceCommandResult{}, err.Cause
		}
		return governanceevents.GovernanceCommandResult{}, err
	}
	data, err := result.Get()
	if err != nil {
		return governanceevents.GovernanceCommandResult{}, err
	}
	response, ok := data.(governanceevents.GovernanceCommandResult)
	if !ok {
		return governanceevents.GovernanceCommandResult{}, common.ErrInvalidArgument
	}
	return response, nil
}

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
	firstSubscriptionCount := len(module.topics)
	if firstSubscriptionCount == 0 {
		t.Fatal("Start did not subscribe governance commands")
	}
	if err := module.Start(); err != nil {
		t.Fatalf("second Start: %v", err)
	}
	if len(module.topics) != firstSubscriptionCount {
		t.Fatalf("second Start subscription count = %d, want %d", len(module.topics), firstSubscriptionCount)
	}
	if err := module.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if module.bizPtr != nil ||
		module.traces != nil ||
		module.eventHub != nil ||
		module.observer != nil ||
		len(module.topics) != 0 {
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
	rawHub := magicevent.NewHub(16)
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
	deps.EventHub = rawHub
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

	response, err := sendGovernanceTestCommand(ctx, rawHub, governanceevents.TopicGovernanceAutoCommandListTraces, governanceevents.ListAutoGovernanceTracesCommand{
		Query: repository.AutoGovernanceTraceQuery{
			WorkspaceID: "ws-governance-owner-a",
			RunID:       "run-governance-owner-a",
			ItemType:    "decision",
			Status:      "resolved",
		},
	})
	if err != nil {
		t.Fatalf("list traces err = %v, want response", err)
	}
	items := response.Traces
	if len(items) != 1 || items[0].ID != "trace-owner-a" {
		t.Fatalf("items = %+v, want scoped governance trace", response.Traces)
	}
}

func governanceModuleContractDependencies() Dependencies {
	return Dependencies{
		AutoGovernanceTraces: memory.NewAutoGovernanceTraceRepository(),
		EventHub:             magicevent.NewHub(16),
		LoadConfig: func() (workorchconfig.Config, error) {
			return workorchconfig.Config{}, nil
		},
	}
}
