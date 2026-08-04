package appservice

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/configcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchconfigurationcontract"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

func ReadServiceConfiguration() (workorchconfigurationcontract.ServiceConfiguration, error) {
	return configcore.ReadServiceConfiguration()
}

func DefaultWebSearchProviderTemplates() []workorchconfigurationcontract.WebSearchProviderTemplate {
	return configcore.DefaultWebSearchProviderTemplates()
}

func UpdateServerConfiguration(cmd configcore.UpdateServerConfigurationCommand) (workorchconfigurationcontract.ServiceConfiguration, error) {
	return configcore.UpdateServerConfiguration(cmd)
}

func UpdateRuntimeConfiguration(cmd configcore.UpdateRuntimeConfigurationCommand) (workorchconfigurationcontract.ServiceConfiguration, error) {
	return configcore.UpdateRuntimeConfiguration(cmd)
}

func UpdateLLMConfiguration(cmd configcore.UpdateLLMConfigurationCommand) (workorchconfigurationcontract.ServiceConfiguration, error) {
	return configcore.UpdateLLMConfiguration(cmd)
}

func UpdateWebSearchConfiguration(cmd configcore.UpdateWebSearchConfigurationCommand) (workorchconfigurationcontract.ServiceConfiguration, error) {
	return configcore.UpdateWebSearchConfiguration(cmd)
}

func GetLLMModelCatalog(ctx context.Context, hub magicevent.Hub, cmd configcore.GetLLMModelCatalogCommand) (workorchmanagementcontract.ProviderCatalogSummary, error) {
	return configcore.GetLLMModelCatalog(ctx, hub, cmd)
}

func GetLLMModelCatalogWithSession(ctx context.Context, session *runtimecore.Session, cmd configcore.GetLLMModelCatalogCommand) (workorchmanagementcontract.ProviderCatalogSummary, error) {
	if session == nil {
		return workorchmanagementcontract.ProviderCatalogSummary{}, common.ErrInvalidArgument
	}
	hub, err := session.RequiredCommandHub()
	if err != nil {
		return workorchmanagementcontract.ProviderCatalogSummary{}, err
	}
	return GetLLMModelCatalog(ctx, hub, cmd)
}

func RefreshLLMModelCatalog(ctx context.Context, hub magicevent.Hub, cmd configcore.RefreshLLMModelCatalogCommand) (workorchmanagementcontract.ProviderCatalogSummary, error) {
	return configcore.RefreshLLMModelCatalog(ctx, hub, cmd)
}

func RefreshLLMModelCatalogWithSession(ctx context.Context, session *runtimecore.Session, cmd configcore.RefreshLLMModelCatalogCommand) (workorchmanagementcontract.ProviderCatalogSummary, error) {
	if session == nil {
		return workorchmanagementcontract.ProviderCatalogSummary{}, common.ErrInvalidArgument
	}
	hub, err := session.RequiredCommandHub()
	if err != nil {
		return workorchmanagementcontract.ProviderCatalogSummary{}, err
	}
	return RefreshLLMModelCatalog(ctx, hub, cmd)
}

func ReadServiceConfigurationWithCatalog(ctx context.Context, hub magicevent.Hub) (workorchconfigurationcontract.ServiceConfiguration, error) {
	return configcore.ReadServiceConfigurationWithCatalog(ctx, hub)
}

func ReadServiceConfigurationWithCatalogWithSession(ctx context.Context, session *runtimecore.Session) (workorchconfigurationcontract.ServiceConfiguration, error) {
	if session == nil {
		return workorchconfigurationcontract.ServiceConfiguration{}, common.ErrInvalidArgument
	}
	hub, err := session.RequiredCommandHub()
	if err != nil {
		return workorchconfigurationcontract.ServiceConfiguration{}, err
	}
	return ReadServiceConfigurationWithCatalog(ctx, hub)
}
