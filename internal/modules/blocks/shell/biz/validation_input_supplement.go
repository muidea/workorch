package shell

import (
	"strings"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

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
