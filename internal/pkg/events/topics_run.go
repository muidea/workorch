package events

const (
	TopicRunCreated           = "/run/created"
	TopicRunStateChanged      = "/run/state/changed"
	TopicRunLLMStreamProgress = "/run/llm_stream/progress"
	TopicRunLLMUsageRecorded  = "/run/llm_usage/recorded"
	TopicStepDispatched       = "/step/dispatched"
	TopicStepStateChanged     = "/step/state/changed"

	TopicRunCommandCreate               = "/run/command/create"
	TopicRunCommandEnsure               = "/run/command/ensure"
	TopicRunCommandGet                  = "/run/command/get"
	TopicRunCommandList                 = "/run/command/list"
	TopicRunCommandListByWorkspace      = "/run/command/list-by-workspace"
	TopicRunCommandGetStep              = "/run/command/get-step"
	TopicRunCommandListStepsByRun       = "/run/command/list-steps-by-run"
	TopicRunCommandGetRuntimeCheckpoint = "/run/command/get-runtime-checkpoint"
	TopicRunCommandListCheckpointsByRun = "/run/command/list-runtime-checkpoints-by-run"
	TopicRunCommandUpdate               = "/run/command/update"
	TopicRunCommandDelete               = "/run/command/delete"
	TopicRunCommandStart                = "/run/command/start"
	TopicRunCommandSupersede            = "/run/command/supersede"
	TopicRunCommandFail                 = "/run/command/fail"
	TopicRunCommandEnsureResumed        = "/run/command/ensure-resumed"
)
