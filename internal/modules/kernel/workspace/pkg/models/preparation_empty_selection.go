package workspace

import (
	"reflect"
	"slices"

	"github.com/muidea/workorch/internal/pkg/common"
)

const PreparationEmptySelectionEvaluationVersion = "workorch-preparation-empty-selection-evaluation/v1"
const PreparationEmptySkillReason = "skill_empty_selection_observed_remaining_checks"
const PreparationEmptyMCPReason = "mcp_empty_selection_observed_remaining_checks"

// This bounded consumer supports exact empty selections only, not unavailable
// catalogs, arbitrary N/A, selected resource freshness or MCP connections. All
// original selected-source compilers and raw owner archives remain mandatory.
func EvaluatePreparationEmptySelections(w *Workspace, a WorkspacePreparationAttempt) []WorkspacePreparationCheckResult {
	checks := slices.Clone(a.Checks)
	if w == nil || a.State != WorkspacePreparationBlocked || a.EvaluationVersion != PreparationCheckEvaluationVersion || a.EmptySelectionEvaluationVersion != PreparationEmptySelectionEvaluationVersion {
		return checks
	}
	var skills *WorkspacePreparationSkillRegistryEvidence
	var souls *WorkspacePreparationSoulEvidence
	var effects *WorkspacePreparationSkillHubEffects
	var mcp *WorkspacePreparationMCPRegistryEvidence
	for i := range w.PreparationSkillRegistryEvidence {
		if w.PreparationSkillRegistryEvidence[i].Ref == a.SkillRegistryEvidenceRef {
			skills = &w.PreparationSkillRegistryEvidence[i]
		}
	}
	for i := range w.PreparationSoulEvidence {
		if w.PreparationSoulEvidence[i].Ref == a.SoulEvidenceRef {
			souls = &w.PreparationSoulEvidence[i]
		}
	}
	for i := range w.PreparationSkillHubEffects {
		if w.PreparationSkillHubEffects[i].Ref == a.SkillHubEffectRef {
			effects = &w.PreparationSkillHubEffects[i]
		}
	}
	for i := range w.PreparationMCPRegistryEvidence {
		if w.PreparationMCPRegistryEvidence[i].Ref == a.MCPRegistryEvidenceRef {
			mcp = &w.PreparationMCPRegistryEvidence[i]
		}
	}
	emptySkill, emptyMCP := false, false
	definitionExact := false
	if skills != nil && skills.Selection != nil {
		for _, seal := range w.PreparationSealEvidence {
			if seal.Ref == a.SealEvidenceRef && seal.validateFor(w, a) == nil {
				d := skills.Selection.Definition
				definitionExact = d.GoalText == seal.Before.Goal.Text && d.GoalTextHash == seal.Before.StartupInput.GoalTextHash && d.DoDItemSetHash == seal.Before.StartupInput.DoDItemSetHash && reflect.DeepEqual(d.DoDItems, seal.Before.DoD.Items)
			}
		}
	}
	if definitionExact && skills != nil && skills.Validate() == nil && skills.Selection != nil && souls != nil && souls.Validate() == nil && completePreparationSoulSelection(souls.Facts) && effects != nil && effects.Validate() == nil && skills.RequestID == a.RequestID && skills.PayloadHash == a.PayloadHash && skills.Facts.SnapshotHash == a.ObservationSnapshotHash && skills.Facts.WorkspaceID == w.ID && skills.Facts.WorkspaceRevision == a.ExpectedWorkspaceRevision+1 && effects.Binding == a.Binding {
		expected := CompilePreparationSkillSelection(skills.Selection.Definition, *skills, souls, effects)
		if expected.FailureCode == "" {
			context := CompilePreparationSkillContext(expected.Selected)
			expected.Context = &context
			references := CompilePreparationSkillReferences(expected, effects)
			expected.References = &references
			status := CompilePreparationSkillStatus(expected, effects)
			expected.Status = &status
			emptySkill = reflect.DeepEqual(expected, *skills.Selection) && len(expected.Selected) == 0 && len(expected.Selection.Matches) == 0 && context.FailureCode == "" && len(context.Projection) == 0 && references.State == "observed_checks_pending" && len(references.Entries) == 0 && status.State == "observed_freshness_pending" && len(status.Entries) == 0
		}
	}
	if emptySkill && mcp != nil && mcp.Validate() == nil && mcp.Selection != nil && mcp.SelectionEvaluationVersion == PreparationMCPSelectionEvaluationVersion && mcp.RequestID == a.RequestID && mcp.PayloadHash == a.PayloadHash && mcp.Binding == a.Binding && mcp.Facts.WorkspaceID == w.ID && mcp.Facts.WorkspaceRevision == a.ExpectedWorkspaceRevision+1 && mcp.Facts.SnapshotHash == a.ObservationSnapshotHash {
		expected := CompilePreparationMCPSelection(mcp.Selection.Definition, *mcp, skills)
		emptyMCP = expected.FailureCode == "" && reflect.DeepEqual(expected, *mcp.Selection) && len(expected.SelectedServerIDs) == 0 && len(expected.SelectedCapabilityIDs) == 0 && len(expected.Selected.Servers) == 0 && len(expected.Selected.Capabilities) == 0
	}
	for i, c := range checks {
		if c.Reason == "binding_changed" || c.Reason == "original_check_interrupted" {
			continue
		}
		switch c.ID {
		case "skill_selection":
			if emptySkill {
				checks[i].State, checks[i].Reason = "passed", PreparationEmptySkillReason
			} else if c.State == "passed" {
				checks[i].State, checks[i].Reason = "blocked", "check_pipeline_incomplete"
			}
		case "mcp_selection":
			if emptyMCP {
				checks[i].State, checks[i].Reason = "passed", PreparationEmptyMCPReason
			} else if c.State == "passed" {
				checks[i].State, checks[i].Reason = "blocked", "check_pipeline_incomplete"
			}
		}
	}
	return checks
}

func validatePreparationEmptySelectionPersistence(w *Workspace) error {
	if w == nil {
		return common.ErrConflict
	}
	if w.StartupPreparation == nil {
		return nil
	}
	for _, a := range w.StartupPreparation.Attempts {
		if a.EmptySelectionEvaluationVersion != "" && !reflect.DeepEqual(a.Checks, EvaluatePreparationEmptySelections(w, a)) {
			return common.ErrConflict
		}
	}
	return nil
}
