package readmodelprojection

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/recordpayload"
	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
	"github.com/muidea/workorch/internal/pkg/llmaudit"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func TestMapLLMUsageTracesFromEventRecordsReplaysAuditIdempotentlyAndDropsUnknownPayload(t *testing.T) {
	const secret = "raw-prompt-content-must-not-be-projected"
	budget := llmaudit.ContextBudget{
		ProviderID: "ai-proxy", ModelID: "gpt-4o", ProviderSubmitted: false,
		ContextGateAction: "fail_fast", ContextGateReason: "runtime_provider_ceiling_exceeded",
		BudgetFailureClass: "provider_ceiling_exceeded",
		ProtectedFields:    []string{"P0.contract", "P4.current_snapshots"},
		RedactedPaths:      []string{"activation.memories[0].content"},
		SourceRefs:         []string{"runtime://authority/authority-1"},
		SectionHashes: []llmaudit.ContextSection{{
			ID: "C3.runtime-authority", Hash: "sha256:authority", SizeBytes: 55,
			SourceRefs: []string{"runtime://focus/focus-1"},
		}},
	}
	newRecord := func(id, occurredAt string) projectioncontract.EventRecord {
		physicalCallID := "physical-" + id
		payload := (recordpayload.LLMUsagePayloadData{
			RequestID: physicalCallID, LogicalRequestID: "logical-audit", PhysicalCallID: physicalCallID,
			WorkspaceID: "ws-audit", RunID: "run-audit", StepID: "authority-1",
			ProviderID: "ai-proxy", Model: "gpt-4o", Operation: "runtime_candidate_preflight",
			Kind: "runtime_candidate_preflight", PhysicalResponseMode: llmaudit.PhysicalResponseModeNonStreaming, ProviderSubmitted: false,
			Status: "failed", FailureClass: "context_integrity", ContextBudget: budget,
		}).EventPayload()
		// A persisted event can contain unrelated fields from a future/bad
		// producer. Projection must not turn them into a prompt transport.
		payload = payload.Merge(workorcheventpayload.New(workorcheventpayload.StringField("prompt", secret)))
		return projectioncontract.EventRecord{
			ID: id, Topic: llmevents.TopicLLMUsageRecorded, OccurredAt: occurredAt,
			WorkspaceID: "ws-audit", RunID: "run-audit", StepID: "authority-1", Payload: payload,
		}
	}
	first := newRecord("audit-event-1", "2026-07-25T00:00:00Z")
	later := newRecord("audit-event-2", "2026-07-25T00:00:01Z")
	projectedPayload := projectEventPayload(first.Payload)
	if logicalRequestID, ok := projectedPayload.String(recordpayload.PayloadKeyLogicalRequestID); !ok || logicalRequestID != "logical-audit" {
		t.Fatalf("projected logical request identity = %q/%v", logicalRequestID, ok)
	}
	if physicalCallID, ok := projectedPayload.String(recordpayload.PayloadKeyPhysicalCallID); !ok || physicalCallID != "physical-audit-event-1" {
		t.Fatalf("projected physical call identity = %q/%v", physicalCallID, ok)
	}
	if _, ok := projectedPayload.Field("prompt"); ok {
		t.Fatalf("unknown prompt field survived generic event payload projection: %+v", projectedPayload)
	}
	traces := mapLLMUsageTracesFromEventRecords([]projectioncontract.EventRecord{first, first, later}, "run-audit")
	if len(traces) != 2 || traces[0].ID != "audit-event-1" || traces[1].ID != "audit-event-2" {
		t.Fatalf("replayed audit traces=%+v", traces)
	}
	_, nested := traces[0].ContextBudget.Object("contextBudget")
	if nested || len(traces[0].ContextBudget.Fields) == 0 {
		// ContextBudget on an LLMUsageTrace is already the inner audit object;
		// an additional nesting would indicate a projection contract regression.
		t.Fatalf("context budget projection=%+v", traces[0].ContextBudget)
	}
	if _, ok := traces[0].ContextBudget.String("contextGateReason"); !ok {
		t.Fatalf("context gate reason missing from projected audit=%+v", traces[0].ContextBudget)
	}
	if _, ok := traces[0].ContextBudget.Field("prompt"); ok {
		t.Fatalf("unknown prompt field survived read projection: %+v", traces[0].ContextBudget)
	}
	if traces[0].PhysicalResponseMode != llmaudit.PhysicalResponseModeNonStreaming || traces[0].ProviderSubmitted {
		t.Fatalf("physical provider audit drifted in projection: %+v", traces[0])
	}
	if traces[0].LogicalRequestID != "logical-audit" || traces[0].PhysicalCallID != "physical-audit-event-1" {
		t.Fatalf("two-level request identity drifted in projection: %+v", traces[0])
	}
	encoded, err := json.Marshal(traces)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), secret) {
		t.Fatalf("read projection leaked prompt content: %s", encoded)
	}
	if !strings.Contains(string(encoded), `"logicalRequestId":"logical-audit"`) || !strings.Contains(string(encoded), `"physicalCallId":"physical-audit-event-1"`) {
		t.Fatalf("public trace omitted two-level request identity: %s", encoded)
	}
}

func TestLLMUsageTraceRequiresCurrentPhysicalResponseModeAndIdentity(t *testing.T) {
	missingMode := workorcheventpayload.New(
		workorcheventpayload.StringField(recordpayload.PayloadKeyWorkspaceID, "ws-current"),
		workorcheventpayload.StringField(recordpayload.PayloadKeyRunID, "run-current"),
		workorcheventpayload.StringField(recordpayload.PayloadKeyStatus, "completed"),
		workorcheventpayload.BoolField(recordpayload.PayloadKeyProviderSubmitted, true),
	)
	if _, ok := llmUsageTraceFromEventRecord(projectioncontract.EventRecord{ID: "missing-mode", WorkspaceID: "ws-current", RunID: "run-current", Payload: missingMode}); ok {
		t.Fatal("LLM usage trace accepted persistence without physicalResponseMode")
	}

	currentPayload := missingMode.Merge(workorcheventpayload.New(
		workorcheventpayload.StringField(recordpayload.PayloadKeyPhysicalResponseMode, string(llmaudit.PhysicalResponseModeStreaming)),
	))
	current, ok := llmUsageTraceFromEventRecord(projectioncontract.EventRecord{ID: "current", WorkspaceID: "ws-current", RunID: "run-current", Payload: currentPayload})
	if !ok || current.PhysicalResponseMode != llmaudit.PhysicalResponseModeStreaming || !current.ProviderSubmitted {
		t.Fatalf("current streaming projection = %+v, ok=%v", current, ok)
	}

	invalidPayload := currentPayload.Merge(workorcheventpayload.New(
		workorcheventpayload.StringField(recordpayload.PayloadKeyPhysicalResponseMode, "adaptive"),
	))
	if _, ok := llmUsageTraceFromEventRecord(projectioncontract.EventRecord{ID: "invalid", WorkspaceID: "ws-current", RunID: "run-current", Payload: invalidPayload}); ok {
		t.Fatal("LLM usage trace accepted an invalid physical response mode")
	}

	missingSubmission := workorcheventpayload.New(
		workorcheventpayload.StringField(recordpayload.PayloadKeyWorkspaceID, "ws-current"),
		workorcheventpayload.StringField(recordpayload.PayloadKeyRunID, "run-current"),
		workorcheventpayload.StringField(recordpayload.PayloadKeyStatus, "completed"),
		workorcheventpayload.StringField(recordpayload.PayloadKeyPhysicalResponseMode, string(llmaudit.PhysicalResponseModeNonStreaming)),
	)
	if _, ok := llmUsageTraceFromEventRecord(projectioncontract.EventRecord{ID: "missing-submission", WorkspaceID: "ws-current", RunID: "run-current", Payload: missingSubmission}); ok {
		t.Fatal("LLM usage trace accepted a missing provider submission fact")
	}

	partialIdentity := currentPayload.Merge(workorcheventpayload.New(
		workorcheventpayload.StringField(recordpayload.PayloadKeyLogicalRequestID, "logical-partial"),
	))
	if _, ok := llmUsageTraceFromEventRecord(projectioncontract.EventRecord{ID: "partial-identity", WorkspaceID: "ws-current", RunID: "run-current", Payload: partialIdentity}); ok {
		t.Fatal("LLM usage trace accepted a partial two-level request identity")
	}

	mismatchedCorrelation := currentPayload.Merge(workorcheventpayload.New(
		workorcheventpayload.StringField(recordpayload.PayloadKeyRequestID, "physical-other"),
		workorcheventpayload.StringField(recordpayload.PayloadKeyLogicalRequestID, "logical-current"),
		workorcheventpayload.StringField(recordpayload.PayloadKeyPhysicalCallID, "physical-current"),
	))
	if _, ok := llmUsageTraceFromEventRecord(projectioncontract.EventRecord{ID: "mismatched-correlation", WorkspaceID: "ws-current", RunID: "run-current", Payload: mismatchedCorrelation}); ok {
		t.Fatal("LLM usage trace accepted physical call identity that differs from correlation request")
	}
}

func TestLLMUsageEqualityIncludesTwoLevelRequestIdentity(t *testing.T) {
	base := projectioncontract.LLMUsageTrace{ID: "usage", LogicalRequestID: "logical", PhysicalCallID: "physical"}
	if !llmUsageEqual(base, base) {
		t.Fatal("equal LLM usage trace was rejected")
	}
	changed := base
	changed.LogicalRequestID = "logical-other"
	if llmUsageEqual(base, changed) {
		t.Fatal("LLM usage equality ignored logical request identity")
	}
	changed = base
	changed.PhysicalCallID = "physical-other"
	if llmUsageEqual(base, changed) {
		t.Fatal("LLM usage equality ignored physical call identity")
	}
}
