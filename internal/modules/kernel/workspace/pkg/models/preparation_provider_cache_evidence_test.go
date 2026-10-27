package workspace

import (
	"reflect"
	"strings"
	"testing"
	"time"

	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
	llmmodel "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestPreparationProviderCacheEvidenceCompleteValueCloneAndRejection(t *testing.T) {
	now := time.Now().UTC()
	f := PreparationProviderCacheFacts{Version: PreparationProviderCacheEvidenceVersion, WorkspaceRevision: 2, Observation: llmevents.ObserveProviderCatalogCacheResult{WorkspaceID: "ws", OwnerEpoch: "llm-preparation-epoch:" + strings.Repeat("A", 26), ObservedAt: now, Catalog: llmmodel.ProviderModelCatalog{ProviderID: "ai-proxy", Source: "ai_proxy", State: llmmodel.ProviderCatalogStateFresh, Revision: "original", FetchedAt: now.Add(-time.Minute), ExpiresAt: now.Add(time.Minute), Models: []llmmodel.ProviderModelDescriptor{{Target: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "embedding"}, RouteOwner: "ai-proxy", CapabilityRevision: "cap-original", SupportedEndpoints: []string{"/v1/embeddings"}, Operations: []string{"embedding"}, Reasoning: llmmodel.ProviderReasoningCapability{Efforts: []string{"原声明"}, InvalidReason: "original-not-normalized"}}}}}}
	e := WorkspacePreparationProviderCacheEvidence{Version: PreparationProviderCacheEvidenceVersion, Ref: PreparationProviderCacheRefPrefix + strings.Repeat("a", 32), RequestID: "original", PayloadHash: "payload", Binding: preparationRecordFixture().Attempts[0].Binding, Facts: f}
	e.ContentHash = e.Hash()
	if e.Validate() != nil || f.BlockedReason() != "provider_declaration_only" || f.Observation.Catalog.Models[0].ContextWindowTokens != 0 {
		t.Fatal("complete declaration changed or optional embedding ceiling made mandatory")
	}
	copy := e.Clone()
	copy.Facts.Observation.Catalog.Models[0].SupportedEndpoints[0] = "caller"
	copy.Facts.Observation.Catalog.Models[0].Operations[0] = "caller"
	copy.Facts.Observation.Catalog.Models[0].Reasoning.Efforts[0] = "caller"
	if e.Validate() != nil || !reflect.DeepEqual(e.Facts, f) {
		t.Fatal("clone normalized or aliased the original owner value")
	}
	for _, change := range []func(*WorkspacePreparationProviderCacheEvidence){
		func(e *WorkspacePreparationProviderCacheEvidence) { e.Ref = e.ContentHash },
		func(e *WorkspacePreparationProviderCacheEvidence) {
			e.Facts.Observation.OwnerEpoch = "credential-derived"
		},
		func(e *WorkspacePreparationProviderCacheEvidence) {
			e.Facts.Observation.Catalog.Models = append(e.Facts.Observation.Catalog.Models, e.Facts.Observation.Catalog.Models[0])
		},
		func(e *WorkspacePreparationProviderCacheEvidence) {
			e.Facts.Observation.Catalog.State = llmmodel.ProviderCatalogStateStale
		},
		func(e *WorkspacePreparationProviderCacheEvidence) {
			e.Facts.Observation.Catalog.FetchedAt = now.Add(time.Second)
		},
		func(e *WorkspacePreparationProviderCacheEvidence) { e.Binding.StartupInputHash = "" },
		func(e *WorkspacePreparationProviderCacheEvidence) { e.Facts.WorkspaceRevision = 0 },
	} {
		bad := e.Clone()
		change(&bad)
		bad.ContentHash = bad.Hash()
		if bad.Validate() == nil {
			t.Fatal("invalid complete cache observation trusted after rehash")
		}
	}
	f.Observation.Catalog.State, f.Observation.Catalog.Stale, f.Observation.Catalog.ErrorCode = llmmodel.ProviderCatalogStateStale, true, "cache_expired"
	f.Observation.ObservedAt = now.Add(2 * time.Minute)
	if f.Validate() != nil || f.BlockedReason() != "provider_cache_expired" {
		t.Fatal("expired historical observation reclassified or credited")
	}
	f.Observation.Catalog = llmmodel.ProviderModelCatalog{ProviderID: "ai-proxy", Source: "ai_proxy", State: llmmodel.ProviderCatalogStateUnavailable, ErrorCode: "cache_miss"}
	if f.Validate() != nil || f.BlockedReason() != "provider_cache_miss" {
		t.Fatal("actual complete miss not preserved")
	}
}

func TestPreparationProviderExpiryWithoutOriginalMaterialNeverGrantsCurrentness(t *testing.T) {
	now := time.Now().UTC()
	for _, w := range []*Workspace{nil, {}, {ID: "ws"}} {
		if _, err := PreparationProviderCacheExpired(w, now); err == nil {
			t.Fatal("invalid scope/revision accepted")
		}
	}
	w := &Workspace{ID: "ws", Revision: 2}
	if _, err := PreparationProviderCacheExpired(w, time.Time{}); err == nil {
		t.Fatal("unknown time accepted")
	}
	if expired, err := PreparationProviderCacheExpired(w, now); err != nil || expired {
		t.Fatal("absent operation invented an observation", err)
	}
	r := preparationRecordFixture()
	w.StartupPreparation = &r
	before := r.Clone()
	if expired, err := PreparationProviderCacheExpired(w, now.Add(24*time.Hour)); err != nil || expired || !reflect.DeepEqual(r, before) || len(w.PreparationProviderCacheEvidence) != 0 {
		t.Fatal("legacy record without material was upgraded or mutated", err)
	}
}
