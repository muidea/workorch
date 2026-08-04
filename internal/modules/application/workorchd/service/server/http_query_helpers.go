package server

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistrycore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
)

func runPriorityFromRequest(r *http.Request, bodyPriority int) int {
	if bodyPriority != 0 {
		return bodyPriority
	}
	if r == nil {
		return 0
	}
	for _, raw := range []string{
		r.URL.Query().Get("priority"),
		r.Header.Get("X-Workorch-Run-Priority"),
	} {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		priority, err := strconv.Atoi(strings.TrimSpace(raw))
		if err == nil {
			return priority
		}
	}
	return 0
}

func eventRecordQueryFromRequest(r *http.Request) readmodelcore.EventRecordQuery {
	values := r.URL.Query()
	limit := defaultEventRecordPageLimit
	if raw := strings.TrimSpace(values.Get("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			limit = parsed
		}
	}
	if limit <= 0 {
		limit = defaultEventRecordPageLimit
	}
	if limit > maxEventRecordPageLimit {
		limit = maxEventRecordPageLimit
	}
	return readmodelcore.EventRecordQuery{
		WorkspaceID:    values.Get("workspaceId"),
		RunID:          values.Get("runId"),
		Topic:          values.Get("topic"),
		SubjectType:    values.Get("subjectType"),
		SubjectID:      values.Get("subjectId"),
		CursorAfterID:  values.Get("cursorAfterId"),
		OccurredAfter:  values.Get("occurredAfter"),
		OccurredBefore: values.Get("occurredBefore"),
		Limit:          limit,
		IncludePayload: strings.EqualFold(strings.TrimSpace(values.Get("includePayload")), "true"),
	}
}

func stepQueryFromRequest(r *http.Request) readmodelcore.StepQuery {
	values := r.URL.Query()
	limit := 0
	if raw := strings.TrimSpace(values.Get("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			limit = parsed
		}
	}
	return readmodelcore.StepQuery{
		CursorAfterID: values.Get("cursorAfterId"),
		Limit:         limit,
	}
}

func toolCallQueryFromRequest(r *http.Request) readmodelcore.ToolCallQuery {
	values := r.URL.Query()
	limit := 0
	if raw := strings.TrimSpace(values.Get("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			limit = parsed
		}
	}
	return readmodelcore.ToolCallQuery{
		CursorAfterID: values.Get("cursorAfterId"),
		Limit:         limit,
	}
}

func runtimeCheckpointQueryFromRequest(r *http.Request) readmodelcore.RuntimeCheckpointQuery {
	values := r.URL.Query()
	limit := 0
	if raw := strings.TrimSpace(values.Get("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			limit = parsed
		}
	}
	return readmodelcore.RuntimeCheckpointQuery{
		CursorAfterID: values.Get("cursorAfterId"),
		Limit:         limit,
	}
}

func resultQueryFromRequest(r *http.Request) readmodelcore.ResultQuery {
	values := r.URL.Query()
	limit := 0
	if raw := strings.TrimSpace(values.Get("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			limit = parsed
		}
	}
	return readmodelcore.ResultQuery{
		CursorAfterID: values.Get("cursorAfterId"),
		Limit:         limit,
	}
}

func patchQueryFromRequest(r *http.Request) readmodelcore.PatchQuery {
	values := r.URL.Query()
	limit := 0
	if raw := strings.TrimSpace(values.Get("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			limit = parsed
		}
	}
	return readmodelcore.PatchQuery{
		CursorAfterID: values.Get("cursorAfterId"),
		Limit:         limit,
	}
}

func validationResultQueryFromRequest(r *http.Request) readmodelcore.ValidationResultQuery {
	values := r.URL.Query()
	limit := 0
	if raw := strings.TrimSpace(values.Get("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			limit = parsed
		}
	}
	return readmodelcore.ValidationResultQuery{
		CursorAfterID: values.Get("cursorAfterId"),
		Limit:         limit,
	}
}

func artifactQueryFromRequest(r *http.Request) readmodelcore.ArtifactQuery {
	values := r.URL.Query()
	limit := 0
	if raw := strings.TrimSpace(values.Get("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			limit = parsed
		}
	}
	return readmodelcore.ArtifactQuery{
		CursorAfterID: values.Get("cursorAfterId"),
		Limit:         limit,
	}
}

func decisionQueryFromRequest(r *http.Request) appmodelcore.DecisionQuery {
	values := r.URL.Query()
	limit := 0
	if raw := strings.TrimSpace(values.Get("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			limit = parsed
		}
	}
	return appmodelcore.DecisionQuery{
		WorkspaceID:   values.Get("workspaceId"),
		RunID:         values.Get("runId"),
		CursorAfterID: values.Get("cursorAfterId"),
		Limit:         limit,
	}
}

func approvalQueryFromRequest(r *http.Request) appmodelcore.ApprovalQuery {
	values := r.URL.Query()
	limit := 0
	if raw := strings.TrimSpace(values.Get("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			limit = parsed
		}
	}
	return appmodelcore.ApprovalQuery{
		WorkspaceID:   values.Get("workspaceId"),
		RunID:         values.Get("runId"),
		CursorAfterID: values.Get("cursorAfterId"),
		Limit:         limit,
	}
}

func mcpCapabilityQueryFromRequest(r *http.Request) contextregistrycore.MCPCapabilityQuery {
	values := r.URL.Query()
	var enabled *bool
	if raw := strings.TrimSpace(values.Get("enabled")); raw != "" {
		if parsed, err := strconv.ParseBool(raw); err == nil {
			enabled = &parsed
		}
	}
	return contextregistrycore.MCPCapabilityQuery{
		ServerID:  values.Get("serverId"),
		Kind:      values.Get("kind"),
		Category:  values.Get("category"),
		Stage:     values.Get("stage"),
		RiskLevel: values.Get("riskLevel"),
		Enabled:   enabled,
		Query:     values.Get("q"),
	}
}
