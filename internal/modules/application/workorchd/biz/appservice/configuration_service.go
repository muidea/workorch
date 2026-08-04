package appservice

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/configcore"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
)

type ServiceConfiguration = configcore.ServiceConfiguration
type ServerConfiguration = configcore.ServerConfiguration
type RuntimeConfiguration = configcore.RuntimeConfiguration
type RuntimeLLMOutputConfiguration = workorchconfig.LLMOutputRuntimeConfig
type LLMConfiguration = configcore.LLMConfiguration
type AIProxyConfigurationView = configcore.AIProxyConfigurationView
type LLMContextBudgetPolicyView = workorchconfig.LLMContextBudgetPolicyConfig
type CustomProviderConfigurationView = configcore.CustomProviderConfigurationView
type CustomProviderModelView = configcore.CustomProviderModelView
type ProviderCatalogSummary = configcore.ProviderCatalogSummary
type ProviderModelDescriptorView = configcore.ProviderModelDescriptorView
type WebSearchConfiguration = configcore.WebSearchConfiguration
type WebSearchProviderConfiguration = configcore.WebSearchProviderConfiguration
type WebSearchProviderTemplate = configcore.WebSearchProviderTemplate
type GovernanceConfiguration = configcore.GovernanceConfiguration
type GovernanceAutoTicketConfiguration = configcore.GovernanceAutoTicketConfiguration
type GovernanceAutoApprovalConfiguration = configcore.GovernanceAutoApprovalConfiguration
type GovernanceBudgetConfiguration = workorchconfig.GovernanceBudgetConfig
type UpdateLLMConfigurationCommand = configcore.UpdateLLMConfigurationCommand
type UpdateServerConfigurationCommand = configcore.UpdateServerConfigurationCommand
type UpdateRuntimeConfigurationCommand = configcore.UpdateRuntimeConfigurationCommand
type UpdateRuntimeLLMOutputConfiguration = configcore.UpdateRuntimeLLMOutputConfiguration
type UpdateWebSearchConfigurationCommand = configcore.UpdateWebSearchConfigurationCommand
type UpdateWebSearchProviderConfiguration = configcore.UpdateWebSearchProviderConfiguration

func ReadServiceConfiguration() (ServiceConfiguration, error) {
	return configcore.ReadServiceConfiguration()
}

func DefaultWebSearchProviderTemplates() []WebSearchProviderTemplate {
	return configcore.DefaultWebSearchProviderTemplates()
}

func UpdateServerConfiguration(cmd UpdateServerConfigurationCommand) (ServiceConfiguration, error) {
	return configcore.UpdateServerConfiguration(cmd)
}

func UpdateRuntimeConfiguration(cmd UpdateRuntimeConfigurationCommand) (ServiceConfiguration, error) {
	return configcore.UpdateRuntimeConfiguration(cmd)
}

func UpdateLLMConfiguration(cmd UpdateLLMConfigurationCommand) (ServiceConfiguration, error) {
	return configcore.UpdateLLMConfiguration(cmd)
}

func UpdateWebSearchConfiguration(cmd UpdateWebSearchConfigurationCommand) (ServiceConfiguration, error) {
	return configcore.UpdateWebSearchConfiguration(cmd)
}

type GetLLMModelCatalogCommand = configcore.GetLLMModelCatalogCommand
type RefreshLLMModelCatalogCommand = configcore.RefreshLLMModelCatalogCommand

func GetLLMModelCatalog(ctx context.Context, hub magicevent.Hub, cmd GetLLMModelCatalogCommand) (ProviderCatalogSummary, error) {
	return configcore.GetLLMModelCatalog(ctx, hub, cmd)
}

func GetLLMModelCatalogWithSession(ctx context.Context, session *RuntimeSession, cmd GetLLMModelCatalogCommand) (ProviderCatalogSummary, error) {
	if session == nil {
		return ProviderCatalogSummary{}, common.ErrInvalidArgument
	}
	hub, err := session.RequiredCommandHub()
	if err != nil {
		return ProviderCatalogSummary{}, err
	}
	return GetLLMModelCatalog(ctx, hub, cmd)
}

func RefreshLLMModelCatalog(ctx context.Context, hub magicevent.Hub, cmd RefreshLLMModelCatalogCommand) (ProviderCatalogSummary, error) {
	return configcore.RefreshLLMModelCatalog(ctx, hub, cmd)
}

func RefreshLLMModelCatalogWithSession(ctx context.Context, session *RuntimeSession, cmd RefreshLLMModelCatalogCommand) (ProviderCatalogSummary, error) {
	if session == nil {
		return ProviderCatalogSummary{}, common.ErrInvalidArgument
	}
	hub, err := session.RequiredCommandHub()
	if err != nil {
		return ProviderCatalogSummary{}, err
	}
	return RefreshLLMModelCatalog(ctx, hub, cmd)
}

func ReadServiceConfigurationWithCatalog(ctx context.Context, hub magicevent.Hub) (ServiceConfiguration, error) {
	return configcore.ReadServiceConfigurationWithCatalog(ctx, hub)
}

func ReadServiceConfigurationWithCatalogWithSession(ctx context.Context, session *RuntimeSession) (ServiceConfiguration, error) {
	if session == nil {
		return ServiceConfiguration{}, common.ErrInvalidArgument
	}
	hub, err := session.RequiredCommandHub()
	if err != nil {
		return ServiceConfiguration{}, err
	}
	return ReadServiceConfigurationWithCatalog(ctx, hub)
}
