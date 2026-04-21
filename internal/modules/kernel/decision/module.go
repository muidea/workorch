package decision

import (
	decisionbiz "github.com/muidea/workorch/internal/modules/kernel/decision/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/common"
	decisionservice "github.com/muidea/workorch/internal/modules/kernel/decision/service"
	"github.com/muidea/workorch/internal/pkg/repository"
)

const ModuleID = modulecommon.ModuleID

type Service = decisionbiz.Service
type CreateTicketCommand = decisionservice.CreateTicketCommand
type ResolveTicketCommand = decisionservice.ResolveTicketCommand

func NewService(
	tickets repository.DecisionTicketRepository,
	resolutions repository.DecisionResolutionRepository,
) *decisionbiz.Service {
	return decisionbiz.NewService(tickets, resolutions)
}
