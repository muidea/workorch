package events

import (
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/common"
	mcpcontracts "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/contracts"
	mcpmodel "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

const definitionPackage = "github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/events"
const CommandDestination = modulecommon.ModuleID

type CreateServerCommand = mcpcontracts.CreateServerCommand
type UpdateServerCommand = mcpcontracts.UpdateServerCommand
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

type UpdateCapabilityCommand = CreateCapabilityCommand
type CapabilityQuery = mcpcontracts.CapabilityQuery

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
	Server       *mcpmodel.Server
	Servers      []*mcpmodel.Server
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
			ret = append(ret, eventcontract.CommandTopic[CreateServerCommand, MCPCommandResult]("mcp", definitionPackage, class, lane, topic).MessageContract())
		case TopicMCPCommandGetServer:
			ret = append(ret, eventcontract.CommandTopic[GetServerCommand, MCPCommandResult]("mcp", definitionPackage, class, lane, topic).MessageContract())
		case TopicMCPCommandListServers:
			ret = append(ret, eventcontract.CommandTopic[ListServersCommand, MCPCommandResult]("mcp", definitionPackage, class, lane, topic).MessageContract())
		case TopicMCPCommandUpdateServer:
			ret = append(ret, eventcontract.CommandTopic[UpdateServerCommand, MCPCommandResult]("mcp", definitionPackage, class, lane, topic).MessageContract())
		case TopicMCPCommandCreateCapability:
			ret = append(ret, eventcontract.CommandTopic[CreateCapabilityCommand, MCPCommandResult]("mcp", definitionPackage, class, lane, topic).MessageContract())
		case TopicMCPCommandGetCapability:
			ret = append(ret, eventcontract.CommandTopic[GetCapabilityCommand, MCPCommandResult]("mcp", definitionPackage, class, lane, topic).MessageContract())
		case TopicMCPCommandListCapabilities:
			ret = append(ret, eventcontract.CommandTopic[ListCapabilitiesCommand, MCPCommandResult]("mcp", definitionPackage, class, lane, topic).MessageContract())
		case TopicMCPCommandUpdateCapability:
			ret = append(ret, eventcontract.CommandTopic[UpdateCapabilityCommand, MCPCommandResult]("mcp", definitionPackage, class, lane, topic).MessageContract())
		case TopicMCPCommandEnsureDefaults:
			ret = append(ret, eventcontract.CommandTopic[EnsureDefaultsCommand, EnsureDefaultsResult]("mcp", definitionPackage, class, lane, topic).MessageContract())
		}
	}
	return ret
}
