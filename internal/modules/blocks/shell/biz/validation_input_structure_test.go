package shell

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	modulecommon "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/common"
	"github.com/muidea/workorch/internal/pkg/common"
	rc "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func shellPostimageFixture(t *testing.T) (string, rc.RuntimeValidationInputStructureRequest, string) {
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
