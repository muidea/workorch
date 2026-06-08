package llm

import (
	"fmt"
	"strings"

	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
)

type providerFacingPrompt struct {
	StablePrefix           string
	StablePrefixSectionIDs []string
	DynamicMessages        []providerPromptMessage
	NormalizedSections     []string
	ProtectedSections      []string
}

func buildProviderFacingPrompt(request llmmodels.ProviderRequest) providerFacingPrompt {
	build := buildProviderPromptMessages(request)
	ret := providerFacingPrompt{
		NormalizedSections: append([]string(nil), build.NormalizedSections...),
		ProtectedSections:  append([]string(nil), build.ProtectedSections...),
	}
	stablePrefixMessages := []providerPromptMessage{}
	seenDynamic := false
	for _, item := range build.Messages {
		item.Content = strings.TrimSpace(item.Content)
		if item.Content == "" {
			continue
		}
		group := promptSectionGroup(item.SectionID)
		if !seenDynamic && isProviderCacheablePrefixGroup(group) {
			stablePrefixMessages = append(stablePrefixMessages, item)
			continue
		}
		seenDynamic = true
		item.Content = providerPromptSectionContent(item, len(ret.DynamicMessages)+1, "C4.dynamic")
		ret.DynamicMessages = append(ret.DynamicMessages, item)
	}
	if len(stablePrefixMessages) == 0 {
		stablePrefixMessages = append(stablePrefixMessages, promptMessageForSection(request, "C0.system", "system", "normalize_allowed", systemPromptForRequest(request)))
	}
	ret.StablePrefix = providerStablePrefixContent(stablePrefixMessages)
	ret.StablePrefixSectionIDs = providerPromptSectionIDsWithFallback(stablePrefixMessages, "C0.stable-prefix")
	if len(ret.DynamicMessages) == 0 {
		fallback := promptMessageForSection(request, "C4.fallback-user-prompt", "user", "normalize_allowed", userPrompt(request))
		fallback.Content = providerPromptSectionContent(fallback, 1, "C4.dynamic")
		ret.DynamicMessages = append(ret.DynamicMessages, fallback)
	}
	return ret
}

func isProviderCacheablePrefixGroup(group string) bool {
	switch group {
	case "C0", "C1", "C2":
		return true
	default:
		return false
	}
}

func providerStablePrefixContent(messages []providerPromptMessage) string {
	parts := make([]string, 0, len(messages))
	for idx, item := range messages {
		content := providerPromptSectionContent(item, idx+1, "C0.stable-prefix")
		if content == "" {
			continue
		}
		parts = append(parts, content)
	}
	return strings.TrimSpace(strings.Join(parts, "\n\n"))
}

func providerPromptSectionContent(item providerPromptMessage, idx int, fallbackPrefix string) string {
	content := strings.TrimSpace(item.Content)
	if content == "" {
		return ""
	}
	sectionID := strings.TrimSpace(item.SectionID)
	if sectionID == "" {
		sectionID = fmt.Sprintf("%s-%02d", fallbackPrefix, idx)
	}
	return "### " + sectionID + "\n" + content
}

func providerPromptSectionIDs(messages []providerPromptMessage) []string {
	return providerPromptSectionIDsWithFallback(messages, "C4.dynamic")
}

func providerPromptSectionIDsWithFallback(messages []providerPromptMessage, fallbackPrefix string) []string {
	if len(messages) == 0 {
		return nil
	}
	ret := make([]string, 0, len(messages))
	for idx, item := range messages {
		sectionID := strings.TrimSpace(item.SectionID)
		if sectionID == "" {
			sectionID = fmt.Sprintf("%s-%02d", fallbackPrefix, idx+1)
		}
		ret = append(ret, sectionID)
	}
	return ret
}

func providerFacingPromptIncludedSections(prompt providerFacingPrompt) []string {
	ret := append([]string(nil), prompt.StablePrefixSectionIDs...)
	ret = append(ret, providerPromptSectionIDs(prompt.DynamicMessages)...)
	return uniqueStrings(ret)
}
