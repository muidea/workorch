package events

import (
	"fmt"
	"strings"
	"unicode/utf8"

	modulecommon "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/common"
	governancemodel "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	"github.com/muidea/workorch/internal/pkg/workorchquerycontract"
)

const definitionPackage = "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/events"
const CommandDestination = modulecommon.ModuleID

const (
	MaxAutoGovernanceTraceQueryLimit       = 500
	MaxAutoGovernanceTraceQueryStringBytes = 4 * 1024
)

type ListAutoGovernanceTracesCommand struct {
	Query workorchquerycontract.AutoGovernanceTraceQuery
}

// Normalized validates the complete public trace-query scope before the
// Governance owner touches its repository. Public queries are always bounded
// and workspace-scoped; owner-private startup recovery uses its repository
// inventory directly and does not pass through this command.
func (c ListAutoGovernanceTracesCommand) Normalized() (ListAutoGovernanceTracesCommand, error) {
	values := []string{
		string(c.Query.WorkspaceID), string(c.Query.RunID), string(c.Query.StepID),
		c.Query.ItemType, c.Query.ItemID, c.Query.Status,
	}
	for _, value := range values {
		if len(value) > MaxAutoGovernanceTraceQueryStringBytes || !utf8.ValidString(value) || value != strings.TrimSpace(value) {
			return ListAutoGovernanceTracesCommand{}, fmt.Errorf("%w: governance trace query contains a noncanonical or oversized identity", common.ErrInvalidArgument)
		}
	}
	if c.Query.WorkspaceID == "" || c.Query.Limit < 1 || c.Query.Limit > MaxAutoGovernanceTraceQueryLimit {
		return ListAutoGovernanceTracesCommand{}, fmt.Errorf("%w: governance trace query requires workspace scope and limit between 1 and %d", common.ErrInvalidArgument, MaxAutoGovernanceTraceQueryLimit)
	}
	return c, nil
}

type GovernanceData struct {
	Trace            governancemodel.AutoGovernanceTrace
	EscalationReason string
}

type GovernanceCommandResult struct {
	Traces []*governancemodel.AutoGovernanceTrace
}

func (r GovernanceCommandResult) Clone() GovernanceCommandResult {
	return GovernanceCommandResult{Traces: governancemodel.CloneAutoGovernanceTraces(r.Traces)}
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
