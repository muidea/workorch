package events

import (
	pathpkg "path"
	"sort"
	"strings"
	"unicode/utf8"

	modulecommon "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/common"
	mcpmodel "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

const definitionPackage = "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/events"
const CommandDestination = modulecommon.ModuleID

// EnvironmentReference is deliberately reference-only. A credential value is
// resolved by the MCP execution owner and never crosses EventHub.
type EnvironmentReference struct {
	Name      string
	Reference string
}

func EnvironmentReferencesFromMap(values map[string]string) []EnvironmentReference {
	ret := make([]EnvironmentReference, 0, len(values))
	for name, reference := range values {
		ret = append(ret, EnvironmentReference{Name: name, Reference: reference})
	}
	sort.Slice(ret, func(i, j int) bool { return ret[i].Name < ret[j].Name })
	return ret
}

func EnvironmentReferenceMap(values []EnvironmentReference) (map[string]string, error) {
	if len(values) > modulecommon.MaxEnvironmentRefs {
		return nil, common.ErrInvalidArgument
	}
	ret := make(map[string]string, len(values))
	for _, item := range values {
		name := strings.TrimSpace(item.Name)
		reference := strings.TrimSpace(item.Reference)
		if !validEnvironmentName(name) || !utf8.ValidString(reference) || len(reference) > modulecommon.MaxTextBytes || !ValidEnvironmentReference(reference) {
			return nil, common.ErrInvalidArgument
		}
		if _, exists := ret[name]; exists {
			return nil, common.ErrInvalidArgument
		}
		ret[name] = reference
	}
	return ret, nil
}

func ValidEnvironmentReference(reference string) bool {
	reference = strings.TrimSpace(reference)
	if strings.HasPrefix(reference, "env://") {
		return validEnvironmentName(strings.TrimPrefix(reference, "env://"))
	}
	switch reference {
	case "{workspaceId}", "{runId}", "{stepId}", "{workspaceDeliverableDirectory}", "{workspaceInternalDirectory}":
		return true
	}
	const prefix = "{workspaceManagedConfig:"
	if !strings.HasPrefix(reference, prefix) || !strings.HasSuffix(reference, "}") {
		return false
	}
	referencePath := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(reference, prefix), "}"))
	if referencePath == "" || strings.Contains(referencePath, "\\") || pathpkg.IsAbs(referencePath) || pathpkg.Clean(referencePath) != referencePath {
		return false
	}
	for _, part := range strings.Split(referencePath, "/") {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	return true
}

func validEnvironmentName(value string) bool {
	if value == "" || len(value) > modulecommon.MaxNameBytes {
		return false
	}
	for idx, char := range []byte(value) {
		if idx == 0 {
			if char != '_' && (char < 'A' || char > 'Z') && (char < 'a' || char > 'z') {
				return false
			}
			continue
		}
		if char != '_' && (char < 'A' || char > 'Z') && (char < 'a' || char > 'z') && (char < '0' || char > '9') {
			return false
		}
	}
	return true
}

type CreateServerCommand struct {
	ServerID              common.MCPServerID
	Name                  string
	Transport             string
	Endpoint              string
	Command               string
	Args                  []string
	EnvironmentRefs       []EnvironmentReference
	Enabled               bool
	AllowedTools          []string
	AllowedResources      []string
	AllowedPrompts        []string
	DefaultApprovalPolicy string
	SessionPolicy         string
	TimeoutMillis         int
}

type UpdateServerCommand CreateServerCommand

type MCPServer struct {
	ID                    common.MCPServerID
	Name                  string
	Transport             string
	Endpoint              string
	Command               string
	Args                  []string
	EnvironmentRefs       []EnvironmentReference
	Enabled               bool
	AllowedTools          []string
	AllowedResources      []string
	AllowedPrompts        []string
	DefaultApprovalPolicy string
	SessionPolicy         string
	TimeoutMillis         int
}

func MCPServerFromModel(item *mcpmodel.Server) (*MCPServer, error) {
	if item == nil {
		return nil, common.ErrInvalidArgument
	}
	refs := EnvironmentReferencesFromMap(item.Env)
	if _, err := EnvironmentReferenceMap(refs); err != nil {
		return nil, err
	}
	return &MCPServer{
		ID: item.ID, Name: item.Name, Transport: item.Transport, Endpoint: item.Endpoint, Command: item.Command,
		Args: append([]string(nil), item.Args...), EnvironmentRefs: refs, Enabled: item.Enabled,
		AllowedTools: append([]string(nil), item.AllowedTools...), AllowedResources: append([]string(nil), item.AllowedResources...),
		AllowedPrompts: append([]string(nil), item.AllowedPrompts...), DefaultApprovalPolicy: item.DefaultApprovalPolicy,
		SessionPolicy: item.SessionPolicy, TimeoutMillis: item.TimeoutMillis,
	}, nil
}

func MCPServersFromModels(items []*mcpmodel.Server) ([]*MCPServer, error) {
	ret := make([]*MCPServer, 0, len(items))
	for _, item := range items {
		value, err := MCPServerFromModel(item)
		if err != nil {
			return nil, err
		}
		ret = append(ret, value)
	}
	return ret, nil
}

func (s *MCPServer) Model() (*mcpmodel.Server, error) {
	if s == nil {
		return nil, common.ErrInvalidArgument
	}
	env, err := EnvironmentReferenceMap(s.EnvironmentRefs)
	if err != nil {
		return nil, err
	}
	return &mcpmodel.Server{
		ID: s.ID, Name: s.Name, Transport: s.Transport, Endpoint: s.Endpoint, Command: s.Command,
		Args: append([]string(nil), s.Args...), Env: env, Enabled: s.Enabled,
		AllowedTools: append([]string(nil), s.AllowedTools...), AllowedResources: append([]string(nil), s.AllowedResources...),
		AllowedPrompts: append([]string(nil), s.AllowedPrompts...), DefaultApprovalPolicy: s.DefaultApprovalPolicy,
		SessionPolicy: s.SessionPolicy, TimeoutMillis: s.TimeoutMillis,
	}, nil
}

type CreateCapabilityCommand struct {
	CapabilityID                 common.MCPCapabilityID
	ServerID                     common.MCPServerID
	Kind                         string
	Name                         string
	DisplayName                  string
	Description                  string
	Category                     string
	Tags                         []string
	UsageHint                    string
	InputSummary                 string
	OutputSummary                string
	StageHints                   []string
	RiskLevel                    string
	EffectScope                  string
	IdempotencyClass             string
	ValidationProofApplicability string
	Schema                       workorcheventpayload.Payload
	URI                          string
	Enabled                      bool
	ApprovalPolicy               string
}

type UpdateCapabilityCommand CreateCapabilityCommand

type CapabilityQuery struct {
	ServerID  common.MCPServerID
	Kind      string
	Category  string
	Stage     string
	RiskLevel string
	Enabled   *bool
	Query     string
}

type GetServerCommand struct{ ServerID common.MCPServerID }
type ListServersCommand struct {
	CursorAfterID common.MCPServerID
	Limit         int
}
type GetCapabilityCommand struct{ CapabilityID common.MCPCapabilityID }
type ListCapabilitiesCommand struct {
	Query         CapabilityQuery
	CursorAfterID common.MCPCapabilityID
	Limit         int
}
type ReadRegistrySnapshotCommand struct{}
type ReadSelectionSnapshotCommand struct {
	ServerIDs     []common.MCPServerID
	CapabilityIDs []common.MCPCapabilityID
}
type EnsureDefaultsCommand struct{}

type CreateServerResult struct {
	Server           *MCPServer
	RegistryRevision string
}
type GetServerResult struct {
	Server           *MCPServer
	RegistryRevision string
}
type ListServersResult struct {
	Servers          []*MCPServer
	NextCursor       common.MCPServerID
	RegistryRevision string
}
type UpdateServerResult struct {
	Server           *MCPServer
	RegistryRevision string
}
type CreateCapabilityResult struct {
	Capability       *mcpmodel.Capability
	RegistryRevision string
}
type GetCapabilityResult struct {
	Capability       *mcpmodel.Capability
	RegistryRevision string
}
type ListCapabilitiesResult struct {
	Capabilities     []*mcpmodel.Capability
	NextCursor       common.MCPCapabilityID
	RegistryRevision string
}
type UpdateCapabilityResult struct {
	Capability       *mcpmodel.Capability
	RegistryRevision string
}
type RegistrySnapshotResult struct {
	Servers          []*MCPServer
	Capabilities     []*mcpmodel.Capability
	RegistryRevision string
}
type SelectionSnapshotResult struct {
	Servers          []*MCPServer
	Capabilities     []*mcpmodel.Capability
	RegistryRevision string
}
type EnsureDefaultsResult struct {
	Ensured          bool
	RegistryRevision string
}

func MessageContracts() []eventcontract.MessageContract {
	return []eventcontract.MessageContract{
		eventcontract.CommandTopic[CreateServerCommand, CreateServerResult](contractOwner, definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicMCPCommandCreateServer).MessageContract(),
		eventcontract.CommandTopic[GetServerCommand, GetServerResult](contractOwner, definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicMCPCommandGetServer).MessageContract(),
		eventcontract.CommandTopic[ListServersCommand, ListServersResult](contractOwner, definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicMCPCommandListServers).MessageContract(),
		eventcontract.CommandTopic[UpdateServerCommand, UpdateServerResult](contractOwner, definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicMCPCommandUpdateServer).MessageContract(),
		eventcontract.CommandTopic[CreateCapabilityCommand, CreateCapabilityResult](contractOwner, definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicMCPCommandCreateCapability).MessageContract(),
		eventcontract.CommandTopic[GetCapabilityCommand, GetCapabilityResult](contractOwner, definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicMCPCommandGetCapability).MessageContract(),
		eventcontract.CommandTopic[ListCapabilitiesCommand, ListCapabilitiesResult](contractOwner, definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicMCPCommandListCapabilities).MessageContract(),
		eventcontract.CommandTopic[UpdateCapabilityCommand, UpdateCapabilityResult](contractOwner, definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicMCPCommandUpdateCapability).MessageContract(),
		eventcontract.CommandTopic[ReadRegistrySnapshotCommand, RegistrySnapshotResult](contractOwner, definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicMCPCommandReadRegistrySnapshot).MessageContract(),
		eventcontract.CommandTopic[ReadSelectionSnapshotCommand, SelectionSnapshotResult](contractOwner, definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicMCPCommandReadSelectionSnapshot).MessageContract(),
		eventcontract.CommandTopic[EnsureDefaultsCommand, EnsureDefaultsResult](contractOwner, definitionPackage, eventcontract.CommandClassWrite, eventcontract.LanePolicyState, TopicMCPCommandEnsureDefaults).MessageContract(),
	}
}
