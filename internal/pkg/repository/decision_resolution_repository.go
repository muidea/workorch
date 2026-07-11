package repository

import (
	"context"

	decision "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type DecisionResolutionRepository interface {
	Create(context.Context, *decision.Resolution) error
	GetByTicketID(context.Context, common.TicketID) (*decision.Resolution, error)
	DeleteByTicketID(context.Context, common.TicketID) error
}
