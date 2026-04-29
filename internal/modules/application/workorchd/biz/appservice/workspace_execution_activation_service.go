package appservice

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"slices"
	"strings"

	llmbiz "github.com/muidea/workorch/internal/modules/blocks/llm/biz"
	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	mcpmodel "github.com/muidea/workorch/internal/modules/kernel/mcp/pkg/models"
	memorymodel "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/models"
	runmodule "github.com/muidea/workorch/internal/modules/kernel/run"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	soulmodule "github.com/muidea/workorch/internal/modules/kernel/soul"
	soulmodel "github.com/muidea/workorch/internal/modules/kernel/soul/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
)

type ActivateWorkspaceExecutionCommand struct {
	GoalID        string   `json:"goalId,omitempty"`
	DoDID         string   `json:"doDId,omitempty"`
	LLMProviderID string   `json:"llmProviderId,omitempty"`
	SoulID        string   `json:"soulId,omitempty"`
	SkillIDs      []string `json:"skillIds,omitempty"`
	MCPServerIDs  []string `json:"mcpServerIds,omitempty"`
	MemoryIDs     []string `json:"memoryIds,omitempty"`
	Priority      int      `json:"priority,omitempty"`
}

type WorkspaceExecutionActivation struct {
	Workspace Workspace         `json:"workspace"`
	Run       Run               `json:"run"`
	Snapshot  WorkspaceSnapshot `json:"snapshot"`
}

func ActivateWorkspaceExecutionWithSession(ctx context.Context, session *RuntimeSession, workspaceID string, cmd ActivateWorkspaceExecutionCommand) (ret WorkspaceExecutionActivation, err error) {
	exports, err := session.Exports()
	if err != nil {
		return WorkspaceExecutionActivation{}, err
	}
	if exports.Services.RunService == nil ||
		exports.Repositories.Workspaces == nil ||
		exports.Repositories.Goals == nil ||
		exports.Repositories.DoDs == nil ||
		exports.Repositories.Runs == nil {
		return WorkspaceExecutionActivation{}, common.ErrInvalidArgument
	}
	defer session.CloseOnReturn(ctx, &err)

	workspace, err := exports.Repositories.Workspaces.Get(ctx, common.WorkspaceID(workspaceID))
	if err != nil {
		return WorkspaceExecutionActivation{}, err
	}
	if workspace.ActiveRunID != "" {
		run, err := exports.Repositories.Runs.Get(ctx, workspace.ActiveRunID)
		if err != nil {
			return WorkspaceExecutionActivation{}, err
		}
		snapshot, err := readWorkspaceSnapshotFromExports(ctx, session, common.WorkspaceID(workspaceID))
		if err != nil {
			return WorkspaceExecutionActivation{}, err
		}
		return WorkspaceExecutionActivation{
			Workspace: snapshot.Workspace,
			Run:       mapRun(run),
			Snapshot:  snapshot,
		}, nil
	}

	goalID, dodID, err := selectWorkspaceExecutionDefinitions(ctx, session, common.WorkspaceID(workspaceID), cmd)
	if err != nil {
		return WorkspaceExecutionActivation{}, err
	}
	if err := validateRunDefinitions(ctx, session, workspaceID, string(goalID), string(dodID)); err != nil {
		return WorkspaceExecutionActivation{}, err
	}
	executionContext, err := buildWorkspaceExecutionContext(ctx, session, common.WorkspaceID(workspaceID), goalID, dodID, cmd)
	if err != nil {
		return WorkspaceExecutionActivation{}, err
	}

	run, err := createAndStartWorkspaceExecutionRun(ctx, session, common.WorkspaceID(workspaceID), goalID, dodID, executionContext, cmd.Priority)
	if err != nil {
		return WorkspaceExecutionActivation{}, err
	}
	snapshot, err := readWorkspaceSnapshotFromExports(ctx, session, common.WorkspaceID(workspaceID))
	if err != nil {
		return WorkspaceExecutionActivation{}, err
	}
	return WorkspaceExecutionActivation{
		Workspace: snapshot.Workspace,
		Run:       mapRun(run),
		Snapshot:  snapshot,
	}, nil
}

func ActivateWorkspaceExecutionWithBackend(ctx context.Context, backend string, dataRoot string, workspaceID string, cmd ActivateWorkspaceExecutionCommand) (WorkspaceExecutionActivation, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return WorkspaceExecutionActivation{}, err
	}
	return ActivateWorkspaceExecutionWithSession(ctx, session, workspaceID, cmd)
}

func ActivateWorkspaceExecutionFromEnv(ctx context.Context, workspaceID string, cmd ActivateWorkspaceExecutionCommand) (WorkspaceExecutionActivation, error) {
	session, err := NewRuntimeSessionFromEnv()
	if err != nil {
		return WorkspaceExecutionActivation{}, err
	}
	return ActivateWorkspaceExecutionWithSession(ctx, session, workspaceID, cmd)
}

func selectWorkspaceExecutionDefinitions(ctx context.Context, session *RuntimeSession, workspaceID common.WorkspaceID, cmd ActivateWorkspaceExecutionCommand) (common.GoalID, common.DoDID, error) {
	exports, err := session.Exports()
	if err != nil {
		return "", "", err
	}

	goalID, err := selectWorkspaceExecutionGoal(ctx, exports.Repositories.Goals.ListByWorkspaceID, workspaceID, common.GoalID(cmd.GoalID))
	if err != nil {
		return "", "", err
	}
	dodID, err := selectWorkspaceExecutionDoD(ctx, exports.Repositories.DoDs.ListByWorkspaceID, workspaceID, common.DoDID(cmd.DoDID))
	if err != nil {
		return "", "", err
	}
	return goalID, dodID, nil
}

func selectWorkspaceExecutionGoal(
	ctx context.Context,
	listByWorkspaceID func(context.Context, common.WorkspaceID) ([]*workspacemodel.Goal, error),
	workspaceID common.WorkspaceID,
	requestedID common.GoalID,
) (common.GoalID, error) {
	items, err := listByWorkspaceID(ctx, workspaceID)
	if err != nil {
		return "", err
	}
	if requestedID != "" {
		for _, item := range items {
			if item.ID == requestedID {
				return requestedID, nil
			}
		}
		return "", common.ErrConflict
	}
	if len(items) == 0 {
		return "", common.ErrInvalidArgument
	}
	if len(items) > 1 {
		return "", common.ErrConflict
	}
	return items[0].ID, nil
}

func selectWorkspaceExecutionDoD(
	ctx context.Context,
	listByWorkspaceID func(context.Context, common.WorkspaceID) ([]*workspacemodel.DoD, error),
	workspaceID common.WorkspaceID,
	requestedID common.DoDID,
) (common.DoDID, error) {
	items, err := listByWorkspaceID(ctx, workspaceID)
	if err != nil {
		return "", err
	}
	readyItems := make([]*workspacemodel.DoD, 0, len(items))
	for _, item := range items {
		if item.State == common.DoDStateReady || item.State == common.DoDStateSatisfied {
			readyItems = append(readyItems, item)
		}
	}
	if requestedID != "" {
		for _, item := range readyItems {
			if item.ID == requestedID {
				return requestedID, nil
			}
		}
		return "", common.ErrConflict
	}
	if len(readyItems) == 0 {
		return "", common.ErrInvalidArgument
	}
	if len(readyItems) > 1 {
		return "", common.ErrConflict
	}
	return readyItems[0].ID, nil
}

func buildWorkspaceExecutionContext(ctx context.Context, session *RuntimeSession, workspaceID common.WorkspaceID, goalID common.GoalID, dodID common.DoDID, cmd ActivateWorkspaceExecutionCommand) (map[string]any, error) {
	exports, err := session.Exports()
	if err != nil {
		return nil, err
	}
	contextData := map[string]any{
		"activation":  "workspace_execution",
		"workspaceId": string(workspaceID),
		"goalId":      string(goalID),
		"doDId":       string(dodID),
	}
	goal, err := exports.Repositories.Goals.Get(ctx, goalID)
	if err != nil {
		return nil, err
	}
	dod, err := exports.Repositories.DoDs.Get(ctx, dodID)
	if err != nil {
		return nil, err
	}
	contextData["goal"] = goalContext(goal)
	contextData["dod"] = dodContext(dod)
	if llmProviderID := strings.TrimSpace(cmd.LLMProviderID); llmProviderID != "" {
		contextData[llmcommon.ExecutionContextLLMProviderID] = llmProviderID
	}

	mcpServerIDs := normalizeStringIDs(cmd.MCPServerIDs)
	memoryIDs := normalizeStringIDs(cmd.MemoryIDs)

	var soul *soulmodel.Soul
	if strings.TrimSpace(cmd.SoulID) != "" {
		if exports.Services.SoulService == nil {
			return nil, common.ErrInvalidArgument
		}
		soul, err = exports.Services.SoulService.GetSoul(ctx, common.SoulID(strings.TrimSpace(cmd.SoulID)))
		if err != nil {
			return nil, err
		}
		contextData["soulId"] = string(soul.ID)
		contextData["soul"] = soulContext(soul)
	}

	skills, skillIDs, skillSelection, err := selectSkillsForWorkspaceExecution(ctx, session, workspaceID, goal, dod, cmd.SkillIDs, soul)
	if err != nil {
		return nil, err
	}
	contextData["skillSelection"] = skillSelection
	if len(skills) > 0 {
		contextData["skillIds"] = skillIDs
		contextData["skills"] = skillContexts(skills)
	}

	requiredMCPServerIDs := requiredMCPServerRefs(skills)
	autoBoundMCPServerIDs := make([]string, 0)
	for _, id := range requiredMCPServerIDs {
		if !slices.Contains(mcpServerIDs, id) {
			mcpServerIDs = append(mcpServerIDs, id)
			autoBoundMCPServerIDs = append(autoBoundMCPServerIDs, id)
		}
	}
	mcpServers, err := selectedMCPServers(ctx, exports.Services.MCPService, mcpServerIDs)
	if err != nil {
		return nil, err
	}
	contextData["mcpSelection"] = map[string]any{
		"policy":                "skill_required_mcp_auto_bind",
		"requestedMCPServerIds": normalizeStringIDs(cmd.MCPServerIDs),
		"requiredMCPServerIds":  requiredMCPServerIDs,
		"autoBoundMCPServerIds": autoBoundMCPServerIDs,
		"selectedMCPServerIds":  mcpServerIDs,
	}
	if len(mcpServers) > 0 {
		contextData["mcpServerIds"] = mcpServerIDs
		contextData["mcpServers"] = mcpServerContexts(mcpServers)
	}

	if soul != nil {
		if err := exports.Services.SoulService.ValidateCapabilitySelection(ctx, soulmodule.ValidateCapabilitySelectionCommand{
			SoulID:        soul.ID,
			SkillRefs:     skillIDs,
			MCPServerRefs: mcpServerIDs,
			ToolKinds:     requiredToolKinds(skills),
		}); err != nil {
			return nil, err
		}
	}

	memories, retrievalEvidence, err := selectedMemoryItems(ctx, workspaceID, exports.Services.MemoryService, memoryIDs, goal, dod)
	if err != nil {
		return nil, err
	}
	if retrievalEvidence != nil {
		contextData["memoryRetrieval"] = retrievalEvidence
	}
	if len(memories) > 0 {
		contextData["memoryIds"] = memoryItemIDs(memories)
		contextData["memories"] = memoryContexts(memories)
	}

	return contextData, nil
}

func selectedSkills(ctx context.Context, session *RuntimeSession, ids []string, workspaceID string) ([]Skill, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	if session == nil {
		return nil, common.ErrInvalidArgument
	}
	exports, err := session.Exports()
	if err != nil {
		return nil, err
	}
	if exports.Repositories.Skills == nil {
		return nil, common.ErrInvalidArgument
	}
	ret := make([]Skill, 0, len(ids))
	for _, id := range ids {
		item, err := exports.Repositories.Skills.Get(ctx, common.SkillID(id))
		if err == nil {
			ret = append(ret, mapSkill(item))
			continue
		}
		if !errors.Is(err, common.ErrNotFound) {
			return nil, err
		}
		skill, err := readSkillHubSkill(ctx, session, id, SkillQuery{WorkspaceID: workspaceID})
		if err != nil {
			return nil, err
		}
		ret = append(ret, skill)
	}
	return ret, nil
}

func selectedMCPServers(ctx context.Context, service interface {
	GetServer(context.Context, common.MCPServerID) (*mcpmodel.Server, error)
}, ids []string) ([]*mcpmodel.Server, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	if service == nil {
		return nil, common.ErrInvalidArgument
	}
	ret := make([]*mcpmodel.Server, 0, len(ids))
	for _, id := range ids {
		item, err := service.GetServer(ctx, common.MCPServerID(id))
		if err != nil {
			return nil, err
		}
		if !item.Enabled {
			return nil, common.ErrConflict
		}
		ret = append(ret, item)
	}
	return ret, nil
}

func selectedMemoryItems(ctx context.Context, workspaceID common.WorkspaceID, service interface {
	GetMemoryItem(context.Context, common.MemoryID) (*memorymodel.MemoryItem, error)
	ListMemoryByWorkspace(context.Context, common.WorkspaceID) ([]*memorymodel.MemoryItem, error)
	SearchMemory(context.Context, repository.MemorySearchQuery) ([]*memorymodel.MemoryItem, error)
}, ids []string, goal *workspacemodel.Goal, dod *workspacemodel.DoD) ([]*memorymodel.MemoryItem, map[string]any, error) {
	if service == nil {
		if len(ids) > 0 {
			return nil, nil, common.ErrInvalidArgument
		}
		return nil, nil, nil
	}
	if len(ids) > 0 {
		ret := make([]*memorymodel.MemoryItem, 0, len(ids))
		for _, id := range ids {
			item, err := service.GetMemoryItem(ctx, common.MemoryID(id))
			if err != nil {
				return nil, nil, err
			}
			if item.WorkspaceID != workspaceID {
				return nil, nil, common.ErrConflict
			}
			ret = append(ret, item)
		}
		return ret, map[string]any{
			"policy":             "explicit_memory_selection",
			"status":             "selected",
			"retrievedMemoryIds": memoryItemIDs(ret),
		}, nil
	}
	semanticItems, semanticEvidence, semanticHandled := semanticMemoryItems(ctx, workspaceID, service, goal, dod)
	if semanticHandled {
		return semanticItems, semanticEvidence, nil
	}
	items, err := service.ListMemoryByWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, nil, err
	}
	if len(items) > 8 {
		items = items[:8]
	}
	return items, map[string]any{
		"policy":             "deterministic_workspace_list",
		"status":             "selected",
		"retrievedMemoryIds": memoryItemIDs(items),
	}, nil
}

func semanticMemoryItems(ctx context.Context, workspaceID common.WorkspaceID, service interface {
	SearchMemory(context.Context, repository.MemorySearchQuery) ([]*memorymodel.MemoryItem, error)
}, goal *workspacemodel.Goal, dod *workspacemodel.DoD) ([]*memorymodel.MemoryItem, map[string]any, bool) {
	cfg, err := workorchconfig.Current()
	if err != nil {
		return nil, map[string]any{
			"policy": "semantic_vector_search",
			"status": "degraded",
			"reason": err.Error(),
		}, true
	}
	providerID := strings.TrimSpace(cfg.LLM.DefaultEmbeddingProvider)
	if providerID == "" {
		return nil, nil, false
	}
	queryText := memoryRetrievalQueryText(goal, dod)
	evidence := map[string]any{
		"policy":              "semantic_vector_search",
		"providerId":          providerID,
		"operation":           llmcommon.ProviderOperationEmbeddings,
		"queryTextPreview":    previewRetrievalQuery(queryText),
		"queryTextSha256":     sha256Hex(queryText),
		"minScore":            0.1,
		"limit":               8,
		"retrievedMemoryIds":  []string{},
		"retrievedMemoryHits": []map[string]any{},
	}
	defaultProviderID, providerConfigs, err := llmbiz.LoadProviderConfigs(cfg.LLM.DefaultProvider, cfg.LLMProviderValueSets())
	if err != nil {
		evidence["status"] = "degraded"
		evidence["reason"] = err.Error()
		return nil, evidence, true
	}
	providerConfig, ok := providerConfigs[providerID]
	if !ok {
		evidence["status"] = "degraded"
		evidence["reason"] = fmt.Sprintf("embedding provider %q is not configured", providerID)
		return nil, evidence, true
	}
	if providerConfig.Unavailable {
		evidence["status"] = "degraded"
		evidence["reason"] = providerConfig.UnavailableReason
		return nil, evidence, true
	}
	adapter := llmbiz.NewRuntimeAdapterFromConfigs(defaultProviderID, providerConfigs)
	embedding, err := adapter.GenerateEmbedding(ctx, providerID, queryText)
	if err != nil {
		evidence["status"] = "degraded"
		evidence["reason"] = err.Error()
		return nil, evidence, true
	}
	items, err := service.SearchMemory(ctx, repository.MemorySearchQuery{
		WorkspaceID: workspaceID,
		Vector:      embedding.Vector,
		MinScore:    0.1,
		Limit:       8,
	})
	if err != nil {
		evidence["status"] = "degraded"
		evidence["reason"] = err.Error()
		return nil, evidence, true
	}
	evidence["status"] = "selected"
	evidence["embeddingModel"] = embedding.Model
	evidence["vectorDimension"] = len(embedding.Vector)
	evidence["retrievedMemoryIds"] = memoryItemIDs(items)
	evidence["retrievedMemoryHits"] = memoryRetrievalHits(items)
	return items, evidence, true
}

func normalizeStringIDs(input []string) []string {
	if len(input) == 0 {
		return nil
	}
	ret := make([]string, 0, len(input))
	seen := map[string]struct{}{}
	for _, item := range input {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		ret = append(ret, item)
	}
	return ret
}

func requiredToolKinds(skills []Skill) []string {
	ret := make([]string, 0)
	seen := map[string]struct{}{}
	for _, skill := range skills {
		for _, kind := range skill.RequiredToolKinds {
			kind = strings.TrimSpace(kind)
			if kind == "" {
				continue
			}
			if _, ok := seen[kind]; ok {
				continue
			}
			seen[kind] = struct{}{}
			ret = append(ret, kind)
		}
	}
	return ret
}

func requiredMCPServerRefs(skills []Skill) []string {
	ret := make([]string, 0)
	seen := map[string]struct{}{}
	for _, skill := range skills {
		for _, id := range skill.RequiredMCPServerRefs {
			id = strings.TrimSpace(id)
			if id == "" {
				continue
			}
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			ret = append(ret, id)
		}
	}
	return ret
}

func memoryItemIDs(items []*memorymodel.MemoryItem) []string {
	ret := make([]string, 0, len(items))
	for _, item := range items {
		ret = append(ret, string(item.ID))
	}
	return ret
}

func memoryRetrievalHits(items []*memorymodel.MemoryItem) []map[string]any {
	ret := make([]map[string]any, 0, len(items))
	for _, item := range items {
		ret = append(ret, map[string]any{
			"id":              string(item.ID),
			"summary":         item.Summary,
			"vectorScore":     item.VectorScore,
			"vectorDimension": len(item.Vector),
		})
	}
	return ret
}

func memoryRetrievalQueryText(goal *workspacemodel.Goal, dod *workspacemodel.DoD) string {
	parts := []string{}
	if goal != nil && strings.TrimSpace(goal.Text) != "" {
		parts = append(parts, "Goal: "+strings.TrimSpace(goal.Text))
	}
	if dod != nil && len(dod.Items) > 0 {
		parts = append(parts, "Definition of Done:")
		for _, item := range dod.Items {
			item = strings.TrimSpace(item)
			if item != "" {
				parts = append(parts, "- "+item)
			}
		}
	}
	return strings.Join(parts, "\n")
}

func previewRetrievalQuery(text string) string {
	text = strings.TrimSpace(text)
	const limit = 240
	if len(text) <= limit {
		return text
	}
	return text[:limit]
}

func sha256Hex(text string) string {
	sum := sha256.Sum256([]byte(text))
	return fmt.Sprintf("%x", sum)
}

func goalContext(item *workspacemodel.Goal) map[string]any {
	if item == nil {
		return nil
	}
	return map[string]any{
		"id":          string(item.ID),
		"workspaceId": string(item.WorkspaceID),
		"text":        item.Text,
	}
}

func dodContext(item *workspacemodel.DoD) map[string]any {
	if item == nil {
		return nil
	}
	return map[string]any{
		"id":          string(item.ID),
		"workspaceId": string(item.WorkspaceID),
		"items":       cloneStrings(item.Items),
		"state":       string(item.State),
	}
}

func soulContext(item *soulmodel.Soul) map[string]any {
	return map[string]any{
		"id":                   string(item.ID),
		"name":                 item.Name,
		"description":          item.Description,
		"businessDomains":      cloneStrings(item.BusinessDomains),
		"systemInstruction":    item.SystemInstruction,
		"planningPolicy":       item.PlanningPolicy,
		"decisionPolicy":       item.DecisionPolicy,
		"approvalPolicy":       item.ApprovalPolicy,
		"validationPolicy":     item.ValidationPolicy,
		"allowedSkillRefs":     cloneStrings(item.AllowedSkillRefs),
		"allowedMCPServerRefs": cloneStrings(item.AllowedMCPServerRefs),
		"allowedToolKinds":     cloneStrings(item.AllowedToolKinds),
		"outputContract":       item.OutputContract,
	}
}

func skillContexts(items []Skill) []map[string]any {
	ret := make([]map[string]any, 0, len(items))
	for _, item := range items {
		ret = append(ret, map[string]any{
			"id":                    item.ID,
			"name":                  item.Name,
			"version":               item.Version,
			"description":           item.Description,
			"triggerRules":          cloneStrings(item.TriggerRules),
			"instruction":           item.Instruction,
			"referenceRefs":         cloneStrings(item.ReferenceRefs),
			"requiredToolKinds":     cloneStrings(item.RequiredToolKinds),
			"requiredMCPServerRefs": cloneStrings(item.RequiredMCPServerRefs),
			"constraints":           cloneStrings(item.Constraints),
			"source":                item.Source,
			"scope":                 item.Scope,
			"workspaceId":           item.WorkspaceID,
			"status":                item.Status,
			"sourceRepository":      item.SourceRepository,
			"sourcePath":            item.SourcePath,
			"targetPath":            item.TargetPath,
			"localPath":             item.LocalPath,
			"repoPath":              item.RepoPath,
			"globalPath":            item.GlobalPath,
			"projectPath":           item.ProjectPath,
			"contentHash":           item.ContentHash,
			"sourceHash":            item.SourceHash,
			"appliedHash":           item.AppliedHash,
			"actualHash":            item.ActualHash,
			"agent":                 item.Agent,
			"managedBy":             item.ManagedBy,
			"sourceSkillId":         item.SourceSkillID,
		})
	}
	return ret
}

func mcpServerContexts(items []*mcpmodel.Server) []map[string]any {
	ret := make([]map[string]any, 0, len(items))
	for _, item := range items {
		ret = append(ret, map[string]any{
			"id":                    string(item.ID),
			"name":                  item.Name,
			"transport":             item.Transport,
			"enabled":               item.Enabled,
			"allowedTools":          cloneStrings(item.AllowedTools),
			"allowedResources":      cloneStrings(item.AllowedResources),
			"defaultApprovalPolicy": item.DefaultApprovalPolicy,
			"timeoutMillis":         item.TimeoutMillis,
		})
	}
	return ret
}

func memoryContexts(items []*memorymodel.MemoryItem) []map[string]any {
	ret := make([]map[string]any, 0, len(items))
	for _, item := range items {
		ret = append(ret, map[string]any{
			"id":                  string(item.ID),
			"workspaceId":         string(item.WorkspaceID),
			"runId":               string(item.RunID),
			"sourceType":          item.SourceType,
			"sourceRef":           item.SourceRef,
			"memoryType":          item.MemoryType,
			"summary":             item.Summary,
			"content":             item.Content,
			"tags":                cloneStrings(item.Tags),
			"importance":          item.Importance,
			"confidence":          item.Confidence,
			"visibility":          item.Visibility,
			"embeddingProviderId": item.EmbeddingProviderID,
			"embeddingModel":      item.EmbeddingModel,
			"vectorReady":         len(item.Vector) > 0,
			"vectorDimension":     len(item.Vector),
			"vectorScore":         item.VectorScore,
		})
	}
	return ret
}

func createAndStartWorkspaceExecutionRun(ctx context.Context, session *RuntimeSession, workspaceID common.WorkspaceID, goalID common.GoalID, dodID common.DoDID, executionContext map[string]any, priority int) (*runmodel.Run, error) {
	exports, err := session.Exports()
	if err != nil {
		return nil, err
	}
	for attempt := 0; attempt < 8; attempt++ {
		runID := common.RunID(generateBusinessID("run"))
		if err := exports.Services.RunService.CreateRun(ctx, runmodule.CreateRunCommand{
			RunID:            runID,
			WorkspaceID:      workspaceID,
			GoalID:           goalID,
			DoDID:            dodID,
			InitialStepKind:  llmcommon.StepKindWorkspaceExecution,
			ExecutionContext: executionContext,
			Priority:         priority,
		}); err != nil {
			if errors.Is(err, common.ErrConflict) {
				continue
			}
			return nil, err
		}
		if err := exports.Services.RunService.EnsureStarted(ctx, runmodule.StartRunCommand{RunID: runID}); err != nil {
			return nil, err
		}
		return exports.Repositories.Runs.Get(ctx, runID)
	}
	return nil, common.ErrConflict
}

func readWorkspaceSnapshotFromExports(ctx context.Context, session *RuntimeSession, workspaceID common.WorkspaceID) (WorkspaceSnapshot, error) {
	deps, err := readModelDependenciesFromSession(session)
	if err != nil {
		return WorkspaceSnapshot{}, err
	}
	return readWorkspaceSnapshot(ctx, deps, workspaceID)
}
