package events

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	llmmodel "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func catalogCacheResultFixture() ObserveProviderCatalogCacheResult {
	now := time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)
	return ObserveProviderCatalogCacheResult{WorkspaceID: "ws", OwnerEpoch: "llm-preparation-epoch:ABCDEFGHIJKLMNOPQRSTUVWXYZ", ObservedAt: now,
		Catalog: llmmodel.ProviderModelCatalog{ProviderID: "ai-proxy", Source: "ai_proxy", Revision: "catalog-sha256:fixture", FetchedAt: now.Add(-time.Second), ExpiresAt: now.Add(time.Minute), State: llmmodel.ProviderCatalogStateFresh}}
}

func TestPreparationCatalogCacheResultExactBindingFreshnessAndCompleteness(t *testing.T) {
	command := ObserveProviderCatalogCacheCommand{WorkspaceID: "ws"}
	for name, mutate := range map[string]func(*ObserveProviderCatalogCacheResult){
		"scope":         func(r *ObserveProviderCatalogCacheResult) { r.WorkspaceID = "other" },
		"missing_epoch": func(r *ObserveProviderCatalogCacheResult) { r.OwnerEpoch = "" },
		"empty_epoch":   func(r *ObserveProviderCatalogCacheResult) { r.OwnerEpoch = "llm-preparation-epoch:" },
		"invalid_epoch": func(r *ObserveProviderCatalogCacheResult) {
			r.OwnerEpoch = "llm-preparation-epoch:" + strings.Repeat("0", 26)
		},
		"wrong_epoch_kind": func(r *ObserveProviderCatalogCacheResult) { r.OwnerEpoch = "target-config-sha256:credential-derived" },
		"unknown_state":    func(r *ObserveProviderCatalogCacheResult) { r.Catalog.State = "ready" },
		"expired_fresh":    func(r *ObserveProviderCatalogCacheResult) { r.Catalog.ExpiresAt = r.ObservedAt },
		"future_fetched":   func(r *ObserveProviderCatalogCacheResult) { r.Catalog.FetchedAt = r.ObservedAt.Add(time.Second) },
		"zero_observed":    func(r *ObserveProviderCatalogCacheResult) { r.ObservedAt = time.Time{} },
		"diagnostic":       func(r *ObserveProviderCatalogCacheResult) { r.Catalog.ErrorMessage = "private diagnostic" },
		"mixed_state":      func(r *ObserveProviderCatalogCacheResult) { r.Catalog.Stale = true },
		"missing_revision": func(r *ObserveProviderCatalogCacheResult) { r.Catalog.Revision = "" },
		"model_limit": func(r *ObserveProviderCatalogCacheResult) {
			r.Catalog.Models = make([]llmmodel.ProviderModelDescriptor, CatalogCacheObservationModelLimit+1)
		},
		"miss_with_facts": func(r *ObserveProviderCatalogCacheResult) {
			r.Catalog.State = llmmodel.ProviderCatalogStateUnavailable
			r.Catalog.ErrorCode = "cache_miss"
		},
	} {
		t.Run(name, func(t *testing.T) {
			result := catalogCacheResultFixture()
			if err := result.ValidateFor(command); err != nil {
				t.Fatal(err)
			}
			mutate(&result)
			if result.ValidateFor(command) == nil {
				t.Fatal("malformed or fabricated fresh cache accepted")
			}
		})
	}
	stale := catalogCacheResultFixture()
	stale.ObservedAt = stale.Catalog.ExpiresAt
	stale.Catalog.State, stale.Catalog.Stale, stale.Catalog.ErrorCode = llmmodel.ProviderCatalogStateStale, true, "cache_expired"
	if stale.ValidateFor(command) != nil {
		t.Fatal("exact-expiry stale observation rejected")
	}
	missing := catalogCacheResultFixture()
	missing.Catalog = llmmodel.ProviderModelCatalog{ProviderID: "ai-proxy", Source: "ai_proxy", State: llmmodel.ProviderCatalogStateUnavailable, ErrorCode: "cache_miss"}
	if missing.ValidateFor(command) != nil {
		t.Fatal("explicit cache miss rejected")
	}
	if (ObserveProviderCatalogCacheCommand{WorkspaceID: "\x00ws"}).Validate() == nil {
		t.Fatal("invalid query scope accepted")
	}
	malformed := catalogCacheResultFixture()
	malformed.Catalog.Revision = strings.Repeat("r", 129)
	if malformed.ValidateFor(command) == nil {
		t.Fatal("unbounded identity accepted")
	}
}

func catalogCacheModelFixture() llmmodel.ProviderModelDescriptor {
	return llmmodel.ProviderModelDescriptor{
		Target:     runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "embedding"},
		RouteOwner: "ai_proxy", CapabilityRevision: "capability:fixture",
		SupportedEndpoints: []string{"/v1/embeddings"}, Operations: []string{"embedding"},
		Reasoning: llmmodel.ProviderReasoningCapability{Valid: true, Efforts: []string{"low", "high"}},
	}
}

func TestPreparationCatalogCacheClonePreservesEmptyAndNilLists(t *testing.T) {
	for _, value := range []ObserveProviderCatalogCacheResult{
		{},
		{Catalog: llmmodel.ProviderModelCatalog{Models: []llmmodel.ProviderModelDescriptor{}}},
		{Catalog: llmmodel.ProviderModelCatalog{Models: []llmmodel.ProviderModelDescriptor{{SupportedEndpoints: []string{}, Operations: []string{}, Reasoning: llmmodel.ProviderReasoningCapability{Efforts: []string{}}}}}},
	} {
		if !reflect.DeepEqual(value, value.Clone()) {
			t.Fatal("detached clone normalized empty lists into nil")
		}
	}
}

func TestPreparationCatalogCacheBoundsPreserveCompleteOriginalDeclaration(t *testing.T) {
	command := ObserveProviderCatalogCacheCommand{WorkspaceID: "ws"}
	base := catalogCacheResultFixture()
	base.Catalog.Models = []llmmodel.ProviderModelDescriptor{catalogCacheModelFixture()}
	before := base.Clone()
	if err := base.ValidateFor(command); err != nil {
		t.Fatalf("zero context/output embedding metadata must remain observable: %v", err)
	}
	clone := base.Clone()
	clone.Catalog.Models[0].Reasoning.Efforts[0] = "caller"
	clone.Catalog.Models[0].Operations[0] = "caller"
	clone.Catalog.Models[0].SupportedEndpoints[0] = "caller"
	if !reflect.DeepEqual(base, before) {
		t.Fatal("clone aliases or normalizes original capability declaration")
	}
	for name, mutate := range map[string]func(*ObserveProviderCatalogCacheResult){
		"duplicate": func(r *ObserveProviderCatalogCacheResult) {
			r.Catalog.Models = append(r.Catalog.Models, r.Catalog.Models[0])
		},
		"scope":            func(r *ObserveProviderCatalogCacheResult) { r.Catalog.Models[0].Target.ProviderID = "other" },
		"raw_identity":     func(r *ObserveProviderCatalogCacheResult) { r.Catalog.Models[0].Target.ModelID += " " },
		"negative_context": func(r *ObserveProviderCatalogCacheResult) { r.Catalog.Models[0].ContextWindowTokens = -1 },
		"negative_output":  func(r *ObserveProviderCatalogCacheResult) { r.Catalog.Models[0].MaxOutputTokens = -1 },
		"invalid_utf8":     func(r *ObserveProviderCatalogCacheResult) { r.Catalog.Models[0].RouteOwner = string([]byte{0xff}) },
		"long_text": func(r *ObserveProviderCatalogCacheResult) {
			r.Catalog.Models[0].Target.ModelID = strings.Repeat("x", 257)
		},
		"long_list":       func(r *ObserveProviderCatalogCacheResult) { r.Catalog.Models[0].Operations = make([]string, 65) },
		"bad_nested_text": func(r *ObserveProviderCatalogCacheResult) { r.Catalog.Models[0].Reasoning.Efforts[0] = "low\n" },
		"complete_byte_bound": func(r *ObserveProviderCatalogCacheResult) {
			r.Catalog.Models = make([]llmmodel.ProviderModelDescriptor, 128)
			for index := range r.Catalog.Models {
				model := catalogCacheModelFixture()
				model.Target.ModelID = fmt.Sprintf("model-%d", index)
				model.Operations = make([]string, 64)
				for entry := range model.Operations {
					model.Operations[entry] = strings.Repeat("x", 256)
				}
				r.Catalog.Models[index] = model
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			value := base.Clone()
			mutate(&value)
			original := value.Clone()
			if value.ValidateFor(command) == nil {
				t.Fatal("invalid complete declaration accepted")
			}
			if !reflect.DeepEqual(value, original) {
				t.Fatal("validation rewrote original input")
			}
		})
	}
}
