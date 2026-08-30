package ownerreadports

import (
	"context"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	applicationcommon "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/common"
	eventstreamevents "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/events"
	"github.com/muidea/workorch/internal/pkg/workorchquerycontract"
)

type eventstreamSourceCaptureHub struct {
	magicevent.Hub
	source      string
	destination string
}

func (h *eventstreamSourceCaptureHub) Send(event magicevent.Event) magicevent.Result {
	h.source = event.Source()
	h.destination = event.Destination()
	result := magicevent.NewResult(event.ID(), event.Source(), event.Destination())
	result.Set(eventstreamevents.ListEventsResult{}, nil)
	return result
}

func TestListEventRecordsUsesCanonicalApplicationOwnerSource(t *testing.T) {
	base := magicevent.NewHub(4)
	t.Cleanup(func() { base.Terminate(context.Background()) })
	hub := &eventstreamSourceCaptureHub{Hub: base}
	if _, err := ListEventRecordsByOwnerCommand(context.Background(), hub, workorchquerycontract.EventRecordQuery{}); err != nil {
		t.Fatal(err)
	}
	if hub.source != applicationcommon.ModuleID || hub.destination != eventstreamevents.CommandDestination {
		t.Fatalf("Eventstream query source/destination = %q/%q", hub.source, hub.destination)
	}
}
