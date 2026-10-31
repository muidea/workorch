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

func preparationProviderSelectionFixture() (PreparationProviderSelectionDefinition, WorkspacePreparationProviderCacheEvidence) {
	w := evaluationWorkspaceFixture()
	a := w.StartupPreparation.Attempts[0]
	now := time.Now().UTC()
	target := runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "exact-model"}
	e := WorkspacePreparationProviderCacheEvidence{Version: PreparationProviderCacheEvidenceVersion, Ref: PreparationProviderCacheRefPrefix + strings.Repeat("b", 32), RequestID: a.RequestID, PayloadHash: a.PayloadHash, Binding: a.Binding,
		Facts: PreparationProviderCacheFacts{Version: PreparationProviderCacheEvidenceVersion, WorkspaceRevision: a.ExpectedWorkspaceRevision + 1, Observation: llmevents.ObserveProviderCatalogCacheResult{WorkspaceID: "ws", OwnerEpoch: "llm-preparation-epoch:ABCDEFGHIJKLMNOPQRSTUVWXYZ", ObservedAt: now,
			Configuration: &llmevents.PreparationProviderConfiguration{Version: llmevents.PreparationProviderConfigurationSnapshotVersion, DefaultChat: target},
			Catalog:       llmmodel.ProviderModelCatalog{ProviderID: "ai-proxy", Source: "ai_proxy", State: llmmodel.ProviderCatalogStateFresh, Revision: "catalog-exact", FetchedAt: now.Add(-time.Second), ExpiresAt: now.Add(time.Minute), Models: []llmmodel.ProviderModelDescriptor{{Target: target, RouteOwner: "ai_proxy", CapabilityRevision: "capability-exact", ContextWindowTokens: 32000, MaxOutputTokens: 4000, CapabilityReady: true, Operations: []string{"responses"}, SupportedEndpoints: []string{"/v1/responses"}, Reasoning: llmmodel.ProviderReasoningCapability{Supported: true, Valid: true, Efforts: []string{"low", "high"}}}}}}}}
	e.ContentHash = e.Hash()
	return PreparationProviderSelectionDefinition{ConfigurationHash: a.Binding.AuthoringConfigurationHash}, e
}

func TestPreparationProviderSelectionReproducedNotHashTrusted(t *testing.T) {
	d, e := preparationProviderSelectionFixture()
	c := CompilePreparationProviderSelection(d, e)
	e.Selection = &c
	e.ContentHash = e.Hash()
	if e.Validate() != nil {
		t.Fatal("valid private compilation rejected")
	}
	clone := e.Clone()
	clone.Selection.Definition.WorkspaceDefault = &runtimecontract.ProviderInferenceSelection{Target: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "caller"}}
	clone.Facts.Observation.Configuration.DefaultChat.ModelID = "caller"
	if e.Facts.Observation.Configuration.DefaultChat.ModelID != "exact-model" {
		t.Fatal("clone aliases source")
	}
	for _, change := range []string{"version", "target", "source", "epoch", "catalog", "failure"} {
		bad := e.Clone()
		switch change {
		case "version":
			bad.Selection.Version = "unknown"
		case "target":
			bad.Selection.Selection.Target.ModelID = "neighbor"
		case "source":
			bad.Selection.Source = "caller"
		case "epoch":
			bad.Selection.OwnerEpoch = "llm-preparation-epoch:BBBBBBBBBBBBBBBBBBBBBBBBBB"
		case "catalog":
			bad.Selection.CatalogRevision = "other"
		case "failure":
			bad.Selection.FailureCode = "operation_unsupported"
		}
		bad.ContentHash = bad.Hash()
		if bad.Validate() == nil {
			t.Fatal("self-rehashed substitution accepted", change)
		}
	}
}

func TestPreparationProviderSelectionOriginalDeclarationBytesPreserved(t *testing.T) {
	d, e := preparationProviderSelectionFixture()
	original := runtimecontract.ProviderInferenceSelection{Target: runtimecontract.ProviderModelTarget{ProviderID: " ai-proxy ", ModelID: " exact-model "}, ReasoningEffort: " high "}
	w := &Workspace{DefaultLLMSelection: &original}
	d = PreparationProviderDefinition(w)
	d.ConfigurationHash = e.Binding.AuthoringConfigurationHash
	c := CompilePreparationProviderSelection(d, e)
	if c.FailureCode != "" || c.Selection != original.Normalized() || !reflect.DeepEqual(c.Definition.WorkspaceDefault, &original) || !reflect.DeepEqual(d.WorkspaceDefault, &original) {
		t.Fatal("effective selection normalization rewrote the original declaration")
	}
	e.Selection = &c
	e.ContentHash = e.Hash()
	if e.Validate() != nil {
		t.Fatal("original declaration cannot be independently recompiled")
	}
	cloned := e.Clone()
	if !reflect.DeepEqual(cloned, e) {
		t.Fatal("cloning changed archived declaration bytes")
	}
	cloned.Selection.Definition.WorkspaceDefault.Target.ModelID = "caller"
	if c.Definition.WorkspaceDefault.Target.ModelID != " exact-model " || original.Target.ModelID != " exact-model " {
		t.Fatal("archived definition aliases caller input")
	}
}

func TestPreparationProviderSelectionFiniteReasonsAndOriginalRecordShape(t *testing.T) {
	for _, code := range []string{"authorization_pending", "ready", "model_missing:raw-config", "unknown"} {
		c := PreparationProviderSelectionCompilation{Version: PreparationProviderConfigurationSelectionVersion, FailureCode: code}
		if c.BlockedReason() != "" {
			t.Fatal("invented failure suffix accepted", code)
		}
	}
	for _, reason := range []string{"provider_selection_authorization_pending", "provider_selection_model_missing", "provider_selection_configuration_source_unavailable"} {
		fixture := func() WorkspacePreparationRecord {
			w := evaluationWorkspaceFixture()
			a := &w.StartupPreparation.Attempts[0]
			a.ProviderCacheEvidenceRef = PreparationProviderCacheRefPrefix + strings.Repeat("b", 32)
			a.Checks = []WorkspacePreparationCheckResult{{ID: "provider_cache", Owner: "llm", Required: true, State: "blocked", Reason: reason}}
			return w.StartupPreparation.Clone()
		}
		if fixture().Validate("ws") != nil {
			t.Fatal("valid bound classification rejected", reason)
		}
		for _, change := range []string{"ref", "version", "owner", "required", "passed", "failed"} {
			r := fixture()
			a := &r.Attempts[0]
			switch change {
			case "ref":
				a.ProviderCacheEvidenceRef = ""
			case "version":
				a.EvaluationVersion, a.ObservationSnapshotHash = "", ""
			case "owner":
				a.Checks[0].Owner = "caller"
			case "required":
				a.Checks[0].Required = false
			case "passed":
				a.Checks[0].State = "passed"
			case "failed":
				a.Checks[0].State = "failed"
			}
			if r.Validate("ws") == nil {
				t.Fatal("unbound classification accepted", reason, change)
			}
		}
	}
}

func TestPreparationProviderConfigurationUsesFrozenSelectionWithoutCatalogGrant(t *testing.T) {
	for _, scenario := range []string{"cold", "expired", "workspace", "override", "unsupported_override", "binding_conflict", "invalid_selection"} {
		t.Run(scenario, func(t *testing.T) {
			d, e := preparationProviderSelectionFixture()
			e.Facts.Observation.Configuration.Version = llmevents.PreparationProviderConfigurationSnapshotVersion
			e.Facts.Observation.Catalog = llmmodel.ProviderModelCatalog{ProviderID: "ai-proxy", Source: "ai_proxy", State: llmmodel.ProviderCatalogStateUnavailable, ErrorCode: "cache_miss"}
			want := ""
			switch scenario {
			case "expired":
				_, fresh := preparationProviderSelectionFixture()
				e.Facts.Observation.Catalog = fresh.Facts.Observation.Catalog
				e.Facts.Observation.ObservedAt = e.Facts.Observation.Catalog.ExpiresAt
				e.Facts.Observation.Catalog.State, e.Facts.Observation.Catalog.Stale, e.Facts.Observation.Catalog.ErrorCode = llmmodel.ProviderCatalogStateStale, true, "cache_expired"
			case "workspace":
				d.WorkspaceDefault = &runtimecontract.ProviderInferenceSelection{Target: e.Facts.Observation.Configuration.DefaultChat, ReasoningEffort: "high"}
				e.Facts.Observation.Configuration.DefaultChat.ModelID = "unselected-owner-default"
			case "override", "unsupported_override":
				e.Facts.Observation.Configuration.Overrides = []llmevents.PreparationProviderModelOverride{{ModelID: "exact-model", ContextWindowTokens: 64000, MaxOutputTokens: 8000, Operations: []string{"responses"}}}
				if scenario == "unsupported_override" {
					e.Facts.Observation.Configuration.Overrides[0].Operations = []string{"embeddings"}
					want = "operation_unsupported"
				}
			case "binding_conflict":
				d.ConfigurationHash = "different"
				want = "source_binding_conflict"
			case "invalid_selection":
				d.WorkspaceDefault = &runtimecontract.ProviderInferenceSelection{}
				want = "selection_invalid"
			}
			beforeD, beforeE := d.Clone(), e.Clone()
			c := CompilePreparationProviderSelection(d, e)
			if c.FailureCode != want || !reflect.DeepEqual(d, beforeD) || !reflect.DeepEqual(e, beforeE) {
				t.Fatal("configuration selection mismatch or source rewritten", c)
			}
			if want == "" && (c.Version != PreparationProviderConfigurationSelectionVersion || c.BlockedReason() != PreparationProviderConfigurationReason || c.Selection.Target.ModelID != "exact-model") {
				t.Fatal("configuration check required a catalog/permission or selected another model", c)
			}
			e.Selection = &c
			e.ContentHash = e.Hash()
			if e.Validate() != nil {
				t.Fatal("configuration result cannot be independently reproduced")
			}
			bad := e.Clone()
			bad.Selection.Selection.Target.ModelID = "neighbor"
			bad.ContentHash = bad.Hash()
			if bad.Validate() == nil {
				t.Fatal("self-rehashed selection substitution accepted")
			}
		})
	}
}
