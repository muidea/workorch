package workorchownercontract

import "github.com/muidea/workorch/internal/pkg/common"

const TopicMCPCommandGetServer = "/mcp/command/get-server"

type MCPServer struct {
	ID                    common.MCPServerID
	Name                  string
	Transport             string
	Endpoint              string
	Command               string
	Args                  []string
	Env                   map[string]string
	Enabled               bool
	AllowedTools          []string
	AllowedResources      []string
	AllowedPrompts        []string
	DefaultApprovalPolicy string
	SessionPolicy         string
	TimeoutMillis         int
}
