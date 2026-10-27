package contextregistryports

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/skillhubpreparation"
)

func TestPreparationSkillReferencesReadExactInstalledRootWithoutRetry(t *testing.T) {
	for _, global := range []bool{false, true} {
		t.Run(map[bool]string{false: "workspace", true: "global"}[global], func(t *testing.T) {
			raw := []byte("---\r\nreference_refs: [references/a.bin, empty]\r\n---\r\noriginal\r\n")
			dir, root, runner := preparationInstructionFixture(t, global, "group/nested", raw)
			path := filepath.Join(root, "group", "nested", "references", "a.bin")
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				t.Fatal(err)
			}
			original := []byte{0xff, 0, 13, 10, 0x80}
			if err := os.WriteFile(path, original, 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, "group", "nested", "empty"), nil, 0600); err != nil {
				t.Fatal(err)
			}
			v, err := ObserveSkillHubPreparationInstructions(context.Background(), runner, "ws", dir, global)
			if err != nil || v.References == nil || runner.calls != 1 {
				t.Fatal("observation", err)
			}
			installed := skillhubpreparation.Installed{Version: skillhubpreparation.InstalledVersion, Root: root, First: []skillhubpreparation.Instruction{preparationInstructionFact(v.Instructions[0])}, Second: []skillhubpreparation.Instruction{preparationInstructionFact(v.ReobservedInstructions[0])}, StartedAt: v.StartedAt, CompletedAt: v.CompletedAt, References: v.References}
			if !v.References.CompleteFor(installed) || len(v.References.First) != 2 || !bytes.Equal(v.References.First[0].File.Raw, original) || !bytes.Equal(v.References.Second[0].File.Raw, original) || v.References.First[0].File.Path != path || len(v.References.First[1].File.Raw) != 0 {
				t.Fatal("original binary/empty reads lost", v.References.FailureCode)
			}
			clone := installed.Clone()
			clone.References.First[0].File.Raw[0] = 0
			if !bytes.Equal(installed.References.First[0].File.Raw, original) {
				t.Fatal("alias")
			}
			for _, mutate := range []func(*skillhubpreparation.References){
				func(r *skillhubpreparation.References) {
					r.First[0].File.Path = filepath.Join(root, "sibling", "a.bin")
				},
				func(r *skillhubpreparation.References) { r.Second = nil },
				func(r *skillhubpreparation.References) { r.Second[0].File.Raw = []byte("drift") },
				func(r *skillhubpreparation.References) { r.First = r.First[:1] },
				func(r *skillhubpreparation.References) { r.Version = "future" },
			} {
				bad := installed.Clone()
				mutate(bad.References)
				if bad.References.CompleteFor(bad) {
					t.Fatal("partial/foreign/drift observation accepted")
				}
			}
			got, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(got, original) || !reflect.DeepEqual(v.Instructions[0].Raw, raw) {
				t.Fatal("observation changed source", err)
			}
		})
	}
}

func TestPreparationSkillReferencesFailClosedWithoutSourceFallback(t *testing.T) {
	for _, scenario := range []string{"missing", "parent", "sibling", "absolute", "url", "unclean", "duplicate", "symlink", "directory", "oversize"} {
		t.Run(scenario, func(t *testing.T) {
			ref := "references/a"
			switch scenario {
			case "parent":
				ref = "../outside"
			case "sibling":
				ref = "../other/a"
			case "absolute":
				ref = "/etc/passwd"
			case "url":
				ref = "https://invalid.test/a"
			case "unclean":
				ref = "references/./a"
			}
			refs := "'" + ref + "'"
			if scenario == "duplicate" {
				refs += ", '" + ref + "'"
			}
			dir, root, runner := preparationInstructionFixture(t, false, "s", []byte("---\nreference_refs: ["+refs+"]\n---\noriginal"))
			if err := os.MkdirAll(filepath.Join(root, "s", "references"), 0700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(root, "s", "references", "a")
			switch scenario {
			case "symlink":
				outside := filepath.Join(t.TempDir(), "outside")
				if err := os.WriteFile(outside, []byte("not allowed"), 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(outside, path); err != nil {
					t.Fatal(err)
				}
			case "directory":
				if err := os.Mkdir(path, 0700); err != nil {
					t.Fatal(err)
				}
			case "oversize":
				if err := os.WriteFile(path, []byte(strings.Repeat("x", skillhubpreparation.InstructionLimit+1)), 0600); err != nil {
					t.Fatal(err)
				}
			}
			v, err := ObserveSkillHubPreparationInstructions(context.Background(), runner, "ws", dir, false)
			if err != nil || v.References == nil || v.References.FailureCode == "" || runner.calls != 1 {
				t.Fatal("unsafe source credited or status repeated", err)
			}
			for _, r := range v.References.First {
				if r.File.Complete || len(r.File.Raw) != 0 {
					t.Fatal("unsafe content was read")
				}
			}
		})
	}
}
