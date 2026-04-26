package mcp

import "github.com/muidea/workorch/internal/pkg/common"

type Server struct {
	ID                    common.MCPServerID
	Name                  string
	Transport             string
	Endpoint              string
	Enabled               bool
	AllowedTools          []string
	AllowedResources      []string
	DefaultApprovalPolicy string
	TimeoutMillis         int
}
