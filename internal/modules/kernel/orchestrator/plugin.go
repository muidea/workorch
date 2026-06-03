package orchestrator

import (
	"context"

	cd "github.com/muidea/magicCommon/def"
	"github.com/muidea/magicCommon/event"
	"github.com/muidea/magicCommon/framework/plugin/initiator"
	pluginmodule "github.com/muidea/magicCommon/framework/plugin/module"
	"github.com/muidea/magicCommon/task"
	runtimecommon "github.com/muidea/workorch/internal/initiators/workorchruntime/pkg/common"
	filecommon "github.com/muidea/workorch/internal/modules/blocks/file/pkg/common"
	gitcommon "github.com/muidea/workorch/internal/modules/blocks/git/pkg/common"
	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	mcpcommon "github.com/muidea/workorch/internal/modules/blocks/mcp/pkg/common"
	sshcommon "github.com/muidea/workorch/internal/modules/blocks/ssh/pkg/common"
	webcommon "github.com/muidea/workorch/internal/modules/blocks/web/pkg/common"
	orchestratorbiz "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz"
)

func init() {
	pluginmodule.Register(New())
}

func (m *Module) Weight() int { return 30 }

func (m *Module) Setup(_ context.Context, _ event.Hub, _ task.BackgroundRoutine) *cd.Error {
	runtime, err := workorchRuntime()
	if err != nil {
		return err
	}
	repos := runtime.Repositories()
	runtimeRegistry := orchestratorbiz.NewRuntimeAdapterRegistry()
	eventAdapter := NewEventRuntimeAdapter(runtime.EventHub())
	for _, stepKind := range []string{filecommon.StepKindList, filecommon.StepKindRead, filecommon.StepKindEdit} {
		if err := runtimeRegistry.Register(stepKind, eventAdapter); err != nil {
			return cd.NewError(cd.Unexpected, err.Error())
		}
	}
	if err := runtimeRegistry.Register("shell/command", eventAdapter); err != nil {
		return cd.NewError(cd.Unexpected, err.Error())
	}
	for _, stepKind := range []string{gitcommon.StepKindRead, gitcommon.StepKindWrite} {
		if err := runtimeRegistry.Register(stepKind, eventAdapter); err != nil {
			return cd.NewError(cd.Unexpected, err.Error())
		}
	}
	if err := runtimeRegistry.Register(sshcommon.StepKindCommand, eventAdapter); err != nil {
		return cd.NewError(cd.Unexpected, err.Error())
	}
	for _, stepKind := range []string{webcommon.StepKindSearch, webcommon.StepKindFetch} {
		if err := runtimeRegistry.Register(stepKind, eventAdapter); err != nil {
			return cd.NewError(cd.Unexpected, err.Error())
		}
	}
	for _, stepKind := range []string{mcpcommon.StepKindToolCall, mcpcommon.StepKindResourceRead, mcpcommon.StepKindPromptGet} {
		if err := runtimeRegistry.Register(stepKind, eventAdapter); err != nil {
			return cd.NewError(cd.Unexpected, err.Error())
		}
	}
	if err := runtimeRegistry.Register(llmcommon.RuntimeBlockKindPatchApply, orchestratorbiz.NewPatchRuntimeAdapter()); err != nil {
		return cd.NewError(cd.Unexpected, err.Error())
	}
	if err := runtimeRegistry.Register(llmcommon.RuntimeBlockKindPatchPreview, orchestratorbiz.NewPatchRuntimeAdapter()); err != nil {
		return cd.NewError(cd.Unexpected, err.Error())
	}
	if err := runtimeRegistry.Register(llmcommon.RuntimeBlockKindPatchRevert, orchestratorbiz.NewPatchRuntimeAdapter()); err != nil {
		return cd.NewError(cd.Unexpected, err.Error())
	}
	if err := runtimeRegistry.Register(llmcommon.StepKindWorkspaceExecution, eventAdapter); err != nil {
		return cd.NewError(cd.Unexpected, err.Error())
	}
	if setupErr := m.Configure(Dependencies{
		Runtime:               runtimeRegistry,
		Steps:                 repos.Steps,
		ToolCalls:             repos.ToolCalls,
		Patches:               repos.Patches,
		ValidationResults:     repos.ValidationResults,
		RuntimeCheckpoints:    repos.RuntimeCheckpoints,
		ArtifactContinuations: repos.ArtifactContinuations,
		RuntimePolicy:         runtime.RuntimePolicy(),
		EventHub:              runtime.EventHub(),
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

func workorchRuntime() (runtimecommon.Helper, *cd.Error) {
	var helper runtimecommon.Helper
	return initiator.GetEntity(runtimecommon.InitiatorID, helper)
}
