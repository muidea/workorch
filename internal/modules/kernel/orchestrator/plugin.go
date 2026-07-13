package orchestrator

import (
	"context"

	cd "github.com/muidea/magicCommon/def"
	"github.com/muidea/magicCommon/event"
	pluginmodule "github.com/muidea/magicCommon/framework/plugin/module"
	"github.com/muidea/magicCommon/task"
	"github.com/muidea/workorch/internal/pkg/workorchdeps"
)

func init() {
	pluginmodule.Register(New())
}

func (m *Module) Weight() int { return 30 }

func (m *Module) Setup(_ context.Context, rawEventHub event.Hub, backgroundRoutine task.BackgroundRoutine) *cd.Error {
	if rawEventHub == nil {
		return cd.NewError(cd.IllegalParam, "event hub is required")
	}
	repos, err := workorchdeps.ResolveOrchestratorRepositories(ModuleID)
	if err != nil {
		return err
	}
	if setupErr := m.Configure(Dependencies{
		Steps:                 repos.Steps,
		ToolCalls:             repos.ToolCalls,
		Patches:               repos.Patches,
		ValidationResults:     repos.ValidationResults,
		RuntimeCheckpoints:    repos.RuntimeCheckpoints,
		EvidenceRecords:       repos.EvidenceRecords,
		CoverageWorkItems:     repos.CoverageWorkItems,
		ArtifactContinuations: repos.ArtifactContinuations,
		CommandHub:            rawEventHub,
		BackgroundRoutine:     backgroundRoutine,
	}); setupErr != nil {
		return cd.NewError(cd.Unexpected, setupErr.Error())
	}
	return nil
}

func (m *Module) Run(_ context.Context) *cd.Error {
	if m == nil {
		return cd.NewError(cd.IllegalParam, "orchestrator module is not configured")
	}
	if err := m.Start(); err != nil {
		return cd.NewError(cd.Unexpected, err.Error())
	}
	return nil
}

func (m *Module) Teardown(ctx context.Context) {
	if m != nil {
		_ = m.Stop(ctx)
	}
}
