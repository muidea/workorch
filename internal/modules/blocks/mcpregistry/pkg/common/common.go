package common

const (
	ModuleID = "blocks/mcp-registry"

	DefaultListLimit         = 100
	MaxListLimit             = 200
	MaxRegistryServers       = 256
	MaxRegistryCapabilities  = 4096
	MaxSelectionServers      = 64
	MaxSelectionCapabilities = 256

	MaxIDBytes         = 512
	MaxNameBytes       = 512
	MaxTextBytes       = 16 * 1024
	MaxSchemaBytes     = 64 * 1024
	MaxStringListItems = 512
	MaxStringItemBytes = 4 * 1024
	MaxQueryBytes      = 4 * 1024
	MaxEnvironmentRefs = 64
)
