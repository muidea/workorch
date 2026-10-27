package configcore

import (
	"context"
	"fmt"
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

func GetLLMModelCatalog(ctx context.Context, hub magicevent.Hub) (workorchmanagementcontract.ProviderCatalogSummary, error) {
	if hub == nil {
		return workorchmanagementcontract.ProviderCatalogSummary{}, common.ErrInvalidArgument
	}
	providerID := runtimecontract.BuiltinAIProxyProviderID
	ev := magicevent.NewEventWithContext(
		llmevents.TopicLLMCommandGetProviderModelCatalog,
		modulecommon.ModuleID,
		llmevents.CommandDestination,
		magicevent.NewHeader(),
		ctx,
		llmevents.GetProviderModelCatalogCommand{},
	)
	ev.BindLaneKey(events.EventHubLaneKeyFor(llmevents.Contracts(), llmevents.TopicLLMCommandGetProviderModelCatalog, eventcontract.LaneScope{
		SubjectType: "llm_catalog",
		SubjectID:   providerID,
	}))
	response, err := sendLLMCommand[llmevents.GetProviderModelCatalogResult](hub, ev)
	if err != nil {
		return workorchmanagementcontract.ProviderCatalogSummary{}, err
	}
	return catalogSummaryFromModel(response.Catalog), nil
}

func RefreshLLMModelCatalog(ctx context.Context, hub magicevent.Hub) (workorchmanagementcontract.ProviderCatalogSummary, error) {
	if hub == nil {
		return workorchmanagementcontract.ProviderCatalogSummary{}, common.ErrInvalidArgument
	}
	persisted, err := workorchconfig.Persisted()
	if err != nil {
		return workorchmanagementcontract.ProviderCatalogSummary{}, err
	}
	persistedRevision, err := workorchconfig.LLMConfigurationRevision(persisted.LLM)
	if err != nil {
		return workorchmanagementcontract.ProviderCatalogSummary{}, err
	}
	active, err := GetActiveLLMConfiguration(ctx, hub)
	if err != nil {
		return workorchmanagementcontract.ProviderCatalogSummary{}, err
	}
	if active.ConfigurationRevision != persistedRevision {
		return workorchmanagementcontract.ProviderCatalogSummary{}, fmt.Errorf("%w: llm configuration was saved and requires a WorkOrch restart before catalog refresh", common.ErrInvalidArgument)
	}
	providerID := runtimecontract.BuiltinAIProxyProviderID
	ev := magicevent.NewEventWithContext(
		llmevents.TopicLLMCommandRefreshProviderModelCatalog,
		modulecommon.ModuleID,
		llmevents.CommandDestination,
		magicevent.NewHeader(),
		ctx,
		llmevents.RefreshProviderModelCatalogCommand{},
	)
	ev.BindLaneKey(events.EventHubLaneKeyFor(llmevents.Contracts(), llmevents.TopicLLMCommandRefreshProviderModelCatalog, eventcontract.LaneScope{
		SubjectType: "llm_catalog",
		SubjectID:   providerID,
	}))
	response, err := sendLLMCommand[llmevents.RefreshProviderModelCatalogResult](hub, ev)
	if err != nil {
		return workorchmanagementcontract.ProviderCatalogSummary{}, err
	}
	return catalogSummaryFromModel(response.Catalog), nil
}

func GetActiveLLMConfiguration(ctx context.Context, hub magicevent.Hub) (llmevents.GetActiveConfigurationResult, error) {
	if hub == nil {
		return llmevents.GetActiveConfigurationResult{}, common.ErrInvalidArgument
	}
	providerID := runtimecontract.BuiltinAIProxyProviderID
	ev := magicevent.NewEventWithContext(
		llmevents.TopicLLMCommandGetActiveConfiguration,
		modulecommon.ModuleID,
		llmevents.CommandDestination,
		magicevent.NewHeader(),
		ctx,
		llmevents.GetActiveConfigurationCommand{},
	)
	ev.BindLaneKey(events.EventHubLaneKeyFor(llmevents.Contracts(), llmevents.TopicLLMCommandGetActiveConfiguration, eventcontract.LaneScope{
		SubjectType: "llm_configuration",
		SubjectID:   providerID,
	}))
	response, err := sendLLMCommand[llmevents.GetActiveConfigurationResult](hub, ev)
	if err != nil {
		return llmevents.GetActiveConfigurationResult{}, err
	}
	if response.ProviderID != providerID || response.ConfigurationRevision == "" || response.DefaultChat.Validate() != nil || response.DefaultChat.ProviderID != providerID {
		return llmevents.GetActiveConfigurationResult{}, fmt.Errorf("%w: invalid active LLM configuration status", common.ErrInvalidArgument)
	}
	if response.DefaultEmbedding != nil && (response.DefaultEmbedding.Validate() != nil || response.DefaultEmbedding.ProviderID != providerID) {
		return llmevents.GetActiveConfigurationResult{}, fmt.Errorf("%w: invalid active LLM embedding default", common.ErrInvalidArgument)
	}
	return response, nil
}

func sendLLMCommand[Response any](hub magicevent.Hub, ev magicevent.Event) (Response, error) {
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
		reasoning := model.Reasoning.Normalized()
		models = append(models, workorchmanagementcontract.ProviderModelDescriptorView{
			ProviderID:          model.Target.ProviderID,
			ModelID:             model.Target.ModelID,
			RouteOwner:          model.RouteOwner,
			ContextWindowTokens: model.ContextWindowTokens,
			MaxOutputTokens:     model.MaxOutputTokens,
			SupportedEndpoints:  append([]string(nil), model.SupportedEndpoints...),
			Operations:          append([]string(nil), model.Operations...),
			Reasoning: workorchmanagementcontract.ProviderReasoningCapabilityView{
				Supported:     reasoning.Supported,
				Valid:         reasoning.Valid,
				DefaultEffort: reasoning.DefaultEffort,
				Efforts:       append([]string(nil), reasoning.Efforts...),
				InvalidReason: reasoning.InvalidReason,
			},
			CapabilityReady:    model.CapabilityReady,
			EmbeddingReady:     model.EmbeddingReady,
			UnavailableReason:  model.UnavailableReason,
			CapabilityRevision: model.CapabilityRevision,
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
	persisted, err := workorchconfig.Persisted()
	if err != nil {
		return workorchconfigurationcontract.ServiceConfiguration{}, err
	}
	persistedRevision, err := workorchconfig.LLMConfigurationRevision(persisted.LLM)
	if err != nil {
		return workorchconfigurationcontract.ServiceConfiguration{}, err
	}
	active, activeErr := GetActiveLLMConfiguration(ctx, hub)
	if activeErr != nil {
		// We cannot prove the persisted configuration is active. Treat the
		// process as restart-pending so callers cannot refresh a catalog with
		// newly saved credentials against an unknown owner snapshot.
		item.LLM.RestartRequired = true
	} else {
		item.LLM.RestartRequired = active.ConfigurationRevision != persistedRevision
		item.LLM.Active = &workorchconfigurationcontract.LLMActiveConfiguration{DefaultChat: active.DefaultChat}
		if active.DefaultEmbedding != nil {
			embedding := *active.DefaultEmbedding
			item.LLM.Active.DefaultEmbedding = &embedding
		}
	}
	catalog, catalogErr := GetLLMModelCatalog(ctx, hub)
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
