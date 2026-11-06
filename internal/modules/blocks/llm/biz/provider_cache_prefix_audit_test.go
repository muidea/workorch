package llm

import (
	"encoding/json"
	"reflect"
	"testing"

	models "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
)

func TestCacheBreakpointAuditSeparatesBodyMetadataAndRemoteFacts(t *testing.T) {
	render := func(observation, tail string) *models.ProviderContextBudget {
		r := sectionCacheRequest("ws-exact", "work")
		r.PromptContext.Sections[3].ID = "C3.semantic-authority"
		r.PromptContext.Sections[3].Content = `{"workspaceId":"ws-exact","workContext":{"content":{"Snapshots":[{"TargetRef":"a","Content":"原文\\r\\n"}],"SnapshotObservations":[{"TargetRef":"a","ObservedAt":"` + observation + `"}],"ManifestRevision":"revision"},"priorRounds":null},"manifest":"current"}`
		r.PromptContext.Sections = append(r.PromptContext.Sections, models.PromptSection{ID: "C4.tail", Role: "user", Content: tail})
		wire := responsesRequest{Model: "model", Input: responsesWireInputForRequest(&r), PromptCacheKey: responsesPromptCacheKey(r), PromptCacheOptions: responsesPromptCacheOptions(r)}
		observeResponsesCacheWire(r.ContextBudget, wire)
		return r.ContextBudget
	}
	base := render("time-a", "attempt0")
	tail := render("time-a", "attempt1")
	metadata := render("time-b", "attempt0")
	if len(base.WireCachePrefixes) != 4 || base.WireCacheBreakpoints != 4 || !reflect.DeepEqual(base.WireCachePrefixes, tail.WireCachePrefixes) {
		t.Fatal("tail or breakpoints lost", base.WireCachePrefixes)
	}
	for i, p := range base.WireCachePrefixes {
		if p.WireBytes <= 0 || p.Hash == "" || i > 0 && p.WireBytes <= base.WireCachePrefixes[i-1].WireBytes {
			t.Fatal("invalid prefix accounting")
		}
		if (p == metadata.WireCachePrefixes[i]) != (i < 2) {
			t.Fatal("body boundary includes changing metadata", i)
		}
	}
	if base.WireCachePrefixes[3].Hash != base.WireCachePrefixHash {
		t.Fatal("last prefix and existing fingerprint diverged")
	}
	clone := cloneProviderContextBudget(base)
	clone.WireCachePrefixes[0].Hash = "changed"
	if clone.WireCachePrefixes[0].Hash == base.WireCachePrefixes[0].Hash {
		t.Fatal("audit clone shares mutable backing")
	}
	observation := providerContextBudgetAudit(base).CacheObservation
	if !reflect.DeepEqual(observation.WireCachePrefixes, base.WireCachePrefixes) {
		t.Fatal("owner dropped per-boundary observations")
	}
	payload := observation.Payload()
	field, ok := payload.Field("wireCachePrefixes")
	if !ok || len(field.Array) != 4 {
		t.Fatal("event payload lost breakpoints")
	}
	if effective, _ := payload.String("effectiveHints"); effective != "unknown" {
		t.Fatal("local breakpoint claimed remote support")
	}
	b, _ := json.Marshal(payload)
	if !json.Valid(b) {
		t.Fatal("invalid typed audit payload")
	}
	observeResponsesCacheWire(base, responsesRequest{})
	if len(base.WireCachePrefixes) != 0 || base.WireCachePrefixHash != "" {
		t.Fatal("stale request observations retained")
	}
}
