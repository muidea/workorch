package llm

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
)

const providerCachePlanSchemaVersion = "workorch.providerCachePlan.v1"

func attachProviderCachePlan(request *llmmodels.ProviderRequest) {
	if request == nil || request.PromptContext == nil || len(request.PromptContext.Sections) == 0 {
		return
	}
	if request.ContextBudget == nil {
		request.ContextBudget = &llmmodels.ProviderContextBudget{}
	}
	plan := buildProviderCachePlan(*request)
	request.ContextBudget.CachePlan = &plan
}

func buildProviderCachePlan(request llmmodels.ProviderRequest) llmmodels.ProviderCachePlan {
	sectionHashes := make([]llmmodels.ProviderCacheSectionHash, 0, len(request.PromptContext.Sections))
	sectionOrder := make([]string, 0, len(request.PromptContext.Sections))
	groupHashes := map[string][]string{}
	breakers := []llmmodels.ProviderCacheBreaker{}
	for _, section := range request.PromptContext.Sections {
		sectionID := strings.TrimSpace(section.ID)
		if sectionID == "" {
			continue
		}
		hash := strings.TrimSpace(section.Hash)
		if hash == "" {
			hash = promptSectionHash(sectionID, section.Content)
		}
		sectionOrder = append(sectionOrder, sectionID)
		group := promptSectionGroup(sectionID)
		if group != "" {
			groupHashes[group] = append(groupHashes[group], sectionID+"\n"+hash)
		}
		sectionHashes = append(sectionHashes, llmmodels.ProviderCacheSectionHash{
			SectionID:          sectionID,
			Name:               section.Name,
			Cacheability:       section.Cacheability,
			Hash:               hash,
			SizeBytes:          len(strings.TrimSpace(section.Content)),
			SourceRefs:         append([]string(nil), section.SourceRefs...),
			DefinitionRevision: section.DefinitionRevision,
		})
		if group == "C4" {
			breakers = append(breakers, llmmodels.ProviderCacheBreaker{
				SectionID: sectionID,
				Reason:    "volatile_execution_tail",
			})
		}
	}
	if definitionChangeImpactChanged(request.Input) {
		breakers = append(breakers, llmmodels.ProviderCacheBreaker{
			SectionID: "C1",
			Reason:    "definition_change_impact",
		})
	}
	stablePrefixHash := cacheGroupHash("C0", groupHashes["C0"])
	workspaceDefinitionHash := cacheGroupHash("C1", groupHashes["C1"])
	capabilityContractHash := cacheGroupHash("C2", groupHashes["C2"])
	stageBundleHash := cacheGroupHash("C3", groupHashes["C3"])
	volatileTailHash := cacheGroupHash("C4", groupHashes["C4"])
	largeRefsHash := cacheGroupHash("C5", groupHashes["C5"])
	plan := llmmodels.ProviderCachePlan{
		SchemaVersion:           providerCachePlanSchemaVersion,
		Strategy:                "provider_neutral_audit",
		CacheKey:                cacheGroupHash("cache-key", []string{stablePrefixHash, workspaceDefinitionHash, capabilityContractHash}),
		StablePrefixHash:        stablePrefixHash,
		WorkspaceDefinitionHash: workspaceDefinitionHash,
		CapabilityContractHash:  capabilityContractHash,
		StageBundleHash:         stageBundleHash,
		VolatileTailHash:        volatileTailHash,
		LargeRefsHash:           largeRefsHash,
		SectionHashes:           sectionHashes,
		CacheBreakers:           uniqueProviderCacheBreakers(breakers),
		Canonicalization: llmmodels.ProviderCanonicalizationPlan{
			SectionOrderPolicy: "preserve_prompt_context_order",
			SectionOrder:       sectionOrder,
			OrderedDataPolicy:  "preserve_runtime_sequence_for_goal_dod_events_tools_validations_steps_memory",
			MapKeyPolicy:       "stable_key_order_for_unordered_maps_only",
			ProviderMapping:    "adapter_optional",
		},
		ObservedRequestShape: llmmodels.ProviderObservedRequestShape{
			ProviderFacingMode:  "prompt_context_turns",
			TurnsHaveSectionIDs: promptContextTurnsHaveSectionIDs(request.PromptContext),
			TypedEventTape:      promptContextHasTypedEventTape(request.PromptContext),
		},
	}
	plan.AdapterHint = providerAdapterCacheHint(plan)
	return plan
}

func promptContextHasTypedEventTape(context *llmmodels.PromptContext) bool {
	if context == nil {
		return false
	}
	for _, section := range context.Sections {
		sectionID := strings.TrimSpace(section.ID)
		if !strings.HasPrefix(sectionID, "C4.") {
			continue
		}
		switch sectionID {
		case "C4.execution-signals", "C4.fallback-user-prompt":
			continue
		default:
			return true
		}
	}
	return false
}

func providerAdapterCacheHint(plan llmmodels.ProviderCachePlan) llmmodels.ProviderAdapterCacheHint {
	cacheablePrefix := make([]string, 0, len(plan.SectionHashes))
	volatileSections := make([]string, 0, len(plan.SectionHashes))
	for _, section := range plan.SectionHashes {
		group := promptSectionGroup(section.SectionID)
		if group == "C0" || group == "C1" || group == "C2" {
			cacheablePrefix = append(cacheablePrefix, section.SectionID)
			continue
		}
		if group == "C4" {
			volatileSections = append(volatileSections, section.SectionID)
		}
		break
	}
	breakers := make([]string, 0, len(plan.CacheBreakers))
	for _, breaker := range plan.CacheBreakers {
		key := strings.TrimSpace(breaker.SectionID)
		reason := strings.TrimSpace(breaker.Reason)
		switch {
		case key != "" && reason != "":
			breakers = append(breakers, key+":"+reason)
		case key != "":
			breakers = append(breakers, key)
		case reason != "":
			breakers = append(breakers, reason)
		}
	}
	return llmmodels.ProviderAdapterCacheHint{
		Enabled:          len(cacheablePrefix) > 0,
		Mode:             "provider_neutral_prefix_hint",
		ProviderMapping:  "adapter_optional_no_core_private_fields",
		CacheablePrefix:  cacheablePrefix,
		VolatileSections: volatileSections,
		CacheBreakers:    uniqueStrings(breakers),
	}
}

func cacheGroupHash(label string, items []string) string {
	if len(items) == 0 {
		return ""
	}
	builder := strings.Builder{}
	builder.WriteString(strings.TrimSpace(label))
	builder.WriteString("\n")
	for _, item := range items {
		builder.WriteString(strings.TrimSpace(item))
		builder.WriteString("\n")
	}
	sum := sha256.Sum256([]byte(builder.String()))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func promptContextTurnsHaveSectionIDs(context *llmmodels.PromptContext) bool {
	if context == nil || len(context.Turns) == 0 {
		return false
	}
	for _, turn := range context.Turns {
		if strings.TrimSpace(turn.SectionID) == "" {
			return false
		}
	}
	return true
}

func uniqueProviderCacheBreakers(items []llmmodels.ProviderCacheBreaker) []llmmodels.ProviderCacheBreaker {
	if len(items) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]llmmodels.ProviderCacheBreaker, 0, len(items))
	for _, item := range items {
		key := strings.TrimSpace(item.SectionID) + "\x00" + strings.TrimSpace(item.Reason)
		if key == "\x00" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, item)
	}
	return out
}
