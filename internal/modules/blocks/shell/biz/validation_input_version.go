package shell

import (
	"strings"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// Input version is encoded in the owner-native identity sealed in the existing
// proof binding. It is not inferred from current defaults during execution or
// replay, and does not create a separate Runtime state machine.
func shellVersionedNativeCheck(check runtimecontract.RuntimeValidationCheckAttestationBinding) bool {
	return shellProjectObservationProtocol(check.ObservationProtocolRef) && shellProjectNativePackage(check.NativeCheckRef) != "" ||
		check.ObservationProtocolRef == shellGoConjunctiveEntrypointObservationProtocolRef && shellConjunctiveNativePackage(check.NativeCheckRef) != "" ||
		shellIsolatedNativeCheck(check) || check.ObservationProtocolRef == shellGoEntrypointObservationProtocolRef &&
		strings.HasPrefix(check.NativeCheckRef, "TestWorkOrchCWI_") && strings.HasSuffix(check.NativeCheckRef, "_InputV3")
}

func shellVersionedNativeCheckTarget(native string) string {
	if pkg := shellProjectNativePackage(native); pkg != "" {
		return pkg + "/input_test.go"
	}
	if pkg := shellConjunctiveNativePackage(native); pkg != "" {
		return pkg + "/input_test.go"
	}
	if shellIsolatedNativeIdentity.MatchString(native) {
		return shellIsolatedNativePackage(native) + "/input_test.go"
	}
	return "workorch_validation/v3/" + strings.ToLower(native) + "_test.go"
}
