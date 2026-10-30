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
		SchemaScope  string
		SchemaFormat string
		JSONSchema   bool
		RawText      bool
		Reasoning    string
		PrefixHash   string
	}{"workorch.section-cache/v2", request.WorkspaceID, request.Config.ID, request.Model, responsesCacheSchemaScope(request), request.SchemaFormat, request.Config.Capabilities.JSONSchemaResponse, request.RawTextOutput, request.ReasoningEffort, sectionMessagePrefixHash(request)})
	sum := sha256.Sum256(identity)
	return "workorch:" + hex.EncodeToString(sum[:])
}

// Runtime schemas bind the current manifest, plan step and batch. Those are
// independently checked on every response, but are volatile cache scope. Only
// the LLM owner's exact known C0 contract selects its stable schema family;
// neither a caller's hash nor a claimed prompt version is sufficient. Unknown
// contracts retain full schema isolation. This is a routing hint, never proof
// that a schema, response, manifest or operation has passed admission.
func responsesCacheSchemaScope(request llmmodels.ProviderRequest) string {
	if request.ValidatePromptLayout() == nil && usesSectionMessages(request) {
		first := request.PromptContext.Sections[0]
		if first.Role == "system" {
			switch request.PromptContext.SchemaVersion {
			case "workorch.runtime_candidate_prompt":
				if first.ID == "C0.runtime-candidate-contract" && first.Content == runtimeCandidateContractCatalog() {
					return "runtime/operations-schema/v1"
				}
			case "workorch.runtime_candidate_prompt/v2":
				if first.ID == "C0.runtime-candidate-contract" && first.Content == runtimeCandidateContractCatalogV2() {
					return "runtime/operations-schema/v2"
				}
			case "workorch.runtime_semantic_plan_prompt":
				if first.ID == "C0.runtime-semantic-contract" && first.Content == runtimeSemanticPlanContractCatalog() {
					return "runtime/semantic-plan-schema/v1"
				}
			}
		}
	}
	return "exact-schema:" + request.Schema
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
