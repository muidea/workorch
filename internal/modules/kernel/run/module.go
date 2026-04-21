package run

import (
	runbiz "github.com/muidea/workorch/internal/modules/kernel/run/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/kernel/run/pkg/common"
	runservice "github.com/muidea/workorch/internal/modules/kernel/run/service"
	"github.com/muidea/workorch/internal/pkg/repository"
)

const ModuleID = modulecommon.ModuleID

type Service = runbiz.Service
type Orchestrator = runbiz.Orchestrator
type CreateRunCommand = runservice.CreateRunCommand
type StartRunCommand = runservice.StartRunCommand
type ResumeRunCommand = runservice.ResumeRunCommand
type BlockOnDecisionCommand = runservice.BlockOnDecisionCommand
type BlockOnApprovalCommand = runservice.BlockOnApprovalCommand
type CompleteRunCommand = runservice.CompleteRunCommand
type FailRunCommand = runservice.FailRunCommand

func NewService(
	runs repository.RunRepository,
	workspaces repository.WorkspaceRepository,
	orchestrator runbiz.Orchestrator,
) *runbiz.Service {
	return runbiz.NewService(runs, workspaces, orchestrator)
}
