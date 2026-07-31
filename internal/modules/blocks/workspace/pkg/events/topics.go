package events

const NotificationDestination = "/workspace/consumers/#"

const (
	TopicWorkspaceCreated      = "/workspace/created"
	TopicWorkspaceStateChanged = "/workspace/state/changed"
	TopicWorkspaceDeleted      = "/workspace/deleted"
	TopicGoalDefined           = "/goal/defined"
	TopicDoDDefined            = "/dod/defined"

	TopicWorkspaceCommandCreate          = "/workspace/command/create"
	TopicWorkspaceCommandEnsure          = "/workspace/command/ensure"
	TopicWorkspaceCommandGet             = "/workspace/command/get"
	TopicWorkspaceCommandList            = "/workspace/command/list"
	TopicWorkspaceCommandUpdate          = "/workspace/command/update"
	TopicWorkspaceCommandUpdateLLMTarget = "/workspace/command/update-llm-target"
	TopicWorkspaceCommandDelete          = "/workspace/command/delete"
	TopicWorkspaceCommandPurge           = "/workspace/command/purge"
	TopicWorkspaceCommandDefineGoal      = "/workspace/command/define-goal"
	TopicWorkspaceCommandUpdateGoal      = "/workspace/command/update-goal"
	TopicWorkspaceCommandDeleteGoal      = "/workspace/command/delete-goal"
	TopicWorkspaceCommandGetGoal         = "/workspace/command/get-goal"
	TopicWorkspaceCommandListGoalsByWS   = "/workspace/command/list-goals-by-workspace"
	TopicWorkspaceCommandDefineDoD       = "/workspace/command/define-dod"
	TopicWorkspaceCommandUpdateDoD       = "/workspace/command/update-dod"
	TopicWorkspaceCommandDeleteDoD       = "/workspace/command/delete-dod"
	TopicWorkspaceCommandGetDoD          = "/workspace/command/get-dod"
	TopicWorkspaceCommandListDoDsByWS    = "/workspace/command/list-dods-by-workspace"

	TopicWorkspaceTemplateCommandCreate = "/workspace-template/command/create"
	TopicWorkspaceTemplateCommandGet    = "/workspace-template/command/get"
	TopicWorkspaceTemplateCommandList   = "/workspace-template/command/list"
	TopicWorkspaceTemplateCommandUpdate = "/workspace-template/command/update"
	TopicWorkspaceTemplateCommandDelete = "/workspace-template/command/delete"

	// RUNTIME-SMO owner-local projection / terminal delivery.
	TopicWorkspaceCommandApplyRunProjectionFact = "/workspace/command/apply-run-projection-fact"
	// TopicWorkspaceCommandExecutionIntent is the only external ingress for a
	// workspace execution request. Workspace owns active-run slot selection and
	// turns this immutable intent into a Run-owner command; callers cannot name
	// a lifecycle target or mutate the slot themselves.
	TopicWorkspaceCommandExecutionIntent = "/workspace/command/execution-intent"
	// TopicWorkspaceCommandReleaseFreshActivationRunSlot consumes a persisted
	// Run-owned non-resumable fact. Workspace alone decides whether its active
	// slot may be released; this is not a terminal delivery or a Run-state
	// transition.
	TopicWorkspaceCommandReleaseFreshActivationRunSlot = "/workspace/command/release-fresh-activation-run-slot"
	TopicWorkspaceCommandApplyTerminalDelivery         = "/workspace/command/apply-terminal-delivery"
)
