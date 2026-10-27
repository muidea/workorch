package events

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"strings"
	"unicode/utf8"

	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

const TopicWorkspacePreparationCheck = "/workspace/activation-preparation/check"
const TopicWorkspacePreparationRecover = "/workspace/activation-preparation/recover"
const TopicWorkspacePreparationSettleBlocked = "/workspace/activation-preparation/settle-blocked"

// Internal application continuation only. These are negative classifications,
// not caller-authored PASS or a readiness credential. ProviderCacheFacts is the
// complete single-point cache observation, MemoryInventoryFacts the complete
// local inventory point observation; SoulFacts/SkillRegistryFacts carry
// complete before/after observations. RunHistoryFacts carries the complete
// ordered Run-owner history result. All are detached, independently rebound
// by Workspace and archived as history, never accepted as a verdict.
// ProcessPolicyFacts carries only double-read local non-sensitive process
// settings. Workspace captures its own confirmed template/configuration input;
// this command cannot submit those policies or a compiled-success credential.
type SettleWorkspacePreparationBlockedCommand struct {
	ProcessPolicyFacts              *workspace.PreparationProcessPolicyFacts
	MCPRegistryFacts                *workspace.PreparationMCPObservationFacts
	MCPRegistryReason               string
	Original                        CheckWorkspacePreparationCommand
	ProviderCacheReason             string
	ProviderCacheFacts              *workspace.PreparationProviderCacheFacts
	MemoryInventoryFacts            *workspace.PreparationMemoryInventoryFacts
	MemoryInventoryReason           string
	RunHistoryReason                string
	RunHistoryFacts                 *workspace.PreparationRunHistoryFacts
	SoulReason                      string
	SoulFacts                       *workspace.PreparationSoulFacts
	SkillRegistryReason             string
	SkillRegistryFacts              *workspace.PreparationSkillCatalogFacts
	SkillHubEffectRef               string
	SkillHubGlobal, SkillHubProject *workspace.PreparationSkillHubStatusFacts
}

func (c SettleWorkspacePreparationBlockedCommand) Validate() error {
	if f := c.ProcessPolicyFacts; f != nil && (f.Validate() != nil || f.WorkspaceID != c.Original.WorkspaceID || c.Original.ExpectedWorkspaceRevision == ^uint64(0) || f.WorkspaceRevision != c.Original.ExpectedWorkspaceRevision+1) {
		return common.ErrInvalidArgument
	}
	if f := c.RunHistoryFacts; f != nil && (f.Validate() != nil || f.Observation.History.WorkspaceID != c.Original.WorkspaceID || f.Observation.RequestID != c.Original.RequestID || c.Original.ExpectedWorkspaceRevision == ^uint64(0) || f.WorkspaceRevision != c.Original.ExpectedWorkspaceRevision+1 || c.RunHistoryReason != f.BlockedReason()) {
		return common.ErrInvalidArgument
	}
	if c.MCPRegistryReason != "" && !workspace.ValidPreparationBlockedReason("mcp_selection", c.MCPRegistryReason) {
		return common.ErrInvalidArgument
	}
	if f := c.MCPRegistryFacts; f != nil {
		if f.Validate() != nil || f.WorkspaceID != c.Original.WorkspaceID || c.Original.ExpectedWorkspaceRevision == ^uint64(0) || f.WorkspaceRevision != c.Original.ExpectedWorkspaceRevision+1 || c.MCPRegistryReason != "mcp_registry_observed_connection_pending" {
			return common.ErrInvalidArgument
		}
	} else if c.MCPRegistryReason == "mcp_registry_observed_connection_pending" {
		return common.ErrInvalidArgument
	}
	if c.MemoryInventoryReason != "" && !workspace.ValidPreparationBlockedReason("memory_selection", c.MemoryInventoryReason) {
		return common.ErrInvalidArgument
	}
	if f := c.MemoryInventoryFacts; f != nil {
		if f.Validate() != nil || f.Observation.WorkspaceID != c.Original.WorkspaceID || c.Original.ExpectedWorkspaceRevision == ^uint64(0) || f.WorkspaceRevision != c.Original.ExpectedWorkspaceRevision+1 || c.MemoryInventoryReason != f.BlockedReason() {
			return common.ErrInvalidArgument
		}
	} else if c.MemoryInventoryReason == "memory_inventory_observed_selection_pending" {
		return common.ErrInvalidArgument
	}
	if f := c.ProviderCacheFacts; f != nil && (f.Validate() != nil || f.Observation.WorkspaceID != c.Original.WorkspaceID || c.Original.ExpectedWorkspaceRevision == ^uint64(0) || f.WorkspaceRevision != c.Original.ExpectedWorkspaceRevision+1 || c.ProviderCacheReason != f.BlockedReason()) {
		return common.ErrInvalidArgument
	}
	if c.SkillHubEffectRef != "" && !workspace.ValidPreparationSkillHubRef(c.SkillHubEffectRef) || c.SkillHubEffectRef == "" && (c.SkillHubGlobal != nil || c.SkillHubProject != nil) {
		return common.ErrInvalidArgument
	}
	if c.SkillHubGlobal != nil && c.SkillHubGlobal.ValidateFor("global", "") != nil {
		return common.ErrInvalidArgument
	}
	if c.SkillHubProject != nil && c.SkillHubProject.ValidateFor("workspace", c.SkillHubProject.Directory) != nil {
		return common.ErrInvalidArgument
	}
	if c.SkillRegistryReason != "" && !workspace.ValidPreparationBlockedReason("skill_selection", c.SkillRegistryReason) {
		return common.ErrInvalidArgument
	}
	if c.SkillRegistryFacts != nil && (c.SkillRegistryReason != "skill_registry_observed_selection_pending" || c.SkillRegistryFacts.Validate() != nil || c.SkillRegistryFacts.WorkspaceID != c.Original.WorkspaceID || c.Original.ExpectedWorkspaceRevision == ^uint64(0) || c.SkillRegistryFacts.WorkspaceRevision != c.Original.ExpectedWorkspaceRevision+1) || c.SkillRegistryFacts == nil && c.SkillRegistryReason == "skill_registry_observed_selection_pending" {
		return common.ErrInvalidArgument
	}
	if c.Original.Validate() != nil || !workspace.ValidPreparationBlockedReason("provider_cache", c.ProviderCacheReason) || !workspace.ValidPreparationBlockedReason("first_activation_no_run", c.RunHistoryReason) {
		return common.ErrInvalidArgument
	}
	if c.SoulReason != "" && !workspace.ValidPreparationBlockedReason("soul_selection", c.SoulReason) {
		return common.ErrInvalidArgument
	}
	if c.SoulFacts != nil && (c.SoulReason != "souls_observed_remaining_checks" || c.SoulFacts.Validate() != nil || c.SoulFacts.WorkspaceID != c.Original.WorkspaceID || c.Original.ExpectedWorkspaceRevision == ^uint64(0) || c.SoulFacts.WorkspaceRevision != c.Original.ExpectedWorkspaceRevision+1) || c.SoulFacts == nil && c.SoulReason == "souls_observed_remaining_checks" {
		return common.ErrInvalidArgument
	}
	return nil
}

// The original intent is also the recovery key. Callers cannot submit a
// manifest, per-check verdict, current resource claim or activation permission.
type CheckWorkspacePreparationCommand struct {
	WorkspaceID               common.WorkspaceID
	RequestID                 string
	ExpectedWorkspaceRevision uint64
}

func (c CheckWorkspacePreparationCommand) Validate() error {
	if c.WorkspaceID == "" || c.RequestID == "" || len(c.RequestID) > 256 || strings.TrimSpace(c.RequestID) != c.RequestID || !utf8.ValidString(c.RequestID) || strings.ContainsAny(c.RequestID, "\x00\r\n") || c.ExpectedWorkspaceRevision == 0 {
		return common.ErrInvalidArgument
	}
	return nil
}

func (c CheckWorkspacePreparationCommand) PayloadHash() string {
	if c.Validate() != nil {
		return ""
	}
	encoded, err := json.Marshal(struct {
		Version string
		Intent  CheckWorkspacePreparationCommand
	}{"workorch-preparation-intent/v1", c})
	if err != nil {
		return ""
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

type CheckWorkspacePreparationResult struct {
	RequestID string
	State     QueryWorkspacePreparationStateResult
	// Detached original owner inputs for Application's pure compiler only.
	// Never included in HTTP/projection DTOs or accepted back as authority.
	CompilationSource *PreparationCompilationSource `json:"-"`
	// Only the first unambiguous acceptance returns this continuation hint.
	// It is not persisted, accepted back as authority, or exposed over HTTP.
	ObservationRequired bool
	// Private accepted plan, returned only to the first unambiguous continuation.
	SkillHubPlan *workspace.WorkspacePreparationSkillHubEffects
	// Original declaration for the first observation only, not an authority
	// credential or an input accepted back by Workspace settlement.
	ProviderDefinition *workspace.PreparationProviderSelectionDefinition
}

func (r CheckWorkspacePreparationResult) ValidateFor(c CheckWorkspacePreparationCommand) error {
	if c.Validate() != nil || r.RequestID != c.RequestID || r.State.ValidateFor(QueryWorkspacePreparationStateCommand{WorkspaceID: c.WorkspaceID}) != nil || r.State.Record == nil {
		return common.ErrConflict
	}
	if r.CompilationSource != nil {
		w, err := r.CompilationSource.Model()
		if err != nil || w.ID != c.WorkspaceID || w.Revision != r.State.WorkspaceRevision || !reflect.DeepEqual(w.StartupPreparation, r.State.Record) || workspace.ValidatePreparationEvidencePersistence(w) != nil {
			return common.ErrConflict
		}
	}
	for _, a := range r.State.Record.Attempts {
		if a.RequestID == c.RequestID && a.PayloadHash == c.PayloadHash() && a.ExpectedWorkspaceRevision == c.ExpectedWorkspaceRevision {
			if r.ProviderDefinition != nil && (!r.ObservationRequired || r.ProviderDefinition.ConfigurationHash != a.Binding.AuthoringConfigurationHash) {
				return common.ErrConflict
			}
			if r.ObservationRequired && (a.State != workspace.WorkspacePreparationAccepted || r.State.Record.Attempts[len(r.State.Record.Attempts)-1].RequestID != c.RequestID) {
				return common.ErrConflict
			}
			if r.SkillHubPlan != nil {
				p := r.SkillHubPlan
				if !r.ObservationRequired || p.Validate() != nil || !p.CompletedAt.IsZero() || p.WorkspaceID != c.WorkspaceID || p.RequestID != c.RequestID || p.PayloadHash != c.PayloadHash() || p.ExpectedWorkspaceRevision != c.ExpectedWorkspaceRevision || p.Ref != a.SkillHubEffectRef || p.Binding != a.Binding {
					return common.ErrConflict
				}
			} else if r.ObservationRequired && a.SkillHubEffectRef != "" {
				return common.ErrConflict
			}
			return nil
		}
	}
	return common.ErrConflict
}
