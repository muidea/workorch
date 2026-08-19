package events

import "github.com/muidea/workorch/internal/pkg/eventcontract"

const contractOwner = "mcp-registry"

func Contracts() []eventcontract.Contract {
	ret := []eventcontract.Contract{}
	ret = append(ret, eventcontract.Build(contractOwner, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery,
		TopicMCPCommandGetServer,
		TopicMCPCommandListServers,
		TopicMCPCommandGetCapability,
		TopicMCPCommandListCapabilities,
		TopicMCPCommandReadRegistrySnapshot,
		TopicMCPCommandReadSelectionSnapshot,
	)...)
	ret = append(ret, eventcontract.Build(contractOwner, eventcontract.CommandClassWrite, eventcontract.LanePolicyState,
		TopicMCPCommandCreateServer,
		TopicMCPCommandUpdateServer,
		TopicMCPCommandCreateCapability,
		TopicMCPCommandUpdateCapability,
		TopicMCPCommandEnsureDefaults,
	)...)
	return ret
}
