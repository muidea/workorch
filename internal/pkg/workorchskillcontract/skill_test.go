package workorchskillcontract

import (
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestStableContentHashNormalizesEquivalentSkillMaterial(t *testing.T) {
	left := Content{ID: " skill-1 ", Name: " Skill ", Version: " v1 ", Instruction: " do work ", TriggerRules: []string{" test ", "go", "go"}}
	right := Content{ID: "skill-1", Name: "Skill", Version: "v1", Instruction: "do work", TriggerRules: []string{"go", "test"}}
	if StableContentHash(left) != StableContentHash(right) {
		t.Fatal("equivalent Skill content produced different hashes")
	}
	right.Instruction = "do different work"
	if StableContentHash(left) == StableContentHash(right) {
		t.Fatal("different Skill instructions produced the same hash")
	}
}

func TestProvenanceRequiresTypedScopeAndSHA256Identity(t *testing.T) {
	valid := Provenance{Source: SourceWorkOrchRegistry, Scope: ScopeGlobal, ContentHash: StableContentHash(Content{ID: "skill-1"})}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid provenance: %v", err)
	}
	for name, value := range map[string]Provenance{
		"missing source":     {Scope: ScopeGlobal, ContentHash: valid.ContentHash},
		"unknown source":     {Source: "external", Scope: ScopeGlobal, ContentHash: valid.ContentHash},
		"invalid hash":       {Source: SourceWorkOrchRegistry, Scope: ScopeGlobal, ContentHash: "hash"},
		"unknown scope":      {Source: SourceWorkOrchRegistry, Scope: "registry", ContentHash: valid.ContentHash},
		"global workspace":   {Source: SourceWorkOrchRegistry, Scope: ScopeGlobal, WorkspaceID: "ws-1", ContentHash: valid.ContentHash},
		"workspace missing":  {Source: SourceSkillHub, Scope: ScopeWorkspace, ContentHash: valid.ContentHash},
		"registry workspace": {Source: SourceWorkOrchRegistry, Scope: ScopeWorkspace, WorkspaceID: "ws-1", ContentHash: valid.ContentHash},
	} {
		t.Run(name, func(t *testing.T) {
			if err := value.Validate(); err == nil {
				t.Fatalf("provenance %+v unexpectedly valid", value)
			}
		})
	}
	workspace := Provenance{Source: SourceSkillHub, Scope: ScopeWorkspace, WorkspaceID: common.WorkspaceID("ws-1"), ContentHash: valid.ContentHash}
	if err := workspace.Validate(); err != nil {
		t.Fatalf("workspace provenance: %v", err)
	}
}
