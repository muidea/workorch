package biz

import (
	"context"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	workorchevents "github.com/muidea/workorch/internal/pkg/events"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// ObservePreparationProviderCatalog is an explicit preparation observation,
// not the persisted-state GET and not an inference-selection/ceiling fallback.
func (b *Biz) ObservePreparationProviderCatalog(ctx context.Context, workspaceID common.WorkspaceID) (llmevents.ObserveProviderCatalogCacheResult, error) {
	return b.observePreparationProviderCatalogFor(ctx, workspaceID, nil)
}

func (b *Biz) observePreparationProviderCatalogFor(ctx context.Context, workspaceID common.WorkspaceID, target *runtimecontract.ProviderModelTarget) (llmevents.ObserveProviderCatalogCacheResult, error) {
	command := llmevents.ObserveProviderCatalogCacheCommand{WorkspaceID: workspaceID, Target: target}
	if b == nil || b.EventHub() == nil || ctx == nil || command.Validate() != nil {
		return llmevents.ObserveProviderCatalogCacheResult{}, common.ErrInvalidArgument
	}
	if err := ctx.Err(); err != nil {
		return llmevents.ObserveProviderCatalogCacheResult{}, err
	}
	bound, err := common.BindRuntimeWorkspaceContext(ctx, workspaceID)
	if err != nil {
		return llmevents.ObserveProviderCatalogCacheResult{}, err
	}
	bounded, cancel := context.WithTimeout(bound, 2*time.Second)
	defer cancel()
	ev := magicevent.NewEventWithContext(llmevents.TopicObserveProviderCatalogCache, ModuleID, llmevents.CommandDestination, magicevent.NewHeader(), bounded, command)
	ev.BindLaneKey(workorchevents.EventHubLaneKeyFor(llmevents.Contracts(), llmevents.TopicObserveProviderCatalogCache, eventcontract.LaneScope{WorkspaceID: string(workspaceID)}))
	result := b.SendEvent(ev)
	if result == nil || result.Error() != nil {
		return llmevents.ObserveProviderCatalogCacheResult{}, common.ErrConflict
	}
	value, resultErr := result.Get()
	response, ok := value.(llmevents.ObserveProviderCatalogCacheResult)
	if resultErr != nil || !ok || response.ValidateFor(command) != nil {
		return llmevents.ObserveProviderCatalogCacheResult{}, common.ErrConflict
	}
	return response.Clone(), nil
}
