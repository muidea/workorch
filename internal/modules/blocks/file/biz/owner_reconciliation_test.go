package file

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/file/pkg/common"
	fileevents "github.com/muidea/workorch/internal/modules/blocks/file/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestFileOwnerReconciliationReportsMissingTargetWithoutInferringGoalSemantics(t *testing.T) {
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
	command := fileReconciliationQueryCommand(workspace, runtimecontract.RuntimeCWIReconciliationBindingRef{
		CoverageWorkItemID:    common.CoverageWorkItemID("cwi-file"),
		BindingID:             "binding-file-missing",
		BindingHash:           "binding-hash-file-missing",
		OwnerModuleIDs:        []string{modulecommon.ModuleID},
		RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange},
		AllowedOperationKinds: []string{runtimecontract.StepKindFileWrite},
		TargetRefs:            []string{"declared.go"},
	})
	result := sendFileReconciliationQuery(t, ctx, hub, command)
	if result.Observation.Status != runtimecontract.RuntimeCWIReconciliationObservationUnsatisfied {
		t.Fatalf("missing target status = %q, want unsatisfied", result.Observation.Status)
	}
	if result.Observation.OriginalDiagnostic == nil || result.Observation.OriginalDiagnostic.OriginalCode != "target_missing" {
		t.Fatalf("missing target diagnostic = %+v", result.Observation.OriginalDiagnostic)
	}
}

func TestFileOwnerReconciliationRejectsUnmappedArtifactReference(t *testing.T) {
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
	if err := os.WriteFile(filepath.Join(workspace, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	command := fileReconciliationQueryCommand(workspace, runtimecontract.RuntimeCWIReconciliationBindingRef{
		CoverageWorkItemID:    common.CoverageWorkItemID("cwi-file-artifact"),
		BindingID:             "binding-file-artifact",
		BindingHash:           "binding-hash-file-artifact",
		OwnerModuleIDs:        []string{modulecommon.ModuleID},
		RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation},
		AllowedOperationKinds: []string{runtimecontract.StepKindFileRead},
		ArtifactRefs:          []string{"workorch://workspace/main.go"},
	})
	result := sendFileReconciliationQuery(t, ctx, hub, command)
	if result.Observation.Status != runtimecontract.RuntimeCWIReconciliationObservationUnverifiable {
		t.Fatalf("unmapped artifact status = %q, want unverifiable", result.Observation.Status)
	}
	if result.Observation.OriginalDiagnostic == nil || result.Observation.OriginalDiagnostic.OriginalCode != "artifact_scope_unverifiable" {
		t.Fatalf("unmapped artifact diagnostic = %+v", result.Observation.OriginalDiagnostic)
	}
}

func TestFileOwnerReconciliationReplaysSealedProfileSelectorCandidates(t *testing.T) {
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
	if err := os.WriteFile(filepath.Join(workspace, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	command := fileReconciliationQueryCommand(workspace, runtimecontract.RuntimeCWIReconciliationBindingRef{
		CoverageWorkItemID: common.CoverageWorkItemID("cwi-file-selector"), BindingID: "binding-file-selector", BindingHash: "binding-hash-file-selector",
		OwnerModuleIDs: []string{modulecommon.ModuleID}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange},
		AllowedOperationKinds: []string{runtimecontract.StepKindFileWrite}, TargetSelectorRef: "file/profile_declared_candidates",
		TargetSelectorOwnerRef: modulecommon.ModuleID, TargetSelectorSubjectRef: "deliverable", TargetSelectorCandidateRefs: []string{"main.go"},
	})
	result := sendFileReconciliationQuery(t, ctx, hub, command)
	if result.Observation.Status != runtimecontract.RuntimeCWIReconciliationObservationSatisfied || len(result.Observation.TargetRefs) != 1 || result.Observation.TargetRefs[0] != "main.go" {
		t.Fatalf("profile selector observation = %+v", result.Observation)
	}
}

func TestFileOwnerReconciliationObservesRealizedProspectiveTargetWithoutReopeningCreateAdmission(t *testing.T) {
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
	const targetRef = "coverage-map.json"
	adapter := NewRuntimeAdapter()
	selectorBinding := runtimeTargetSelectionBinding{ownerRef: modulecommon.ModuleID, subjectRef: "workspace_deliverable"}
	admission, err := adapter.AcquireRuntimeTargetSelection(workspace, "target-selection-before-create", "semantic_proof_target", runtimeFileSelectorWorkspaceRelativeNewTarget, targetRef, 4, selectorBinding)
	if err != nil {
		t.Fatal(err)
	}
	if admission.Disposition != runtimecontract.RuntimeTargetSelectionCreate {
		t.Fatalf("pre-effect admission = %+v, want create", admission)
	}

	command := fileReconciliationQueryCommand(workspace, runtimecontract.RuntimeCWIReconciliationBindingRef{
		CoverageWorkItemID: common.CoverageWorkItemID("cwi-file-prospective"), BindingID: "binding-file-prospective", BindingHash: "binding-hash-file-prospective",
		OwnerModuleIDs: []string{modulecommon.ModuleID}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange},
		AllowedOperationKinds: []string{runtimecontract.StepKindFileWrite}, TargetSelectorRef: runtimeFileSelectorWorkspaceRelativeNewTarget,
		TargetSelectorOwnerRef: modulecommon.ModuleID, TargetSelectorSubjectRef: "workspace_deliverable", ProspectiveTargetRefs: []string{targetRef},
	})
	before := sendFileReconciliationQuery(t, ctx, hub, command)
	if before.Observation.Status != runtimecontract.RuntimeCWIReconciliationObservationUnsatisfied || len(before.Observation.TargetRefs) != 1 || before.Observation.TargetRefs[0] != targetRef {
		t.Fatalf("pre-effect reconciliation = %+v, want exact missing target", before.Observation)
	}

	content := "{\"covered\":true}\n"
	if _, err := adapter.ApplyPatch(PatchApplyRequest{
		WorkspaceDirectory: workspace,
		TargetRef:          targetRef,
		PatchType:          patchTypeFileWrite,
		Content:            content,
		ContentPresent:     true,
		CreateOnly:         true,
	}); err != nil {
		t.Fatal(err)
	}
	postEffectInfo, err := os.Stat(filepath.Join(workspace, targetRef))
	if err != nil {
		t.Fatal(err)
	}

	// Creation admission remains create-only: an already-realized prospective
	// target is still unsupported for a second file/write operation.
	admission, err = adapter.AcquireRuntimeTargetSelection(workspace, "target-selection-after-create", "semantic_proof_target", runtimeFileSelectorWorkspaceRelativeNewTarget, targetRef, 4, selectorBinding)
	if err != nil {
		t.Fatal(err)
	}
	if admission.Disposition != runtimecontract.RuntimeTargetSelectionUnsupported {
		t.Fatalf("post-effect admission = %+v, want unsupported", admission)
	}

	manifest, err := adapter.AcquireWorkspaceManifest(workspace)
	if err != nil {
		t.Fatal(err)
	}
	after := sendFileReconciliationQuery(t, ctx, hub, command)
	if after.Observation.Status != runtimecontract.RuntimeCWIReconciliationObservationSatisfied ||
		after.Observation.ObservedRevision != manifest.Revision || len(after.Observation.TargetRefs) != 1 || after.Observation.TargetRefs[0] != targetRef ||
		after.Observation.OriginalDiagnostic != nil {
		t.Fatalf("post-effect reconciliation = %+v, want current exact satisfied target", after.Observation)
	}
	duplicate := sendFileReconciliationQuery(t, ctx, hub, command)
	if !reflect.DeepEqual(duplicate.Observation, after.Observation) {
		t.Fatalf("duplicate reconciliation drifted: first=%+v duplicate=%+v", after.Observation, duplicate.Observation)
	}
	afterQueriesInfo, err := os.Stat(filepath.Join(workspace, targetRef))
	if err != nil {
		t.Fatal(err)
	}
	gotContent, err := os.ReadFile(filepath.Join(workspace, targetRef))
	if err != nil {
		t.Fatal(err)
	}
	if string(gotContent) != content || !afterQueriesInfo.ModTime().Equal(postEffectInfo.ModTime()) || afterQueriesInfo.Mode() != postEffectInfo.Mode() || afterQueriesInfo.Size() != postEffectInfo.Size() {
		t.Fatalf("read-only reconciliation changed target: content=%q before=%+v after=%+v", gotContent, postEffectInfo, afterQueriesInfo)
	}
}

func TestFileOwnerReconciliationClassifiesAmbiguousAndOutOfWorkspaceTargetsAsUnverifiable(t *testing.T) {
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
	for _, targetRef := range []string{"first.txt", "second.txt"} {
		if err := os.WriteFile(filepath.Join(workspace, targetRef), []byte(targetRef+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Run("ambiguous sealed selector", func(t *testing.T) {
		command := fileReconciliationQueryCommand(workspace, runtimecontract.RuntimeCWIReconciliationBindingRef{
			CoverageWorkItemID: common.CoverageWorkItemID("cwi-file-ambiguous"), BindingID: "binding-file-ambiguous", BindingHash: "binding-hash-file-ambiguous",
			OwnerModuleIDs: []string{modulecommon.ModuleID}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange},
			AllowedOperationKinds: []string{runtimecontract.StepKindFileWrite}, TargetSelectorRef: runtimeFileTargetSelectorProfileCandidates,
			TargetSelectorOwnerRef: modulecommon.ModuleID, TargetSelectorSubjectRef: "deliverable", TargetSelectorCandidateRefs: []string{"first.txt", "second.txt"},
		})
		result := sendFileReconciliationQuery(t, ctx, hub, command)
		if result.Observation.Status != runtimecontract.RuntimeCWIReconciliationObservationUnverifiable || result.Observation.OriginalDiagnostic == nil || result.Observation.OriginalDiagnostic.OriginalCode != "target_selection_ambiguous" {
			t.Fatalf("ambiguous selector observation = %+v", result.Observation)
		}
	})
	t.Run("out of workspace target", func(t *testing.T) {
		command := fileReconciliationQueryCommand(workspace, runtimecontract.RuntimeCWIReconciliationBindingRef{
			CoverageWorkItemID: common.CoverageWorkItemID("cwi-file-outside"), BindingID: "binding-file-outside", BindingHash: "binding-hash-file-outside",
			OwnerModuleIDs: []string{modulecommon.ModuleID}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange},
			AllowedOperationKinds: []string{runtimecontract.StepKindFileWrite}, TargetRefs: []string{"../escape.txt"},
		})
		result := sendFileReconciliationQuery(t, ctx, hub, command)
		if result.Observation.Status != runtimecontract.RuntimeCWIReconciliationObservationUnverifiable || result.Observation.OriginalDiagnostic == nil || result.Observation.OriginalDiagnostic.OriginalCode != "target_outside_workspace" {
			t.Fatalf("out-of-workspace observation = %+v", result.Observation)
		}
	})
	t.Run("prospective target outside profile candidates", func(t *testing.T) {
		command := fileReconciliationQueryCommand(workspace, runtimecontract.RuntimeCWIReconciliationBindingRef{
			CoverageWorkItemID: common.CoverageWorkItemID("cwi-file-prospective-outside-profile"), BindingID: "binding-file-prospective-outside-profile", BindingHash: "binding-hash-file-prospective-outside-profile",
			OwnerModuleIDs: []string{modulecommon.ModuleID}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange},
			AllowedOperationKinds: []string{runtimecontract.StepKindFileWrite}, TargetSelectorRef: runtimeFileTargetSelectorProfileCandidates,
			TargetSelectorOwnerRef: modulecommon.ModuleID, TargetSelectorSubjectRef: "deliverable", TargetSelectorCandidateRefs: []string{"first.txt"}, ProspectiveTargetRefs: []string{"second.txt"},
		})
		result := sendFileReconciliationQuery(t, ctx, hub, command)
		if result.Observation.Status != runtimecontract.RuntimeCWIReconciliationObservationUnverifiable || result.Observation.OriginalDiagnostic == nil || result.Observation.OriginalDiagnostic.OriginalCode != "prospective_target_outside_selector" {
			t.Fatalf("out-of-profile prospective observation = %+v", result.Observation)
		}
	})
	t.Run("explicit target cannot hide unauthorized prospective selector", func(t *testing.T) {
		command := fileReconciliationQueryCommand(workspace, runtimecontract.RuntimeCWIReconciliationBindingRef{
			CoverageWorkItemID: common.CoverageWorkItemID("cwi-file-unauthorized-prospective"), BindingID: "binding-file-unauthorized-prospective", BindingHash: "binding-hash-file-unauthorized-prospective",
			OwnerModuleIDs: []string{modulecommon.ModuleID}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange},
			AllowedOperationKinds: []string{runtimecontract.StepKindFileWrite}, TargetRefs: []string{"first.txt"},
			TargetSelectorRef: runtimeFileTargetSelectorUniqueWorkspaceEntry, ProspectiveTargetRefs: []string{"second.txt"},
		})
		result := sendFileReconciliationQuery(t, ctx, hub, command)
		if result.Observation.Status != runtimecontract.RuntimeCWIReconciliationObservationUnverifiable || result.Observation.OriginalDiagnostic == nil || result.Observation.OriginalDiagnostic.OriginalCode != "prospective_target_unsupported_for_selector" {
			t.Fatalf("hidden unauthorized prospective observation = %+v", result.Observation)
		}
	})
}

func TestFileOwnerReconciliationKeepsManifestExcludedProspectiveTargetsUnverifiable(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(32)
	t.Cleanup(func() { hub.Terminate(ctx) })
	owner, err := New(hub, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { owner.Teardown(ctx) })

	selectors := []string{
		runtimeFileSelectorWorkspaceRelativeNewTarget,
		runtimeFileTargetSelectorWorkspaceDeliverable,
		runtimeFileTargetSelectorProfileCandidates,
	}
	for _, selector := range selectors {
		for _, targetRef := range []string{"node_modules/generated.txt", "nested/.workorch-data/generated.txt"} {
			for _, present := range []bool{false, true} {
				state := "missing"
				if present {
					state = "present"
				}
				t.Run(selector+"/"+targetRef+"/"+state, func(t *testing.T) {
					workspace := t.TempDir()
					absolute := filepath.Join(workspace, filepath.FromSlash(targetRef))
					if present {
						if err := os.MkdirAll(filepath.Dir(absolute), 0o755); err != nil {
							t.Fatal(err)
						}
						if err := os.WriteFile(absolute, []byte("legacy effect\n"), 0o644); err != nil {
							t.Fatal(err)
						}
					}
					binding := runtimecontract.RuntimeCWIReconciliationBindingRef{
						CoverageWorkItemID: common.CoverageWorkItemID("cwi-file-excluded-prospective"), BindingID: "binding-file-excluded-prospective", BindingHash: "binding-hash-file-excluded-prospective",
						OwnerModuleIDs: []string{modulecommon.ModuleID}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange},
						AllowedOperationKinds: []string{runtimecontract.StepKindFileWrite}, TargetSelectorRef: selector,
						TargetSelectorOwnerRef: modulecommon.ModuleID, TargetSelectorSubjectRef: "workspace_deliverable", ProspectiveTargetRefs: []string{targetRef},
					}
					if selector == runtimeFileTargetSelectorProfileCandidates {
						binding.TargetSelectorCandidateRefs = []string{targetRef}
					}
					before, err := NewRuntimeAdapter().AcquireWorkspaceManifest(workspace)
					if err != nil {
						t.Fatal(err)
					}
					command := fileReconciliationQueryCommand(workspace, binding)
					first := sendFileReconciliationQuery(t, ctx, hub, command)
					duplicate := sendFileReconciliationQuery(t, ctx, hub, command)
					if first.Observation.Status != runtimecontract.RuntimeCWIReconciliationObservationUnverifiable ||
						first.Observation.OriginalDiagnostic == nil || first.Observation.OriginalDiagnostic.OriginalCode != "target_outside_manifest" {
						t.Fatalf("manifest-excluded reconciliation = %+v", first.Observation)
					}
					if !reflect.DeepEqual(first.Observation, duplicate.Observation) {
						t.Fatalf("duplicate manifest-excluded observation drifted: first=%+v duplicate=%+v", first.Observation, duplicate.Observation)
					}
					after, err := NewRuntimeAdapter().AcquireWorkspaceManifest(workspace)
					if err != nil {
						t.Fatal(err)
					}
					if !reflect.DeepEqual(before, after) {
						t.Fatalf("manifest-excluded reconciliation mutated workspace manifest: before=%+v after=%+v", before, after)
					}
					if !present {
						if _, statErr := os.Lstat(absolute); !os.IsNotExist(statErr) {
							t.Fatalf("manifest-excluded reconciliation created target %q: %v", targetRef, statErr)
						}
					} else if content, readErr := os.ReadFile(absolute); readErr != nil || string(content) != "legacy effect\n" {
						t.Fatalf("manifest-excluded reconciliation changed target %q: content=%q err=%v", targetRef, string(content), readErr)
					}
				})
			}
		}
	}
}

func TestFileOwnerReconciliationReloadsCurrentFactAfterOwnerRestartAndResourceDeletion(t *testing.T) {
	ctx := context.Background()
	workspace := t.TempDir()
	const targetRef = "restart-proof.txt"
	if err := os.WriteFile(filepath.Join(workspace, targetRef), []byte("sealed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	command := fileReconciliationQueryCommand(workspace, runtimecontract.RuntimeCWIReconciliationBindingRef{
		CoverageWorkItemID: common.CoverageWorkItemID("cwi-file-restart"), BindingID: "binding-file-restart", BindingHash: "binding-hash-file-restart",
		OwnerModuleIDs: []string{modulecommon.ModuleID}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange},
		AllowedOperationKinds: []string{runtimecontract.StepKindFileWrite}, TargetRefs: []string{targetRef},
	})

	queryWithFreshOwner := func() fileevents.QueryRuntimeCWIReconciliationResult {
		hub := magicevent.NewHub(16)
		owner, err := New(hub, nil)
		if err != nil {
			t.Fatal(err)
		}
		if err := owner.Start(); err != nil {
			t.Fatal(err)
		}
		value := sendFileReconciliationQuery(t, ctx, hub, command)
		owner.Teardown(ctx)
		hub.Terminate(ctx)
		return value
	}

	first := queryWithFreshOwner()
	second := queryWithFreshOwner()
	if first.Observation.Status != runtimecontract.RuntimeCWIReconciliationObservationSatisfied || !reflect.DeepEqual(first.Observation, second.Observation) {
		t.Fatalf("fresh-owner duplicate observation drifted: first=%+v second=%+v", first.Observation, second.Observation)
	}
	if err := os.Remove(filepath.Join(workspace, targetRef)); err != nil {
		t.Fatal(err)
	}
	missing := queryWithFreshOwner()
	if missing.Observation.Status != runtimecontract.RuntimeCWIReconciliationObservationUnsatisfied || len(missing.Observation.TargetRefs) != 1 || missing.Observation.TargetRefs[0] != targetRef || missing.Observation.OriginalDiagnostic == nil || missing.Observation.OriginalDiagnostic.OriginalCode != "target_missing" {
		t.Fatalf("post-delete fresh-owner observation = %+v", missing.Observation)
	}
	if missing.Observation.ObservedRevision == first.Observation.ObservedRevision {
		t.Fatalf("post-delete observation reused stale manifest revision %q", missing.Observation.ObservedRevision)
	}
}

func TestFileOwnerReconciliationSelectorCardinalityAndProspectiveMatrix(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(32)
	t.Cleanup(func() { hub.Terminate(ctx) })
	owner, err := New(hub, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { owner.Teardown(ctx) })

	type matrixCase struct {
		name           string
		selector       string
		prospective    string
		currentTargets []string
		wantStatus     runtimecontract.RuntimeCWIReconciliationObservationStatus
		wantTargetRefs []string
		wantDiagnostic string
	}
	cases := []matrixCase{
		// workspace-relative-new-file-target verifies one sealed path and is
		// intentionally independent of unrelated manifest cardinality.
		{name: "new/no-prospective/zero", selector: runtimeFileSelectorWorkspaceRelativeNewTarget, wantStatus: runtimecontract.RuntimeCWIReconciliationObservationUnverifiable, wantDiagnostic: "prospective_target_missing"},
		{name: "new/no-prospective/one", selector: runtimeFileSelectorWorkspaceRelativeNewTarget, currentTargets: []string{"one.txt"}, wantStatus: runtimecontract.RuntimeCWIReconciliationObservationUnverifiable, wantDiagnostic: "prospective_target_missing"},
		{name: "new/no-prospective/many", selector: runtimeFileSelectorWorkspaceRelativeNewTarget, currentTargets: []string{"one.txt", "two.txt"}, wantStatus: runtimecontract.RuntimeCWIReconciliationObservationUnverifiable, wantDiagnostic: "prospective_target_missing"},
		{name: "new/prospective/zero", selector: runtimeFileSelectorWorkspaceRelativeNewTarget, prospective: "prospective.txt", wantStatus: runtimecontract.RuntimeCWIReconciliationObservationUnsatisfied, wantTargetRefs: []string{"prospective.txt"}, wantDiagnostic: "target_missing"},
		{name: "new/prospective/one", selector: runtimeFileSelectorWorkspaceRelativeNewTarget, prospective: "prospective.txt", currentTargets: []string{"prospective.txt"}, wantStatus: runtimecontract.RuntimeCWIReconciliationObservationSatisfied, wantTargetRefs: []string{"prospective.txt"}},
		{name: "new/prospective/many", selector: runtimeFileSelectorWorkspaceRelativeNewTarget, prospective: "prospective.txt", currentTargets: []string{"one.txt", "prospective.txt"}, wantStatus: runtimecontract.RuntimeCWIReconciliationObservationSatisfied, wantTargetRefs: []string{"prospective.txt"}},

		// unique_workspace_entry never authorizes prospective creation.
		{name: "unique/no-prospective/zero", selector: runtimeFileTargetSelectorUniqueWorkspaceEntry, wantStatus: runtimecontract.RuntimeCWIReconciliationObservationUnsatisfied, wantDiagnostic: "target_selection_unsatisfied"},
		{name: "unique/no-prospective/one", selector: runtimeFileTargetSelectorUniqueWorkspaceEntry, currentTargets: []string{"one.txt"}, wantStatus: runtimecontract.RuntimeCWIReconciliationObservationSatisfied, wantTargetRefs: []string{"one.txt"}},
		{name: "unique/no-prospective/many", selector: runtimeFileTargetSelectorUniqueWorkspaceEntry, currentTargets: []string{"one.txt", "two.txt"}, wantStatus: runtimecontract.RuntimeCWIReconciliationObservationUnverifiable, wantDiagnostic: "target_selection_ambiguous"},
		{name: "unique/prospective/zero", selector: runtimeFileTargetSelectorUniqueWorkspaceEntry, prospective: "prospective.txt", wantStatus: runtimecontract.RuntimeCWIReconciliationObservationUnverifiable, wantDiagnostic: "prospective_target_unsupported_for_selector"},
		{name: "unique/prospective/one", selector: runtimeFileTargetSelectorUniqueWorkspaceEntry, prospective: "prospective.txt", currentTargets: []string{"prospective.txt"}, wantStatus: runtimecontract.RuntimeCWIReconciliationObservationUnverifiable, wantDiagnostic: "prospective_target_unsupported_for_selector"},
		{name: "unique/prospective/many", selector: runtimeFileTargetSelectorUniqueWorkspaceEntry, prospective: "prospective.txt", currentTargets: []string{"one.txt", "prospective.txt"}, wantStatus: runtimecontract.RuntimeCWIReconciliationObservationUnverifiable, wantDiagnostic: "prospective_target_unsupported_for_selector"},

		// workspace_deliverable is the complete bounded manifest scope. Multiple
		// entries are factual scope, not selector ambiguity.
		{name: "workspace/no-prospective/zero", selector: runtimeFileTargetSelectorWorkspaceDeliverable, wantStatus: runtimecontract.RuntimeCWIReconciliationObservationUnsatisfied, wantDiagnostic: "target_selection_unsatisfied"},
		{name: "workspace/no-prospective/one", selector: runtimeFileTargetSelectorWorkspaceDeliverable, currentTargets: []string{"one.txt"}, wantStatus: runtimecontract.RuntimeCWIReconciliationObservationSatisfied, wantTargetRefs: []string{"one.txt"}},
		{name: "workspace/no-prospective/many", selector: runtimeFileTargetSelectorWorkspaceDeliverable, currentTargets: []string{"one.txt", "two.txt"}, wantStatus: runtimecontract.RuntimeCWIReconciliationObservationSatisfied, wantTargetRefs: []string{"one.txt", "two.txt"}},
		{name: "workspace/prospective/zero", selector: runtimeFileTargetSelectorWorkspaceDeliverable, prospective: "prospective.txt", wantStatus: runtimecontract.RuntimeCWIReconciliationObservationUnsatisfied, wantTargetRefs: []string{"prospective.txt"}, wantDiagnostic: "target_missing"},
		{name: "workspace/prospective/one", selector: runtimeFileTargetSelectorWorkspaceDeliverable, prospective: "prospective.txt", currentTargets: []string{"prospective.txt"}, wantStatus: runtimecontract.RuntimeCWIReconciliationObservationSatisfied, wantTargetRefs: []string{"prospective.txt"}},
		{name: "workspace/prospective/many", selector: runtimeFileTargetSelectorWorkspaceDeliverable, prospective: "prospective.txt", currentTargets: []string{"one.txt", "prospective.txt"}, wantStatus: runtimecontract.RuntimeCWIReconciliationObservationSatisfied, wantTargetRefs: []string{"one.txt", "prospective.txt"}},

		// profile_declared_candidates remains a unique intersection. A sealed
		// prospective path is permitted only inside that declared set.
		{name: "profile/no-prospective/zero", selector: runtimeFileTargetSelectorProfileCandidates, wantStatus: runtimecontract.RuntimeCWIReconciliationObservationUnsatisfied, wantDiagnostic: "target_selection_unsatisfied"},
		{name: "profile/no-prospective/one", selector: runtimeFileTargetSelectorProfileCandidates, currentTargets: []string{"one.txt"}, wantStatus: runtimecontract.RuntimeCWIReconciliationObservationSatisfied, wantTargetRefs: []string{"one.txt"}},
		{name: "profile/no-prospective/many", selector: runtimeFileTargetSelectorProfileCandidates, currentTargets: []string{"one.txt", "two.txt"}, wantStatus: runtimecontract.RuntimeCWIReconciliationObservationUnverifiable, wantDiagnostic: "target_selection_ambiguous"},
		{name: "profile/prospective/zero", selector: runtimeFileTargetSelectorProfileCandidates, prospective: "prospective.txt", wantStatus: runtimecontract.RuntimeCWIReconciliationObservationUnsatisfied, wantTargetRefs: []string{"prospective.txt"}, wantDiagnostic: "target_missing"},
		{name: "profile/prospective/one", selector: runtimeFileTargetSelectorProfileCandidates, prospective: "prospective.txt", currentTargets: []string{"prospective.txt"}, wantStatus: runtimecontract.RuntimeCWIReconciliationObservationSatisfied, wantTargetRefs: []string{"prospective.txt"}},
		{name: "profile/prospective/many", selector: runtimeFileTargetSelectorProfileCandidates, prospective: "prospective.txt", currentTargets: []string{"one.txt", "prospective.txt"}, wantStatus: runtimecontract.RuntimeCWIReconciliationObservationUnverifiable, wantDiagnostic: "target_selection_ambiguous"},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			workspace := t.TempDir()
			for _, targetRef := range testCase.currentTargets {
				if err := os.WriteFile(filepath.Join(workspace, targetRef), []byte(targetRef+"\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			binding := runtimecontract.RuntimeCWIReconciliationBindingRef{
				CoverageWorkItemID: common.CoverageWorkItemID("cwi-file-selector-matrix"), BindingID: "binding-file-selector-matrix", BindingHash: "binding-hash-file-selector-matrix",
				OwnerModuleIDs: []string{modulecommon.ModuleID}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange},
				AllowedOperationKinds: []string{runtimecontract.StepKindFileWrite}, TargetSelectorRef: testCase.selector,
				TargetSelectorOwnerRef: modulecommon.ModuleID, TargetSelectorSubjectRef: "workspace_deliverable", ProspectiveTargetRefs: []string{testCase.prospective},
			}
			if testCase.selector == runtimeFileTargetSelectorProfileCandidates {
				binding.TargetSelectorCandidateRefs = []string{"one.txt", "prospective.txt", "two.txt"}
			}
			command := fileReconciliationQueryCommand(workspace, binding)
			manifestBefore, err := NewRuntimeAdapter().AcquireWorkspaceManifest(workspace)
			if err != nil {
				t.Fatal(err)
			}
			first := sendFileReconciliationQuery(t, ctx, hub, command)
			second := sendFileReconciliationQuery(t, ctx, hub, command)
			if first.Observation.Status != testCase.wantStatus || !reflect.DeepEqual(first.Observation.TargetRefs, testCase.wantTargetRefs) || first.Observation.ObservedRevision != manifestBefore.Revision {
				t.Fatalf("selector observation = %+v, want status=%q refs=%v revision=%q", first.Observation, testCase.wantStatus, testCase.wantTargetRefs, manifestBefore.Revision)
			}
			if !reflect.DeepEqual(first.Observation, second.Observation) {
				t.Fatalf("duplicate selector observation drifted: first=%+v second=%+v", first.Observation, second.Observation)
			}
			if testCase.wantDiagnostic == "" {
				if first.Observation.OriginalDiagnostic != nil {
					t.Fatalf("satisfied selector has diagnostic: %+v", first.Observation.OriginalDiagnostic)
				}
			} else if first.Observation.OriginalDiagnostic == nil || first.Observation.OriginalDiagnostic.OriginalCode != testCase.wantDiagnostic {
				t.Fatalf("selector diagnostic = %+v, want code %q", first.Observation.OriginalDiagnostic, testCase.wantDiagnostic)
			}
			manifestAfter, err := NewRuntimeAdapter().AcquireWorkspaceManifest(workspace)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(manifestBefore, manifestAfter) {
				t.Fatalf("selector reconciliation mutated workspace manifest: before=%+v after=%+v", manifestBefore, manifestAfter)
			}
		})
	}
}

func fileReconciliationQueryCommand(workspace string, binding runtimecontract.RuntimeCWIReconciliationBindingRef) fileevents.QueryRuntimeCWIReconciliationCommand {
	return fileevents.QueryRuntimeCWIReconciliationCommand{
		WorkspaceID:                   "ws-file-reconciliation",
		RunID:                         "run-file-reconciliation",
		RequestID:                     "request-file-reconciliation",
		Generation:                    1,
		DefinitionRevision:            "definition-file-reconciliation",
		BindingSetHash:                "binding-set-file-reconciliation",
		OwnerModuleID:                 modulecommon.ModuleID,
		Binding:                       binding,
		WorkspaceDeliverableDirectory: workspace,
		WorkspaceBaselineRevision:     "tree-file-reconciliation",
		WorkspaceBaselineRef:          "workspace-tree/tree-file-reconciliation",
	}
}

func sendFileReconciliationQuery(t *testing.T, ctx context.Context, hub magicevent.Hub, command fileevents.QueryRuntimeCWIReconciliationCommand) fileevents.QueryRuntimeCWIReconciliationResult {
	t.Helper()
	response := hub.Send(magicevent.NewEventWithContext(
		fileevents.TopicQueryRuntimeCWIReconciliation,
		runtimecontract.RuntimeModuleOrchestrator, modulecommon.ModuleID, magicevent.NewHeader(), ctx, command,
	))
	data, err := response.Get()
	if err != nil {
		t.Fatal(err)
	}
	result, ok := data.(fileevents.QueryRuntimeCWIReconciliationResult)
	if !ok {
		t.Fatalf("response type = %T", data)
	}
	return result
}
