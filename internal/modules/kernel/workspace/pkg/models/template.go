package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// WorkspaceTemplate is the workspace owner's durable template DTO. Repository
// representations are intentionally kept inside the workspace owner.
type WorkspaceTemplate struct {
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

// WorkspaceForCurrentTemplate is a detached authoring view. A changed Template
// replaces its previously materialized binding, not Goal/DoD, directories,
// provider selection, lifecycle or admission history. Persistence belongs to
// the private definition-admission transaction, never to this pure helper.
func WorkspaceForCurrentTemplate(workspace *Workspace, template *WorkspaceTemplate) (*Workspace, error) {
	if workspace == nil || template == nil || workspace.TemplateID != template.ID {
		return nil, common.ErrInvalidArgument
	}
	next := *workspace
	next.PreparationFence = workspace.PreparationFence.Clone()
	boundHash := workspace.TemplateBindingHash
	if boundHash == "" && workspace.StartupInput != nil {
		boundHash = workspace.StartupInput.TemplateSnapshotHash
	}
	currentHash := WorkspaceTemplateSnapshotHash(template)
	if boundHash == currentHash {
		return &next, nil
	}
	next.TemplateBindingHash = currentHash
	next.ProfileID = template.ID
	next.DefaultSoulID = common.SoulID(template.DefaultSoulID)
	next.SoulBindings = nil
	if len(template.SoulBindings) > 0 {
		next.SoulBindings = make(map[string]common.SoulID, len(template.SoulBindings))
	}
	for stage, id := range template.SoulBindings {
		next.SoulBindings[stage] = common.SoulID(id)
	}
	next.ContextHints = template.ContextHints.Clone()
	next.EvidencePolicies = workorcheventpayload.CloneSlice(template.EvidencePolicies)
	return &next, nil
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
