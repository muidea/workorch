package workorchruntime

import (
	"context"
	"testing"

	eventstreammodel "github.com/muidea/workorch/internal/modules/kernel/eventstream/pkg/models"
	"github.com/muidea/workorch/internal/pkg/repository"
)

type fakeEventStreamService struct{}

func (fakeEventStreamService) ListEvents(context.Context, repository.EventRecordQuery) ([]*eventstreammodel.EventRecord, error) {
	return nil, nil
}

func TestRuntimeTeardownClearsEventStreamService(t *testing.T) {
	runtime := New()
	runtime.SetEventStreamService(fakeEventStreamService{})
	if runtime.EventStreamService() == nil {
		t.Fatal("test setup failed: event stream service was not set")
	}

	runtime.Teardown(context.Background())

	if runtime.EventStreamService() != nil {
		t.Fatal("Teardown did not clear event stream service")
	}
}
