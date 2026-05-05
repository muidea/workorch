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
	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	mcpcommon "github.com/muidea/workorch/internal/modules/blocks/mcp/pkg/common"
	sshcommon "github.com/muidea/workorch/internal/modules/blocks/ssh/pkg/common"
	orchestratorbiz "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz"
)

func init() {
	pluginmodule.Register(newPlugin())
}

type plugin struct {
	module *Module
}

func newPlugin() *plugin {
	return &plugin{}
}

func (p *plugin) ID() string { return ModuleID }

func (p *plugin) Weight() int { return 30 }

func (p *plugin) Setup(_ event.Hub, _ task.BackgroundRoutine) *cd.Error {
	runtime, err := workorchRuntime()
	if err != nil {
		return err
	}
	repos := runtime.Repositories()
	runtimeRegistry := orchestratorbiz.NewRuntimeAdapterRegistry()
	if runtime.FileRuntimeAdapter() != nil {
		for _, stepKind := range []string{filecommon.StepKindList, filecommon.StepKindRead, filecommon.StepKindEdit} {
			if err := runtimeRegistry.Register(stepKind, runtime.FileRuntimeAdapter()); err != nil {
				return cd.NewError(cd.Unexpected, err.Error())
			}
		}
	}
	if runtime.ShellRuntimeAdapter() != nil {
		if err := runtimeRegistry.Register("shell/command", runtime.ShellRuntimeAdapter()); err != nil {
			return cd.NewError(cd.Unexpected, err.Error())
		}
	}
	if runtime.SSHRuntimeAdapter() != nil {
		if err := runtimeRegistry.Register(sshcommon.StepKindCommand, runtime.SSHRuntimeAdapter()); err != nil {
			return cd.NewError(cd.Unexpected, err.Error())
		}
	}
	if runtime.MCPRuntimeAdapter() != nil {
		for _, stepKind := range []string{mcpcommon.StepKindToolCall, mcpcommon.StepKindResourceRead, mcpcommon.StepKindPromptGet} {
			if err := runtimeRegistry.Register(stepKind, runtime.MCPRuntimeAdapter()); err != nil {
				return cd.NewError(cd.Unexpected, err.Error())
			}
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
	if runtime.LLMRuntimeAdapter() != nil {
		if err := runtimeRegistry.Register(llmcommon.StepKindWorkspaceExecution, runtime.LLMRuntimeAdapter()); err != nil {
			return cd.NewError(cd.Unexpected, err.Error())
		}
	}
	p.module = New()
	if setupErr := p.module.Setup(Dependencies{
		Runtime:            runtimeRegistry,
		Runs:               repos.Runs,
		Steps:              repos.Steps,
		Workspaces:         repos.Workspaces,
		Tickets:            repos.Tickets,
		Resolutions:        repos.Resolutions,
		Approvals:          repos.Approvals,
		Results:            repos.Results,
		Artifacts:          repos.Artifacts,
		ToolCalls:          repos.ToolCalls,
		Patches:            repos.Patches,
		ValidationResults:  repos.ValidationResults,
		RuntimeCheckpoints: repos.RuntimeCheckpoints,
		MemoryItems:        repos.MemoryItems,
		MemoryRecallTraces: repos.MemoryRecallTraces,
		EventHub:           runtime.EventHub(),
	}); setupErr != nil {
		return cd.NewError(cd.Unexpected, setupErr.Error())
	}
	runtime.SetOrchestratorService(p.module.Service())
	return nil
}

func (p *plugin) Run() *cd.Error {
	if p == nil || p.module == nil {
		return cd.NewError(cd.IllegalParam, "orchestrator module is not configured")
	}
	if err := p.module.Run(); err != nil {
		return cd.NewError(cd.Unexpected, err.Error())
	}
	return nil
}

func (p *plugin) Teardown() {
	if p == nil || p.module == nil {
		return
	}
	_ = p.module.Teardown(context.Background())
	p.module = nil
}

func (p *plugin) Service() Service {
	if p == nil || p.module == nil {
		return nil
	}
	return p.module.Service()
}

func workorchRuntime() (runtimecommon.Helper, *cd.Error) {
	var helper runtimecommon.Helper
	return initiator.GetEntity(runtimecommon.InitiatorID, helper)
}
