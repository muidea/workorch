package events

import (
	modulecommon "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/common"
	governancemodel "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/models"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	"github.com/muidea/workorch/internal/pkg/workorchquerycontract"
)

const definitionPackage = "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/events"
const CommandDestination = modulecommon.ModuleID

type ListAutoGovernanceTracesCommand struct {
	Query workorchquerycontract.AutoGovernanceTraceQuery
}

type GovernanceData struct {
	Trace            governancemodel.AutoGovernanceTrace
	EscalationReason string
}

type GovernanceCommandResult struct {
	Traces []*governancemodel.AutoGovernanceTrace
}

func MessageContracts() []eventcontract.MessageContract {
	ret := []eventcontract.MessageContract{
		eventcontract.CommandTopic[ListAutoGovernanceTracesCommand, GovernanceCommandResult]("governance", definitionPackage, eventcontract.CommandClassRead, eventcontract.LanePolicyQuery, TopicGovernanceAutoCommandListTraces).MessageContract(),
	}
	for _, topic := range []string{
		TopicGovernanceAutoEvaluated,
		TopicGovernanceAutoResolved,
		TopicGovernanceAutoEscalated,
		TopicGovernanceAutoBlocked,
	} {
		ret = append(ret, eventcontract.DataTopic[GovernanceData]("governance", definitionPackage, eventcontract.LanePolicyState, topic).MessageContract())
	}
	return ret
}
