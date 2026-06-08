package runtimeevent

import "github.com/muidea/workorch/internal/modules/blocks/pkg/runtimeevent/eventdefs"

const TopicRuntimeBlockCommandExecutePrefix = eventdefs.TopicRuntimeBlockCommandExecutePrefix

func RuntimeBlockExecuteTopic(kind string) string {
	return eventdefs.RuntimeBlockExecuteTopic(kind)
}
