package workorchport

import (
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

func SoulInfoFromManagement(item workorchmanagementcontract.Soul) SoulInfo {
	return SoulInfo{
		ID: common.SoulID(item.ID), Name: item.Name, Description: item.Description,
		BusinessDomains: cloneProjectionStrings(item.BusinessDomains), SoulMD: item.SoulMD,
	}
}

func SoulInfosFromManagement(items []workorchmanagementcontract.Soul) []SoulInfo {
	if len(items) == 0 {
		return nil
	}
	ret := make([]SoulInfo, 0, len(items))
	for _, item := range items {
		ret = append(ret, SoulInfoFromManagement(item))
	}
	return ret
}

func SkillInfoFromManagement(item workorchmanagementcontract.Skill) SkillInfo {
	return SkillInfo{ID: common.SkillID(item.ID), Name: item.Name, Version: item.Version, Description: item.Description, Instruction: item.Instruction}
}

func SkillInfosFromManagement(items []workorchmanagementcontract.Skill) []SkillInfo {
	if len(items) == 0 {
		return nil
	}
	ret := make([]SkillInfo, 0, len(items))
	for _, item := range items {
		ret = append(ret, SkillInfoFromManagement(item))
	}
	return ret
}

func MCPServerInfoFromManagement(item workorchmanagementcontract.MCPServer) MCPServerInfo {
	return MCPServerInfo{ID: common.MCPServerID(item.ID), Name: item.Name, Transport: item.Transport, Endpoint: item.Endpoint, Enabled: item.Enabled}
}

func MCPServerInfosFromManagement(items []workorchmanagementcontract.MCPServer) []MCPServerInfo {
	if len(items) == 0 {
		return nil
	}
	ret := make([]MCPServerInfo, 0, len(items))
	for _, item := range items {
		ret = append(ret, MCPServerInfoFromManagement(item))
	}
	return ret
}

func MCPCapabilityInfoFromManagement(item workorchmanagementcontract.MCPCapability) MCPCapabilityInfo {
	return MCPCapabilityInfo{
		ID: common.MCPCapabilityID(item.ID), ServerID: common.MCPServerID(item.ServerID),
		Kind: item.Kind, Name: item.Name, DisplayName: item.DisplayName, Description: item.Description,
	}
}

func MCPCapabilityInfosFromManagement(items []workorchmanagementcontract.MCPCapability) []MCPCapabilityInfo {
	if len(items) == 0 {
		return nil
	}
	ret := make([]MCPCapabilityInfo, 0, len(items))
	for _, item := range items {
		ret = append(ret, MCPCapabilityInfoFromManagement(item))
	}
	return ret
}

func MemoryItemInfoFromManagement(item workorchmanagementcontract.MemoryItem) MemoryItemInfo {
	return MemoryItemInfo{
		ID: common.MemoryID(item.ID), WorkspaceID: common.WorkspaceID(item.WorkspaceID), RunID: common.RunID(item.RunID),
		SourceType: item.SourceType, SourceRef: item.SourceRef, MemoryType: item.MemoryType,
		Content: item.Content, Summary: item.Summary, Tags: cloneProjectionStrings(item.Tags),
		Importance: item.Importance, Confidence: item.Confidence, Visibility: item.Visibility,
		EmbeddingProviderID: item.EmbeddingProviderID, EmbeddingModel: item.EmbeddingModel,
		EmbeddingText: item.EmbeddingText, VectorReady: item.VectorReady,
		VectorDimension: item.VectorDimension, VectorScore: item.VectorScore,
	}
}

func MemoryItemInfoPtrFromManagement(item *workorchmanagementcontract.MemoryItem) *MemoryItemInfo {
	if item == nil {
		return nil
	}
	ret := MemoryItemInfoFromManagement(*item)
	return &ret
}

func MemoryItemInfosFromManagement(items []workorchmanagementcontract.MemoryItem) []MemoryItemInfo {
	if len(items) == 0 {
		return nil
	}
	ret := make([]MemoryItemInfo, 0, len(items))
	for _, item := range items {
		ret = append(ret, MemoryItemInfoFromManagement(item))
	}
	return ret
}

func MemoryMutationFromInfo(info *MemoryItemInfo) *workorchmanagementcontract.MemoryMutation {
	if info == nil {
		return &workorchmanagementcontract.MemoryMutation{}
	}
	return &workorchmanagementcontract.MemoryMutation{
		MemoryID: string(info.ID), RunID: string(info.RunID), SourceType: info.SourceType, SourceRef: info.SourceRef,
		MemoryType: info.MemoryType, Content: info.Content, Summary: info.Summary, Tags: cloneProjectionStrings(info.Tags),
		Importance: info.Importance, Confidence: info.Confidence, Visibility: info.Visibility,
		EmbeddingProviderID: info.EmbeddingProviderID, EmbeddingModel: info.EmbeddingModel,
		EmbeddingText: info.EmbeddingText, Vector: append([]float64(nil), info.Vector...),
	}
}

func TemplateInfoFromManagement(item workorchmanagementcontract.WorkspaceTemplate) TemplateInfo {
	return TemplateInfo{
		ID: item.ID, Name: item.Name, Description: item.Description, GoalText: item.GoalText,
		DoDItems: cloneProjectionStrings(item.DoDItems), DefaultSoulID: common.SoulID(item.DefaultSoulID),
		SoulBindings: cloneProjectionStringMap(item.SoulBindings), ContextHints: projectionPayload(item.ContextHints),
		EvidencePolicies: projectionPayloads(item.EvidencePolicies),
	}
}

func TemplateInfoPtrFromManagement(item *workorchmanagementcontract.WorkspaceTemplate) *TemplateInfo {
	if item == nil {
		return nil
	}
	ret := TemplateInfoFromManagement(*item)
	return &ret
}

func TemplateInfosFromManagement(items []workorchmanagementcontract.WorkspaceTemplate) []TemplateInfo {
	if len(items) == 0 {
		return nil
	}
	ret := make([]TemplateInfo, 0, len(items))
	for _, item := range items {
		ret = append(ret, TemplateInfoFromManagement(item))
	}
	return ret
}

func ManagementTemplateFromInfo(info *TemplateInfo) *workorchmanagementcontract.WorkspaceTemplate {
	if info == nil {
		return &workorchmanagementcontract.WorkspaceTemplate{}
	}
	return &workorchmanagementcontract.WorkspaceTemplate{
		ID: info.ID, Name: info.Name, Description: info.Description, GoalText: info.GoalText,
		DoDItems: cloneProjectionStrings(info.DoDItems), DefaultSoulID: string(info.DefaultSoulID),
		SoulBindings: cloneProjectionStringMap(info.SoulBindings), ContextHints: info.ContextHints.Map(),
		EvidencePolicies: workorcheventpayload.MapSlice(info.EvidencePolicies),
	}
}

func cloneProjectionStringMap(input map[string]string) map[string]string {
	if len(input) == 0 {
		return nil
	}
	ret := make(map[string]string, len(input))
	for key, value := range input {
		ret[key] = value
	}
	return ret
}
