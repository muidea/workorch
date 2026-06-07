package readmodelports

import (
	"context"
	"errors"
	"testing"

	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type fakeDecisionResolutionReader struct {
	items map[common.TicketID]*decisionmodel.Resolution
	calls []common.TicketID
	err   error
}

func (f *fakeDecisionResolutionReader) GetByTicketID(_ context.Context, ticketID common.TicketID) (*decisionmodel.Resolution, error) {
	f.calls = append(f.calls, ticketID)
	if f.err != nil {
		return nil, f.err
	}
	item := f.items[ticketID]
	if item == nil {
		return nil, common.ErrNotFound
	}
	return item, nil
}

func TestResolutionsForTicketsReadsOnlyResolvedTickets(t *testing.T) {
	reader := &fakeDecisionResolutionReader{
		items: map[common.TicketID]*decisionmodel.Resolution{
			"ticket-1": {ID: "resolution-1", TicketID: "ticket-1"},
		},
	}
	items := []*decisionmodel.Ticket{
		{ID: "ticket-1", State: common.DecisionTicketStateResolved},
		{ID: "ticket-2", State: common.DecisionTicketStateOpen},
		nil,
	}

	got, err := ResolutionsForTickets(context.Background(), reader, items)
	if err != nil {
		t.Fatalf("ResolutionsForTickets: %v", err)
	}
	if len(reader.calls) != 1 || reader.calls[0] != "ticket-1" {
		t.Fatalf("calls = %+v, want [ticket-1]", reader.calls)
	}
	if got["ticket-1"] == nil || got["ticket-1"].ID != "resolution-1" {
		t.Fatalf("resolution map = %+v", got)
	}
}

func TestResolutionsForTicketsIgnoresMissingResolution(t *testing.T) {
	reader := &fakeDecisionResolutionReader{}

	got, err := ResolutionsForTickets(context.Background(), reader, []*decisionmodel.Ticket{
		{ID: "ticket-missing", State: common.DecisionTicketStateResolved},
	})
	if err != nil {
		t.Fatalf("ResolutionsForTickets: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got = %+v, want empty map", got)
	}
}

func TestResolutionsForTicketsReturnsReaderError(t *testing.T) {
	wantErr := errors.New("reader failed")
	reader := &fakeDecisionResolutionReader{err: wantErr}

	_, err := ResolutionsForTickets(context.Background(), reader, []*decisionmodel.Ticket{
		{ID: "ticket-1", State: common.DecisionTicketStateResolved},
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want %v", err, wantErr)
	}
}

func TestResolutionsForTicketsAllowsNilReader(t *testing.T) {
	got, err := ResolutionsForTickets(context.Background(), nil, []*decisionmodel.Ticket{
		{ID: "ticket-1", State: common.DecisionTicketStateResolved},
	})
	if err != nil {
		t.Fatalf("ResolutionsForTickets: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got = %+v, want empty map", got)
	}
}
