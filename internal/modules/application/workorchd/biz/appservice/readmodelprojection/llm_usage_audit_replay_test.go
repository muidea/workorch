package readmodelprojection

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/llmaudit"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
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
	newRecord := func(id, occurredAt string) readmodelcore.EventRecord {
		payload := (events.LLMUsagePayloadData{
			RequestID: id, WorkspaceID: "ws-audit", RunID: "run-audit", StepID: "authority-1",
			ProviderID: "ai-proxy", Model: "gpt-4o", Operation: "runtime_candidate_preflight",
			Kind: "runtime_candidate_preflight", ProviderSubmitted: false,
			Status: "failed", FailureClass: "context_integrity", ContextBudget: budget,
		}).EventPayload()
		// A persisted event can contain unrelated fields from a future/bad
		// producer. Projection must not turn them into a prompt transport.
		payload = payload.Merge(workorcheventpayload.New(workorcheventpayload.StringField("prompt", secret)))
		return readmodelcore.EventRecord{
			ID: id, Topic: llmevents.TopicLLMUsageRecorded, OccurredAt: occurredAt,
			WorkspaceID: "ws-audit", RunID: "run-audit", StepID: "authority-1", Payload: payload,
		}
	}
	first := newRecord("audit-event-1", "2026-07-25T00:00:00Z")
	later := newRecord("audit-event-2", "2026-07-25T00:00:01Z")
	traces := MapLLMUsageTracesFromEventRecords([]readmodelcore.EventRecord{first, first, later}, "run-audit")
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
	encoded, err := json.Marshal(traces)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), secret) {
		t.Fatalf("read projection leaked prompt content: %s", encoded)
	}
}
