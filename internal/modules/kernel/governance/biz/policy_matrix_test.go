package governance

import (
	"context"
	"testing"

	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	governancemodel "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func seedResolvedGovernanceTraces(t *testing.T, count int, itemType string, action string) *memory.AutoGovernanceTraceRepository {
	t.Helper()
	repo := memory.NewAutoGovernanceTraceRepository()
	for idx := 0; idx < count; idx++ {
		itemID := itemType + "-resolved-" + string(rune('a'+idx))
		mustNoErr(t, repo.Upsert(context.Background(), &governancemodel.AutoGovernanceTrace{
			ID:          autoGovernanceTraceID("ws-policy-matrix", itemType, itemID),
			WorkspaceID: "ws-policy-matrix",
			RunID:       "run-policy-matrix",
			ItemType:    itemType,
			ItemID:      itemID,
			Action:      action,
			Status:      autoGovernanceStatusResolved,
			OccurredAt:  "2026-08-21T00:00:0" + string(rune('0'+idx)) + "Z",
		}))
	}
	return repo
}

func TestGovernancePerRunBudgetBoundaries(t *testing.T) {
	ctx := context.Background()
	for _, used := range []int{1, 2} {
		t.Run("ticket-used-"+string(rune('0'+used)), func(t *testing.T) {
			repo := seedResolvedGovernanceTraces(t, used, "ticket", "answer")
			enabled := true
			svc := &Service{
				traces: repo,
				loadConfig: func() (workorchconfig.Config, error) {
					return workorchconfig.Config{Governance: workorchconfig.GovernanceConfig{AutoTicket: workorchconfig.GovernanceAutoTicketConfig{
						Enabled: &enabled, MaxPerRun: 2, MinConfidence: 0.97,
					}}}, nil
				},
			}
			eval := svc.evaluateTicket(ctx, &decisionmodel.Ticket{
				ID: "ticket-current", WorkspaceID: "ws-policy-matrix", RunID: "run-policy-matrix",
				DecisionKind: "provide_missing_info", State: common.DecisionTicketStateOpen,
				Context: workorcheventpayload.New(workorcheventpayload.StringField("answer", "value")),
			}, nil, nil, nil, nil)
			if used == 1 && (!eval.canAutoResolve || eval.policyBlocked != "") {
				t.Fatalf("ticket below budget = %+v, want auto resolve", eval)
			}
			if used == 2 && eval.policyBlocked != "budget_exhausted" {
				t.Fatalf("ticket at budget = %+v, want budget_exhausted", eval)
			}
		})

		t.Run("approval-used-"+string(rune('0'+used)), func(t *testing.T) {
			repo := seedResolvedGovernanceTraces(t, used, "approval", "approve")
			svc := &Service{traces: repo}
			allowReadOnly := true
			eval := svc.evaluateApproval(ctx, &approvalmodel.Approval{
				ID: "approval-current", WorkspaceID: "ws-policy-matrix", RunID: "run-policy-matrix",
				Action: "read configuration", State: common.ApprovalStatePending,
				Context: workorcheventpayload.New(workorcheventpayload.StringField("risk", "read_only")),
			}, nil, nil, workorchconfig.GovernanceConfig{AutoApproval: workorchconfig.GovernanceAutoApprovalConfig{
				MaxPerRun: 2, MinConfidence: 0.96, AllowReadOnly: &allowReadOnly,
			}})
			if used == 1 && (!eval.canAutoApprove || eval.policyBlocked != "") {
				t.Fatalf("approval below budget = %+v, want auto approve", eval)
			}
			if used == 2 && eval.policyBlocked != "budget_exhausted" {
				t.Fatalf("approval at budget = %+v, want budget_exhausted", eval)
			}
		})
	}
}

func TestGovernanceConfidenceAndHumanPermissionBoundaries(t *testing.T) {
	ctx := context.Background()
	enabled := true
	for _, threshold := range []struct {
		name        string
		ticketMin   float64
		approvalMin float64
		wantAllowed bool
	}{
		{name: "equal", ticketMin: 0.97, approvalMin: 0.96, wantAllowed: true},
		{name: "above", ticketMin: 0.970001, approvalMin: 0.960001, wantAllowed: false},
	} {
		t.Run(threshold.name, func(t *testing.T) {
			ticketService := &Service{
				traces: memory.NewAutoGovernanceTraceRepository(),
				loadConfig: func() (workorchconfig.Config, error) {
					return workorchconfig.Config{Governance: workorchconfig.GovernanceConfig{AutoTicket: workorchconfig.GovernanceAutoTicketConfig{
						Enabled: &enabled, MaxPerRun: 3, MinConfidence: threshold.ticketMin,
					}}}, nil
				},
			}
			ticketEval := ticketService.evaluateTicket(ctx, &decisionmodel.Ticket{
				ID: "ticket-confidence", WorkspaceID: "ws-confidence", RunID: "run-confidence",
				DecisionKind: "provide_missing_info", State: common.DecisionTicketStateOpen,
				Context: workorcheventpayload.New(workorcheventpayload.StringField("answer", "value")),
			}, nil, nil, nil, nil)
			if ticketEval.canAutoResolve != threshold.wantAllowed {
				t.Fatalf("ticket allowed = %t, want %t: %+v", ticketEval.canAutoResolve, threshold.wantAllowed, ticketEval)
			}

			allowReadOnly := true
			approvalService := &Service{traces: memory.NewAutoGovernanceTraceRepository()}
			approvalEval := approvalService.evaluateApproval(ctx, &approvalmodel.Approval{
				ID: "approval-confidence", WorkspaceID: "ws-confidence", RunID: "run-confidence",
				Action: "read configuration", State: common.ApprovalStatePending,
				Context: workorcheventpayload.New(workorcheventpayload.StringField("risk", "read_only")),
			}, nil, nil, workorchconfig.GovernanceConfig{AutoApproval: workorchconfig.GovernanceAutoApprovalConfig{
				MaxPerRun: 3, MinConfidence: threshold.approvalMin, AllowReadOnly: &allowReadOnly,
			}})
			if approvalEval.canAutoApprove != threshold.wantAllowed {
				t.Fatalf("approval allowed = %t, want %t: %+v", approvalEval.canAutoApprove, threshold.wantAllowed, approvalEval)
			}
		})
	}

	allowWorkspaceWrite := true
	permissionEval := (&Service{traces: memory.NewAutoGovernanceTraceRepository()}).evaluateApproval(ctx, &approvalmodel.Approval{
		ID: "approval-human", WorkspaceID: "ws-confidence", RunID: "run-confidence",
		Action: "edit secret configuration", State: common.ApprovalStatePending,
		Context: workorcheventpayload.New(workorcheventpayload.StringField("risk", "secret_access")),
	}, nil, nil, workorchconfig.GovernanceConfig{AutoApproval: workorchconfig.GovernanceAutoApprovalConfig{
		MaxPerRun: 3, AllowWorkspaceWrite: &allowWorkspaceWrite, RequireHumanFor: []string{"secret_access"},
	}})
	if permissionEval.canAutoApprove || permissionEval.policyBlocked != "require_human_policy_matched" {
		t.Fatalf("human permission evaluation = %+v, want fail-closed policy match", permissionEval)
	}
}

func TestGovernanceSharedBudgetConsecutiveAndPromptMatrices(t *testing.T) {
	ctx := context.Background()
	for _, itemType := range []string{"ticket", "approval"} {
		for _, test := range []struct {
			name       string
			statuses   []string
			currentID  string
			promptKeys []string
			budget     workorchconfig.GovernanceBudgetConfig
			wantReason string
		}{
			{name: "below consecutive threshold", statuses: []string{autoGovernanceStatusEscalated}, budget: workorchconfig.GovernanceBudgetConfig{MaxConsecutiveFailures: 2}},
			{name: "at consecutive threshold", statuses: []string{autoGovernanceStatusEscalated, autoGovernanceStatusBlocked}, budget: workorchconfig.GovernanceBudgetConfig{MaxConsecutiveFailures: 2}, wantReason: "consecutive_failures_exhausted"},
			{name: "resolved resets consecutive", statuses: []string{autoGovernanceStatusEscalated, autoGovernanceStatusBlocked, autoGovernanceStatusResolved, autoGovernanceStatusEscalated}, budget: workorchconfig.GovernanceBudgetConfig{MaxConsecutiveFailures: 2}},
			{name: "current item excluded", statuses: []string{autoGovernanceStatusEscalated, autoGovernanceStatusBlocked}, currentID: itemType + "-history-1", budget: workorchconfig.GovernanceBudgetConfig{MaxConsecutiveFailures: 2}},
			{name: "below repeated prompt threshold", statuses: []string{autoGovernanceStatusEscalated}, promptKeys: []string{"same"}, budget: workorchconfig.GovernanceBudgetConfig{MaxRepeatedSamePrompt: 2}},
			{name: "at repeated prompt threshold", statuses: []string{autoGovernanceStatusEscalated, autoGovernanceStatusEscalated}, promptKeys: []string{"same", "same"}, budget: workorchconfig.GovernanceBudgetConfig{MaxRepeatedSamePrompt: 2}, wantReason: "repeated_prompt_exhausted"},
		} {
			t.Run(itemType+"/"+test.name, func(t *testing.T) {
				items := make([]*governancemodel.AutoGovernanceTrace, 0, len(test.statuses))
				for idx, status := range test.statuses {
					prompt := ""
					if idx < len(test.promptKeys) {
						prompt = test.promptKeys[idx]
					}
					items = append(items, &governancemodel.AutoGovernanceTrace{
						ID: common.AutoGovernanceTraceID("trace-" + itemType + "-" + string(rune('a'+idx))), WorkspaceID: "ws-shared-budget", RunID: "run-shared-budget",
						ItemType: itemType, ItemID: itemType + "-history-" + string(rune('0'+idx)), Status: status,
						PromptFingerprint: prompt, OccurredAt: "2026-08-21T00:00:0" + string(rune('0'+idx)) + "Z",
					})
				}
				// Return reverse order to prove the policy, rather than the repository,
				// owns chronological consecutive-failure semantics.
				for left, right := 0, len(items)-1; left < right; left, right = left+1, right-1 {
					items[left], items[right] = items[right], items[left]
				}
				svc := &Service{traces: &staticGovernanceTraceInventory{items: items}}
				reason, blocked, err := svc.blockedBySharedBudgetPolicy(ctx, workorchconfig.GovernanceConfig{Budget: test.budget}, "ws-shared-budget", "run-shared-budget", itemType, test.currentID, "same")
				if err != nil {
					t.Fatalf("evaluate shared budget: %v", err)
				}
				if blocked != (test.wantReason != "") || reason != test.wantReason {
					t.Fatalf("blocked/reason = %t/%q, want %t/%q", blocked, reason, test.wantReason != "", test.wantReason)
				}
			})
		}
	}
}
