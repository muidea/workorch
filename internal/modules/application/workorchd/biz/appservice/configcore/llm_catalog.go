package configcore

import (
	"context"
	"fmt"
	"strings"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	modulecommon "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/common"
	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
	"github.com/muidea/workorch/internal/pkg/workorchconfigurationcontract"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type GetLLMModelCatalogCommand struct {
	ProviderID string `json:"providerId,omitempty"`
}

type RefreshLLMModelCatalogCommand struct {
	ProviderID string `json:"providerId,omitempty"`
}

func GetLLMModelCatalog(ctx context.Context, hub magicevent.Hub, cmd GetLLMModelCatalogCommand) (workorchmanagementcontract.ProviderCatalogSummary, error) {
	if hub == nil {
		return workorchmanagementcontract.ProviderCatalogSummary{}, common.ErrInvalidArgument
	}
	providerID := catalogProviderID(cmd.ProviderID)
	ev := magicevent.NewEventWithContext(
		llmevents.TopicLLMCommandGetProviderModelCatalog,
		modulecommon.ModuleID,
		llmevents.CommandDestination,
		magicevent.NewHeader(),
		ctx,
		llmevents.GetProviderModelCatalogCommand{ProviderID: providerID},
	)
	ev.BindLaneKey(events.EventHubLaneKeyFor(llmevents.Contracts(), llmevents.TopicLLMCommandGetProviderModelCatalog, eventcontract.LaneScope{
		SubjectType: "llm_catalog",
		SubjectID:   providerID,
	}))
	response, err := sendLLMCatalogCommand[llmevents.GetProviderModelCatalogResult](hub, ev)
	if err != nil {
		return workorchmanagementcontract.ProviderCatalogSummary{}, err
	}
	return catalogSummaryFromModel(response.Catalog), nil
}

func RefreshLLMModelCatalog(ctx context.Context, hub magicevent.Hub, cmd RefreshLLMModelCatalogCommand) (workorchmanagementcontract.ProviderCatalogSummary, error) {
	if hub == nil {
		return workorchmanagementcontract.ProviderCatalogSummary{}, common.ErrInvalidArgument
	}
	providerID := catalogProviderID(cmd.ProviderID)
	ev := magicevent.NewEventWithContext(
		llmevents.TopicLLMCommandRefreshProviderModelCatalog,
		modulecommon.ModuleID,
		llmevents.CommandDestination,
		magicevent.NewHeader(),
		ctx,
		llmevents.RefreshProviderModelCatalogCommand{ProviderID: providerID},
	)
	ev.BindLaneKey(events.EventHubLaneKeyFor(llmevents.Contracts(), llmevents.TopicLLMCommandRefreshProviderModelCatalog, eventcontract.LaneScope{
		SubjectType: "llm_catalog",
		SubjectID:   providerID,
	}))
	response, err := sendLLMCatalogCommand[llmevents.RefreshProviderModelCatalogResult](hub, ev)
	if err != nil {
		return workorchmanagementcontract.ProviderCatalogSummary{}, err
	}
	return catalogSummaryFromModel(response.Catalog), nil
}

func catalogProviderID(providerID string) string {
	providerID = strings.TrimSpace(providerID)
	if providerID == "" {
		return runtimecontract.BuiltinAIProxyProviderID
	}
	return providerID
}

func sendLLMCatalogCommand[Response any](hub magicevent.Hub, ev magicevent.Event) (Response, error) {
	var zero Response
	if hub == nil || ev == nil {
		return zero, common.ErrInvalidArgument
	}
	result := hub.Send(ev)
	if result == nil {
		return zero, common.ErrInvalidArgument
	}
	if resultErr := result.Error(); resultErr != nil {
		if resultErr.Cause != nil {
			return zero, resultErr.Cause
		}
		return zero, resultErr
	}
	data, resultErr := result.Get()
	if resultErr != nil {
		return zero, resultErr
	}
	response, ok := data.(Response)
	if !ok {
		return zero, fmt.Errorf("%w: topic %s expected result %T, actual %T", common.ErrInvalidArgument, ev.ID(), zero, data)
	}
	return response, nil
}

func catalogSummaryFromModel(catalog llmmodels.ProviderModelCatalog) workorchmanagementcontract.ProviderCatalogSummary {
	models := make([]workorchmanagementcontract.ProviderModelDescriptorView, 0, len(catalog.Models))
	for _, model := range catalog.Models {
		models = append(models, workorchmanagementcontract.ProviderModelDescriptorView{
			ProviderID:          model.Target.ProviderID,
			ModelID:             model.Target.ModelID,
			RouteOwner:          model.RouteOwner,
			ContextWindowTokens: model.ContextWindowTokens,
			MaxOutputTokens:     model.MaxOutputTokens,
			SupportedEndpoints:  append([]string(nil), model.SupportedEndpoints...),
			Operations:          append([]string(nil), model.Operations...),
			CapabilityReady:     model.CapabilityReady,
			UnavailableReason:   model.UnavailableReason,
			CapabilityRevision:  model.CapabilityRevision,
		})
	}
	return workorchmanagementcontract.ProviderCatalogSummary{
		ProviderID:   catalog.ProviderID,
		State:        string(catalog.State),
		Revision:     catalog.Revision,
		FetchedAt:    formatCatalogTime(catalog.FetchedAt),
		ExpiresAt:    formatCatalogTime(catalog.ExpiresAt),
		Stale:        catalog.Stale,
		Source:       catalog.Source,
		ErrorCode:    catalog.ErrorCode,
		ErrorMessage: catalog.ErrorMessage,
		Models:       models,
	}
}

func formatCatalogTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}

// ReadServiceConfigurationWithCatalog enriches the LLM read model with a live catalog when hub is available.
func ReadServiceConfigurationWithCatalog(ctx context.Context, hub magicevent.Hub) (workorchconfigurationcontract.ServiceConfiguration, error) {
	item, err := ReadServiceConfiguration()
	if err != nil {
		return workorchconfigurationcontract.ServiceConfiguration{}, err
	}
	if hub == nil {
		return item, nil
	}
	catalog, catalogErr := GetLLMModelCatalog(ctx, hub, GetLLMModelCatalogCommand{ProviderID: runtimecontract.BuiltinAIProxyProviderID})
	if catalogErr != nil {
		item.LLM.Catalog = workorchmanagementcontract.ProviderCatalogSummary{
			ProviderID:   runtimecontract.BuiltinAIProxyProviderID,
			State:        string(llmmodels.ProviderCatalogStateUnavailable),
			ErrorCode:    string(llmmodels.ProviderModelFailureCatalogUnavailable),
			ErrorMessage: catalogErr.Error(),
		}
		return item, nil
	}
	item.LLM.Catalog = catalog
	return item, nil
}

// Ensure package reference for workorchconfig in this file is not required; keep for future.
var _ = workorchconfig.BuiltinAIProxyProviderID
