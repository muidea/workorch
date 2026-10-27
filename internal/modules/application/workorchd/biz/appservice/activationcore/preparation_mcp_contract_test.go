package activationcore

import (
	"reflect"
	"slices"
	"testing"

	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	payload "github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

// The preparation preview canonicalizes set order, but must preserve the
// existing activation precedence and required-reference union for valid input.
func TestPreparationMCPDefaultsPreserveActivationSetSemantics(t *testing.T) {
	for _, overlay := range []map[string]any{
		nil,
		{"shell": map[string]any{"defaultMCPServerIds": []string{"override", "a"}}},
		{"shell": map[string]any{"defaultMCPServerIds": []string{}, "defaultCapabilityIds": []string{}}},
		{"shell": "replace-object", "other": map[string]any{"defaultMCPServerIds": []string{"z", "z"}}},
	} {
		base := map[string]any{"shell": map[string]any{"defaultMCPServerIds": []string{"z", "a"}, "defaultCapabilityIds": []string{"cap"}}, "file": map[string]any{"defaultMCPServerIds": []string{"a"}}}
		basePayload, err := payload.FromMap(base)
		if err != nil {
			t.Fatal(err)
		}
		overlayPayload, err := payload.FromMap(overlay)
		if err != nil {
			t.Fatal(err)
		}
		w := &workspace.Workspace{DefinitionAuthoring: &workspace.DefinitionAuthoringRecord{TemplateSnapshot: workspace.WorkspaceTemplate{ContextHints: basePayload}}, ContextHints: overlayPayload}
		d := workspace.PreparationMCPDefinition(w)
		merged := MergeTemplateWorkspaceToolContextHints(base, overlay)
		canonical := func(ids []string) []string {
			if len(ids) == 0 {
				return nil
			}
			slices.Sort(ids)
			return slices.Compact(ids)
		}
		if d.FailureCode != "" || !reflect.DeepEqual(d.DefaultServerIDs, canonical(DefaultMCPServerIDsFromContextHints(merged))) || !reflect.DeepEqual(d.DefaultCapabilityIDs, canonical(DefaultCapabilityIDsFromContextHints(merged))) {
			t.Fatal("preparation changed valid activation defaults")
		}
		skills := []workorchmanagementcontract.Skill{{RequiredMCPServerRefs: []string{"required", "a"}, RequiredCapabilityRefs: []string{"required-cap", "cap"}}}
		selection := WorkspaceMCPSelectionContext(nil, d.DefaultServerIDs, nil, d.DefaultCapabilityIDs, skills)
		wantServers := canonical(append(slices.Clone(d.DefaultServerIDs), "required", "a"))
		wantCapabilities := canonical(append(slices.Clone(d.DefaultCapabilityIDs), "required-cap", "cap"))
		if !reflect.DeepEqual(canonical(selection.SelectedMCPServerIDs), wantServers) || !reflect.DeepEqual(canonical(selection.SelectedCapabilityIDs), wantCapabilities) {
			t.Fatal("activation required-reference union changed")
		}
	}
}
