package runtimeevent

import "github.com/muidea/workorch/internal/modules/blocks/pkg/runtimeevent/eventdefs"

const TopicRuntimeBlockCommandExecutePrefix = eventdefs.TopicRuntimeBlockCommandExecutePrefix

const (
	TopicRuntimeBlockExecuteAccepted  = eventdefs.TopicRuntimeBlockExecuteAccepted
	TopicRuntimeBlockExecuteProgress  = eventdefs.TopicRuntimeBlockExecuteProgress
	TopicRuntimeBlockExecuteCompleted = eventdefs.TopicRuntimeBlockExecuteCompleted
	TopicRuntimeBlockExecuteFailed    = eventdefs.TopicRuntimeBlockExecuteFailed
)

func RuntimeBlockExecuteTopic(kind string) string {
	return eventdefs.RuntimeBlockExecuteTopic(kind)
}
