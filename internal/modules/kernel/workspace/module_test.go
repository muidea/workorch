package workspace

import (
	"context"
	"errors"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
)

func TestModuleIdentityConfigureStartStopContract(t *testing.T) {
	module := New()
	if module.ID() != ModuleID {
		t.Fatalf("ID = %q, want %q", module.ID(), ModuleID)
	}
	if module.servicePtr != nil {
		t.Fatal("new module service pointer is non-nil before Configure")
	}

	hub := magicevent.NewHub(16)
	if err := module.Configure(Dependencies{
		Workspaces: memory.NewWorkspaceRepository(),
		Goals:      memory.NewGoalRepository(),
		DoDs:       memory.NewDoDRepository(),
		Templates:  memory.NewWorkspaceTemplateRepository(),
		EventHub:   hub,
	}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if module.servicePtr == nil {
		t.Fatal("service pointer is nil after Configure")
	}
	if module.eventHub == nil {
		t.Fatal("event hub is nil after Configure")
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	firstSubscriptionCount := len(module.topics)
	if firstSubscriptionCount == 0 {
		t.Fatal("Start did not subscribe workspace commands")
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
	if module.servicePtr != nil ||
		module.bizPtr != nil ||
		module.eventHub != nil ||
		module.workspaces != nil ||
		module.goals != nil ||
		module.dods != nil ||
		module.templates != nil ||
		module.observer != nil ||
		len(module.topics) != 0 {
		t.Fatalf("module was not cleared after Stop: %+v", module)
	}
	if err := module.Stop(context.Background()); err != nil {
		t.Fatalf("second Stop: %v", err)
	}
}

func TestModuleConfigureRequiresAllRepositories(t *testing.T) {
	valid := Dependencies{
		Workspaces: memory.NewWorkspaceRepository(),
		Goals:      memory.NewGoalRepository(),
		DoDs:       memory.NewDoDRepository(),
		Templates:  memory.NewWorkspaceTemplateRepository(),
		EventHub:   magicevent.NewHub(16),
	}
	tests := []struct {
		name string
		deps Dependencies
	}{
		{name: "missing workspaces", deps: Dependencies{Goals: valid.Goals, DoDs: valid.DoDs, Templates: valid.Templates, EventHub: valid.EventHub}},
		{name: "missing goals", deps: Dependencies{Workspaces: valid.Workspaces, DoDs: valid.DoDs, Templates: valid.Templates, EventHub: valid.EventHub}},
		{name: "missing dods", deps: Dependencies{Workspaces: valid.Workspaces, Goals: valid.Goals, Templates: valid.Templates, EventHub: valid.EventHub}},
		{name: "missing templates", deps: Dependencies{Workspaces: valid.Workspaces, Goals: valid.Goals, DoDs: valid.DoDs, EventHub: valid.EventHub}},
		{name: "missing event hub", deps: Dependencies{Workspaces: valid.Workspaces, Goals: valid.Goals, DoDs: valid.DoDs, Templates: valid.Templates}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := New().Configure(tc.deps); !errors.Is(err, common.ErrInvalidArgument) {
				t.Fatalf("Configure err = %v, want invalid argument", err)
			}
		})
	}
}
