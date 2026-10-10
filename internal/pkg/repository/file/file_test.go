package file

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	eventstreammodel "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/models"
	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	runtimeevidencemodel "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	deletioncontract "github.com/muidea/workorch/internal/pkg/workorchdeletioncontract"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	"github.com/muidea/workorch/internal/pkg/workorchquerycontract"
	orchestratormodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func mustFileRepositoryPayload(t *testing.T, input map[string]any) workorcheventpayload.Payload {
	t.Helper()
	payload, err := workorcheventpayload.FromMap(input)
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

func fileEventStreamPurgeFence(workspaceID common.WorkspaceID) eventstreammodel.WorkspaceDeletion {
	scope := deletioncontract.Scope{
		WorkspaceID: workspaceID, DeletionID: "delete-" + string(workspaceID), DeletionGeneration: 1,
		Mode: deletioncontract.ModePurge, QuiescenceID: "quiesced-" + string(workspaceID),
	}
	scope.RequestHash = deletioncontract.StableRequestHash(scope)
	now := time.Date(2026, 8, 26, 0, 0, 0, 0, time.UTC).Format(time.RFC3339Nano)
	return eventstreammodel.WorkspaceDeletion{
		WorkspaceID: scope.WorkspaceID, DeletionID: scope.DeletionID, DeletionGeneration: scope.DeletionGeneration,
		Mode: scope.Mode, QuiescenceID: scope.QuiescenceID, RequestHash: scope.RequestHash,
		FenceID: "fence-" + string(workspaceID), Phase: eventstreammodel.WorkspaceDeletionPhaseFenced,
		Revision: 1, SealAckHash: "seal-" + string(workspaceID), RequestedAt: now, UpdatedAt: now,
	}
}

func TestWorkspaceRepositoryGetReturnsErrorWhenPersistedJSONCorrupt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "workspace.json")
	corrupt := []byte("{not-json")
	if err := os.WriteFile(path, corrupt, 0o644); err != nil {
		t.Fatalf("write corrupt workspace file: %v", err)
	}

	repo := NewWorkspaceRepository(path)
	if _, err := repo.Get(context.Background(), "ws-corrupt"); err == nil {
		t.Fatal("Get err = nil, want json error")
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read workspace file after corrupt load: %v", err)
	}
	if string(after) != string(corrupt) {
		t.Fatalf("workspace file changed after corrupt load: got %q want %q", string(after), string(corrupt))
	}
}

func TestWorkspaceRepositoryRejectsRetiredOrUnknownFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "workspace.json")
	data := []byte(`{"ws-retired":{"ID":"ws-retired","State":"active","retiredSchemaVersion":"v1"}}`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewWorkspaceRepository(path).Get(context.Background(), "ws-retired"); err == nil || !strings.Contains(err.Error(), `unknown field "retiredSchemaVersion"`) {
		t.Fatalf("Get error = %v, want strict retired-field rejection", err)
	}
}

func TestWorkspaceRepositoryPersistsAndClonesDefaultLLMSelection(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "workspace.json")
	repo := NewWorkspaceRepository(path)
	target := orchestratormodel.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "model-a"}
	item := &workspacemodel.Workspace{ID: "ws-model-target", Title: "workspace", State: common.WorkspaceStateActive, DefaultLLMSelection: &orchestratormodel.ProviderInferenceSelection{Target: target, ReasoningEffort: "high"}}
	if err := repo.Create(ctx, item); err != nil {
		t.Fatalf("Create: %v", err)
	}
	item.DefaultLLMSelection.Target.ModelID = "mutated"
	stored, err := repo.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if stored.DefaultLLMSelection == nil || stored.DefaultLLMSelection.Target.ProviderID != "ai-proxy" || stored.DefaultLLMSelection.Target.ModelID != "model-a" {
		t.Fatalf("stored target = %+v", stored.DefaultLLMSelection)
	}
	stored.DefaultLLMSelection.Target.ProviderID = "mutated"
	again, err := NewWorkspaceRepository(path).Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("second Get: %v", err)
	}
	if again.DefaultLLMSelection == nil || again.DefaultLLMSelection.Target.ProviderID != "ai-proxy" {
		t.Fatalf("cloned target = %+v", again.DefaultLLMSelection)
	}
}

func TestWorkspaceRepositoryNeverPersistsTransientAuthoringSummary(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "workspace.json")
	repo := NewWorkspaceRepository(path)
	summary := &workspacemodel.AuthoringSummary{State: "failed", FailurePhase: "proposal", FailureCode: "stale", ReviewCount: 2, InputRevision: 4}
	item := &workspacemodel.Workspace{ID: "ws-transient-summary", Title: "workspace", State: common.WorkspaceStateActive, AuthoringSummary: summary}
	if err := repo.Create(ctx, item); err != nil {
		t.Fatal(err)
	}
	if item.AuthoringSummary == nil {
		t.Fatal("repository unexpectedly mutated the caller's transient summary")
	}
	stored, err := repo.Get(ctx, item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.AuthoringSummary != nil {
		t.Fatalf("file repository restored transient summary: %+v", stored.AuthoringSummary)
	}
	wire, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(wire), "authoringSummary") || strings.Contains(string(wire), "stale") {
		t.Fatalf("file repository persisted transient summary: %s", wire)
	}
}

func TestWorkspaceRepositoryPersistsOptionalDefaultLLMSelectionAsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "workspace.json")
	repo := NewWorkspaceRepository(path)
	if err := repo.Create(context.Background(), &workspacemodel.Workspace{ID: "ws-current", Title: "current", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("create current workspace: %v", err)
	}
	item, err := repo.Get(context.Background(), "ws-current")
	if err != nil {
		t.Fatalf("read current workspace: %v", err)
	}
	if item.DefaultLLMSelection != nil {
		t.Fatalf("optional target = %+v, want nil", item.DefaultLLMSelection)
	}
}

func TestWorkspaceRepositoryRejectsNullContextHints(t *testing.T) {
	path := filepath.Join(t.TempDir(), "workspace.json")
	data := []byte(`{"ws-invalid-null":{"Revision":1,"ID":"ws-invalid-null","Title":"invalid null","State":"active","ContextHints":null,"EvidencePolicies":null}}`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write invalid workspace: %v", err)
	}
	if _, err := NewWorkspaceRepository(path).Get(context.Background(), "ws-invalid-null"); !errors.Is(err, workorcheventpayload.ErrInvalidPayload) {
		t.Fatalf("Get error = %v, want invalid payload", err)
	}
}

func TestWorkspaceRepositoryRejectsNonObjectContextHints(t *testing.T) {
	path := filepath.Join(t.TempDir(), "workspace.json")
	if err := os.WriteFile(path, []byte(`{"ws-invalid":{"ID":"ws-invalid","ContextHints":[]}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewWorkspaceRepository(path).Get(context.Background(), "ws-invalid"); !errors.Is(err, workorcheventpayload.ErrInvalidPayload) {
		t.Fatalf("Get error = %v, want invalid payload", err)
	}
}

func TestResultRepositoryRejectsNullSummaryDiagnostics(t *testing.T) {
	root := filepath.Join(t.TempDir(), "results")
	repo := NewResultRepository(root)
	resultID := common.ResultID("run-invalid/result-1")
	path := repo.resultPath(resultID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	data := []byte(`{"Revision":1,"ID":"run-invalid/result-1","WorkspaceID":"ws-invalid","RunID":"run-invalid","State":"accepted","SummaryDetail":{"GoalOutcome":"accepted","Diagnostics":null}}`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := repo.ListForRecovery(context.Background(), "", 10); !errors.Is(err, workorcheventpayload.ErrInvalidPayload) {
		t.Fatalf("ListForRecovery error = %v, want invalid payload", err)
	}
}

func TestResultRepositoryRejectsNonObjectSummaryDiagnostics(t *testing.T) {
	root := filepath.Join(t.TempDir(), "results")
	repo := NewResultRepository(root)
	path := repo.resultPath("run-invalid/result-1")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"ID":"run-invalid/result-1","SummaryDetail":{"Diagnostics":"invalid"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Get(context.Background(), "run-invalid/result-1"); !errors.Is(err, workorcheventpayload.ErrInvalidPayload) {
		t.Fatalf("Get error = %v, want invalid payload", err)
	}
}

func TestRunRepositoryCreateReturnsErrorWhenPersistedJSONCorrupt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "run.json")
	corrupt := []byte("{not-json")
	if err := os.WriteFile(path, corrupt, 0o644); err != nil {
		t.Fatalf("write corrupt run file: %v", err)
	}

	repo := NewRunRepository(path)
	err := repo.Create(context.Background(), &runmodel.Run{
		ID:          common.RunID("run-corrupt"),
		WorkspaceID: common.WorkspaceID("ws-corrupt"),
		State:       common.RunStatePending,
	})
	if err == nil {
		t.Fatal("Create err = nil, want json error")
	}
	if errors.Is(err, common.ErrConflict) || errors.Is(err, common.ErrNotFound) {
		t.Fatalf("Create err = %v, want persisted json error", err)
	}

	after, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("read run file after corrupt create: %v", readErr)
	}
	if string(after) != string(corrupt) {
		t.Fatalf("run file changed after corrupt create: got %q want %q", string(after), string(corrupt))
	}
}

func TestRunRepositoryFreshLoadRejectsAmbiguousBootstrapAuthorityWithoutMutation(t *testing.T) {
	policy := orchestratormodel.RuntimeEvidencePolicy{
		ID: "policy-1", AppliesTo: orchestratormodel.StepKindFileWrite,
		CoverageRequirements: []orchestratormodel.RuntimeCoverageEvidenceRequirement{{ID: "dod-1", Scope: "dodItem"}},
	}.Normalized()
	tests := map[string][]orchestratormodel.RuntimeEvidencePolicy{
		"duplicate policy": {policy, policy},
		"duplicate nested requirement": {func() orchestratormodel.RuntimeEvidencePolicy {
			value := policy
			value.CoverageRequirements = append(value.CoverageRequirements, value.CoverageRequirements[0])
			return value
		}()},
		"duplicate nested check ref": {func() orchestratormodel.RuntimeEvidencePolicy {
			value := policy
			value.CoverageRequirements[0].RequiredEvidenceKinds = []string{orchestratormodel.EvidenceKindValidation}
			value.CoverageRequirements[0].ValidationRecipeID = "recipe-1"
			value.CoverageRequirements[0].RequiredCheckRefs = []string{"check-1", "check-1"}
			return value
		}()},
		"whitespace nested target": {func() orchestratormodel.RuntimeEvidencePolicy {
			value := policy
			value.CoverageRequirements[0].TargetRefs = []string{" target.go "}
			return value
		}()},
	}
	for name, policies := range tests {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "run.json")
			bootstrap := orchestratormodel.RuntimeBootstrap{Activation: orchestratormodel.RuntimeActivationContext{EvidencePolicies: policies}}
			item := runmodel.Run{ID: "run-raw-bootstrap", WorkspaceID: "ws-raw-bootstrap", State: common.RunStatePending, RuntimeBootstrap: &bootstrap}
			repo := NewRunRepository(path)
			if err := repo.Create(context.Background(), &item); err == nil || !strings.Contains(err.Error(), "evidence polic") {
				t.Fatalf("file Run repository accepted duplicate bootstrap authority on write: %v", err)
			}
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatalf("rejected Run write created persistence file: %v", err)
			}
			data, err := json.Marshal(map[common.RunID]runmodel.Run{item.ID: item})
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, data, 0o600); err != nil {
				t.Fatal(err)
			}

			if _, err := repo.Get(context.Background(), item.ID); err == nil || !strings.Contains(err.Error(), "evidence polic") {
				t.Fatalf("fresh Run load accepted duplicate bootstrap authority: %v", err)
			}
			after, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(after, data) {
				t.Fatal("invalid persisted Run was rewritten during failed fresh load")
			}
		})
	}
}

func TestWorkspaceRepositoryRejectsEmptyPersistedFileWithoutMutation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "workspace.json")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatalf("write empty workspace file: %v", err)
	}

	repo := NewWorkspaceRepository(path)
	err := repo.Create(context.Background(), &workspacemodel.Workspace{
		ID:    common.WorkspaceID("ws-empty"),
		Title: "empty workspace",
		State: common.WorkspaceStateActive,
	})
	if err == nil {
		t.Fatal("Create error = nil, want strict empty-file rejection")
	}
	data, readErr := os.ReadFile(path)
	if readErr != nil || len(data) != 0 {
		t.Fatalf("persisted file changed: bytes=%d err=%v", len(data), readErr)
	}
}

func TestEventRecordRepositoryAppendsJSONLSegmentsWithoutMapRewrite(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "event-records")
	repo := NewEventRecordRepository(root)
	payload := strings.Repeat("x", eventRecordPayloadInlineThreshold+1)

	if err := repo.Append(ctx, &eventstreammodel.EventRecord{
		ID:          common.EventRecordID("event-jsonl-1"),
		Topic:       "/run/state/changed",
		OccurredAt:  "2026-05-16T00:00:00Z",
		WorkspaceID: common.WorkspaceID("ws-jsonl"),
		RunID:       common.RunID("run-jsonl"),
		Payload:     mustFileRepositoryPayload(t, map[string]any{"large": payload}),
	}); err != nil {
		t.Fatalf("append event: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "events", "events-000001.jsonl")); err != nil {
		t.Fatalf("event JSONL segment stat: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "payloads", filepath.FromSlash(safeEventRecordFileName("event-jsonl-1"))+".json")); err != nil {
		t.Fatalf("large event payload ref stat: %v", err)
	}
	beforeDefaultList := repository.EventRecordIOMetricsSnapshot()
	items, err := repo.List(ctx, workorchquerycontract.EventRecordQuery{RunID: common.RunID("run-jsonl")})
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(items) != 1 || len(items[0].Payload.Fields) != 0 {
		t.Fatalf("events = %+v, want lightweight event without payload", items)
	}
	if items[0].PayloadRef == "" || len(items[0].PayloadSummary.Fields) == 0 {
		t.Fatalf("event payload projection = ref:%q summary:%+v, want ref and summary", items[0].PayloadRef, items[0].PayloadSummary)
	}
	afterDefaultList := repository.EventRecordIOMetricsSnapshot()
	if afterDefaultList.MetadataScansTotal <= beforeDefaultList.MetadataScansTotal ||
		afterDefaultList.MetadataRecordsScannedTotal <= beforeDefaultList.MetadataRecordsScannedTotal {
		t.Fatalf("event metadata metrics before/after = %+v/%+v, want metadata scan pressure incremented", beforeDefaultList, afterDefaultList)
	}
	if afterDefaultList.PayloadHydratesTotal != beforeDefaultList.PayloadHydratesTotal ||
		afterDefaultList.PayloadHydrateBytesTotal != beforeDefaultList.PayloadHydrateBytesTotal {
		t.Fatalf("event payload hydrate metrics before/after default list = %+v/%+v, want unchanged", beforeDefaultList, afterDefaultList)
	}
	withPayload, err := repo.List(ctx, workorchquerycontract.EventRecordQuery{RunID: common.RunID("run-jsonl"), IncludePayload: true})
	if err != nil {
		t.Fatalf("list events with payload: %v", err)
	}
	if len(withPayload) != 1 {
		t.Fatalf("events = %+v, want one rehydrated payload", withPayload)
	}
	large, ok := withPayload[0].Payload.String("large")
	if !ok || large != payload {
		t.Fatalf("events = %+v, want rehydrated large payload", withPayload)
	}
	afterPayloadList := repository.EventRecordIOMetricsSnapshot()
	if afterPayloadList.PayloadHydratesTotal <= afterDefaultList.PayloadHydratesTotal ||
		afterPayloadList.PayloadHydrateBytesTotal <= afterDefaultList.PayloadHydrateBytesTotal {
		t.Fatalf("event payload hydrate metrics default/with-payload = %+v/%+v, want hydrate incremented", afterDefaultList, afterPayloadList)
	}
}

func TestEventRecordRepositoryFailsClosedOnCorruptPersistedOccurredAt(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "event-records")
	repo := NewEventRecordRepository(root)
	record := &eventstreammodel.EventRecord{
		ID: "event-corrupt-time", Version: "workorch.event", Topic: "/test", OccurredAt: "2026-08-30T00:00:00Z",
		SourceModule: "blocks/test", SubjectType: "test", SubjectID: "corrupt-time", StreamPosition: 1,
	}
	if err := repo.Append(ctx, record); err != nil {
		t.Fatal(err)
	}
	segmentPath := filepath.Join(root, "events", eventRecordSegmentName(1))
	data, err := os.ReadFile(segmentPath)
	if err != nil {
		t.Fatal(err)
	}
	corrupt := bytes.Replace(data, []byte(record.OccurredAt), []byte("invalid"), 1)
	if bytes.Equal(corrupt, data) {
		t.Fatal("persisted OccurredAt was not found in JSONL segment")
	}
	if err := os.WriteFile(segmentPath, corrupt, 0o644); err != nil {
		t.Fatal(err)
	}
	fresh := NewEventRecordRepository(root)
	if _, err := fresh.Get(ctx, record.ID); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("Get corrupt OccurredAt error = %v, want conflict", err)
	}
	if _, err := fresh.List(ctx, workorchquerycontract.EventRecordQuery{}); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("List corrupt OccurredAt error = %v, want conflict", err)
	}
}

func TestEventRecordRepositoryRejectsNullExternalPayload(t *testing.T) {
	repo := NewEventRecordRepository(filepath.Join(t.TempDir(), "event-records"))
	if err := os.MkdirAll(repo.eventSegmentDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	segmentData := []byte(`{"record":{"ID":"event-invalid-null-external","Version":"workorch.event","Topic":"/workspace/created","OccurredAt":"2026-08-22T00:00:00Z","SourceModule":"blocks/workspace","SubjectType":"workspace","SubjectID":"ws-invalid-null-external","WorkspaceID":"ws-invalid-null-external","StreamPosition":1,"Revision":1,"Payload":null,"PayloadSummary":{"fieldCount":1}},"payloadRef":"payloads/invalid-external.json"}` + "\n")
	segmentPath := filepath.Join(repo.eventSegmentDir(), eventRecordSegmentName(1))
	if err := os.WriteFile(segmentPath, segmentData, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.List(context.Background(), workorchquerycontract.EventRecordQuery{}); !errors.Is(err, workorcheventpayload.ErrInvalidPayload) {
		t.Fatalf("List error = %v, want invalid payload", err)
	}
}

func TestEventRecordRepositoryRejectsExternalPayloadWithoutCurrentSummary(t *testing.T) {
	repo := NewEventRecordRepository(filepath.Join(t.TempDir(), "event-records"))
	if err := os.MkdirAll(repo.eventSegmentDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	segmentData := []byte(`{"record":{"ID":"event-missing-summary","Version":"workorch.event","Topic":"/workspace/created","OccurredAt":"2026-08-22T00:00:00Z","SourceModule":"blocks/workspace","SubjectType":"workspace","SubjectID":"ws-missing-summary","WorkspaceID":"ws-missing-summary","StreamPosition":1,"Revision":1,"Payload":{},"PayloadSummary":{}},"payloadRef":"payloads/event-missing-summary.json"}` + "\n")
	segmentPath := filepath.Join(repo.eventSegmentDir(), eventRecordSegmentName(1))
	if err := os.WriteFile(segmentPath, segmentData, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.List(context.Background(), workorchquerycontract.EventRecordQuery{}); !errors.Is(err, workorcheventpayload.ErrInvalidPayload) {
		t.Fatalf("List error = %v, want invalid current payload summary", err)
	}
}

func TestEventRecordRepositoryRejectsNonObjectPayload(t *testing.T) {
	tests := []struct {
		name string
		line string
	}{
		{
			name: "inline null",
			line: `{"record":{"ID":"event-inline-null","Topic":"/test","Payload":null}}`,
		},
		{
			name: "array with external ref",
			line: `{"record":{"ID":"event-array-ref","Topic":"/test","Payload":[]},"payloadRef":"payloads/event-array-ref.json"}`,
		},
		{
			name: "scalar with external ref",
			line: `{"record":{"ID":"event-scalar-ref","Topic":"/test","Payload":"invalid"},"payloadRef":"payloads/event-scalar-ref.json"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := NewEventRecordRepository(filepath.Join(t.TempDir(), "event-records"))
			if err := os.MkdirAll(repo.eventSegmentDir(), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(repo.eventSegmentDir(), eventRecordSegmentName(1)), []byte(tt.line+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := repo.List(context.Background(), workorchquerycontract.EventRecordQuery{}); !errors.Is(err, workorcheventpayload.ErrInvalidPayload) {
				t.Fatalf("List error = %v, want invalid payload", err)
			}
		})
	}
}

func TestEventRecordRepositoryPurgeWorkspacePhysicallyCompactsPayloads(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "event-records")
	repo := NewEventRecordRepository(root)
	secret := "purged-workspace-secret-material"
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "unrelated-owner-state.json"), []byte(`{"preserved":true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	firstPosition, err := repo.ReserveEventRecordStreamPosition(ctx)
	if err != nil {
		t.Fatal(err)
	}
	secondPosition, err := repo.ReserveEventRecordStreamPosition(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range []*eventstreammodel.EventRecord{
		{
			ID: "event-purge-physical", Version: "workorch.event", Topic: "/test", OccurredAt: "2026-08-30T00:00:00Z",
			SourceModule: "blocks/test", SubjectType: "test", SubjectID: "purge-physical",
			WorkspaceID: "ws-purge-physical", StreamPosition: firstPosition, Revision: 1,
			Payload: mustFileRepositoryPayload(t, map[string]any{"content": secret + strings.Repeat("x", eventRecordPayloadInlineThreshold+1)}),
		},
		{
			ID: "event-keep-physical", Version: "workorch.event", Topic: "/test", OccurredAt: "2026-08-30T00:00:01Z",
			SourceModule: "blocks/test", SubjectType: "test", SubjectID: "keep-physical",
			WorkspaceID: "ws-keep-physical", StreamPosition: secondPosition, Revision: 1,
			Payload: mustFileRepositoryPayload(t, map[string]any{"content": "keep" + strings.Repeat("y", eventRecordPayloadInlineThreshold+1)}),
		},
	} {
		if err := repo.Append(ctx, item); err != nil {
			t.Fatal(err)
		}
	}
	// Exercise compaction of pre-existing tombstoned data as well as active
	// records. The purge must remove the old JSONL line and external payload.
	if err := repo.Delete(ctx, "event-purge-physical"); err != nil {
		t.Fatal(err)
	}
	// This package-local call verifies the compactor itself. Production callers
	// can only reach it through the exact-fenced Eventstream purger.
	if deleted, err := repo.purgeWorkspace(ctx, fileEventStreamPurgeFence("ws-purge-physical")); err != nil || deleted != 0 {
		t.Fatalf("purge deleted=%d err=%v", deleted, err)
	}
	if err := filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil || info == nil || info.IsDir() {
			return walkErr
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(data), secret) || strings.Contains(string(data), "event-purge-physical") {
			return fmt.Errorf("purged record remains in %s", path)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	kept, err := repo.Get(common.ContextWithRuntimeWorkspaceID(ctx, "ws-keep-physical"), "event-keep-physical")
	if err != nil || kept.WorkspaceID != "ws-keep-physical" {
		t.Fatalf("retained record = %+v err=%v", kept, err)
	}
	if data, err := os.ReadFile(filepath.Join(root, "unrelated-owner-state.json")); err != nil || string(data) != `{"preserved":true}` {
		t.Fatalf("unrelated owner state = %q err=%v", data, err)
	}
	thirdPosition, err := repo.ReserveEventRecordStreamPosition(ctx)
	if err != nil || thirdPosition <= secondPosition {
		t.Fatalf("post-purge stream position = %d err=%v, want > %d", thirdPosition, err, secondPosition)
	}
}

func TestArtifactContinuationRepositoryStoresContentOutsideMetadata(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "artifact-continuations")
	repo := NewArtifactContinuationRepository(root)
	continuationID := common.ArtifactContinuationID("artifact-continuation-path-001")
	chunkContent := strings.Repeat("chunk-body-", 64)
	assembledContent := strings.Repeat("assembled-body-", 64)

	if err := repo.Create(ctx, &orchestratormodel.ArtifactContinuation{
		ID:          continuationID,
		WorkspaceID: common.WorkspaceID("ws-path"),
		RunID:       common.RunID("run-path"),
		RootStepID:  common.StepID("run-path/step-1"),
		ArtifactID:  "artifact-001",
		TargetRef:   "deliverable/path.txt",
		State:       common.ArtifactContinuationStateChunking,
	}); err != nil {
		t.Fatalf("create artifact continuation: %v", err)
	}
	if err := repo.SaveChunk(ctx, &orchestratormodel.ArtifactContinuationChunk{
		ContinuationID: continuationID,
		Sequence:       1,
		ArtifactID:     "artifact-001",
		TargetRef:      "deliverable/path.txt",
		ContentHash:    "hash-001",
		Bytes:          int64(len(chunkContent)),
	}, chunkContent); err != nil {
		t.Fatalf("save chunk: %v", err)
	}
	if err := repo.SaveAssembled(ctx, &orchestratormodel.ArtifactContinuationAssembledMetadata{
		ContinuationID: continuationID,
		TargetRef:      "deliverable/path.txt",
		Bytes:          int64(len(assembledContent)),
		RollingHash:    "rolling-hash",
	}, assembledContent); err != nil {
		t.Fatalf("save assembled: %v", err)
	}

	dir := filepath.Join(root, string(continuationID))
	chunkMetadata, err := os.ReadFile(filepath.Join(dir, "chunks", "001.json"))
	if err != nil {
		t.Fatalf("read chunk metadata: %v", err)
	}
	if strings.Contains(string(chunkMetadata), chunkContent) {
		t.Fatalf("chunk metadata contains chunk content")
	}
	chunkBody, err := os.ReadFile(filepath.Join(dir, "chunks", "001.content"))
	if err != nil {
		t.Fatalf("read chunk content: %v", err)
	}
	if string(chunkBody) != chunkContent {
		t.Fatalf("chunk content = %q, want original content", string(chunkBody))
	}
	assembledMetadata, err := os.ReadFile(filepath.Join(dir, "assembled", "metadata.json"))
	if err != nil {
		t.Fatalf("read assembled metadata: %v", err)
	}
	if strings.Contains(string(assembledMetadata), assembledContent) {
		t.Fatalf("assembled metadata contains assembled content")
	}
	assembledBody, err := os.ReadFile(filepath.Join(dir, "assembled", "content"))
	if err != nil {
		t.Fatalf("read assembled content: %v", err)
	}
	if string(assembledBody) != assembledContent {
		t.Fatalf("assembled content = %q, want original content", string(assembledBody))
	}
}

func TestStepRepositoryStoresCurrentSequenceLayout(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "steps")
	repo := NewStepRepository(root)
	stepID := common.StepID("run-item/step-1")

	if err := repo.Create(ctx, &runmodel.Step{
		ID:     stepID,
		RunID:  common.RunID("run-item"),
		Kind:   "llm",
		State:  common.StepStateSucceeded,
		Input:  mustFileRepositoryPayload(t, map[string]any{"prompt": "hello"}),
		Output: mustFileRepositoryPayload(t, map[string]any{"answer": "world"}),
	}); err != nil {
		t.Fatalf("create step: %v", err)
	}
	itemDir := filepath.Join(root, "run-item", "001-step-1")
	for _, name := range []string{"metadata.json", "input.json", "output.json"} {
		if _, err := os.Stat(filepath.Join(itemDir, name)); err != nil {
			t.Fatalf("step item %s stat: %v", name, err)
		}
	}
	item, err := repo.Get(ctx, stepID)
	if err != nil {
		t.Fatalf("get step: %v", err)
	}
	prompt, _ := item.Input.String("prompt")
	answer, _ := item.Output.String("answer")
	if prompt != "hello" || answer != "world" {
		t.Fatalf("step payload = %+v/%+v, want split input/output", item.Input, item.Output)
	}
}

func TestStepRepositoryRejectsNullDetachedPayloadMetadata(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "steps")
	repo := NewStepRepository(root)
	stepID := common.StepID("run-invalid-null/step-1")
	if err := repo.Create(ctx, &runmodel.Step{
		ID:     stepID,
		RunID:  "run-invalid-null",
		Kind:   "llm",
		State:  common.StepStateSucceeded,
		Input:  mustFileRepositoryPayload(t, map[string]any{"prompt": "hello"}),
		Output: mustFileRepositoryPayload(t, map[string]any{"answer": "world"}),
	}); err != nil {
		t.Fatalf("create step: %v", err)
	}

	metadataPath := filepath.Join(root, "run-invalid-null", "001-step-1", "metadata.json")
	metadata, err := os.ReadFile(metadataPath)
	if err != nil {
		t.Fatal(err)
	}
	invalidMetadata := strings.ReplaceAll(string(metadata), `"Input":{}`, `"Input":null`)
	invalidMetadata = strings.ReplaceAll(invalidMetadata, `"Output":{}`, `"Output":null`)
	if invalidMetadata == string(metadata) || !strings.Contains(invalidMetadata, `"Input":null`) || !strings.Contains(invalidMetadata, `"Output":null`) {
		t.Fatalf("metadata fixture does not contain null payloads: %s", invalidMetadata)
	}
	if err := os.WriteFile(metadataPath, []byte(invalidMetadata), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := NewStepRepository(root).Get(ctx, stepID); !errors.Is(err, workorcheventpayload.ErrInvalidPayload) {
		t.Fatalf("Get error = %v, want invalid payload", err)
	}
}

func TestStepRepositoryRejectsNonObjectDetachedPayloadMetadata(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "steps")
	repo := NewStepRepository(root)
	stepID := common.StepID("run-invalid-step/step-1")
	if err := repo.Create(ctx, &runmodel.Step{ID: stepID, RunID: "run-invalid-step"}); err != nil {
		t.Fatalf("create step: %v", err)
	}
	metadataPath := filepath.Join(root, "run-invalid-step", "001-step-1", "metadata.json")
	metadata, err := os.ReadFile(metadataPath)
	if err != nil {
		t.Fatal(err)
	}
	invalidMetadata := strings.Replace(string(metadata), `"Input":{}`, `"Input":[]`, 1)
	if invalidMetadata == string(metadata) {
		t.Fatalf("metadata fixture does not contain Input object: %s", metadata)
	}
	if err := os.WriteFile(metadataPath, []byte(invalidMetadata), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := NewStepRepository(root).Get(ctx, stepID); !errors.Is(err, workorcheventpayload.ErrInvalidPayload) {
		t.Fatalf("Get error = %v, want invalid payload", err)
	}
}

func TestStepRepositoryListByRunUsesSequenceScope(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "steps")
	repo := NewStepRepository(root)
	for _, item := range []*runmodel.Step{
		{
			ID:    "run-a/step-1",
			RunID: "run-a",
			Kind:  "shell",
			State: common.StepStateSucceeded,
			Input: mustFileRepositoryPayload(t, map[string]any{
				"workspaceId":         "ws-a",
				"largeRuntimePayload": strings.Repeat("x", 8192),
			}),
			Output: mustFileRepositoryPayload(t, map[string]any{
				"status": "completed",
				"value":  "a1",
			}),
		},
		{
			ID:     "run-a/step-2",
			RunID:  "run-a",
			Kind:   "shell",
			State:  common.StepStateSucceeded,
			Output: mustFileRepositoryPayload(t, map[string]any{"value": "a2"}),
		},
		{
			ID:     "run-b/step-1",
			RunID:  "run-b",
			Kind:   "shell",
			State:  common.StepStateSucceeded,
			Output: mustFileRepositoryPayload(t, map[string]any{"value": "b1"}),
		},
	} {
		if err := repo.Create(ctx, item); err != nil {
			t.Fatalf("create step %s: %v", item.ID, err)
		}
	}

	items, err := repo.ListByRun(ctx, "run-a")
	if err != nil {
		t.Fatalf("list run-a steps: %v", err)
	}
	if len(items) != 2 || items[0].ID != "run-a/step-1" || items[1].ID != "run-a/step-2" {
		t.Fatalf("run-a steps = %+v, want only run-a steps in order", items)
	}
	for _, item := range items {
		if item.RunID != "run-a" {
			t.Fatalf("listed step run = %q, want run-a", item.RunID)
		}
	}

	summaries, err := repo.ListByRunSummary(ctx, "run-a")
	if err != nil {
		t.Fatalf("list run-a step summaries: %v", err)
	}
	if len(summaries) != 2 || summaries[0].ID != "run-a/step-1" || summaries[1].ID != "run-a/step-2" {
		t.Fatalf("run-a step summaries = %+v, want only run-a steps in order", summaries)
	}
	workspaceID, _ := summaries[0].Input.String("workspaceId")
	if workspaceID != "ws-a" {
		t.Fatalf("summary input workspaceId = %v, want ws-a", workspaceID)
	}
	if _, ok := summaries[0].Input.Field("largeRuntimePayload"); ok {
		t.Fatalf("summary retained largeRuntimePayload, want omitted")
	}
	status, _ := summaries[0].Output.String("status")
	if status != "completed" {
		t.Fatalf("summary output status = %v, want completed", status)
	}
	if _, ok := summaries[0].Output.Field("value"); ok {
		t.Fatalf("summary retained non-whitelisted output value, want omitted")
	}
}

func TestStepRepositoryListByRunPreservesCreationOrderWhenStepIDSortWouldDiffer(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "steps")
	repo := NewStepRepository(root)
	runID := common.RunID("run-sequence-order")
	created := []common.StepID{
		common.StepID(runID + "/repair-010-of-graph-015-014-verify"),
		common.StepID(runID + "/repair-001-of-graph-016-001-fix-timeout"),
	}
	for _, stepID := range created {
		if err := repo.Create(ctx, &runmodel.Step{
			ID:    stepID,
			RunID: runID,
			Kind:  "llm/workspace_execution",
			State: common.StepStateSucceeded,
		}); err != nil {
			t.Fatalf("create step %s: %v", stepID, err)
		}
	}

	items, err := repo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list steps: %v", err)
	}
	if len(items) != len(created) || items[0].ID != created[0] || items[1].ID != created[1] {
		t.Fatalf("steps = %+v, want creation order %v", items, created)
	}
	summaries, err := repo.ListByRunSummary(ctx, runID)
	if err != nil {
		t.Fatalf("list step summaries: %v", err)
	}
	if len(summaries) != len(created) || summaries[0].ID != created[0] || summaries[1].ID != created[1] {
		t.Fatalf("step summaries = %+v, want creation order %v", summaries, created)
	}
}

func TestStepRepositoryListByRunCacheReturnsClonesAndInvalidatesOnUpdate(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "steps")
	repo := NewStepRepository(root)
	step := &runmodel.Step{
		ID:     "run-cache/step-1",
		RunID:  "run-cache",
		Kind:   "llm/workspace_execution",
		State:  common.StepStateExecuting,
		Input:  mustFileRepositoryPayload(t, map[string]any{"workspaceId": "ws-cache"}),
		Output: mustFileRepositoryPayload(t, map[string]any{"status": "running"}),
	}
	if err := repo.Create(ctx, step); err != nil {
		t.Fatalf("create step: %v", err)
	}
	items, err := repo.ListByRun(ctx, step.RunID)
	if err != nil {
		t.Fatalf("list steps: %v", err)
	}
	items[0].Output.Fields[0].Value.String = "mutated-by-caller"
	cached, err := repo.ListByRun(ctx, step.RunID)
	if err != nil {
		t.Fatalf("list cached steps: %v", err)
	}
	cachedStatus, _ := cached[0].Output.String("status")
	if cachedStatus != "running" {
		t.Fatalf("cached step output = %v, want clone isolated from caller mutation", cached[0].Output)
	}
	step.State = common.StepStateSucceeded
	step.Output = mustFileRepositoryPayload(t, map[string]any{"status": "completed"})
	if err := repo.Update(ctx, step); err != nil {
		t.Fatalf("update step: %v", err)
	}
	updated, err := repo.ListByRun(ctx, step.RunID)
	if err != nil {
		t.Fatalf("list updated steps: %v", err)
	}
	updatedStatus, _ := updated[0].Output.String("status")
	if updated[0].State != common.StepStateSucceeded || updatedStatus != "completed" {
		t.Fatalf("updated step = state %s output %+v, want completed from invalidated cache", updated[0].State, updated[0].Output)
	}
}

func TestStepRepositoryListByRunSummaryRejectsInvalidCurrentSummary(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "steps")
	repo := NewStepRepository(root)
	stepID := common.StepID("run-invalid-summary/step-1")
	item := &runmodel.Step{
		ID:    stepID,
		RunID: "run-invalid-summary",
		Kind:  "llm/workspace_execution",
		State: common.StepStateSucceeded,
		Input: mustFileRepositoryPayload(t, map[string]any{
			"workspaceId":           "ws-invalid-summary",
			"runtimeRepairFollowUp": true,
			"largeRuntimePayload":   strings.Repeat("x", 8192),
		}),
		Output: mustFileRepositoryPayload(t, map[string]any{
			"status":           "completed",
			"failureSignature": "class=runtime_failed|targetRef=main.go",
			"rawResponse":      strings.Repeat("y", 8192),
		}),
	}
	if err := repo.Create(ctx, item); err != nil {
		t.Fatalf("create step: %v", err)
	}

	itemDir := filepath.Join(root, "run-invalid-summary", "001-step-1")
	invalidSummary := `{
		"ID":"run-invalid-summary/step-1",
		"RunID":"run-invalid-summary",
		"Kind":"llm/workspace_execution",
		"State":"completed",
		"Input":{"workspaceId":"ws-invalid-summary","largeRuntimePayload":"` + strings.Repeat("z", 4096) + `"},
		"Output":{"status":"completed","rawResponse":"` + strings.Repeat("r", 4096) + `"}
	}`
	if err := os.WriteFile(filepath.Join(itemDir, "summary.json"), []byte(invalidSummary), 0o644); err != nil {
		t.Fatalf("write invalid current summary: %v", err)
	}

	if _, err := repo.ListByRunSummary(ctx, "run-invalid-summary"); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("ListByRunSummary error = %v, want invalid argument", err)
	}

	after, err := os.ReadFile(filepath.Join(itemDir, "summary.json"))
	if err != nil {
		t.Fatalf("read invalid current summary: %v", err)
	}
	if string(after) != invalidSummary {
		t.Fatal("invalid current summary was rewritten during read")
	}
}

func TestToolCallRepositoryStoresCurrentSequenceLayout(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "tool-calls")
	repo := NewToolCallRepository(root)
	toolCallID := common.ToolCallID("run-item/step-1/tool-1")

	if err := repo.Create(ctx, &runtimeevidencemodel.ToolCall{
		ID:        toolCallID,
		RunID:     common.RunID("run-item"),
		StepID:    common.StepID("run-item/step-1"),
		ToolName:  "shell",
		State:     common.ToolCallStateCompleted,
		Arguments: mustFileRepositoryPayload(t, map[string]any{"command": "true"}),
		Output:    mustFileRepositoryPayload(t, map[string]any{"exitCode": float64(0)}),
	}); err != nil {
		t.Fatalf("create tool call: %v", err)
	}
	itemDir := filepath.Join(root, "run-item", "001-step-1-tool-1")
	for _, name := range []string{"metadata.json", "arguments.json", "output.json"} {
		if _, err := os.Stat(filepath.Join(itemDir, name)); err != nil {
			t.Fatalf("tool call item %s stat: %v", name, err)
		}
	}
	items, err := repo.ListByStep(ctx, common.StepID("run-item/step-1"))
	if err != nil {
		t.Fatalf("list tool calls by step: %v", err)
	}
	command, _ := items[0].Arguments.String("command")
	exitCode, _ := items[0].Output.Int64("exitCode")
	if len(items) != 1 || command != "true" || exitCode != 0 {
		t.Fatalf("tool calls = %+v, want rehydrated arguments/output", items)
	}
}

func TestToolCallRepositoryRejectsNullDetachedPayloadMetadata(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "tool-calls")
	repo := NewToolCallRepository(root)
	toolCallID := common.ToolCallID("run-invalid-null/step-1/tool-1")
	if err := repo.Create(ctx, &runtimeevidencemodel.ToolCall{
		ID:        toolCallID,
		RunID:     "run-invalid-null",
		StepID:    "run-invalid-null/step-1",
		ToolName:  "shell",
		State:     common.ToolCallStateCompleted,
		Arguments: mustFileRepositoryPayload(t, map[string]any{"command": "true"}),
		Output:    mustFileRepositoryPayload(t, map[string]any{"exitCode": float64(0)}),
	}); err != nil {
		t.Fatalf("create tool call: %v", err)
	}

	metadataPath := filepath.Join(root, "run-invalid-null", "001-step-1-tool-1", "metadata.json")
	metadata, err := os.ReadFile(metadataPath)
	if err != nil {
		t.Fatal(err)
	}
	invalidMetadata := strings.ReplaceAll(string(metadata), `"Arguments":{}`, `"Arguments":null`)
	invalidMetadata = strings.ReplaceAll(invalidMetadata, `"Output":{}`, `"Output":null`)
	if invalidMetadata == string(metadata) || !strings.Contains(invalidMetadata, `"Arguments":null`) || !strings.Contains(invalidMetadata, `"Output":null`) {
		t.Fatalf("metadata fixture does not contain null payloads: %s", invalidMetadata)
	}
	if err := os.WriteFile(metadataPath, []byte(invalidMetadata), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := NewToolCallRepository(root).Get(ctx, toolCallID); !errors.Is(err, workorcheventpayload.ErrInvalidPayload) {
		t.Fatalf("Get error = %v, want invalid payload", err)
	}
}

func TestToolCallRepositoryRejectsNonObjectDetachedPayloadMetadata(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "tool-calls")
	repo := NewToolCallRepository(root)
	toolCallID := common.ToolCallID("run-invalid-tool/step-1/tool-1")
	if err := repo.Create(ctx, &runtimeevidencemodel.ToolCall{ID: toolCallID, RunID: "run-invalid-tool", StepID: "run-invalid-tool/step-1"}); err != nil {
		t.Fatalf("create tool call: %v", err)
	}
	metadataPath := filepath.Join(root, "run-invalid-tool", "001-step-1-tool-1", "metadata.json")
	metadata, err := os.ReadFile(metadataPath)
	if err != nil {
		t.Fatal(err)
	}
	invalidMetadata := strings.Replace(string(metadata), `"Arguments":{}`, `"Arguments":"invalid"`, 1)
	if invalidMetadata == string(metadata) {
		t.Fatalf("metadata fixture does not contain Arguments object: %s", metadata)
	}
	if err := os.WriteFile(metadataPath, []byte(invalidMetadata), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := NewToolCallRepository(root).Get(ctx, toolCallID); !errors.Is(err, workorcheventpayload.ErrInvalidPayload) {
		t.Fatalf("Get error = %v, want invalid payload", err)
	}
}

func TestRuntimeEvidenceRepositoriesUseRunScopedSequencePathsAndReuseExistingPaths(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	runID := common.RunID("run-evidence")
	stepID := common.StepID("run-evidence/graph-001/001-plan")

	stepRepo := NewStepRepository(filepath.Join(root, "steps"))
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    stepID,
		RunID: runID,
		Kind:  "llm/planning",
		State: common.StepStateExecuting,
	}); err != nil {
		t.Fatalf("create step: %v", err)
	}
	stepDir := filepath.Join(root, "steps", "run-evidence", "001-graph-001-001-plan")
	if _, err := os.Stat(filepath.Join(stepDir, "metadata.json")); err != nil {
		t.Fatalf("step sequence dir stat: %v", err)
	}
	step, err := stepRepo.Get(ctx, stepID)
	if err != nil {
		t.Fatalf("get step by logical id: %v", err)
	}
	step.State = common.StepStateSucceeded
	step.Output = mustFileRepositoryPayload(t, map[string]any{"status": "done"})
	if err := stepRepo.Update(ctx, step); err != nil {
		t.Fatalf("update step: %v", err)
	}
	if _, err := os.Stat(filepath.Join(stepDir, "output.json")); err != nil {
		t.Fatalf("step update should reuse sequence dir: %v", err)
	}

	validationRepo := NewValidationResultRepository(filepath.Join(root, "validations"))
	validationID := common.ValidationResultID("run-evidence/graph-001/001-plan/validation/runtime-step")
	if err := validationRepo.Create(ctx, &runtimeevidencemodel.ValidationResult{
		ID:             validationID,
		WorkspaceID:    common.WorkspaceID("ws-evidence"),
		RunID:          runID,
		StepID:         stepID,
		ValidationKind: "runtime_step",
		State:          common.ValidationResultStatePassed,
	}); err != nil {
		t.Fatalf("create validation: %v", err)
	}
	validationDir := filepath.Join(root, "validations", "run-evidence", "001-graph-001-001-plan-validation-runtime-step")
	if _, err := os.Stat(filepath.Join(validationDir, "metadata.json")); err != nil {
		t.Fatalf("validation sequence dir stat: %v", err)
	}
	if _, err := validationRepo.Get(ctx, validationID); err != nil {
		t.Fatalf("get validation by logical id: %v", err)
	}
}

func TestRunEvidenceRepositoriesStoreCurrentItemLayouts(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	runID := common.RunID("run-evidence")
	stepID := common.StepID("run-evidence/step-1")

	patchPath := filepath.Join(root, "patches")
	patchRepo := NewPatchRepository(patchPath)
	patchID := common.PatchID("run-evidence/patch-1")
	if err := patchRepo.Create(ctx, &runtimeevidencemodel.Patch{
		ID:      patchID,
		RunID:   runID,
		StepID:  stepID,
		Diff:    "diff --git a/file b/file",
		State:   common.PatchStatePreviewed,
		Summary: "preview",
	}); err != nil {
		t.Fatalf("create patch: %v", err)
	}
	assertItemFileExists(t, patchPath, string(patchID))
	patches, err := patchRepo.ListByStep(ctx, stepID)
	if err != nil {
		t.Fatalf("list patches by step: %v", err)
	}
	if len(patches) != 1 || patches[0].Diff == "" {
		t.Fatalf("patches = %+v, want rehydrated item", patches)
	}

	validationPath := filepath.Join(root, "validations")
	validationRepo := NewValidationResultRepository(validationPath)
	validationID := common.ValidationResultID("run-evidence/validation-1")
	if err := validationRepo.Create(ctx, &runtimeevidencemodel.ValidationResult{
		ID:           validationID,
		RunID:        runID,
		StepID:       stepID,
		State:        common.ValidationResultStatePassed,
		Summary:      "passed",
		EvidenceTags: []string{"unit"},
	}); err != nil {
		t.Fatalf("create validation result: %v", err)
	}
	if _, err := os.Stat(filepath.Join(validationPath, "run-evidence", "001-validation-1", "metadata.json")); err != nil {
		t.Fatalf("expected sequence-scoped validation result: %v", err)
	}

	checkpointPath := filepath.Join(root, "checkpoints")
	checkpointRepo := NewRuntimeCheckpointRepository(checkpointPath)
	checkpointID := common.CheckpointID("run-evidence/checkpoint-1")
	if err := checkpointRepo.Create(ctx, &runmodel.RuntimeCheckpoint{
		ID:             checkpointID,
		RunID:          runID,
		CheckpointType: common.CheckpointTypeRunState,
		Data:           mustFileRepositoryPayload(t, map[string]any{"state": "running"}),
	}); err != nil {
		t.Fatalf("create runtime checkpoint: %v", err)
	}
	assertItemFileExists(t, checkpointPath, string(checkpointID))
	checkpoint, err := checkpointRepo.Get(ctx, checkpointID)
	if err != nil {
		t.Fatalf("get runtime checkpoint: %v", err)
	}
	state, _ := checkpoint.Data.String("state")
	if state != "running" {
		t.Fatalf("checkpoint data = %+v, want rehydrated data", checkpoint.Data)
	}

	resultPath := filepath.Join(root, "results")
	resultRepo := NewResultRepository(resultPath)
	resultID := common.ResultID("run-evidence/result-1")
	if err := resultRepo.Create(ctx, &resultmodel.Result{
		ID:           resultID,
		RunID:        runID,
		State:        common.ResultStateAccepted,
		Summary:      "accepted",
		EvidenceRefs: []string{"evidence-1"},
	}); err != nil {
		t.Fatalf("create result: %v", err)
	}
	assertItemFileExists(t, resultPath, string(resultID))

	artifactPath := filepath.Join(root, "artifacts")
	artifactRepo := NewArtifactRepository(artifactPath)
	artifactID := common.ArtifactID("run-evidence/artifact-1")
	if err := artifactRepo.Create(ctx, &resultmodel.Artifact{
		ID:           artifactID,
		RunID:        runID,
		ResultID:     resultID,
		ArtifactType: "text",
		State:        common.ArtifactStateAvailable,
		Metadata:     mustFileRepositoryPayload(t, map[string]any{"bytes": float64(12)}),
	}); err != nil {
		t.Fatalf("create artifact: %v", err)
	}
	assertItemFileExists(t, artifactPath, string(artifactID))
	artifacts, err := artifactRepo.ListByResult(ctx, resultID)
	if err != nil {
		t.Fatalf("list artifacts by result: %v", err)
	}
	if len(artifacts) != 1 {
		t.Fatalf("artifacts = %+v, want one rehydrated artifact", artifacts)
	}
	bytes, _ := artifacts[0].Metadata.Float64("bytes")
	if bytes != float64(12) {
		t.Fatalf("artifacts = %+v, want rehydrated metadata", artifacts)
	}
}

func TestGovernanceRepositoriesStoreTypedPayloads(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()

	ticketPath := filepath.Join(root, "decisions", "tickets")
	ticketRepo := NewDecisionTicketRepository(ticketPath)
	ticketID := common.TicketID("run-governance/ticket-1")
	if err := ticketRepo.Create(ctx, &decisionmodel.Ticket{
		ID:          ticketID,
		WorkspaceID: common.WorkspaceID("ws-governance"),
		RunID:       common.RunID("run-governance"),
		Reason:      "need human decision",
		Context:     mustFileRepositoryPayload(t, map[string]any{"field": "value"}),
		State:       common.DecisionTicketStateOpen,
	}); err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	assertItemFileExists(t, ticketPath, string(ticketID))
	tickets, err := ticketRepo.ListByRunID(ctx, common.RunID("run-governance"))
	if err != nil {
		t.Fatalf("list tickets by run: %v", err)
	}
	field, _ := tickets[0].Context.String("field")
	if len(tickets) != 1 || field != "value" {
		t.Fatalf("tickets = %+v, want rehydrated context", tickets)
	}

	resolutionPath := filepath.Join(root, "decisions", "resolutions")
	resolutionRepo := NewDecisionResolutionRepository(resolutionPath)
	resolutionID := common.ResolutionID("run-governance/resolution-1")
	if err := resolutionRepo.Create(ctx, &decisionmodel.Resolution{
		ID:       resolutionID,
		TicketID: ticketID,
		Type:     "provide_missing_info",
		Payload:  mustFileRepositoryPayload(t, map[string]any{"answer": "ok"}),
	}); err != nil {
		t.Fatalf("create resolution: %v", err)
	}
	assertItemFileExists(t, resolutionPath, string(resolutionID))
	resolution, err := resolutionRepo.GetByTicketID(ctx, ticketID)
	if err != nil {
		t.Fatalf("get resolution by ticket: %v", err)
	}
	answer, _ := resolution.Payload.String("answer")
	if answer != "ok" {
		t.Fatalf("resolution payload = %+v, want rehydrated payload", resolution.Payload)
	}

	approvalPath := filepath.Join(root, "approvals")
	approvalRepo := NewApprovalRepository(approvalPath)
	approvalID := common.ApprovalID("run-governance/approval-1")
	if err := approvalRepo.Create(ctx, &approvalmodel.Approval{
		ID:          approvalID,
		WorkspaceID: common.WorkspaceID("ws-governance"),
		RunID:       common.RunID("run-governance"),
		Reason:      "need approval",
		Context:     mustFileRepositoryPayload(t, map[string]any{"risk": "high"}),
		State:       common.ApprovalStatePending,
	}); err != nil {
		t.Fatalf("create approval: %v", err)
	}
	assertItemFileExists(t, approvalPath, string(approvalID))
	approvals, err := approvalRepo.ListByWorkspaceID(ctx, common.WorkspaceID("ws-governance"))
	if err != nil {
		t.Fatalf("list approvals by workspace: %v", err)
	}
	risk, _ := approvals[0].Context.String("risk")
	if len(approvals) != 1 || risk != "high" {
		t.Fatalf("approvals = %+v, want rehydrated context", approvals)
	}
}

func assertItemFileExists(t *testing.T, root string, id string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(safeRepositoryItemFileName(id))+".json")); err != nil {
		t.Fatalf("item file for %s under %s stat: %v", id, root, err)
	}
}

func TestDecisionResolutionRepositoryStoresLongReadableIDsWithSafeFileName(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	repo := NewDecisionResolutionRepository(filepath.Join(root, "decisions", "resolutions"))
	ticketID := common.TicketID("run-001-" + strings.Repeat("context-gate-retry-", 12) + "context-integrity-decision")
	resolutionID := common.ResolutionID("resolution-ws-real-run-" + string(ticketID) + "-provide_missing_info")

	if err := repo.Create(ctx, &decisionmodel.Resolution{
		ID:       resolutionID,
		TicketID: ticketID,
		Type:     "provide_missing_info",
		Payload:  mustFileRepositoryPayload(t, map[string]any{"answer": "continue"}),
	}); err != nil {
		t.Fatalf("create long resolution: %v", err)
	}
	resolution, err := repo.GetByTicketID(ctx, ticketID)
	if err != nil {
		t.Fatalf("get long resolution by ticket: %v", err)
	}
	longAnswer, _ := resolution.Payload.String("answer")
	if resolution.ID != resolutionID || longAnswer != "continue" {
		t.Fatalf("resolution = %+v, want stored long ID and payload", resolution)
	}

	paths, err := repositoryItemPaths(filepath.Join(root, "decisions", "resolutions"))
	if err != nil {
		t.Fatalf("list resolution files: %v", err)
	}
	if len(paths) != 1 {
		t.Fatalf("resolution file count = %d, want 1: %v", len(paths), paths)
	}
	name := filepath.Base(paths[0])
	if len(strings.TrimSuffix(name, filepath.Ext(name))) > 160 {
		t.Fatalf("resolution filename segment too long: len=%d name=%s", len(strings.TrimSuffix(name, filepath.Ext(name))), name)
	}
	if !strings.Contains(name, "-") {
		t.Fatalf("resolution filename = %s, want readable prefix plus hash suffix", name)
	}
}

func TestStepRepositoryStoresLongReadableIDsWithBoundedSequenceDirectories(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	repo := NewStepRepository(filepath.Join(root, "steps"))
	runID := common.RunID("run-" + strings.Repeat("very-long-scope-", 30))
	stepID := common.StepID(string(runID) + "/repair-" + strings.Repeat("very-long-step-", 40))

	if err := repo.Create(ctx, &runmodel.Step{
		ID:     stepID,
		RunID:  runID,
		Kind:   "llm/workspace_execution",
		State:  common.StepStateSucceeded,
		Input:  mustFileRepositoryPayload(t, map[string]any{"input": "ok"}),
		Output: mustFileRepositoryPayload(t, map[string]any{"output": "ok"}),
	}); err != nil {
		t.Fatalf("create long step: %v", err)
	}
	got, err := repo.Get(ctx, stepID)
	if err != nil {
		t.Fatalf("get long step: %v", err)
	}
	output, _ := got.Output.String("output")
	if got.ID != stepID || got.RunID != runID || output != "ok" {
		t.Fatalf("step = %+v, want long id round trip with payload", got)
	}
	listed, err := repo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list long run steps: %v", err)
	}
	if len(listed) != 1 || listed[0].ID != stepID {
		t.Fatalf("listed = %+v, want long step by run", listed)
	}

	dirs, err := repositoryMetadataDirs(filepath.Join(root, "steps"))
	if err != nil {
		t.Fatalf("metadata dirs: %v", err)
	}
	if len(dirs) != 1 {
		t.Fatalf("metadata dirs = %+v, want one", dirs)
	}
	rel, err := filepath.Rel(filepath.Join(root, "steps"), dirs[0])
	if err != nil {
		t.Fatalf("relative metadata dir: %v", err)
	}
	for _, segment := range strings.Split(filepath.ToSlash(rel), "/") {
		if len(segment) > 160 {
			t.Fatalf("metadata segment len = %d, want <= 160, segment=%q rel=%s", len(segment), segment, rel)
		}
	}
}

func TestWorkspaceRepositorySharedInstanceConcurrentCreate(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "workspace.json")
	repo := NewWorkspaceRepository(path)
	const total = 100

	var wg sync.WaitGroup
	errs := make(chan error, total)
	for idx := 0; idx < total; idx++ {
		idx := idx
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- repo.Create(ctx, &workspacemodel.Workspace{
				ID:    common.WorkspaceID(fmt.Sprintf("ws-shared-%03d", idx)),
				Title: fmt.Sprintf("shared workspace %03d", idx),
				State: common.WorkspaceStateActive,
			})
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent Create with shared repository instance: %v", err)
		}
	}

	items, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List after concurrent Create: %v", err)
	}
	if len(items) != total {
		t.Fatalf("workspace count = %d, want %d", len(items), total)
	}
}

func TestWorkspaceRepositoryIndependentInstancesConcurrentCreateStress(t *testing.T) {
	if os.Getenv("WORKORCH_STRESS_TEST") != "1" {
		t.Skip("set WORKORCH_STRESS_TEST=1 to run file backend cross-instance concurrency stress")
	}
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "workspace.json")
	const total = 100

	var wg sync.WaitGroup
	errs := make(chan error, total)
	for idx := 0; idx < total; idx++ {
		idx := idx
		wg.Add(1)
		go func() {
			defer wg.Done()
			repo := NewWorkspaceRepository(path)
			errs <- repo.Create(ctx, &workspacemodel.Workspace{
				ID:    common.WorkspaceID(fmt.Sprintf("ws-independent-%03d", idx)),
				Title: fmt.Sprintf("independent workspace %03d", idx),
				State: common.WorkspaceStateActive,
			})
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent Create with independent repository instances: %v", err)
		}
	}

	items, err := NewWorkspaceRepository(path).List(ctx)
	if err != nil {
		t.Fatalf("List after independent concurrent Create: %v", err)
	}
	if len(items) != total {
		t.Fatalf("workspace count = %d, want %d", len(items), total)
	}
}
