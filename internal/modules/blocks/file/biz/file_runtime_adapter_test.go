package file

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	fileevents "github.com/muidea/workorch/internal/modules/blocks/file/pkg/events"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type runtimeAdapterFailingInfoEntry struct {
	fs.DirEntry
	err error
}

func (e runtimeAdapterFailingInfoEntry) Info() (fs.FileInfo, error) {
	return nil, e.err
}

func TestRuntimeAdapterProbeRejectsDirectoryPatchTargetWithoutMutation(t *testing.T) {
	workspace := t.TempDir()
	patchType, content := "file_write", "new content\n"
	adapter := NewRuntimeAdapter()
	inventory, err := adapter.AcquireRuntimeOperationAdmissibility(workspace, "admissibility-file", []string{runtimecontract.RuntimeBlockKindPatchApply}, []string{"."}, 4)
	if err != nil {
		t.Fatal(err)
	}
	failure := adapter.ProbeRuntimeOperationFeasibility(workspace, inventory, runtimecontract.RuntimeOperation{
		ID: "patch-directory", Kind: runtimecontract.RuntimeBlockKindPatchApply, TargetRefs: []string{"."},
		Parameters: []runtimecontract.OperationParameter{
			{Name: "patchType", String: &patchType},
			{Name: "content", String: &content},
		},
	})
	if failure == nil || failure.Code != "operation_target_not_regular" {
		t.Fatalf("directory patch feasibility failure = %+v", failure)
	}
	entries, err := os.ReadDir(workspace)
	if err != nil || len(entries) != 0 {
		t.Fatalf("probe mutated workspace entries=%+v err=%v", entries, err)
	}
}

func TestRuntimeAdapterFileWriteSameContentPreservesWorkspaceManifestRevision(t *testing.T) {
	workspace := t.TempDir()
	const targetRef = "existing.txt"
	const content = "unchanged\n"
	if err := os.WriteFile(filepath.Join(workspace, targetRef), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	adapter := NewRuntimeAdapter()
	before, err := adapter.AcquireWorkspaceManifest(workspace)
	if err != nil {
		t.Fatal(err)
	}
	result, err := adapter.ApplyPatch(PatchApplyRequest{
		WorkspaceDirectory: workspace,
		TargetRef:          targetRef,
		PatchType:          patchTypeFileWrite,
		Content:            content,
		ContentPresent:     true,
		BaseHash:           sha256Hex([]byte(content)),
	})
	if err != nil || result.BeforeHash != result.AfterHash || result.BeforeHash == "" {
		t.Fatalf("same-content file write = %+v err=%v", result, err)
	}
	after, err := adapter.AcquireWorkspaceManifest(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if after.Revision != before.Revision {
		t.Fatalf("same-content file write changed workspace revision: before=%s after=%s", before.Revision, after.Revision)
	}
}

func TestRuntimeAdapterAcquireRuntimeTargetSelectionIsCardinalityBounded(t *testing.T) {
	adapter := NewRuntimeAdapter()
	t.Run("empty workspace grounds creation scope", func(t *testing.T) {
		selection, err := adapter.AcquireRuntimeTargetSelection(t.TempDir(), "target-selection-create", "deliverable", runtimeFileTargetSelectorWorkspaceDeliverable, "initial-deliverable.txt", 4)
		if err != nil {
			t.Fatal(err)
		}
		if selection.Disposition != runtimecontract.RuntimeTargetSelectionCreate || selection.CandidateCount != 0 || len(selection.TargetRefs) != 0 || len(selection.CandidateTargetRefs) != 0 || selection.SnapshotID == "" {
			t.Fatalf("empty workspace creation selection = %+v", selection)
		}
		if selection.SelectorRef != runtimeFileTargetSelectorWorkspaceDeliverable || selection.ProspectiveTargetRef != "initial-deliverable.txt" {
			t.Fatalf("creation selection lost owner selector binding: %+v", selection)
		}
	})
	t.Run("post-binding bootstrap target uses open declaration selector", func(t *testing.T) {
		binding := runtimeTargetSelectionBinding{ownerRef: "software-dev", subjectRef: "go-service/source"}
		selection, err := adapter.AcquireRuntimeTargetSelection(
			t.TempDir(), "target-selection-bootstrap-create", "implementation",
			runtimeFileTargetSelectorWorkspaceDeliverable, "cmd/service/main.go", 4, binding,
		)
		if err != nil {
			t.Fatal(err)
		}
		if selection.Disposition != runtimecontract.RuntimeTargetSelectionCreate || selection.ProspectiveTargetRef != "cmd/service/main.go" ||
			selection.SelectorOwnerRef != binding.ownerRef || selection.SelectorSubjectRef != binding.subjectRef ||
			len(selection.SelectorCandidateRefs) != 0 || selection.CandidateCount != 0 {
			t.Fatalf("post-binding bootstrap target selection = %+v", selection)
		}
	})
	t.Run("workspace-relative new target grounds an absent prospective path", func(t *testing.T) {
		workspace := t.TempDir()
		binding := runtimeTargetSelectionBinding{ownerRef: "blocks/file", subjectRef: "workspace_deliverable"}
		selection, err := adapter.AcquireRuntimeTargetSelection(workspace, "target-selection-prospective", "semantic_proof_target", runtimeFileSelectorWorkspaceRelativeNewTarget, "coverage-map.json", 4, binding)
		if err != nil {
			t.Fatal(err)
		}
		if selection.Disposition != runtimecontract.RuntimeTargetSelectionCreate || selection.ProspectiveTargetRef != "coverage-map.json" || selection.CandidateCount != 0 || len(selection.TargetRefs) != 0 || len(selection.CandidateTargetRefs) != 0 || selection.SnapshotID == "" {
			t.Fatalf("prospective creation selection = %+v", selection)
		}
		if selection.SelectorOwnerRef != binding.ownerRef || selection.SelectorSubjectRef != binding.subjectRef {
			t.Fatalf("prospective selection lost owner binding: %+v", selection)
		}
	})
	t.Run("workspace-relative new target rejects an existing prospective path", func(t *testing.T) {
		workspace := t.TempDir()
		if err := os.WriteFile(filepath.Join(workspace, "coverage-map.json"), []byte("{}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		selection, err := adapter.AcquireRuntimeTargetSelection(workspace, "target-selection-existing-prospective", "semantic_proof_target", runtimeFileSelectorWorkspaceRelativeNewTarget, "coverage-map.json", 4,
			runtimeTargetSelectionBinding{ownerRef: "blocks/file", subjectRef: "workspace_deliverable"})
		if err != nil {
			t.Fatal(err)
		}
		if selection.Disposition != runtimecontract.RuntimeTargetSelectionUnsupported || selection.ProspectiveTargetRef != "coverage-map.json" || selection.CandidateCount != 0 || len(selection.TargetRefs) != 0 || len(selection.CandidateTargetRefs) != 0 || selection.SnapshotID == "" {
			t.Fatalf("existing prospective selection = %+v", selection)
		}
	})
	t.Run("empty workspace without profile artifact is typed absent", func(t *testing.T) {
		selection, err := adapter.AcquireRuntimeTargetSelection(t.TempDir(), "target-selection-absent", "deliverable", runtimeFileTargetSelectorWorkspaceDeliverable, "", 4)
		if err != nil {
			t.Fatal(err)
		}
		if selection.Disposition != runtimecontract.RuntimeTargetSelectionAbsent || selection.CandidateCount != 0 || selection.ProspectiveTargetRef != "" || selection.SnapshotID == "" {
			t.Fatalf("empty workspace absent selection = %+v", selection)
		}
	})
	t.Run("profile-bound empty workspace is a File-owner creation scope", func(t *testing.T) {
		binding := runtimeTargetSelectionBinding{ownerRef: "software-dev", subjectRef: "workspace_deliverable_create_scope"}
		selection, err := adapter.AcquireRuntimeTargetSelection(t.TempDir(), "target-selection-empty-profile-scope", "deliverable", runtimeFileTargetSelectorWorkspaceDeliverable, "", 4, binding)
		if err != nil {
			t.Fatal(err)
		}
		if selection.Disposition != runtimecontract.RuntimeTargetSelectionScope || selection.CandidateCount != 0 || len(selection.TargetRefs) != 0 ||
			selection.SelectorOwnerRef != binding.ownerRef || selection.SelectorSubjectRef != binding.subjectRef || selection.ProspectiveTargetRef != "" {
			t.Fatalf("profile-bound empty workspace scope = %+v", selection)
		}
	})
	t.Run("all-of declaration keeps an empty workspace open for semantic creation", func(t *testing.T) {
		binding := runtimeTargetSelectionBinding{
			ownerRef: "software-dev", subjectRef: "workspace_deliverable_create_scope",
			prospectiveTargetAllOfConstraintRefs: []string{
				"workspace-relative-path",
				"new-target-requires-owner-selector",
			},
		}
		selection, err := adapter.AcquireRuntimeTargetSelection(t.TempDir(), "target-selection-empty-all-of-scope", "deliverable", runtimeFileTargetSelectorWorkspaceDeliverable, "", 4, binding)
		if err != nil {
			t.Fatal(err)
		}
		if selection.Disposition != runtimecontract.RuntimeTargetSelectionScope || selection.CandidateCount != 0 ||
			!runtimecontract.SameRuntimeProspectiveTargetAllOfConstraintRefs(selection.ProspectiveTargetAllOfConstraintRefs, binding.prospectiveTargetAllOfConstraintRefs) {
			t.Fatalf("all-of empty workspace scope = %+v", selection)
		}
	})
	t.Run("unsupported all-of declaration is a typed owner fact", func(t *testing.T) {
		binding := runtimeTargetSelectionBinding{
			ownerRef: "software-dev", subjectRef: "workspace_deliverable_create_scope",
			prospectiveTargetAllOfConstraintRefs: []string{"workspace-relative-path"},
		}
		selection, err := adapter.AcquireRuntimeTargetSelection(t.TempDir(), "target-selection-unsupported-all-of", "deliverable", runtimeFileTargetSelectorWorkspaceDeliverable, "", 4, binding)
		if err != nil {
			t.Fatal(err)
		}
		if selection.Disposition != runtimecontract.RuntimeTargetSelectionUnsupported ||
			!runtimecontract.SameRuntimeProspectiveTargetAllOfConstraintRefs(selection.ProspectiveTargetAllOfConstraintRefs, binding.prospectiveTargetAllOfConstraintRefs) {
			t.Fatalf("unsupported all-of selection = %+v", selection)
		}
	})
	t.Run("empty workspace rejects noncanonical prospective target", func(t *testing.T) {
		workspace := t.TempDir()
		for _, target := range []string{"../escape.txt", "/absolute.txt", "nested/../deliverable.txt", `nested\\deliverable.txt`} {
			t.Run(target, func(t *testing.T) {
				if _, err := adapter.AcquireRuntimeTargetSelection(workspace, "target-selection-invalid-"+strings.ReplaceAll(target, "/", "-"), "deliverable", runtimeFileTargetSelectorWorkspaceDeliverable, target, 4); !errors.Is(err, common.ErrInvalidArgument) {
					t.Fatalf("prospective target %q error = %v, want invalid argument", target, err)
				}
			})
		}
	})
	t.Run("unique", func(t *testing.T) {
		workspace := t.TempDir()
		if err := os.WriteFile(filepath.Join(workspace, "deliverable.txt"), []byte("value\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		selection, err := adapter.AcquireRuntimeTargetSelection(workspace, "target-selection-1", "deliverable", runtimeFileTargetSelectorUniqueWorkspaceEntry, "", 4)
		if err != nil {
			t.Fatal(err)
		}
		if selection.Disposition != runtimecontract.RuntimeTargetSelectionSelected || len(selection.TargetRefs) != 1 || selection.TargetRefs[0] != "deliverable.txt" || selection.SnapshotID == "" {
			t.Fatalf("unique selection = %+v", selection)
		}
		if selection.SelectorRef != runtimeFileTargetSelectorUniqueWorkspaceEntry {
			t.Fatalf("unique selection lost owner selector binding: %+v", selection)
		}
	})
	t.Run("selected inventory preserves prospective declaration", func(t *testing.T) {
		workspace := t.TempDir()
		if err := os.WriteFile(filepath.Join(workspace, "initial-deliverable.txt"), []byte("value\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		selection, err := adapter.AcquireRuntimeTargetSelection(workspace, "target-selection-after-create", "deliverable", runtimeFileTargetSelectorWorkspaceDeliverable, "initial-deliverable.txt", 4)
		if err != nil {
			t.Fatal(err)
		}
		if selection.Disposition != runtimecontract.RuntimeTargetSelectionSelected || selection.TargetRefs[0] != "initial-deliverable.txt" || selection.ProspectiveTargetRef != "initial-deliverable.txt" {
			t.Fatalf("selection lost prospective declaration after create: %+v", selection)
		}
	})
	t.Run("bounded workspace scope", func(t *testing.T) {
		workspace := t.TempDir()
		for _, name := range []string{"implementation.txt", "support.txt"} {
			if err := os.WriteFile(filepath.Join(workspace, name), []byte(name), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		selection, err := adapter.AcquireRuntimeTargetSelection(workspace, "target-selection-scope", "deliverable", runtimeFileTargetSelectorWorkspaceDeliverable, "", 4)
		if err != nil {
			t.Fatal(err)
		}
		if selection.Disposition != runtimecontract.RuntimeTargetSelectionScope || selection.CandidateCount != 2 || len(selection.TargetRefs) != 2 || len(selection.CandidateTargetRefs) != 2 || selection.TargetRefs[0] != "implementation.txt" || selection.TargetRefs[1] != "support.txt" {
			t.Fatalf("workspace scope selection = %+v", selection)
		}
		bounded, err := adapter.AcquireRuntimeTargetSelection(workspace, "target-selection-scope-bounded", "deliverable", runtimeFileTargetSelectorWorkspaceDeliverable, "", 1)
		if err != nil {
			t.Fatal(err)
		}
		if bounded.Disposition != runtimecontract.RuntimeTargetSelectionAmbiguous || bounded.CandidateCount != 2 || len(bounded.TargetRefs) != 0 || len(bounded.CandidateTargetRefs) != 1 {
			t.Fatalf("bounded workspace scope selection = %+v", bounded)
		}
	})
	t.Run("ambiguous", func(t *testing.T) {
		workspace := t.TempDir()
		for _, name := range []string{"validation-entry.txt", "implementation.txt"} {
			if err := os.WriteFile(filepath.Join(workspace, name), []byte(name), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		selection, err := adapter.AcquireRuntimeTargetSelection(workspace, "target-selection-2", "deliverable", runtimeFileTargetSelectorUniqueWorkspaceEntry, "", 1)
		if err != nil {
			t.Fatal(err)
		}
		if selection.Disposition != runtimecontract.RuntimeTargetSelectionAmbiguous || len(selection.TargetRefs) != 0 || len(selection.CandidateTargetRefs) != 1 || selection.CandidateCount != 2 {
			t.Fatalf("ambiguous selection = %+v", selection)
		}
	})
	t.Run("unknown selector returns typed unsupported observation", func(t *testing.T) {
		selection, err := adapter.AcquireRuntimeTargetSelection(t.TempDir(), "target-selection-3", "deliverable", "profile/free_form_selector", "", 4)
		if err != nil {
			t.Fatal(err)
		}
		if selection.Disposition != runtimecontract.RuntimeTargetSelectionUnsupported || selection.ObservedRevision != "unsupported-selector:profile/free_form_selector" || selection.SnapshotID == "" {
			t.Fatalf("unknown File selector observation = %+v", selection)
		}
	})
}

func TestRuntimeAdapterUniqueWorkspaceEntryProspectiveAdmissionMatrix(t *testing.T) {
	type matrixCase struct {
		name               string
		currentTargets     []string
		prospective        string
		wantDisposition    runtimecontract.RuntimeTargetSelectionDisposition
		wantTargetRefs     []string
		wantCandidateRefs  []string
		wantCandidateCount int
	}
	cases := []matrixCase{
		{name: "zero/no prospective", wantDisposition: runtimecontract.RuntimeTargetSelectionAbsent},
		{name: "one/no prospective", currentTargets: []string{"one.txt"}, wantDisposition: runtimecontract.RuntimeTargetSelectionSelected, wantTargetRefs: []string{"one.txt"}, wantCandidateRefs: []string{"one.txt"}, wantCandidateCount: 1},
		{name: "many/no prospective", currentTargets: []string{"one.txt", "two.txt"}, wantDisposition: runtimecontract.RuntimeTargetSelectionAmbiguous, wantCandidateRefs: []string{"one.txt", "two.txt"}, wantCandidateCount: 2},
		{name: "zero/prospective", prospective: "prospective.txt", wantDisposition: runtimecontract.RuntimeTargetSelectionUnsupported},
		{name: "one/prospective", currentTargets: []string{"one.txt"}, prospective: "prospective.txt", wantDisposition: runtimecontract.RuntimeTargetSelectionUnsupported},
		{name: "many/prospective", currentTargets: []string{"one.txt", "two.txt"}, prospective: "prospective.txt", wantDisposition: runtimecontract.RuntimeTargetSelectionUnsupported},
		{name: "noncanonical prospective", prospective: "../escape.txt", wantDisposition: runtimecontract.RuntimeTargetSelectionUnsupported},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			workspace := t.TempDir()
			for _, targetRef := range testCase.currentTargets {
				if err := os.WriteFile(filepath.Join(workspace, targetRef), []byte(targetRef+"\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			before, err := NewRuntimeAdapter().AcquireWorkspaceManifest(workspace)
			if err != nil {
				t.Fatal(err)
			}
			acquire := func(adapter *RuntimeAdapter) runtimecontract.RuntimeTargetSelection {
				selection, err := adapter.AcquireRuntimeTargetSelection(
					workspace, "target-selection-unique-prospective-matrix", "deliverable",
					runtimeFileTargetSelectorUniqueWorkspaceEntry, testCase.prospective, 4,
				)
				if err != nil {
					t.Fatal(err)
				}
				return selection
			}
			first := acquire(NewRuntimeAdapter())
			duplicate := acquire(NewRuntimeAdapter())
			if first.Disposition != testCase.wantDisposition ||
				!slices.Equal(first.TargetRefs, testCase.wantTargetRefs) ||
				!slices.Equal(first.CandidateTargetRefs, testCase.wantCandidateRefs) ||
				first.CandidateCount != testCase.wantCandidateCount ||
				first.ProspectiveTargetRef != testCase.prospective ||
				first.ObservedRevision != before.Revision || first.SnapshotID == "" {
				t.Fatalf("unique selector admission = %+v, want disposition=%q targets=%v candidates=%v count=%d prospective=%q revision=%q", first, testCase.wantDisposition, testCase.wantTargetRefs, testCase.wantCandidateRefs, testCase.wantCandidateCount, testCase.prospective, before.Revision)
			}
			if !reflect.DeepEqual(first, duplicate) {
				t.Fatalf("duplicate/fresh-adapter admission drifted: first=%+v duplicate=%+v", first, duplicate)
			}
			after, err := NewRuntimeAdapter().AcquireWorkspaceManifest(workspace)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before, after) {
				t.Fatalf("unique selector admission mutated workspace: before=%+v after=%+v", before, after)
			}
		})
	}
}

func TestRuntimeAdapterProspectiveAdmissionRejectsManifestExcludedTargetsWithoutMutation(t *testing.T) {
	type selectorCase struct {
		name              string
		selector          string
		inventoryVariants [][]string
	}
	selectors := []selectorCase{
		{name: "workspace-relative-new", selector: runtimeFileSelectorWorkspaceRelativeNewTarget, inventoryVariants: [][]string{nil}},
		{name: "workspace-deliverable", selector: runtimeFileTargetSelectorWorkspaceDeliverable, inventoryVariants: [][]string{nil, {"current.txt"}, {"current-a.txt", "current-b.txt"}}},
		{name: "profile-declared", selector: runtimeFileTargetSelectorProfileCandidates, inventoryVariants: [][]string{nil, {"current.txt"}, {"current-a.txt", "current-b.txt"}}},
	}
	excludedSegments := []string{".git", "node_modules", "vendor", "dist", "build", ".workorch-data"}
	for _, selector := range selectors {
		for _, currentTargets := range selector.inventoryVariants {
			for _, excluded := range excludedSegments {
				for _, targetRef := range []string{excluded + "/artifact.txt", "nested/" + excluded + "/artifact.txt"} {
					name := fmt.Sprintf("%s/inventory-%d/%s", selector.name, len(currentTargets), strings.ReplaceAll(targetRef, "/", "-"))
					t.Run(name, func(t *testing.T) {
						workspace := t.TempDir()
						adapter := NewRuntimeAdapter()
						for _, currentTarget := range currentTargets {
							if err := os.WriteFile(filepath.Join(workspace, currentTarget), []byte(currentTarget+"\n"), 0o644); err != nil {
								t.Fatal(err)
							}
						}
						before, err := adapter.AcquireWorkspaceManifest(workspace)
						if err != nil {
							t.Fatal(err)
						}
						binding := runtimeTargetSelectionBinding{}
						if selector.selector == runtimeFileTargetSelectorProfileCandidates {
							candidateRefs := append([]string{targetRef}, currentTargets...)
							binding = runtimeTargetSelectionBinding{ownerRef: "profile-audit", subjectRef: "deliverable", candidateRefs: candidateRefs}
						}
						acquire := func() runtimecontract.RuntimeTargetSelection {
							selection, acquireErr := NewRuntimeAdapter().AcquireRuntimeTargetSelection(
								workspace, "target-selection-excluded-prospective", "deliverable", selector.selector, targetRef, 4, binding,
							)
							if acquireErr != nil {
								t.Fatal(acquireErr)
							}
							return selection
						}
						first, duplicate := acquire(), acquire()
						if first.Disposition != runtimecontract.RuntimeTargetSelectionUnsupported || first.ProspectiveTargetRef != targetRef ||
							first.CandidateCount != 0 || len(first.TargetRefs) != 0 || len(first.CandidateTargetRefs) != 0 ||
							first.ObservedRevision != before.Revision || first.SnapshotID == "" {
							t.Fatalf("excluded prospective admission = %+v", first)
						}
						if !reflect.DeepEqual(first, duplicate) {
							t.Fatalf("duplicate excluded prospective admission drifted: first=%+v duplicate=%+v", first, duplicate)
						}
						if _, statErr := os.Lstat(filepath.Join(workspace, filepath.FromSlash(targetRef))); !os.IsNotExist(statErr) {
							t.Fatalf("excluded prospective admission changed target %q: %v", targetRef, statErr)
						}
						after, err := adapter.AcquireWorkspaceManifest(workspace)
						if err != nil {
							t.Fatal(err)
						}
						if !reflect.DeepEqual(before, after) {
							t.Fatalf("excluded prospective admission mutated manifest: before=%+v after=%+v", before, after)
						}
					})
				}
			}
		}
	}
}

func TestRuntimeAdapterProspectiveAdmissionDoesNotIgnoreNonParticipatingTarget(t *testing.T) {
	for _, currentTargets := range [][]string{{"current.txt"}, {"current-a.txt", "current-b.txt"}} {
		t.Run(fmt.Sprintf("workspace-deliverable/inventory-%d", len(currentTargets)), func(t *testing.T) {
			workspace := t.TempDir()
			for _, currentTarget := range currentTargets {
				if err := os.WriteFile(filepath.Join(workspace, currentTarget), []byte(currentTarget+"\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			before, err := NewRuntimeAdapter().AcquireWorkspaceManifest(workspace)
			if err != nil {
				t.Fatal(err)
			}
			const prospectiveTarget = "future.txt"
			acquire := func() runtimecontract.RuntimeTargetSelection {
				selection, acquireErr := NewRuntimeAdapter().AcquireRuntimeTargetSelection(
					workspace, "target-selection-nonparticipating-prospective", "deliverable", runtimeFileTargetSelectorWorkspaceDeliverable, prospectiveTarget, 4,
				)
				if acquireErr != nil {
					t.Fatal(acquireErr)
				}
				return selection
			}
			first, duplicate := acquire(), acquire()
			wantDisposition := runtimecontract.RuntimeTargetSelectionSelected
			if len(currentTargets) > 1 {
				wantDisposition = runtimecontract.RuntimeTargetSelectionScope
			}
			if first.Disposition != wantDisposition || first.ProspectiveTargetRef != prospectiveTarget ||
				first.CandidateCount != len(currentTargets) || !slices.Equal(first.TargetRefs, currentTargets) ||
				!reflect.DeepEqual(first, duplicate) {
				t.Fatalf("workspace prospective admission first=%+v duplicate=%+v", first, duplicate)
			}
			if _, statErr := os.Lstat(filepath.Join(workspace, prospectiveTarget)); !os.IsNotExist(statErr) {
				t.Fatalf("prospective admission changed target: %v", statErr)
			}
			after, err := NewRuntimeAdapter().AcquireWorkspaceManifest(workspace)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before, after) {
				t.Fatalf("workspace prospective admission mutated manifest: before=%+v after=%+v", before, after)
			}
		})

		t.Run(fmt.Sprintf("profile-declared/inventory-%d", len(currentTargets)), func(t *testing.T) {
			workspace := t.TempDir()
			for _, currentTarget := range currentTargets {
				if err := os.WriteFile(filepath.Join(workspace, currentTarget), []byte(currentTarget+"\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			before, err := NewRuntimeAdapter().AcquireWorkspaceManifest(workspace)
			if err != nil {
				t.Fatal(err)
			}
			const prospectiveTarget = "future.txt"
			candidateRefs := append([]string{prospectiveTarget}, currentTargets...)
			binding := runtimeTargetSelectionBinding{ownerRef: "profile-audit", subjectRef: "deliverable", candidateRefs: candidateRefs}
			acquire := func() runtimecontract.RuntimeTargetSelection {
				selection, acquireErr := NewRuntimeAdapter().AcquireRuntimeTargetSelection(
					workspace, "target-selection-nonparticipating-prospective", "deliverable", runtimeFileTargetSelectorProfileCandidates, prospectiveTarget, 4, binding,
				)
				if acquireErr != nil {
					t.Fatal(acquireErr)
				}
				return selection
			}
			first, duplicate := acquire(), acquire()
			if first.Disposition != runtimecontract.RuntimeTargetSelectionUnsupported || first.ProspectiveTargetRef != prospectiveTarget ||
				first.CandidateCount != 0 || len(first.TargetRefs) != 0 || len(first.CandidateTargetRefs) != 0 || first.SnapshotID == "" {
				t.Fatalf("nonparticipating prospective admission = %+v", first)
			}
			if !reflect.DeepEqual(first, duplicate) {
				t.Fatalf("duplicate nonparticipating prospective admission drifted: first=%+v duplicate=%+v", first, duplicate)
			}
			after, err := NewRuntimeAdapter().AcquireWorkspaceManifest(workspace)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before, after) {
				t.Fatalf("nonparticipating prospective admission mutated manifest: before=%+v after=%+v", before, after)
			}
		})
	}
}

func TestRuntimeAdapterProfileProspectiveMustBelongToDeclaredCandidatesAcrossCardinality(t *testing.T) {
	for _, currentTargets := range [][]string{nil, {"current.txt"}, {"current-a.txt", "current-b.txt"}} {
		t.Run(fmt.Sprintf("inventory-%d", len(currentTargets)), func(t *testing.T) {
			workspace := t.TempDir()
			for _, currentTarget := range currentTargets {
				if err := os.WriteFile(filepath.Join(workspace, currentTarget), []byte(currentTarget+"\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			before, err := NewRuntimeAdapter().AcquireWorkspaceManifest(workspace)
			if err != nil {
				t.Fatal(err)
			}
			candidateRefs := append([]string{"declared-future.txt"}, currentTargets...)
			binding := runtimeTargetSelectionBinding{ownerRef: "profile-audit", subjectRef: "deliverable", candidateRefs: candidateRefs}
			for attempt := 0; attempt < 2; attempt++ {
				_, acquireErr := NewRuntimeAdapter().AcquireRuntimeTargetSelection(
					workspace, "target-selection-outside-profile", "deliverable", runtimeFileTargetSelectorProfileCandidates, "outside-profile.txt", 4, binding,
				)
				if !errors.Is(acquireErr, common.ErrInvalidArgument) {
					t.Fatalf("outside-profile prospective error = %v, want invalid argument", acquireErr)
				}
			}
			after, err := NewRuntimeAdapter().AcquireWorkspaceManifest(workspace)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before, after) {
				t.Fatalf("outside-profile prospective admission mutated manifest: before=%+v after=%+v", before, after)
			}
		})
	}
}

func TestRuntimeAdapterExistingProspectiveMustMatchCurrentInventory(t *testing.T) {
	t.Run("workspace deliverable complete scope", func(t *testing.T) {
		workspace := t.TempDir()
		for _, targetRef := range []string{"current-a.txt", "current-b.txt"} {
			if err := os.WriteFile(filepath.Join(workspace, targetRef), []byte(targetRef+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		selection, err := NewRuntimeAdapter().AcquireRuntimeTargetSelection(
			workspace, "target-selection-existing-scope", "deliverable", runtimeFileTargetSelectorWorkspaceDeliverable, "current-a.txt", 4,
		)
		if err != nil {
			t.Fatal(err)
		}
		if selection.Disposition != runtimecontract.RuntimeTargetSelectionScope || selection.ProspectiveTargetRef != "current-a.txt" ||
			selection.CandidateCount != 2 || !slices.Equal(selection.TargetRefs, []string{"current-a.txt", "current-b.txt"}) {
			t.Fatalf("existing workspace prospective selection = %+v", selection)
		}
	})
	t.Run("profile declared exact current target", func(t *testing.T) {
		workspace := t.TempDir()
		for _, targetRef := range []string{"current-a.txt", "current-b.txt"} {
			if err := os.WriteFile(filepath.Join(workspace, targetRef), []byte(targetRef+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		selection, err := NewRuntimeAdapter().AcquireRuntimeTargetSelection(
			workspace, "target-selection-profile-existing", "deliverable", runtimeFileTargetSelectorProfileCandidates, "current-a.txt", 4,
			runtimeTargetSelectionBinding{ownerRef: "profile-audit", subjectRef: "deliverable", candidateRefs: []string{"current-a.txt", "current-b.txt"}},
		)
		if err != nil {
			t.Fatal(err)
		}
		if selection.Disposition != runtimecontract.RuntimeTargetSelectionAmbiguous || selection.ProspectiveTargetRef != "current-a.txt" ||
			selection.CandidateCount != 2 || !slices.Equal(selection.CandidateTargetRefs, []string{"current-a.txt", "current-b.txt"}) {
			t.Fatalf("existing profile prospective selection = %+v", selection)
		}
	})
}

func TestRuntimeAdapterAcquireProfileDeclaredTargetSelectionOnlyIntersectsCandidates(t *testing.T) {
	adapter := NewRuntimeAdapter()
	binding := runtimeTargetSelectionBinding{ownerRef: "profile-lsp", subjectRef: "implementation", candidateRefs: []string{"internal/lsp/manager.go", "internal/lsp/server.go"}}
	t.Run("one declared file ignores unrelated workspace files", func(t *testing.T) {
		workspace := t.TempDir()
		for name := range map[string]string{"README.md": "unrelated", "internal/lsp/manager.go": "package lsp"} {
			path := filepath.Join(workspace, name)
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(name), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		selection, err := adapter.AcquireRuntimeTargetSelection(workspace, "target-selection-profile-one", "implementation", runtimeFileTargetSelectorProfileCandidates, "", 4, binding)
		if err != nil {
			t.Fatal(err)
		}
		if selection.Disposition != runtimecontract.RuntimeTargetSelectionSelected || len(selection.TargetRefs) != 1 || selection.TargetRefs[0] != "internal/lsp/manager.go" || selection.SelectorOwnerRef != "profile-lsp" || selection.SelectorSubjectRef != "implementation" || !runtimecontract.SameRuntimeTargetSelectorCandidateRefs(selection.SelectorCandidateRefs, binding.candidateRefs) {
			t.Fatalf("profile declared selection = %+v", selection)
		}
	})
	t.Run("multiple declared files remain ambiguous", func(t *testing.T) {
		workspace := t.TempDir()
		for _, name := range binding.candidateRefs {
			path := filepath.Join(workspace, name)
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(name), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		selection, err := adapter.AcquireRuntimeTargetSelection(workspace, "target-selection-profile-many", "implementation", runtimeFileTargetSelectorProfileCandidates, "", 4, binding)
		if err != nil {
			t.Fatal(err)
		}
		if selection.Disposition != runtimecontract.RuntimeTargetSelectionAmbiguous || len(selection.TargetRefs) != 0 || selection.CandidateCount != 2 || len(selection.CandidateTargetRefs) != 2 {
			t.Fatalf("profile declared ambiguous selection = %+v", selection)
		}
	})
	t.Run("declared absent prospective file is create scope", func(t *testing.T) {
		workspace := t.TempDir()
		createBinding := runtimeTargetSelectionBinding{ownerRef: "profile-lsp", subjectRef: "implementation", candidateRefs: []string{"internal/lsp/manager.go"}}
		selection, err := adapter.AcquireRuntimeTargetSelection(workspace, "target-selection-profile-create", "implementation", runtimeFileTargetSelectorProfileCandidates, "internal/lsp/manager.go", 4, createBinding)
		if err != nil {
			t.Fatal(err)
		}
		if selection.Disposition != runtimecontract.RuntimeTargetSelectionCreate || selection.ProspectiveTargetRef != "internal/lsp/manager.go" || selection.CandidateCount != 0 {
			t.Fatalf("profile declared create selection = %+v", selection)
		}
	})
}

func TestFileMutationReceiptObservationReportsAfterRevisionForAllStateChangingHandlers(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "target.txt"), []byte("after\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// File edit, file write and patch apply all call this owner-local receipt
	// constructor. Keep the variants explicit here so a future handler cannot
	// bypass the required owner-observed target/tree revisions.
	for _, variant := range []string{"file_edit", "file_write", "patch_apply"} {
		t.Run(variant, func(t *testing.T) {
			observation := fileMutationReceiptObservation(workspace, []string{"target.txt"}, runtimecontract.RuntimeOperationExecutionStatusAccepted, RuntimeFileOperationResult{
				BeforeHash: "before-" + variant, AfterHash: "after-" + variant,
			}, "workspace-tree-before-"+variant)
			if observation.ObservedEffectScope != runtimecontract.RuntimeEffectWorkspaceDurable || !observation.Changed || observation.NoOp {
				t.Fatalf("%s receipt observation state = %+v", variant, observation)
			}
			var target, tree *runtimecontract.RuntimeResourceChange
			for idx := range observation.ResourceChanges {
				change := observation.ResourceChanges[idx]
				switch {
				case change.Domain == runtimecontract.RuntimeResourceWorkspaceTarget && change.SubjectRef == "target.txt":
					target = &change
				case change.Domain == runtimecontract.RuntimeResourceWorkspaceTree && change.SubjectRef == "workspace_deliverable":
					tree = &change
				}
			}
			if target == nil || target.OwnerModuleID != ModuleID || target.BeforeRevision != "before-"+variant || target.AfterRevision != "after-"+variant {
				t.Fatalf("%s target resource change = %+v", variant, observation.ResourceChanges)
			}
			if tree == nil || tree.OwnerModuleID != ModuleID || tree.BeforeRevision != "workspace-tree-before-"+variant || tree.AfterRevision == "" {
				t.Fatalf("%s workspace tree resource change = %+v", variant, observation.ResourceChanges)
			}
		})
	}
	for _, testCase := range []struct {
		name   string
		status runtimecontract.RuntimeOperationExecutionStatus
		result RuntimeFileOperationResult
	}{
		{name: "accepted_no_op", status: runtimecontract.RuntimeOperationExecutionStatusAccepted, result: RuntimeFileOperationResult{BeforeHash: "same", AfterHash: "same"}},
		{name: "rejected", status: runtimecontract.RuntimeOperationExecutionStatusRejected, result: RuntimeFileOperationResult{BeforeHash: "before", AfterHash: "after"}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			observation := fileMutationReceiptObservation(workspace, []string{"target.txt"}, testCase.status, testCase.result)
			if len(observation.ResourceChanges) != 0 || observation.Changed {
				t.Fatalf("%s fabricated durable resource changes: %+v", testCase.name, observation)
			}
		})
	}
}

func TestRuntimeAdapterProbeRejectsStaleAdmissibilitySnapshotWithoutMutation(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "main.go")
	if err := os.WriteFile(path, []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	adapter := NewRuntimeAdapter()
	inventory, err := adapter.AcquireRuntimeOperationAdmissibility(workspace, "admissibility-file", []string{runtimecontract.StepKindFileEdit}, []string{"main.go"}, 4)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("package main\n// changed after inventory\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	oldText, newText := "package main\n", "package app\n"
	failure := adapter.ProbeRuntimeOperationFeasibility(workspace, inventory, runtimecontract.RuntimeOperation{
		ID: "stale-edit", Kind: runtimecontract.StepKindFileEdit, TargetRefs: []string{"main.go"},
		Parameters: []runtimecontract.OperationParameter{
			{Name: "oldText", String: &oldText},
			{Name: "newText", String: &newText},
		},
	})
	if failure == nil || failure.Code != "operation_admissibility_stale" {
		t.Fatalf("stale inventory feasibility failure = %+v", failure)
	}
	content, err := os.ReadFile(path)
	if err != nil || string(content) != "package main\n// changed after inventory\n" {
		t.Fatalf("probe mutated workspace content=%q err=%v", content, err)
	}
}

func TestRuntimeAdapterAdmissibilityInventoryPrioritizesFocusTargetsWithinBound(t *testing.T) {
	workspace := t.TempDir()
	for _, name := range []string{"a.txt", "b.txt", "focus.txt"} {
		if err := os.WriteFile(filepath.Join(workspace, name), []byte(name+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	inventory, err := NewRuntimeAdapter().AcquireRuntimeOperationAdmissibility(
		workspace, "admissibility-file", []string{runtimecontract.StepKindFileEdit}, []string{"focus.txt"}, 2,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(inventory.TargetRefs) != 2 || inventory.TargetRefs[0] != "focus.txt" || inventory.TargetRefs[1] != "a.txt" {
		t.Fatalf("bounded inventory = %#v, want focus target first then canonical manifest entry", inventory.TargetRefs)
	}
}

func TestRuntimeAdapterAdmissibilityPublishesCreateOnlyWriteAndRevisionBoundReplacement(t *testing.T) {
	workspace := t.TempDir()
	const targetRef = "existing.go"
	if err := os.WriteFile(filepath.Join(workspace, targetRef), []byte("package existing\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	inventory, err := NewRuntimeAdapter().AcquireRuntimeOperationAdmissibility(
		workspace,
		"admissibility-file",
		[]string{runtimecontract.StepKindFileWrite, runtimecontract.StepKindFileEdit, runtimecontract.RuntimeBlockKindPatchApply},
		[]string{targetRef},
		4,
	)
	if err != nil {
		t.Fatal(err)
	}
	write, found, err := runtimecontract.RuntimeOperationAdmissibilityVariantForProposal(inventory, runtimecontract.StepKindFileWrite, "")
	if err != nil || !found {
		t.Fatalf("file/write owner fact = %+v found=%t err=%v", write, found, err)
	}
	if len(write.AllowedTargetPolicies) != 1 || write.AllowedTargetPolicies[0] != runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget {
		t.Fatalf("file/write owner fact is not create-only: %+v", write)
	}
	edit, found, err := runtimecontract.RuntimeOperationAdmissibilityVariantForProposal(inventory, runtimecontract.StepKindFileEdit, "")
	if err != nil || !found || !containsString(edit.SelectorRefs, runtimeFileSelectorWorkspaceRelativeExistingTarget) ||
		!containsString(inventory.SelectorRefs, runtimeFileSelectorWorkspaceRelativeExistingTarget) {
		t.Fatalf("file/edit owner fact lacks existing-target authorization: edit=%+v inventory=%+v err=%v", edit, inventory, err)
	}
	for _, policy := range []runtimecontract.RuntimeOperationTargetPolicy{
		runtimecontract.RuntimeOperationTargetPolicyExactTarget,
		runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope,
	} {
		allowed, _, _, _, err := runtimecontract.RuntimeOperationAdmissibilityAllowsTargetPolicy(inventory, runtimecontract.StepKindFileWrite, "", policy)
		if err != nil || allowed {
			t.Fatalf("file/write policy %q allowed=%t err=%v", policy, allowed, err)
		}
	}
	allowed, _, _, found, err := runtimecontract.RuntimeOperationAdmissibilityAllowsTargetPolicy(
		inventory, runtimecontract.RuntimeBlockKindPatchApply, patchTypeTextReplace, runtimecontract.RuntimeOperationTargetPolicyExactTarget,
	)
	if err != nil || !found || !allowed {
		t.Fatalf("patch/apply text_replace replacement fact allowed=%t found=%t err=%v inventory=%+v", allowed, found, err, inventory)
	}
}

func TestRuntimeAdapterProbeAllowsSelectedNewFileWriteTargetWithoutMutation(t *testing.T) {
	workspace := t.TempDir()
	adapter := NewRuntimeAdapter()
	inventory, err := adapter.AcquireRuntimeOperationAdmissibility(
		workspace, "admissibility-file", []string{runtimecontract.RuntimeBlockKindPatchApply}, nil, 4,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(inventory.TargetRefs) != 0 || len(inventory.SelectorRefs) != 1 || inventory.SelectorRefs[0] != runtimeFileSelectorWorkspaceRelativeNewTarget {
		t.Fatalf("empty-workspace inventory = %+v, want bounded new-target selector", inventory)
	}
	patchType, content, target := "file_write", "package main\n", "main.go"
	failure := adapter.ProbeRuntimeOperationFeasibility(workspace, inventory, runtimecontract.RuntimeOperation{
		ID: "create-main", Kind: runtimecontract.RuntimeBlockKindPatchApply, TargetRefs: []string{target},
		Parameters: []runtimecontract.OperationParameter{
			{Name: "patchType", String: &patchType},
			{Name: "content", String: &content},
		},
	})
	if failure != nil {
		t.Fatalf("new file_write target rejected: %+v", failure)
	}
	if _, err := os.Stat(filepath.Join(workspace, target)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("feasibility probe created target: %v", err)
	}
}

func TestRuntimeAdapterProbeReportsTypedStaleAfterExternalWorkspaceMutation(t *testing.T) {
	workspace := t.TempDir()
	target := filepath.Join(workspace, "main.go")
	if err := os.WriteFile(target, []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	adapter := NewRuntimeAdapter()
	inventory, err := adapter.AcquireRuntimeOperationAdmissibility(
		workspace, "admissibility-file", []string{runtimecontract.StepKindFileEdit}, []string{"main.go"}, 4,
	)
	if err != nil {
		t.Fatal(err)
	}
	observation := fileAdmissibilityObservation("admissibility-file", inventory, []string{"main.go"}, nil)
	if err := os.WriteFile(target, []byte("package main\n// externally changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	path := "main.go"
	failure, stale := adapter.ProbeRuntimeOperationFeasibilityWithObservation(workspace, inventory, runtimecontract.RuntimeOperation{
		ID: "edit-main", Kind: runtimecontract.StepKindFileEdit, TargetRefs: []string{path},
	}, &observation)
	if failure != nil || stale == nil {
		t.Fatalf("external mutation probe failure/stale=%+v/%+v, want typed stale", failure, stale)
	}
	if stale.PrerequisiteID != observation.PrerequisiteID || stale.ObservationID != observation.ObservationID ||
		stale.Domain != runtimecontract.RuntimeResourceWorkspaceTree || stale.OwnerModuleID != ModuleID ||
		stale.ExpectedRevision != inventory.ObservedRevision || stale.ObservedRevision == inventory.ObservedRevision {
		t.Fatalf("stale = %+v, want exact file-owned observation drift", stale)
	}
	content, err := os.ReadFile(target)
	if err != nil || string(content) != "package main\n// externally changed\n" {
		t.Fatalf("probe mutated externally changed target content=%q err=%v", content, err)
	}
}

func TestRuntimeAdapterProbeRejectsNewTargetWithoutOwnerSelector(t *testing.T) {
	workspace := t.TempDir()
	adapter := NewRuntimeAdapter()
	inventory, err := adapter.AcquireRuntimeOperationAdmissibility(
		workspace, "admissibility-file", []string{runtimecontract.RuntimeBlockKindPatchApply}, nil, 4,
	)
	if err != nil {
		t.Fatal(err)
	}
	inventory.SelectorRefs = nil
	inventory.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(inventory)
	patchType, content, target := "file_write", "package main\n", "main.go"
	failure := adapter.ProbeRuntimeOperationFeasibility(workspace, inventory, runtimecontract.RuntimeOperation{
		ID: "create-main", Kind: runtimecontract.RuntimeBlockKindPatchApply, TargetRefs: []string{target},
		Parameters: []runtimecontract.OperationParameter{
			{Name: "patchType", String: &patchType},
			{Name: "content", String: &content},
		},
	})
	if failure == nil || failure.Code != "operation_target_not_admissible" {
		t.Fatalf("new target without selector failure = %+v", failure)
	}
}

func TestRuntimeAdapterProbeNewTargetSelectorRejectsWorkspaceEscape(t *testing.T) {
	workspace := t.TempDir()
	adapter := NewRuntimeAdapter()
	inventory, err := adapter.AcquireRuntimeOperationAdmissibility(
		workspace, "admissibility-file", []string{runtimecontract.RuntimeBlockKindPatchApply}, nil, 4,
	)
	if err != nil {
		t.Fatal(err)
	}
	patchType, content, target := "file_write", "outside\n", "../outside.txt"
	failure := adapter.ProbeRuntimeOperationFeasibility(workspace, inventory, runtimecontract.RuntimeOperation{
		ID: "escape-workspace", Kind: runtimecontract.RuntimeBlockKindPatchApply, TargetRefs: []string{target},
		Parameters: []runtimecontract.OperationParameter{
			{Name: "patchType", String: &patchType},
			{Name: "content", String: &content},
		},
	})
	if failure == nil || failure.Code != "operation_target_invalid" {
		t.Fatalf("workspace escape failure = %+v", failure)
	}
}

func TestRuntimeAdapterProbeNewTargetSelectorDoesNotAuthorizeExistingUnlistedTarget(t *testing.T) {
	workspace := t.TempDir()
	for _, name := range []string{"a.txt", "b.txt"} {
		if err := os.WriteFile(filepath.Join(workspace, name), []byte(name+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	adapter := NewRuntimeAdapter()
	inventory, err := adapter.AcquireRuntimeOperationAdmissibility(
		workspace, "admissibility-file", []string{runtimecontract.RuntimeBlockKindPatchApply}, nil, 1,
	)
	if err != nil {
		t.Fatal(err)
	}
	patchType, content, target := "file_write", "changed\n", "b.txt"
	failure := adapter.ProbeRuntimeOperationFeasibility(workspace, inventory, runtimecontract.RuntimeOperation{
		ID: "overwrite-unlisted", Kind: runtimecontract.RuntimeBlockKindPatchApply, TargetRefs: []string{target},
		Parameters: []runtimecontract.OperationParameter{
			{Name: "patchType", String: &patchType},
			{Name: "content", String: &content},
		},
	})
	if failure == nil || failure.Code != "operation_target_not_admissible" {
		t.Fatalf("existing unlisted target failure = %+v", failure)
	}
	current, err := os.ReadFile(filepath.Join(workspace, target))
	if err != nil || string(current) != "b.txt\n" {
		t.Fatalf("probe mutated existing target content=%q err=%v", current, err)
	}
}

func TestRuntimeAdapterProbeRejectsMissingTextReplaceDespiteNewTargetSelector(t *testing.T) {
	workspace := t.TempDir()
	adapter := NewRuntimeAdapter()
	inventory, err := adapter.AcquireRuntimeOperationAdmissibility(
		workspace, "admissibility-file", []string{runtimecontract.RuntimeBlockKindPatchApply}, nil, 4,
	)
	if err != nil {
		t.Fatal(err)
	}
	patchType, oldText, newText, target := "text_replace", "old", "new", "main.go"
	failure := adapter.ProbeRuntimeOperationFeasibility(workspace, inventory, runtimecontract.RuntimeOperation{
		ID: "edit-missing", Kind: runtimecontract.RuntimeBlockKindPatchApply, TargetRefs: []string{target},
		Parameters: []runtimecontract.OperationParameter{
			{Name: "patchType", String: &patchType},
			{Name: "oldText", String: &oldText},
			{Name: "newText", String: &newText},
		},
	})
	if failure == nil || failure.Code != "operation_target_missing" {
		t.Fatalf("missing text_replace target failure = %+v", failure)
	}
}

func stringPointer(value string) *string { return &value }

func TestRuntimeAdapterReadReturnsContentAndHash(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	result, err := NewRuntimeAdapter().ReadFile(context.Background(), FileReadRequest{
		WorkspaceDirectory: workspace,
		Path:               "main.go",
		StartLine:          1,
		EndLine:            1,
	})
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if result.Content != "package main\n" {
		t.Fatalf("content = %q, want first line", result.Content)
	}
	if result.ContentHash == "" || result.Binary || result.TargetRef != "main.go" ||
		result.ContentComplete || result.SafeForPatch || result.SafeForFileWrite {
		t.Fatalf("read metadata = %+v, want partial read marked unsafe for exact patch", result)
	}
}

func TestRuntimeAdapterReadMarksCompleteFullTextSafeForPatch(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	result, err := NewRuntimeAdapter().ReadFile(context.Background(), FileReadRequest{
		WorkspaceDirectory: workspace,
		Path:               "main.go",
	})
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if !result.ContentComplete || !result.SafeForPatch || !result.SafeForFileWrite {
		t.Fatalf("read metadata = %+v, want complete safe target snapshot metadata", result)
	}
}

func TestRuntimeAdapterReadMarksRedactedAndTruncatedContentUnsafeForPatch(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "secret.txt"), []byte("token=[REDACTED]\n"), 0o644); err != nil {
		t.Fatalf("seed secret file: %v", err)
	}
	redacted, err := NewRuntimeAdapter().ReadFile(context.Background(), FileReadRequest{
		WorkspaceDirectory: workspace,
		Path:               "secret.txt",
	})
	if err != nil {
		t.Fatalf("read redacted file: %v", err)
	}
	if !redacted.Redacted || redacted.ContentComplete || redacted.SafeForPatch || redacted.SafeForFileWrite {
		t.Fatalf("redacted output = %+v, want unsafe target snapshot", redacted)
	}

	if err := os.WriteFile(filepath.Join(workspace, "large.txt"), []byte(strings.Repeat("x", defaultMaxReadBytes+1)), 0o644); err != nil {
		t.Fatalf("seed large file: %v", err)
	}
	truncated, err := NewRuntimeAdapter().ReadFile(context.Background(), FileReadRequest{
		WorkspaceDirectory: workspace,
		Path:               "large.txt",
	})
	if err != nil {
		t.Fatalf("read large file: %v", err)
	}
	if !truncated.Truncated || truncated.ContentComplete || truncated.SafeForPatch || truncated.SafeForFileWrite {
		t.Fatalf("truncated output = %+v, want unsafe target snapshot", truncated)
	}
}

func TestRuntimeAdapterReadStreamsWholeHashWithinBoundedContent(t *testing.T) {
	workspace := t.TempDir()
	content := strings.Repeat("line\n", 20_000)
	if err := os.WriteFile(filepath.Join(workspace, "large.txt"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := NewRuntimeAdapter().ReadFile(context.Background(), FileReadRequest{
		WorkspaceDirectory: workspace, Path: "large.txt", MaxBytes: 32,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Content) != 32 || !result.Truncated || result.ContentHash != sha256Hex([]byte(content)) || result.Size != int64(len(content)) || result.LineCount != 20_000 {
		t.Fatalf("bounded streaming read = %+v", result)
	}

	ranged, err := NewRuntimeAdapter().ReadFile(context.Background(), FileReadRequest{
		WorkspaceDirectory: workspace, Path: "large.txt", MaxBytes: 32, StartLine: 19_999, EndLine: 19_999,
	})
	if err != nil {
		t.Fatal(err)
	}
	if ranged.Content != "line\n" || ranged.Truncated || ranged.ContentHash != result.ContentHash || ranged.ContentComplete {
		t.Fatalf("late bounded range = %+v", ranged)
	}
}

func TestRuntimeAdapterReadDetectsBinaryBeyondReturnedPrefix(t *testing.T) {
	workspace := t.TempDir()
	content := append([]byte(strings.Repeat("a", defaultMaxReadBytes+1)), 0xff)
	if err := os.WriteFile(filepath.Join(workspace, "large.bin"), content, 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := NewRuntimeAdapter().ReadFile(context.Background(), FileReadRequest{
		WorkspaceDirectory: workspace, Path: "large.bin", MaxBytes: 16,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Binary || !result.Truncated || result.Content != "" || result.ContentHash != sha256Hex(content) {
		t.Fatalf("binary streaming read = %+v", result)
	}
}

func TestFileExecuteHandlerRejectsNilResultBeforePrivateEffect(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(8)
	t.Cleanup(func() { hub.Terminate(ctx) })
	owner, err := New(hub, nil)
	if err != nil {
		t.Fatal(err)
	}
	receiptObserver := magicevent.NewSimpleObserver(orchestratorevents.CommandDestination, hub)
	receiptObserver.Subscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(orchestratorevents.QueryRuntimeOperationReceiptResult{}, nil)
	})
	receiptObserver.Subscribe(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(orchestratorevents.CommitRuntimeOperationReceiptCommand)
		result.Set(orchestratorevents.CommitRuntimeOperationReceiptResult{Receipt: command.Receipt}, nil)
	})
	t.Cleanup(func() {
		receiptObserver.Unsubscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt)
		receiptObserver.Unsubscribe(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt)
	})
	workspace := t.TempDir()
	target := filepath.Join(workspace, "main.txt")
	if err := os.WriteFile(target, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	oldText, newText := "old", "new"
	operation := runtimecontract.RuntimeOperation{
		ID: "edit-1", Kind: runtimecontract.StepKindFileEdit, TargetRefs: []string{"main.txt"},
		Parameters: []runtimecontract.OperationParameter{{Name: "oldText", String: &oldText}, {Name: "newText", String: &newText}},
	}
	sealed, err := fileevents.SealRuntimeOperation(operation)
	if err != nil {
		t.Fatal(err)
	}
	command := fileevents.ExecuteFileEditOperationCommand{
		RuntimeOperationExecutionContext: runtimecontract.RuntimeOperationExecutionContext{
			Execution: fileBizTestExecutionEnvelope(operation, runtimecontract.RuntimeOperationExecutionEnvelope{
				ExecutionKey: "exec/ws-1/run-1/dispatch-1/edit-1", Fence: 1, ExpectedEffectScope: runtimecontract.RuntimeEffectWorkspaceDurable,
			}),
			WorkspaceID: "ws-1", RunID: "run-1", DispatchID: "dispatch-1", Transaction: runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "authority-1"},
			OperationID: "edit-1", TargetRefs: []string{"main.txt"}, EvidenceKind: runtimecontract.EvidenceKindStateChange, WorkspaceDeliverableDirectory: workspace,
		},
		SealedOperation: sealed,
	}
	event := magicevent.NewEventWithContext(fileevents.TopicExecuteFileEditOperation, runtimecontract.RuntimeModuleOrchestrator, ModuleID, magicevent.NewHeader(), ctx, command)
	owner.handleFileEditOperation(event, nil)
	content, err := os.ReadFile(target)
	if err != nil || string(content) != "old\n" {
		t.Fatalf("nil-result strong call changed content=%q err=%v", content, err)
	}
}

func TestFileReceiptBoundaryRejectsMalformedCrossOwnerResults(t *testing.T) {
	operationHash := runtimecontract.StableRuntimeOperationHash(runtimecontract.RuntimeOperation{ID: "op-1", Kind: runtimecontract.StepKindFileRead, TargetRefs: []string{"target.txt"}})
	execution := runtimecontract.RuntimeOperationExecutionEnvelope{
		ExecutionKey: "exec/ws-1/run-1/dispatch-1/op-1", Fence: 2, ExpectedEffectScope: runtimecontract.RuntimeEffectNone,
		OperationHash: operationHash, IdempotencyClass: runtimecontract.RuntimeOperationIdempotencySafeToReplay,
	}
	execution.AuthorityHash = runtimecontract.StableRuntimeOperationExecutionAuthorityHash(runtimecontract.RuntimeOperationExecutionRecord{
		OperationID: "op-1", OperationHash: execution.OperationHash, ExecutionKey: execution.ExecutionKey, ExpectedEffectScope: execution.ExpectedEffectScope,
		IdempotencyClass: execution.IdempotencyClass,
	})
	valid := runtimecontract.RuntimeOperationReceipt{
		ExecutionKey: execution.ExecutionKey, Fence: execution.Fence, OperationID: "op-1",
		Outcome: runtimecontract.RuntimeOperationOutcomeNoOp, ObservedEffectScope: runtimecontract.RuntimeEffectNone,
		Evidence: runtimecontract.RuntimeOperationEvidence{Summary: "no change"},
	}
	ownerError := &cd.Error{Code: cd.Unexpected, Message: "injected owner failure", Cause: errors.New("injected owner failure")}

	t.Run("query", func(t *testing.T) {
		wrongKey := valid
		wrongKey.ExecutionKey = "exec/other"
		wrongFence := valid
		wrongFence.Fence++
		wrongOperation := valid
		wrongOperation.OperationID = "op-other"
		invalid := valid
		invalid.Outcome = "invalid"
		for _, test := range []struct {
			name      string
			data      any
			ownerErr  *cd.Error
			wantFound bool
			wantErr   bool
		}{
			{name: "authorized absent", data: orchestratorevents.QueryRuntimeOperationReceiptResult{}},
			{name: "exact", data: orchestratorevents.QueryRuntimeOperationReceiptResult{Found: true, Receipt: &valid}, wantFound: true},
			{name: "wrong result type", data: "wrong", wantErr: true},
			{name: "owner error", ownerErr: ownerError, wantErr: true},
			{name: "found without receipt", data: orchestratorevents.QueryRuntimeOperationReceiptResult{Found: true}, wantErr: true},
			{name: "receipt while absent", data: orchestratorevents.QueryRuntimeOperationReceiptResult{Receipt: &valid}, wantErr: true},
			{name: "invalid receipt", data: orchestratorevents.QueryRuntimeOperationReceiptResult{Found: true, Receipt: &invalid}, wantErr: true},
			{name: "wrong execution key", data: orchestratorevents.QueryRuntimeOperationReceiptResult{Found: true, Receipt: &wrongKey}, wantErr: true},
			{name: "wrong fence", data: orchestratorevents.QueryRuntimeOperationReceiptResult{Found: true, Receipt: &wrongFence}, wantErr: true},
			{name: "wrong operation", data: orchestratorevents.QueryRuntimeOperationReceiptResult{Found: true, Receipt: &wrongOperation}, wantErr: true},
		} {
			t.Run(test.name, func(t *testing.T) {
				ctx := context.Background()
				hub := magicevent.NewHub(8)
				t.Cleanup(func() { hub.Terminate(ctx) })
				owner, err := New(hub, nil)
				if err != nil {
					t.Fatal(err)
				}
				observer := magicevent.NewSimpleObserver(orchestratorevents.CommandDestination, hub)
				observer.Subscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt, func(_ magicevent.Event, result magicevent.Result) {
					result.Set(test.data, test.ownerErr)
				})
				t.Cleanup(func() { observer.Unsubscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt) })

				receipt, found, err := owner.queryRuntimeOperationReceipt(ctx, "ws-1", "run-1", "dispatch-1", valid.OperationID, execution)
				if test.wantErr {
					if err == nil || found {
						t.Fatalf("found=%v receipt=%+v err=%v, want fail closed", found, receipt, err)
					}
					return
				}
				if err != nil || found != test.wantFound {
					t.Fatalf("found=%v receipt=%+v err=%v", found, receipt, err)
				}
			})
		}
	})

	t.Run("commit", func(t *testing.T) {
		for _, test := range []struct {
			name     string
			response func(runtimecontract.RuntimeOperationReceipt) (any, *cd.Error)
			wantErr  bool
		}{
			{name: "exact", response: func(receipt runtimecontract.RuntimeOperationReceipt) (any, *cd.Error) {
				return orchestratorevents.CommitRuntimeOperationReceiptResult{Receipt: receipt, StateRevision: 7}, nil
			}},
			{name: "wrong result type", response: func(runtimecontract.RuntimeOperationReceipt) (any, *cd.Error) { return "wrong", nil }, wantErr: true},
			{name: "owner error", response: func(runtimecontract.RuntimeOperationReceipt) (any, *cd.Error) { return nil, ownerError }, wantErr: true},
			{name: "missing receipt", response: func(runtimecontract.RuntimeOperationReceipt) (any, *cd.Error) {
				return orchestratorevents.CommitRuntimeOperationReceiptResult{}, nil
			}, wantErr: true},
			{name: "wrong identity", response: func(receipt runtimecontract.RuntimeOperationReceipt) (any, *cd.Error) {
				receipt.ExecutionKey = "exec/other"
				return orchestratorevents.CommitRuntimeOperationReceiptResult{Receipt: receipt}, nil
			}, wantErr: true},
			{name: "payload drift", response: func(receipt runtimecontract.RuntimeOperationReceipt) (any, *cd.Error) {
				receipt.Evidence.Summary = "different"
				return orchestratorevents.CommitRuntimeOperationReceiptResult{Receipt: receipt}, nil
			}, wantErr: true},
		} {
			t.Run(test.name, func(t *testing.T) {
				ctx := context.Background()
				hub := magicevent.NewHub(8)
				t.Cleanup(func() { hub.Terminate(ctx) })
				owner, err := New(hub, nil)
				if err != nil {
					t.Fatal(err)
				}
				observer := magicevent.NewSimpleObserver(orchestratorevents.CommandDestination, hub)
				observer.Subscribe(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt, func(event magicevent.Event, result magicevent.Result) {
					command := event.Data().(orchestratorevents.CommitRuntimeOperationReceiptCommand)
					data, responseErr := test.response(command.Receipt)
					result.Set(data, responseErr)
				})
				t.Cleanup(func() { observer.Unsubscribe(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt) })

				receipt, err := owner.commitRuntimeOperationReceipt(ctx, "ws-1", "run-1", "authority-1", "dispatch-1", execution, valid)
				if test.wantErr {
					if err == nil {
						t.Fatalf("receipt=%+v err=nil, want fail closed", receipt)
					}
					return
				}
				if err != nil || !runtimecontract.SameRuntimeOperationReceipt(receipt, valid) {
					t.Fatalf("receipt=%+v err=%v", receipt, err)
				}
			})
		}
	})
}

func TestFileApprovalGrantMustMatchExactTargetAndAction(t *testing.T) {
	requirement := fileApprovalRequirement(".env", "sensitive target", runtimecontract.StepKindFileEdit)
	grant := runtimecontract.RuntimeOperationGovernanceGrant{
		ResolutionID: "resolution-1", ResolutionHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ApprovalID: "approval-1", ExecutionKey: "execution-1",
		GovernanceFence: 1, ResumeFence: 2, ResumeDispatchID: "dispatch-1", DependencyKind: string(runtimecontract.GovernanceApproval),
		SubjectRef: ".env", RequestedAction: runtimecontract.StepKindFileEdit, RiskLevel: "high",
		AuthorizationHash: runtimecontract.StableRuntimeAuthorizationContextHash(requirement.AuthorizationContext), GovernanceReceiptHash: "receipt-1",
	}
	execution := runtimecontract.RuntimeOperationExecutionEnvelope{ExecutionKey: grant.ExecutionKey, Fence: grant.ResumeFence, ExpectedEffectScope: runtimecontract.RuntimeEffectWorkspaceDurable, GovernanceGrant: &grant}
	if !fileApprovalGranted(execution, ".env", runtimecontract.StepKindFileEdit) {
		t.Fatal("exact File approval grant was rejected")
	}
	for _, mutate := range []func(*runtimecontract.RuntimeOperationGovernanceGrant){
		func(value *runtimecontract.RuntimeOperationGovernanceGrant) {
			value.SubjectRef = ".github/workflows/ci.yml"
		},
		func(value *runtimecontract.RuntimeOperationGovernanceGrant) {
			value.RequestedAction = runtimecontract.RuntimeBlockKindPatchApply
		},
		func(value *runtimecontract.RuntimeOperationGovernanceGrant) { value.AuthorizationHash = "other" },
	} {
		drifted := grant
		mutate(&drifted)
		execution.GovernanceGrant = &drifted
		if fileApprovalGranted(execution, ".env", runtimecontract.StepKindFileEdit) {
			t.Fatalf("drifted File approval grant was accepted: %+v", drifted)
		}
	}
}

func TestRuntimeAdapterOptionalReadMissingFileCompletes(t *testing.T) {
	workspace := t.TempDir()

	result, err := NewRuntimeAdapter().ReadFile(context.Background(), FileReadRequest{
		WorkspaceDirectory: workspace,
		Path:               "README.md",
		Optional:           true,
	})
	if err != nil {
		t.Fatalf("optional read missing file: %v", err)
	}
	if result.Status != "completed" || result.Exists || result.Path != "README.md" {
		t.Fatalf("output = %+v, want completed exists=false", result)
	}
}

func TestRuntimeAdapterListSkipsGeneratedDirectories(t *testing.T) {
	workspace := t.TempDir()
	if err := os.Mkdir(filepath.Join(workspace, "node_modules"), 0o755); err != nil {
		t.Fatalf("mkdir node_modules: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "node_modules", "ignored.js"), []byte("ignored"), 0o644); err != nil {
		t.Fatalf("seed ignored file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "go.mod"), []byte("module example\n"), 0o644); err != nil {
		t.Fatalf("seed go.mod: %v", err)
	}

	result, err := NewRuntimeAdapter().ListFiles(FileListRequest{
		WorkspaceDirectory: workspace,
		Path:               ".",
		Recursive:          true,
	})
	if err != nil {
		t.Fatalf("list files: %v", err)
	}
	seenGoMod := false
	for _, entry := range result.Entries {
		if entry.Path == "node_modules/ignored.js" {
			t.Fatalf("list included skipped generated file: %+v", result.Entries)
		}
		if entry.Path == "go.mod" {
			seenGoMod = true
		}
	}
	if !seenGoMod {
		t.Fatalf("list entries = %+v, want go.mod", result.Entries)
	}
}

func TestRuntimeAdapterListNonRecursiveSkipsIgnoredDirectoriesWithoutFailing(t *testing.T) {
	workspace := t.TempDir()
	if err := os.Mkdir(filepath.Join(workspace, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspace, ".git", "HEAD"), []byte("ref: refs/heads/main\n"), 0o644); err != nil {
		t.Fatalf("seed .git file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("seed main.go: %v", err)
	}

	result, err := NewRuntimeAdapter().ListFiles(FileListRequest{
		WorkspaceDirectory: workspace,
		Path:               ".",
	})
	if err != nil {
		t.Fatalf("list files: %v", err)
	}
	if len(result.Entries) != 1 || result.Entries[0].Path != "main.go" {
		t.Fatalf("entries = %+v, want only main.go", result.Entries)
	}
}

func TestRuntimeAdapterListAndManifestDistinguishExactBoundFromOverflow(t *testing.T) {
	workspace := t.TempDir()
	for _, excluded := range []string{".git", "node_modules", "vendor", "dist", "build", ".workorch-data"} {
		path := filepath.Join(workspace, excluded)
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(path, "ignored.txt"), []byte("ignored\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for index := range defaultMaxEntries {
		name := fmt.Sprintf("file-%03d.txt", index)
		if err := os.WriteFile(filepath.Join(workspace, name), []byte(name+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	adapter := NewRuntimeAdapter()
	exact, err := adapter.ListFiles(FileListRequest{WorkspaceDirectory: workspace, Path: ".", Recursive: true, MaxEntries: defaultMaxEntries})
	if err != nil {
		t.Fatal(err)
	}
	if exact.Truncated || len(exact.Entries) != defaultMaxEntries {
		t.Fatalf("exact-bound list entries=%d truncated=%t, want %d/false", len(exact.Entries), exact.Truncated, defaultMaxEntries)
	}
	manifest, err := adapter.AcquireWorkspaceManifest(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.TargetRefs) != defaultMaxEntries || manifest.Revision == "" {
		t.Fatalf("exact-bound manifest targets=%d revision=%q", len(manifest.TargetRefs), manifest.Revision)
	}

	if err := os.WriteFile(filepath.Join(workspace, "overflow.txt"), []byte("overflow\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	overflow, err := adapter.ListFiles(FileListRequest{WorkspaceDirectory: workspace, Path: ".", Recursive: true, MaxEntries: defaultMaxEntries})
	if err != nil {
		t.Fatal(err)
	}
	if !overflow.Truncated || len(overflow.Entries) != defaultMaxEntries {
		t.Fatalf("overflow list entries=%d truncated=%t, want %d/true", len(overflow.Entries), overflow.Truncated, defaultMaxEntries)
	}
	if _, err := adapter.AcquireWorkspaceManifest(workspace); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("overflow manifest error = %v, want invalid argument", err)
	}
}

func TestRuntimeAdapterProspectiveAdmissionReservesManifestCapacity(t *testing.T) {
	writeEntries := func(t *testing.T, workspace string, count int) {
		t.Helper()
		for index := range count {
			name := fmt.Sprintf("file-%03d.txt", index)
			if err := os.WriteFile(filepath.Join(workspace, name), []byte(name+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	writeMixedEntries := func(t *testing.T, workspace string, count int) {
		t.Helper()
		if count < 1 {
			t.Fatal("mixed inventory requires at least one entry")
		}
		directory := filepath.Join(workspace, "existing-dir")
		if err := os.Mkdir(directory, 0o755); err != nil {
			t.Fatal(err)
		}
		writeEntries(t, directory, count-1)
	}
	selectors := []struct {
		name     string
		selector string
		binding  runtimeTargetSelectionBinding
	}{
		{name: "workspace-relative-new", selector: runtimeFileSelectorWorkspaceRelativeNewTarget},
		{name: "workspace-deliverable", selector: runtimeFileTargetSelectorWorkspaceDeliverable},
		{name: "profile-declared", selector: runtimeFileTargetSelectorProfileCandidates, binding: runtimeTargetSelectionBinding{
			ownerRef: "profile-capacity", subjectRef: "deliverable", candidateRefs: []string{"future.txt"},
		}},
	}
	for _, selector := range selectors {
		t.Run(selector.name+"/exact-bound", func(t *testing.T) {
			workspace := t.TempDir()
			writeEntries(t, workspace, defaultMaxEntries)
			before, err := NewRuntimeAdapter().AcquireWorkspaceManifest(workspace)
			if err != nil {
				t.Fatal(err)
			}
			selection, err := NewRuntimeAdapter().AcquireRuntimeTargetSelection(
				workspace, "capacity-bound", "deliverable", selector.selector, "future.txt",
				runtimecontract.RuntimeOwnerInventoryItemsMaxCount, selector.binding,
			)
			if err != nil {
				t.Fatal(err)
			}
			if selection.Disposition != runtimecontract.RuntimeTargetSelectionUnsupported || selection.CandidateCount != 0 ||
				len(selection.TargetRefs) != 0 || len(selection.CandidateTargetRefs) != 0 {
				t.Fatalf("exact-bound prospective selection = %+v", selection)
			}
			if _, statErr := os.Lstat(filepath.Join(workspace, "future.txt")); !os.IsNotExist(statErr) {
				t.Fatalf("capacity admission changed prospective target: %v", statErr)
			}
			after, err := NewRuntimeAdapter().AcquireWorkspaceManifest(workspace)
			if err != nil || !reflect.DeepEqual(before, after) {
				t.Fatalf("capacity admission mutated manifest: before=%+v after=%+v err=%v", before, after, err)
			}
		})
	}

	t.Run("workspace-relative-new/one-slot", func(t *testing.T) {
		workspace := t.TempDir()
		writeEntries(t, workspace, defaultMaxEntries-1)
		selection, err := NewRuntimeAdapter().AcquireRuntimeTargetSelection(
			workspace, "capacity-one-slot", "deliverable", runtimeFileSelectorWorkspaceRelativeNewTarget, "future.txt",
			runtimecontract.RuntimeOwnerInventoryItemsMaxCount,
		)
		if err != nil || selection.Disposition != runtimecontract.RuntimeTargetSelectionCreate {
			t.Fatalf("one-slot prospective selection = %+v err=%v", selection, err)
		}
		if err := os.WriteFile(filepath.Join(workspace, "future.txt"), []byte("future\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		manifest, err := NewRuntimeAdapter().AcquireWorkspaceManifest(workspace)
		if err != nil || len(manifest.TargetRefs) != defaultMaxEntries {
			t.Fatalf("one-slot realized manifest targets=%d err=%v", len(manifest.TargetRefs), err)
		}
	})

	t.Run("workspace-relative-new/mixed-entry-bound", func(t *testing.T) {
		workspace := t.TempDir()
		writeMixedEntries(t, workspace, defaultMaxEntries)
		manifest, err := NewRuntimeAdapter().AcquireWorkspaceManifest(workspace)
		if err != nil || len(manifest.TargetRefs) != defaultMaxEntries-1 {
			t.Fatalf("mixed exact-bound manifest targets=%d err=%v", len(manifest.TargetRefs), err)
		}
		selection, err := NewRuntimeAdapter().AcquireRuntimeTargetSelection(
			workspace, "capacity-mixed-bound", "deliverable", runtimeFileSelectorWorkspaceRelativeNewTarget, "future.txt",
			runtimecontract.RuntimeOwnerInventoryItemsMaxCount,
		)
		if err != nil || selection.Disposition != runtimecontract.RuntimeTargetSelectionUnsupported {
			t.Fatalf("mixed exact-bound prospective selection = %+v err=%v", selection, err)
		}
		if _, statErr := os.Lstat(filepath.Join(workspace, "future.txt")); !os.IsNotExist(statErr) {
			t.Fatalf("mixed capacity admission changed prospective target: %v", statErr)
		}
	})

	t.Run("workspace-relative-new/nested-create-reserves-all-entries", func(t *testing.T) {
		for _, testCase := range []struct {
			name            string
			initialEntries  int
			wantDisposition runtimecontract.RuntimeTargetSelectionDisposition
		}{
			{name: "three-slots", initialEntries: defaultMaxEntries - 3, wantDisposition: runtimecontract.RuntimeTargetSelectionCreate},
			{name: "two-slots", initialEntries: defaultMaxEntries - 2, wantDisposition: runtimecontract.RuntimeTargetSelectionUnsupported},
		} {
			t.Run(testCase.name, func(t *testing.T) {
				workspace := t.TempDir()
				writeMixedEntries(t, workspace, testCase.initialEntries)
				selection, err := NewRuntimeAdapter().AcquireRuntimeTargetSelection(
					workspace, "capacity-nested", "deliverable", runtimeFileSelectorWorkspaceRelativeNewTarget, "new-a/new-b/future.txt",
					runtimecontract.RuntimeOwnerInventoryItemsMaxCount,
				)
				if err != nil || selection.Disposition != testCase.wantDisposition {
					t.Fatalf("nested prospective selection = %+v err=%v", selection, err)
				}
				if _, statErr := os.Lstat(filepath.Join(workspace, "new-a")); !os.IsNotExist(statErr) {
					t.Fatalf("nested capacity admission created a parent directory: %v", statErr)
				}
			})
		}
	})
}

func TestRuntimeAdapterCreateEffectRechecksAndSerializesManifestCapacity(t *testing.T) {
	writeEntries := func(t *testing.T, workspace string, count int) {
		t.Helper()
		for index := range count {
			name := fmt.Sprintf("file-%03d.txt", index)
			if err := os.WriteFile(filepath.Join(workspace, name), []byte(name+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}

	t.Run("stale 499 admission cannot create item 501", func(t *testing.T) {
		workspace := t.TempDir()
		writeEntries(t, workspace, defaultMaxEntries-1)
		adapter := NewRuntimeAdapter()
		selection, err := adapter.AcquireRuntimeTargetSelection(
			workspace, "capacity-toctou", "deliverable", runtimeFileSelectorWorkspaceRelativeNewTarget, "future.txt",
			runtimecontract.RuntimeOwnerInventoryItemsMaxCount,
		)
		if err != nil || selection.Disposition != runtimecontract.RuntimeTargetSelectionCreate {
			t.Fatalf("initial selection = %+v err=%v", selection, err)
		}
		if err := os.WriteFile(filepath.Join(workspace, "concurrent.txt"), []byte("concurrent\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		result, err := adapter.ApplyPatch(PatchApplyRequest{
			WorkspaceDirectory: workspace, TargetRef: "future.txt", PatchType: patchTypeFileWrite,
			Content: "future\n", ContentPresent: true, CreateOnly: true,
		})
		if !errors.Is(err, common.ErrConflict) || result.FailureCode != "workspace_manifest_capacity_exhausted" {
			t.Fatalf("stale-capacity effect result=%+v err=%v", result, err)
		}
		if _, statErr := os.Lstat(filepath.Join(workspace, "future.txt")); !os.IsNotExist(statErr) {
			t.Fatalf("stale-capacity effect wrote future target: %v", statErr)
		}
	})

	t.Run("stale nested admission counts a concurrent directory", func(t *testing.T) {
		workspace := t.TempDir()
		writeEntries(t, workspace, defaultMaxEntries-3)
		adapter := NewRuntimeAdapter()
		selection, err := adapter.AcquireRuntimeTargetSelection(
			workspace, "capacity-nested-toctou", "deliverable", runtimeFileSelectorWorkspaceRelativeNewTarget, "new-a/new-b/future.txt",
			runtimecontract.RuntimeOwnerInventoryItemsMaxCount,
		)
		if err != nil || selection.Disposition != runtimecontract.RuntimeTargetSelectionCreate {
			t.Fatalf("initial nested selection = %+v err=%v", selection, err)
		}
		if err := os.Mkdir(filepath.Join(workspace, "concurrent-dir"), 0o755); err != nil {
			t.Fatal(err)
		}
		result, err := adapter.ApplyPatch(PatchApplyRequest{
			WorkspaceDirectory: workspace, TargetRef: "new-a/new-b/future.txt", PatchType: patchTypeFileWrite,
			Content: "future\n", ContentPresent: true, CreateOnly: true,
		})
		if !errors.Is(err, common.ErrConflict) || result.FailureCode != "workspace_manifest_capacity_exhausted" {
			t.Fatalf("stale nested capacity effect result=%+v err=%v", result, err)
		}
		if _, statErr := os.Lstat(filepath.Join(workspace, "new-a")); !os.IsNotExist(statErr) {
			t.Fatalf("stale nested capacity effect created a parent directory: %v", statErr)
		}
	})

	t.Run("concurrent creates retain one final slot", func(t *testing.T) {
		workspace := t.TempDir()
		writeEntries(t, workspace, defaultMaxEntries-1)
		adapter := NewRuntimeAdapter()
		start := make(chan struct{})
		type outcome struct {
			result RuntimeFileOperationResult
			err    error
		}
		outcomes := make(chan outcome, 2)
		for _, targetRef := range []string{"future-a.txt", "future-b.txt"} {
			targetRef := targetRef
			go func() {
				<-start
				result, err := adapter.ApplyPatch(PatchApplyRequest{
					WorkspaceDirectory: workspace, TargetRef: targetRef, PatchType: patchTypeFileWrite,
					Content: targetRef + "\n", ContentPresent: true, CreateOnly: true,
				})
				outcomes <- outcome{result: result, err: err}
			}()
		}
		close(start)
		succeeded, capacityRejected := 0, 0
		for range 2 {
			value := <-outcomes
			switch {
			case value.err == nil:
				succeeded++
			case errors.Is(value.err, common.ErrConflict) && value.result.FailureCode == "workspace_manifest_capacity_exhausted":
				capacityRejected++
			default:
				t.Fatalf("unexpected concurrent create outcome=%+v err=%v", value.result, value.err)
			}
		}
		manifest, err := adapter.AcquireWorkspaceManifest(workspace)
		if err != nil || succeeded != 1 || capacityRejected != 1 || len(manifest.TargetRefs) != defaultMaxEntries {
			t.Fatalf("concurrent capacity success/rejected/targets=%d/%d/%d err=%v", succeeded, capacityRejected, len(manifest.TargetRefs), err)
		}
	})
}

func TestRuntimeAdapterManifestFailsClosedOnIncompleteInventoryObservation(t *testing.T) {
	injectedErr := errors.New("injected manifest observation failure")
	type failureCase struct {
		name      string
		configure func(*RuntimeAdapter)
	}
	cases := []failureCase{
		{
			name: "walk error",
			configure: func(adapter *RuntimeAdapter) {
				adapter.walkDir = func(root string, walkFn fs.WalkDirFunc) error {
					return filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
						if filepath.Base(path) == "z-fault.txt" {
							return walkFn(path, entry, injectedErr)
						}
						return walkFn(path, entry, walkErr)
					})
				}
			},
		},
		{
			name: "entry info error",
			configure: func(adapter *RuntimeAdapter) {
				adapter.walkDir = func(root string, walkFn fs.WalkDirFunc) error {
					return filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
						if walkErr == nil && filepath.Base(path) == "z-fault.txt" {
							entry = runtimeAdapterFailingInfoEntry{DirEntry: entry, err: injectedErr}
						}
						return walkFn(path, entry, walkErr)
					})
				}
			},
		},
		{
			name: "content hash error",
			configure: func(adapter *RuntimeAdapter) {
				adapter.streamHashAndType = func(path string) (string, bool, error) {
					if filepath.Base(path) == "z-fault.txt" {
						return "", false, injectedErr
					}
					return streamFileHashAndBinary(path)
				}
			},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			workspace := t.TempDir()
			seed := map[string]string{"a-kept.txt": "kept\n", "z-fault.txt": "fault\n"}
			for targetRef, content := range seed {
				if err := os.WriteFile(filepath.Join(workspace, targetRef), []byte(content), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			adapter := NewRuntimeAdapter()
			testCase.configure(adapter)

			listed, err := adapter.ListFiles(FileListRequest{WorkspaceDirectory: workspace, Path: ".", Recursive: true})
			if err == nil || listed.FailureCode != "file_list_failed" {
				t.Fatalf("incomplete list = %+v err=%v, want file_list_failed", listed, err)
			}
			if _, err := adapter.AcquireWorkspaceManifest(workspace); err == nil {
				t.Fatal("incomplete observation produced a workspace manifest")
			}
			if _, err := adapter.AcquireRuntimeTargetSelection(
				workspace, "target-selection-incomplete-unique", "deliverable", runtimeFileTargetSelectorUniqueWorkspaceEntry, "", 4,
			); err == nil {
				t.Fatal("incomplete observation degraded to a unique target selection")
			}
			if _, err := adapter.AcquireRuntimeTargetSelection(
				workspace, "target-selection-incomplete-create", "deliverable", runtimeFileTargetSelectorWorkspaceDeliverable, "future.txt", 4,
			); err == nil {
				t.Fatal("incomplete observation admitted a prospective target")
			}

			for targetRef, want := range seed {
				got, err := os.ReadFile(filepath.Join(workspace, targetRef))
				if err != nil || string(got) != want {
					t.Fatalf("incomplete observation mutated %s: content=%q err=%v", targetRef, got, err)
				}
			}
			if _, err := os.Lstat(filepath.Join(workspace, "future.txt")); !os.IsNotExist(err) {
				t.Fatalf("incomplete observation created future target: %v", err)
			}
		})
	}
}

func TestAcquireWorkspaceManifestRevisionTracksContent(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "go.mod")
	if err := os.WriteFile(path, []byte("module example.com/one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	first, err := NewRuntimeAdapter().AcquireWorkspaceManifest(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.TargetRefs) != 1 || first.TargetRefs[0] != "go.mod" || first.Revision == "" {
		t.Fatalf("first manifest = %+v", first)
	}
	if err := os.WriteFile(path, []byte("module example.com/two\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	second, err := NewRuntimeAdapter().AcquireWorkspaceManifest(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if second.Revision == first.Revision {
		t.Fatalf("manifest revision did not change with content: first=%+v second=%+v", first, second)
	}
}

func TestAcquireWorkspaceManifestRevisionTracksDeliverableTopologyOnly(t *testing.T) {
	workspace := t.TempDir()
	adapter := NewRuntimeAdapter()
	write := func(ref, content string) {
		t.Helper()
		path := filepath.Join(workspace, filepath.FromSlash(ref))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	manifest := func() runtimecontract.WorkspaceManifest {
		t.Helper()
		value, err := adapter.AcquireWorkspaceManifest(workspace)
		if err != nil {
			t.Fatal(err)
		}
		return value
	}

	write("a.txt", "a\n")
	baseline := manifest()
	if len(baseline.TargetRefs) != 1 || baseline.TargetRefs[0] != "a.txt" || baseline.Revision == "" {
		t.Fatalf("baseline manifest = %+v", baseline)
	}

	write("b.txt", "b\n")
	created := manifest()
	if created.Revision == baseline.Revision || !reflect.DeepEqual(created.TargetRefs, []string{"a.txt", "b.txt"}) {
		t.Fatalf("create did not invalidate manifest: baseline=%+v created=%+v", baseline, created)
	}
	if err := os.Remove(filepath.Join(workspace, "b.txt")); err != nil {
		t.Fatal(err)
	}
	deleted := manifest()
	if deleted.Revision != baseline.Revision || !reflect.DeepEqual(deleted.TargetRefs, baseline.TargetRefs) {
		t.Fatalf("delete did not restore manifest: baseline=%+v deleted=%+v", baseline, deleted)
	}

	if err := os.Rename(filepath.Join(workspace, "a.txt"), filepath.Join(workspace, "renamed.txt")); err != nil {
		t.Fatal(err)
	}
	renamed := manifest()
	if renamed.Revision == baseline.Revision || !reflect.DeepEqual(renamed.TargetRefs, []string{"renamed.txt"}) {
		t.Fatalf("rename did not invalidate manifest: baseline=%+v renamed=%+v", baseline, renamed)
	}
	if err := os.Rename(filepath.Join(workspace, "renamed.txt"), filepath.Join(workspace, "a.txt")); err != nil {
		t.Fatal(err)
	}
	if restored := manifest(); restored.Revision != baseline.Revision || !reflect.DeepEqual(restored.TargetRefs, baseline.TargetRefs) {
		t.Fatalf("rename-back did not restore manifest: baseline=%+v restored=%+v", baseline, restored)
	}

	for _, ref := range []string{".workorch-data/runtime.json", ".git/HEAD", "node_modules/generated.js", "vendor/cache.go", "dist/bundle.js", "build/output.bin"} {
		write(ref, "generated\n")
	}
	generated := manifest()
	if generated.Revision != baseline.Revision || !reflect.DeepEqual(generated.TargetRefs, baseline.TargetRefs) {
		t.Fatalf("owner-internal/generated paths polluted deliverable manifest: baseline=%+v generated=%+v", baseline, generated)
	}
}

func TestRuntimeAdapterRejectsPathEscape(t *testing.T) {
	workspace := t.TempDir()
	_, err := NewRuntimeAdapter().ReadFile(context.Background(), FileReadRequest{
		WorkspaceDirectory: workspace,
		Path:               "../outside.txt",
	})
	if err == nil {
		t.Fatalf("file/read should reject parent traversal")
	}
}

func TestRuntimeAdapterRejectsSymlinkEscape(t *testing.T) {
	workspace := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0o644); err != nil {
		t.Fatalf("seed outside file: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(workspace, "outside")); err != nil {
		t.Fatalf("create symlink: %v", err)
	}

	_, err := NewRuntimeAdapter().ReadFile(context.Background(), FileReadRequest{
		WorkspaceDirectory: workspace,
		Path:               "outside/secret.txt",
	})
	if err == nil {
		t.Fatalf("file/read should reject symlink escape")
	}
}

func TestRuntimeAdapterTextReplacePreviewDoesNotWrite(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "main.go")
	original := "package main\n\nfunc main() {\n\tprintln(\"old\")\n}\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	sum := sha256.Sum256([]byte(original))
	baseHash := hex.EncodeToString(sum[:])

	result, err := NewRuntimeAdapter().PreviewPatch(PatchApplyRequest{
		WorkspaceDirectory: workspace, PatchType: "text_replace", TargetRef: "main.go",
		OldText: "println(\"old\")", NewText: "println(\"new\")", NewTextPresent: true, BaseHash: baseHash,
	})
	if err != nil {
		t.Fatalf("edit file: %v", err)
	}
	if result.Stage != "patch_preview" || result.MatchCount != 1 || result.PatchType != "text_replace" || result.TargetRef != "main.go" {
		t.Fatalf("preview = %+v, want text replacement preview with one match", result)
	}
	current, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read after edit: %v", err)
	}
	if string(current) != original {
		t.Fatalf("file/edit wrote file; content = %q", string(current))
	}
}

func TestRuntimeAdapterEditRejectsAmbiguousMatch(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "main.go"), []byte("x\nx\n"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	_, err := NewRuntimeAdapter().PreviewPatch(PatchApplyRequest{
		WorkspaceDirectory: workspace, PatchType: "text_replace", TargetRef: "main.go",
		OldText: "x", NewText: "y", NewTextPresent: true,
	})
	if err == nil {
		t.Fatalf("file/edit should reject ambiguous oldText")
	}
}

func TestRuntimeAdapterEditRejectsBaseHashMismatch(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "main.go"), []byte("old\n"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	_, err := NewRuntimeAdapter().PreviewPatch(PatchApplyRequest{
		WorkspaceDirectory: workspace, PatchType: "text_replace", TargetRef: "main.go",
		OldText: "old", NewText: "new", NewTextPresent: true, BaseHash: "stale",
	})
	if err == nil {
		t.Fatalf("file/edit should reject stale baseHash")
	}
}

func TestRuntimeAdapterTextReplacePreviewUsesExplicitOccurrenceWithoutWriting(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "main.go")
	original := "x\nx\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	result, err := NewRuntimeAdapter().PreviewPatch(PatchApplyRequest{
		WorkspaceDirectory: workspace, PatchType: "text_replace", TargetRef: "main.go",
		OldText: "x", NewText: "y", NewTextPresent: true, Occurrence: 2,
	})
	if err != nil {
		t.Fatalf("edit file occurrence: %v", err)
	}
	if result.MatchCount != 2 {
		t.Fatalf("preview = %+v, want two source matches with explicit occurrence", result)
	}
	current, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read after edit: %v", err)
	}
	if string(current) != original {
		t.Fatalf("file/edit wrote file; content = %q", string(current))
	}
}

func TestRuntimeAdapterEditRejectsBinaryFile(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "asset.bin")
	if err := os.WriteFile(path, []byte{0x00, 0x01, 0x02, 0x03}, 0o644); err != nil {
		t.Fatalf("seed binary file: %v", err)
	}

	_, err := NewRuntimeAdapter().PreviewPatch(PatchApplyRequest{
		WorkspaceDirectory: workspace, PatchType: "text_replace", TargetRef: "asset.bin",
		OldText: "\x00", NewText: "x", NewTextPresent: true,
	})
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("file/edit binary error = %v, want invalid argument", err)
	}
	content, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("read binary after edit: %v", readErr)
	}
	if string(content) != string([]byte{0x00, 0x01, 0x02, 0x03}) {
		t.Fatalf("binary content changed: %v", content)
	}
}

func TestRuntimeAdapterPatchPreviewFileWriteDoesNotWrite(t *testing.T) {
	workspace := t.TempDir()
	target := filepath.Join(workspace, "README.md")

	result, err := NewRuntimeAdapter().PreviewPatch(PatchApplyRequest{
		WorkspaceDirectory: workspace, PatchType: "file_write", TargetRef: "README.md",
		Content: "hello\n", ContentPresent: true,
	})
	if err != nil {
		t.Fatalf("patch preview file_write: %v", err)
	}
	if result.Stage != "patch_preview" || result.WillWrite {
		t.Fatalf("preview output = %+v, want non-writing preview", result)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("patch/preview wrote target or unexpected stat error: %v", err)
	}
}

func TestRuntimeAdapterPatchApplyFileWriteWritesOnce(t *testing.T) {
	workspace := t.TempDir()
	target := filepath.Join(workspace, "README.md")

	result, err := NewRuntimeAdapter().ApplyPatch(PatchApplyRequest{
		WorkspaceDirectory: workspace, PatchType: "file_write", TargetRef: "README.md",
		Content: "hello\n", ContentPresent: true,
	})
	if err != nil {
		t.Fatalf("patch apply file_write: %v", err)
	}
	if result.Stage != "patch_apply" || result.AfterHash == "" || result.ContentHash == "" {
		t.Fatalf("apply output = %+v, want hash metadata", result)
	}
	content, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read applied file: %v", err)
	}
	if string(content) != "hello\n" {
		t.Fatalf("applied content = %q", string(content))
	}
}

func TestRuntimeAdapterPatchApplyFileWriteRejectsExistingTargetWithoutBaseHash(t *testing.T) {
	workspace := t.TempDir()
	target := filepath.Join(workspace, "main.go")
	original := []byte("package main\n")
	if err := os.WriteFile(target, original, 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := NewRuntimeAdapter().ApplyPatch(PatchApplyRequest{
		WorkspaceDirectory: workspace, PatchType: "file_write", TargetRef: "main.go",
		Content: "# markdown\n", ContentPresent: true,
	})
	if !errors.Is(err, common.ErrConflict) || result.FailureCode != "patch_basehash_required" {
		t.Fatalf("unversioned replacement result=%+v err=%v", result, err)
	}
	content, readErr := os.ReadFile(target)
	if readErr != nil || string(content) != string(original) {
		t.Fatalf("unversioned replacement changed target to %q err=%v", string(content), readErr)
	}
}

func TestRuntimeAdapterPatchApplyFileWriteConfirmsSealedReplayAfterReceiptLoss(t *testing.T) {
	workspace := t.TempDir()
	target := filepath.Join(workspace, "README.md")
	before := []byte("before\n")
	if err := os.WriteFile(target, before, 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	baseHash := sha256Hex(before)
	request := PatchApplyRequest{
		WorkspaceDirectory: workspace, PatchType: patchTypeFileWrite, TargetRef: "README.md",
		Content: "after\n", ContentPresent: true, BaseHash: baseHash,
	}
	first, err := NewRuntimeAdapter().ApplyPatch(request)
	if err != nil || first.BeforeHash != baseHash || first.AfterHash == "" || first.BeforeHash == first.AfterHash {
		t.Fatalf("initial sealed write = %+v err=%v", first, err)
	}
	// Model a process loss after the durable write and before its owner receipt
	// reached the Orchestrator. The retry sees the sealed postcondition, does
	// not need the original in-memory result, and preserves the same logical
	// resource delta for the persisted execution key/fence.
	replayed, err := NewRuntimeAdapter().ApplyPatch(request)
	if err != nil || replayed.Summary != "patch file_write replay confirmed" || replayed.BeforeHash != baseHash || replayed.AfterHash != first.AfterHash || replayed.ContentHash != first.ContentHash {
		t.Fatalf("lost-receipt replay = %+v err=%v", replayed, err)
	}
	observation := fileMutationReceiptObservation(workspace, replayed.TargetRefs, runtimecontract.RuntimeOperationExecutionStatusAccepted, replayed)
	if !observation.Changed || observation.NoOp || observation.BeforeHash != baseHash || observation.AfterHash != first.AfterHash {
		t.Fatalf("replayed receipt observation lost durable delta: %+v", observation)
	}
	content, err := os.ReadFile(target)
	if err != nil || string(content) != "after\n" {
		t.Fatalf("replay changed sealed postcondition: content=%q err=%v", content, err)
	}
}

func TestRuntimeAdapterPatchApplyReplayProofCoversAllDurableVariants(t *testing.T) {
	tests := []struct {
		name       string
		parameters func() []runtimecontract.OperationParameter
	}{
		{
			name: "file_write",
			parameters: func() []runtimecontract.OperationParameter {
				patchType, content := patchTypeFileWrite, "after\n"
				return []runtimecontract.OperationParameter{{Name: "patchType", String: &patchType}, {Name: "content", String: &content}}
			},
		},
		{
			name: "text_replace",
			parameters: func() []runtimecontract.OperationParameter {
				patchType, oldText, newText := patchTypeTextReplace, "before", "after"
				return []runtimecontract.OperationParameter{{Name: "patchType", String: &patchType}, {Name: "oldText", String: &oldText}, {Name: "newText", String: &newText}}
			},
		},
		{
			name: "unified_diff",
			parameters: func() []runtimecontract.OperationParameter {
				patchType := patchTypeUnifiedDiff
				diff := "--- a/README.md\n+++ b/README.md\n@@ -1,1 +1,1 @@\n-before\n+after\n"
				return []runtimecontract.OperationParameter{{Name: "patchType", String: &patchType}, {Name: "diff", String: &diff}}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			workspace := t.TempDir()
			if err := os.WriteFile(filepath.Join(workspace, "README.md"), []byte("before\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			adapter := NewRuntimeAdapter()
			inventory, err := adapter.AcquireRuntimeOperationAdmissibility(workspace, "admissibility-file", []string{runtimecontract.RuntimeBlockKindPatchApply}, []string{"README.md"}, 4)
			if err != nil {
				t.Fatal(err)
			}
			operation := runtimecontract.RuntimeOperation{ID: "patch-1", Kind: runtimecontract.RuntimeBlockKindPatchApply, TargetRefs: []string{"README.md"}, Parameters: test.parameters()}
			if test.name == "file_write" {
				baseHash := sha256Hex([]byte("before\n"))
				operation.Parameters = append(operation.Parameters, runtimecontract.OperationParameter{Name: "baseHash", String: &baseHash})
			}
			failure, stale, proof := adapter.ProbeRuntimeOperationFeasibilityWithReplayProof(workspace, inventory, operation, nil)
			if failure != nil || stale != nil || proof == nil || proof.OwnerModuleID != ModuleID || proof.OperationID != operation.ID || proof.BeforeHash == proof.AfterHash || proof.AdmissibilitySnapshotID != inventory.SnapshotID {
				t.Fatalf("owner replay proof = %+v failure=%+v stale=%+v", proof, failure, stale)
			}
			request, ok := patchApplyRequestFromRuntimeOperation(workspace, operation)
			if !ok {
				t.Fatal("patch request conversion failed")
			}
			request.ReplayProof = proof
			first, err := adapter.ApplyPatch(request)
			if err != nil || first.BeforeHash != proof.BeforeHash || first.AfterHash != proof.AfterHash {
				t.Fatalf("initial proof-bound apply = %+v err=%v", first, err)
			}
			replayed, err := adapter.ApplyPatch(request)
			if err != nil || replayed.Summary != "patch replay proof confirmed" || replayed.BeforeHash != proof.BeforeHash || replayed.AfterHash != proof.AfterHash {
				t.Fatalf("proof replay = %+v err=%v", replayed, err)
			}
			content, err := os.ReadFile(filepath.Join(workspace, "README.md"))
			if err != nil || sha256Hex(content) != proof.AfterHash {
				t.Fatalf("replay changed target content=%q hash=%q err=%v", content, sha256Hex(content), err)
			}
			// A receipt-loss replay may confirm only the postcondition sealed by
			// the original owner feasibility probe. If another writer changes the
			// target before recovery, the File owner must not apply the old patch
			// again or treat the external content as the original operation's
			// durable effect.
			if err := os.WriteFile(filepath.Join(workspace, "README.md"), []byte("external-change\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := adapter.ApplyPatch(request); !errors.Is(err, common.ErrConflict) {
				t.Fatalf("stale replay proof error = %v, want conflict", err)
			}
			content, err = os.ReadFile(filepath.Join(workspace, "README.md"))
			if err != nil || string(content) != "external-change\n" {
				t.Fatalf("stale replay rewrote external content=%q err=%v", content, err)
			}
		})
	}
}

func TestFileOperationSnapshotPrerequisiteRequestRequiresMissingCurrentSnapshot(t *testing.T) {
	targetRef, patchType, oldText, newText := "target.txt", "text_replace", "old", "new"
	operation := runtimecontract.RuntimeOperation{
		ID: "replace-target", Kind: runtimecontract.RuntimeBlockKindPatchApply, TargetRefs: []string{targetRef},
		Parameters: []runtimecontract.OperationParameter{{Name: "patchType", String: &patchType}, {Name: "oldText", String: &oldText}, {Name: "newText", String: &newText}},
	}
	failure := &runtimecontract.FailureFact{Class: runtimecontract.FailureClassCandidateContractFailure, Code: "operation_replay_proof_unavailable", Signature: "file-preview-conflict", Message: "preview cannot prove patch", TargetRefs: []string{targetRef}}
	request := fileOperationSnapshotPrerequisiteRequest(operation, failure, nil)
	if request == nil || request.OperationID != operation.ID || request.Kind != runtimecontract.PrerequisiteKindTargetSnapshot || request.OwnerModuleID != ModuleID || request.SubjectRef != targetRef {
		t.Fatalf("missing-snapshot feasibility request = %+v", request)
	}
	if repeated := fileOperationSnapshotPrerequisiteRequest(operation, failure, &runtimecontract.TargetSnapshot{TargetRef: targetRef, ContentHash: "current", ContentState: runtimecontract.SnapshotContentStateComplete}); repeated != nil {
		t.Fatalf("existing current snapshot must leave a repeated patch as an ordinary candidate failure: %+v", repeated)
	}
}

func TestFileOwnerPatchApplyReplaysSealedWriteAfterReceiptCommitFailure(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	owner, err := New(hub, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { owner.Teardown(ctx) })

	workspace := t.TempDir()
	before := []byte("before\n")
	if err := os.WriteFile(filepath.Join(workspace, "README.md"), before, 0o644); err != nil {
		t.Fatal(err)
	}
	baseHash := sha256Hex(before)
	proof := runtimecontract.RuntimeOperationReplayProof{
		OwnerModuleID: ModuleID, OperationID: "write-1", TargetRefs: []string{"README.md"},
		BeforeHash: baseHash, AfterHash: sha256Hex([]byte("after\n")), AdmissibilitySnapshotID: "admissibility-1",
	}
	commitCalls := 0
	receipts := map[string]runtimecontract.RuntimeOperationReceipt{}
	receiptObserver := magicevent.NewSimpleObserver(orchestratorevents.CommandDestination, hub)
	receiptObserver.Subscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt, func(event magicevent.Event, result magicevent.Result) {
		command := event.Data().(orchestratorevents.QueryRuntimeOperationReceiptCommand)
		receipt, found := receipts[command.ExecutionKey]
		response := orchestratorevents.QueryRuntimeOperationReceiptResult{Found: found}
		if found {
			response.Receipt = &receipt
		}
		result.Set(response, nil)
	})
	receiptObserver.Subscribe(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt, func(event magicevent.Event, result magicevent.Result) {
		commitCalls++
		if commitCalls == 1 {
			result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: "injected receipt CAS loss", Cause: errors.New("injected receipt CAS loss")})
			return
		}
		command := event.Data().(orchestratorevents.CommitRuntimeOperationReceiptCommand)
		if receipt, found := receipts[command.Receipt.ExecutionKey]; found {
			result.Set(orchestratorevents.CommitRuntimeOperationReceiptResult{Receipt: receipt}, nil)
			return
		}
		receipts[command.Receipt.ExecutionKey] = command.Receipt
		result.Set(orchestratorevents.CommitRuntimeOperationReceiptResult{Receipt: command.Receipt}, nil)
	})
	t.Cleanup(func() {
		receiptObserver.Unsubscribe(orchestratorevents.TopicOrchestratorQueryRuntimeOperationReceipt)
		receiptObserver.Unsubscribe(orchestratorevents.TopicOrchestratorCommitRuntimeOperationReceipt)
	})

	patchType := patchTypeFileWrite
	operationContent := "after\n"
	operation := runtimecontract.RuntimeOperation{
		ID: "write-1", Kind: runtimecontract.RuntimeBlockKindPatchApply, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, TargetRefs: []string{"README.md"},
		Parameters: []runtimecontract.OperationParameter{
			{Name: "patchType", String: &patchType}, {Name: "content", String: &operationContent}, {Name: "baseHash", String: &baseHash},
		},
	}
	sealedOperation, err := fileevents.SealRuntimeOperation(operation)
	if err != nil {
		t.Fatal(err)
	}
	command := fileevents.ExecutePatchApplyOperationCommand{
		RuntimeOperationExecutionContext: runtimecontract.RuntimeOperationExecutionContext{
			Execution: fileBizTestExecutionEnvelope(operation, runtimecontract.RuntimeOperationExecutionEnvelope{
				ExecutionKey: "exec/ws-1/run-1/dispatch-1/write-1", Fence: 1, ExpectedEffectScope: runtimecontract.RuntimeEffectWorkspaceDurable,
				AdmissibilitySnapshotID: proof.AdmissibilitySnapshotID, ReplayProof: &proof,
			}),
			WorkspaceID: "ws-1", RunID: "run-1", DispatchID: "dispatch-1",
			Transaction: runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "authority-1"},
			OperationID: "write-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, TargetRefs: []string{"README.md"},
			EvidenceKind: runtimecontract.EvidenceKindStateChange, WorkspaceDeliverableDirectory: workspace,
		},
		SealedOperation: sealedOperation,
	}
	send := func() magicevent.Result {
		return hub.Send(magicevent.NewEventWithContext(fileevents.TopicExecutePatchApplyOperation, runtimecontract.RuntimeModuleOrchestrator, ModuleID, magicevent.NewHeader(), ctx, command))
	}
	if _, err := send().Get(); err == nil {
		t.Fatal("first owner execution unexpectedly succeeded despite receipt commit loss")
	}
	content, err := os.ReadFile(filepath.Join(workspace, "README.md"))
	if err != nil || string(content) != "after\n" {
		t.Fatalf("first execution did not apply sealed write: content=%q err=%v", content, err)
	}
	data, resultErr := send().Get()
	if resultErr != nil {
		t.Fatalf("replay after receipt loss: %v", resultErr)
	}
	response, ok := data.(fileevents.ExecuteOperationResult)
	if !ok || response.Receipt.ExecutionKey != command.Execution.ExecutionKey || !response.Receipt.Changed || response.Receipt.BeforeHash != baseHash || response.Receipt.AfterHash == baseHash || commitCalls != 2 {
		t.Fatalf("owner replay did not commit the sealed logical delta once: response=%+v calls=%d", response, commitCalls)
	}
	content, err = os.ReadFile(filepath.Join(workspace, "README.md"))
	if err != nil || string(content) != "after\n" {
		t.Fatalf("replay rewrote sealed postcondition: content=%q err=%v", content, err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "README.md"), []byte("external-change\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	data, resultErr = send().Get()
	if resultErr != nil {
		t.Fatalf("duplicate persisted receipt: %v", resultErr)
	}
	duplicate, ok := data.(fileevents.ExecuteOperationResult)
	if !ok || !runtimecontract.SameRuntimeOperationReceipt(duplicate.Receipt, response.Receipt) || commitCalls != 2 {
		t.Fatalf("duplicate delivery did not reuse persisted receipt: response=%+v calls=%d", duplicate, commitCalls)
	}
	content, err = os.ReadFile(filepath.Join(workspace, "README.md"))
	if err != nil || string(content) != "external-change\n" {
		t.Fatalf("duplicate persisted receipt applied a second file effect: content=%q err=%v", content, err)
	}
}

func fileBizTestExecutionEnvelope(operation runtimecontract.RuntimeOperation, envelope runtimecontract.RuntimeOperationExecutionEnvelope) runtimecontract.RuntimeOperationExecutionEnvelope {
	envelope.OperationHash = runtimecontract.StableRuntimeOperationHash(operation)
	envelope.IdempotencyClass = runtimecontract.ResolveOperationIdempotencyClass(operation, nil)
	envelope.AuthorityHash = runtimecontract.StableRuntimeOperationExecutionAuthorityHash(runtimecontract.RuntimeOperationExecutionRecord{
		OperationID: operation.ID, OperationHash: envelope.OperationHash, ExecutionKey: envelope.ExecutionKey, ExpectedEffectScope: envelope.ExpectedEffectScope,
		IdempotencyClass: envelope.IdempotencyClass, AdmissibilitySnapshotID: envelope.AdmissibilitySnapshotID,
		AcquisitionBinding: envelope.AcquisitionBinding, ReplayProof: envelope.ReplayProof,
	})
	return envelope
}

func TestRuntimeAdapterProbeRejectsUnverifiedFileWriteReplayBaseHash(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "README.md"), []byte("current\n"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	adapter := NewRuntimeAdapter()
	inventory, err := adapter.AcquireRuntimeOperationAdmissibility(workspace, "admissibility-file", []string{runtimecontract.RuntimeBlockKindPatchApply}, []string{"README.md"}, 4)
	if err != nil {
		t.Fatal(err)
	}
	patchType, content, staleHash := patchTypeFileWrite, "after\n", "unverified-prior-revision"
	failure := adapter.ProbeRuntimeOperationFeasibility(workspace, inventory, runtimecontract.RuntimeOperation{
		ID: "sealed-file-write", Kind: runtimecontract.RuntimeBlockKindPatchApply, TargetRefs: []string{"README.md"},
		Parameters: []runtimecontract.OperationParameter{
			{Name: "patchType", String: &patchType},
			{Name: "content", String: &content}, {Name: "baseHash", String: &staleHash},
		},
	})
	if failure == nil || failure.Code != "operation_base_hash_mismatch" {
		t.Fatalf("unverified replay precondition = %+v", failure)
	}
}

func TestRuntimeAdapterPatchApplyRejectsBaseHashMismatch(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "README.md"), []byte("current\n"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	_, err := NewRuntimeAdapter().ApplyPatch(PatchApplyRequest{
		WorkspaceDirectory: workspace, PatchType: "file_write", TargetRef: "README.md",
		Content: "next\n", ContentPresent: true, BaseHash: "stale",
	})
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("patch/apply baseHash error = %v, want conflict", err)
	}
	content, readErr := os.ReadFile(filepath.Join(workspace, "README.md"))
	if readErr != nil {
		t.Fatalf("read after rejected patch: %v", readErr)
	}
	if string(content) != "current\n" {
		t.Fatalf("rejected patch changed file: %q", string(content))
	}
}

func TestRuntimeAdapterPatchApplyTextReplaceRejectsMissingOldText(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "main.txt"), []byte("old\n"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	_, err := NewRuntimeAdapter().ApplyPatch(PatchApplyRequest{
		WorkspaceDirectory: workspace, PatchType: "text_replace", TargetRef: "main.txt",
		OldText: "missing", NewText: "new", NewTextPresent: true,
	})
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("patch/apply oldText error = %v, want conflict", err)
	}
}

func TestRuntimeAdapterPatchApplyTextReplaceRejectsAmbiguousOldText(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "main.txt"), []byte("x\nx\n"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	_, err := NewRuntimeAdapter().ApplyPatch(PatchApplyRequest{
		WorkspaceDirectory: workspace, PatchType: "text_replace", TargetRef: "main.txt",
		OldText: "x", NewText: "y", NewTextPresent: true,
	})
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("patch/apply ambiguous oldText error = %v, want conflict", err)
	}
}

func TestRuntimeAdapterPatchApplyRejectsBinaryTarget(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "asset.bin")
	if err := os.WriteFile(path, []byte{0, 1, 2}, 0o644); err != nil {
		t.Fatalf("seed binary file: %v", err)
	}

	_, err := NewRuntimeAdapter().ApplyPatch(PatchApplyRequest{
		WorkspaceDirectory: workspace, PatchType: "file_write", TargetRef: "asset.bin",
		Content: "text\n", ContentPresent: true,
	})
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("patch/apply binary target error = %v, want invalid argument", err)
	}
	content, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("read binary file: %v", readErr)
	}
	if string(content) != string([]byte{0, 1, 2}) {
		t.Fatalf("binary target changed: %v", content)
	}
}

func TestRuntimeAdapterPatchApplyRejectsPathEscape(t *testing.T) {
	workspace := t.TempDir()
	_, err := NewRuntimeAdapter().ApplyPatch(PatchApplyRequest{
		WorkspaceDirectory: workspace, PatchType: "file_write", TargetRef: "../outside.txt",
		Content: "outside\n", ContentPresent: true,
	})
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("patch/apply path escape error = %v, want invalid argument", err)
	}
}

func TestRuntimeAdapterPatchApplyRejectsAbsolutePath(t *testing.T) {
	workspace := t.TempDir()
	_, err := NewRuntimeAdapter().ApplyPatch(PatchApplyRequest{
		WorkspaceDirectory: workspace, PatchType: "file_write", TargetRef: filepath.Join(workspace, "outside.txt"),
		Content: "outside\n", ContentPresent: true,
	})
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("patch/apply absolute path error = %v, want invalid argument", err)
	}
}

func TestRuntimeAdapterPatchApplyRejectsSymlinkEscape(t *testing.T) {
	workspace := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(workspace, "outside")); err != nil {
		t.Fatalf("create symlink: %v", err)
	}
	_, err := NewRuntimeAdapter().ApplyPatch(PatchApplyRequest{
		WorkspaceDirectory: workspace, PatchType: "file_write", TargetRef: "outside/file.txt",
		Content: "outside\n", ContentPresent: true,
	})
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("patch/apply symlink escape error = %v, want invalid argument", err)
	}
}

func TestRuntimeAdapterPatchApplyRejectsMissingParentConflict(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "dir"), []byte("not a directory"), 0o644); err != nil {
		t.Fatalf("seed parent file: %v", err)
	}
	_, err := NewRuntimeAdapter().ApplyPatch(PatchApplyRequest{
		WorkspaceDirectory: workspace, PatchType: "file_write", TargetRef: "dir/file.txt",
		Content: "hello\n", ContentPresent: true,
	})
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("patch/apply parent conflict error = %v, want conflict", err)
	}
}

func TestRuntimeAdapterPatchApplyConsumesValidatedApprovalGrant(t *testing.T) {
	workspace := t.TempDir()
	request := PatchApplyRequest{
		WorkspaceDirectory: workspace, PatchType: "file_write", TargetRef: ".env",
		Content: "TOKEN=granted\n", ContentPresent: true,
	}
	if _, err := NewRuntimeAdapter().ApplyPatch(request); err == nil {
		t.Fatal("sensitive patch/apply must require approval before side effects")
	}
	if _, err := os.Stat(filepath.Join(workspace, ".env")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("approval-required preview changed file: %v", err)
	}
	request.ApprovalGranted = true
	result, err := NewRuntimeAdapter().ApplyPatch(request)
	if err != nil {
		t.Fatalf("granted patch/apply: %v", err)
	}
	content, readErr := os.ReadFile(filepath.Join(workspace, ".env"))
	if readErr != nil || string(content) != "TOKEN=granted\n" || result.AfterHash == "" {
		t.Fatalf("granted patch result=%+v content=%q err=%v", result, string(content), readErr)
	}
}

func TestRuntimeAdapterPatchApplyAllowsLargeWorkspaceLocalSourceChange(t *testing.T) {
	workspace := t.TempDir()
	target := filepath.Join(workspace, "internal", "lsp", "manager.go")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	before := strings.Repeat("// previous implementation\n", 293)
	after := strings.Repeat("// replacement implementation\n", 228)
	if err := os.WriteFile(target, []byte(before), 0o644); err != nil {
		t.Fatal(err)
	}

	request := PatchApplyRequest{
		WorkspaceDirectory: workspace, PatchType: patchTypeFileWrite, TargetRef: "internal/lsp/manager.go",
		Content: after, ContentPresent: true, BaseHash: sha256Hex([]byte(before)),
	}
	preview, err := NewRuntimeAdapter().PreviewPatch(request)
	if err != nil {
		t.Fatalf("preview large workspace-local source change: %v", err)
	}
	if preview.ApprovalRequired || preview.RiskLevel != "low" {
		t.Fatalf("large workspace-local source preview unexpectedly requires approval: %+v", preview)
	}
	result, err := NewRuntimeAdapter().ApplyPatch(request)
	if err != nil {
		t.Fatalf("apply large workspace-local source change: %v", err)
	}
	if result.AfterHash == result.BeforeHash || result.FailureCode != "" {
		t.Fatalf("large workspace-local source apply = %+v", result)
	}
	content, err := os.ReadFile(target)
	if err != nil || string(content) != after {
		t.Fatalf("large workspace-local source content=%q err=%v", string(content), err)
	}
}

func TestRuntimeAdapterPatchApplyUnifiedDiffSuccess(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "main.txt")
	if err := os.WriteFile(path, []byte("one\ntwo\nthree\n"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	diff := "--- a/main.txt\n+++ b/main.txt\n@@ -1,3 +1,3 @@\n one\n-two\n+TWO\n three\n"

	result, err := NewRuntimeAdapter().ApplyPatch(PatchApplyRequest{
		WorkspaceDirectory: workspace, PatchType: "unified_diff", TargetRef: "main.txt", Diff: diff,
	})
	if err != nil {
		t.Fatalf("patch/apply unified diff: %v", err)
	}
	if result.DiffHash == "" || result.AddedLines != 1 || result.DeletedLines != 1 {
		t.Fatalf("unified diff output = %+v, want diff metadata", result)
	}
	content, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("read patched file: %v", readErr)
	}
	if string(content) != "one\nTWO\nthree\n" {
		t.Fatalf("patched content = %q", string(content))
	}
}

func TestRuntimeAdapterPatchApplyUnifiedDiffConsolidatesMultipleHunks(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "main.txt")
	before := "one\ntwo\nthree\nfour\nfive\nsix\nseven\neight\n"
	if err := os.WriteFile(path, []byte(before), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	diff := "--- a/main.txt\n+++ b/main.txt\n" +
		"@@ -1,3 +1,3 @@\n one\n-two\n+TWO\n three\n" +
		"@@ -6,3 +6,3 @@\n six\n-seven\n+SEVEN\n eight\n"

	adapter := NewRuntimeAdapter()
	preview, err := adapter.PreviewPatch(PatchApplyRequest{
		WorkspaceDirectory: workspace, PatchType: "unified_diff", TargetRef: "main.txt", Diff: diff,
		BaseHash: sha256Hex([]byte(before)),
	})
	if err != nil {
		t.Fatalf("preview multi-hunk unified diff: %v", err)
	}
	if preview.AddedLines != 2 || preview.DeletedLines != 2 || preview.BeforeHash == preview.AfterHash {
		t.Fatalf("multi-hunk preview = %+v", preview)
	}
	result, err := adapter.ApplyPatch(PatchApplyRequest{
		WorkspaceDirectory: workspace, PatchType: "unified_diff", TargetRef: "main.txt", Diff: diff,
		BaseHash: sha256Hex([]byte(before)),
	})
	if err != nil {
		t.Fatalf("apply multi-hunk unified diff: %v", err)
	}
	if result.AddedLines != 2 || result.DeletedLines != 2 || result.AfterHash != preview.AfterHash {
		t.Fatalf("multi-hunk apply = %+v, preview=%+v", result, preview)
	}
	content, readErr := os.ReadFile(path)
	if readErr != nil || string(content) != "one\nTWO\nthree\nfour\nfive\nsix\nSEVEN\neight\n" {
		t.Fatalf("multi-hunk content = %q err=%v", string(content), readErr)
	}
}

func TestRuntimeAdapterPatchApplyUnifiedDiffRejectsStaleContext(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "main.txt"), []byte("one\nchanged\nthree\n"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	diff := "--- a/main.txt\n+++ b/main.txt\n@@ -1,3 +1,3 @@\n one\n-two\n+TWO\n three\n"

	_, err := NewRuntimeAdapter().ApplyPatch(PatchApplyRequest{
		WorkspaceDirectory: workspace, PatchType: "unified_diff", TargetRef: "main.txt", Diff: diff,
	})
	if err == nil {
		t.Fatalf("patch/apply unified diff should reject stale context")
	}
	content, readErr := os.ReadFile(filepath.Join(workspace, "main.txt"))
	if readErr != nil {
		t.Fatalf("read after rejected diff: %v", readErr)
	}
	if string(content) != "one\nchanged\nthree\n" {
		t.Fatalf("rejected diff changed file: %q", string(content))
	}
}

func TestRuntimeAdapterRuntimeFeasibilityRejectsStaleUnifiedDiff(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "main.txt"), []byte("one\nchanged\nthree\n"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	adapter := NewRuntimeAdapter()
	inventory, err := adapter.AcquireRuntimeOperationAdmissibility(workspace, "admissibility-file", []string{runtimecontract.RuntimeBlockKindPatchApply}, []string{"main.txt"}, 4)
	if err != nil {
		t.Fatal(err)
	}
	patchType, target, diff := "unified_diff", "main.txt", "--- a/main.txt\n+++ b/main.txt\n@@ -1,3 +1,3 @@\n one\n-two\n+TWO\n three\n"
	failure := adapter.ProbeRuntimeOperationFeasibility(workspace, inventory, runtimecontract.RuntimeOperation{
		ID: "stale-diff", Kind: runtimecontract.RuntimeBlockKindPatchApply, TargetRefs: []string{target},
		Parameters: []runtimecontract.OperationParameter{
			{Name: "patchType", String: &patchType}, {Name: "diff", String: &diff},
		},
	})
	if failure == nil || failure.Code != "unified_diff_apply_conflict" {
		t.Fatalf("failure = %+v, want unified_diff_apply_conflict", failure)
	}
	authorization := fileCandidateConformanceAuthorization(t, runtimecontract.RuntimeBlockKindPatchApply, runtimecontract.RuntimeOperationTargetPolicyExactTarget, target)
	violation := fileOperationCandidateApplicabilityViolation(runtimecontract.RuntimeOperation{
		ID: "stale-diff", Kind: runtimecontract.RuntimeBlockKindPatchApply, TargetRefs: []string{target},
		Parameters: []runtimecontract.OperationParameter{{Name: "patchType", String: &patchType}, {Name: "diff", String: &diff}},
	}, authorization, inventory, failure)
	if violation == nil || violation.ConstraintRef != "patch/unified_diff/diff/observed_target" ||
		violation.RequiredShape != runtimecontract.RuntimeOperationCandidateRequiredShapeUnifiedDiffApplicableToObservedTarget ||
		violation.ActualShape != runtimecontract.RuntimeOperationCandidateActualShapeObservedTargetContextMismatch ||
		violation.TargetRef != target || violation.ObservedRevision != inventory.ObservedRevision {
		t.Fatalf("candidate applicability violation = %+v", violation)
	}
	content, err := os.ReadFile(filepath.Join(workspace, "main.txt"))
	if err != nil || string(content) != "one\nchanged\nthree\n" {
		t.Fatalf("read-only feasibility changed file content=%q err=%v", string(content), err)
	}
}
