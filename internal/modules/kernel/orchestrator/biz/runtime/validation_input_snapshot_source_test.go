package runtime

import (
	"strings"
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestReviewedInputSnapshotRequiresActualExactOwnerReads(t *testing.T) {
	input := runtimecontract.RuntimeValidationProofRequirementBinding{RequirementHash: "requirement", DefinitionRevision: "definition", CoverageWorkItemID: "cwi", DeclarationRef: "original-declaration", ValidationRecipeID: "recipe",
		RequiredCheckRefs: []string{"check"}, CheckAttestations: []runtimecontract.RuntimeValidationCheckAttestationBinding{{RequiredCheckRef: "check", ObservationProtocolRef: "native-protocol", NativeCheckRef: "native-check"}}}
	input.InputReview = &runtimecontract.RuntimeValidationInputReviewSubject{ProtocolRef: runtimecontract.RuntimeValidationInputReviewProtocol,
		Files: []runtimecontract.RuntimeValidationInputReviewFile{{TargetRef: "input.go", Content: "original input\n", ContentHash: runtimecontract.RuntimeValidationInputContentHash("original input\n")}}}
	input.InputReview.Hash = runtimecontract.StableRuntimeValidationInputReviewSubjectHash(*input.InputReview, input)
	set := runtimecontract.RuntimeOwnerRevisionSet{DefinitionRevision: "definition", Entries: []runtimecontract.RuntimeOwnerRevisionRef{{OwnerKind: "capability", OwnerInstanceID: "owner/ws", ResourceKind: string(runtimecontract.RuntimeResourceWorkspaceTarget), ResourceID: "input.go", Revision: input.InputReview.Files[0].ContentHash, DependencyHash: strings.Repeat("a", 64)}}}.WithHash()
	if err := validateRuntimeReviewedInputReads(input, set, "owner/ws"); err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"wrong-owner", "wrong-path", "different-bytes", "wrong-domain", "missing", "ambiguous", "wrong-definition"} {
		t.Run(scenario, func(t *testing.T) {
			bad := *runtimecontract.CloneRuntimeOwnerRevisionSet(&set)
			switch scenario {
			case "wrong-owner":
				bad.Entries[0].OwnerInstanceID = "another-owner/ws"
			case "wrong-path":
				bad.Entries[0].ResourceID = "same-bytes-elsewhere.go"
			case "different-bytes":
				bad.Entries[0].Revision = runtimecontract.RuntimeValidationInputContentHash("replaced input\n")
			case "wrong-domain":
				bad.Entries[0].ResourceKind = "another-domain"
			case "missing":
				bad.Entries = nil
			case "ambiguous":
				entry := bad.Entries[0]
				entry.DependencyHash = strings.Repeat("b", 64)
				bad.Entries = append(bad.Entries, entry)
			case "wrong-definition":
				bad.DefinitionRevision = "another-definition"
			}
			bad = bad.WithHash()
			if validateRuntimeReviewedInputReads(input, bad, "owner/ws") == nil {
				t.Fatal("self-rehashed read set replaced exact original validation input reads")
			}
		})
	}
}
