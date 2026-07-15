package appservice

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/configcore"
)

type ServiceConfiguration = configcore.ServiceConfiguration
type ServerConfiguration = configcore.ServerConfiguration
type RepositoryConfiguration = configcore.RepositoryConfiguration
type RuntimeConfiguration = configcore.RuntimeConfiguration
type RuntimeLLMOutputConfiguration = configcore.RuntimeLLMOutputConfiguration
type LLMConfiguration = configcore.LLMConfiguration
type AIProxyConfigurationView = configcore.AIProxyConfigurationView
type LLMContextBudgetPolicyView = configcore.LLMContextBudgetPolicyView
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
type GovernanceBudgetConfiguration = configcore.GovernanceBudgetConfiguration
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

func RefreshLLMModelCatalog(ctx context.Context, hub magicevent.Hub, cmd RefreshLLMModelCatalogCommand) (ProviderCatalogSummary, error) {
	return configcore.RefreshLLMModelCatalog(ctx, hub, cmd)
}

func ReadServiceConfigurationWithCatalog(ctx context.Context, hub magicevent.Hub) (ServiceConfiguration, error) {
	return configcore.ReadServiceConfigurationWithCatalog(ctx, hub)
}
