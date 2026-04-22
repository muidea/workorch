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
	repos := buildRepositories(backend, dataRoot)

	eventHub := events.NewMemoryHub()
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
		Runtime:    legacyModule.RuntimeAdapter(),
		Runs:       repos.run,
		Steps:      repos.step,
		Workspaces: repos.workspace,
		Tickets:    repos.ticket,
		Approvals:  repos.approval,
		EventHub:   eventHub,
	}))

	runModule := runmodule.New()
	mustSetup("run", runModule.Setup(runmodule.Dependencies{
		Runs:         repos.run,
		Workspaces:   repos.workspace,
		Orchestrator: orchestratorModule.Service(),
	}))

	modules := []lifecycle{
		legacyModule,
		workspaceModule,
		decisionModule,
		approvalModule,
		orchestratorModule,
		runModule,
	}
	for _, item := range modules {
		mustSetup(item.ID(), item.Run())
	}

	return &Runtime{
		exports: Exports{
			Repositories: RepositoryExports{
				Workspaces: repos.workspace,
				Runs:       repos.run,
				Steps:      repos.step,
				Tickets:    repos.ticket,
				Approvals:  repos.approval,
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
		modules: modules,
	}
}

func mustSetup(name string, err error) {
	if err != nil {
		panic("setup module " + name + ": " + err.Error())
	}
}
