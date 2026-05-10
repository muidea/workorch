package governance

import (
	"context"

	cd "github.com/muidea/magicCommon/def"
	"github.com/muidea/magicCommon/event"
	"github.com/muidea/magicCommon/framework/plugin/initiator"
	pluginmodule "github.com/muidea/magicCommon/framework/plugin/module"
	"github.com/muidea/magicCommon/task"
	runtimecommon "github.com/muidea/workorch/internal/initiators/workorchruntime/pkg/common"
	approvalmodule "github.com/muidea/workorch/internal/modules/blocks/approval/service"
	decisioncommon "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/common"
	decisionmodule "github.com/muidea/workorch/internal/modules/kernel/decision/service"
	runmodule "github.com/muidea/workorch/internal/modules/kernel/run/service"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
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

func (p *plugin) Weight() int { return 45 }

func (p *plugin) Setup(_ event.Hub, _ task.BackgroundRoutine) *cd.Error {
	runtime, err := workorchRuntime()
	if err != nil {
		return err
	}
	repos := runtime.Repositories()
	decisionSvc := runtime.DecisionService()
	approvalSvc := runtime.ApprovalService()
	runSvc := runtime.RunService()
	if decisionSvc == nil || approvalSvc == nil || runSvc == nil {
		return cd.NewError(cd.IllegalParam, "governance dependencies are not configured")
	}
	p.module = New()
	setupErr := p.module.Setup(Dependencies{
		Tickets:              repos.Tickets,
		Resolutions:          repos.Resolutions,
		Approvals:            repos.Approvals,
		Runs:                 repos.Runs,
		Goals:                repos.Goals,
		DoDs:                 repos.DoDs,
		MemoryItems:          repos.MemoryItems,
		AutoGovernanceTraces: repos.AutoGovernanceTraces,
		EventHub:             runtime.EventHub(),
		ResolveDecision: func(ctx context.Context, ticketID common.TicketID, resolutionType string, payload map[string]any) error {
			if !decisioncommon.IsSupportedResolutionType(resolutionType) {
				resolutionType = decisioncommon.ResolutionTypeProvideMissingInfo
			}
			return decisionSvc.EnsureResolvedTicket(ctx, decisionmodule.ResolveTicketCommand{
				ResolutionID: common.ResolutionID("resolution-" + string(ticketID) + "-auto-governance"),
				TicketID:     ticketID,
				Type:         resolutionType,
				Payload:      payload,
			})
		},
		ResolveApproval: func(ctx context.Context, approvalID common.ApprovalID) error {
			return approvalSvc.EnsureApproved(ctx, approvalmodule.ResolveApprovalCommand{ApprovalID: approvalID})
		},
		ResumeRun: func(ctx context.Context, runID common.RunID) error {
			return runSvc.EnsureResumed(ctx, runmodule.ResumeRunCommand{RunID: runID})
		},
		LoadConfig: workorchconfig.Current,
	})
	if setupErr != nil {
		return cd.NewError(cd.Unexpected, setupErr.Error())
	}
	return nil
}

func (p *plugin) Run() *cd.Error {
	if p == nil || p.module == nil {
		return cd.NewError(cd.IllegalParam, "governance module is not configured")
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

func workorchRuntime() (runtimecommon.Helper, *cd.Error) {
	var helper runtimecommon.Helper
	return initiator.GetEntity(runtimecommon.InitiatorID, helper)
}
