package decision

import (
	"context"
	"errors"
	"reflect"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	decisioncontracts "github.com/muidea/workorch/internal/modules/blocks/decision/biz/contracts"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	"github.com/muidea/workorch/internal/pkg/workorchrepo"
)

func contractChangeCommandForTest() decisionevents.RegisterContractChangeProposalCommand {
	return decisionevents.RegisterContractChangeProposalCommand{TicketID: "change-ticket", RequestID: "register-1", Proposal: decisionmodel.ContractChangeProposal{
		SchemaVersion: 1, WorkspaceID: "change-ws", BaseDefinitionRevision: "definition-1", BaseStartupInputHash: "startup-1",
		ProposedBy: "test-user", TriggerFacts: "观测原文\r\n\x00", Discovery: "发现过程", RootCauseOrUnknown: "根因未知",
		WhyChange: "完整原因\r\n 保留原文\x00", EvidenceGaps: "证据尚缺，禁止应用",
		Differences:  []decisionmodel.ContractChangeDifference{{ItemID: "criterion-1", Kind: "criterion", Action: "modify", OriginalText: "原判据\r\n", ProposedText: "拟议判据\r\n", SemanticChange: "改变验收；尚未批准"}},
		Alternatives: "保留原判据并继续排查", NoChangeConsequences: "局部阻塞", UnaffectedContinuationConditions: "仍依原授权",
		Impacts:            []decisionmodel.ContractChangeImpact{{SubjectID: "criterion-1", Kind: "criterion", ProposedHandling: "unknown", Reason: "缺实际证据"}},
		EffectsAndReceipts: "unknown效果未结", OutstandingCosts: "原成本未结", ReservedBudget: "原预算保留",
		AdditionalCostDurationRisk: "新增成本未知", RequiredAuthorization: "未获新增授权", DecisionScope: "原合同持有人",
		ApplicationConditions: "最终候选及精确approve未完成", RecoveryRestrictions: "不得抹去unknown效果",
	}}
}

func TestContractChangeRegistrationMountedOwnerKeepsHistoryWithoutApproval(t *testing.T) {
	for _, backend := range []string{"memory", "workspace-file"} {
		t.Run(backend, func(t *testing.T) {
			ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), "change-ws")
			root := t.TempDir()
			var tickets repository.DecisionTicketRepository
			var resolutions repository.DecisionResolutionRepository
			var owners repository.DecisionOwnerRepositories
			if backend == "memory" {
				repos := memory.NewDecisionOwnerRepositorySet()
				owners, tickets, resolutions = repos.OwnerRepositories(), repos.Tickets, repos.Resolutions
			} else {
				repos := workorchrepo.NewWorkspaceScopedFileRepositorySet(root)
				owners, tickets, resolutions = repos.DecisionOwner, repos.Tickets, repos.Resolutions
			}
			hub := magicevent.NewHub(64)
			t.Cleanup(func() { hub.Terminate(context.Background()) })
			owner, err := New(hub, nil, Dependencies{OwnerRepositories: owners, EventHub: hub})
			if err != nil {
				t.Fatal(err)
			}
			if err := owner.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := owner.Teardown(context.Background()); err != nil {
					t.Error(err)
				}
			})
			cmd := contractChangeCommandForTest()
			if _, err := sendDecisionOwnerContractCommand(ctx, hub, decisionevents.TopicRegisterContractChangeProposal, "kernel/governance", cmd); !errors.Is(err, common.ErrInvalidArgument) {
				t.Fatalf("伪来源: %v", err)
			}
			if _, err := tickets.Get(ctx, cmd.TicketID); !errors.Is(err, common.ErrNotFound) {
				t.Fatalf("伪来源写入: %v", err)
			}
			register := func(c decisionevents.RegisterContractChangeProposalCommand, want string) decisionevents.RegisterContractChangeProposalResult {
				t.Helper()
				data, err := sendDecisionOwnerContractCommand(ctx, hub, decisionevents.TopicRegisterContractChangeProposal, workspaceDefinitionClarificationSource, c)
				result, ok := data.(decisionevents.RegisterContractChangeProposalResult)
				if err != nil || !ok || result.ValidateFor(c) != nil || result.Disposition != want {
					t.Fatalf("登记=%#v err=%v", data, err)
				}
				return result
			}
			first := register(cmd, "registered")
			register(cmd, "duplicate")
			stored, err := tickets.Get(ctx, cmd.TicketID)
			if err != nil || stored.ValidateContractChange() != nil || stored.RunID != "" || !reflect.DeepEqual(stored.ContractChangeRevisions[0].Proposal, cmd.Proposal) {
				t.Fatalf("原持久材料=%+v err=%v", stored, err)
			}
			stored.ContractChangeRevisions[0].Proposal.Differences[0].OriginalText = "污染副本"
			register(cmd, "duplicate")
			next := cmd
			next.RequestID = "register-2"
			next.ExpectedRevision = first.Revision
			next.ExpectedHash = first.Hash
			next.Proposal = cmd.Proposal.Clone()
			next.Proposal.WhyChange += "\r\n补充原因"
			selfDeclared := next
			selfDeclared.Proposal = next.Proposal.Clone()
			selfDeclared.Proposal.FinalCandidate = &decisionmodel.ContractChangeCandidate{ID: "final-candidate", Revision: 2, Hash: "final-hash"}
			if _, err := sendDecisionOwnerContractCommand(ctx, hub, decisionevents.TopicRegisterContractChangeProposal, workspaceDefinitionClarificationSource, selfDeclared); err == nil {
				t.Fatal("自报最终候选未经 Workspace 原事实登记")
			}
			register(next, "registered")
			register(cmd, "duplicate")
			conflict := next
			conflict.RequestID = "stale-request"
			if _, err := sendDecisionOwnerContractCommand(ctx, hub, decisionevents.TopicRegisterContractChangeProposal, workspaceDefinitionClarificationSource, conflict); !errors.Is(err, common.ErrConflict) {
				t.Fatalf("旧版本覆盖=%v", err)
			}
			conflict = cmd
			conflict.Proposal = cmd.Proposal.Clone()
			conflict.Proposal.WhyChange = "同key异内容"
			if _, err := sendDecisionOwnerContractCommand(ctx, hub, decisionevents.TopicRegisterContractChangeProposal, workspaceDefinitionClarificationSource, conflict); !errors.Is(err, common.ErrConflict) {
				t.Fatalf("同key漂移=%v", err)
			}
			stored, err = tickets.Get(ctx, cmd.TicketID)
			if err != nil || len(stored.ContractChangeRevisions) != 2 || stored.ValidateContractChange() != nil {
				t.Fatalf("版本历史=%+v err=%v", stored, err)
			}
			if _, _, _, err := owner.service.EnsureResolvedTicketAndReturnWithTransition(ctx, decisioncontracts.ResolveTicketCommand{ResolutionID: "forged-resolution", TicketID: cmd.TicketID, WorkspaceID: cmd.Proposal.WorkspaceID, Type: "replan"}); !errors.Is(err, common.ErrConflict) {
				t.Fatalf("普通决议=%v", err)
			}
			if err := owner.reconcileDecisionTicket(ctx, stored); err != nil {
				t.Fatal(err)
			}
			if _, err := resolutions.GetByTicketID(ctx, cmd.TicketID); !errors.Is(err, common.ErrNotFound) {
				t.Fatalf("登记产生批准=%v", err)
			}
			if backend == "workspace-file" {
				reopened := workorchrepo.NewWorkspaceScopedFileRepositorySet(root)
				actual, err := reopened.Tickets.Get(ctx, cmd.TicketID)
				if err != nil || !reflect.DeepEqual(actual, stored) {
					t.Fatalf("file reopen原文漂移=%v", err)
				}
				runs, err := reopened.Runs.List(ctx)
				if err != nil || len(runs) != 0 {
					t.Fatalf("无Run登记伪造Run=%d %v", len(runs), err)
				}
			}
		})
	}
}

func TestContractChangePrivateReducerRejectsOrdinaryTransitions(t *testing.T) {
	cmd := contractChangeCommandForTest()
	seed := &decisionmodel.Ticket{ID: cmd.TicketID, WorkspaceID: cmd.Proposal.WorkspaceID, Reason: cmd.Proposal.WhyChange, DecisionKind: decisionmodel.ContractChangeDecisionKind,
		ContractChangeRevisions: []decisionmodel.ContractChangeRevision{{RequestID: cmd.RequestID, Revision: 1, Hash: cmd.Proposal.Hash(), Proposal: cmd.Proposal}}}
	current, err := reduceDecisionLifecycle(nil, lifecycleInternalEvent{Kind: lifecycleInternalEventOpened, TicketID: seed.ID, Ticket: seed})
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []lifecycleInternalEventKind{lifecycleInternalEventResolutionObserved, lifecycleInternalEventResolutionRecovered, lifecycleInternalEventProposalPrepared, lifecycleInternalEventCreatedNotificationPublished} {
		if _, err := reduceDecisionLifecycle(current, lifecycleInternalEvent{Kind: kind, TicketID: seed.ID, ResolvedAt: "now"}); !errors.Is(err, common.ErrConflict) {
			t.Fatalf("非法transition %s=%v", kind, err)
		}
	}
}
