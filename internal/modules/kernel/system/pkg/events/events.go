package events

const (
	TopicSystemCommandEnsureSoulDefaults              = "/system/command/ensure-soul-defaults"
	TopicSystemCommandEnsureWorkspaceTemplateDefaults = "/system/command/ensure-workspace-template-defaults"
	TopicSystemCommandEnsureMCPDefaults               = "/system/command/ensure-mcp-defaults"
)

const (
	SoulDefaultsDestination              = "blocks/soul"
	WorkspaceTemplateDefaultsDestination = "blocks/workspace"
	MCPDefaultsDestination               = "blocks/mcp-registry"
)

type EnsureSoulDefaultsCommand struct{}

type EnsureSoulDefaultsResult struct {
	Ensured bool
}

type EnsureWorkspaceTemplateDefaultsCommand struct{}

type EnsureWorkspaceTemplateDefaultsResult struct {
	Ensured bool
}

type EnsureMCPDefaultsCommand struct{}

type EnsureMCPDefaultsResult struct {
	Ensured bool
}
