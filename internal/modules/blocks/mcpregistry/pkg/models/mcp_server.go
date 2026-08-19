package mcp

import "github.com/muidea/workorch/internal/pkg/common"

// Server is the MCP Registry owner model. Env values are references only; the
// execution owner resolves them after the EventHub boundary.
type Server struct {
	ID                    common.MCPServerID `json:"id"`
	Name                  string             `json:"name"`
	Transport             string             `json:"transport"`
	Endpoint              string             `json:"endpoint,omitempty"`
	Command               string             `json:"command,omitempty"`
	Args                  []string           `json:"args,omitempty"`
	Env                   map[string]string  `json:"environmentRefs,omitempty"`
	Enabled               bool               `json:"enabled"`
	AllowedTools          []string           `json:"allowedTools,omitempty"`
	AllowedResources      []string           `json:"allowedResources,omitempty"`
	AllowedPrompts        []string           `json:"allowedPrompts,omitempty"`
	DefaultApprovalPolicy string             `json:"defaultApprovalPolicy,omitempty"`
	SessionPolicy         string             `json:"sessionPolicy,omitempty"`
	TimeoutMillis         int                `json:"timeoutMillis,omitempty"`
}
