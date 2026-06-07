package appservice

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/configcore"

type ServiceConfiguration = configcore.ServiceConfiguration
type ServerConfiguration = configcore.ServerConfiguration
type RepositoryConfiguration = configcore.RepositoryConfiguration
type RuntimeConfiguration = configcore.RuntimeConfiguration
type RuntimeLLMOutputConfiguration = configcore.RuntimeLLMOutputConfiguration
type LLMConfiguration = configcore.LLMConfiguration
type LLMModelCapabilityConfiguration = configcore.LLMModelCapabilityConfiguration
type LLMProviderConfiguration = configcore.LLMProviderConfiguration
type LLMProviderTemplate = configcore.LLMProviderTemplate
type WebSearchConfiguration = configcore.WebSearchConfiguration
type WebSearchProviderConfiguration = configcore.WebSearchProviderConfiguration
type WebSearchProviderTemplate = configcore.WebSearchProviderTemplate
type GovernanceConfiguration = configcore.GovernanceConfiguration
type GovernanceAutoTicketConfiguration = configcore.GovernanceAutoTicketConfiguration
type GovernanceAutoApprovalConfiguration = configcore.GovernanceAutoApprovalConfiguration
type GovernanceBudgetConfiguration = configcore.GovernanceBudgetConfiguration
type UpdateLLMConfigurationCommand = configcore.UpdateLLMConfigurationCommand
type UpdateServerConfigurationCommand = configcore.UpdateServerConfigurationCommand
type UpdateLLMProviderConfiguration = configcore.UpdateLLMProviderConfiguration
type UpdateLLMProviderInferenceProfile = configcore.UpdateLLMProviderInferenceProfile
type UpdateLLMProviderContextProfile = configcore.UpdateLLMProviderContextProfile
type UpdateWebSearchConfigurationCommand = configcore.UpdateWebSearchConfigurationCommand
type UpdateWebSearchProviderConfiguration = configcore.UpdateWebSearchProviderConfiguration

func ReadServiceConfiguration() (ServiceConfiguration, error) {
	return configcore.ReadServiceConfiguration()
}

func DefaultLLMProviderTemplates() []LLMProviderTemplate {
	return configcore.DefaultLLMProviderTemplates()
}

func DefaultWebSearchProviderTemplates() []WebSearchProviderTemplate {
	return configcore.DefaultWebSearchProviderTemplates()
}

func UpdateServerConfiguration(cmd UpdateServerConfigurationCommand) (ServiceConfiguration, error) {
	return configcore.UpdateServerConfiguration(cmd)
}

func UpdateLLMConfiguration(cmd UpdateLLMConfigurationCommand) (ServiceConfiguration, error) {
	return configcore.UpdateLLMConfiguration(cmd)
}

func UpdateWebSearchConfiguration(cmd UpdateWebSearchConfigurationCommand) (ServiceConfiguration, error) {
	return configcore.UpdateWebSearchConfiguration(cmd)
}
