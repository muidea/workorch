package governancecore

import (
	"context"

	governanceevents "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/events"
	governancemodel "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/models"
)

type AutoGovernanceTraceReader interface {
	List(context.Context, governanceevents.AutoGovernanceTraceQuery) ([]*governancemodel.AutoGovernanceTrace, error)
}
