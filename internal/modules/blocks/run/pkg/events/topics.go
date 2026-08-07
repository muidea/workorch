package events

const NotificationDestination = "/run/consumers/#"

const (
	TopicRunCreated           = "/run/created"
	TopicRunStarted           = "/run/started"
	TopicRunBlockedOnDecision = "/run/blocked_on_decision"
	TopicRunBlockedOnApproval = "/run/blocked_on_approval"
	TopicRunCompleted         = "/run/completed"
	TopicRunFailed            = "/run/failed"
	TopicRunCancelled         = "/run/cancelled"
	TopicRunSuperseded        = "/run/superseded"
	TopicRunStateChanged      = "/run/state/changed"
	// TopicRunRuntimeCommandsReady is a process-lifecycle fact, not a Run
	// state transition. It releases startup recovery actions only after the Run
	// owner's regular state commands have been registered.
	TopicRunRuntimeCommandsReady = "/run/runtime-commands-ready"
	// TopicRunContinuationRequested is an owner notification for an accepted
	// Continue intent against an already-active Run. It asks SM-4 for a re-drive
	// without claiming a second SM-1 state transition.
	TopicRunContinuationRequested           = "/run/continuation-requested"
	TopicRunCommandCreate                   = "/run/command/create"
	TopicRunCommandGet                      = "/run/command/get"
	TopicRunCommandList                     = "/run/command/list"
	TopicRunCommandListByWorkspace          = "/run/command/list-by-workspace"
	TopicRunCommandRuntimeRecoveryInventory = "/run/command/runtime-recovery-inventory"
	// Runtime recovery reads are available during Setup because
	// Orchestrator performs its synchronous SM-4 inventory/load before the Run
	// module reaches Run. They are deliberately narrower than the normal Get
	// command, which remains a Run-phase API.
	TopicRunCommandRuntimeStateRecordReadiness = "/run/command/runtime-state-record-readiness"
	TopicRunCommandRuntimeStateRecord          = "/run/command/runtime-state-record"
	TopicRunCommandGetStep                     = "/run/command/get-step"
	TopicRunCommandListStepsByRun              = "/run/command/list-steps-by-run"
	TopicRunCommandListStepSummariesByRun      = "/run/command/list-step-summaries-by-run"
	TopicRunCommandGetRuntimeCheckpoint        = "/run/command/get-runtime-checkpoint"
	TopicRunCommandListCheckpointsByRun        = "/run/command/list-runtime-checkpoints-by-run"
	TopicRunCommandPurgeRuntimeResources       = "/run/command/purge-runtime-resources"
	TopicRunCommandStoreRuntimeStateRecord     = "/run/command/store-runtime-state-record"
	TopicRunCommandDelete                      = "/run/command/delete"

	// RUNTIME-SMO owner-local lifecycle commands. External inputs never carry a
	// target RunState; only Run Biz may migrate SM-1.
	TopicRunFactGovernanceWait           = "/run/fact/governance-wait"
	TopicRunFactContinuationEligible     = "/run/fact/continuation-eligible"
	TopicRunFactTerminalDelivery         = "/run/fact/terminal-delivery"
	TopicRunFactRuntimeStateIncompatible = "/run/fact/runtime-state-incompatible"
	// TopicRunCommandWorkspaceExecutionIntent receives a Workspace-owner
	// selection/authorization fact. Run alone decides the resulting SM-1 work.
	TopicRunCommandWorkspaceExecutionIntent = "/run/command/workspace-execution-intent"
)
