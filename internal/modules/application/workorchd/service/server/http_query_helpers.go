package server

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistrycore"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchquerycontract"
)

func eventRecordQueryFromRequest(r *http.Request) workorchquerycontract.EventRecordQuery {
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
	return workorchquerycontract.EventRecordQuery{
		WorkspaceID:    common.WorkspaceID(values.Get("workspaceId")),
		RunID:          common.RunID(values.Get("runId")),
		Topic:          values.Get("topic"),
		ExcludeTopics:  compactQueryValues(values["excludeTopic"]),
		SubjectType:    values.Get("subjectType"),
		SubjectID:      values.Get("subjectId"),
		CursorAfterID:  common.EventRecordID(values.Get("cursorAfterId")),
		OccurredAfter:  values.Get("occurredAfter"),
		OccurredBefore: values.Get("occurredBefore"),
		Limit:          limit,
		IncludePayload: strings.EqualFold(strings.TrimSpace(values.Get("includePayload")), "true"),
	}
}

func runResourcePageQueryFromRequest(r *http.Request) workorchquerycontract.RunResourcePageQuery {
	values := r.URL.Query()
	limit := 0
	if raw := strings.TrimSpace(values.Get("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			limit = parsed
		}
	}
	return workorchquerycontract.RunResourcePageQuery{
		WorkspaceID:   common.WorkspaceID(values.Get("workspaceId")),
		RunID:         common.RunID(values.Get("runId")),
		CursorAfterID: values.Get("cursorAfterId"),
		Limit:         limit,
	}
}

func stepQueryFromRequest(r *http.Request) workorchquerycontract.RunResourcePageQuery {
	return runResourcePageQueryFromRequest(r)
}
func toolCallQueryFromRequest(r *http.Request) workorchquerycontract.RunResourcePageQuery {
	return runResourcePageQueryFromRequest(r)
}
func runtimeCheckpointQueryFromRequest(r *http.Request) workorchquerycontract.RunResourcePageQuery {
	return runResourcePageQueryFromRequest(r)
}
func resultQueryFromRequest(r *http.Request) workorchquerycontract.RunResourcePageQuery {
	return runResourcePageQueryFromRequest(r)
}
func patchQueryFromRequest(r *http.Request) workorchquerycontract.RunResourcePageQuery {
	return runResourcePageQueryFromRequest(r)
}
func validationResultQueryFromRequest(r *http.Request) workorchquerycontract.RunResourcePageQuery {
	return runResourcePageQueryFromRequest(r)
}
func artifactQueryFromRequest(r *http.Request) workorchquerycontract.RunResourcePageQuery {
	return runResourcePageQueryFromRequest(r)
}
func decisionQueryFromRequest(r *http.Request) workorchquerycontract.RunResourcePageQuery {
	return runResourcePageQueryFromRequest(r)
}
func approvalQueryFromRequest(r *http.Request) workorchquerycontract.RunResourcePageQuery {
	return runResourcePageQueryFromRequest(r)
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
