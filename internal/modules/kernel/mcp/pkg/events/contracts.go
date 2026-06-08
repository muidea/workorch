package events

import "github.com/muidea/workorch/internal/pkg/eventcontract"

func Contracts() []eventcontract.Contract {
	ret := []eventcontract.Contract{}
	ret = append(ret, eventcontract.Build("mcp", eventcontract.CommandClassRead, eventcontract.LanePolicyQuery,
		TopicMCPCommandGetServer,
		TopicMCPCommandListServers,
		TopicMCPCommandGetCapability,
		TopicMCPCommandListCapabilities,
	)...)
	ret = append(ret, eventcontract.Build("mcp", eventcontract.CommandClassWrite, eventcontract.LanePolicyState,
		TopicMCPCommandCreateServer,
		TopicMCPCommandUpdateServer,
		TopicMCPCommandCreateCapability,
		TopicMCPCommandUpdateCapability,
	)...)
	return ret
}
