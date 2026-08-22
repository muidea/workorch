package workorchport

import (
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
	"github.com/muidea/workorch/internal/pkg/workorchconfigurationcontract"
)

func ServiceConfigFromConfiguration(item workorchconfigurationcontract.ServiceConfiguration) ServiceConfig {
	return ServiceConfig{
		ServerAddr: item.Server.Addr, Backend: item.Repository.Backend, DataDir: item.Repository.DataDir,
		DefaultChat:    item.LLM.DefaultChat,
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
		if !model.CapabilityReady || !containsProjectionString(model.Operations, workorchconfig.LLMOperationResponses) {
			continue
		}
		ret = append(ret, ChatModelTargetOption{
			ProviderID: model.ProviderID, ModelID: model.ModelID, DisplayName: model.ModelID,
			ContextWindowTokens: model.ContextWindowTokens, MaxOutputTokens: model.MaxOutputTokens,
			CapabilityReady: true, CapabilityRevision: model.CapabilityRevision, Source: item.LLM.Catalog.Source,
			ReasoningSupported: model.Reasoning.Supported, ReasoningValid: model.Reasoning.Valid,
			DefaultReasoningEffort: model.Reasoning.DefaultEffort,
			ReasoningEfforts:       append([]string(nil), model.Reasoning.Efforts...),
			ReasoningInvalidReason: model.Reasoning.InvalidReason,
		})
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
