package orchestrator

import (
	"context"
	"testing"

	decisionbiz "github.com/muidea/workorch/internal/modules/blocks/decision/biz"
	llmbiz "github.com/muidea/workorch/internal/modules/blocks/llm/biz"
	mcpblockbiz "github.com/muidea/workorch/internal/modules/blocks/mcp/biz"
	sshbiz "github.com/muidea/workorch/internal/modules/blocks/ssh/biz"
	webbiz "github.com/muidea/workorch/internal/modules/blocks/web/biz"

	magicevent "github.com/muidea/magicCommon/event"
	decisionmodule "github.com/muidea/workorch/internal/modules/blocks/decision"
	llmmodule "github.com/muidea/workorch/internal/modules/blocks/llm"
	mcpmodule "github.com/muidea/workorch/internal/modules/blocks/mcp"
	sshmodule "github.com/muidea/workorch/internal/modules/blocks/ssh"
	webmodule "github.com/muidea/workorch/internal/modules/blocks/web"
	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
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
				if err := owner.Configure(mcpblockbiz.Dependencies{EventHub: hub}); err != nil {
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
				if err := owner.Configure(sshbiz.Dependencies{EventHub: hub}); err != nil {
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
				if err := owner.Configure(webbiz.Dependencies{EventHub: hub}); err != nil {
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
				if err := owner.Configure(llmbiz.Dependencies{EventHub: hub, LoadLLMConfig: testLLMConfigurationSnapshot}); err != nil {
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
				if err := owner.Configure(decisionbiz.Dependencies{OwnerRepositories: testDecisionOwnerRepositories(memory.NewDecisionTicketRepository(), memory.NewDecisionResolutionRepository(), memory.NewDecisionWorkspaceDeletionRepository()), EventHub: hub}); err != nil {
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
			facts := runtime.PreflightRuntimeFacts(runtimecontract.RuntimeFacts{
				WorkspaceID: "ws-receiver", RunID: "run-receiver", DefinitionRevision: "definition-receiver", EvidenceRevision: "evidence-receiver",
				Focus: runtimecontract.CoverageFocus{ID: "focus-receiver"},
			}, []runtimecontract.RuntimeRequirement{{Kind: runtimecontract.PrerequisiteKindCapability, OwnerModuleID: owner.moduleID, CapabilityClass: owner.class, RequiredCapabilities: []string{owner.kind}}})
			prerequisite := facts.Prerequisites.Pending[0]
			committed, err := AcquireAndCommitRuntimeCapabilitiesPrerequisite(ctx, hub, facts, runtimecontract.RuntimeCapabilityAcquisitionRequest{
				WorkspaceID: facts.WorkspaceID, RunID: facts.RunID, AuthoritySnapshotID: "authority-receiver", PrerequisiteID: prerequisite.ID,
				OwnerModuleID: owner.moduleID, CapabilityClass: owner.class, RequiredKinds: []string{owner.kind},
			})
			if err != nil {
				t.Fatalf("owner %s did not publish a capability receipt: %v", owner.moduleID, err)
			}
			observation, ok := runtimecontract.RuntimePrerequisiteObservationForID(committed.Resources.PrerequisiteObservations, prerequisite.ID)
			if !ok || observation.OwnerModuleID != owner.moduleID {
				t.Fatalf("owner %s observation = %#v", owner.moduleID, observation)
			}
		})
	}
}
