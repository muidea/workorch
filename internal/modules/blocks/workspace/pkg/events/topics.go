package events

const NotificationDestination = "/workspace/consumers/#"

const (
	TopicWorkspaceCreated      = "/workspace/created"
	TopicWorkspaceStateChanged = "/workspace/state/changed"
	TopicWorkspaceDeleted      = "/workspace/deleted"
	TopicGoalDefined           = "/goal/defined"
	TopicDoDDefined            = "/dod/defined"

	TopicWorkspaceCommandCreate             = "/workspace/command/create"
	TopicWorkspaceCommandGet                = "/workspace/command/get"
	TopicWorkspaceCommandList               = "/workspace/command/list"
	TopicWorkspaceCommandUpdate             = "/workspace/command/update"
	TopicWorkspaceCommandUpdateLLMSelection = "/workspace/command/update-llm-selection"
	TopicWorkspaceCommandDelete             = "/workspace/command/delete"
	TopicWorkspaceCommandDefineGoal         = "/workspace/command/define-goal"
	TopicWorkspaceCommandUpdateGoal         = "/workspace/command/update-goal"
	TopicWorkspaceCommandDeleteGoal         = "/workspace/command/delete-goal"
	TopicWorkspaceCommandGetGoal            = "/workspace/command/get-goal"
	TopicWorkspaceCommandListGoalsByWS      = "/workspace/command/list-goals-by-workspace"
	TopicWorkspaceCommandDefineDoD          = "/workspace/command/define-dod"
	TopicWorkspaceCommandUpdateDoD          = "/workspace/command/update-dod"
	TopicWorkspaceCommandDeleteDoD          = "/workspace/command/delete-dod"
	TopicWorkspaceCommandGetDoD             = "/workspace/command/get-dod"
	TopicWorkspaceCommandListDoDsByWS       = "/workspace/command/list-dods-by-workspace"

	TopicWorkspaceTemplateCommandCreate         = "/workspace-template/command/create"
	TopicWorkspaceTemplateCommandGet            = "/workspace-template/command/get"
	TopicWorkspaceTemplateCommandList           = "/workspace-template/command/list"
	TopicWorkspaceTemplateCommandUpdate         = "/workspace-template/command/update"
	TopicWorkspaceTemplateCommandDelete         = "/workspace-template/command/delete"
	TopicWorkspaceTemplateCommandEnsureDefaults = "/workspace-template/command/ensure-defaults"

	// RUNTIME-SMO owner-local projection / terminal delivery.
	// TopicWorkspaceCommandExecutionIntent is the only external ingress for a
	// workspace execution request. Workspace owns active-run slot selection and
	// turns this immutable intent into a Run-owner command; callers cannot name
	// a lifecycle target or mutate the slot themselves.
	TopicWorkspaceCommandExecutionIntent    = "/workspace/command/execution-intent"
	TopicWorkspaceFactTerminalDelivery      = "/workspace/fact/terminal-delivery"
	TopicWorkspaceDefinitionAdmissionIntent = "/workspace/definition-admission/intent"
)
