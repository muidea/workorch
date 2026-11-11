package shell

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	modulecommon "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/common"
	"github.com/muidea/workorch/internal/pkg/common"
	rc "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func shellPostimageFixture(t *testing.T, corpusBytes ...int) (string, rc.RuntimeValidationInputStructureRequest, string) {
	t.Helper()
	recipe, requirement, _ := conjunctiveSourceRequirement(t)
	recipe.AdapterRef = modulecommon.ValidationRecipeAdapterProjectMultiFailureDiagnostic
	checks, ok := shellValidationCheckAttestations(recipe, requirement)
	if !ok {
		t.Fatal("checks")
	}
	root := t.TempDir()
	target := shellValidationNativeCheckTargetRef(requirement, checks...)
	good := conjunctiveSourceInput(checks, "", "") + "func TestExtra(t *testing.T){ t.Fatal(\"additive\") }\n"
	bad := good + "func init() {}\n"
	writeNativeStructureSource(t, root, "go.mod", "module fixture\ngo 1.26\n")
	writeNativeStructureSource(t, root, target, bad)
	refs := []string{"go.mod", target}
	if len(corpusBytes) != 0 {
		padding := corpusBytes[0] - len(bad) - len("module fixture\ngo 1.26\n")
		writeNativeStructureSource(t, root, "notes.txt", strings.Repeat("x", padding))
		refs = append(refs, "notes.txt")
	}
	manifest := shellTestWorkspaceManifestBinding("postimage", refs)
	surfaces := shellValidationSurfaces([]rc.RuntimeValidationRecipe{recipe}, []rc.RuntimeValidationRecipe{recipe}, []rc.RuntimeValidationRecipe{recipe}, []rc.RuntimeValidationProofRequirement{requirement}, manifest, "tree", root)
	catalog := rc.RuntimeOperationAdmissibility{PrerequisiteID: "catalog", OwnerModuleID: ModuleID, ObservedRevision: "tree", OperationKinds: []string{rc.RuntimeBlockKindManagedValidationSession}, ValidationSurfaces: surfaces}
	catalog.SnapshotID = rc.StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	if err := catalog.Validate(); err != nil {
		t.Fatal("catalog", err)
	}
	observation := rc.RuntimePrerequisiteObservation{PrerequisiteID: "catalog", Kind: rc.PrerequisiteKindOperationAdmissibility, OwnerModuleID: ModuleID, ContentHash: catalog.SnapshotID, Dependencies: []rc.RuntimeFreshnessDependency{{Domain: rc.RuntimeResourceWorkspaceTree, OwnerModuleID: rc.RuntimeModuleFile, SubjectRef: "workspace_deliverable", Revision: "tree"}}}
	observation.ObservationID = rc.StableRuntimePrerequisiteObservationID(observation)
	if err := observation.Validate(); err != nil {
		t.Fatal("observation", err)
	}
	facts := rc.RuntimeFacts{WorkspaceID: "ws-postimage", RunID: "run-postimage", DefinitionSeed: rc.RuntimeDefinitionSeed{Hash: "seed"}, Focus: rc.CoverageFocus{CoverageWorkItemIDs: []common.CoverageWorkItemID{requirement.CoverageWorkItemID}}, Resources: rc.RuntimeResourceFacts{OperationAdmissibilities: []rc.RuntimeOperationAdmissibility{catalog}, PrerequisiteObservations: []rc.RuntimePrerequisiteObservation{observation}}}
	revision, err := rc.CompileRuntimeOwnerInputRevision(facts)
	if err != nil {
		t.Fatal(err)
	}
	post := rc.RuntimeValidationInputReviewFile{TargetRef: target, Content: good, ContentHash: rc.RuntimeValidationInputContentHash(good)}
	proof := rc.RuntimeOperationReplayProof{OwnerModuleID: rc.RuntimeModuleFile, OperationID: "edit", TargetRefs: []string{target}, BeforeHash: rc.RuntimeValidationInputContentHash(bad), AfterHash: post.ContentHash, AdmissibilitySnapshotID: "file-catalog", AcquisitionBinding: "file-acquisition"}
	request := rc.RuntimeInputStructureRequest(string(facts.WorkspaceID), string(facts.RunID), rc.AuthorityTransaction{AuthoritySnapshotID: "authority"}, "manifest", rc.RuntimeOperation{ID: "edit", Kind: rc.StepKindFileEdit, TargetRefs: []string{target}}, revision, proof, post)
	if err := request.Validate(); err != nil {
		t.Fatal(err)
	}
	return root, request, good
}

func TestInputPostimageCorpusCapacityIsOwnerConstraintNotIntegrityFailure(t *testing.T) {
	root, request, good := shellPostimageFixture(t, rc.RuntimeValidationDiagnosticSourceMaxBytes-152)
	capacity := request.Revision.Capacity()
	if capacity.Bytes != 262144-152 || capacity.RemainingBytes != 152 || capacity.FileCount != 3 || len(capacity.Targets) != 1 {
		t.Fatalf("lost exact complete corpus capacity: %+v", capacity)
	}
	before, err := os.ReadFile(filepath.Join(root, request.Postimage.TargetRef))
	if err != nil {
		t.Fatal(err)
	}
	for _, delta := range []int{-1, 0, 1, 3786} {
		t.Run(fmt.Sprint(delta), func(t *testing.T) {
			r := request
			size := capacity.Targets[0].MaxPostimageBytes + delta
			// UTF-8 bytes, not characters or estimated tokens.
			r.Postimage.Content = good + "/*字" + strings.Repeat("x", size-len(good)-7) + "*/"
			r.Postimage.ContentHash = rc.RuntimeValidationInputContentHash(r.Postimage.Content)
			r.Proof.AfterHash = r.Postimage.ContentHash
			if err := r.Validate(); err != nil {
				t.Fatal("complete transport rejected before owner observation", err)
			}
			fact, err := inspectShellValidationInputPostimage(r)
			if err != nil || fact.ValidateForRequest(r) != nil {
				t.Fatal("lost formal exact owner fact", err)
			}
			if delta > 0 && fact.Code != "validation_input_review_capacity_exceeded" || delta <= 0 && fact.Code != "" {
				t.Fatalf("wrong capacity classification delta=%d code=%s", delta, fact.Code)
			}
			raw, _ := json.Marshal(r)
			var restored rc.RuntimeValidationInputStructureRequest
			if err := json.Unmarshal(raw, &restored); err != nil {
				t.Fatal(err)
			}
			again, err := inspectShellValidationInputPostimage(restored)
			if err != nil || !reflect.DeepEqual(fact, again) {
				t.Fatal("recovery changed owner outcome", err)
			}
			bad := r
			bad.Postimage.Content += "tamper"
			if _, err := inspectShellValidationInputPostimage(bad); err == nil {
				t.Fatal("integrity failure became correctable constraint")
			}
		})
	}
	after, _ := os.ReadFile(filepath.Join(root, request.Postimage.TargetRef))
	if string(before) != string(after) {
		t.Fatal("constraint inspection executed a write")
	}
}

func TestInputPostimageUsesSameRulesWithoutWriting(t *testing.T) {
	root, request, good := shellPostimageFixture(t)
	before, _ := os.ReadFile(filepath.Join(root, request.Postimage.TargetRef))
	for _, scenario := range []string{"valid", "syntax", "interception", "missing", "signature", "helper-no-assertion"} {
		t.Run(scenario, func(t *testing.T) {
			r := request
			switch scenario {
			case "syntax":
				r.Postimage.Content += "func broken("
			case "interception":
				r.Postimage.Content += "func init(){}\n"
			case "missing":
				r.Postimage.Content = strings.ReplaceAll(good, r.Revision.Binding.CheckAttestations[0].NativeCheckRef, "helperOriginal")
			case "signature":
				r.Postimage.Content = strings.Replace(good, "(t *testing.T)", "()", 1)
			case "helper-no-assertion":
				r.Postimage.Content = strings.ReplaceAll(good, "t.Fatal(", "unreachable(")
			}
			r.Postimage.ContentHash = rc.RuntimeValidationInputContentHash(r.Postimage.Content)
			r.Proof.AfterHash = r.Postimage.ContentHash
			fact, err := inspectShellValidationInputPostimage(r)
			if err != nil || fact.ValidateForRequest(r) != nil {
				t.Fatalf("invalid factual response: %v", err)
			}
			if (fact.Code == "") != (scenario == "valid") {
				t.Fatalf("wrong structure: %+v", fact)
			}
			// Actual filesystem and pure frozen corpus must produce the same code.
			dir := t.TempDir()
			for _, f := range r.Revision.InputFiles() {
				body := f.Content
				if f.TargetRef == r.Postimage.TargetRef {
					body = r.Postimage.Content
				}
				writeNativeStructureSource(t, dir, f.TargetRef, body)
			}
			var refs []string
			for _, f := range r.Revision.InputFiles() {
				refs = append(refs, f.TargetRef)
			}
			disk := inspectShellNativeValidationChecks(dir, refs, r.Revision.Binding.CheckAttestations)
			if disk.Code != fact.Code || disk.CheckRef != fact.CheckRef || !reflect.DeepEqual(disk.TargetRefs, fact.TargetRefs) {
				t.Fatal("write-before/readiness rules diverged", disk, fact)
			}
			raw, _ := json.Marshal(r)
			var restored rc.RuntimeValidationInputStructureRequest
			if json.Unmarshal(raw, &restored) != nil {
				t.Fatal("reload")
			}
			again, err := inspectShellValidationInputPostimage(restored)
			if err != nil || !reflect.DeepEqual(fact, again) {
				t.Fatal("replay changed conclusion", err)
			}
		})
	}
	after, _ := os.ReadFile(filepath.Join(root, request.Postimage.TargetRef))
	if string(before) != string(after) {
		t.Fatal("inspection wrote live source")
	}
}

func TestNeighborPostimageRepairsOnlyExactObservedMethod(t *testing.T) {
	recipe, requirement, _ := conjunctiveSourceRequirement(t)
	recipe.AdapterRef = modulecommon.ValidationRecipeAdapterProjectMultiFailureDiagnostic
	checks, ok := shellValidationCheckAttestations(recipe, requirement)
	if !ok {
		t.Fatal("mapping")
	}
	root := t.TempDir()
	current := shellValidationNativeCheckTargetRef(requirement, checks...)
	neighbor := "historical_test.go"
	original := "package fixture\nimport(\"os\";\"testing\")\nfunc TestOriginal(t *testing.T){_,err:=os.Executable();if err!=nil{t.Fatal(err)}}\n"
	repaired := "package fixture\nimport \"testing\"\nfunc TestOriginal(t *testing.T){t.Fatal(\"original criterion\")}\n"
	writeNativeStructureSource(t, root, "go.mod", "module fixture\ngo 1.26\n")
	writeNativeStructureSource(t, root, current, conjunctiveSourceInput(checks, "", "")+"func init(){}\n")
	writeNativeStructureSource(t, root, neighbor, original)
	binding := shellTestWorkspaceManifestBinding("neighbor", []string{"go.mod", current, neighbor})
	surfaces := shellValidationSurfaces([]rc.RuntimeValidationRecipe{recipe}, []rc.RuntimeValidationRecipe{recipe}, []rc.RuntimeValidationRecipe{recipe}, []rc.RuntimeValidationProofRequirement{requirement}, binding, "tree", root)
	catalog := rc.RuntimeOperationAdmissibility{PrerequisiteID: "catalog", OwnerModuleID: ModuleID, ObservedRevision: "tree", OperationKinds: []string{rc.RuntimeBlockKindManagedValidationSession}, ValidationSurfaces: surfaces}
	catalog.SnapshotID = rc.StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	observation := rc.RuntimePrerequisiteObservation{PrerequisiteID: "catalog", Kind: rc.PrerequisiteKindOperationAdmissibility, OwnerModuleID: ModuleID, ContentHash: catalog.SnapshotID, Dependencies: []rc.RuntimeFreshnessDependency{{Domain: rc.RuntimeResourceWorkspaceTree, OwnerModuleID: rc.RuntimeModuleFile, SubjectRef: "workspace_deliverable", Revision: "tree"}}}
	observation.ObservationID = rc.StableRuntimePrerequisiteObservationID(observation)
	facts := rc.RuntimeFacts{WorkspaceID: "ws-neighbor", RunID: "run-neighbor", DefinitionSeed: rc.RuntimeDefinitionSeed{Hash: "seed"}, Focus: rc.CoverageFocus{CoverageWorkItemIDs: []common.CoverageWorkItemID{requirement.CoverageWorkItemID}}, Resources: rc.RuntimeResourceFacts{OperationAdmissibilities: []rc.RuntimeOperationAdmissibility{catalog}, PrerequisiteObservations: []rc.RuntimePrerequisiteObservation{observation}}}
	revision, err := rc.CompileRuntimeOwnerInputRevision(facts)
	if err != nil {
		t.Fatal(err)
	}
	post := rc.RuntimeValidationInputReviewFile{TargetRef: neighbor, Content: repaired, ContentHash: rc.RuntimeValidationInputContentHash(repaired)}
	proof := rc.RuntimeOperationReplayProof{OwnerModuleID: rc.RuntimeModuleFile, OperationID: "edit", TargetRefs: []string{neighbor}, BeforeHash: rc.RuntimeValidationInputContentHash(original), AfterHash: post.ContentHash, AdmissibilitySnapshotID: "file-catalog", AcquisitionBinding: "acquisition"}
	request := rc.RuntimeInputStructureRequest(string(facts.WorkspaceID), string(facts.RunID), rc.AuthorityTransaction{AuthoritySnapshotID: "authority"}, "manifest", rc.RuntimeOperation{ID: "edit", Kind: rc.StepKindFileEdit, TargetRefs: []string{neighbor}}, revision, proof, post)
	valid, err := inspectShellValidationInputPostimage(request)
	if err != nil || valid.Code != "" {
		t.Fatalf("neighbor edit incorrectly required unrelated original defect repair: %+v %v", valid, err)
	}
	// Recovery consumes the same frozen source, independently of later disk state.
	raw, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	var restored rc.RuntimeValidationInputStructureRequest
	if err = json.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	again, err := inspectShellValidationInputPostimage(restored)
	if err != nil || !reflect.DeepEqual(valid, again) {
		t.Fatal("frozen method recovery drift", err)
	}
	// Same target's forbidden method is still rejected, with no mutation/credit.
	request.Postimage.Content = original + "// another image\n"
	request.Postimage.ContentHash = rc.RuntimeValidationInputContentHash(request.Postimage.Content)
	request.Proof.AfterHash = request.Postimage.ContentHash
	invalid, err := inspectShellValidationInputPostimage(request)
	if err != nil || invalid.Code != "native_check_unmanaged_self_execution" {
		t.Fatalf("unrepaired method admitted: %+v %v", invalid, err)
	}
	// Fresh owner observation still reports the independent current-package defect.
	writeNativeStructureSource(t, root, neighbor, repaired)
	if got := inspectShellNativeValidationChecks(root, binding.Manifest.TargetRefs, checks); got.Code != "native_check_versioned_contract_invalid" {
		t.Fatalf("other defect disappeared: %+v", got)
	}
}
