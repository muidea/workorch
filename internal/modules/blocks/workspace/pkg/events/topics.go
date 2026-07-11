package events

const NotificationDestination = "/workspace/consumers/#"

const (
	TopicWorkspaceCreated      = "/workspace/created"
	TopicWorkspaceStateChanged = "/workspace/state/changed"
	TopicWorkspaceDeleted      = "/workspace/deleted"
	TopicGoalDefined           = "/goal/defined"
	TopicDoDDefined            = "/dod/defined"

	TopicWorkspaceCommandCreate        = "/workspace/command/create"
	TopicWorkspaceCommandEnsure        = "/workspace/command/ensure"
	TopicWorkspaceCommandGet           = "/workspace/command/get"
	TopicWorkspaceCommandList          = "/workspace/command/list"
	TopicWorkspaceCommandUpdate        = "/workspace/command/update"
	TopicWorkspaceCommandDelete        = "/workspace/command/delete"
	TopicWorkspaceCommandPurge         = "/workspace/command/purge"
	TopicWorkspaceCommandDefineGoal    = "/workspace/command/define-goal"
	TopicWorkspaceCommandUpdateGoal    = "/workspace/command/update-goal"
	TopicWorkspaceCommandDeleteGoal    = "/workspace/command/delete-goal"
	TopicWorkspaceCommandGetGoal       = "/workspace/command/get-goal"
	TopicWorkspaceCommandListGoalsByWS = "/workspace/command/list-goals-by-workspace"
	TopicWorkspaceCommandDefineDoD     = "/workspace/command/define-dod"
	TopicWorkspaceCommandUpdateDoD     = "/workspace/command/update-dod"
	TopicWorkspaceCommandDeleteDoD     = "/workspace/command/delete-dod"
	TopicWorkspaceCommandGetDoD        = "/workspace/command/get-dod"
	TopicWorkspaceCommandListDoDsByWS  = "/workspace/command/list-dods-by-workspace"

	TopicWorkspaceTemplateCommandCreate = "/workspace-template/command/create"
	TopicWorkspaceTemplateCommandGet    = "/workspace-template/command/get"
	TopicWorkspaceTemplateCommandList   = "/workspace-template/command/list"
	TopicWorkspaceTemplateCommandUpdate = "/workspace-template/command/update"
	TopicWorkspaceTemplateCommandDelete = "/workspace-template/command/delete"
)
