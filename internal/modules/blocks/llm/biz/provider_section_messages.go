package llm

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"

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
		blocks := []responsesInputContent{{Type: "input_text", Text: "### " + section.ID + "\n" + section.Content}}
		if role == "user" && (section.ID == "C3.semantic-authority" || section.ID == "C3.runtime-authority") {
			// At most three C3 boundaries plus C2: content, complete work, authority.
			// New blocks concatenate exactly; old frozen messages are never rebuilt.
			ends := []int{}
			if section.ID == "C3.semantic-authority" {
				if end := semanticContentPrefixEnd(section.Content); end > 0 {
					ends = append(ends, end)
				}
				if end := semanticWorkPrefixEnd(section.Content); end > 0 && (len(ends) == 0 || end > ends[len(ends)-1]) {
					ends = append(ends, end)
				}
			}
			if len(ends) == 0 || ends[len(ends)-1] < len(section.Content) {
				ends = append(ends, len(section.Content))
			}
			blocks = nil
			begin := 0
			for _, end := range ends {
				text := section.Content[begin:end]
				if begin == 0 {
					text = "### " + section.ID + "\n" + text
				}
				blocks = append(blocks, responsesInputContent{Type: "input_text", Text: text, PromptCacheBreakpoint: &responsesCacheBreakpoint{Mode: "explicit"}})
				begin = end
			}
		}
		messages = append(messages, responsesInputMessage{Role: role, Content: blocks})
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
	if request.FrozenCacheKey != "" {
		return request.FrozenCacheKey
	}
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
	}{"workorch.section-cache", request.WorkspaceID, request.Config.ID, request.Model, responsesCacheSchemaScope(request), request.SchemaFormat, request.Config.Capabilities.JSONSchemaResponse, request.RawTextOutput, request.ReasoningEffort, sectionMessagePrefixHash(request)})
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
				if first.ID == "C0.runtime-candidate-contract" && first.Content == runtimeCandidateContractCatalogCurrent() {
					return "runtime/operations-schema"
				}
			case "workorch.runtime_semantic_plan_prompt":
				if first.ID == "C0.runtime-semantic-contract" && first.Content == runtimeSemanticPlanContractCatalog() {
					return "runtime/semantic-plan-schema"
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
	if len(request.FrozenSectionMessages) > 0 {
		return append(json.RawMessage(nil), request.FrozenSectionMessages...)
	}
	messages := sectionMessages(*request, false)
	wire, _ := json.Marshal(messages)
	return wire
}

// Return the exact complete work prefix; content has its own earlier boundary.
func semanticWorkPrefixEnd(content string) int {
	_, end := jsonFieldRange(content, "workContext")
	return end
}

// jsonFieldRange returns offsets into the original JSON, not re-marshaled text.
func jsonFieldRange(content, field string) (start, end int) {
	d := json.NewDecoder(bytes.NewBufferString(content))
	if token, err := d.Token(); err != nil || token != json.Delim('{') {
		return 0, 0
	}
	for d.More() {
		key, err := d.Token()
		if err != nil {
			return 0, 0
		}
		var raw json.RawMessage
		if d.Decode(&raw) != nil {
			return 0, 0
		}
		if key == field && string(raw) != "null" {
			start, end = int(d.InputOffset())-len(raw), int(d.InputOffset())
		}
	}
	if _, err := d.Token(); err != nil {
		return 0, 0
	}
	if _, err := d.Token(); err != io.EOF {
		return 0, 0
	}
	return
}

func semanticContentPrefixEnd(content string) int {
	start, end := jsonFieldRange(content, "workContext")
	if end == 0 {
		return 0
	}
	innerStart, innerEnd := jsonFieldRange(content[start:end], "content")
	if innerEnd == 0 {
		return 0
	}
	_, bodiesEnd := jsonFieldRange(content[start+innerStart:start+innerEnd], "Snapshots")
	if bodiesEnd == 0 {
		return 0
	}
	return start + innerStart + bodiesEnd
}
