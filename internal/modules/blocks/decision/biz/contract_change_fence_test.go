package decision

import (
	"context"
	"errors"
	"reflect"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	deletioncontract "github.com/muidea/workorch/internal/pkg/workorchdeletioncontract"
)

func TestContractChangeDeletionFenceAndOrphanResolutionRejectWithoutMutation(t *testing.T) {
	for _, cause := range []string{"deletion", "orphan-resolution"} {
		t.Run(cause, func(t *testing.T) {
			cmd := contractChangeCommandForTest()
			ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), cmd.Proposal.WorkspaceID)
			repos := memory.NewDecisionOwnerRepositorySet()
			hub := magicevent.NewHub(32)
			t.Cleanup(func() { hub.Terminate(context.Background()) })
			owner, err := New(hub, nil, Dependencies{OwnerRepositories: repos.OwnerRepositories(), EventHub: hub})
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
			if _, err := sendDecisionOwnerContractCommand(ctx, hub, decisionevents.TopicRegisterContractChangeProposal, workspaceDefinitionClarificationSource, cmd); err != nil {
				t.Fatal(err)
			}
			before, err := repos.Tickets.Get(ctx, cmd.TicketID)
			if err != nil {
				t.Fatal(err)
			}
			want := common.ErrConflict
			if cause == "deletion" {
				scope := deletioncontract.Scope{WorkspaceID: cmd.Proposal.WorkspaceID, DeletionID: "delete-change", DeletionGeneration: 1, Mode: deletioncontract.ModePurge, QuiescenceID: "quiescence-change"}
				scope.RequestHash = deletioncontract.StableRequestHash(scope)
				if _, err := owner.applyWorkspaceDeletionInternalEvent(ctx, nil, workspaceDeletionInternalEvent{Kind: workspaceDeletionInternalEventFenceObserved, Scope: scope}); err != nil {
					t.Fatal(err)
				}
				want = common.ErrNotFound
			} else {
				if err := repos.Resolutions.Create(ctx, &decisionmodel.Resolution{ID: "orphan", TicketID: cmd.TicketID, Type: "replan"}); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := sendDecisionOwnerContractCommand(ctx, hub, decisionevents.TopicRegisterContractChangeProposal, workspaceDefinitionClarificationSource, cmd); !errors.Is(err, want) {
				t.Fatalf("%s=%v", cause, err)
			}
			if cause != "deletion" {
				after, err := repos.Tickets.Get(ctx, cmd.TicketID)
				if err != nil || !reflect.DeepEqual(before, after) {
					t.Fatalf("拒绝仍改票据=%v", err)
				}
			}
		})
	}
}
