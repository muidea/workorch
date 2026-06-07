package runtimeports

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	"github.com/muidea/workorch/internal/pkg/events"
)

func NewEventHubDependencies(hub events.Hub) runtimecore.Dependencies {
	actionPort := NewEventHubActionPort(hub)
	return runtimecore.Dependencies{
		Snapshot: NewEventHubSnapshotReadPort(hub),
		Actions: runtimecore.ActionDependencies{
			Workspace: actionPort,
			Run:       actionPort,
			Decision:  actionPort,
			Approval:  actionPort,
		},
	}
}
