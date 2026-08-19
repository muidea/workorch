// Package contracts defines Approval-owner-local use-case inputs. These are
// not EventHub commands; cross-owner callers must use approval/pkg/events.
package contracts

import (
	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

type CreateApprovalCommand struct {
	ApprovalID        common.ApprovalID
	WorkspaceID       common.WorkspaceID
	RunID             common.RunID
	StepID            common.StepID
	Reason            string
	Action            string
	Context           workorcheventpayload.Payload
	AutoGovernance    workorcheventpayload.Payload
	RuntimeGovernance *approvalmodel.RuntimeGovernanceBinding
}

type ResolveApprovalCommand struct {
	ResolutionID common.ResolutionID
	ApprovalID   common.ApprovalID
	WorkspaceID  common.WorkspaceID
	RunID        common.RunID
	Principal    approvalmodel.ApprovalResolutionPrincipal
	Context      workorcheventpayload.Payload
}
