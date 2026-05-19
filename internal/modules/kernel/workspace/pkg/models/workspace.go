package workspace

import "github.com/muidea/workorch/internal/pkg/common"

type Workspace struct {
	ID                   common.WorkspaceID
	Title                string
	Description          string
	DeliverableDirectory string
	InternalDirectory    string
	State                common.WorkspaceState
	ActiveRunID          common.RunID
	TemplateID           string
	ProfileID            string
	DefaultSoulID        common.SoulID
	SoulBindings         map[string]common.SoulID
	ContextHints         map[string]any
	EvidencePolicies     []map[string]any
}
