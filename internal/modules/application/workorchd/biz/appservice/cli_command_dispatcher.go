package appservice

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

// CLICommandDispatcher owns /cli/command handling for workorchd. The CLI/TUI
// backend publishes command envelopes only; owner state is read or changed here.
type CLICommandDispatcher struct {
	session  *RuntimeSession
	eventHub events.Hub
	unsub    func()
}

func NewCLICommandDispatcher(session *RuntimeSession, eventHub events.Hub) *CLICommandDispatcher {
	return &CLICommandDispatcher{session: session, eventHub: eventHub}
}

func (d *CLICommandDispatcher) Run() error {
	if d == nil || d.session == nil || d.eventHub == nil {
		return common.ErrInvalidArgument
	}
	if d.unsub != nil {
		return nil
	}
	d.unsub = d.eventHub.Subscribe(events.TopicCLICommand, d.handleEnvelope)
	return nil
}

func (d *CLICommandDispatcher) Close() {
	if d == nil {
		return
	}
	if d.unsub != nil {
		d.unsub()
		d.unsub = nil
	}
}

func (d *CLICommandDispatcher) handleEnvelope(ctx context.Context, envelope events.Envelope) error {
	cmd, ok := events.DecodeCLICommandPayload(envelope.Payload)
	if !ok {
		return common.ErrInvalidArgument
	}
	result := d.dispatchCommand(ctx, cmd)
	resultEnvelope := events.Envelope{
		Topic:        events.TopicCLIResult,
		SourceModule: "workorchd-cli-dispatcher",
		SubjectType:  cmd.SubjectType,
		SubjectID:    cmd.SubjectID,
		WorkspaceID:  string(cmd.WorkspaceID),
		RunID:        string(cmd.RunID),
		Payload:      events.CLIResultPayloadMap(result),
	}
	if asyncHub, ok := d.eventHub.(events.AsyncHub); ok {
		_ = asyncHub.PublishAsync(context.Background(), resultEnvelope)
	} else {
		_ = d.eventHub.Publish(context.Background(), resultEnvelope)
	}
	return events.Response(resultEnvelope)
}

func (d *CLICommandDispatcher) dispatchCommand(ctx context.Context, cmd events.CLICommandPayload) events.CLIResultPayload {
	result := events.CLIResultPayload{
		CorrelationID: cmd.CorrelationID,
		CommandID:     cmd.CommandID,
		OccurredAt:    time.Now().UTC(),
		WorkspaceID:   cmd.WorkspaceID,
		RunID:         cmd.RunID,
	}
	resource, resources, err := d.handleCommand(ContextWithEventHub(ctx, d.eventHub), cmd)
	if err != nil {
		result.OK = false
		result.Error = err.Error()
		return result
	}
	result.OK = true
	result.Resource = resource
	result.Resources = resources
	return result
}

func (d *CLICommandDispatcher) handleCommand(ctx context.Context, cmd events.CLICommandPayload) (map[string]any, []map[string]any, error) {
	if cmd.WorkspaceID != "" {
		ctx = common.ContextWithRuntimeWorkspaceID(ctx, cmd.WorkspaceID)
	}
	switch cmd.Command {
	case events.CommandWorkspaceCreate:
		item, err := CreateWorkspaceWithTemplateWithSession(ctx, d.borrow(), CreateWorkspaceCommand{
			Title:                stringValue(cmd.Payload, "title"),
			Description:          stringValue(cmd.Payload, "description"),
			DeliverableDirectory: stringValue(cmd.Payload, "deliverableDirectory"),
			TemplateID:           stringValue(cmd.Payload, "templateId"),
		})
		return cliResourceMap(item), nil, err
	case events.CommandWorkspaceGet:
		item, err := ReadWorkspaceWithSession(ctx, d.borrow(), cmd.SubjectID)
		return cliResourceMap(item), nil, err
	case events.CommandWorkspaceList:
		items, err := ListWorkspacesWithSession(ctx, d.borrow())
		return nil, cliResourceMaps(items), err
	case events.CommandWorkspaceSnapshot:
		item, err := ReadWorkspaceSnapshotWithSession(ctx, d.borrow(), cmd.SubjectID)
		return cliResourceMap(item), nil, err
	case events.CommandWorkspaceDelete:
		err := DeleteWorkspaceWithSession(ctx, d.borrow(), cmd.SubjectID, DeleteWorkspaceCommand{Purge: boolValue(cmd.Payload, "purge")})
		return nil, nil, err
	case events.CommandGoalDefine:
		item, err := DefineGoalWithSession(ctx, d.borrow(), string(cmd.WorkspaceID), stringValue(cmd.Payload, "text"))
		return cliResourceMap(item), nil, err
	case events.CommandGoalList:
		items, err := ListGoalsWithSession(ctx, d.borrow(), string(cmd.WorkspaceID))
		return nil, cliResourceMaps(items), err
	case events.CommandGoalGet:
		item, err := ReadGoalWithSession(ctx, d.borrow(), cmd.SubjectID)
		return cliResourceMap(item), nil, err
	case events.CommandGoalUpdate:
		item, err := UpdateGoalWithSession(ctx, d.borrow(), cmd.SubjectID, stringValue(cmd.Payload, "text"))
		return cliResourceMap(item), nil, err
	case events.CommandDoDDefine:
		item, err := DefineDoDWithSession(ctx, d.borrow(), string(cmd.WorkspaceID), stringSliceValue(cmd.Payload, "items"))
		return cliResourceMap(item), nil, err
	case events.CommandDoDList:
		items, err := ListDoDsWithSession(ctx, d.borrow(), string(cmd.WorkspaceID))
		return nil, cliResourceMaps(items), err
	case events.CommandDoDGet:
		item, err := ReadDoDWithSession(ctx, d.borrow(), cmd.SubjectID)
		return cliResourceMap(item), nil, err
	case events.CommandDoDUpdate:
		item, err := UpdateDoDWithSession(ctx, d.borrow(), cmd.SubjectID, stringSliceValue(cmd.Payload, "items"))
		return cliResourceMap(item), nil, err
	case events.CommandRunActivate:
		item, err := ActivateWorkspaceExecutionWithSession(ctx, d.borrow(), string(cmd.WorkspaceID), ActivateWorkspaceExecutionCommand{
			GoalID:                stringValue(cmd.Payload, "goalId"),
			DoDID:                 stringValue(cmd.Payload, "doDId"),
			DialogLanguage:        stringValue(cmd.Payload, "dialogLanguage"),
			SoulID:                stringValue(cmd.Payload, "soulId"),
			SkillIDs:              stringSliceValue(cmd.Payload, "skillIds"),
			MCPServerIDs:          stringSliceValue(cmd.Payload, "mcpServerIds"),
			Priority:              intValue(cmd.Payload, "priority"),
			ExecutionContextPatch: mapValue(cmd.Payload, "executionContextPatch"),
		})
		return cliResourceMap(item.Run), nil, err
	case events.CommandRunResume:
		item, err := ResumeRunWithSession(ctx, d.borrow(), cmd.SubjectID, mapValue(cmd.Payload, "executionContextPatch"))
		return cliResourceMap(item), nil, err
	case events.CommandRunGet:
		item, err := ReadRunWithSession(ctx, d.borrow(), cmd.SubjectID)
		return cliResourceMap(item), nil, err
	case events.CommandRunList:
		items, err := ListRunsWithSession(ctx, d.borrow(), string(cmd.WorkspaceID))
		return nil, cliResourceMaps(items), err
	case events.CommandRunStatus:
		item, err := ReadRunStatusWithSession(ctx, d.borrow(), cmd.SubjectID)
		return cliResourceMap(item), nil, err
	case events.CommandDecisionResolve:
		item, err := ResolveDecisionWithSession(ctx, d.borrow(), cmd.SubjectID, ResolveDecisionCommand{
			ResolutionType:    stringValue(cmd.Payload, "resolutionType"),
			ResolutionPayload: firstMapValue(cmd.Payload, "payload", "resolutionPayload"),
		})
		return cliResourceMap(item), nil, err
	case events.CommandDecisionList:
		items, err := ListDecisionsWithSession(ctx, d.borrow(), string(cmd.WorkspaceID), "")
		return nil, cliResourceMaps(items), err
	case events.CommandDecisionGet:
		item, err := ReadDecisionWithSession(ctx, d.borrow(), cmd.SubjectID)
		return cliResourceMap(item), nil, err
	case events.CommandDecisionBatch:
		items, err := BatchResolveDecisionsWithSession(ctx, d.borrow(), BatchResolveDecisionCommand{
			TicketIDs:         stringSliceValue(cmd.Payload, "ticketIds"),
			ResolutionType:    stringValue(cmd.Payload, "resolutionType"),
			ResolutionPayload: firstMapValue(cmd.Payload, "payload", "resolutionPayload"),
		})
		return nil, cliResourceMaps(items), err
	case events.CommandApprovalApprove:
		item, err := ApproveApprovalWithSession(ctx, d.borrow(), cmd.SubjectID)
		return cliResourceMap(item), nil, err
	case events.CommandApprovalReject:
		item, err := RejectApprovalWithSession(ctx, d.borrow(), cmd.SubjectID)
		return cliResourceMap(item), nil, err
	case events.CommandApprovalList:
		items, err := ListApprovalsWithSession(ctx, d.borrow(), string(cmd.WorkspaceID), "")
		return nil, cliResourceMaps(items), err
	case events.CommandApprovalGet:
		item, err := ReadApprovalWithSession(ctx, d.borrow(), cmd.SubjectID)
		return cliResourceMap(item), nil, err
	case events.CommandApprovalBatch:
		items, err := BatchResolveApprovalsWithSession(ctx, d.borrow(), BatchResolveApprovalCommand{
			ApprovalIDs: stringSliceValue(cmd.Payload, "approvalIds"),
			Action:      stringValue(cmd.Payload, "action"),
		})
		return nil, cliResourceMaps(items), err
	case events.CommandSoulCreate:
		item, err := CreateSoulWithSession(ctx, d.borrow(), *decodeCLIOrZero[CreateSoulCommand](cmd.Payload))
		return cliResourceMap(item), nil, err
	case events.CommandSoulGet:
		item, err := ReadSoulWithSession(ctx, d.borrow(), cmd.SubjectID)
		return cliResourceMap(item), nil, err
	case events.CommandSoulList:
		items, err := ListSoulsWithSession(ctx, d.borrow())
		return nil, cliResourceMaps(items), err
	case events.CommandSoulUpdate:
		item, err := UpdateSoulWithSession(ctx, d.borrow(), cmd.SubjectID, *decodeCLIOrZero[UpdateSoulCommand](cmd.Payload))
		return cliResourceMap(item), nil, err
	case events.CommandSoulDelete:
		err := DeleteSoulWithSession(ctx, d.borrow(), cmd.SubjectID)
		return nil, nil, err
	case events.CommandSkillCreate:
		item, err := CreateSkillWithSession(ctx, d.borrow(), *decodeCLIOrZero[CreateSkillCommand](cmd.Payload))
		return cliResourceMap(item), nil, err
	case events.CommandSkillGet:
		item, err := ReadSkillWithSession(ctx, d.borrow(), cmd.SubjectID, SkillQuery{WorkspaceID: string(cmd.WorkspaceID)})
		return cliResourceMap(item), nil, err
	case events.CommandSkillList:
		items, err := ListSkillsWithSession(ctx, d.borrow(), SkillQuery{WorkspaceID: string(cmd.WorkspaceID)})
		return nil, cliResourceMaps(items), err
	case events.CommandMCPServerCreate:
		item, err := CreateMCPServerWithSession(ctx, d.borrow(), *decodeCLIOrZero[CreateMCPServerCommand](cmd.Payload))
		return cliResourceMap(item), nil, err
	case events.CommandMCPServerGet:
		item, err := ReadMCPServerWithSession(ctx, d.borrow(), cmd.SubjectID)
		return cliResourceMap(item), nil, err
	case events.CommandMCPServerList:
		items, err := ListMCPServersWithSession(ctx, d.borrow())
		return nil, cliResourceMaps(items), err
	case events.CommandMCPServerUpdate:
		item, err := UpdateMCPServerWithSession(ctx, d.borrow(), cmd.SubjectID, *decodeCLIOrZero[UpdateMCPServerCommand](cmd.Payload))
		return cliResourceMap(item), nil, err
	case events.CommandMCPCapabilityList:
		items, err := ListMCPCapabilitiesWithSession(ctx, d.borrow(), MCPCapabilityQuery{})
		return nil, cliResourceMaps(items), err
	case events.CommandMemoryCreate:
		item, err := CreateMemoryItemWithSession(ctx, d.borrow(), string(cmd.WorkspaceID), memoryCommandFromPayload(cmd.Payload))
		return cliResourceMap(item), nil, err
	case events.CommandMemoryGet:
		item, err := ReadMemoryItemWithSession(ctx, d.borrow(), cmd.SubjectID)
		return cliResourceMap(item), nil, err
	case events.CommandMemoryUpdate:
		item, err := UpdateMemoryItemWithSession(ctx, d.borrow(), cmd.SubjectID, memoryCommandFromPayload(cmd.Payload))
		return cliResourceMap(item), nil, err
	case events.CommandMemoryDelete:
		err := DeleteMemoryItemWithSession(ctx, d.borrow(), cmd.SubjectID)
		return nil, nil, err
	case events.CommandMemoryList:
		items, err := SearchMemoryWithSession(ctx, d.borrow(), MemoryQuery{WorkspaceID: string(cmd.WorkspaceID)})
		return nil, cliResourceMaps(items), err
	case events.CommandMemorySearch:
		items, err := SearchMemoryWithSession(ctx, d.borrow(), MemoryQuery{
			WorkspaceID: string(cmd.WorkspaceID),
			Query:       stringValue(cmd.Payload, "query"),
			Tags:        stringSliceValue(cmd.Payload, "tags"),
			Types:       stringSliceValue(cmd.Payload, "types"),
			Limit:       intValue(cmd.Payload, "limit"),
		})
		return nil, cliResourceMaps(items), err
	case events.CommandTemplateCreate:
		item, err := CreateWorkspaceTemplateWithSession(ctx, d.borrow(), *decodeCLIOrZero[WorkspaceTemplate](cmd.Payload))
		return cliResourceMap(item), nil, err
	case events.CommandTemplateGet:
		item, err := d.readWorkspaceTemplate(ctx, cmd.SubjectID)
		return cliResourceMap(item), nil, err
	case events.CommandTemplateList:
		items, err := ListWorkspaceTemplatesWithSession(ctx, d.borrow())
		return nil, cliResourceMaps(items), err
	case events.CommandTemplateUpdate:
		item, err := UpdateWorkspaceTemplateWithSession(ctx, d.borrow(), cmd.SubjectID, *decodeCLIOrZero[WorkspaceTemplate](cmd.Payload))
		return cliResourceMap(item), nil, err
	case events.CommandTemplateDelete:
		err := DeleteWorkspaceTemplateWithSession(ctx, d.borrow(), cmd.SubjectID)
		return nil, nil, err
	case events.CommandConfigurationGet:
		item, err := ReadServiceConfiguration()
		return cliServiceConfigResourceMap(item), nil, err
	default:
		return nil, nil, fmt.Errorf("unsupported CLI command %q", cmd.Command)
	}
}

func (d *CLICommandDispatcher) borrow() *RuntimeSession {
	if d == nil || d.session == nil {
		return nil
	}
	return d.session.Borrow()
}

func (d *CLICommandDispatcher) readWorkspaceTemplate(ctx context.Context, templateID string) (WorkspaceTemplate, error) {
	items, err := ListWorkspaceTemplatesWithSession(ctx, d.borrow())
	if err != nil {
		return WorkspaceTemplate{}, err
	}
	for _, item := range items {
		if item.ID == templateID {
			return item, nil
		}
	}
	return WorkspaceTemplate{}, common.ErrNotFound
}

func cliResourceMap(v any) map[string]any {
	if v == nil {
		return nil
	}
	switch typed := v.(type) {
	case Run:
		return cliRunResourceMap(typed)
	case *Run:
		if typed == nil {
			return nil
		}
		return cliRunResourceMap(*typed)
	case WorkspaceSnapshot:
		return cliWorkspaceSnapshotResourceMap(typed)
	case *WorkspaceSnapshot:
		if typed == nil {
			return nil
		}
		return cliWorkspaceSnapshotResourceMap(*typed)
	}
	return cliResourceMapAlias(v)
}

func cliRunResourceMap(run Run) map[string]any {
	resource := cliResourceMapAlias(run)
	if resource == nil {
		return nil
	}
	if contextMap, ok := resource["executionContext"].(map[string]any); ok {
		resource["executionContext"] = compactCLIExecutionContext(contextMap)
	}
	return resource
}

func cliWorkspaceSnapshotResourceMap(snapshot WorkspaceSnapshot) map[string]any {
	slim := snapshot
	slim.Runs = nil
	resource := cliResourceMapAlias(slim)
	if resource == nil {
		return nil
	}
	runs := make([]map[string]any, 0, len(snapshot.Runs))
	for _, run := range snapshot.Runs {
		runs = append(runs, cliRunResourceMap(run))
	}
	resource["runs"] = runs
	return resource
}

func cliResourceMapAlias(v any) map[string]any {
	data, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		return nil
	}
	return out
}

func cliResourceMaps[T any](items []T) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		out = append(out, cliResourceMap(item))
	}
	return out
}

func cliServiceConfigResourceMap(cfg ServiceConfiguration) map[string]any {
	providers := make([]map[string]any, 0, len(cfg.LLM.Providers))
	for _, item := range cfg.LLM.Providers {
		providers = append(providers, map[string]any{
			"ID":        item.ID,
			"Provider":  item.Provider,
			"BaseURL":   item.BaseURL,
			"Model":     item.Model,
			"Timeout":   item.Timeout,
			"Disabled":  item.Disabled,
			"Streaming": item.Streaming,
		})
	}
	return map[string]any{
		"ServerAddr":        cfg.Server.Addr,
		"Backend":           cfg.Repository.Backend,
		"DataDir":           cfg.Repository.DataDir,
		"DefaultProvider":   cfg.LLM.DefaultProviderID,
		"MaxConcurrentRuns": cfg.Server.MaxConcurrentRuns,
		"Providers":         providers,
		"LLMOutput": map[string]any{
			"MaxPatchContentBytes":               cfg.Runtime.LLMOutput.MaxPatchContentBytes,
			"MaxGraphBytes":                      cfg.Runtime.LLMOutput.MaxGraphBytes,
			"MaxGraphFileWrites":                 cfg.Runtime.LLMOutput.MaxGraphFileWrites,
			"SchemaInvalidLargePreviewThreshold": cfg.Runtime.LLMOutput.SchemaInvalidLargePreviewThreshold,
			"MaxArtifactChunkBytes":              cfg.Runtime.LLMOutput.MaxArtifactChunkBytes,
			"PreviousChunkTailBytes":             cfg.Runtime.LLMOutput.PreviousChunkTailBytes,
			"MaxChunkRetryAttempts":              cfg.Runtime.LLMOutput.MaxChunkRetryAttempts,
			"MaxArtifactPlanAttempts":            cfg.Runtime.LLMOutput.MaxArtifactPlanAttempts,
			"MaxArtifactReplanAttempts":          cfg.Runtime.LLMOutput.MaxArtifactReplanAttempts,
		},
	}
}

func compactCLIExecutionContext(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	ret := map[string]any{}
	for _, key := range []string{
		"goal",
		"goals",
		"dod",
		"dods",
		"operatorInputQueue",
		"latestOperatorInput",
	} {
		if value, ok := input[key]; ok {
			ret[key] = cloneCLIResultValue(value, 0)
		}
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func cloneCLIResultValue(value any, depth int) any {
	if depth >= 6 {
		return nil
	}
	switch typed := value.(type) {
	case map[string]any:
		ret := make(map[string]any, len(typed))
		for key, value := range typed {
			ret[key] = cloneCLIResultValue(value, depth+1)
		}
		return ret
	case []map[string]any:
		ret := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			if cloned, ok := cloneCLIResultValue(item, depth+1).(map[string]any); ok {
				ret = append(ret, cloned)
			}
		}
		return ret
	case []any:
		ret := make([]any, 0, len(typed))
		for _, item := range typed {
			ret = append(ret, cloneCLIResultValue(item, depth+1))
		}
		return ret
	default:
		return value
	}
}

func decodeCLIOrZero[T any](payload map[string]any) *T {
	data, _ := json.Marshal(payload)
	var out T
	_ = json.Unmarshal(data, &out)
	return &out
}

func memoryCommandFromPayload(payload map[string]any) CreateMemoryItemCommand {
	cmd := *decodeCLIOrZero[CreateMemoryItemCommand](payload)
	if cmd.MemoryID == "" {
		cmd.MemoryID = firstStringValue(payload, "memoryId", "ID", "id")
	}
	return cmd
}

func stringValue(payload map[string]any, key string) string {
	if payload == nil {
		return ""
	}
	if v, ok := payload[key].(string); ok {
		return v
	}
	return ""
}

func firstStringValue(payload map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := stringValue(payload, key); value != "" {
			return value
		}
	}
	return ""
}

func boolValue(payload map[string]any, key string) bool {
	if payload == nil {
		return false
	}
	if v, ok := payload[key].(bool); ok {
		return v
	}
	return false
}

func intValue(payload map[string]any, key string) int {
	if payload == nil {
		return 0
	}
	switch v := payload[key].(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	default:
		return 0
	}
}

func mapValue(payload map[string]any, key string) map[string]any {
	if payload == nil {
		return nil
	}
	if v, ok := payload[key].(map[string]any); ok {
		return v
	}
	return nil
}

func firstMapValue(payload map[string]any, keys ...string) map[string]any {
	for _, key := range keys {
		if value := mapValue(payload, key); value != nil {
			return value
		}
	}
	return nil
}

func stringSliceValue(payload map[string]any, key string) []string {
	if payload == nil {
		return nil
	}
	switch v := payload[key].(type) {
	case []string:
		return append([]string(nil), v...)
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}
