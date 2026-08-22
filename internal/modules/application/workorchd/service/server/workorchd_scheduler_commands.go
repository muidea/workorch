package server

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	"github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/recordpayload"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func (s *serverState) admitWorkspaceExecution(ctx context.Context, topic string, command workorchdevents.WorkspaceRunRequestedCommand) (workorchdevents.WorkspaceRunRequestedCommand, error) {
	if s == nil {
		return workorchdevents.WorkspaceRunRequestedCommand{}, common.ErrInvalidArgument
	}
	if s.workspaceExecutionAdmission != nil {
		return s.workspaceExecutionAdmission.AdmitWorkspaceExecution(ctx, topic, command)
	}
	return workorchdevents.WorkspaceRunRequestedCommand{}, fmt.Errorf("%w: workspace execution admission is not configured", common.ErrConflict)
}

func (s *serverState) updateWorkspaceLLMSelection(ctx context.Context, workspaceID string, selection *runtimecontract.ProviderInferenceSelection, expectedRevision uint64) (appmodelcore.Workspace, error) {
	if s != nil && s.updateWorkspaceLLMSelectionPort != nil {
		return s.updateWorkspaceLLMSelectionPort(ctx, workspaceID, selection, expectedRevision)
	}
	return appmodelcore.Workspace{}, fmt.Errorf("%w: workspace LLM selection application port is not configured", common.ErrConflict)
}

func schedulerCommandSource(source string, fallback string) string {
	source = strings.TrimSpace(source)
	if source != "" {
		return source
	}
	fallback = strings.TrimSpace(fallback)
	if fallback != "" {
		return fallback
	}
	return "unknown"
}

// prepareWorkspaceRunSchedulerCommand establishes one immutable request
// correlation for HTTP, TUI and local EventHub scheduler entries.
func prepareWorkspaceRunSchedulerCommand(topic string, command workorchdevents.WorkspaceRunRequestedCommand) workorchdevents.WorkspaceRunRequestedCommand {
	return normalizeWorkspaceExecutionCommandAt(topic, command, time.Now().UTC())
}

// normalizeWorkspaceExecutionCommandAt establishes one immutable request
// correlation using an explicit clock.  Keeping the clock outside the
// normalizer makes request identity reproducible in the application use-case
// tests without making time an authority input.
func normalizeWorkspaceExecutionCommandAt(topic string, command workorchdevents.WorkspaceRunRequestedCommand, now time.Time) workorchdevents.WorkspaceRunRequestedCommand {
	if strings.TrimSpace(command.RequestID) == "" {
		if now.IsZero() {
			now = time.Unix(0, 0).UTC()
		}
		parts := []string{
			"workorchd",
			schedulerKindFromTopic(topic),
			strings.TrimSpace(string(command.WorkspaceID)),
			fmt.Sprintf("%d", now.UTC().UnixNano()),
		}
		for idx, part := range parts {
			if part == "" {
				parts[idx] = "none"
			}
		}
		command.RequestID = strings.Join(parts, "-")
	}
	command.Source = schedulerCommandSource(command.Source, "http")
	return command
}

// NormalizeWorkspaceExecutionCommandAt is the deterministic variant used by
// application-owned execution coordination.  It only fills request
// identity/source and never selects an owner state transition.
func NormalizeWorkspaceExecutionCommandAt(topic string, command workorchdevents.WorkspaceRunRequestedCommand, now time.Time) workorchdevents.WorkspaceRunRequestedCommand {
	return normalizeWorkspaceExecutionCommandAt(topic, command, now)
}

func schedulerAcceptedHTTPBody(config Config, action string, command workorchdevents.WorkspaceRunRequestedCommand) map[string]any {
	workspaceID := strings.TrimSpace(string(command.WorkspaceID))
	body := map[string]any{
		"service":     "workorch",
		"backend":     config.Backend,
		"dataRoot":    config.DataRoot,
		"accepted":    true,
		"workspaceId": workspaceID,
	}
	if value := strings.TrimSpace(action); value != "" {
		body["action"] = value
	}
	for key, value := range map[string]string{
		recordpayload.PayloadKeyIntent:    command.Intent,
		recordpayload.PayloadKeyRequestID: command.RequestID,
		recordpayload.PayloadKeySource:    command.Source,
		recordpayload.PayloadKeyReason:    command.Reason,
	} {
		if value = strings.TrimSpace(value); value != "" {
			body[key] = value
		}
	}
	return body
}

func schedulerKindFromTopic(topic string) string {
	switch topic {
	case workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested:
		return schedulerKindWorkspaceRun
	default:
		return "workorchd_scheduler_command"
	}
}

func (s *serverState) recordSchedulerCommandResult(topic string, result string) {
	if s == nil || s.schedulerDiagnostics == nil {
		return
	}
	s.schedulerDiagnostics.recordCommand(schedulerKindFromTopic(topic), result)
}

// recordWorkspaceRunFailure captures an application activation failure even
// when no Run was created. It is a scheduler/read-model diagnostic only: it
// neither creates a Run nor changes Runtime authority.
func (s *serverState) recordWorkspaceRunFailure(command workorchdevents.WorkspaceRunRequestedCommand, kind string, reason string, cause error) {
	if s == nil || s.schedulerDiagnostics == nil || cause == nil {
		return
	}
	workspaceID := strings.TrimSpace(string(command.WorkspaceID))
	if workspaceID == "" {
		return
	}
	if strings.TrimSpace(kind) == "" {
		kind = schedulerKindWorkspaceActivation
	}
	if strings.TrimSpace(reason) == "" {
		reason = schedulerReasonWorkspaceActivationFailed
	}
	s.schedulerDiagnostics.recordContinuation(schedulerDiagnosticInput{
		WorkspaceID:    workspaceID,
		Kind:           kind,
		State:          schedulerDiagnosticStateFailed,
		Reason:         reason,
		Detail:         cause.Error(),
		Recoverability: schedulerRecoverabilityManualRetry,
		DedupeKey:      schedulerDedupeKey(workspaceID, "", kind, reason),
	})
}

func schedulerStringValue(payload map[string]any, key string) string {
	if payload == nil {
		return ""
	}
	value, _ := payload[key].(string)
	return strings.TrimSpace(value)
}

func schedulerInt(payload map[string]any, key string) int {
	if payload == nil {
		return 0
	}
	switch value := payload[key].(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	default:
		return 0
	}
}

func schedulerMap(payload map[string]any, key string) map[string]any {
	if payload == nil {
		return nil
	}
	if value, ok := payload[key].(map[string]any); ok {
		return value
	}
	return nil
}

func schedulerStringMap(payload map[string]any, key string) map[string]string {
	if payload == nil {
		return nil
	}
	switch value := payload[key].(type) {
	case map[string]string:
		ret := make(map[string]string, len(value))
		for k, v := range value {
			ret[k] = v
		}
		return ret
	case map[string]any:
		ret := make(map[string]string, len(value))
		for k, v := range value {
			if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
				ret[k] = strings.TrimSpace(s)
			}
		}
		if len(ret) > 0 {
			return ret
		}
	}
	return nil
}

func schedulerStringSlice(payload map[string]any, key string) []string {
	if payload == nil {
		return nil
	}
	switch value := payload[key].(type) {
	case []string:
		return append([]string(nil), value...)
	case []any:
		ret := make([]string, 0, len(value))
		for _, item := range value {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				ret = append(ret, strings.TrimSpace(s))
			}
		}
		if len(ret) > 0 {
			return ret
		}
	}
	return nil
}
