package orchestrator

import (
	"context"

	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func (o *RuntimeOrchestrator) ensureCoverageLedger(ctx context.Context, run *runModel.Run) bool {
	if o == nil || run == nil {
		return false
	}
	changed := o.coverageLedger().ensure(run)
	decisionRefs, approvalRefs := o.governanceEvidenceRefs(ctx, run.ID)
	if o.coverageLedger().updateFromGovernanceEvidence(run, decisionRefs, approvalRefs, "governance-owner-sync") {
		changed = true
	}
	return changed
}

func (o *RuntimeOrchestrator) governanceEvidenceRefs(ctx context.Context, runID common.RunID) ([]string, []string) {
	if o == nil || runID == "" {
		return nil, nil
	}
	decisionRefs := make([]string, 0)
	if o.tickets != nil {
		tickets, err := o.tickets.ListByRunID(ctx, runID)
		if err == nil {
			for _, ticket := range tickets {
				if ticket == nil || ticket.State != common.DecisionTicketStateResolved {
					continue
				}
				decisionRefs = append(decisionRefs, string(ticket.ID))
				if o.resolutions != nil {
					resolution, resolutionErr := o.resolutions.GetByTicketID(ctx, ticket.ID)
					if resolutionErr == nil && resolution != nil && resolution.ID != "" {
						decisionRefs = append(decisionRefs, string(resolution.ID))
					}
				}
			}
		}
	}
	approvalRefs := make([]string, 0)
	if o.approvals != nil {
		approvals, err := o.approvals.ListByRunID(ctx, runID)
		if err == nil {
			for _, approval := range approvals {
				if approval == nil || approval.State != common.ApprovalStateApproved {
					continue
				}
				approvalRefs = append(approvalRefs, string(approval.ID))
			}
		}
	}
	return compactUniqueStrings(decisionRefs), compactUniqueStrings(approvalRefs)
}
