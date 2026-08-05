package events

import (
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
	workorchport "github.com/muidea/workorch/internal/pkg/workorchport"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type CLIWorkspaceCreateCommand struct {
	Request workorchmanagementcontract.WorkspaceCreateRequest
}
type CLIWorkspaceCreateResult struct{ Workspace CLIWorkspaceInfo }
type CLIWorkspaceGetCommand struct{ WorkspaceID common.WorkspaceID }
type CLIWorkspaceGetResult struct{ Workspace CLIWorkspaceInfo }
type CLIWorkspaceListCommand struct{}
type CLIWorkspaceListResult struct{ Workspaces []CLIWorkspaceInfo }
type CLIWorkspaceSnapshotCommand struct{ WorkspaceID common.WorkspaceID }
type CLIWorkspaceSnapshotResult struct {
	Snapshot CLIWorkspaceSnapshotInfo
}
type CLIWorkspaceDeleteCommand struct {
	WorkspaceID common.WorkspaceID
	Purge       bool
}
type CLIWorkspaceDeleteResult struct{ Deleted bool }
type CLIWorkspaceUpdateLLMTargetCommand struct {
	WorkspaceID common.WorkspaceID
	Target      *runtimecontract.ProviderModelTarget
}
type CLIWorkspaceUpdateLLMTargetResult struct{ Workspace CLIWorkspaceInfo }

type CLIGoalDefineCommand struct {
	WorkspaceID common.WorkspaceID
	Text        string
}
type CLIGoalDefineResult struct{ Goal workorchport.GoalInfo }
type CLIGoalListCommand struct{ WorkspaceID common.WorkspaceID }
type CLIGoalListResult struct{ Goals []workorchport.GoalInfo }
type CLIGoalGetCommand struct {
	WorkspaceID common.WorkspaceID
	GoalID      common.GoalID
}
type CLIGoalGetResult struct{ Goal workorchport.GoalInfo }
type CLIGoalUpdateCommand struct {
	WorkspaceID common.WorkspaceID
	GoalID      common.GoalID
	Text        string
}
type CLIGoalUpdateResult struct{ Goal workorchport.GoalInfo }

type CLIDoDDefineCommand struct {
	WorkspaceID common.WorkspaceID
	Items       []string
}
type CLIDoDDefineResult struct{ DoD workorchport.DoDInfo }
type CLIDoDListCommand struct{ WorkspaceID common.WorkspaceID }
type CLIDoDListResult struct{ DoDs []workorchport.DoDInfo }
type CLIDoDGetCommand struct {
	WorkspaceID common.WorkspaceID
	DoDID       common.DoDID
}
type CLIDoDGetResult struct{ DoD workorchport.DoDInfo }
type CLIDoDUpdateCommand struct {
	WorkspaceID common.WorkspaceID
	DoDID       common.DoDID
	Items       []string
}
type CLIDoDUpdateResult struct{ DoD workorchport.DoDInfo }

type CLIRunGetCommand struct {
	WorkspaceID common.WorkspaceID
	RunID       common.RunID
}
type CLIRunGetResult struct{ Run workorchport.RunInfo }
type CLIRunListCommand struct{ WorkspaceID common.WorkspaceID }
type CLIRunListResult struct{ Runs []workorchport.RunInfo }
type CLIRunStatusCommand struct {
	WorkspaceID common.WorkspaceID
	RunID       common.RunID
}
type CLIRunStatusResult struct{ Status workorchport.RunStatusInfo }

type CLIDecisionResolveCommand struct {
	RequestID string
	Request   workorchport.ResolveDecisionRequest
}
type CLIDecisionResolveResult struct{ Decision workorchport.DecisionInfo }
type CLIDecisionBatchResolveCommand struct {
	RequestID      string
	WorkspaceID    common.WorkspaceID
	TicketIDs      []common.TicketID
	ResolutionType string
	Payload        workorcheventpayload.Payload
}
type CLIDecisionBatchResolveResult struct{ Decisions []workorchport.DecisionInfo }
type CLIDecisionListCommand struct{ WorkspaceID common.WorkspaceID }
type CLIDecisionListResult struct{ Decisions []workorchport.DecisionInfo }
type CLIDecisionGetCommand struct {
	WorkspaceID common.WorkspaceID
	TicketID    common.TicketID
}
type CLIDecisionGetResult struct{ Decision workorchport.DecisionInfo }

type CLIApprovalApproveCommand struct {
	RequestID   string
	WorkspaceID common.WorkspaceID
	ApprovalID  common.ApprovalID
	Payload     workorcheventpayload.Payload
}
type CLIApprovalApproveResult struct{ Approval workorchport.ApprovalInfo }
type CLIApprovalRejectCommand struct {
	RequestID   string
	WorkspaceID common.WorkspaceID
	ApprovalID  common.ApprovalID
	Payload     workorcheventpayload.Payload
}
type CLIApprovalRejectResult struct{ Approval workorchport.ApprovalInfo }
type CLIApprovalBatchResolveCommand struct {
	RequestID   string
	WorkspaceID common.WorkspaceID
	ApprovalIDs []common.ApprovalID
	Action      string
	Payload     workorcheventpayload.Payload
}
type CLIApprovalBatchResolveResult struct{ Approvals []workorchport.ApprovalInfo }
type CLIApprovalListCommand struct{ WorkspaceID common.WorkspaceID }
type CLIApprovalListResult struct{ Approvals []workorchport.ApprovalInfo }
type CLIApprovalGetCommand struct {
	WorkspaceID common.WorkspaceID
	ApprovalID  common.ApprovalID
}
type CLIApprovalGetResult struct{ Approval workorchport.ApprovalInfo }

type CLISoulCreateCommand struct{ Soul workorchport.SoulInfo }
type CLISoulCreateResult struct{ Soul workorchport.SoulInfo }
type CLISoulGetCommand struct{ SoulID common.SoulID }
type CLISoulGetResult struct{ Soul workorchport.SoulInfo }
type CLISoulListCommand struct{}
type CLISoulListResult struct{ Souls []workorchport.SoulInfo }
type CLISoulUpdateCommand struct {
	SoulID common.SoulID
	Soul   workorchport.SoulInfo
}
type CLISoulUpdateResult struct{ Soul workorchport.SoulInfo }
type CLISoulDeleteCommand struct{ SoulID common.SoulID }
type CLISoulDeleteResult struct{ Deleted bool }

type CLISkillCreateCommand struct{ Skill workorchport.SkillInfo }
type CLISkillCreateResult struct{ Skill workorchport.SkillInfo }
type CLISkillGetCommand struct {
	WorkspaceID common.WorkspaceID
	SkillID     common.SkillID
}
type CLISkillGetResult struct{ Skill workorchport.SkillInfo }
type CLISkillListCommand struct{ WorkspaceID common.WorkspaceID }
type CLISkillListResult struct{ Skills []workorchport.SkillInfo }

type CLIMCPServerCreateCommand struct{ Server workorchport.MCPServerInfo }
type CLIMCPServerCreateResult struct{ Server workorchport.MCPServerInfo }
type CLIMCPServerGetCommand struct{ ServerID common.MCPServerID }
type CLIMCPServerGetResult struct{ Server workorchport.MCPServerInfo }
type CLIMCPServerListCommand struct{}
type CLIMCPServerListResult struct{ Servers []workorchport.MCPServerInfo }
type CLIMCPServerUpdateCommand struct {
	ServerID common.MCPServerID
	Server   workorchport.MCPServerInfo
}
type CLIMCPServerUpdateResult struct{ Server workorchport.MCPServerInfo }
type CLIMCPCapabilityListCommand struct{}
type CLIMCPCapabilityListResult struct {
	Capabilities []workorchport.MCPCapabilityInfo
}

type CLIMemoryCreateCommand struct {
	WorkspaceID common.WorkspaceID
	Memory      workorchport.MemoryItemInfo
}
type CLIMemoryCreateResult struct{ Memory workorchport.MemoryItemInfo }
type CLIMemoryGetCommand struct {
	WorkspaceID common.WorkspaceID
	MemoryID    common.MemoryID
}
type CLIMemoryGetResult struct{ Memory workorchport.MemoryItemInfo }
type CLIMemoryUpdateCommand struct {
	WorkspaceID common.WorkspaceID
	MemoryID    common.MemoryID
	Memory      workorchport.MemoryItemInfo
}
type CLIMemoryUpdateResult struct{ Memory workorchport.MemoryItemInfo }
type CLIMemoryDeleteCommand struct {
	WorkspaceID common.WorkspaceID
	MemoryID    common.MemoryID
}
type CLIMemoryDeleteResult struct{ Deleted bool }
type CLIMemoryListCommand struct{ WorkspaceID common.WorkspaceID }
type CLIMemoryListResult struct{ Memories []workorchport.MemoryItemInfo }
type CLIMemorySearchCommand struct {
	WorkspaceID common.WorkspaceID
	Request     workorchport.MemorySearchRequest
}
type CLIMemorySearchResult struct{ Memories []workorchport.MemoryItemInfo }

type CLITemplateCreateCommand struct{ Template CLITemplateInfo }
type CLITemplateCreateResult struct{ Template CLITemplateInfo }
type CLITemplateGetCommand struct{ TemplateID string }
type CLITemplateGetResult struct{ Template CLITemplateInfo }
type CLITemplateListCommand struct{}
type CLITemplateListResult struct{ Templates []CLITemplateInfo }
type CLITemplateUpdateCommand struct {
	TemplateID string
	Template   CLITemplateInfo
}
type CLITemplateUpdateResult struct{ Template CLITemplateInfo }
type CLITemplateDeleteCommand struct{ TemplateID string }
type CLITemplateDeleteResult struct{ Deleted bool }

type CLIConfigurationGetCommand struct{}
type CLIConfigurationGetResult struct{ Configuration workorchport.ServiceConfig }
type CLIConfigurationRefreshChatModelsCommand struct{ ProviderID string }
type CLIConfigurationRefreshChatModelsResult struct{ Configuration workorchport.ServiceConfig }

func CLIMessageContracts() []eventcontract.MessageContract {
	return eventcontract.MessageContracts(
		eventcontract.FacadeTopic[CLIWorkspaceCreateCommand, CLIWorkspaceCreateResult]("cli.workspace", definitionPackage, TopicCLIWorkspaceCreate),
		eventcontract.FacadeTopic[CLIWorkspaceGetCommand, CLIWorkspaceGetResult]("cli.workspace", definitionPackage, TopicCLIWorkspaceGet),
		eventcontract.FacadeTopic[CLIWorkspaceListCommand, CLIWorkspaceListResult]("cli.workspace", definitionPackage, TopicCLIWorkspaceList),
		eventcontract.FacadeTopic[CLIWorkspaceSnapshotCommand, CLIWorkspaceSnapshotResult]("cli.workspace", definitionPackage, TopicCLIWorkspaceSnapshot),
		eventcontract.FacadeTopic[CLIWorkspaceDeleteCommand, CLIWorkspaceDeleteResult]("cli.workspace", definitionPackage, TopicCLIWorkspaceDelete),
		eventcontract.FacadeTopic[CLIWorkspaceUpdateLLMTargetCommand, CLIWorkspaceUpdateLLMTargetResult]("cli.workspace", definitionPackage, TopicCLIWorkspaceUpdateLLMTarget),
		eventcontract.FacadeTopic[CLIGoalDefineCommand, CLIGoalDefineResult]("cli.goal", definitionPackage, TopicCLIGoalDefine),
		eventcontract.FacadeTopic[CLIGoalListCommand, CLIGoalListResult]("cli.goal", definitionPackage, TopicCLIGoalList),
		eventcontract.FacadeTopic[CLIGoalGetCommand, CLIGoalGetResult]("cli.goal", definitionPackage, TopicCLIGoalGet),
		eventcontract.FacadeTopic[CLIGoalUpdateCommand, CLIGoalUpdateResult]("cli.goal", definitionPackage, TopicCLIGoalUpdate),
		eventcontract.FacadeTopic[CLIDoDDefineCommand, CLIDoDDefineResult]("cli.dod", definitionPackage, TopicCLIDoDDefine),
		eventcontract.FacadeTopic[CLIDoDListCommand, CLIDoDListResult]("cli.dod", definitionPackage, TopicCLIDoDList),
		eventcontract.FacadeTopic[CLIDoDGetCommand, CLIDoDGetResult]("cli.dod", definitionPackage, TopicCLIDoDGet),
		eventcontract.FacadeTopic[CLIDoDUpdateCommand, CLIDoDUpdateResult]("cli.dod", definitionPackage, TopicCLIDoDUpdate),
		eventcontract.FacadeTopic[CLIRunGetCommand, CLIRunGetResult]("cli.run", definitionPackage, TopicCLIRunGet),
		eventcontract.FacadeTopic[CLIRunListCommand, CLIRunListResult]("cli.run", definitionPackage, TopicCLIRunList),
		eventcontract.FacadeTopic[CLIRunStatusCommand, CLIRunStatusResult]("cli.run", definitionPackage, TopicCLIRunStatus),
		eventcontract.FacadeTopic[CLIDecisionResolveCommand, CLIDecisionResolveResult]("cli.decision", definitionPackage, TopicCLIDecisionResolve),
		eventcontract.FacadeTopic[CLIDecisionBatchResolveCommand, CLIDecisionBatchResolveResult]("cli.decision", definitionPackage, TopicCLIDecisionBatchResolve),
		eventcontract.FacadeTopic[CLIDecisionListCommand, CLIDecisionListResult]("cli.decision", definitionPackage, TopicCLIDecisionList),
		eventcontract.FacadeTopic[CLIDecisionGetCommand, CLIDecisionGetResult]("cli.decision", definitionPackage, TopicCLIDecisionGet),
		eventcontract.FacadeTopic[CLIApprovalApproveCommand, CLIApprovalApproveResult]("cli.approval", definitionPackage, TopicCLIApprovalApprove),
		eventcontract.FacadeTopic[CLIApprovalRejectCommand, CLIApprovalRejectResult]("cli.approval", definitionPackage, TopicCLIApprovalReject),
		eventcontract.FacadeTopic[CLIApprovalBatchResolveCommand, CLIApprovalBatchResolveResult]("cli.approval", definitionPackage, TopicCLIApprovalBatchResolve),
		eventcontract.FacadeTopic[CLIApprovalListCommand, CLIApprovalListResult]("cli.approval", definitionPackage, TopicCLIApprovalList),
		eventcontract.FacadeTopic[CLIApprovalGetCommand, CLIApprovalGetResult]("cli.approval", definitionPackage, TopicCLIApprovalGet),
		eventcontract.FacadeTopic[CLISoulCreateCommand, CLISoulCreateResult]("cli.soul", definitionPackage, TopicCLISoulCreate),
		eventcontract.FacadeTopic[CLISoulGetCommand, CLISoulGetResult]("cli.soul", definitionPackage, TopicCLISoulGet),
		eventcontract.FacadeTopic[CLISoulListCommand, CLISoulListResult]("cli.soul", definitionPackage, TopicCLISoulList),
		eventcontract.FacadeTopic[CLISoulUpdateCommand, CLISoulUpdateResult]("cli.soul", definitionPackage, TopicCLISoulUpdate),
		eventcontract.FacadeTopic[CLISoulDeleteCommand, CLISoulDeleteResult]("cli.soul", definitionPackage, TopicCLISoulDelete),
		eventcontract.FacadeTopic[CLISkillCreateCommand, CLISkillCreateResult]("cli.skill", definitionPackage, TopicCLISkillCreate),
		eventcontract.FacadeTopic[CLISkillGetCommand, CLISkillGetResult]("cli.skill", definitionPackage, TopicCLISkillGet),
		eventcontract.FacadeTopic[CLISkillListCommand, CLISkillListResult]("cli.skill", definitionPackage, TopicCLISkillList),
		eventcontract.FacadeTopic[CLIMCPServerCreateCommand, CLIMCPServerCreateResult]("cli.mcp", definitionPackage, TopicCLIMCPServerCreate),
		eventcontract.FacadeTopic[CLIMCPServerGetCommand, CLIMCPServerGetResult]("cli.mcp", definitionPackage, TopicCLIMCPServerGet),
		eventcontract.FacadeTopic[CLIMCPServerListCommand, CLIMCPServerListResult]("cli.mcp", definitionPackage, TopicCLIMCPServerList),
		eventcontract.FacadeTopic[CLIMCPServerUpdateCommand, CLIMCPServerUpdateResult]("cli.mcp", definitionPackage, TopicCLIMCPServerUpdate),
		eventcontract.FacadeTopic[CLIMCPCapabilityListCommand, CLIMCPCapabilityListResult]("cli.mcp", definitionPackage, TopicCLIMCPCapabilityList),
		eventcontract.FacadeTopic[CLIMemoryCreateCommand, CLIMemoryCreateResult]("cli.memory", definitionPackage, TopicCLIMemoryCreate),
		eventcontract.FacadeTopic[CLIMemoryGetCommand, CLIMemoryGetResult]("cli.memory", definitionPackage, TopicCLIMemoryGet),
		eventcontract.FacadeTopic[CLIMemoryUpdateCommand, CLIMemoryUpdateResult]("cli.memory", definitionPackage, TopicCLIMemoryUpdate),
		eventcontract.FacadeTopic[CLIMemoryDeleteCommand, CLIMemoryDeleteResult]("cli.memory", definitionPackage, TopicCLIMemoryDelete),
		eventcontract.FacadeTopic[CLIMemoryListCommand, CLIMemoryListResult]("cli.memory", definitionPackage, TopicCLIMemoryList),
		eventcontract.FacadeTopic[CLIMemorySearchCommand, CLIMemorySearchResult]("cli.memory", definitionPackage, TopicCLIMemorySearch),
		eventcontract.FacadeTopic[CLITemplateCreateCommand, CLITemplateCreateResult]("cli.template", definitionPackage, TopicCLITemplateCreate),
		eventcontract.FacadeTopic[CLITemplateGetCommand, CLITemplateGetResult]("cli.template", definitionPackage, TopicCLITemplateGet),
		eventcontract.FacadeTopic[CLITemplateListCommand, CLITemplateListResult]("cli.template", definitionPackage, TopicCLITemplateList),
		eventcontract.FacadeTopic[CLITemplateUpdateCommand, CLITemplateUpdateResult]("cli.template", definitionPackage, TopicCLITemplateUpdate),
		eventcontract.FacadeTopic[CLITemplateDeleteCommand, CLITemplateDeleteResult]("cli.template", definitionPackage, TopicCLITemplateDelete),
		eventcontract.FacadeTopic[CLIConfigurationGetCommand, CLIConfigurationGetResult]("cli.configuration", definitionPackage, TopicCLIConfigurationGet),
		eventcontract.FacadeTopic[CLIConfigurationRefreshChatModelsCommand, CLIConfigurationRefreshChatModelsResult]("cli.configuration", definitionPackage, TopicCLIConfigurationRefreshChatModels),
	)
}

func CLITopics() []string {
	return []string{
		TopicCLIWorkspaceCreate, TopicCLIWorkspaceGet, TopicCLIWorkspaceList, TopicCLIWorkspaceSnapshot, TopicCLIWorkspaceDelete, TopicCLIWorkspaceUpdateLLMTarget,
		TopicCLIGoalDefine, TopicCLIGoalList, TopicCLIGoalGet, TopicCLIGoalUpdate,
		TopicCLIDoDDefine, TopicCLIDoDList, TopicCLIDoDGet, TopicCLIDoDUpdate,
		TopicCLIRunGet, TopicCLIRunList, TopicCLIRunStatus,
		TopicCLIDecisionResolve, TopicCLIDecisionBatchResolve, TopicCLIDecisionList, TopicCLIDecisionGet,
		TopicCLIApprovalApprove, TopicCLIApprovalReject, TopicCLIApprovalBatchResolve, TopicCLIApprovalList, TopicCLIApprovalGet,
		TopicCLISoulCreate, TopicCLISoulGet, TopicCLISoulList, TopicCLISoulUpdate, TopicCLISoulDelete,
		TopicCLISkillCreate, TopicCLISkillGet, TopicCLISkillList,
		TopicCLIMCPServerCreate, TopicCLIMCPServerGet, TopicCLIMCPServerList, TopicCLIMCPServerUpdate, TopicCLIMCPCapabilityList,
		TopicCLIMemoryCreate, TopicCLIMemoryGet, TopicCLIMemoryUpdate, TopicCLIMemoryDelete, TopicCLIMemoryList, TopicCLIMemorySearch,
		TopicCLITemplateCreate, TopicCLITemplateGet, TopicCLITemplateList, TopicCLITemplateUpdate, TopicCLITemplateDelete,
		TopicCLIConfigurationGet, TopicCLIConfigurationRefreshChatModels,
	}
}
