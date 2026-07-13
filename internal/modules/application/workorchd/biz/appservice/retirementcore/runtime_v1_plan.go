package retirementcore

import (
	"fmt"
	"sort"
	"strings"

	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const RuntimeV1RetirementReason = "runtime_contract_v1_retired"

type RuntimeV1Inventory struct {
	Runs       []*runmodel.Run
	Workspaces []*workspacemodel.Workspace
	Tickets    []*decisionmodel.Ticket
	Approvals  []*approvalmodel.Approval
}

type RuntimeV1RunAction struct {
	WorkspaceID common.WorkspaceID
	RunID       common.RunID
	Reason      string
}

type RuntimeV1WorkspaceAction struct {
	WorkspaceID common.WorkspaceID
	ActiveRunID common.RunID
}

type RuntimeV1TicketAction struct {
	WorkspaceID common.WorkspaceID
	RunID       common.RunID
	StepID      common.StepID
	TicketID    common.TicketID
}

type RuntimeV1ApprovalAction struct {
	WorkspaceID common.WorkspaceID
	RunID       common.RunID
	StepID      common.StepID
	ApprovalID  common.ApprovalID
}

type RuntimeV1RetirementFailure struct {
	SubjectType string
	SubjectID   string
	Reason      string
}

type RuntimeV1RetirementReport struct {
	ScannedV1Runs           int
	SupersededRuns          int
	ClearedActiveRunIDs     int
	SupersededTickets       int
	SupersededApprovals     int
	ResourceScopesToRelease int
	ReleasedResourceScopes  int
	Failures                []RuntimeV1RetirementFailure
}

type RuntimeV1RetirementPlan struct {
	Runs       []RuntimeV1RunAction
	Workspaces []RuntimeV1WorkspaceAction
	Tickets    []RuntimeV1TicketAction
	Approvals  []RuntimeV1ApprovalAction
	Report     RuntimeV1RetirementReport
}

func BuildRuntimeV1RetirementPlan(inventory RuntimeV1Inventory) RuntimeV1RetirementPlan {
	plan := RuntimeV1RetirementPlan{}
	v1Runs := map[common.RunID]*runmodel.Run{}
	retiringRuns := map[common.RunID]*runmodel.Run{}
	for _, run := range inventory.Runs {
		if !isRuntimeV1Run(run) {
			continue
		}
		plan.Report.ScannedV1Runs++
		if run.ID == "" || run.WorkspaceID == "" {
			plan.Report.Failures = append(plan.Report.Failures, RuntimeV1RetirementFailure{
				SubjectType: "run",
				SubjectID:   string(run.ID),
				Reason:      "missing_run_or_workspace_id",
			})
			continue
		}
		v1Runs[run.ID] = run
		if common.RunStateIsTerminal(run.State) {
			continue
		}
		retiringRuns[run.ID] = run
		plan.Runs = append(plan.Runs, RuntimeV1RunAction{
			WorkspaceID: run.WorkspaceID,
			RunID:       run.ID,
			Reason:      RuntimeV1RetirementReason,
		})
	}

	for _, workspace := range inventory.Workspaces {
		if workspace == nil || workspace.ID == "" || workspace.ActiveRunID == "" {
			continue
		}
		if _, ok := v1Runs[workspace.ActiveRunID]; !ok {
			continue
		}
		plan.Workspaces = append(plan.Workspaces, RuntimeV1WorkspaceAction{
			WorkspaceID: workspace.ID,
			ActiveRunID: workspace.ActiveRunID,
		})
	}

	for _, ticket := range inventory.Tickets {
		if ticket == nil || ticket.State != common.DecisionTicketStateOpen {
			continue
		}
		if _, ok := v1Runs[ticket.RunID]; !ok {
			continue
		}
		plan.Tickets = append(plan.Tickets, RuntimeV1TicketAction{
			WorkspaceID: ticket.WorkspaceID,
			RunID:       ticket.RunID,
			StepID:      ticket.StepID,
			TicketID:    ticket.ID,
		})
	}

	for _, approval := range inventory.Approvals {
		if approval == nil || approval.State != common.ApprovalStatePending {
			continue
		}
		if _, ok := v1Runs[approval.RunID]; !ok {
			continue
		}
		plan.Approvals = append(plan.Approvals, RuntimeV1ApprovalAction{
			WorkspaceID: approval.WorkspaceID,
			RunID:       approval.RunID,
			StepID:      approval.StepID,
			ApprovalID:  approval.ID,
		})
	}

	sortRuntimeV1RetirementPlan(&plan)
	plan.Report.SupersededRuns = len(plan.Runs)
	plan.Report.ClearedActiveRunIDs = len(plan.Workspaces)
	plan.Report.SupersededTickets = len(plan.Tickets)
	plan.Report.SupersededApprovals = len(plan.Approvals)
	plan.Report.ResourceScopesToRelease = len(plan.Runs)
	return plan
}

func isRuntimeV1Run(run *runmodel.Run) bool {
	if run == nil {
		return false
	}
	if run.RuntimeState != nil || run.RuntimeBootstrap != nil {
		return false
	}
	return strings.TrimSpace(run.RuntimeContractVersion) != runtimecontract.RuntimeContractVersionV2
}

func sortRuntimeV1RetirementPlan(plan *RuntimeV1RetirementPlan) {
	if plan == nil {
		return
	}
	sort.Slice(plan.Runs, func(i, j int) bool {
		return fmt.Sprint(plan.Runs[i].WorkspaceID, "/", plan.Runs[i].RunID) < fmt.Sprint(plan.Runs[j].WorkspaceID, "/", plan.Runs[j].RunID)
	})
	sort.Slice(plan.Workspaces, func(i, j int) bool {
		return plan.Workspaces[i].WorkspaceID < plan.Workspaces[j].WorkspaceID
	})
	sort.Slice(plan.Tickets, func(i, j int) bool {
		return fmt.Sprint(plan.Tickets[i].WorkspaceID, "/", plan.Tickets[i].RunID, "/", plan.Tickets[i].TicketID) < fmt.Sprint(plan.Tickets[j].WorkspaceID, "/", plan.Tickets[j].RunID, "/", plan.Tickets[j].TicketID)
	})
	sort.Slice(plan.Approvals, func(i, j int) bool {
		return fmt.Sprint(plan.Approvals[i].WorkspaceID, "/", plan.Approvals[i].RunID, "/", plan.Approvals[i].ApprovalID) < fmt.Sprint(plan.Approvals[j].WorkspaceID, "/", plan.Approvals[j].RunID, "/", plan.Approvals[j].ApprovalID)
	})
	sort.Slice(plan.Report.Failures, func(i, j int) bool {
		return fmt.Sprint(plan.Report.Failures[i].SubjectType, "/", plan.Report.Failures[i].SubjectID) < fmt.Sprint(plan.Report.Failures[j].SubjectType, "/", plan.Report.Failures[j].SubjectID)
	})
}
