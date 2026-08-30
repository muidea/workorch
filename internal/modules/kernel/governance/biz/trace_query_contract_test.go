package governance

import (
	"context"
	"errors"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	governanceevents "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/events"
	governancemodel "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	"github.com/muidea/workorch/internal/pkg/workorchquerycontract"
)

type countingGovernanceTraceQueryRepository struct {
	repository.AutoGovernanceTraceRepository
	lists int
	query workorchquerycontract.AutoGovernanceTraceQuery
}

func (r *countingGovernanceTraceQueryRepository) List(_ context.Context, query workorchquerycontract.AutoGovernanceTraceQuery) ([]*governancemodel.AutoGovernanceTrace, error) {
	r.lists++
	r.query = query
	return nil, nil
}

func TestGovernanceTraceQueryAdmissionFailsBeforeOwnerRepositoryIO(t *testing.T) {
	valid := governanceevents.ListAutoGovernanceTracesCommand{Query: workorchquerycontract.AutoGovernanceTraceQuery{WorkspaceID: "workspace-1", Limit: 10}}
	for name, fixture := range map[string]struct {
		event magicevent.Event
	}{
		"nil event":           {},
		"empty source":        {event: governanceTraceQueryEvent("", valid)},
		"whitespace source":   {event: governanceTraceQueryEvent(" ", valid)},
		"noncanonical source": {event: governanceTraceQueryEvent(" application/workorchd/appservice ", valid)},
		"wrong source":        {event: governanceTraceQueryEvent("kernel/orchestrator", valid)},
		"wrong topic":         {event: magicevent.NewEventWithContext("/wrong", "application/workorchd/appservice", governanceevents.CommandDestination, magicevent.NewHeader(), context.Background(), valid)},
		"wrong payload":       {event: governanceTraceQueryEvent("application/workorchd/appservice", "wrong")},
		"empty workspace":     {event: governanceTraceQueryEvent("application/workorchd/appservice", governanceevents.ListAutoGovernanceTracesCommand{Query: workorchquerycontract.AutoGovernanceTraceQuery{Limit: 10}})},
		"zero limit":          {event: governanceTraceQueryEvent("application/workorchd/appservice", governanceevents.ListAutoGovernanceTracesCommand{Query: workorchquerycontract.AutoGovernanceTraceQuery{WorkspaceID: "workspace-1"}})},
		"oversized limit":     {event: governanceTraceQueryEvent("application/workorchd/appservice", governanceevents.ListAutoGovernanceTracesCommand{Query: workorchquerycontract.AutoGovernanceTraceQuery{WorkspaceID: "workspace-1", Limit: governanceevents.MaxAutoGovernanceTraceQueryLimit + 1}})},
	} {
		t.Run(name, func(t *testing.T) {
			repository := &countingGovernanceTraceQueryRepository{}
			service := &Service{traces: repository}
			if _, err := service.executeCommand(fixture.event); !errors.Is(err, common.ErrInvalidArgument) {
				t.Fatalf("error = %v, want invalid argument", err)
			}
			if repository.lists != 0 {
				t.Fatalf("repository List calls = %d, want zero", repository.lists)
			}
		})
	}
}

func TestGovernanceTraceQueryExactApplicationSourceReachesBoundedOwnerRead(t *testing.T) {
	repository := &countingGovernanceTraceQueryRepository{}
	service := &Service{traces: repository}
	command := governanceevents.ListAutoGovernanceTracesCommand{Query: workorchquerycontract.AutoGovernanceTraceQuery{
		WorkspaceID: "workspace-1", RunID: "run-1", ItemType: "decision", Limit: 10,
	}}
	if _, err := service.executeCommand(governanceTraceQueryEvent("application/workorchd/appservice", command)); err != nil {
		t.Fatal(err)
	}
	if repository.lists != 1 || repository.query != command.Query {
		t.Fatalf("repository calls=%d query=%+v, want one exact bounded query", repository.lists, repository.query)
	}
}

func governanceTraceQueryEvent(source string, data any) magicevent.Event {
	return magicevent.NewEventWithContext(
		governanceevents.TopicGovernanceAutoCommandListTraces,
		source,
		governanceevents.CommandDestination,
		magicevent.NewHeader(),
		context.Background(),
		data,
	)
}
