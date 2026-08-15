package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
	"github.com/muidea/workorch/internal/pkg/workorchquerycontract"
)

// governanceProjector is a typed, stateless read-only facade table. A fresh
// value is returned for each call so callers cannot replace a process-global
// projection function or create a second mutable projection owner.
type governanceProjector struct {
	MapApproval            func(*approvalmodel.Approval, *projectioncontract.AutoGovernanceSummary) projectioncontract.Approval
	MapDecision            func(*decisionmodel.Ticket, *decisionmodel.Resolution, *projectioncontract.AutoGovernanceSummary) projectioncontract.Decision
	PageApprovals          func([]*approvalmodel.Approval, workorchquerycontract.RunResourcePageQuery) ([]*approvalmodel.Approval, int, string, string, bool)
	PageDecisionTickets    func([]*decisionmodel.Ticket, workorchquerycontract.RunResourcePageQuery) ([]*decisionmodel.Ticket, int, string, string, bool)
	ProjectApprovalView    func(projectioncontract.Approval) projectioncontract.Approval
	ProjectDecisionView    func(projectioncontract.Decision) projectioncontract.Decision
	HasOpenDecisionBlocker func([]projectioncontract.Decision, string, string, string) bool
	HasOpenApprovalBlocker func([]projectioncontract.Approval, string, string, string) bool
	NormalizeWorkspace     func(readmodelcore.WorkspaceSnapshotState) readmodelcore.WorkspaceSnapshotState
	PromptMode             func(readmodelcore.WorkspaceSnapshotState) string
}

// GovernanceProjection returns the one typed application-facing facade for
// decision/approval/governance projections. Its functions are pure and own no
// state, command path, retry ledger, or Runtime transition.
func GovernanceProjection() governanceProjector {
	return governanceProjector{
		MapApproval:            mapApproval,
		MapDecision:            mapDecision,
		PageApprovals:          pageApprovals,
		PageDecisionTickets:    pageDecisionTickets,
		ProjectApprovalView:    projectApprovalView,
		ProjectDecisionView:    projectDecisionView,
		HasOpenDecisionBlocker: hasOpenDecisionBlocker,
		HasOpenApprovalBlocker: hasOpenApprovalBlocker,
		NormalizeWorkspace:     normalizeWorkspaceGovernanceProjection,
		PromptMode:             workspacePromptMode,
	}
}
