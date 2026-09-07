package shell

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestNativeCheckStructureResolvesOnlyReachableTestingAssertions(t *testing.T) {
	const name = "TestWorkOrchCWI_STRUCTURE"
	for _, test := range []struct {
		name, body, helpers, wantCode string
	}{
		{"direct", `if 1 != 1 { t.Fatal("bad") }`, "", ""},
		{"same package helper", `verify(t, 1)`, `func verify(check *testing.T, got int) { if got != 1 { check.Errorf("bad") } }`, ""},
		{"helper subtest", `t.Run("required", func(child *testing.T) { verify(child, 1) })`, `func verify(check *testing.T, got int) { if got != 1 { check.Fatal("bad") } }`, ""},
		{"nested helpers", `first(t)`, `func first(check *testing.T) { second(check) }; func second(check *testing.T) { check.Error("bad") }`, ""},
		{"unused helper", `_ = 1`, `func unused(check *testing.T) { check.Fatal("bad") }`, "native_check_assertion_unresolved"},
		{"uninvoked closure", `_ = func() { t.Fatal("not invoked") }`, "", "native_check_assertion_unresolved"},
		{"wrong helper argument", `var other *testing.T; verify(other)`, `func verify(check *testing.T) { check.Fatal("bad") }`, "native_check_assertion_unresolved"},
		{"shadowed testing object", `{ t := struct{ Fatal func(string) }{}; t.Fatal("not testing") }`, "", "native_check_assertion_unresolved"},
		{"helper cycle", `first(t)`, `func first(check *testing.T) { second(check) }; func second(check *testing.T) { first(check) }`, "native_check_assertion_unresolved"},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			entry := "package fixture\nimport \"testing\"\nfunc " + name + "(t *testing.T) { " + test.body + " }"
			writeNativeStructureSource(t, root, "entry_test.go", entry)
			refs := []string{"entry_test.go"}
			if test.helpers != "" {
				writeNativeStructureSource(t, root, "helpers_test.go", "package fixture\nimport \"testing\"\n"+test.helpers)
				refs = append(refs, "helpers_test.go")
			}
			attestations := []runtimecontract.RuntimeValidationCheckAttestationBinding{{RequiredCheckRef: "check", ObservationProtocolRef: shellGoTestObservationProtocolRef, NativeCheckRef: name}}
			got := inspectShellNativeValidationChecks(root, refs, attestations)
			if got.Code != test.wantCode {
				t.Fatalf("inspection = %+v, want %q", got, test.wantCode)
			}
			if declared := shellNativeValidationChecksDeclared(root, refs, attestations); declared != (test.wantCode == "") {
				t.Fatalf("catalog readiness disagrees with diagnostic: %t / %+v", declared, got)
			}
		})
	}
}

func TestNativeCheckStructurePreservesImportAndPackageIdentity(t *testing.T) {
	const name = "TestWorkOrchCWI_SCOPE"
	attestations := []runtimecontract.RuntimeValidationCheckAttestationBinding{{RequiredCheckRef: "check", ObservationProtocolRef: shellGoTestObservationProtocolRef, NativeCheckRef: name}}
	root := t.TempDir()
	writeNativeStructureSource(t, root, "entry_test.go", "package fixture\nimport check \"testing\"\nfunc "+name+"(t *check.T) { verify(t) }")
	writeNativeStructureSource(t, root, "helpers_test.go", "package fixture\nimport tt \"testing\"\nfunc verify(t *tt.T) { t.Fatal(\"bad\") }")
	if got := inspectShellNativeValidationChecks(root, []string{"entry_test.go", "helpers_test.go"}, attestations); got.Code != "" {
		t.Fatalf("testing aliases were not resolved: %+v", got)
	}
	writeNativeStructureSource(t, root, "helpers_test.go", "package other\nimport \"testing\"\nfunc verify(t *testing.T) { t.Fatal(\"bad\") }")
	if got := inspectShellNativeValidationChecks(root, []string{"entry_test.go", "helpers_test.go"}, attestations); got.Code != "native_check_assertion_unresolved" {
		t.Fatalf("cross-package helper leaked: %+v", got)
	}
	writeNativeStructureSource(t, root, "other_test.go", "package other\nimport \"testing\"\nfunc "+name+"(t *testing.T) { t.Fatal(\"bad\") }")
	if got := inspectShellNativeValidationChecks(root, []string{"entry_test.go", "other_test.go"}, attestations); got.Code != "native_check_ambiguous" {
		t.Fatalf("ambiguous native check admitted: %+v", got)
	}
}

func TestNativeCheckStructureFailsClosedAtSourceBound(t *testing.T) {
	root := t.TempDir()
	writeNativeStructureSource(t, root, "large_test.go", strings.Repeat(" ", maxNativeCheckSourceBytes+1))
	attestations := []runtimecontract.RuntimeValidationCheckAttestationBinding{{RequiredCheckRef: "check", ObservationProtocolRef: shellGoTestObservationProtocolRef, NativeCheckRef: "TestWorkOrchCWI_LIMIT"}}
	if got := inspectShellNativeValidationChecks(root, []string{"large_test.go"}, attestations); got.Code != "native_check_analysis_limit" {
		t.Fatalf("source bound not enforced: %+v", got)
	}
	if got := inspectShellNativeValidationChecks(root, []string{"missing_test.go"}, attestations); got.Code != "native_check_source_unavailable" {
		t.Fatalf("missing source was not diagnosed: %+v", got)
	}
}

func writeNativeStructureSource(t *testing.T, root, ref, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filepath.Join(root, ref)), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ref), []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestNativeCheckStructureBoundsHelpersAndRejectsSymlinkEscape(t *testing.T) {
	const name = "TestWorkOrchCWI_BOUND"
	attestations := []runtimecontract.RuntimeValidationCheckAttestationBinding{{RequiredCheckRef: "check", ObservationProtocolRef: shellGoBehaviorObservationProtocolRef, NativeCheckRef: name}}
	root := t.TempDir()
	var source strings.Builder
	fmt.Fprintf(&source, "package fixture\nimport \"testing\"\nfunc %s(t *testing.T) { helper0(t) }\n", name)
	for index := 0; index < maxNativeCheckCallDepth; index++ {
		fmt.Fprintf(&source, "func helper%d(t *testing.T) { helper%d(t) }\n", index, index+1)
	}
	fmt.Fprintf(&source, "func helper%d(t *testing.T) { t.Fatal(\"unreachable within bound\") }\n", maxNativeCheckCallDepth)
	writeNativeStructureSource(t, root, "entry_test.go", source.String())
	if got := inspectShellNativeValidationChecks(root, []string{"entry_test.go"}, attestations); got.Code != "native_check_analysis_limit" {
		t.Fatalf("helper depth was not bounded: %+v", got)
	}
	outside := t.TempDir()
	writeNativeStructureSource(t, outside, "helper_test.go", "package fixture\nimport \"testing\"\nfunc "+name+"(t *testing.T) { t.Fatal(\"outside\") }")
	if err := os.Symlink(outside, filepath.Join(root, "outside")); err != nil {
		t.Fatal(err)
	}
	if got := inspectShellNativeValidationChecks(root, []string{"outside/helper_test.go"}, attestations); got.Code != "native_check_source_unavailable" {
		t.Fatalf("outside source supplied readiness: %+v", got)
	}
}

func TestNativeCheckStructureLeavesCompilationFailuresToExecutionOwner(t *testing.T) {
	const name = "TestWorkOrchCWI_COMPILE"
	attestations := []runtimecontract.RuntimeValidationCheckAttestationBinding{{RequiredCheckRef: "check", ObservationProtocolRef: shellGoBehaviorObservationProtocolRef, NativeCheckRef: name}}
	root := t.TempDir()
	writeNativeStructureSource(t, root, "implementation.go", "package fixture\nfunc invalid(")
	if got := inspectShellNativeValidationChecks(root, []string{"implementation.go"}, attestations); got.Code != "native_check_source_invalid" {
		t.Fatalf("partial source supplied a declaration: %+v", got)
	}
	writeNativeStructureSource(t, root, "entry_test.go", "package fixture\nimport \"testing\"\nfunc "+name+"(t *testing.T) { t.Fatal(\"compile first\") }")
	if got := inspectShellNativeValidationChecks(root, []string{"implementation.go", "entry_test.go"}, attestations); got.Code != "" {
		t.Fatalf("independent build failure suppressed a valid native declaration: %+v", got)
	}
}
