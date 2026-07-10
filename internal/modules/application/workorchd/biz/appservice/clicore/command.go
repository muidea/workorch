package clicore

import (
	"time"

	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
)

// Command is the dispatcher-internal normalized command shape. It is created
// from the owner-defined typed EventHub command and is never transported.
type Command struct {
	CommandID     string
	CorrelationID string
	Actor         string
	IssuedAt      time.Time
	WorkspaceID   common.WorkspaceID
	RunID         common.RunID
	SubjectType   string
	SubjectID     string
	Command       string
	Payload       map[string]any
}

const typedResourceKey = "__workorchTypedResource"

func TypedResource(resource map[string]any) any {
	if resource == nil {
		return nil
	}
	return resource[typedResourceKey]
}

const (
	CommandWorkspaceCreate   = workorchdevents.CommandWorkspaceCreate
	CommandWorkspaceGet      = workorchdevents.CommandWorkspaceGet
	CommandWorkspaceList     = workorchdevents.CommandWorkspaceList
	CommandWorkspaceSnapshot = workorchdevents.CommandWorkspaceSnapshot
	CommandWorkspaceDelete   = workorchdevents.CommandWorkspaceDelete
	CommandGoalDefine        = workorchdevents.CommandGoalDefine
	CommandGoalList          = workorchdevents.CommandGoalList
	CommandGoalGet           = workorchdevents.CommandGoalGet
	CommandGoalUpdate        = workorchdevents.CommandGoalUpdate
	CommandDoDDefine         = workorchdevents.CommandDoDDefine
	CommandDoDList           = workorchdevents.CommandDoDList
	CommandDoDGet            = workorchdevents.CommandDoDGet
	CommandDoDUpdate         = workorchdevents.CommandDoDUpdate
	CommandRunActivate       = workorchdevents.CommandRunActivate
	CommandRunResume         = workorchdevents.CommandRunResume
	CommandRunGet            = workorchdevents.CommandRunGet
	CommandRunList           = workorchdevents.CommandRunList
	CommandRunStatus         = workorchdevents.CommandRunStatus
	CommandRunCancel         = workorchdevents.CommandRunCancel
	CommandDecisionResolve   = workorchdevents.CommandDecisionResolve
	CommandDecisionBatch     = workorchdevents.CommandDecisionBatch
	CommandDecisionList      = workorchdevents.CommandDecisionList
	CommandDecisionGet       = workorchdevents.CommandDecisionGet
	CommandApprovalApprove   = workorchdevents.CommandApprovalApprove
	CommandApprovalReject    = workorchdevents.CommandApprovalReject
	CommandApprovalBatch     = workorchdevents.CommandApprovalBatch
	CommandApprovalList      = workorchdevents.CommandApprovalList
	CommandApprovalGet       = workorchdevents.CommandApprovalGet
	CommandSoulCreate        = workorchdevents.CommandSoulCreate
	CommandSoulGet           = workorchdevents.CommandSoulGet
	CommandSoulList          = workorchdevents.CommandSoulList
	CommandSoulUpdate        = workorchdevents.CommandSoulUpdate
	CommandSoulDelete        = workorchdevents.CommandSoulDelete
	CommandSkillCreate       = workorchdevents.CommandSkillCreate
	CommandSkillGet          = workorchdevents.CommandSkillGet
	CommandSkillList         = workorchdevents.CommandSkillList
	CommandMCPServerCreate   = workorchdevents.CommandMCPServerCreate
	CommandMCPServerGet      = workorchdevents.CommandMCPServerGet
	CommandMCPServerList     = workorchdevents.CommandMCPServerList
	CommandMCPServerUpdate   = workorchdevents.CommandMCPServerUpdate
	CommandMCPCapabilityList = workorchdevents.CommandMCPCapabilityList
	CommandMemoryCreate      = workorchdevents.CommandMemoryCreate
	CommandMemoryGet         = workorchdevents.CommandMemoryGet
	CommandMemoryUpdate      = workorchdevents.CommandMemoryUpdate
	CommandMemoryDelete      = workorchdevents.CommandMemoryDelete
	CommandMemoryList        = workorchdevents.CommandMemoryList
	CommandMemorySearch      = workorchdevents.CommandMemorySearch
	CommandTemplateCreate    = workorchdevents.CommandTemplateCreate
	CommandTemplateGet       = workorchdevents.CommandTemplateGet
	CommandTemplateList      = workorchdevents.CommandTemplateList
	CommandTemplateUpdate    = workorchdevents.CommandTemplateUpdate
	CommandTemplateDelete    = workorchdevents.CommandTemplateDelete
	CommandConfigurationGet  = workorchdevents.CommandConfigurationGet
)
