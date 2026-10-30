package readmodelprojection

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/recordpayload"
	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
	"github.com/muidea/workorch/internal/pkg/llmaudit"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func TestCacheAuditSurvivesRealIngressAndProjection(t *testing.T) {
	for _, workspace := range []string{"workspace-A", "workspace-B"} {
		t.Run(workspace, func(t *testing.T) {
			t.Parallel()
			for _, known := range []bool{false, true} {
				budget := llmaudit.ContextBudget{
					ProviderID: "provider", ModelID: "model", CapabilityRevision: "cap-rev", ConfigurationRevision: "config-rev",
					ContextWindowTokens: 128000, ProviderMaxOutputTokens: 16384, EffectiveMaxOutputTokens: 4096,
					MaxBytes: 512000, ActualBytes: 200000, MaxTokens: 120000, EstimatedInputTokens: 50000, EstimatedOutputTokens: 4096,
					ActualTokens: 50100, MaxRequestBytes: 512000, ProfileMode: "typed-ceiling", CapabilitySource: "sealed_ceiling",
					ProviderSubmitted: true, StablePrefixHash: "runtime-prefix", RenderedStablePrefixHash: "rendered-prefix", CacheStrategy: "stable_prefix_dynamic_tail",
					IncludedSections: []string{"C0", "C1", "C2", "C3", "C4", "C5"}, ProtectedFields: []string{"original-criteria"},
					IntegrityWarnings: []string{"known-warning"}, SourceRefs: []string{"runtime://definition/rev"},
					CacheObservation: llmaudit.CacheObservation{Renderer: "workorch.runtime_candidate_prompt/v2", MessageLayout: "section_messages/v1", Stage: "NeedStateChange", MetadataState: "complete", SourceSectionCount: 6, RecordedSectionCount: 6, RequestedHints: "explicit_prefix_breakpoint"},
				}
				for i := 0; i < 6; i++ {
					budget.SectionHashes = append(budget.SectionHashes, llmaudit.ContextSection{ID: fmt.Sprintf("C%d", i), Hash: fmt.Sprintf("sha256:%s-%d", workspace, i), SizeBytes: 100 + i, SourceRefs: []string{fmt.Sprintf("runtime://section/%d", i)}})
				}
				physical := "physical-" + workspace
				// Populate every optional top-level field, including failureClass: the
				// test must go through Eventstream's real field/depth/byte ceiling.
				raw := (recordpayload.LLMUsagePayloadData{
					RequestID: physical, LogicalRequestID: "logical-" + workspace, PhysicalCallID: physical,
					WorkspaceID: workspace, RunID: "run", StepID: "authority", ProviderID: "provider", Provider: "adapter", APISurface: "responses", Model: "model", Operation: "responses", CapabilityRevision: "cap-rev", ConfigurationRevision: "config-rev", StablePrefixHash: "runtime-prefix", Kind: "runtime_candidate",
					InputTokens: 50000, OutputTokens: 100, TotalTokens: 50100, UsageSource: "provider_native", RequestBytes: 200000, ResponseBytes: 400, PhysicalResponseMode: llmaudit.PhysicalResponseModeNonStreaming, ContextBudget: budget,
					CacheUsage: llmaudit.CacheUsage{CacheReadTokensKnown: known, Source: "provider_native"}, StartedAt: "2026-10-30T13:00:00Z", CompletedAt: "2026-10-30T13:00:01Z", DurationMs: 1000, Status: "failed", FailureClass: "contract", ProviderSubmitted: true,
				}).EventPayload()
				sanitized := recordpayload.SanitizeAuditPayload(raw)
				if truncated, _ := sanitized.Bool("auditPayloadTruncated"); truncated {
					t.Fatalf("ordinary complete audit was truncated: %s", sanitized.CanonicalString())
				}
				record := projectioncontract.EventRecord{ID: "event", Topic: llmevents.TopicLLMUsageRecorded, WorkspaceID: workspace, RunID: "run", Payload: sanitized}
				duplicate := record
				duplicate.ID = "redelivered-event"
				traces := mapLLMUsageTracesFromEventRecords([]projectioncontract.EventRecord{record, record, duplicate}, "run")
				if len(traces) != 1 {
					t.Fatalf("physical call counted more than once: %d", len(traces))
				}
				got := traces[0]
				if got.PhysicalCallID != physical || got.WorkspaceID != workspace || got.InputTokens != 50000 || got.FailureClass != "contract" {
					t.Fatalf("audit correlation lost: %+v", got)
				}
				if sections := llmContextBudgetSectionHashes(got.ContextBudget); !reflect.DeepEqual(sections, budget.SectionHashes) {
					t.Fatalf("six sections changed: %+v", sections)
				}
				if fields := llmContextBudgetProtectedFields(got.ContextBudget); len(fields) != 1 || fields[0] != "original-criteria" {
					t.Fatalf("processing group lost: %v", fields)
				}
				observation, _ := got.ContextBudget.Object("cacheObservation")
				if effective, _ := observation.String("effectiveHints"); effective != "unknown" {
					t.Fatal("client hint became gateway support")
				}
				if renderer, _ := observation.String("renderer"); renderer != budget.CacheObservation.Renderer {
					t.Fatal("renderer lost")
				}
				if isKnown := llmUsageCacheMeasurementKnown(got.CacheUsage, "cacheReadTokens"); isKnown != known {
					t.Fatalf("unknown became a known zero: %v", isKnown)
				}
			}
		})
	}
}

func TestCacheAuditProjectionKeepsLegacyAndClosesNewGroups(t *testing.T) {
	raw := workorcheventpayload.New(
		workorcheventpayload.StringArrayField("protectedFields", []string{"legacy"}),
		workorcheventpayload.ObjectField("processing", workorcheventpayload.New(workorcheventpayload.StringArrayField("includedSections", []string{"C0"}), workorcheventpayload.StringField("prompt", "private"))),
		workorcheventpayload.ObjectField("cacheObservation", workorcheventpayload.New(workorcheventpayload.StringField("metadataState", "bounded"), workorcheventpayload.StringField("prompt", "private"))),
	)
	got := sanitizeLLMContextBudgetAudit(raw)
	if fields := llmContextBudgetProtectedFields(got); len(fields) != 1 || fields[0] != "legacy" {
		t.Fatal("legacy event was reinterpreted")
	}
	for _, name := range []string{"processing", "cacheObservation"} {
		v, _ := got.Object(name)
		if _, ok := v.Field("prompt"); ok {
			t.Fatal("unknown raw nested field escaped")
		}
	}
}

func TestCacheAuditDoesNotHideIngressTruncation(t *testing.T) {
	raw := workorcheventpayload.New(
		workorcheventpayload.ObjectField("contextBudget", llmaudit.ContextBudget{ProviderID: "provider"}.Payload()),
		workorcheventpayload.StringField("oversized", strings.Repeat("x", 513)),
	)
	sanitized := recordpayload.SanitizeAuditPayload(raw)
	budget := llmUsageTraceContextBudget(sanitized)
	if truncated, _ := budget.Bool("auditPayloadTruncated"); !truncated {
		t.Fatal("ingress incompleteness vanished in public usage view")
	}
	observation, _ := budget.Object("cacheObservation")
	if state, _ := observation.String("metadataState"); state != "unavailable" {
		t.Fatal("missing metadata claimed completeness")
	}
}

func TestNativeCacheSummaryUsesPhysicalCallsAndMeasuredDenominators(t *testing.T) {
	trace := func(id string, submitted, known bool, input, read int) projectioncontract.LLMUsageTrace {
		return projectioncontract.LLMUsageTrace{ID: id, WorkspaceID: "ws", RunID: "run", LogicalRequestID: "logical", PhysicalCallID: id, ProviderSubmitted: submitted, UsageSource: "provider_native", InputTokens: input, CacheUsage: (llmaudit.CacheUsage{CacheReadTokensKnown: known, CacheReadTokens: read, Source: "provider_native"}).Payload()}
	}
	hit := trace("hit", true, true, 100, 80)
	redelivery := hit
	redelivery.ID = "other-notification"
	preflight := trace("preflight", false, false, 0, 0)
	miss := trace("miss", true, true, 200, 0)
	unknown := trace("unknown", true, false, 700, 0)
	estimated := trace("estimated", true, true, 900, 400)
	estimated.Estimated = true
	native := summarizeLLMUsage([]projectioncontract.LLMUsageTrace{hit, redelivery, preflight, miss, unknown, estimated}).NativeCache
	if native.PhysicalCalls != 4 || native.MeasuredCalls != 2 || native.UnknownCalls != 2 || native.HitCalls != 1 || native.MeasuredInputTokens != 300 || native.CacheReadTokens != 80 || native.UncachedInputTokens != 220 || native.HitRequestRatio != 0.5 || native.CachedInputTokenRatio != float64(80)/300 {
		t.Fatalf("measurement scopes mixed: %+v", native)
	}
}
