package repository

import (
	"context"

	governancemodel "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type AutoGovernanceTraceQuery struct {
	WorkspaceID common.WorkspaceID
	RunID       common.RunID
	StepID      common.StepID
	ItemType    string
	ItemID      string
	Status      string
	Limit       int
}

type AutoGovernanceTraceRepository interface {
	Create(context.Context, *governancemodel.AutoGovernanceTrace) error
	Upsert(context.Context, *governancemodel.AutoGovernanceTrace) error
	Get(context.Context, common.AutoGovernanceTraceID) (*governancemodel.AutoGovernanceTrace, error)
	List(context.Context, AutoGovernanceTraceQuery) ([]*governancemodel.AutoGovernanceTrace, error)
	Delete(context.Context, common.AutoGovernanceTraceID) error
}
