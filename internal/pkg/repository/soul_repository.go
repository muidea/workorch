package repository

import (
	"context"

	soul "github.com/muidea/workorch/internal/modules/blocks/soul/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type SoulRepository interface {
	Create(context.Context, *soul.Soul) error
	Get(context.Context, common.SoulID) (*soul.Soul, error)
	List(context.Context) ([]*soul.Soul, error)
	Update(context.Context, *soul.Soul) error
	Delete(context.Context, common.SoulID) error
}
