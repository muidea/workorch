package contracts

import (
	"context"

	"github.com/muidea/workorch/internal/pkg/common"
)

type RuntimeOrchestrator interface {
	StartRun(context.Context, common.RunID) error
	DispatchNextStep(context.Context, common.RunID) error
	BlockOnDecision(context.Context, common.RunID, common.TicketID) error
	BlockOnApproval(context.Context, common.RunID, common.ApprovalID) error
	ResumeRun(context.Context, common.RunID) error
	CompleteRun(context.Context, common.RunID) error
	FailRun(context.Context, common.RunID, string) error
}
