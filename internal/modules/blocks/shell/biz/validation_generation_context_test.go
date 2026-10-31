package shell

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestValidationGenerationContextObservesActualSourceWithoutTruncationOrInputSubstitution(t *testing.T) {
	root := t.TempDir()
	writeNativeStructureSource(t, root, "go.mod", "module delivery\ngo 1.26\n")
	raw := "package delivery\r\nfunc Value() string { return \"实际实现\" }\r\n"
	writeNativeStructureSource(t, root, "delivery/value.go", raw)
	writeNativeStructureSource(t, root, "delivery/value_test.go", "test-local substitute must not become implementation")
	writeNativeStructureSource(t, root, "workorch_validation/conjunctive/input.go", "old validation is not implementation")
	manifest := runtimecontract.WorkspaceManifest{Revision: "tree", TargetRefs: []string{"workorch_validation/conjunctive/input.go", "go.mod", "delivery/value_test.go", "delivery/value.go"}}
	c := shellValidationGenerationContext(root, manifest, "catalog")
	if c.Validate("catalog") != nil || c.Outcome != "complete" || len(c.Files) != 2 || c.Files[0].Content != raw {
		t.Fatalf("actual source missing: %+v", c)
	}
	before := c.Clone()
	if err := os.WriteFile(filepath.Join(root, "delivery/value.go"), []byte("package delivery\nfunc Value() int { return 8 }\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if after := shellValidationGenerationContext(root, manifest, "catalog"); reflect.DeepEqual(before, after) {
		t.Fatal("old write/source observation reused")
	}
	for _, scenario := range []string{"missing", "oversize", "binary", "symlink"} {
		t.Run(scenario, func(t *testing.T) {
			directory := t.TempDir()
			writeNativeStructureSource(t, directory, "go.mod", "module fixture\n")
			m := runtimecontract.WorkspaceManifest{TargetRefs: []string{"go.mod", "source.go"}}
			switch scenario {
			case "oversize":
				writeNativeStructureSource(t, directory, "source.go", strings.Repeat("x", runtimecontract.RuntimeValidationGenerationContextMaxBytes+1))
			case "binary":
				writeNativeStructureSource(t, directory, "source.go", string([]byte{255, 254}))
			case "symlink":
				if err := os.Symlink(filepath.Join(root, "delivery/value.go"), filepath.Join(directory, "source.go")); err != nil {
					t.Fatal(err)
				}
			}
			got := shellValidationGenerationContext(directory, m, "r1")
			if got.Validate("r1") != nil || got.Outcome == "complete" || len(got.Files) != 0 {
				t.Fatal("partial source passed as complete")
			}
		})
	}
}

func TestConjunctivePreparationPublishesOneSharedActualGenerationContext(t *testing.T) {
	recipe, requirement, _ := conjunctiveSourceRequirement(t)
	root := t.TempDir()
	writeNativeStructureSource(t, root, "go.mod", "module fixture\ngo 1.26\n")
	writeNativeStructureSource(t, root, "delivery/value.go", "package delivery\nfunc Compute(x int) int { return x+7 }\n")
	binding := shellTestWorkspaceManifestBinding("tree", []string{"go.mod", "delivery/value.go"})
	surfaces := shellValidationSurfaces([]runtimecontract.RuntimeValidationRecipe{recipe}, []runtimecontract.RuntimeValidationRecipe{recipe}, nil, []runtimecontract.RuntimeValidationProofRequirement{requirement}, binding, "catalog", root)
	if len(surfaces) != 1 || surfaces[0].Validate() != nil || surfaces[0].GenerationContext == nil || surfaces[0].GenerationContext.Outcome != "complete" || len(surfaces[0].GenerationContext.Files) != 2 || len(surfaces[0].ProofRequirementPreparations) != 1 || len(surfaces[0].ProofRequirementPreparations[0].CheckAttestations) != 3 {
		t.Fatal("shared preparation context not connected")
	}
	if !reflect.DeepEqual(surfaces[0].ProofRequirementPreparations[0].SourceObligationChecks, requirement.SourceObligationChecks) {
		t.Fatal("preparation lost original per-check criteria")
	}
}

func TestValidationGenerationContextConcurrentProjectsAreIndependent(t *testing.T) {
	for _, task := range []struct{ name, source string }{
		{"gateway", "package gateway\nfunc Route() string { return \"proxy\" }\n"},
		{"inventory", "package inventory\nfunc Count() int { return 99 }\n"},
	} {
		t.Run(task.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			writeNativeStructureSource(t, root, "source.go", task.source)
			manifest := runtimecontract.WorkspaceManifest{TargetRefs: []string{"source.go"}}
			for range 3 {
				c := shellValidationGenerationContext(root, manifest, "revision-"+task.name)
				if c.Validate("revision-"+task.name) != nil || len(c.Files) != 1 || c.Files[0].Content != task.source {
					t.Fatal("another project's source or revision leaked into this context")
				}
				c.Files[0].Content = "mutated consumer copy"
			}
		})
	}
}
