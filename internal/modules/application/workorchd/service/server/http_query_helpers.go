package server

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice"
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

func eventRecordQueryFromRequest(r *http.Request) appservice.EventRecordQuery {
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
	return appservice.EventRecordQuery{
		WorkspaceID:    values.Get("workspaceId"),
		RunID:          values.Get("runId"),
		Topic:          values.Get("topic"),
		SubjectType:    values.Get("subjectType"),
		SubjectID:      values.Get("subjectId"),
		CursorAfterID:  values.Get("cursorAfterId"),
		OccurredAfter:  values.Get("occurredAfter"),
		OccurredBefore: values.Get("occurredBefore"),
		Limit:          limit,
	}
}

func stepQueryFromRequest(r *http.Request) appservice.StepQuery {
	values := r.URL.Query()
	limit := 0
	if raw := strings.TrimSpace(values.Get("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			limit = parsed
		}
	}
	return appservice.StepQuery{
		CursorAfterID: values.Get("cursorAfterId"),
		Limit:         limit,
	}
}

func toolCallQueryFromRequest(r *http.Request) appservice.ToolCallQuery {
	values := r.URL.Query()
	limit := 0
	if raw := strings.TrimSpace(values.Get("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			limit = parsed
		}
	}
	return appservice.ToolCallQuery{
		CursorAfterID: values.Get("cursorAfterId"),
		Limit:         limit,
	}
}

func runtimeCheckpointQueryFromRequest(r *http.Request) appservice.RuntimeCheckpointQuery {
	values := r.URL.Query()
	limit := 0
	if raw := strings.TrimSpace(values.Get("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			limit = parsed
		}
	}
	return appservice.RuntimeCheckpointQuery{
		CursorAfterID: values.Get("cursorAfterId"),
		Limit:         limit,
	}
}

func resultQueryFromRequest(r *http.Request) appservice.ResultQuery {
	values := r.URL.Query()
	limit := 0
	if raw := strings.TrimSpace(values.Get("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			limit = parsed
		}
	}
	return appservice.ResultQuery{
		CursorAfterID: values.Get("cursorAfterId"),
		Limit:         limit,
	}
}

func patchQueryFromRequest(r *http.Request) appservice.PatchQuery {
	values := r.URL.Query()
	limit := 0
	if raw := strings.TrimSpace(values.Get("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			limit = parsed
		}
	}
	return appservice.PatchQuery{
		CursorAfterID: values.Get("cursorAfterId"),
		Limit:         limit,
	}
}

func validationResultQueryFromRequest(r *http.Request) appservice.ValidationResultQuery {
	values := r.URL.Query()
	limit := 0
	if raw := strings.TrimSpace(values.Get("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			limit = parsed
		}
	}
	return appservice.ValidationResultQuery{
		CursorAfterID: values.Get("cursorAfterId"),
		Limit:         limit,
	}
}

func artifactQueryFromRequest(r *http.Request) appservice.ArtifactQuery {
	values := r.URL.Query()
	limit := 0
	if raw := strings.TrimSpace(values.Get("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			limit = parsed
		}
	}
	return appservice.ArtifactQuery{
		CursorAfterID: values.Get("cursorAfterId"),
		Limit:         limit,
	}
}

func decisionQueryFromRequest(r *http.Request) appservice.DecisionQuery {
	values := r.URL.Query()
	limit := 0
	if raw := strings.TrimSpace(values.Get("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			limit = parsed
		}
	}
	return appservice.DecisionQuery{
		WorkspaceID:   values.Get("workspaceId"),
		RunID:         values.Get("runId"),
		CursorAfterID: values.Get("cursorAfterId"),
		Limit:         limit,
	}
}

func approvalQueryFromRequest(r *http.Request) appservice.ApprovalQuery {
	values := r.URL.Query()
	limit := 0
	if raw := strings.TrimSpace(values.Get("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			limit = parsed
		}
	}
	return appservice.ApprovalQuery{
		WorkspaceID:   values.Get("workspaceId"),
		RunID:         values.Get("runId"),
		CursorAfterID: values.Get("cursorAfterId"),
		Limit:         limit,
	}
}

func mcpCapabilityQueryFromRequest(r *http.Request) appservice.MCPCapabilityQuery {
	values := r.URL.Query()
	var enabled *bool
	if raw := strings.TrimSpace(values.Get("enabled")); raw != "" {
		if parsed, err := strconv.ParseBool(raw); err == nil {
			enabled = &parsed
		}
	}
	return appservice.MCPCapabilityQuery{
		ServerID:  values.Get("serverId"),
		Kind:      values.Get("kind"),
		Category:  values.Get("category"),
		Stage:     values.Get("stage"),
		RiskLevel: values.Get("riskLevel"),
		Enabled:   enabled,
		Query:     values.Get("q"),
	}
}
