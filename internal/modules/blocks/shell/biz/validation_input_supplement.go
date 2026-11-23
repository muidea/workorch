package shell

import (
	"strings"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// This declaration and the native-check inspector form one Shell-owned
// contract. The bounded prose is public input syntax, never parser diagnostics.
const shellSupplementContentContractRef = "shell/go-test-input/single-conjunctive-test/v2"
const shellSupplementContentRequirements = "Create one Go test source file with a package clause, imports, and exactly one top-level function named nativeCheckRef with signature func(t *testing.T). No other top-level declarations: no helper functions, methods, const, var, type, init, TestMain, or original-check redefinition. Put constants, variables, types and any helper closures inside that test function. Place testing assertions directly in the test body or a direct t.Run callback, not solely in helper closures. Call the actual workspace delivery implementation; source-text checks, test-local replacement implementations, dependency-only calls and subprocess-only execution do not establish workspace behavior under this observation protocol. Preserve originalNativeCheckRef and all existing validation inputs; both original and supplemental checks must execute and pass."

const shellEntrypointContentContractRef = "shell/go-test-input/conjunctive-entrypoint-test/v3"
const shellEntrypointContentRequirements = "Create one Go test source file with package/imports and the exact nativeCheckRef func(t *testing.T). Named same-package helpers, types, constants and data-only variables are allowed; no init, TestMain, methods, other Test functions, original-check redefinition or function calls/closures in global initializers. Assertions must be reachable through the real testing.T, directly, in t.Run or named helpers. Exercise the actual workspace Go delivery, either in-process or via a Go-built executable inheriting the owner GOFLAGS and GOCOVERDIR; do not override instrumentation, use prebuilt binaries, exit the test runner, mutate validation inputs or substitute source strings/test-local/dependency-only behavior. Non-Go backend processes are allowed but do not themselves prove Go delivery execution. Assert the actual protocol responses and every required language/criterion, reporting missing dependencies as unverified, never passed. Preserve all original checks and inputs; original and supplement must execute and pass together."

const shellProjectSupplementContentContractRef = "shell/go-test-input/project-supplement/v1"
const shellProjectSupplementContentRequirements = "Create ONLY targetRef in this requirement package declaring the single supplemental nativeCheckRef with signature func(t *testing.T). Preserve originalNativeCheckRef and all original checks and inputs all-of; do not omit, alias, or redefine any existing check. Same-package non-method helper functions, types, constants, and data-only vars are allowed; NO receiver methods, NO other Test functions, NO init, NO TestMain, NO global initializer calls or closures, NO nested modules, NO cgo, NO imports of other validation packages, NO os.Executable or instrumentation overrides. Keep all helpers and fixtures within this package (bounded by owner review envelope) and exercise actual non-test workspace Go delivery in-process or via a freshly Go-built executable inheriting owner GOFLAGS and GOCOVERDIR; assert original criteria and every required case through real testing.T. Source-text checks, test-local substitutes, dependency-only calls, and skips grant no proof. Root go build ./..., go test ./..., go test -race ./... and all original plus supplemental CWI checks execute all-of within one original operation deadline (owner default if omitted; allowed 1000-600000ms), including preparation, publication, and cleanup; creation, readiness, and review grant no execution or acceptance credit; scratch is not a security sandbox."

func shellSelectValidationInputAdditionCheck(attestations []runtimecontract.RuntimeValidationCheckAttestationBinding) (runtimecontract.RuntimeValidationCheckAttestationBinding, bool) {
	normalized := runtimecontract.NormalizeRuntimeValidationCheckAttestationBindings(attestations)
	if len(normalized) == 0 {
		return runtimecontract.RuntimeValidationCheckAttestationBinding{}, false
	}
	protocol := normalized[0].ObservationProtocolRef
	selected := normalized[0]
	foundPrimary := false
	for _, check := range normalized {
		if check.SupplementalNativeCheckRef != "" || check.ObservationProtocolRef != protocol {
			return runtimecontract.RuntimeValidationCheckAttestationBinding{}, false
		}
		if shellProjectObservationProtocol(check.ObservationProtocolRef) {
			if !shellProjectRequiresBehavior(check.ObservationProtocolRef) {
				return runtimecontract.RuntimeValidationCheckAttestationBinding{}, false
			}
		} else if check.ObservationProtocolRef != shellGoBehaviorObservationProtocolRef && !shellEntrypointObservationProtocol(check.ObservationProtocolRef) {
			return runtimecontract.RuntimeValidationCheckAttestationBinding{}, false
		}
		if !foundPrimary && strings.HasPrefix(check.RequiredCheckRef, "workorch-cwi-check-") {
			selected = check
			foundPrimary = true
		}
	}
	return selected, true
}

func shellValidationInputAddition(check runtimecontract.RuntimeValidationCheckAttestationBinding) runtimecontract.RuntimeValidationInputAddition {
	addition := runtimecontract.RuntimeValidationInputAddition{
		RequiredCheckRef: check.RequiredCheckRef, OriginalNativeCheckRef: check.NativeCheckRef,
		NativeCheckRef: shellSupplementCheckRef(check.NativeCheckRef), TargetRef: shellSupplementTargetRef(check.NativeCheckRef),
		ContentContractRef: shellSupplementContentContractRef, ContentRequirements: shellSupplementContentRequirements,
	}
	if shellEntrypointObservationProtocol(check.ObservationProtocolRef) {
		addition.ContentContractRef, addition.ContentRequirements = shellEntrypointContentContractRef, shellEntrypointContentRequirements
	}
	if shellIsolatedNativeCheck(check) {
		addition.ContentContractRef = shellIsolatedInputContentContractRef
		addition.ContentRequirements = shellIsolatedInputContentRequirements + " Preserve originalNativeCheckRef; original and supplemental checks must execute and pass conjunctively in this same package."
	}
	if check.ObservationProtocolRef == shellGoConjunctiveEntrypointObservationProtocolRef {
		addition.ContentContractRef = shellConjunctiveInputContentContractRef
		addition.ContentRequirements = shellConjunctiveInputContentRequirements + " Create only this supplemental nativeCheckRef, preserving every original check in the same package."
	}
	if shellProjectObservationProtocol(check.ObservationProtocolRef) {
		addition.ContentContractRef = shellProjectSupplementContentContractRef
		addition.ContentRequirements = shellProjectSupplementContentRequirements
		if check.ObservationProtocolRef == shellProjectMultiFailureDiagnosticProtocolRef {
			addition.ContentRequirements += shellNativeSourceExecutionRequirements
		}
	}
	addition.ContentRequirements += shellAssertionFeedbackRequirements
	return addition
}

// The original native input is retained. A separately created, CWI-specific
// supplement is conjunctive and selected only from the fresh owner manifest.
// No receipt, old binding, Definition or assertion source is rewritten here.
func shellSupplementCheckRef(native string) string { return native + "_Behavior" }

func shellSupplementTargetRef(native string) string {
	if pkg := shellProjectNativePackage(native); pkg != "" {
		digest := strings.ToLower(strings.TrimPrefix(native, "TestWorkOrchCWI_"))
		return pkg + "/behavior_" + digest + "_test.go"
	}
	if pkg := shellConjunctiveNativePackage(native); pkg != "" {
		digest := strings.ToLower(strings.TrimPrefix(native, "TestWorkOrchCWI_"))
		return pkg + "/behavior_" + digest + "_test.go"
	}
	if shellIsolatedNativeIdentity.MatchString(native) {
		return shellIsolatedNativePackage(native) + "/behavior_test.go"
	}
	return "workorch_validation/behavior_" + strings.ToLower(strings.TrimPrefix(native, "TestWorkOrchCWI_")) + "_test.go"
}

func shellValidationSupplementAttestations(values []runtimecontract.RuntimeValidationCheckAttestationBinding, refs []string) []runtimecontract.RuntimeValidationCheckAttestationBinding {
	values = runtimecontract.CloneRuntimeValidationCheckAttestationBindings(values)
	supplemented := false
	for i := range values {
		if values[i].ObservationProtocolRef != shellGoBehaviorObservationProtocolRef && !shellEntrypointObservationProtocol(values[i].ObservationProtocolRef) {
			continue
		}
		if shellStringIncluded(refs, shellSupplementTargetRef(values[i].NativeCheckRef)) {
			values[i].SupplementalNativeCheckRef = shellSupplementCheckRef(values[i].NativeCheckRef)
			supplemented = true
		}
	}
	if supplemented {
		for i := range values {
			if values[i].ObservationProtocolRef == shellGoBehaviorObservationProtocolRef {
				values[i].ObservationProtocolRef = shellGoSupplementObservationProtocolRef
			}
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
