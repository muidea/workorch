package shell

import (
	"go/parser"
	"go/token"
	"slices"
	"strings"
	"testing"

	modulecommon "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/common"
	"github.com/muidea/workorch/internal/pkg/common"
	rc "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestProjectAdditionalTestsShareInventoryAndReadinessRules(t *testing.T) {
	recipe, r, _ := conjunctiveSourceRequirement(t)
	recipe.AdapterRef = modulecommon.ValidationRecipeAdapterProjectMultiFailureDiagnostic
	checks, ok := shellValidationCheckAttestations(recipe, r)
	if !ok {
		t.Fatal("mapping")
	}
	root := t.TempDir()
	target := shellValidationNativeCheckTargetRef(r, checks...)
	source := "package checks\nimport \"testing\"\nfunc " + checks[0].NativeCheckRef + "(t *testing.T){t.Fatal(\"real check\")}\ntype helper struct{}\nfunc(helper) TestMethod(){}\nfunc Testhelper(){}\nfunc TestExtra(t *testing.T){t.Fatal(\"additive failure\")}\n"
	for _, check := range checks[1:] {
		source += "func " + check.NativeCheckRef + "(t *testing.T){t.Fatal(\"required\")}\n"
	}
	writeNativeStructureSource(t, root, "go.mod", "module fixture\ngo 1.26\n")
	writeNativeStructureSource(t, root, target, source)
	refs := []string{"go.mod", target}
	inspection := inspectShellNativeValidationChecks(root, refs, checks)
	if inspection.Code != "" {
		t.Fatalf("legal additive form rejected: %+v", inspection)
	}
	b, err := rc.RuntimeValidationProofRequirementBindingForRecipe(r, recipe.ID, checks...)
	if err != nil {
		t.Fatal(err)
	}
	subject, code := shellInputReview(root, refs, b)
	if code != "" || len(subject.DiagnosticSources) != len(checks)+1 {
		t.Fatalf("inventory differs: %s %+v", code, subject)
	}
	for _, suffix := range []string{"func init(){}", "func TestMain(m *testing.M){}"} {
		file, err := parser.ParseFile(token.NewFileSet(), target, source+suffix, 0)
		if err != nil {
			t.Fatal(err)
		}
		if shellConjunctiveInputDeclarations(file, checks) {
			t.Fatal("runner interception admitted")
		}
	}
	// The original required native check is still mandatory; the additive test
	// cannot substitute its identity or assertions.
	writeNativeStructureSource(t, root, target, strings.Replace(source, checks[0].NativeCheckRef, "helperOriginal", 1))
	if got := inspectShellNativeValidationChecks(root, refs, checks); got.Code != "native_check_missing" {
		t.Fatalf("lost required member: %+v", got)
	}
	requirements := shellProjectContentRequirements(recipe.AdapterRef)
	if strings.Contains(requirements, "16 files/64 KiB") || strings.Contains(requirements, "未声明 native Test") || !strings.Contains(requirements, "64 files/256 KiB") {
		t.Fatal("selected instructions contradict reader")
	}
	if !strings.Contains(requirements, shellProjectProofTopologyRequirements) || !strings.Contains(subject.ScopeRequirements, shellProjectProofTopologyRequirements) || !strings.Contains(subject.ScopeRequirements, shellNativeFailureObservationRequirements) {
		t.Fatal("generation and exact source review disagree on current proof responsibilities")
	}
}

func TestStructuralMaintenanceRetainsExactMalformedInputWithoutReview(t *testing.T) {
	recipe, r, checks := conjunctiveSourceRequirement(t)
	root := t.TempDir()
	target := shellValidationNativeCheckTargetRef(r, checks...)
	source := conjunctiveSourceInput(checks, "", "") + "func init() {}\n"
	writeNativeStructureSource(t, root, "go.mod", "module fixture\ngo 1.26\n")
	writeNativeStructureSource(t, root, target, source)
	refs := []string{"go.mod", target}
	manifest := shellTestWorkspaceManifestBinding("maintenance", refs)
	surfaces := shellValidationSurfaces([]rc.RuntimeValidationRecipe{recipe}, []rc.RuntimeValidationRecipe{recipe}, []rc.RuntimeValidationRecipe{recipe}, []rc.RuntimeValidationProofRequirement{r}, manifest, "policy", root)
	if len(surfaces) != 1 || len(surfaces[0].ProofRequirementUnavailabilities) != 1 {
		t.Fatalf("missing exact failure: %+v", surfaces)
	}
	u := surfaces[0].ProofRequirementUnavailabilities[0]
	if u.Maintenance == nil || u.Validate() != nil || len(u.Maintenance.TargetRefs) != 1 || u.Maintenance.TargetRefs[0] != target || u.Maintenance.Binding.InputReview != nil || len(surfaces[0].ProofRequirementBindings) != 0 {
		t.Fatalf("unbound or falsely ready maintenance: %+v", u)
	}
	found := false
	for _, f := range u.Maintenance.Files {
		if f.TargetRef == target {
			found = f.Content == source
		}
	}
	if !found {
		t.Fatal("original malformed bytes were changed")
	}
	for _, code := range []string{"native_check_analysis_limit", "native_check_source_out_of_scope", "native_check_protocol_unsupported", "native_check_source_unavailable"} {
		if shellValidationInputMaintenance(root, refs, shellNativeCheckInspection{Code: code, TargetRefs: []string{target}}, r, recipe, checks) != nil {
			t.Fatal("unavailable failure acquired authority", code)
		}
	}
}

func TestProjectMethodMaintenanceIncludesOnlyAffectedNeighbor(t *testing.T) {
	recipe, requirement, _ := conjunctiveSourceRequirement(t)
	recipe.AdapterRef = modulecommon.ValidationRecipeAdapterProjectMultiFailureDiagnostic
	checks, ok := shellValidationCheckAttestations(recipe, requirement)
	if !ok {
		t.Fatal("mapping")
	}
	root := t.TempDir()
	target := shellValidationNativeCheckTargetRef(requirement, checks...)
	current := conjunctiveSourceInput(checks, "", "")
	neighbor := "historical_test.go"
	original := "package fixture\nimport(\"os\";\"os/exec\";\"testing\")\nfunc TestHistorical(t *testing.T){ p,_:=os.Executable(); if err:=exec.Command(p).Run();err!=nil{t.Fatal(err)} }\n"
	writeNativeStructureSource(t, root, "go.mod", "module fixture\ngo 1.26\n")
	writeNativeStructureSource(t, root, target, current)
	writeNativeStructureSource(t, root, neighbor, original)
	// Another completed package is still executed, but its full body must not
	// consume the current CWI's independent semantic review/maintenance envelope.
	other := "workorch_validation/other/other_test.go"
	writeNativeStructureSource(t, root, other, "package other\nimport \"testing\"\nfunc TestOther(t *testing.T){t.Fatal(\"independent\")}\n/*"+strings.Repeat("x", rc.RuntimeValidationDiagnosticSourceMaxBytes)+"*/")
	refs := []string{"go.mod", target, neighbor, other}
	inspection := inspectShellNativeValidationChecks(root, refs, checks)
	if inspection.Code != "native_check_unmanaged_self_execution" || len(inspection.TargetRefs) != 1 || inspection.TargetRefs[0] != neighbor {
		t.Fatalf("lost executed neighbor method: %+v", inspection)
	}
	maintenance := shellValidationInputMaintenance(root, refs, inspection, requirement, recipe, checks)
	if maintenance == nil || maintenance.Validate(requirement, recipe.ID) != nil {
		t.Fatal("missing exact maintenance")
	}
	found := false
	for _, f := range maintenance.Files {
		if f.TargetRef == other {
			t.Fatal("unaffected history entered maintenance")
		}
		if f.TargetRef == neighbor {
			found = f.Content == original
		}
	}
	if !found {
		t.Fatal("missing original bytes")
	}
	binding, err := rc.RuntimeValidationProofRequirementBindingForRecipe(requirement, recipe.ID, checks...)
	if err != nil {
		t.Fatal(err)
	}
	subject, code := shellInputReview(root, refs, binding)
	if code != "" {
		t.Fatal(code)
	}
	for _, f := range subject.Files {
		if f.TargetRef == neighbor || f.TargetRef == other {
			t.Fatal("execution method expanded semantic review")
		}
	}
	// Pure postimage inspection uses exactly the same rule and frozen corpus.
	files := map[string]string{}
	var frozen []string
	for _, f := range maintenance.Files {
		files[f.TargetRef] = f.Content
		frozen = append(frozen, f.TargetRef)
	}
	read := func(ref string) ([]byte, error) { return []byte(files[ref]), nil }
	if got := inspectShellNativeValidationSources(frozen, checks, read); got.Code != inspection.Code {
		t.Fatalf("frozen rule drift: %+v", got)
	}
	files[neighbor] = "package fixture\nimport \"testing\"\nfunc TestHistorical(t *testing.T){t.Fatal(\"original criterion retained\")}\n"
	if got := inspectShellNativeValidationSources(frozen, checks, read); got.Code != "" {
		t.Fatalf("repaired method refused: %+v", got)
	}
	// A diagnostic cannot authorize a path absent from the observed inventory.
	if shellValidationInputMaintenance(root, refs, shellNativeCheckInspection{Code: inspection.Code, TargetRefs: []string{"unobserved_test.go"}}, requirement, recipe, checks) != nil {
		t.Fatal("unobserved target authorized")
	}
}

func TestProjectMethodMaintenanceProtectionCoversEachProjectRecipe(t *testing.T) {
	for _, adapter := range []string{modulecommon.ValidationRecipeAdapterProjectConjunction, modulecommon.ValidationRecipeAdapterProjectFailureDiagnostic, modulecommon.ValidationRecipeAdapterProjectMultiFailureDiagnostic} {
		t.Run(adapter, func(t *testing.T) {
			recipe, requirement, _ := conjunctiveSourceRequirement(t)
			recipe.AdapterRef = adapter
			checks, ok := shellValidationCheckAttestations(recipe, requirement)
			if !ok {
				t.Fatal("mapping")
			}
			root := t.TempDir()
			current := shellValidationNativeCheckTargetRef(requirement, checks...)
			writeNativeStructureSource(t, root, "go.mod", "module fixture\ngo 1.26\n")
			writeNativeStructureSource(t, root, current, conjunctiveSourceInput(checks, "", ""))
			neighbor := "historical_test.go"
			writeNativeStructureSource(t, root, neighbor, "package fixture\nimport(\"os\";\"testing\")\nfunc TestOriginal(t *testing.T){_,err:=os.Executable();if err!=nil{t.Fatal(err)}}\n")
			binding := shellTestWorkspaceManifestBinding("project-method", []string{"go.mod", current, neighbor})
			surfaces := shellValidationSurfaces([]rc.RuntimeValidationRecipe{recipe}, []rc.RuntimeValidationRecipe{recipe}, []rc.RuntimeValidationRecipe{recipe}, []rc.RuntimeValidationProofRequirement{requirement}, binding, "tree", root)
			if len(surfaces) != 1 || len(surfaces[0].ProofRequirementUnavailabilities) != 1 {
				t.Fatalf("lost method failure: %+v", surfaces)
			}
			m := surfaces[0].ProofRequirementUnavailabilities[0].Maintenance
			if m == nil || !slices.Contains(surfaces[0].InputTargetRefs, neighbor) || !slices.Equal(m.TargetRefs, []string{neighbor}) {
				t.Fatal("maintenance escaped protected actual execution inputs")
			}
		})
	}
}

func TestProjectNeighborMethodMaintenanceAllowsAbsentCurrentSubjectPackage(t *testing.T) {
	for _, tc := range []struct {
		name, body, fixed, wantCode string
	}{
		{
			name:     "neighbor uses runtime.Caller under trimpath",
			body:     "package other\nimport (\"os\";\"runtime\";\"testing\")\nfunc TestOther(t *testing.T){_,_,_,ok:=runtime.Caller(0);if !ok{_,_=os.Getwd();t.Fatal(\"fail\")}}\n",
			fixed:    "package other\nimport (\"os\";\"testing\")\nfunc TestOther(t *testing.T){if _,err:=os.Getwd();err!=nil{t.Fatal(err)}}\n",
			wantCode: "native_check_trimpath_caller_path",
		},
		{
			name:     "neighbor uses os.Executable",
			body:     "package other\nimport (\"os\";\"testing\")\nfunc TestOther(t *testing.T){if _,err:=os.Executable();err!=nil{t.Fatal(err)}}\n",
			fixed:    "package other\nimport \"testing\"\nfunc TestOther(t *testing.T){if false{t.Fatal(\"original\")}}\n",
			wantCode: "native_check_unmanaged_self_execution",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recipe, requirement, _ := conjunctiveSourceRequirement(t)
			recipe.AdapterRef = modulecommon.ValidationRecipeAdapterProjectMultiFailureArtifact
			requirement.ValidationRecipeID = recipe.ID
			requirement.Hash = rc.StableRuntimeValidationProofRequirementHash(requirement)
			root := t.TempDir()
			neighbor := "workorch_validation/project/2d63f640cf1d727b6bceef65dab88c74e2cb579c53f3ca7189917f62b3f8f572/input_test.go"
			writeNativeStructureSource(t, root, "go.mod", "module fixture\ngo 1.26\n")
			writeNativeStructureSource(t, root, neighbor, tc.body)
			// Note: current CWI's own package file does NOT exist on disk yet!
			refs := []string{"go.mod", neighbor}
			binding := shellTestWorkspaceManifestBinding("absent-subject-neighbor", refs)
			surfaces := shellValidationSurfaces([]rc.RuntimeValidationRecipe{recipe}, []rc.RuntimeValidationRecipe{recipe}, []rc.RuntimeValidationRecipe{recipe}, []rc.RuntimeValidationProofRequirement{requirement}, binding, "tree", root)
			if len(surfaces) != 1 || len(surfaces[0].ProofRequirementPreparations) != 0 || len(surfaces[0].ProofRequirementUnavailabilities) != 1 {
				t.Fatalf("unexpected surface state when neighbor method defect blocks uncreated CWI: %+v", surfaces)
			}
			u := surfaces[0].ProofRequirementUnavailabilities[0]
			if u.OriginalDiagnostic == nil || u.OriginalDiagnostic.OriginalCode != tc.wantCode {
				t.Fatalf("unexpected unavailability code: %+v", u.OriginalDiagnostic)
			}
			if u.Maintenance == nil || u.Maintenance.Validate(requirement, recipe.ID) != nil || !slices.Equal(u.Maintenance.TargetRefs, []string{neighbor}) {
				t.Fatalf("neighbor maintenance unavailable when current CWI subject is absent: %+v", u.Maintenance)
			}
			catalog := rc.RuntimeOperationAdmissibility{PrerequisiteID: "catalog", OwnerModuleID: ModuleID, ObservedRevision: "tree", OperationKinds: []string{rc.RuntimeBlockKindManagedValidationSession}, ValidationSurfaces: surfaces}
			catalog.SnapshotID = rc.StableRuntimeOperationAdmissibilitySnapshotID(catalog)
			observation := rc.RuntimePrerequisiteObservation{PrerequisiteID: "catalog", Kind: rc.PrerequisiteKindOperationAdmissibility, OwnerModuleID: ModuleID, ContentHash: catalog.SnapshotID, Dependencies: []rc.RuntimeFreshnessDependency{{Domain: rc.RuntimeResourceWorkspaceTree, OwnerModuleID: rc.RuntimeModuleFile, SubjectRef: "workspace_deliverable", Revision: "tree"}}}
			observation.ObservationID = rc.StableRuntimePrerequisiteObservationID(observation)
			facts := rc.RuntimeFacts{WorkspaceID: "ws-absent", RunID: "run-absent", DefinitionSeed: rc.RuntimeDefinitionSeed{Hash: "seed"}, Focus: rc.CoverageFocus{CoverageWorkItemIDs: []common.CoverageWorkItemID{requirement.CoverageWorkItemID}}, Resources: rc.RuntimeResourceFacts{OperationAdmissibilities: []rc.RuntimeOperationAdmissibility{catalog}, PrerequisiteObservations: []rc.RuntimePrerequisiteObservation{observation}}}
			revision, err := rc.CompileRuntimeOwnerInputRevision(facts)
			if err != nil {
				t.Fatalf("CompileRuntimeOwnerInputRevision failed for absent-subject neighbor maintenance: %v", err)
			}
			badPost := rc.RuntimeValidationInputReviewFile{TargetRef: neighbor, Content: tc.body + "// unfixed\n"}
			badPost.ContentHash = rc.RuntimeValidationInputContentHash(badPost.Content)
			badProof := rc.RuntimeOperationReplayProof{OwnerModuleID: rc.RuntimeModuleFile, OperationID: "edit", TargetRefs: []string{neighbor}, BeforeHash: rc.RuntimeValidationInputContentHash(tc.body), AfterHash: badPost.ContentHash, AdmissibilitySnapshotID: "file-catalog", AcquisitionBinding: "acquisition"}
			badReq := rc.RuntimeInputStructureRequest(string(facts.WorkspaceID), string(facts.RunID), rc.AuthorityTransaction{AuthoritySnapshotID: "authority"}, "manifest", rc.RuntimeOperation{ID: "edit", Kind: rc.StepKindFileEdit, TargetRefs: []string{neighbor}}, revision, badProof, badPost)
			if obs, err := inspectShellValidationInputPostimage(badReq); err != nil || obs.Code != tc.wantCode {
				t.Fatalf("unfixed neighbor postimage not rejected: %+v err=%v", obs, err)
			}
			goodPost := rc.RuntimeValidationInputReviewFile{TargetRef: neighbor, Content: tc.fixed, ContentHash: rc.RuntimeValidationInputContentHash(tc.fixed)}
			goodProof := rc.RuntimeOperationReplayProof{OwnerModuleID: rc.RuntimeModuleFile, OperationID: "edit", TargetRefs: []string{neighbor}, BeforeHash: rc.RuntimeValidationInputContentHash(tc.body), AfterHash: goodPost.ContentHash, AdmissibilitySnapshotID: "file-catalog", AcquisitionBinding: "acquisition"}
			goodReq := rc.RuntimeInputStructureRequest(string(facts.WorkspaceID), string(facts.RunID), rc.AuthorityTransaction{AuthoritySnapshotID: "authority"}, "manifest", rc.RuntimeOperation{ID: "edit", Kind: rc.StepKindFileEdit, TargetRefs: []string{neighbor}}, revision, goodProof, goodPost)
			if obs, err := inspectShellValidationInputPostimage(goodReq); err != nil || obs.Code != "" {
				t.Fatalf("fixed neighbor postimage rejected: %+v err=%v", obs, err)
			}
		})
	}
}
