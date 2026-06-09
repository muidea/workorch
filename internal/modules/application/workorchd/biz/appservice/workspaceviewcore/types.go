package workspaceviewcore

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/pkg/events"
)

const (
	StateBuilding = "building"
	StateReady    = "ready"
	StateStale    = "stale"
	StateDegraded = "degraded"
	StateFailed   = "failed"
	StateStopped  = "stopped"
)

type Status struct {
	State               string `json:"state"`
	GlobalRevision      int64  `json:"globalRevision"`
	WorkspaceRevision   int64  `json:"workspaceRevision,omitempty"`
	SourceEventRevision int64  `json:"sourceEventRevision,omitempty"`
	WorkspaceTotal      int    `json:"workspaceTotal"`
	UpdatedAt           string `json:"updatedAt,omitempty"`
	RebuiltAt           string `json:"rebuiltAt,omitempty"`
	Degraded            bool   `json:"degraded,omitempty"`
	Error               string `json:"error,omitempty"`
}

type Loader interface {
	ListWorkspaces(context.Context) ([]appmodelcore.Workspace, error)
	ReadWorkspace(context.Context, string) (appmodelcore.Workspace, error)
	ReadWorkspaceSnapshot(context.Context, string) (readmodelcore.WorkspaceSnapshot, error)
}

type Scheduler func(name string, fn func(context.Context)) bool

type ManagerOptions struct {
	Loader   Loader
	Hub      events.Hub
	Schedule Scheduler
}
