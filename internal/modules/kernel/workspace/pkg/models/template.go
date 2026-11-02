package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/runtimeprofiledecode"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// WorkspaceTemplate is the workspace owner's durable template DTO. Repository
// representations are intentionally kept inside the workspace owner.
type WorkspaceTemplate struct {
	// Builtin is query provenance only. It is never persisted or hashed.
	Builtin           bool     `json:"-"`
	ID                string   `json:"id"`
	ParentTemplateIDs []string `json:"parentTemplateIds,omitempty"`
	// EffectiveSources is populated only on a resolved read, root first. Raw
	// repository records leave it empty; it binds inherited content to its source.
	EffectiveSources []TemplateSource               `json:"effectiveSources,omitempty"`
	Name             string                         `json:"name"`
	Description      string                         `json:"description,omitempty"`
	GoalText         string                         `json:"goalText,omitempty"`
	DoDItems         []string                       `json:"doDItems,omitempty"`
	DefaultSoulID    string                         `json:"defaultSoulId,omitempty"`
	SoulBindings     map[string]string              `json:"soulBindings,omitempty"`
	ContextHints     workorcheventpayload.Payload   `json:"contextHints,omitempty"`
	EvidencePolicies []workorcheventpayload.Payload `json:"evidencePolicies,omitempty"`
}

// WorkspaceTemplateSnapshotHash binds the complete declaration, including
// stage routing. Historical snapshots are never rehashed by this function.
func WorkspaceTemplateSnapshotHash(template *WorkspaceTemplate) string {
	if template == nil {
		return ""
	}
	h := sha256.New()
	values := []string{template.ID, template.Name, template.Description, template.GoalText, strings.Join(template.DoDItems, "\x00"), template.DefaultSoulID, template.ContextHints.CanonicalString()}
	for _, policy := range template.EvidencePolicies {
		values = append(values, policy.CanonicalString())
	}
	stages := make([]string, 0, len(template.SoulBindings))
	for stage := range template.SoulBindings {
		stages = append(stages, stage)
	}
	sort.Strings(stages)
	for _, stage := range stages {
		values = append(values, stage, template.SoulBindings[stage])
	}
	// Preserve the byte-for-byte historical hash for templates without parents.
	// A derived snapshot binds both effective content and every raw ancestor.
	for _, parentID := range template.ParentTemplateIDs {
		values = append(values, "parent-template-id", parentID)
	}
	if len(template.EffectiveSources) > 1 {
		values = append(values, "effective-template-sources", inheritedTemplateSourceHash(template.EffectiveSources))
	}
	for _, value := range values {
		_, _ = fmt.Fprintf(h, "%08x%s", len(value), value)
	}
	return "template-snapshot-sha256:" + hex.EncodeToString(h.Sum(nil))
}

// WorkspaceForCurrentTemplate is a detached current-declaration view. A changed Template
// replaces its previously materialized binding, not Goal/DoD, directories,
// provider selection, lifecycle or admission history. Persistence belongs to
// the private definition-admission transaction, never to this pure helper.
func WorkspaceForCurrentTemplate(workspace *Workspace, template *WorkspaceTemplate) (*Workspace, error) {
	if workspace == nil || template == nil || workspace.TemplateID != template.ID {
		return nil, common.ErrInvalidArgument
	}
	next := *workspace
	next.PreparationFence = workspace.PreparationFence.Clone()
	next.FirstExecutionPreparationEvidence = workspace.FirstExecutionPreparationEvidence.Clone()
	currentHash := WorkspaceTemplateSnapshotHash(template)
	next.TemplateBindingHash = currentHash
	next.ProfileID = template.ID
	next.DefaultSoulID = common.SoulID(template.DefaultSoulID)
	next.SoulBindings = make(map[string]common.SoulID, len(template.SoulBindings))
	for stage, id := range template.SoulBindings {
		next.SoulBindings[stage] = common.SoulID(id)
	}
	next.ContextHints = template.ContextHints.Clone()
	next.EvidencePolicies = workorcheventpayload.CloneSlice(template.EvidencePolicies)
	// The original declaration is provenance, not a historical implementation.
	// Only fields actually inherited from it are replaced; project overrides stay.
	if record := workspace.DefinitionAuthoring; record != nil {
		original := &record.TemplateSnapshot
		if workspace.DefaultSoulID != common.SoulID(original.DefaultSoulID) {
			next.DefaultSoulID = workspace.DefaultSoulID
		}
		for stage, id := range workspace.SoulBindings {
			if string(id) != original.SoulBindings[stage] {
				next.SoulBindings[stage] = id
			}
		}
		overrides := templateOverrides(workspace.ContextHints.Map(), original.ContextHints.Map())
		// Resource budgets belong to the confirmed business boundary, even when
		// their initial values were supplied by a system default.
		effective := mergeCurrentTemplate(template.ContextHints.Map(), overrides)
		if workspace.StartupInput != nil || workspace.DefinitionAuthoringConfirmation != nil {
			oldEffective := runtimeprofiledecode.MergeTemplateWorkspaceContextHintsForExecutionContract(original.ContextHints.Map(), workspace.ContextHints.Map())
			if limits, ok := oldEffective["executionLimits"]; ok {
				// The budget is one complete authorization, not recursively
				// merged defaults that can acquire new limits after deployment.
				effective["executionLimits"] = limits
			} else {
				delete(effective, "executionLimits")
			}
		}
		var err error
		next.ContextHints, err = workorcheventpayload.FromMap(effective)
		if err != nil {
			return nil, err
		}
		if !reflect.DeepEqual(workspace.EvidencePolicies, original.EvidencePolicies) {
			next.EvidencePolicies = workorcheventpayload.CloneSlice(workspace.EvidencePolicies)
		}
	} else {
		// With no original inheritance source, explicit project values cannot be
		// classified as obsolete system defaults merely by comparing a hash.
		effective := mergeCurrentTemplate(template.ContextHints.Map(), workspace.ContextHints.Map())
		var err error
		next.ContextHints, err = workorcheventpayload.FromMap(effective)
		if err != nil {
			return nil, err
		}
		if workspace.DefaultSoulID != "" {
			next.DefaultSoulID = workspace.DefaultSoulID
		}
		for stage, id := range workspace.SoulBindings {
			next.SoulBindings[stage] = id
		}
		if len(workspace.EvidencePolicies) > 0 {
			next.EvidencePolicies = workorcheventpayload.CloneSlice(workspace.EvidencePolicies)
		}
	}

	return &next, nil
}

// templateOverrides computes detached explicit configuration, never persisted
// migration or a hash-based permission. Both sources are original full values.
func templateOverrides(actual, inherited map[string]any) map[string]any {
	result := map[string]any{}
	for k, value := range actual {
		// These keys are excluded when template defaults are materialized.
		// Their presence in Workspace is explicit, even if equal to a default.
		if k == "managedWorkspaceConfigs" || k == "defaultMCPServerIds" || k == "defaultCapabilityIds" {
			result[k] = value
			continue
		}
		a, aok := value.(map[string]any)
		b, bok := inherited[k].(map[string]any)
		if aok && bok {
			if nested := templateOverrides(a, b); len(nested) > 0 {
				result[k] = nested
			}
		} else if !reflect.DeepEqual(value, inherited[k]) {
			result[k] = value
		}
	}
	return result
}
func mergeCurrentTemplate(base, overrides map[string]any) map[string]any {
	result := map[string]any{}
	for k, v := range base {
		result[k] = v
	}
	for k, v := range overrides {
		a, aok := v.(map[string]any)
		b, bok := result[k].(map[string]any)
		if aok && bok {
			result[k] = mergeCurrentTemplate(b, a)
		} else {
			result[k] = v
		}
	}
	return result
}

// WorkspaceConfigurationHash fences only live authoring configuration, not
// lifecycle, candidate history or an old Run's immutable startup snapshot.
func WorkspaceConfigurationHash(workspace *Workspace) string {
	if workspace == nil {
		return ""
	}
	return workspaceConfigurationView(workspace).Hash()
}

// Complete non-secret preimage of the existing configuration hash. Keep field
// names/order and nil normalization identical to the historical hash shape.
// This is a declaration snapshot, not provider permission or resource state.
type WorkspaceConfigurationSnapshot struct {
	Directory, InternalDirectory, TemplateID, ProfileID string
	DefaultLLMSelection                                 *runtimecontract.ProviderInferenceSelection
	DefaultSoulID                                       common.SoulID
	SoulBindings                                        map[string]common.SoulID
	ContextHints                                        workorcheventpayload.Payload
	EvidencePolicies                                    []workorcheventpayload.Payload
}

func CaptureWorkspaceConfiguration(w *Workspace) WorkspaceConfigurationSnapshot {
	return workspaceConfigurationView(w).Clone()
}

func workspaceConfigurationView(w *Workspace) WorkspaceConfigurationSnapshot {
	if w == nil {
		return WorkspaceConfigurationSnapshot{}
	}
	return WorkspaceConfigurationSnapshot{Directory: w.DeliverableDirectory, InternalDirectory: w.InternalDirectory, TemplateID: w.TemplateID, ProfileID: w.ProfileID, DefaultLLMSelection: w.DefaultLLMSelection, DefaultSoulID: w.DefaultSoulID, SoulBindings: w.SoulBindings, ContextHints: w.ContextHints, EvidencePolicies: w.EvidencePolicies}
}

func (c WorkspaceConfigurationSnapshot) Clone() WorkspaceConfigurationSnapshot {
	if c.DefaultLLMSelection != nil {
		value := *c.DefaultLLMSelection
		c.DefaultLLMSelection = &value
	}
	if c.SoulBindings != nil {
		bindings := make(map[string]common.SoulID, len(c.SoulBindings))
		for stage, id := range c.SoulBindings {
			bindings[stage] = id
		}
		c.SoulBindings = bindings
	}
	c.ContextHints = c.ContextHints.Clone()
	if c.EvidencePolicies != nil && len(c.EvidencePolicies) == 0 {
		c.EvidencePolicies = []workorcheventpayload.Payload{}
	} else {
		c.EvidencePolicies = workorcheventpayload.CloneSlice(c.EvidencePolicies)
	}
	return c
}

func (configuration WorkspaceConfigurationSnapshot) Hash() string {
	// Empty bindings may be represented as nil across EventHub. Do not turn
	// that transport detail into configuration drift on a duplicate request.
	if len(configuration.SoulBindings) == 0 {
		configuration.SoulBindings = nil
	}
	if len(configuration.EvidencePolicies) == 0 {
		configuration.EvidencePolicies = nil
	}
	encoded, err := json.Marshal(configuration)
	if err != nil {
		return ""
	}
	hash := sha256.Sum256(encoded)
	return "workspace-configuration-sha256:" + hex.EncodeToString(hash[:])
}
