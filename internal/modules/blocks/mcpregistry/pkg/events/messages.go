package events

import (
	"sort"
	"strings"

	modulecommon "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/common"
	mcpmodel "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

const definitionPackage = "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/events"
const CommandDestination = modulecommon.ModuleID

type CreateServerCommand struct {
	ServerID              common.MCPServerID
	Name                  string
	Transport             string
	Endpoint              string
	Command               string
	Args                  []string
	Env                   []EnvironmentVariable
	Enabled               bool
	AllowedTools          []string
	AllowedResources      []string
	AllowedPrompts        []string
	DefaultApprovalPolicy string
	SessionPolicy         string
	TimeoutMillis         int
}

type UpdateServerCommand struct {
	ServerID              common.MCPServerID
	Name                  string
	Transport             string
	Endpoint              string
	Command               string
	Args                  []string
	Env                   []EnvironmentVariable
	Enabled               bool
	AllowedTools          []string
	AllowedResources      []string
	AllowedPrompts        []string
	DefaultApprovalPolicy string
	SessionPolicy         string
	TimeoutMillis         int
}

// EnvironmentVariable is the stable EventHub representation of one MCP
// process environment entry. Maps remain private to the registry owner and
// provider adapters so message ordering and duplicate handling are explicit.
type EnvironmentVariable struct {
	Name  string
	Value string
}

func EnvironmentVariablesFromMap(values map[string]string) []EnvironmentVariable {
	ret := make([]EnvironmentVariable, 0, len(values))
	for name, value := range values {
		ret = append(ret, EnvironmentVariable{Name: name, Value: value})
	}
	sort.Slice(ret, func(i, j int) bool { return ret[i].Name < ret[j].Name })
	return ret
}

func EnvironmentVariableMap(values []EnvironmentVariable) (map[string]string, error) {
	ret := make(map[string]string, len(values))
	for _, item := range values {
		if strings.TrimSpace(item.Name) == "" {
			return nil, common.ErrInvalidArgument
		}
		if _, exists := ret[item.Name]; exists {
			return nil, common.ErrInvalidArgument
		}
		ret[item.Name] = item.Value
	}
	return ret, nil
}

// MCPServer is an owner-local immutable result DTO. It deliberately does not
// expose the registry persistence model or its map-backed Env field.
type MCPServer struct {
	ID                    common.MCPServerID
	Name                  string
	Transport             string
	Endpoint              string
	Command               string
	Args                  []string
	Env                   []EnvironmentVariable
	Enabled               bool
	AllowedTools          []string
	AllowedResources      []string
	AllowedPrompts        []string
	DefaultApprovalPolicy string
	SessionPolicy         string
	TimeoutMillis         int
}

func MCPServerFromModel(item *mcpmodel.Server) *MCPServer {
	if item == nil {
		return nil
	}
	return &MCPServer{
		ID: item.ID, Name: item.Name, Transport: item.Transport, Endpoint: item.Endpoint, Command: item.Command,
		Args: append([]string(nil), item.Args...), Env: EnvironmentVariablesFromMap(item.Env), Enabled: item.Enabled,
		AllowedTools: append([]string(nil), item.AllowedTools...), AllowedResources: append([]string(nil), item.AllowedResources...),
		AllowedPrompts: append([]string(nil), item.AllowedPrompts...), DefaultApprovalPolicy: item.DefaultApprovalPolicy,
		SessionPolicy: item.SessionPolicy, TimeoutMillis: item.TimeoutMillis,
	}
}

func MCPServersFromModels(items []*mcpmodel.Server) []*MCPServer {
	ret := make([]*MCPServer, 0, len(items))
	for _, item := range items {
		ret = append(ret, MCPServerFromModel(item))
	}
	return ret
}

func (s *MCPServer) Model() (*mcpmodel.Server, error) {
	if s == nil {
		return nil, common.ErrInvalidArgument
	}
	env, err := EnvironmentVariableMap(s.Env)
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
	ID               common.MCPCapabilityID
	ServerID         common.MCPServerID
	Kind             string
	Name             string
	DisplayName      string
	Description      string
	Category         string
	Tags             []string
	UsageHint        string
	InputSummary     string
	OutputSummary    string
	StageHints       []string
	RiskLevel        string
	EffectScope      string
	IdempotencyClass string
	Schema           workorcheventpayload.Payload
	URI              string
	Enabled          bool
	ApprovalPolicy   string
	Status           string
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

type GetServerCommand struct {
	ServerID common.MCPServerID
}

type ListServersCommand struct{}

type GetCapabilityCommand struct {
	CapabilityID common.MCPCapabilityID
}

type ListCapabilitiesCommand struct {
	Query CapabilityQuery
}

type EnsureDefaultsCommand struct{}

type EnsureDefaultsResult struct {
	Ensured bool
}

type MCPCommandResult struct {
	Server       *MCPServer
	Servers      []*MCPServer
	Capability   *mcpmodel.Capability
	Capabilities []*mcpmodel.Capability
}

func MessageContracts() []eventcontract.MessageContract {
	ret := []eventcontract.MessageContract{}
	for _, topic := range []string{
		TopicMCPCommandCreateServer,
		TopicMCPCommandGetServer,
		TopicMCPCommandListServers,
		TopicMCPCommandUpdateServer,
		TopicMCPCommandCreateCapability,
		TopicMCPCommandGetCapability,
		TopicMCPCommandListCapabilities,
		TopicMCPCommandUpdateCapability,
		TopicMCPCommandEnsureDefaults,
	} {
		class := eventcontract.CommandClassWrite
		lane := eventcontract.LanePolicyState
		switch topic {
		case TopicMCPCommandGetServer, TopicMCPCommandListServers, TopicMCPCommandGetCapability, TopicMCPCommandListCapabilities:
			class = eventcontract.CommandClassRead
			lane = eventcontract.LanePolicyQuery
		}
		switch topic {
		case TopicMCPCommandCreateServer:
			ret = append(ret, eventcontract.CommandTopic[CreateServerCommand, MCPCommandResult](contractOwner, definitionPackage, class, lane, topic).MessageContract())
		case TopicMCPCommandGetServer:
			ret = append(ret, eventcontract.CommandTopic[GetServerCommand, MCPCommandResult](contractOwner, definitionPackage, class, lane, topic).MessageContract())
		case TopicMCPCommandListServers:
			ret = append(ret, eventcontract.CommandTopic[ListServersCommand, MCPCommandResult](contractOwner, definitionPackage, class, lane, topic).MessageContract())
		case TopicMCPCommandUpdateServer:
			ret = append(ret, eventcontract.CommandTopic[UpdateServerCommand, MCPCommandResult](contractOwner, definitionPackage, class, lane, topic).MessageContract())
		case TopicMCPCommandCreateCapability:
			ret = append(ret, eventcontract.CommandTopic[CreateCapabilityCommand, MCPCommandResult](contractOwner, definitionPackage, class, lane, topic).MessageContract())
		case TopicMCPCommandGetCapability:
			ret = append(ret, eventcontract.CommandTopic[GetCapabilityCommand, MCPCommandResult](contractOwner, definitionPackage, class, lane, topic).MessageContract())
		case TopicMCPCommandListCapabilities:
			ret = append(ret, eventcontract.CommandTopic[ListCapabilitiesCommand, MCPCommandResult](contractOwner, definitionPackage, class, lane, topic).MessageContract())
		case TopicMCPCommandUpdateCapability:
			ret = append(ret, eventcontract.CommandTopic[UpdateCapabilityCommand, MCPCommandResult](contractOwner, definitionPackage, class, lane, topic).MessageContract())
		case TopicMCPCommandEnsureDefaults:
			ret = append(ret, eventcontract.CommandTopic[EnsureDefaultsCommand, EnsureDefaultsResult](contractOwner, definitionPackage, class, lane, topic).MessageContract())
		}
	}
	return ret
}
