package runtimeports

import (
	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	"github.com/muidea/workorch/internal/pkg/events"
)

func NewEventHubDependencies(hub events.Hub, commandHub magicevent.Hub) runtimecore.Dependencies {
	actionPort := NewEventHubActionPort(commandHub)
	return runtimecore.Dependencies{
		Snapshot: NewEventHubSnapshotReadPort(commandHub),
		Actions: runtimecore.ActionDependencies{
			Workspace: actionPort,
			Run:       actionPort,
			Decision:  actionPort,
			Approval:  actionPort,
		},
	}
}
