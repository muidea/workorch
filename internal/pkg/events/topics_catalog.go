package events

const (
	TopicSoulCommandCreate             = "/soul/command/create"
	TopicSoulCommandGet                = "/soul/command/get"
	TopicSoulCommandList               = "/soul/command/list"
	TopicSoulCommandUpdate             = "/soul/command/update"
	TopicSoulCommandDelete             = "/soul/command/delete"
	TopicSoulCommandValidateCapability = "/soul/command/validate-capability"

	TopicSkillCommandCreate = "/skill/command/create"
	TopicSkillCommandGet    = "/skill/command/get"
	TopicSkillCommandList   = "/skill/command/list"
	TopicSkillCommandUpdate = "/skill/command/update"

	TopicMCPCommandCreateServer     = "/mcp/command/create-server"
	TopicMCPCommandGetServer        = "/mcp/command/get-server"
	TopicMCPCommandListServers      = "/mcp/command/list-servers"
	TopicMCPCommandUpdateServer     = "/mcp/command/update-server"
	TopicMCPCommandCreateCapability = "/mcp/command/create-capability"
	TopicMCPCommandGetCapability    = "/mcp/command/get-capability"
	TopicMCPCommandListCapabilities = "/mcp/command/list-capabilities"
	TopicMCPCommandUpdateCapability = "/mcp/command/update-capability"
)
