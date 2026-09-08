package shell

import (
	"strings"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// This declaration and the native-check inspector form one Shell-owned
// contract. The bounded prose is public input syntax, never parser diagnostics.
const shellSupplementContentContractRef = "shell/go-test-input/single-conjunctive-test/v1"
const shellSupplementContentRequirements = "Create one Go test source file with a package clause, imports, and exactly one top-level function named nativeCheckRef with signature func(t *testing.T). No other top-level declarations: no helper functions, methods, const, var, type, init, TestMain, or original-check redefinition. Put constants, variables, types and any helper closures inside that test function. Place testing assertions directly in the test body or a direct t.Run callback, not solely in helper closures. Call the actual workspace delivery implementation; source-text checks, test-local replacement implementations, dependency-only calls and subprocess-only execution do not establish workspace behavior under this observation protocol. Preserve originalNativeCheckRef and all existing validation inputs; both original and supplemental checks must execute and pass."

func shellValidationInputAddition(check runtimecontract.RuntimeValidationCheckAttestationBinding) runtimecontract.RuntimeValidationInputAddition {
	return runtimecontract.RuntimeValidationInputAddition{
		RequiredCheckRef: check.RequiredCheckRef, OriginalNativeCheckRef: check.NativeCheckRef,
		NativeCheckRef: shellSupplementCheckRef(check.NativeCheckRef), TargetRef: shellSupplementTargetRef(check.NativeCheckRef),
		ContentContractRef: shellSupplementContentContractRef, ContentRequirements: shellSupplementContentRequirements,
	}
}

// The original native input is retained. A separately created, CWI-specific
// supplement is conjunctive and selected only from the fresh owner manifest.
// No receipt, old binding, Definition or assertion source is rewritten here.
func shellSupplementCheckRef(native string) string { return native + "_Behavior" }

func shellSupplementTargetRef(native string) string {
	return "workorch_validation/behavior_" + strings.ToLower(strings.TrimPrefix(native, "TestWorkOrchCWI_")) + "_test.go"
}

func shellValidationSupplementAttestations(values []runtimecontract.RuntimeValidationCheckAttestationBinding, refs []string) []runtimecontract.RuntimeValidationCheckAttestationBinding {
	values = runtimecontract.CloneRuntimeValidationCheckAttestationBindings(values)
	supplemented := false
	for i := range values {
		if values[i].ObservationProtocolRef != shellGoBehaviorObservationProtocolRef {
			continue
		}
		if shellStringIncluded(refs, shellSupplementTargetRef(values[i].NativeCheckRef)) {
			values[i].SupplementalNativeCheckRef = shellSupplementCheckRef(values[i].NativeCheckRef)
			supplemented = true
		}
	}
	if supplemented {
		for i := range values {
			values[i].ObservationProtocolRef = shellGoSupplementObservationProtocolRef
		}
	}
	return values
}

// The same bounded AST analysis checks every required input independently.
func shellFlattenNativeCheckAttestations(values []runtimecontract.RuntimeValidationCheckAttestationBinding) []runtimecontract.RuntimeValidationCheckAttestationBinding {
	var result []runtimecontract.RuntimeValidationCheckAttestationBinding
	for _, value := range runtimecontract.NormalizeRuntimeValidationCheckAttestationBindings(values) {
		for _, ref := range value.NativeCheckRefs() {
			copy := value
			copy.NativeCheckRef, copy.SupplementalNativeCheckRef = ref, ""
			result = append(result, copy)
		}
	}
	return result
}
