package events

import (
	"reflect"
	"testing"
)

func TestPreparationProviderConfigurationDetachedAndFinite(t *testing.T) {
	fixture := func() ObserveProviderCatalogCacheResult {
		r := catalogCacheResultFixture()
		r.Configuration = &PreparationProviderConfiguration{Version: PreparationProviderConfigurationSnapshotVersion, DefaultChat: catalogCacheModelFixture().Target, Overrides: []PreparationProviderModelOverride{{ModelID: "model", ContextWindowTokens: 32000, MaxOutputTokens: 4000, Operations: []string{"responses"}}}}
		return r
	}
	command := ObserveProviderCatalogCacheCommand{WorkspaceID: "ws"}
	original := fixture()
	if original.ValidateFor(command) != nil {
		t.Fatal("valid configuration declaration rejected")
	}
	clone := original.Clone()
	clone.Configuration.Overrides[0].Operations[0] = "embeddings"
	clone.Configuration.DefaultChat.ModelID = "caller"
	if !reflect.DeepEqual(original, fixture()) {
		t.Fatal("configuration clone aliases owner values")
	}
	for _, change := range []string{"version", "default", "raw_default", "provider", "duplicate", "unsorted", "context", "output", "operation", "duplicate_operation", "limit"} {
		t.Run(change, func(t *testing.T) {
			r := fixture()
			c := r.Configuration
			switch change {
			case "version":
				c.Version = "unknown"
			case "default":
				c.DefaultChat.ModelID = ""
			case "raw_default":
				c.DefaultChat.ModelID += " "
			case "provider":
				c.DefaultChat.ProviderID = "other"
			case "duplicate":
				c.Overrides = append(c.Overrides, c.Overrides[0])
			case "unsorted":
				c.Overrides = append(c.Overrides, PreparationProviderModelOverride{ModelID: "aaa", ContextWindowTokens: 1, MaxOutputTokens: 1, Operations: []string{"responses"}})
			case "context":
				c.Overrides[0].ContextWindowTokens = 0
			case "output":
				c.Overrides[0].MaxOutputTokens = -1
			case "operation":
				c.Overrides[0].Operations = []string{"unknown"}
			case "duplicate_operation":
				c.Overrides[0].Operations = []string{"responses", "responses"}
			case "limit":
				c.Overrides = make([]PreparationProviderModelOverride, CatalogCacheObservationModelLimit+1)
			}
			before := r.Clone()
			if r.ValidateFor(command) == nil || !reflect.DeepEqual(before, r) {
				t.Fatal("invalid declaration accepted or rewritten")
			}
		})
	}
	legacy := catalogCacheResultFixture()
	if legacy.ValidateFor(command) != nil || legacy.Configuration != nil {
		t.Fatal("old history upgraded or rejected")
	}
}
