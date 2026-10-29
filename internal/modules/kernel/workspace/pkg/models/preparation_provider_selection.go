package workspace

import (
	"reflect"
	"slices"

	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
	llmmodel "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const PreparationProviderSelectionVersion = "workorch-preparation-provider-selection/v1"
const PreparationProviderConfigurationSelectionVersion = "workorch-preparation-provider-selection/v2"
const PreparationProviderConfigurationReason = "provider_configuration_observed_remaining_checks"

func IsPreparationProviderSelectionReason(reason string) bool {
	switch reason {
	case "provider_selection_budget_observed_authorization_pending", "provider_selection_budget_unavailable", "provider_selection_budget_source_conflict", "provider_selection_authorization_pending", "provider_selection_source_binding_conflict", "provider_selection_configuration_source_unavailable", "provider_selection_invalid", "provider_selection_model_missing", "provider_selection_capability_incomplete", "provider_selection_operation_unsupported", "provider_selection_reasoning_unsupported":
		return true
	}
	return false
}

func (c PreparationProviderSelectionCompilation) BlockedReason() string {
	if c.Version != PreparationProviderSelectionVersion && c.Version != PreparationProviderConfigurationSelectionVersion {
		return ""
	}
	switch c.FailureCode {
	case "":
		if c.Version == PreparationProviderConfigurationSelectionVersion {
			return PreparationProviderConfigurationReason
		}
		if c.Budget != nil {
			return "provider_selection_budget_observed_authorization_pending"
		}
		return "provider_selection_authorization_pending"
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
	Descriptor      *llmmodel.ProviderModelDescriptor
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
	if c.Descriptor != nil {
		d := *c.Descriptor
		d.SupportedEndpoints = slices.Clone(d.SupportedEndpoints)
		d.Operations = slices.Clone(d.Operations)
		d.Reasoning.Efforts = slices.Clone(d.Reasoning.Efforts)
		c.Descriptor = &d
	}
	return c
}

func CompilePreparationProviderSelection(d PreparationProviderSelectionDefinition, e WorkspacePreparationProviderCacheEvidence) PreparationProviderSelectionCompilation {
	o := e.Facts.Observation
	c := PreparationProviderSelectionCompilation{Version: PreparationProviderSelectionVersion, Definition: d.Clone(), OwnerEpoch: o.OwnerEpoch, CatalogRevision: o.Catalog.Revision}
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
	if o.Configuration.Version == llmevents.PreparationProviderConfigurationSnapshotVersion {
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
	switch o.Catalog.State {
	case llmmodel.ProviderCatalogStateUnavailable:
		return fail("cache_miss")
	case llmmodel.ProviderCatalogStateStale:
		return fail("cache_expired")
	}
	for i := range o.Catalog.Models {
		if o.Catalog.Models[i].Target == c.Selection.Target {
			v := o.Catalog.Models[i]
			c.Descriptor = &v
			break
		}
	}
	var overrideOperations []string
	for _, override := range o.Configuration.Overrides {
		if override.ModelID == c.Selection.Target.ModelID {
			c.OverrideModelID, overrideOperations = override.ModelID, override.Operations
			break
		}
	}
	// Match activation's exact override semantics without deriving a second
	// ceiling: an override may declare capacity but cannot contradict a known
	// catalog operation; missing reasoning metadata is not invented by it.
	if c.OverrideModelID != "" {
		if !slices.Contains(overrideOperations, "responses") || c.Descriptor != nil && len(c.Descriptor.Operations) > 0 && !slices.Contains(c.Descriptor.Operations, "responses") {
			return fail("operation_unsupported")
		}
	} else {
		if c.Descriptor == nil {
			return fail("model_missing")
		}
		if !c.Descriptor.CapabilityReady || c.Descriptor.ContextWindowTokens <= 0 || c.Descriptor.MaxOutputTokens <= 0 {
			return fail("capability_incomplete")
		}
		if !slices.Contains(c.Descriptor.Operations, "responses") {
			return fail("operation_unsupported")
		}
	}
	if c.Selection.ReasoningEffort != "" && (c.Descriptor == nil || !c.Descriptor.Reasoning.Allows(c.Selection.ReasoningEffort)) {
		return fail("reasoning_unsupported")
	}
	if o.Budget != nil {
		if o.Budget.Target != c.Selection.Target {
			return fail("budget_source_conflict")
		}
		if o.Budget.FailureCode != "" {
			return fail("budget_unavailable")
		}
		b := *o.Budget
		c.Budget = &b
	}
	return c.Clone()
}

func validatePreparationProviderSelection(e WorkspacePreparationProviderCacheEvidence) error {
	if e.Selection == nil {
		return nil // historical cache-only observation; never upgraded on reads
	}
	if (e.Selection.Version != PreparationProviderSelectionVersion && e.Selection.Version != PreparationProviderConfigurationSelectionVersion) || !reflect.DeepEqual(*e.Selection, CompilePreparationProviderSelection(e.Selection.Definition, e)) {
		return common.ErrConflict
	}
	return nil
}
