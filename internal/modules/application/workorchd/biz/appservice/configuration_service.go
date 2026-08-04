package appservice

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/configcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

func ReadServiceConfiguration() (configcore.ServiceConfiguration, error) {
	return configcore.ReadServiceConfiguration()
}

func DefaultWebSearchProviderTemplates() []configcore.WebSearchProviderTemplate {
	return configcore.DefaultWebSearchProviderTemplates()
}

func UpdateServerConfiguration(cmd configcore.UpdateServerConfigurationCommand) (configcore.ServiceConfiguration, error) {
	return configcore.UpdateServerConfiguration(cmd)
}

func UpdateRuntimeConfiguration(cmd configcore.UpdateRuntimeConfigurationCommand) (configcore.ServiceConfiguration, error) {
	return configcore.UpdateRuntimeConfiguration(cmd)
}

func UpdateLLMConfiguration(cmd configcore.UpdateLLMConfigurationCommand) (configcore.ServiceConfiguration, error) {
	return configcore.UpdateLLMConfiguration(cmd)
}

func UpdateWebSearchConfiguration(cmd configcore.UpdateWebSearchConfigurationCommand) (configcore.ServiceConfiguration, error) {
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

func ReadServiceConfigurationWithCatalog(ctx context.Context, hub magicevent.Hub) (configcore.ServiceConfiguration, error) {
	return configcore.ReadServiceConfigurationWithCatalog(ctx, hub)
}

func ReadServiceConfigurationWithCatalogWithSession(ctx context.Context, session *runtimecore.Session) (configcore.ServiceConfiguration, error) {
	if session == nil {
		return configcore.ServiceConfiguration{}, common.ErrInvalidArgument
	}
	hub, err := session.RequiredCommandHub()
	if err != nil {
		return configcore.ServiceConfiguration{}, err
	}
	return ReadServiceConfigurationWithCatalog(ctx, hub)
}
