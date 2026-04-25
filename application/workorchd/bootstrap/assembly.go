package bootstrap

import (
	"os"

	approvalmodule "github.com/muidea/workorch/internal/modules/blocks/approval"
	legacymodule "github.com/muidea/workorch/internal/modules/blocks/legacy"
	decisionmodule "github.com/muidea/workorch/internal/modules/kernel/decision"
	orchestratormodule "github.com/muidea/workorch/internal/modules/kernel/orchestrator"
	runmodule "github.com/muidea/workorch/internal/modules/kernel/run"
	workspacemodule "github.com/muidea/workorch/internal/modules/kernel/workspace"
	"github.com/muidea/workorch/internal/pkg/events"
)

func NewFromEnv() *Runtime {
	return NewWithBackend(os.Getenv(envBackend), os.Getenv(envDataRoot))
}

func NewWithBackend(backend string, dataRoot string) *Runtime {
	return NewWithBackendAndEventHub(backend, dataRoot, nil)
}

func NewWithBackendAndEventHub(backend string, dataRoot string, eventHub events.Hub) *Runtime {
	repos := buildRepositories(backend, dataRoot)

	if eventHub == nil {
		eventHub = events.NewMagicHub()
	}
	legacyModule := legacymodule.New()
	mustSetup("legacy", legacyModule.Setup())

	workspaceModule := workspacemodule.New()
	mustSetup("workspace", workspaceModule.Setup(workspacemodule.Dependencies{
		Workspaces: repos.workspace,
		EventHub:   eventHub,
	}))

	decisionModule := decisionmodule.New()
	mustSetup("decision", decisionModule.Setup(decisionmodule.Dependencies{
		Tickets:     repos.ticket,
		Resolutions: repos.resolution,
		EventHub:    eventHub,
	}))

	approvalModule := approvalmodule.New()
	mustSetup("approval", approvalModule.Setup(approvalmodule.Dependencies{
		Approvals: repos.approval,
		EventHub:  eventHub,
	}))

	orchestratorModule := orchestratormodule.New()
	mustSetup("orchestrator", orchestratorModule.Setup(orchestratormodule.Dependencies{
		Runtime:     legacyModule.RuntimeAdapter(),
		Runs:        repos.run,
		Steps:       repos.step,
		Workspaces:  repos.workspace,
		Tickets:     repos.ticket,
		Resolutions: repos.resolution,
		Approvals:   repos.approval,
		EventHub:    eventHub,
	}))

	runModule := runmodule.New()
	mustSetup("run", runModule.Setup(runmodule.Dependencies{
		Runs:         repos.run,
		Workspaces:   repos.workspace,
		Orchestrator: orchestratorModule.Service(),
		EventHub:     eventHub,
	}))

	modules := []lifecycle{
		legacyModule,
		workspaceModule,
		decisionModule,
		approvalModule,
		orchestratorModule,
		runModule,
	}
	moduleLifecycle, err := newModulePluginLifecycle(modules)
	mustSetup("module-lifecycle", err)
	mustSetup("module-lifecycle", moduleLifecycle.Run())

	return &Runtime{
		exports: Exports{
			Repositories: RepositoryExports{
				Workspaces:  repos.workspace,
				Runs:        repos.run,
				Steps:       repos.step,
				Tickets:     repos.ticket,
				Resolutions: repos.resolution,
				Approvals:   repos.approval,
			},
			Services: ServiceExports{
				WorkspaceService: workspaceModule.Service(),
				DecisionService:  decisionModule.Service(),
				ApprovalService:  approvalModule.Service(),
				RunService:       runModule.Service(),
			},
			Infrastructure: InfrastructureExports{
				EventHub: eventHub,
			},
		},
		modules:         modules,
		moduleLifecycle: moduleLifecycle,
	}
}

func mustSetup(name string, err error) {
	if err != nil {
		panic("setup module " + name + ": " + err.Error())
	}
}
