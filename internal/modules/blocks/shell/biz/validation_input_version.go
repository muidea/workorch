package shell

import (
	"strings"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// Input version is encoded in the owner-native identity sealed in the existing
// proof binding. It is not inferred from current defaults during execution or
// replay, and does not create a separate Runtime state machine.
func shellVersionedNativeCheck(check runtimecontract.RuntimeValidationCheckAttestationBinding) bool {
	return check.ObservationProtocolRef == shellGoEntrypointObservationProtocolRef &&
		strings.HasPrefix(check.NativeCheckRef, "TestWorkOrchCWI_") && strings.HasSuffix(check.NativeCheckRef, "_InputV3")
}

func shellVersionedNativeCheckTarget(native string) string {
	return "workorch_validation/v3/" + strings.ToLower(native) + "_test.go"
}
