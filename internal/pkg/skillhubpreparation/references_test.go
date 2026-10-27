package skillhubpreparation

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/fileobservation"
)

func TestPreparationSkillReferencesBoundPlanAndOriginalDiagnostics(t *testing.T) {
	f, _ := installedFixture()
	for _, scenario := range []string{"valid", "limit", "duplicate", "parent", "unknown_metadata", "source_path"} {
		t.Run(scenario, func(t *testing.T) {
			v := f.Clone()
			refs := "references/a"
			switch scenario {
			case "parent":
				refs = "../other"
			case "duplicate":
				refs = "references/a, references/a"
			case "limit":
				// Multiple valid installed skills exceed only the scope bound.
				v.First = nil
				for i := range 5 {
					items := []string{}
					for n := range 64 {
						items = append(items, fmt.Sprintf("r%d", n))
					}
					raw := []byte("---\nreference_refs: [" + strings.Join(items, ",") + "]\n---\nbody")
					v.First = append(v.First, Instruction{SkillID: fmt.Sprintf("s%d", i), Path: fmt.Sprintf("%s/s%d/SKILL.md", v.Root, i), Raw: raw, Complete: true})
				}
			}
			if scenario != "limit" {
				raw := []byte("---\nreference_refs: [" + refs + "]\n---\nbody")
				if scenario == "unknown_metadata" {
					raw = []byte("---\nReferenceRefs: [a]\n---\nbody")
				}
				v.First[0].Raw = raw
				if scenario == "source_path" {
					v.First[0].Path = "/fallback/source/SKILL.md"
				}
			}
			plan, code := ReferencePlan(v.Root, v.First)
			if scenario == "valid" {
				if code != "" || len(plan) != 1 {
					t.Fatal("valid plan rejected", code)
				}
			} else if code == "" || plan != nil {
				t.Fatal("invalid/oversized plan silently truncated", scenario)
			}
		})
	}
	r := References{}
	original := []byte("original diagnostic\xff\x00\r\n")
	r.PreserveFailure("reference_unavailable", errors.New(string(original)))
	if !bytes.Equal(r.ErrorText, original) || r.ErrorBytes != uint64(len(original)) || r.ErrorSHA256 != fileobservation.Digest(original) {
		t.Fatal("diagnostic rewritten")
	}
	clone := r.Clone()
	clone.ErrorText[0] = 'X'
	if bytes.Equal(clone.ErrorText, r.ErrorText) {
		t.Fatal("diagnostic alias")
	}
	large := strings.Repeat("x", (64<<10)+1)
	r = References{}
	r.PreserveFailure("reference_unavailable", errors.New(large))
	if len(r.ErrorText) != 0 || r.ErrorBytes != uint64(len(large)) || r.ErrorSHA256 != fileobservation.Digest([]byte(large)) || r.ErrorNotSavedReason != "error_exceeds_storage_limit" {
		t.Fatal("oversized diagnostic silently truncated")
	}
}
