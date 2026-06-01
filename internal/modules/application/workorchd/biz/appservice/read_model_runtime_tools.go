package appservice

import (
	"context"
	"net/url"
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/pkg/presenter"
	gitcommon "github.com/muidea/workorch/internal/modules/blocks/git/pkg/common"
	webcommon "github.com/muidea/workorch/internal/modules/blocks/web/pkg/common"
	mcpmodel "github.com/muidea/workorch/internal/modules/kernel/mcp/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
)

func mapToolCalls(items []*runmodel.ToolCall) []ToolCall {
	if len(items) == 0 {
		return nil
	}
	ret := make([]ToolCall, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		ret = append(ret, ToolCall{
			ID:          string(item.ID),
			WorkspaceID: string(item.WorkspaceID),
			RunID:       string(item.RunID),
			StepID:      string(item.StepID),
			ToolName:    item.ToolName,
			Arguments:   cloneReadModelMap(item.Arguments),
			State:       string(item.State),
			StartedAt:   item.StartedAt,
			EndedAt:     item.EndedAt,
			ExitSummary: item.ExitSummary,
			Output:      cloneReadModelMap(item.Output),
			Error:       item.Error,
		})
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func mapWebSourceTraces(steps []presenter.StepView) []WebSourceTrace {
	if len(steps) == 0 {
		return nil
	}
	ret := make([]WebSourceTrace, 0)
	for _, step := range steps {
		if step.Output == nil {
			continue
		}
		switch step.Kind {
		case webcommon.StepKindSearch:
			trace := webSearchTrace(step)
			if len(trace.Sources) > 0 || trace.Query != "" {
				ret = append(ret, trace)
			}
		case webcommon.StepKindFetch:
			trace := webFetchTrace(step)
			if len(trace.FetchedSources) > 0 || trace.ApprovalID != "" {
				ret = append(ret, trace)
			}
		}
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func mapGitOperationTraces(steps []presenter.StepView) []GitOperationTrace {
	if len(steps) == 0 {
		return nil
	}
	ret := make([]GitOperationTrace, 0)
	for _, step := range steps {
		if step.Kind != gitcommon.StepKindRead && step.Kind != gitcommon.StepKindWrite {
			continue
		}
		if len(step.Output) == 0 {
			continue
		}
		commit := readModelMapValue(step.Output["commit"])
		ret = append(ret, GitOperationTrace{
			StepID:           step.ID,
			Kind:             step.Kind,
			Operation:        readModelStringValue(step.Output["operation"]),
			Status:           readModelStringValue(step.Output["status"]),
			RepoRoot:         readModelStringValue(step.Output["repoRoot"]),
			WorkingDir:       readModelStringValue(step.Output["workingDir"]),
			Branch:           readModelStringValue(step.Output["branch"]),
			HeadCommit:       readModelStringValue(step.Output["headCommit"]),
			HeadCommitBefore: readModelStringValue(step.Output["headCommitBefore"]),
			HeadCommitAfter:  readModelStringValue(step.Output["headCommitAfter"]),
			CommitHash:       readModelStringValue(commit["hash"]),
			CommitSubject:    readModelStringValue(commit["subject"]),
			ChangedFiles:     mapGitChangedFiles(step.Output["changedFiles"]),
			StagedFiles:      mapGitChangedFiles(step.Output["stagedFiles"]),
			Truncated:        readModelBoolValue(step.Output["truncated"]),
			ZeroWork:         readModelBoolValue(step.Output["zeroWork"]),
			ErrorCode:        readModelStringValue(step.Output["errorCode"]),
			Summary:          readModelStringValue(step.Output["summary"]),
			EvidenceRefs:     []string{step.ID},
		})
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func mapGitChangedFiles(value any) []GitChangedFile {
	items := readModelMapSliceValue(value)
	if len(items) == 0 {
		return nil
	}
	ret := make([]GitChangedFile, 0, len(items))
	for _, item := range items {
		path := readModelStringValue(item["path"])
		if path == "" {
			continue
		}
		ret = append(ret, GitChangedFile{
			Path:      path,
			OldPath:   readModelStringValue(item["oldPath"]),
			Status:    readModelStringValue(item["status"]),
			Staged:    readModelBoolValue(item["staged"]),
			Unstaged:  readModelBoolValue(item["unstaged"]),
			Untracked: readModelBoolValue(item["untracked"]),
		})
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func webSearchTrace(step presenter.StepView) WebSourceTrace {
	output := step.Output
	evidenceRef := step.ID
	results := readModelMapSliceValue(output["results"])
	sources := make([]WebSource, 0, len(results))
	for _, result := range results {
		item := WebSource{
			Title:       readModelStringValue(result["title"]),
			URL:         readModelStringValue(result["url"]),
			Snippet:     readModelStringValue(result["snippet"]),
			Source:      readModelStringValue(result["source"]),
			PublishedAt: readModelStringValue(result["publishedAt"]),
			Rank:        readModelIntValue(result["rank"]),
			EvidenceRef: evidenceRef,
		}
		if item.URL != "" {
			sources = append(sources, item)
		}
	}
	return WebSourceTrace{
		Query:          readModelStringValue(output["query"]),
		ProviderID:     readModelStringValue(output["provider"]),
		SearchedAt:     readModelStringValue(output["searchedAt"]),
		Sources:        sources,
		EvidenceRefs:   []string{evidenceRef},
		PolicyDecision: readModelPolicyDecision(output["policy"]),
	}
}

func webFetchTrace(step presenter.StepView) WebSourceTrace {
	output := step.Output
	evidenceRef := step.ID
	fetched := FetchedWebSource{
		URL:         readModelStringValue(output["url"]),
		FinalURL:    readModelStringValue(output["finalUrl"]),
		Title:       readModelStringValue(output["title"]),
		Source:      readModelStringValue(output["source"]),
		RetrievedAt: readModelStringValue(output["retrievedAt"]),
		ContentHash: readModelStringValue(output["contentHash"]),
		Truncated:   readModelBoolValue(output["truncated"]),
		EvidenceRef: evidenceRef,
	}
	fetchedSources := []FetchedWebSource(nil)
	if fetched.URL != "" {
		fetchedSources = []FetchedWebSource{fetched}
	}
	return WebSourceTrace{
		ProviderID:     readModelStringValue(output["provider"]),
		FetchedSources: fetchedSources,
		EvidenceRefs:   []string{evidenceRef},
		ApprovalID:     readModelStringValue(output["approvalId"]),
		PolicyDecision: readModelPolicyDecision(output["policy"]),
	}
}

func selectedRuntimeCapabilities(executionContext map[string]any, catalog map[string]RuntimeCapability) []RuntimeCapability {
	if len(executionContext) == 0 {
		return nil
	}
	ret := make([]RuntimeCapability, 0)
	seen := map[string]struct{}{}
	for _, raw := range readModelMapSliceValue(executionContext["mcpCapabilities"]) {
		item := runtimeCapabilityFromContext(raw)
		if item.ID == "" {
			continue
		}
		if _, ok := seen[item.ID]; ok {
			continue
		}
		seen[item.ID] = struct{}{}
		ret = append(ret, item)
	}
	for _, id := range readModelStringSliceValue(executionContext["capabilityIds"]) {
		if _, ok := seen[id]; ok {
			continue
		}
		if item, ok := catalog[id]; ok {
			ret = append(ret, item)
		} else {
			ret = append(ret, RuntimeCapability{ID: id, DisplayName: id})
		}
		seen[id] = struct{}{}
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func usedRuntimeCapabilities(snapshot RunSnapshot, catalog map[string]RuntimeCapability) []RuntimeCapability {
	if len(snapshot.ToolCalls) == 0 {
		return nil
	}
	selected := map[string]RuntimeCapability{}
	for _, item := range snapshot.SelectedCapabilities {
		selected[item.ID] = item
	}
	used := map[string]RuntimeCapability{}
	order := make([]string, 0)
	for _, toolCall := range snapshot.ToolCalls {
		identity := runtimeCapabilityIdentity(toolCall)
		if identity.ID == "" {
			continue
		}
		item, ok := used[identity.ID]
		if !ok {
			if loaded, exists := catalog[identity.ID]; exists {
				item = loaded
			} else if selectedItem, exists := selected[identity.ID]; exists {
				item = selectedItem
			} else {
				item = identity
			}
			order = append(order, identity.ID)
		}
		item.UseCount++
		item.EvidenceRefs = compactUniqueStrings(append(item.EvidenceRefs, toolCall.ID, toolCall.StepID))
		used[identity.ID] = item
	}
	ret := make([]RuntimeCapability, 0, len(order))
	for _, id := range order {
		ret = append(ret, used[id])
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func runtimeCapabilityCatalog(ctx context.Context, deps readModelDependencies, executionContext map[string]any, toolCalls []ToolCall) map[string]RuntimeCapability {
	if deps.mcpCapabilities == nil || !requiresRuntimeCapabilityCatalog(executionContext, toolCalls) {
		return nil
	}
	items, err := deps.mcpCapabilities.List(ctx)
	if err != nil {
		return nil
	}
	return runtimeCapabilityCatalogFromItems(executionContext, toolCalls, items)
}

func runtimeCapabilityCatalogFromItems(executionContext map[string]any, toolCalls []ToolCall, items []*mcpmodel.Capability) map[string]RuntimeCapability {
	if items == nil || !requiresRuntimeCapabilityCatalog(executionContext, toolCalls) {
		return nil
	}
	ret := make(map[string]RuntimeCapability, len(items))
	for _, item := range items {
		capability := runtimeCapabilityFromModel(item)
		if capability.ID == "" {
			continue
		}
		ret[capability.ID] = capability
	}
	return ret
}

func requiresRuntimeCapabilityCatalog(executionContext map[string]any, toolCalls []ToolCall) bool {
	if len(readModelStringSliceValue(executionContext["capabilityIds"])) > 0 {
		return true
	}
	for _, toolCall := range toolCalls {
		if runtimeCapabilityIdentity(toolCall).ID != "" {
			return true
		}
	}
	return false
}

func runtimeCapabilityIdentity(toolCall ToolCall) RuntimeCapability {
	source := toolCall.Output
	if len(source) == 0 {
		source = toolCall.Arguments
	}
	serverID := readModelStringValue(source["serverId"])
	operation := readModelStringValue(source["operation"])
	var kind string
	var name string
	switch operation {
	case "mcp/tool_call":
		kind = mcpmodel.CapabilityKindTool
		name = readModelStringValue(source["tool"])
	case "mcp/resource_read":
		kind = mcpmodel.CapabilityKindResource
		name = readModelStringValue(source["uri"])
	case "mcp/prompt_get":
		kind = mcpmodel.CapabilityKindPrompt
		name = readModelStringValue(source["prompt"])
	}
	if serverID == "" || kind == "" || name == "" {
		return RuntimeCapability{}
	}
	id := defaultRuntimeCapabilityID(serverID, kind, name)
	return RuntimeCapability{
		ID:          id,
		ServerID:    serverID,
		Kind:        kind,
		Name:        name,
		DisplayName: name,
	}
}

func runtimeCapabilityFromModel(item *mcpmodel.Capability) RuntimeCapability {
	if item == nil {
		return RuntimeCapability{}
	}
	return RuntimeCapability{
		ID:             string(item.ID),
		ServerID:       string(item.ServerID),
		Kind:           item.Kind,
		Name:           item.Name,
		DisplayName:    firstNonEmptyReadModel(item.DisplayName, item.Name, string(item.ID)),
		Description:    item.Description,
		Category:       item.Category,
		Tags:           cloneReadModelStringSlice(item.Tags),
		RiskLevel:      item.RiskLevel,
		Enabled:        item.Enabled,
		ApprovalPolicy: item.ApprovalPolicy,
		Status:         item.Status,
	}
}

func runtimeCapabilityFromContext(raw map[string]any) RuntimeCapability {
	id := readModelStringValue(raw["id"])
	return RuntimeCapability{
		ID:             id,
		ServerID:       readModelStringValue(raw["serverId"]),
		Kind:           readModelStringValue(raw["kind"]),
		Name:           readModelStringValue(raw["name"]),
		DisplayName:    firstNonEmptyReadModel(readModelStringValue(raw["displayName"]), readModelStringValue(raw["name"]), id),
		Description:    readModelStringValue(raw["description"]),
		Category:       readModelStringValue(raw["category"]),
		Tags:           readModelStringSliceValue(raw["tags"]),
		RiskLevel:      readModelStringValue(raw["riskLevel"]),
		Enabled:        readModelBoolValue(raw["enabled"]),
		ApprovalPolicy: readModelStringValue(raw["approvalPolicy"]),
		Status:         readModelStringValue(raw["status"]),
	}
}

func defaultRuntimeCapabilityID(serverID string, kind string, name string) string {
	return strings.TrimSpace(serverID) + ":" + strings.TrimSpace(kind) + ":" + url.PathEscape(strings.TrimSpace(name))
}
