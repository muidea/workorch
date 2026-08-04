package governancecore

import (
	"context"

	governancemodel "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/models"
	"github.com/muidea/workorch/internal/pkg/workorchquerycontract"
)

type AutoGovernanceTraceReader interface {
	List(context.Context, workorchquerycontract.AutoGovernanceTraceQuery) ([]*governancemodel.AutoGovernanceTrace, error)
}
