package workspace

import (
	"errors"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func TestResolveWorkspaceTemplateInheritanceSnapshotAndLegacyHash(t *testing.T) {
	policy, _ := workorcheventpayload.FromMap(map[string]any{"id": "rule", "required": true})
	hints, _ := workorcheventpayload.FromMap(map[string]any{"profileContracts": map[string]any{"a": map[string]any{"enabled": true}}})
	parent := &WorkspaceTemplate{ID: "parent", Name: "Parent", GoalText: "goal", DoDItems: []string{"parent required"}, SoulBindings: map[string]string{"planning": "soul"}, ContextHints: hints, EvidencePolicies: []workorcheventpayload.Payload{policy}}
	child := &WorkspaceTemplate{ID: "child", ParentTemplateIDs: []string{parent.ID}, Name: "Child", DoDItems: []string{"child required"}}
	items := map[string]*WorkspaceTemplate{parent.ID: parent, child.ID: child}
	load := func(id string) (*WorkspaceTemplate, error) {
		if item := items[id]; item != nil {
			return item, nil
		}
		return nil, common.ErrNotFound
	}
	resolved, err := ResolveWorkspaceTemplate(child.ID, load)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.ID != child.ID || len(resolved.ParentTemplateIDs) != 1 || resolved.ParentTemplateIDs[0] != parent.ID || resolved.GoalText != "goal" || len(resolved.DoDItems) != 2 || resolved.DoDItems[0] != "parent required" || resolved.SoulBindings["planning"] != "soul" || len(resolved.EvidencePolicies) != 1 || len(resolved.EffectiveSources) != 2 {
		t.Fatalf("inherited content missing: %+v", resolved)
	}
	before := WorkspaceTemplateSnapshotHash(resolved)
	parent.Description = "changed source"
	changed, err := ResolveWorkspaceTemplate(child.ID, load)
	if err != nil {
		t.Fatal(err)
	}
	if WorkspaceTemplateSnapshotHash(changed) == before {
		t.Fatal("parent edit did not fence new activation")
	}
	if WorkspaceTemplateSnapshotHash(parent) != WorkspaceTemplateSnapshotHash(mustResolve(t, parent.ID, load)) {
		t.Fatal("parentless snapshot hash drifted")
	}
	if len(child.DoDItems) != 1 || len(parent.DoDItems) != 1 {
		t.Fatal("resolution mutated raw declarations")
	}
}

func mustResolve(t *testing.T, id string, load func(string) (*WorkspaceTemplate, error)) *WorkspaceTemplate {
	t.Helper()
	value, err := ResolveWorkspaceTemplate(id, load)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func TestResolveWorkspaceTemplateFailsClosedOnConflictsAndInvalidChain(t *testing.T) {
	policyRequired, _ := workorcheventpayload.FromMap(map[string]any{"id": "rule", "required": true})
	policyOptional, _ := workorcheventpayload.FromMap(map[string]any{"id": "rule", "required": false})
	parent := &WorkspaceTemplate{ID: "parent", EvidencePolicies: []workorcheventpayload.Payload{policyRequired}}
	child := &WorkspaceTemplate{ID: "child", ParentTemplateIDs: []string{"parent"}, EvidencePolicies: []workorcheventpayload.Payload{policyOptional}}
	items := map[string]*WorkspaceTemplate{"parent": parent, "child": child}
	load := func(id string) (*WorkspaceTemplate, error) {
		if item := items[id]; item != nil {
			return item, nil
		}
		return nil, common.ErrNotFound
	}
	if _, err := ResolveWorkspaceTemplate("child", load); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("required downgrade: %v", err)
	}
	child.EvidencePolicies = nil
	child.ContextHints = workorcheventpayload.New(workorcheventpayload.StringField("configuration", "child"))
	parent.ContextHints = workorcheventpayload.New(workorcheventpayload.StringField("configuration", "parent"))
	if _, err := ResolveWorkspaceTemplate("child", load); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("ambiguous context: %v", err)
	}
	child.ContextHints = workorcheventpayload.Payload{}
	child.ParentTemplateIDs = []string{"missing"}
	if _, err := ResolveWorkspaceTemplate("child", load); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("missing ancestor: %v", err)
	}
	child.ParentTemplateIDs = []string{"parent"}
	parent.ParentTemplateIDs = []string{"child"}
	if _, err := ResolveWorkspaceTemplate("child", load); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("cycle: %v", err)
	}
}
