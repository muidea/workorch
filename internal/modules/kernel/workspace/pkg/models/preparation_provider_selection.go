package workspace

import (
	"reflect"
	"slices"

	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const PreparationProviderConfigurationSelectionVersion = "workorch-preparation-provider-selection"
const PreparationProviderConfigurationReason = "provider_configuration_observed_remaining_checks"

func IsPreparationProviderSelectionReason(reason string) bool {
	switch reason {
	case "provider_selection_budget_observed_authorization_pending", "provider_selection_budget_unavailable", "provider_selection_budget_source_conflict", "provider_selection_authorization_pending", "provider_selection_source_binding_conflict", "provider_selection_configuration_source_unavailable", "provider_selection_invalid", "provider_selection_model_missing", "provider_selection_capability_incomplete", "provider_selection_operation_unsupported", "provider_selection_reasoning_unsupported":
		return true
	}
	return false
}

func (c PreparationProviderSelectionCompilation) BlockedReason() string {
	if c.Version != PreparationProviderConfigurationSelectionVersion {
		return ""
	}
	switch c.FailureCode {
	case "":
		return PreparationProviderConfigurationReason
	case "cache_miss":
		return "provider_cache_miss"
	case "cache_expired":
		return "provider_cache_expired"
	case "selection_invalid":
		return "provider_selection_invalid"
	case "budget_unavailable", "budget_source_conflict", "source_binding_conflict", "configuration_source_unavailable", "model_missing", "capability_incomplete", "operation_unsupported", "reasoning_unsupported":
		return "provider_selection_" + c.FailureCode
	}
	return ""
}

func (e WorkspacePreparationProviderCacheEvidence) BlockedReason() string {
	if e.Selection == nil {
		return e.Facts.BlockedReason()
	}
	return e.Selection.BlockedReason()
}

// Original Workspace selection only; nil means use the same frozen LLM owner
// default as activation, never the first catalog entry or an invented target.
type PreparationProviderSelectionDefinition struct {
	ConfigurationHash string
	WorkspaceDefault  *runtimecontract.ProviderInferenceSelection
}

func (d PreparationProviderSelectionDefinition) Clone() PreparationProviderSelectionDefinition {
	// Archive the original declaration; activation normalization applies only
	// to the effective Selection, not to the source material or its clone.
	if d.WorkspaceDefault != nil {
		selection := *d.WorkspaceDefault
		d.WorkspaceDefault = &selection
	}
	return d
}

func PreparationProviderDefinition(w *Workspace) PreparationProviderSelectionDefinition {
	if w == nil {
		return PreparationProviderSelectionDefinition{}
	}
	return (PreparationProviderSelectionDefinition{ConfigurationHash: WorkspaceConfigurationHash(w), WorkspaceDefault: w.DefaultLLMSelection}).Clone()
}

// Exact historical declaration compilation with an optional owner capacity
// preview, not a Runtime ProviderCeiling, request allocation, remote permission,
// connectivity, continuous freshness or readiness proof.
// Original full owner configuration declarations and catalog remain archived.
type PreparationProviderSelectionCompilation struct {
	Version         string
	Definition      PreparationProviderSelectionDefinition
	OwnerEpoch      string
	CatalogRevision string
	Source          string
	Selection       runtimecontract.ProviderInferenceSelection
	OverrideModelID string
	FailureCode     string
	Budget          *llmevents.PreparationProviderBudget `json:",omitempty"`
}

func (c PreparationProviderSelectionCompilation) Clone() PreparationProviderSelectionCompilation {
	if c.Budget != nil {
		b := *c.Budget
		c.Budget = &b
	}
	c.Definition = c.Definition.Clone()
	return c
}

func CompilePreparationProviderSelection(d PreparationProviderSelectionDefinition, e WorkspacePreparationProviderCacheEvidence) PreparationProviderSelectionCompilation {
	o := e.Facts.Observation
	c := PreparationProviderSelectionCompilation{Version: PreparationProviderConfigurationSelectionVersion, Definition: d.Clone(), OwnerEpoch: o.OwnerEpoch, CatalogRevision: o.Catalog.Revision}
	fail := func(code string) PreparationProviderSelectionCompilation { c.FailureCode = code; return c.Clone() }
	if d.ConfigurationHash == "" || d.ConfigurationHash != e.Binding.AuthoringConfigurationHash || e.Binding.Validate() != nil || e.Facts.Validate() != nil {
		return fail("source_binding_conflict")
	}
	if o.Configuration == nil {
		return fail("configuration_source_unavailable")
	}
	if d.WorkspaceDefault == nil {
		c.Source, c.Selection.Target = "llm_owner_default", o.Configuration.DefaultChat
	} else {
		c.Source, c.Selection = "workspace_default", d.WorkspaceDefault.Normalized()
	}
	if c.Selection.Validate() != nil || c.Selection.Target.ProviderID != runtimecontract.BuiltinAIProxyProviderID {
		return fail("selection_invalid")
	}

	// Activation consumes the configured target, never the first catalog
	// model. A cold/expired catalog is diagnostic data, not a second grant.
	// Runtime independently resolves its exact ceiling before any call.
	c.Version = PreparationProviderConfigurationSelectionVersion
	for _, override := range o.Configuration.Overrides {
		if override.ModelID == c.Selection.Target.ModelID {
			c.OverrideModelID = override.ModelID
			if !slices.Contains(override.Operations, "responses") {
				return fail("operation_unsupported")
			}
		}
	}
	if o.Budget != nil {
		if o.Budget.Target != c.Selection.Target {
			return fail("budget_source_conflict")
		}
		if o.Budget.FailureCode == "" {
			b := *o.Budget
			c.Budget = &b
		}
	}
	return c.Clone()

}

func validatePreparationProviderSelection(e WorkspacePreparationProviderCacheEvidence) error {
	if e.Selection == nil {
		return nil // No selection is compiled until owner configuration is observed.
	}
	if e.Selection.Version != PreparationProviderConfigurationSelectionVersion || !reflect.DeepEqual(*e.Selection, CompilePreparationProviderSelection(e.Selection.Definition, e)) {
		return common.ErrConflict
	}
	return nil
}
