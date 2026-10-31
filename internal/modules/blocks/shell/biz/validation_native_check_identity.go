package shell

import (
	"strings"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// Native identities distinguish the review target and isolation scope sealed
// in the proof binding; they do not select a historical input format.
func shellReviewedNativeCheck(check runtimecontract.RuntimeValidationCheckAttestationBinding) bool {
	return shellProjectObservationProtocol(check.ObservationProtocolRef) && shellProjectNativePackage(check.NativeCheckRef) != "" ||
		check.ObservationProtocolRef == shellGoConjunctiveEntrypointObservationProtocolRef && shellConjunctiveNativePackage(check.NativeCheckRef) != "" ||
		shellIsolatedNativeCheck(check) || check.ObservationProtocolRef == shellGoEntrypointObservationProtocolRef &&
		strings.HasPrefix(check.NativeCheckRef, "TestWorkOrchCWI_") && strings.HasSuffix(check.NativeCheckRef, "_EntrypointInput")
}

func shellReviewedNativeCheckTarget(native string) string {
	if pkg := shellProjectNativePackage(native); pkg != "" {
		return pkg + "/input_test.go"
	}
	if pkg := shellConjunctiveNativePackage(native); pkg != "" {
		return pkg + "/input_test.go"
	}
	if shellIsolatedNativeIdentity.MatchString(native) {
		return shellIsolatedNativePackage(native) + "/input_test.go"
	}
	return "workorch_validation/entrypoint/" + strings.ToLower(native) + "_test.go"
}
