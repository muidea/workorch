package appservice

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"

type Workspace = appmodelcore.Workspace
type CreateWorkspaceCommand = appmodelcore.CreateWorkspaceCommand
type ActivateWorkspaceExecutionCommand = appmodelcore.ActivateWorkspaceExecutionCommand
type DeleteWorkspaceCommand = appmodelcore.DeleteWorkspaceCommand
type WorkspaceSoulBindings = appmodelcore.WorkspaceSoulBindings
type SoulRoutingBinding = appmodelcore.SoulRoutingBinding
type SoulRouting = appmodelcore.SoulRouting
type Goal = appmodelcore.Goal
type DoD = appmodelcore.DoD
type Run = appmodelcore.Run
type Decision = appmodelcore.Decision
type DecisionQuery = appmodelcore.DecisionQuery
type DecisionPage = appmodelcore.DecisionPage
type ResolveDecisionCommand = appmodelcore.ResolveDecisionCommand
type BatchResolveDecisionCommand = appmodelcore.BatchResolveDecisionCommand
type Approval = appmodelcore.Approval
type ApprovalQuery = appmodelcore.ApprovalQuery
type ApprovalPage = appmodelcore.ApprovalPage
type BatchResolveApprovalCommand = appmodelcore.BatchResolveApprovalCommand

const (
	SoulBindingStagePlanning   = appmodelcore.SoulBindingStagePlanning
	SoulBindingStageExecution  = appmodelcore.SoulBindingStageExecution
	SoulBindingStageDecision   = appmodelcore.SoulBindingStageDecision
	SoulBindingStageApproval   = appmodelcore.SoulBindingStageApproval
	SoulBindingStageValidation = appmodelcore.SoulBindingStageValidation
	SoulBindingStageArtifact   = appmodelcore.SoulBindingStageArtifact
)
