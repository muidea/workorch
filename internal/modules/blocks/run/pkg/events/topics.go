package events

const NotificationDestination = "/run/consumers/#"

const (
	TopicRunCreated           = "/run/created"
	TopicRunStarted           = "/run/started"
	TopicRunResumed           = "/run/resumed"
	TopicRunBlockedOnDecision = "/run/blocked_on_decision"
	TopicRunBlockedOnApproval = "/run/blocked_on_approval"
	TopicRunCompleted         = "/run/completed"
	TopicRunFailed            = "/run/failed"
	TopicRunCancelled         = "/run/cancelled"
	TopicRunSuperseded        = "/run/superseded"
	TopicRunStateChanged      = "/run/state/changed"
	// TopicRunContinuationRequested is an owner notification for an accepted
	// Continue intent against an already-active Run. It asks SM-4 for a re-drive
	// without claiming a second SM-1 state transition.
	TopicRunContinuationRequested             = "/run/continuation-requested"
	TopicRunCommandCreate                     = "/run/command/create"
	TopicRunCommandEnsure                     = "/run/command/ensure"
	TopicRunCommandGet                        = "/run/command/get"
	TopicRunCommandList                       = "/run/command/list"
	TopicRunCommandListByWorkspace            = "/run/command/list-by-workspace"
	TopicRunCommandRuntimeV2RecoveryInventory = "/run/command/runtime-v2-recovery-inventory"
	TopicRunCommandGetStep                    = "/run/command/get-step"
	TopicRunCommandListStepsByRun             = "/run/command/list-steps-by-run"
	TopicRunCommandListStepSummariesByRun     = "/run/command/list-step-summaries-by-run"
	TopicRunCommandGetRuntimeCheckpoint       = "/run/command/get-runtime-checkpoint"
	TopicRunCommandListCheckpointsByRun       = "/run/command/list-runtime-checkpoints-by-run"
	TopicRunCommandPurgeRuntimeResources      = "/run/command/purge-runtime-resources"
	TopicRunCommandUpdate                     = "/run/command/update"
	TopicRunCommandLinkSuccessor              = "/run/command/link-successor"
	TopicRunCommandDelete                     = "/run/command/delete"

	// RUNTIME-SMO owner-local lifecycle commands. External inputs never carry a
	// target RunState; only Run Biz may migrate SM-1.
	TopicRunCommandActivate                      = "/run/command/activate"
	TopicRunCommandContinue                      = "/run/command/continue"
	TopicRunCommandApplyGovernanceWait           = "/run/command/apply-governance-wait"
	TopicRunCommandApplyContinuationEligible     = "/run/command/apply-continuation-eligible"
	TopicRunCommandApplyTerminalDelivery         = "/run/command/apply-terminal-delivery"
	TopicRunCommandApplyRuntimeStateIncompatible = "/run/command/apply-runtime-state-incompatible"
)
