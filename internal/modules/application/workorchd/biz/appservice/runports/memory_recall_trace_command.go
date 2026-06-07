package runports

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownercommandports"
	memorymodel "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/models"
	"github.com/muidea/workorch/internal/pkg/events"
)

func RecordMemoryRecallTrace(ctx context.Context, hub events.Hub, trace *memorymodel.MemoryRecallTrace) error {
	return ownercommandports.RecordMemoryRecallTrace(ctx, hub, trace)
}
