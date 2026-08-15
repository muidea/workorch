package biz

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
	"github.com/muidea/workorch/internal/pkg/workorchquerycontract"
)

func (p *runtimeApplicationPorts) configureGovernancePorts() {
	ports := &p.ports
	ports.Governance.ListDecisions = func(ctx context.Context, _, _, workspaceID, runID string) ([]projectioncontract.Decision, error) {
		var ret []projectioncontract.Decision
		err := p.withSession(ctx, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.ListDecisionsWithSession(ctx, session, workspaceID, runID)
			return err
		})
		return ret, err
	}
	ports.Governance.ListDecisionPage = func(ctx context.Context, _, _ string, query workorchquerycontract.RunResourcePageQuery) (appmodelcore.DecisionPage, error) {
		var ret appmodelcore.DecisionPage
		err := p.withSession(ctx, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.ListDecisionPageWithSession(ctx, session, query)
			return err
		})
		return ret, err
	}
	ports.Governance.ReadDecision = func(ctx context.Context, _, _, ticketID string) (projectioncontract.Decision, error) {
		var ret projectioncontract.Decision
		err := p.withSession(ctx, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.ReadDecisionWithSession(ctx, session, ticketID)
			return err
		})
		return ret, err
	}
	ports.Governance.ResolveDecision = func(ctx context.Context, _, _, ticketID string, cmd appmodelcore.ResolveDecisionCommand) (projectioncontract.Decision, error) {
		var ret projectioncontract.Decision
		err := p.withSession(ctx, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.ResolveDecisionWithSession(ctx, session, ticketID, cmd)
			return err
		})
		return ret, err
	}
	ports.Governance.BatchResolveDecisions = func(ctx context.Context, _, _ string, cmd appmodelcore.BatchResolveDecisionCommand) ([]projectioncontract.Decision, error) {
		var ret []projectioncontract.Decision
		err := p.withSession(ctx, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.BatchResolveDecisionsWithSession(ctx, session, cmd)
			return err
		})
		return ret, err
	}
	ports.Governance.ListApprovals = func(ctx context.Context, _, _, workspaceID, runID string) ([]projectioncontract.Approval, error) {
		var ret []projectioncontract.Approval
		err := p.withSession(ctx, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.ListApprovalsWithSession(ctx, session, workspaceID, runID)
			return err
		})
		return ret, err
	}
	ports.Governance.ListApprovalPage = func(ctx context.Context, _, _ string, query workorchquerycontract.RunResourcePageQuery) (appmodelcore.ApprovalPage, error) {
		var ret appmodelcore.ApprovalPage
		err := p.withSession(ctx, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.ListApprovalPageWithSession(ctx, session, query)
			return err
		})
		return ret, err
	}
	ports.Governance.ReadApproval = func(ctx context.Context, _, _, approvalID string) (projectioncontract.Approval, error) {
		var ret projectioncontract.Approval
		err := p.withSession(ctx, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.ReadApprovalWithSession(ctx, session, approvalID)
			return err
		})
		return ret, err
	}
	ports.Governance.ApproveApproval = func(ctx context.Context, _, _, approvalID string, cmd appmodelcore.ResolveApprovalCommand) (projectioncontract.Approval, error) {
		var ret projectioncontract.Approval
		err := p.withSession(ctx, func(session *runtimecore.Session) error {
			var err error
			if cmd.Action == "" {
				cmd.Action = "approve"
			}
			ret, err = appservice.ResolveApprovalWithSession(ctx, session, approvalID, cmd)
			return err
		})
		return ret, err
	}
	ports.Governance.RejectApproval = func(ctx context.Context, _, _, approvalID string, cmd appmodelcore.ResolveApprovalCommand) (projectioncontract.Approval, error) {
		var ret projectioncontract.Approval
		err := p.withSession(ctx, func(session *runtimecore.Session) error {
			var err error
			if cmd.Action == "" {
				cmd.Action = "reject"
			}
			ret, err = appservice.ResolveApprovalWithSession(ctx, session, approvalID, cmd)
			return err
		})
		return ret, err
	}
	ports.Governance.BatchResolveApprovals = func(ctx context.Context, _, _ string, cmd appmodelcore.BatchResolveApprovalCommand) ([]projectioncontract.Approval, error) {
		var ret []projectioncontract.Approval
		err := p.withSession(ctx, func(session *runtimecore.Session) error {
			var err error
			ret, err = appservice.BatchResolveApprovalsWithSession(ctx, session, cmd)
			return err
		})
		return ret, err
	}
}
