package llm

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
)

type responsesInputMessage struct {
	Role    string                  `json:"role"`
	Content []responsesInputContent `json:"content"`
}

type responsesInputContent struct {
	Type                  string                    `json:"type"`
	Text                  string                    `json:"text"`
	PromptCacheBreakpoint *responsesCacheBreakpoint `json:"prompt_cache_breakpoint,omitempty"`
}

type responsesCacheBreakpoint struct {
	Mode string `json:"mode"`
}
type responsesCacheOptions struct {
	Mode string `json:"mode"`
}

func responsesPromptCacheOptions(request llmmodels.ProviderRequest) *responsesCacheOptions {
	if !usesSectionMessages(request) {
		return nil
	}
	return &responsesCacheOptions{Mode: "explicit"}
}

func usesSectionMessages(request llmmodels.ProviderRequest) bool {
	return request.PromptContext != nil && request.PromptContext.MessageLayout == llmmodels.PromptMessageLayoutSectionsV1
}

// sectionMessages preserves real roles and exact section content. In particular,
// cacheable user data must not become developer instructions for cache reuse.
func sectionMessages(request llmmodels.ProviderRequest, prefixOnly bool) []responsesInputMessage {
	var messages []responsesInputMessage
	lastPrefix := -1
	for _, section := range request.PromptContext.Sections {
		if prefixOnly && !isProviderCacheablePrefixGroup(promptSectionGroup(section.ID)) {
			break
		}
		role := section.Role
		if role == "system" {
			role = "developer"
		}
		messages = append(messages, responsesInputMessage{Role: role, Content: []responsesInputContent{{Type: "input_text", Text: "### " + section.ID + "\n" + section.Content}}})
		if isProviderCacheablePrefixGroup(promptSectionGroup(section.ID)) {
			lastPrefix = len(messages) - 1
		}
	}
	if lastPrefix >= 0 {
		messages[lastPrefix].Content[0].PromptCacheBreakpoint = &responsesCacheBreakpoint{Mode: "explicit"}
	}
	return messages
}

func sectionMessagePrefixHash(request llmmodels.ProviderRequest) string {
	prefix, _ := json.Marshal(sectionMessages(request, true)) // concrete strings only
	sum := sha256.Sum256(prefix)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func responsesPromptCacheKey(request llmmodels.ProviderRequest) string {
	if !usesSectionMessages(request) {
		return ""
	}
	// No physical-call identity, review kind, timestamp, endpoint or credential.
	// Scope and actual rendered content, including roles, isolate cache reuse.
	identity, _ := json.Marshal(struct {
		Version      string
		Workspace    string
		Provider     string
		Model        string
		Schema       string
		SchemaFormat string
		Reasoning    string
		PrefixHash   string
	}{"workorch.section-cache/v1", request.WorkspaceID, request.Config.ID, request.Model, request.Schema, request.SchemaFormat, request.ReasoningEffort, sectionMessagePrefixHash(request)})
	sum := sha256.Sum256(identity)
	return "workorch:" + hex.EncodeToString(sum[:])
}

func responsesWireInputForRequest(request *llmmodels.ProviderRequest) json.RawMessage {
	if !usesSectionMessages(*request) {
		wire, _ := json.Marshal(responsesInputForRequest(request))
		return wire
	}
	// Recompute from actual content; caller-supplied hashes/plans never select
	// the wire cache identity. The shared budget is owner-local audit metadata.
	attachProviderCachePlan(request)
	request.ContextBudget.PromptCaptured = true
	request.ContextBudget.IncludedSections = nil
	for _, s := range request.PromptContext.Sections {
		request.ContextBudget.IncludedSections = append(request.ContextBudget.IncludedSections, s.ID)
	}
	messages := sectionMessages(*request, false)
	wire, _ := json.Marshal(messages)
	return wire
}
