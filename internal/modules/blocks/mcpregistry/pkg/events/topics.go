package events

import "github.com/muidea/workorch/internal/pkg/workorchownercontract"

const (
	TopicMCPCommandCreateServer     = "/mcp/command/create-server"
	TopicMCPCommandGetServer        = workorchownercontract.TopicMCPCommandGetServer
	TopicMCPCommandListServers      = "/mcp/command/list-servers"
	TopicMCPCommandUpdateServer     = "/mcp/command/update-server"
	TopicMCPCommandCreateCapability = "/mcp/command/create-capability"
	TopicMCPCommandGetCapability    = "/mcp/command/get-capability"
	TopicMCPCommandListCapabilities = "/mcp/command/list-capabilities"
	TopicMCPCommandUpdateCapability = "/mcp/command/update-capability"
	TopicMCPCommandEnsureDefaults   = "/mcp/command/ensure-defaults"
)
