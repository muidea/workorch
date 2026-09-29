package workspace

import (
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestResolvedTemplateParentReferencesAreDetached(t *testing.T) {
	parent := &WorkspaceTemplate{ID: "parent"}
	child := &WorkspaceTemplate{ID: "child", ParentTemplateIDs: []string{"parent"}}
	value, err := ResolveWorkspaceTemplate("child", func(id string) (*WorkspaceTemplate, error) {
		if id == child.ID {
			return child, nil
		}
		return parent, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	value.ParentTemplateIDs[0] = "changed"
	if child.ParentTemplateIDs[0] != "parent" {
		t.Fatal("resolved parent references alias owner declaration")
	}
}

func TestResolveWorkspaceTemplateSharedAncestorCannotHideDepth(t *testing.T) {
	items := map[string]*WorkspaceTemplate{
		"root":   {ID: "root"},
		"shared": {ID: "shared", ParentTemplateIDs: []string{"root"}},
	}
	parent := "shared"
	for i := 0; i < maxTemplateDepth-2; i++ {
		id := fmt.Sprintf("chain-%d", i)
		items[id] = &WorkspaceTemplate{ID: id, ParentTemplateIDs: []string{parent}}
		parent = id
	}
	items["leaf"] = &WorkspaceTemplate{ID: "leaf", ParentTemplateIDs: []string{"shared", parent}}
	load := func(id string) (*WorkspaceTemplate, error) { return items[id], nil }
	for _, parents := range [][]string{{"shared", parent}, {parent, "shared"}} {
		items["leaf"].ParentTemplateIDs = parents
		if _, err := ResolveWorkspaceTemplate("leaf", load); !errors.Is(err, common.ErrConflict) {
			t.Fatalf("shared ancestor bypassed depth bound for %v: %v", parents, err)
		}
	}
}

func TestResolveWorkspaceTemplateDiamondStableTopoAndBounds(t *testing.T) {
	items := map[string]*WorkspaceTemplate{
		"root":  {ID: "root", DoDItems: []string{"root"}},
		"left":  {ID: "left", ParentTemplateIDs: []string{"root"}, DoDItems: []string{"left"}},
		"right": {ID: "right", ParentTemplateIDs: []string{"root"}, DoDItems: []string{"right"}},
		"leaf":  {ID: "leaf", ParentTemplateIDs: []string{"left", "right"}, DoDItems: []string{"leaf"}},
	}
	load := func(id string) (*WorkspaceTemplate, error) {
		if item := items[id]; item != nil {
			return item, nil
		}
		return nil, common.ErrNotFound
	}
	value, err := ResolveWorkspaceTemplate("leaf", load)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(value.DoDItems, []string{"root", "left", "right", "leaf"}) {
		t.Fatalf("diamond duplicated ancestor or changed order: %v", value.DoDItems)
	}
	ids := make([]string, 0, len(value.EffectiveSources))
	for _, source := range value.EffectiveSources {
		ids = append(ids, source.TemplateID)
	}
	if !slices.Equal(ids, []string{"root", "left", "right", "leaf"}) {
		t.Fatalf("sources not stable topological order: %v", ids)
	}
	baseHash := WorkspaceTemplateSnapshotHash(value)
	items["leaf"].ParentTemplateIDs = []string{"right", "left"}
	other, err := ResolveWorkspaceTemplate("leaf", load)
	if err != nil {
		t.Fatal(err)
	}
	if WorkspaceTemplateSnapshotHash(other) == baseHash {
		t.Fatal("parent order did not enter hash")
	}
	items["leaf"].ParentTemplateIDs = []string{"left", "left"}
	if _, err := ResolveWorkspaceTemplate("leaf", load); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("duplicate parent accepted: %v", err)
	}
	items["leaf"].ParentTemplateIDs = []string{"left", "right"}
	for idx := 0; idx < maxTemplateDepth; idx++ {
		id := fmt.Sprintf("depth-%d", idx)
		parent := "leaf"
		if idx > 0 {
			parent = fmt.Sprintf("depth-%d", idx-1)
		}
		items[id] = &WorkspaceTemplate{ID: id, ParentTemplateIDs: []string{parent}}
	}
	if _, err := ResolveWorkspaceTemplate(fmt.Sprintf("depth-%d", maxTemplateDepth-1), load); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("depth exceeded: %v", err)
	}
	// Broad DAG exceeds the distinct-node budget without exceeding chain depth.
	wide := &WorkspaceTemplate{ID: "wide"}
	items[wide.ID] = wide
	for idx := 0; idx < maxTemplateNodes; idx++ {
		id := fmt.Sprintf("wide-%d", idx)
		items[id] = &WorkspaceTemplate{ID: id}
		wide.ParentTemplateIDs = append(wide.ParentTemplateIDs, id)
	}
	if _, err := ResolveWorkspaceTemplate("wide", load); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("node budget exceeded: %v", err)
	}
}
