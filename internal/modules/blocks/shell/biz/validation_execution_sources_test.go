package shell

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	modulecommon "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/common"
	"github.com/muidea/workorch/internal/pkg/common"
	rc "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestSequentialCWIReviewScopePreservesAllProjectExecutions(t *testing.T) {
	root := t.TempDir()
	writeNativeStructureSource(t, root, "go.mod", "module fixture\ngo 1.26.7\n")
	writeNativeStructureSource(t, root, "delivery/value.go", "package delivery\nfunc Value()int{return 4}\n")
	refs := []string{"go.mod", "delivery/value.go"}
	recipe := isolatedInputRecipe()
	recipe.AdapterRef = modulecommon.ValidationRecipeAdapterProjectMultiFailureDiagnostic
	recipe.InputReviewProtocolRef = rc.RuntimeValidationInputReviewProtocol
	var previous rc.RuntimeValidationProofRequirementBinding
	var original *rc.RuntimeValidationInputReviewSubject
	for ordinal := 0; ordinal < 2; ordinal++ {
		requirement := shellValidationProofRequirementForTest(t, recipe.ID, common.CoverageWorkItemID(fmt.Sprintf("cwi-%d", ordinal)))
		checks, ok := shellValidationCheckAttestations(recipe, requirement)
		if !ok {
			t.Fatal("missing native mapping")
		}
		binding, err := rc.RuntimeValidationProofRequirementBindingForRecipe(requirement, recipe.ID, checks...)
		if err != nil {
			t.Fatal(err)
		}
		target := shellValidationNativeCheckTargetRef(requirement, checks...)
		body := fmt.Sprintf("package checks\nimport(\"testing\";\"os\";\"os/exec\";delivery \"fixture/delivery\")\n%s\nfunc %s(t *testing.T){if managedNativeSource(t){return};if delivery.Value()!=4{t.Fatal(\"value is not four\")}}\n/*%s*/\n", managedNativeSourceTestHelper, checks[0].NativeCheckRef, strings.Repeat("x", 140000))
		writeNativeStructureSource(t, root, target, body)
		refs = append(refs, target)
		slices.Sort(refs)
		binding.InputReview, _ = shellInputReview(root, refs, binding)
		if binding.InputReview == nil || len(binding.InputReview.Files) != 2 || len(binding.InputReview.DiagnosticSources) != 1 {
			t.Fatal("review did not retain exactly the complete applicable package and module input")
		}
		if original != nil {
			reobserved, code := shellInputReview(root, refs, previous)
			if code != "" || !reflect.DeepEqual(original, reobserved) {
				t.Fatal("adding an unrelated CWI changed the prior semantic subject", code)
			}
		}
		original, previous = binding.InputReview, binding
		request := RuntimeCommandRequest{WorkspaceID: "ws-sequential", RunID: "run-sequential", StepID: common.StepID("dispatch/" + fmt.Sprint(ordinal)),
			WorkspaceDeliverableDirectory: root, WorkspaceInternalDirectory: t.TempDir(), ValidationRecipeID: recipe.ID, ValidationRecipes: []rc.RuntimeValidationRecipe{recipe},
			Command: recipe.Command, Args: slices.Clone(recipe.Args), TimeoutMillis: 60000, TargetRefs: []string{target}, ValidationScopeTargetRefs: slices.Clone(refs),
			ValidationCheckAttestations: checks, validationProofBinding: binding, ManagedValidation: rc.RuntimeManagedValidation{RuntimeManaged: true, Kind: rc.RuntimeBlockKindManagedValidationSession}}
		if err := instrumentManagedValidationNativeChecks(&request); err != nil {
			t.Fatal(err)
		}
		execution, err := NewRuntimeAdapter().executeShellProjectScope(context.Background(), request, shellProjectRawScope{WorkspaceID: request.WorkspaceID, RunID: request.RunID, ExecutionKey: "operation-" + fmt.Sprint(ordinal)})
		if err != nil || execution.Conjunction == nil || !execution.Conjunction.Complete || len(execution.Conjunction.NativeSources) != ordinal+1 {
			t.Fatalf("sequential execution %d: %v result=%+v", ordinal, err, execution.Conjunction)
		}
		if err := rc.ValidateRuntimeValidationDiagnosticExecution(*execution.Result.Evidence.Validation, binding, refs); err != nil {
			t.Fatal(err)
		}
		for _, mutate := range []func(*rc.RuntimeValidationObservation){
			func(o *rc.RuntimeValidationObservation) {
				for i := range o.ExecutionConjunction.NativeSources {
					source := &o.ExecutionConjunction.NativeSources[i]
					if source.InputTargetRef == target {
						source.InputContentHash = rc.RuntimeValidationInputContentHash("different input")
					}
				}
			},
			func(o *rc.RuntimeValidationObservation) {
				o.ExecutionConjunction.NativeSources[0].InputTargetRef = "outside/sealed/scope"
			},
		} {
			changed := execution.Result.Evidence.Validation.Clone()
			mutate(&changed)
			if rc.ValidateRuntimeValidationDiagnosticExecution(changed, binding, refs) == nil {
				t.Fatal("successful execution admitted altered input provenance")
			}
		}
		for _, stage := range execution.Conjunction.Stages[1:3] {
			if stage.Tests != ordinal+1 || len(stage.NativeExecutions) != ordinal+1 {
				t.Fatal("fresh root test/race omitted a historical source")
			}
		}
	}
}
