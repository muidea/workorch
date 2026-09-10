package decision

import (
	"context"
	"strings"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/common"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestInputReviewRecoveryCannotUseTicketQuestionAsMissingAnswer(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(32)
	t.Cleanup(func() { hub.Terminate(ctx) })
	repositories := memory.NewDecisionOwnerRepositorySet()
	owner, err := New(hub, nil, Dependencies{OwnerRepositories: repositories.OwnerRepositories(), EventHub: hub})
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { owner.Teardown(ctx) })
	var commits int
	observer := magicevent.NewSimpleObserver(orchestratorevents.CommandDestination, hub)
	observer.Subscribe(orchestratorevents.TopicOrchestratorCommitRuntimeGovernanceTicket, func(_ magicevent.Event, _ magicevent.Result) { commits++ })
	t.Cleanup(func() { observer.Unsubscribe(orchestratorevents.TopicOrchestratorCommitRuntimeGovernanceTicket) })
	ticket := &decisionmodel.Ticket{ID: "ticket-review", WorkspaceID: "workspace-review", RunID: "run-review", DecisionKind: "runtime_governance", FailureClass: runtimecontract.RuntimeValidationInputReviewFailureClass, Reason: "Unanswered question is not supplied information"}
	for _, fields := range []map[string]any{nil, {"summary": ""}, {"summary": " "}, {"summary": strings.Repeat("x", runtimecontract.RuntimeValidationInputReviewMaxReasonBytes+1)}} {
		proposal := &decisionmodel.RuntimeGovernanceResolutionProposal{ResolutionType: modulecommon.ResolutionTypeProvideMissingInfo, Payload: mustDecisionPayload(t, fields)}
		if _, err := owner.commitRuntimeGovernanceTicketProposal(ctx, ticket, proposal); err == nil {
			t.Fatal("pending review recovered without real information")
		}
	}
	if commits != 0 {
		t.Fatal("invalid pending proposal reached Runtime continuation")
	}
	if validRuntimeInputReviewInformationSummary("\xff", true) || validRuntimeInputReviewInformationSummary("facts", false) || !validRuntimeInputReviewInformationSummary("Facts supplied for re-review.", true) {
		t.Fatal("information boundary has incorrect UTF-8/presence semantics")
	}
}
