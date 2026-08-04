package workorchclient

import (
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

// Workspace resource types (matches workorchd HTTP JSON format with lowercase camelCase tags).
type WorkspaceListProjection struct {
	Projection projectioncontract.ProjectionStatus `json:"projection"`
	Workspaces []projectioncontract.Workspace      `json:"workspaces"`
}

// Snapshot is the full workspace/run snapshot returned by the read model.
type WorkspaceSnapshotProjection struct {
	Projection projectioncontract.ProjectionStatus  `json:"projection"`
	Snapshot   projectioncontract.WorkspaceSnapshot `json:"snapshot"`
}

type WorkspaceRuntimeViewProjection struct {
	Service     string                                  `json:"service"`
	Backend     string                                  `json:"backend,omitempty"`
	DataRoot    string                                  `json:"dataRoot,omitempty"`
	Projection  projectioncontract.ProjectionStatus     `json:"projection"`
	RuntimeView projectioncontract.WorkspaceRuntimeView `json:"runtimeView"`
	Error       string                                  `json:"error,omitempty"`
}
