package llm

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
)

type providerFacingPrompt struct {
	StablePrefix           string
	StablePrefixSectionIDs []string
	DynamicMessages        []llmmodels.PromptTurn
	NormalizedSections     []string
	ProtectedSections      []string
}

func buildProviderFacingPrompt(request llmmodels.ProviderRequest) providerFacingPrompt {
	sections := providerPromptSections(request.PromptContext)
	stable := make([]llmmodels.PromptTurn, 0, len(sections))
	ret := providerFacingPrompt{}
	seenDynamic := false
	for _, section := range sections {
		content := strings.TrimSpace(section.Content)
		if content == "" {
			continue
		}
		message := llmmodels.PromptTurn{Role: normalizedPromptRole(section.Role), Content: content, SectionID: strings.TrimSpace(section.ID)}
		group := promptSectionGroup(message.SectionID)
		if !seenDynamic && isProviderCacheablePrefixGroup(group) {
			stable = append(stable, message)
		} else {
			seenDynamic = true
			message.Content = providerPromptSectionContent(message, len(ret.DynamicMessages)+1, "C3.dynamic")
			ret.DynamicMessages = append(ret.DynamicMessages, message)
		}
		if section.Normalized {
			ret.NormalizedSections = append(ret.NormalizedSections, message.SectionID)
		}
		if section.Protected {
			ret.ProtectedSections = append(ret.ProtectedSections, message.SectionID)
		}
	}
	if request.PromptContext != nil && request.PromptContext.Normalization != nil {
		ret.NormalizedSections = append(ret.NormalizedSections, request.PromptContext.Normalization.NormalizedSections...)
		ret.ProtectedSections = append(ret.ProtectedSections, request.PromptContext.Normalization.ProtectedSections...)
	}
	if len(stable) == 0 && len(ret.DynamicMessages) == 0 {
		system, user := providerFallbackPrompts(request)
		stable = append(stable, llmmodels.PromptTurn{Role: "system", Content: system, SectionID: "C0.structured-json-contract"})
		ret.DynamicMessages = append(ret.DynamicMessages, llmmodels.PromptTurn{Role: "user", Content: providerPromptSectionContent(llmmodels.PromptTurn{Role: "user", Content: user, SectionID: "C3.structured-json-request"}, 1, "C3.dynamic"), SectionID: "C3.structured-json-request"})
	}
	ret.StablePrefix = providerStablePrefixContent(stable)
	ret.StablePrefixSectionIDs = providerPromptSectionIDsWithFallback(stable, "C0.stable-prefix")
	ret.NormalizedSections = uniqueStrings(ret.NormalizedSections)
	ret.ProtectedSections = uniqueStrings(ret.ProtectedSections)
	return ret
}

func providerPromptSections(context *llmmodels.PromptContext) []llmmodels.PromptSection {
	if context == nil || len(context.Sections) == 0 {
		return nil
	}
	ret := append([]llmmodels.PromptSection(nil), context.Sections...)
	sort.SliceStable(ret, func(i, j int) bool { return ret[i].DisplayOrder < ret[j].DisplayOrder })
	return ret
}

func providerFallbackPrompts(request llmmodels.ProviderRequest) (string, string) {
	schema := strings.TrimSpace(request.Schema)
	if schema == "" {
		schema = "the requested JSON schema"
	}
	system := "Return exactly one JSON object matching " + schema + ". Do not wrap the JSON in markdown."
	payload, err := json.Marshal(struct {
		Kind           string            `json:"kind,omitempty"`
		Schema         string            `json:"schema,omitempty"`
		DialogLanguage string            `json:"dialogLanguage,omitempty"`
		TraceRefs      map[string]string `json:"traceRefs,omitempty"`
	}{request.Kind, request.Schema, request.DialogLanguage, request.TraceRefs})
	if err != nil {
		payload = []byte("{}")
	}
	return system, string(payload)
}

func normalizedPromptRole(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "system":
		return "system"
	case "assistant":
		return "assistant"
	default:
		return "user"
	}
}

func isProviderCacheablePrefixGroup(group string) bool {
	switch group {
	case "C0", "C1", "C2":
		return true
	default:
		return false
	}
}

func providerStablePrefixContent(messages []llmmodels.PromptTurn) string {
	parts := make([]string, 0, len(messages))
	for idx, item := range messages {
		if content := providerPromptSectionContent(item, idx+1, "C0.stable-prefix"); content != "" {
			parts = append(parts, content)
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n\n"))
}

func providerPromptSectionContent(item llmmodels.PromptTurn, idx int, fallbackPrefix string) string {
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

func providerPromptSectionIDs(messages []llmmodels.PromptTurn) []string {
	return providerPromptSectionIDsWithFallback(messages, "C3.dynamic")
}

func providerPromptSectionIDsWithFallback(messages []llmmodels.PromptTurn, fallbackPrefix string) []string {
	ret := make([]string, 0, len(messages))
	for idx, item := range messages {
		sectionID := strings.TrimSpace(item.SectionID)
		if sectionID == "" {
			sectionID = fmt.Sprintf("%s-%02d", fallbackPrefix, idx+1)
		}
		ret = append(ret, sectionID)
	}
	return nilIfEmptyStrings(ret)
}

func providerFacingPromptIncludedSections(prompt providerFacingPrompt) []string {
	ret := append([]string(nil), prompt.StablePrefixSectionIDs...)
	ret = append(ret, providerPromptSectionIDs(prompt.DynamicMessages)...)
	return uniqueStrings(ret)
}

func promptSectionGroup(sectionID string) string {
	sectionID = strings.TrimSpace(sectionID)
	if len(sectionID) >= 2 && sectionID[0] == 'C' && sectionID[1] >= '0' && sectionID[1] <= '9' {
		return sectionID[:2]
	}
	return ""
}

func promptSectionHash(sectionID, content string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(sectionID) + "\n" + strings.TrimSpace(content)))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func uniqueStrings(values []string) []string {
	ret := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		ret = append(ret, value)
	}
	return nilIfEmptyStrings(ret)
}

func nilIfEmptyStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	return values
}

func cacheControlSentForRequest(config llmmodels.ProviderConfig, prompt providerFacingPrompt) bool {
	return config.CacheCapability.MessageCacheControl && strings.TrimSpace(prompt.StablePrefix) != ""
}
