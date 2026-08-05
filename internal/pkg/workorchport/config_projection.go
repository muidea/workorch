package workorchport

import "github.com/muidea/workorch/internal/pkg/workorchconfigurationcontract"

func ServiceConfigFromConfiguration(item workorchconfigurationcontract.ServiceConfiguration) ServiceConfig {
	return ServiceConfig{
		ServerAddr: item.Server.Addr, Backend: item.Repository.Backend, DataDir: item.Repository.DataDir,
		DefaultChat: item.LLM.DefaultChat, MaxConcurrentRuns: item.Server.MaxConcurrentRuns,
		AIProxyBaseURL: item.LLM.AIProxy.BaseURL, LLMOutput: item.Runtime.LLMOutput,
		ChatModelTargets: ChatModelTargetOptionsFromConfiguration(&item),
	}
}

func ServiceConfigPtrFromConfiguration(item *workorchconfigurationcontract.ServiceConfiguration) *ServiceConfig {
	if item == nil {
		return &ServiceConfig{}
	}
	ret := ServiceConfigFromConfiguration(*item)
	return &ret
}

func ChatModelTargetOptionsFromConfiguration(item *workorchconfigurationcontract.ServiceConfiguration) []ChatModelTargetOption {
	if item == nil {
		return nil
	}
	ret := make([]ChatModelTargetOption, 0)
	for _, model := range item.LLM.Catalog.Models {
		if !model.CapabilityReady || !containsProjectionString(model.Operations, "chat_completions") {
			continue
		}
		ret = append(ret, ChatModelTargetOption{
			ProviderID: model.ProviderID, ModelID: model.ModelID, DisplayName: model.ModelID,
			ContextWindowTokens: model.ContextWindowTokens, MaxOutputTokens: model.MaxOutputTokens,
			CapabilityReady: true, CapabilityRevision: model.CapabilityRevision, Source: item.LLM.Catalog.Source,
		})
	}
	for _, provider := range item.LLM.CustomProviders {
		for _, model := range provider.Models {
			if !containsProjectionString(model.Operations, "chat_completions") {
				continue
			}
			ret = append(ret, ChatModelTargetOption{
				ProviderID: provider.ID, ModelID: model.ID, DisplayName: model.ID,
				ContextWindowTokens: model.ContextWindowTokens, MaxOutputTokens: model.MaxOutputTokens,
				CapabilityReady: true, Source: "configuration",
			})
		}
	}
	return NormalizeChatModelTargetOptions(ret)
}

func containsProjectionString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
