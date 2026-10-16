package shell

import (
	"context"
	"fmt"
	"slices"
	"testing"

	shellcommon "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// Native input performs real nested production commands and executes the CLI,
// as the public input does. It must survive owner instrumentation unchanged.
func TestProjectMultiFailureReviewedInputNestedCommands(t *testing.T) {
	root, internal := t.TempDir(), t.TempDir()
	recipe := isolatedInputRecipe()
	recipe.AdapterRef = shellcommon.ValidationRecipeAdapterProjectMultiFailureDiagnostic
	recipe.InputReviewProtocolRef = runtimecontract.RuntimeValidationInputReviewProtocolV3
	requirement := shellValidationProofRequirementForTest(t, recipe.ID, "nested-cwi")
	checks, ok := shellValidationCheckAttestations(recipe, requirement)
	if !ok {
		t.Fatal("no exact v8 attestation")
	}
	target := shellValidationNativeCheckTargetRef(requirement, checks...)
	input := fmt.Sprintf(`package checks
import ("testing"; "os/exec"; "path/filepath"; "strings"; delivery "fixture/delivery")
func %s(t *testing.T) {
 if delivery.Value()!=4 { t.Fatal("real implementation failed") }
 for _,args:=range [][]string{{"build","./..."},{"test","./delivery","./cmd/..."},{"test","-race","./delivery","./cmd/..."},{"vet","./delivery","./cmd/..."}} {
  command:=exec.Command("go",args...); command.Dir=%q
  if output,err:=command.CombinedOutput(); err!=nil { t.Fatalf("real project command %%v: %%v %%s",args,err,output) }
 }
 bin:=filepath.Join(t.TempDir(),"delivery")
 build:=exec.Command("go","build","-o",bin,"./cmd/delivery"); build.Dir=%q
 if output,err:=build.CombinedOutput(); err!=nil { t.Fatalf("build: %%v %%s",err,output) }
 output,err:=exec.Command(bin).Output(); if err!=nil || strings.TrimSpace(string(output))!="4" { t.Fatal("real CLI failed") }
}
`, checks[0].NativeCheckRef, root, root)
	files := map[string]string{"go.mod": "module fixture\ngo 1.26\n", "delivery/value.go": "package delivery\nfunc Value() int{return 4}\n", "delivery/value_test.go": "package delivery\nimport \"testing\"\nfunc TestProduction(t *testing.T){if Value()!=4{t.Fatal(\"wrong\")}}\n", "cmd/delivery/main.go": "package main\nimport(\"fmt\";\"fixture/delivery\")\nfunc main(){fmt.Println(delivery.Value())}\n", target: input}
	var refs []string
	for ref, body := range files {
		writeNativeStructureSource(t, root, ref, body)
		refs = append(refs, ref)
	}
	slices.Sort(refs)
	binding, err := runtimecontract.RuntimeValidationProofRequirementBindingForRecipe(requirement, recipe.ID, checks...)
	if err != nil {
		t.Fatal(err)
	}
	binding.InputReview, _ = shellVersionedInputReview(root, refs, binding)
	if binding.InputReview == nil {
		t.Fatal("missing independently observable input")
	}
	request := RuntimeCommandRequest{WorkspaceID: "ws", RunID: "run", StepID: "dispatch/op", WorkspaceDeliverableDirectory: root, WorkspaceInternalDirectory: internal, ValidationRecipeID: recipe.ID, ValidationRecipes: []runtimecontract.RuntimeValidationRecipe{recipe}, Command: recipe.Command, Args: recipe.Args, TimeoutMillis: 60000, TargetRefs: []string{target}, ValidationScopeTargetRefs: refs, ValidationCheckAttestations: checks, validationProofBinding: binding, ManagedValidation: runtimecontract.RuntimeManagedValidation{RuntimeManaged: true, Kind: runtimecontract.RuntimeBlockKindManagedValidationSession}}
	if err := instrumentManagedValidationNativeChecks(&request); err != nil {
		t.Fatal(err)
	}
	result, err := NewRuntimeAdapter().executeShellProjectScope(context.Background(), request, shellProjectRawScope{WorkspaceID: "ws", RunID: "run", ExecutionKey: "nested-real-input"})
	if err != nil || result.Conjunction == nil || !result.Conjunction.Complete {
		store, openErr := openShellProjectRawStore(internal)
		if openErr == nil {
			defer store.Close()
			for _, refs := range result.Raw {
				for stream, ref := range map[string]string{"stdout": refs.Stdout, "stderr": refs.Stderr} {
					if raw, readErr := store.Read(context.Background(), shellProjectRawScope{WorkspaceID: "ws", RunID: "run", ExecutionKey: "nested-real-input", StageID: refs.StageID, Stream: stream}, ref); readErr == nil {
						t.Logf("original owner %s/%s output: %s", refs.StageID, stream, raw.Raw)
					}
				}
			}
		}
		t.Fatalf("real nested input failed under exact instrumentation: %v", err)
	}
}
