package events

const NotificationDestination = "/run/consumers/#"

const (
	TopicRunCreated                       = "/run/created"
	TopicRunStarted                       = "/run/started"
	TopicRunResumed                       = "/run/resumed"
	TopicRunBlockedOnDecision             = "/run/blocked_on_decision"
	TopicRunBlockedOnApproval             = "/run/blocked_on_approval"
	TopicRunCompleted                     = "/run/completed"
	TopicRunFailed                        = "/run/failed"
	TopicRunCancelled                     = "/run/cancelled"
	TopicRunSuperseded                    = "/run/superseded"
	TopicRunStateChanged                  = "/run/state/changed"
	TopicRunCommandCreate                 = "/run/command/create"
	TopicRunCommandEnsure                 = "/run/command/ensure"
	TopicRunCommandGet                    = "/run/command/get"
	TopicRunCommandList                   = "/run/command/list"
	TopicRunCommandListByWorkspace        = "/run/command/list-by-workspace"
	TopicRunCommandGetStep                = "/run/command/get-step"
	TopicRunCommandListStepsByRun         = "/run/command/list-steps-by-run"
	TopicRunCommandListStepSummariesByRun = "/run/command/list-step-summaries-by-run"
	TopicRunCommandGetRuntimeCheckpoint   = "/run/command/get-runtime-checkpoint"
	TopicRunCommandListCheckpointsByRun   = "/run/command/list-runtime-checkpoints-by-run"
	TopicRunCommandUpdate                 = "/run/command/update"
	TopicRunCommandDelete                 = "/run/command/delete"
)
