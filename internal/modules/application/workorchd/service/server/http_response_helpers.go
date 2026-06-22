package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice"
	"github.com/muidea/workorch/internal/modules/blocks/pkg/runtimeevent"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
)

func (m *serverMetrics) render(config Config, projection *workspaceProjectionManager) string {
	body := fmt.Sprintf(
		"# TYPE workorch_http_requests_total counter\n"+
			"workorch_http_requests_total{endpoint=\"healthz\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"readyz\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"metrics\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"workspace_create\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"workspace_delete\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"workspace_list\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"workspace_get\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"workspace_snapshot\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"goal_define\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"goal_list\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"goal_get\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"dod_define\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"dod_list\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"dod_get\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"run_create\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"run_list\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"run_get\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"run_snapshot\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"run_status\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"run_observability\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"artifact_content\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"run_task_progress\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"run_context_bundle\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"run_task_decomposition\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"run_start\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"run_resume\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"workspace_execution_activation\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"decision_list\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"decision_get\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"decision_resolve\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"approval_list\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"approval_get\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"approval_approve\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"approval_reject\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"configuration_read\"} %d\n"+
			"workorch_http_requests_total{endpoint=\"configuration_update\"} %d\n"+
			"# TYPE workorch_run_requests_in_flight gauge\n"+
			"workorch_run_requests_in_flight %d\n"+
			"# TYPE workorch_run_queue_waiting gauge\n"+
			"workorch_run_queue_waiting %d\n"+
			"# TYPE workorch_run_queue_total counter\n"+
			"workorch_run_queue_total %d\n"+
			"# TYPE workorch_run_outcomes_total counter\n"+
			"workorch_run_outcomes_total{status=\"succeeded\"} %d\n"+
			"workorch_run_outcomes_total{status=\"failed\"} %d\n"+
			"workorch_run_outcomes_total{status=\"rejected\"} %d\n"+
			"workorch_run_outcomes_total{status=\"canceled\"} %d\n"+
			"# TYPE workorch_server_max_concurrent_runs gauge\n"+
			"workorch_server_max_concurrent_runs %d\n"+
			"# TYPE workorch_stale_dispatched_step_min_age_seconds gauge\n"+
			"workorch_stale_dispatched_step_min_age_seconds %.0f\n",
		m.healthzRequests.Load(),
		m.readyzRequests.Load(),
		m.metricsRequests.Load(),
		m.workspaceCreateRequests.Load(),
		m.workspaceDeleteRequests.Load(),
		m.workspaceListRequests.Load(),
		m.workspaceGetRequests.Load(),
		m.workspaceSnapshotRequests.Load(),
		m.goalDefineRequests.Load(),
		m.goalListRequests.Load(),
		m.goalGetRequests.Load(),
		m.dodDefineRequests.Load(),
		m.dodListRequests.Load(),
		m.dodGetRequests.Load(),
		m.runCreateRequests.Load(),
		m.runListRequests.Load(),
		m.runGetRequests.Load(),
		m.runSnapshotRequests.Load(),
		m.runStatusRequests.Load(),
		m.runObservabilityRequests.Load(),
		m.artifactContentRequests.Load(),
		m.runTaskProgressRequests.Load(),
		m.runContextBundleRequests.Load(),
		m.runTaskDecompositionRequests.Load(),
		m.runStartRequests.Load(),
		m.runResumeRequests.Load(),
		m.workspaceActivationRequests.Load(),
		m.decisionListRequests.Load(),
		m.decisionGetRequests.Load(),
		m.decisionResolveRequests.Load(),
		m.approvalListRequests.Load(),
		m.approvalGetRequests.Load(),
		m.approvalApproveRequests.Load(),
		m.approvalRejectRequests.Load(),
		m.configurationReadRequests.Load(),
		m.configurationUpdateRequests.Load(),
		m.runInFlight.Load(),
		m.runQueueWaiting.Load(),
		m.runQueuedTotal.Load(),
		m.runSucceeded.Load(),
		m.runFailed.Load(),
		m.runRejected.Load(),
		m.runCanceled.Load(),
		config.MaxConcurrentRuns,
		config.StaleDispatchedStepMinAge.Seconds(),
	)
	if m != nil {
		body += m.schedulerDiagnostics.renderMetrics()
	}
	return body + renderEventRecordIOMetrics() + renderRuntimeEventPublishMetrics() + renderWorkspaceProjectionMetrics(projection)
}

func renderEventRecordIOMetrics() string {
	metrics := repository.EventRecordIOMetricsSnapshot()
	return fmt.Sprintf(
		"# TYPE workorch_event_record_metadata_scans_total counter\n"+
			"workorch_event_record_metadata_scans_total %d\n"+
			"# TYPE workorch_event_record_metadata_records_scanned_total counter\n"+
			"workorch_event_record_metadata_records_scanned_total %d\n"+
			"# TYPE workorch_event_record_payload_hydrates_total counter\n"+
			"workorch_event_record_payload_hydrates_total %d\n"+
			"# TYPE workorch_event_record_payload_hydrate_bytes_total counter\n"+
			"workorch_event_record_payload_hydrate_bytes_total %d\n",
		metrics.MetadataScansTotal,
		metrics.MetadataRecordsScannedTotal,
		metrics.PayloadHydratesTotal,
		metrics.PayloadHydrateBytesTotal,
	)
}

func renderRuntimeEventPublishMetrics() string {
	metrics := runtimeevent.RuntimeEventPublishMetricsSnapshot()
	return fmt.Sprintf(
		"# TYPE workorch_runtime_event_publish_failures_total counter\n"+
			"workorch_runtime_event_publish_failures_total{category=\"accepted\"} %d\n"+
			"workorch_runtime_event_publish_failures_total{category=\"terminal\"} %d\n"+
			"workorch_runtime_event_publish_failures_total{category=\"progress\"} %d\n"+
			"workorch_runtime_event_publish_failures_total{category=\"worker_lifecycle\"} %d\n"+
			"workorch_runtime_event_publish_failures_total{category=\"worker_lost\"} %d\n"+
			"# TYPE workorch_runtime_heartbeat_timer_start_failures_total counter\n"+
			"workorch_runtime_heartbeat_timer_start_failures_total %d\n",
		metrics.AcceptedPublishFailuresTotal,
		metrics.TerminalPublishFailuresTotal,
		metrics.ProgressPublishFailuresTotal,
		metrics.WorkerLifecycleFailuresTotal,
		metrics.WorkerLostPublishFailuresTotal,
		metrics.HeartbeatTimerStartFailuresTotal,
	)
}

func renderWorkspaceProjectionMetrics(projection *workspaceProjectionManager) string {
	var metrics workspaceProjectionMetrics
	if projection != nil {
		metrics = projection.Metrics()
	}
	var b strings.Builder
	b.WriteString("# TYPE workorch_workspace_projection_rebuild_total counter\n")
	b.WriteString(fmt.Sprintf("workorch_workspace_projection_rebuild_total{scope=\"full\"} %d\n", metrics.FullRebuildTotal))
	for _, item := range metrics.WorkspaceRebuilds {
		b.WriteString(fmt.Sprintf("workorch_workspace_projection_rebuild_total{scope=\"workspace\",workspace=\"%s\"} %d\n", prometheusLabelValue(item.WorkspaceID), item.RebuildTotal))
	}
	b.WriteString("# TYPE workorch_workspace_projection_rebuild_failed_total counter\n")
	b.WriteString(fmt.Sprintf("workorch_workspace_projection_rebuild_failed_total{scope=\"full\"} %d\n", metrics.FullRebuildFailedTotal))
	for _, item := range metrics.WorkspaceRebuilds {
		b.WriteString(fmt.Sprintf("workorch_workspace_projection_rebuild_failed_total{scope=\"workspace\",workspace=\"%s\"} %d\n", prometheusLabelValue(item.WorkspaceID), item.FailedTotal))
	}
	b.WriteString("# TYPE workorch_workspace_projection_rebuild_duration_ms_total counter\n")
	b.WriteString(fmt.Sprintf("workorch_workspace_projection_rebuild_duration_ms_total{scope=\"full\"} %d\n", metrics.FullRebuildDurationMsTotal))
	for _, item := range metrics.WorkspaceRebuilds {
		b.WriteString(fmt.Sprintf("workorch_workspace_projection_rebuild_duration_ms_total{scope=\"workspace\",workspace=\"%s\"} %d\n", prometheusLabelValue(item.WorkspaceID), item.DurationMsTotal))
	}
	b.WriteString("# TYPE workorch_workspace_projection_pending_rebuilds gauge\n")
	b.WriteString(fmt.Sprintf("workorch_workspace_projection_pending_rebuilds %d\n", metrics.PendingWorkspaceRebuilds))
	b.WriteString("# TYPE workorch_workspace_projection_debounced_rebuild_total counter\n")
	b.WriteString(fmt.Sprintf("workorch_workspace_projection_debounced_rebuild_total %d\n", metrics.WorkspaceRebuildDebouncedTotal))
	b.WriteString("# TYPE workorch_workspace_projection_schedule_dropped_total counter\n")
	b.WriteString(fmt.Sprintf("workorch_workspace_projection_schedule_dropped_total %d\n", metrics.ScheduleDroppedTotal))
	b.WriteString("# TYPE workorch_workspace_projection_active_run_heartbeats gauge\n")
	b.WriteString(fmt.Sprintf("workorch_workspace_projection_active_run_heartbeats %d\n", metrics.ActiveRunHeartbeatTotal))
	b.WriteString("# TYPE workorch_workspace_projection_active_run_heartbeat_age_seconds_max gauge\n")
	b.WriteString(fmt.Sprintf("workorch_workspace_projection_active_run_heartbeat_age_seconds_max %d\n", metrics.ActiveRunHeartbeatAgeSecondsMax))
	b.WriteString("# TYPE workorch_workspace_projection_stalled_runs gauge\n")
	b.WriteString(fmt.Sprintf("workorch_workspace_projection_stalled_runs %d\n", metrics.StalledRunTotal))
	b.WriteString("# TYPE workorch_workspace_projection_ineffective_retry_runs gauge\n")
	b.WriteString(fmt.Sprintf("workorch_workspace_projection_ineffective_retry_runs %d\n", metrics.IneffectiveRetryRunTotal))
	b.WriteString("# TYPE workorch_convergence_active_slices gauge\n")
	b.WriteString(fmt.Sprintf("workorch_convergence_active_slices %d\n", metrics.ConvergenceActiveSlices))
	b.WriteString("# TYPE workorch_convergence_open_focuses gauge\n")
	b.WriteString(fmt.Sprintf("workorch_convergence_open_focuses %d\n", metrics.ConvergenceOpenFocuses))
	b.WriteString("# TYPE workorch_convergence_repair_attempts_total counter\n")
	b.WriteString(fmt.Sprintf("workorch_convergence_repair_attempts_total %d\n", metrics.ConvergenceRepairAttemptsTotal))
	b.WriteString("# TYPE workorch_convergence_no_delta_rounds_total counter\n")
	b.WriteString(fmt.Sprintf("workorch_convergence_no_delta_rounds_total %d\n", metrics.ConvergenceNoDeltaRoundsTotal))
	b.WriteString("# TYPE workorch_convergence_budget_escalations_total counter\n")
	b.WriteString(fmt.Sprintf("workorch_convergence_budget_escalations_total %d\n", metrics.ConvergenceBudgetEscalations))
	return b.String()
}

func prometheusLabelValue(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	value = strings.ReplaceAll(value, "\n", `\n`)
	return value
}

func resourceIDFromPath(path string, prefix string) (string, bool) {
	if !strings.HasPrefix(path, prefix) {
		return "", false
	}
	trimmed := strings.TrimPrefix(path, prefix)
	if trimmed == "" || strings.Contains(trimmed, "/") {
		return "", false
	}
	return trimmed, true
}

func memoryQueryFromValues(values url.Values) appservice.MemoryQuery {
	limit, _ := strconv.Atoi(strings.TrimSpace(values.Get("limit")))
	return appservice.MemoryQuery{
		WorkspaceID: strings.TrimSpace(values.Get("workspaceId")),
		Query:       strings.TrimSpace(values.Get("q")),
		Tags:        compactQueryValues(values["tag"]),
		Types:       compactQueryValues(values["type"]),
		Limit:       limit,
	}
}

func memoryRecallTraceQueryFromValues(values url.Values) appservice.MemoryRecallTraceQuery {
	limit, _ := strconv.Atoi(strings.TrimSpace(values.Get("limit")))
	return appservice.MemoryRecallTraceQuery{
		WorkspaceID: strings.TrimSpace(values.Get("workspaceId")),
		RunID:       strings.TrimSpace(values.Get("runId")),
		StepID:      strings.TrimSpace(values.Get("stepId")),
		Stage:       strings.TrimSpace(values.Get("stage")),
		Source:      strings.TrimSpace(values.Get("source")),
		Status:      strings.TrimSpace(values.Get("status")),
		Limit:       limit,
	}
}

func compactQueryValues(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	ret := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		ret = append(ret, value)
	}
	return ret
}

func workspaceActionFromPath(path string) (workspaceID string, action string, ok bool) {
	if !strings.HasPrefix(path, "/api/v1/workorch/workspaces/") {
		return "", "", false
	}
	trimmed := strings.TrimPrefix(path, "/api/v1/workorch/workspaces/")
	if trimmed == "" {
		return "", "", false
	}
	parts := strings.Split(trimmed, "/")
	if len(parts) == 1 && parts[0] != "" {
		return parts[0], "", true
	}
	if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
		return parts[0], parts[1], true
	}
	return "", "", false
}

func workspaceResourceIDFromPath(path string, resource string) (workspaceID string, resourceID string, ok bool) {
	if !strings.HasPrefix(path, "/api/v1/workorch/workspaces/") {
		return "", "", false
	}
	trimmed := strings.TrimPrefix(path, "/api/v1/workorch/workspaces/")
	if trimmed == "" {
		return "", "", false
	}
	parts := strings.Split(trimmed, "/")
	if len(parts) == 3 && parts[0] != "" && parts[1] == resource && parts[2] != "" {
		return parts[0], parts[2], true
	}
	return "", "", false
}

func workspaceRunActionFromPath(path string) (workspaceID string, runID string, action string, ok bool) {
	if !strings.HasPrefix(path, "/api/v1/workorch/workspaces/") {
		return "", "", "", false
	}
	trimmed := strings.TrimPrefix(path, "/api/v1/workorch/workspaces/")
	if trimmed == "" {
		return "", "", "", false
	}
	parts := strings.Split(trimmed, "/")
	if len(parts) == 3 && parts[0] != "" && parts[1] == "runs" && parts[2] != "" {
		return parts[0], parts[2], "", true
	}
	if len(parts) == 4 && parts[0] != "" && parts[1] == "runs" && parts[2] != "" && parts[3] != "" {
		return parts[0], parts[2], parts[3], true
	}
	return "", "", "", false
}

func decisionActionFromPath(path string) (ticketID string, action string, ok bool) {
	if !strings.HasPrefix(path, "/api/v1/workorch/decisions/") {
		return "", "", false
	}
	trimmed := strings.TrimPrefix(path, "/api/v1/workorch/decisions/")
	if trimmed == "" {
		return "", "", false
	}
	parts := strings.Split(trimmed, "/")
	if len(parts) == 1 && parts[0] != "" {
		return parts[0], "", true
	}
	if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
		return parts[0], parts[1], true
	}
	return "", "", false
}

func approvalActionFromPath(path string) (approvalID string, action string, ok bool) {
	if !strings.HasPrefix(path, "/api/v1/workorch/approvals/") {
		return "", "", false
	}
	trimmed := strings.TrimPrefix(path, "/api/v1/workorch/approvals/")
	if trimmed == "" {
		return "", "", false
	}
	parts := strings.Split(trimmed, "/")
	if len(parts) == 1 && parts[0] != "" {
		return parts[0], "", true
	}
	if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
		return parts[0], parts[1], true
	}
	return "", "", false
}

func mcpServerActionFromPath(path string) (serverID string, action string, ok bool) {
	if !strings.HasPrefix(path, "/api/v1/workorch/mcp/servers/") {
		return "", "", false
	}
	trimmed := strings.TrimPrefix(path, "/api/v1/workorch/mcp/servers/")
	if trimmed == "" {
		return "", "", false
	}
	parts := strings.Split(trimmed, "/")
	if len(parts) == 1 && parts[0] != "" {
		return parts[0], "", true
	}
	if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
		return parts[0], parts[1], true
	}
	return "", "", false
}

func durationFromEnv(name string, fallback time.Duration) (time.Duration, error) {
	cfg, err := workorchconfig.Current()
	if err != nil {
		return 0, err
	}
	serverConfig, err := ConfigFromWorkorch(cfg)
	if err != nil {
		return 0, err
	}
	switch name {
	case configServerReadHeaderTimeout:
		return serverConfig.ReadHeaderTimeout, nil
	case configServerReadTimeout:
		return serverConfig.ReadTimeout, nil
	case configServerWriteTimeout:
		return serverConfig.WriteTimeout, nil
	case configServerIdleTimeout:
		return serverConfig.IdleTimeout, nil
	case configServerShutdownTimeout:
		return serverConfig.ShutdownTimeout, nil
	case configServerStaleDispatchedStepMinAge:
		return serverConfig.StaleDispatchedStepMinAge, nil
	default:
		return fallback, nil
	}
}

func durationFromValues(values map[string]string, name string, fallback time.Duration) (time.Duration, error) {
	return durationFromValue(name, values[name], fallback)
}

func durationFromValue(name string, value string, fallback time.Duration) (time.Duration, error) {
	if value == "" {
		return fallback, nil
	}
	duration, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("invalid duration for %s: %w", name, err)
	}
	return duration, nil
}

func int64FromEnv(name string, fallback int64) (int64, error) {
	cfg, err := workorchconfig.Current()
	if err != nil {
		return 0, err
	}
	if name == configServerMaxConcurrentRuns {
		serverConfig, err := ConfigFromWorkorch(cfg)
		if err != nil {
			return 0, err
		}
		return serverConfig.MaxConcurrentRuns, nil
	}
	return fallback, nil
}

func int64FromValues(values map[string]string, name string, fallback int64) (int64, error) {
	value := values[name]
	if value == "" {
		return fallback, nil
	}
	number, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid integer for %s: %w", name, err)
	}
	if number <= 0 {
		return 0, fmt.Errorf("invalid integer for %s: must be positive", name)
	}
	return number, nil
}

func firstNonEmpty(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func writeMethodNotAllowed(w http.ResponseWriter) {
	writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "method not allowed"})
}

func writeBusinessError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, common.ErrInvalidArgument):
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
	case errors.Is(err, common.ErrBlocked):
		writeJSON(w, http.StatusConflict, errorResponse{Error: err.Error()})
	case errors.Is(err, common.ErrConflict):
		writeJSON(w, http.StatusConflict, errorResponse{Error: err.Error()})
	case errors.Is(err, common.ErrNotFound):
		writeJSON(w, http.StatusNotFound, errorResponse{Error: err.Error()})
	default:
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
	}
}

func writeArtifactContent(w http.ResponseWriter, item appservice.ArtifactContent) {
	contentType := strings.TrimSpace(item.ContentType)
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	fileName := strings.TrimSpace(item.FileName)
	if fileName == "" {
		fileName = "artifact"
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": fileName}))
	w.Header().Set("X-Workorch-Artifact-URI", item.URI)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(item.Content)
}

func writeRuntimeRawContent(w http.ResponseWriter, item appservice.RuntimeRawContent) {
	contentType := strings.TrimSpace(item.ContentType)
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	fileName := strings.TrimSpace(item.FileName)
	if fileName == "" {
		fileName = "payload"
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": fileName}))
	w.Header().Set("X-Workorch-Raw-Ref", item.Ref)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(item.Content)
}

func writeRunQueueError(w http.ResponseWriter, err error) {
	writeJSON(w, http.StatusRequestTimeout, errorResponse{
		Error: fmt.Sprintf("run execution request canceled while waiting for available execution slot: %v", err),
	})
}

func decodeJSON(r *http.Request, dst any) error {
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return fmt.Errorf("invalid request body: %w", err)
	}
	return nil
}

func decodeOptionalJSON(r *http.Request, dst any) error {
	if r.Body == nil || r.ContentLength == 0 {
		return nil
	}
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return fmt.Errorf("invalid request body: %w", err)
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	_ = encoder.Encode(payload)
}
