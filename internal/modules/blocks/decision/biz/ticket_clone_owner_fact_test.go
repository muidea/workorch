package decision

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
)

// 实际 Decision owner 仍反证原持久票据；仓库读取副本不能改变 query/replay 所用来源。
func TestWorkspaceDefinitionClarificationReadCopiesCannotRewriteOwnerFact(t *testing.T) {
	ctx := context.Background()
	hub := newRecordingDecisionHub(t)
	repositories := memory.NewDecisionOwnerRepositorySet()
	owner, err := New(hub, nil, Dependencies{OwnerRepositories: repositories.OwnerRepositories(), EventHub: hub})
	if err != nil {
		t.Fatal(err)
	}
	open := workspaceDefinitionClarificationCommandForBizTest().Normalized()
	bound := common.ContextWithRuntimeWorkspaceID(ctx, open.WorkspaceID)
	if _, err := owner.openWorkspaceDefinitionClarification(bound, open); err != nil {
		t.Fatal(err)
	}
	query := decisionevents.QueryWorkspaceDefinitionClarificationFactCommand{WorkspaceID: open.WorkspaceID, TicketID: open.TicketID}
	original, err := owner.queryWorkspaceDefinitionClarificationFact(bound, query)
	if err != nil || !original.Found || original.ValidateFor(query) != nil {
		t.Fatalf("原 owner fact: %+v err=%v", original, err)
	}
	body, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var expected decisionevents.QueryWorkspaceDefinitionClarificationFactResult
	if err := json.Unmarshal(body, &expected); err != nil {
		t.Fatal(err)
	}
	mutate := func(item *decisionmodel.Ticket) {
		item.AllowedRecoveryActions[0] = "caller-action"
		item.DefinitionClarification.CandidateHash = "caller-candidate"
		item.DefinitionClarification.OpenBindingHash = "caller-binding"
		item.DefinitionClarification.OffendingRefs[0] = "caller-ref"
	}
	assertOriginal := func(t *testing.T) {
		t.Helper()
		current, err := owner.queryWorkspaceDefinitionClarificationFact(bound, query)
		if err != nil || current.ValidateFor(query) != nil || !reflect.DeepEqual(current, expected) {
			t.Fatalf("副本改写原 owner fact: %+v err=%v", current, err)
		}
		replayed, err := owner.openWorkspaceDefinitionClarification(bound, open)
		if err != nil || replayed.ValidateFor(open) != nil || replayed.Disposition != decisionevents.DefinitionClarificationDuplicate {
			t.Fatalf("原 ticket exact replay 受副本污染: %+v err=%v", replayed, err)
		}
		if _, err := repositories.Resolutions.GetByTicketID(bound, open.TicketID); err != common.ErrNotFound {
			t.Fatalf("只读副本产生 Resolution: %v", err)
		}
		if commands := hub.EvidenceCommands(); len(commands) != 0 {
			t.Fatal("只读副本产生 Runtime evidence")
		}
	}
	for _, read := range []struct {
		name string
		load func() ([]*decisionmodel.Ticket, error)
	}{
		{"get", func() ([]*decisionmodel.Ticket, error) {
			item, err := repositories.Tickets.Get(bound, open.TicketID)
			return []*decisionmodel.Ticket{item}, err
		}},
		{"all", func() ([]*decisionmodel.Ticket, error) { return repositories.Tickets.List(bound) }},
		{"workspace", func() ([]*decisionmodel.Ticket, error) {
			return repositories.Tickets.ListByWorkspaceID(bound, open.WorkspaceID)
		}},
	} {
		t.Run(read.name, func(t *testing.T) {
			items, err := read.load()
			if err != nil || len(items) != 1 {
				t.Fatalf("actual repository read: %+v err=%v", items, err)
			}
			mutate(items[0])
			assertOriginal(t)
		})
	}
}
