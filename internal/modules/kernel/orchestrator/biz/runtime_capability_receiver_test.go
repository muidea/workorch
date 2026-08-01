package orchestrator

import (
	"context"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	decisionmodule "github.com/muidea/workorch/internal/modules/blocks/decision"
	llmmodule "github.com/muidea/workorch/internal/modules/blocks/llm"
	mcpmodule "github.com/muidea/workorch/internal/modules/blocks/mcp"
	sshmodule "github.com/muidea/workorch/internal/modules/blocks/ssh"
	webmodule "github.com/muidea/workorch/internal/modules/blocks/web"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// File, Git and Shell have independent file-backed restart matrices. The
// remaining receivers share their exact receipt constructor, but still need a
// real EventHub producer-consumer proof so an owner can neither lose its
// subscription nor accidentally publish another owner's inventory.
func TestRuntimeCapabilityReceiverModulesPublishDeclaredInventory(t *testing.T) {
	ctx := context.Background()
	owners := []struct {
		name     string
		moduleID string
		kind     string
		class    runtimecontract.RuntimeCapabilityClass
		start    func(t *testing.T, hub magicevent.Hub) func()
	}{
		{
			name: "mcp", moduleID: mcpmodule.ModuleID, kind: mcpmodule.StepKindResourceRead, class: runtimecontract.RuntimeCapabilityClassValidation,
			start: func(t *testing.T, hub magicevent.Hub) func() {
				t.Helper()
				owner := mcpmodule.New()
				if err := owner.Configure(mcpmodule.Dependencies{EventHub: hub}); err != nil {
					t.Fatal(err)
				}
				if err := owner.Start(); err != nil {
					t.Fatal(err)
				}
				return func() { _ = owner.Stop(ctx) }
			},
		},
		{
			name: "ssh", moduleID: sshmodule.ModuleID, kind: sshmodule.StepKindCommand, class: runtimecontract.RuntimeCapabilityClassStateChange,
			start: func(t *testing.T, hub magicevent.Hub) func() {
				t.Helper()
				owner := sshmodule.New()
				if err := owner.Configure(sshmodule.Dependencies{EventHub: hub}); err != nil {
					t.Fatal(err)
				}
				if err := owner.Start(); err != nil {
					t.Fatal(err)
				}
				return func() { _ = owner.Stop(ctx) }
			},
		},
		{
			name: "web", moduleID: webmodule.ModuleID, kind: webmodule.StepKindFetch, class: runtimecontract.RuntimeCapabilityClassValidation,
			start: func(t *testing.T, hub magicevent.Hub) func() {
				t.Helper()
				owner := webmodule.New()
				if err := owner.Configure(webmodule.Dependencies{EventHub: hub}); err != nil {
					t.Fatal(err)
				}
				if err := owner.Start(); err != nil {
					t.Fatal(err)
				}
				return func() { _ = owner.Stop(ctx) }
			},
		},
		{
			name: "llm", moduleID: llmmodule.ModuleID, kind: llmmodule.StepKindWorkspaceExecution, class: runtimecontract.RuntimeCapabilityClassValidation,
			start: func(t *testing.T, hub magicevent.Hub) func() {
				t.Helper()
				owner := llmmodule.New()
				if err := owner.Configure(llmmodule.Dependencies{EventHub: hub}); err != nil {
					t.Fatal(err)
				}
				if err := owner.Start(); err != nil {
					t.Fatal(err)
				}
				return func() { _ = owner.Stop(ctx) }
			},
		},
		{
			name: "decision", moduleID: decisionmodule.ModuleID, kind: runtimecontract.RuntimeBlockKindDecisionRequest, class: runtimecontract.RuntimeCapabilityClassGovernance,
			start: func(t *testing.T, hub magicevent.Hub) func() {
				t.Helper()
				owner := decisionmodule.New()
				if err := owner.Configure(decisionmodule.Dependencies{EventHub: hub, Tickets: memory.NewDecisionTicketRepository(), Resolutions: memory.NewDecisionResolutionRepository()}); err != nil {
					t.Fatal(err)
				}
				if err := owner.Start(); err != nil {
					t.Fatal(err)
				}
				return func() { _ = owner.Stop(ctx) }
			},
		},
	}
	for _, owner := range owners {
		owner := owner
		t.Run(owner.name, func(t *testing.T) {
			hub := magicevent.NewHub(16)
			defer hub.Terminate(ctx)
			stop := owner.start(t, hub)
			defer stop()
			result := hub.Send(magicevent.NewEvent(orchestratorevents.TopicOrchestratorAcquireRuntimeCapabilities, "runtime-capability-test", owner.moduleID, magicevent.NewHeader(), orchestratorevents.AcquireRuntimeCapabilitiesCommand{
				WorkspaceID: "ws-receiver", RunID: "run-receiver", AuthoritySnapshotID: "authority-receiver", PrerequisiteID: "capability-receiver",
				ModuleID: owner.moduleID, CapabilityClass: owner.class, RequiredKinds: []string{owner.kind},
			}))
			if result == nil || result.Error() != nil {
				t.Fatalf("owner %s did not publish a capability receipt: %v", owner.moduleID, result)
			}
			data, err := result.Get()
			if err != nil {
				t.Fatalf("owner %s receipt get: %v", owner.moduleID, err)
			}
			receipt, ok := data.(orchestratorevents.AcquireRuntimeCapabilitiesResult)
			if !ok || receipt.PrerequisiteID != "capability-receiver" || receipt.Observation == nil || receipt.Observation.OwnerModuleID != owner.moduleID {
				t.Fatalf("owner %s receipt = %#v", owner.moduleID, data)
			}
		})
	}
}
