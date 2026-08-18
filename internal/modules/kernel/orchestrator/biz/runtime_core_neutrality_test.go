package orchestrator

import (
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"testing"
)

func TestRuntimeCorePolicyCompilersHaveNoCapabilitySpecificBranches(t *testing.T) {
	_, filename, _, ok := goruntime.Caller(0)
	if !ok {
		t.Fatal("resolve test source path")
	}
	bizDirectory := filepath.Dir(filename)
	tests := []struct {
		path      string
		forbidden []string
	}{
		{
			path: filepath.Clean(filepath.Join(bizDirectory, "../../../../pkg/workorchruntimecontract/runtime_effect_resolve.go")),
			forbidden: []string{
				"RuntimeMCPCapabilityContext", "StepKindMCP", "RuntimeModuleMCP", "RuntimeModuleShell",
				"serverId", "RiskLevel", "stateChanging",
			},
		},
		{
			path: filepath.Join(bizDirectory, "runtime/prerequisite.go"),
			forbidden: []string{
				"RuntimeModuleShell", "StepKindShell", "ManagedValidation", "managed-validation",
			},
		},
		{
			path: filepath.Join(bizDirectory, "runtime/reducer.go"),
			forbidden: []string{
				"RuntimeModuleMCP", "StepKindMCP", "RuntimeModuleShell", "StepKindShell",
				"creationScopeAdmissibleFile", "creation_scope_file",
			},
		},
		{
			path: filepath.Join(bizDirectory, "internal/candidateadmission/evaluator.go"),
			forbidden: []string{
				"RuntimeModuleMCP", "StepKindMCP", "RuntimeModuleShell", "StepKindShell",
				"RuntimeModuleFile", "StepKindFile",
			},
		},
	}
	for _, testCase := range tests {
		data, err := os.ReadFile(testCase.path)
		if err != nil {
			t.Fatalf("read %s: %v", testCase.path, err)
		}
		for _, forbidden := range testCase.forbidden {
			if strings.Contains(string(data), forbidden) {
				t.Errorf("%s contains capability-specific Runtime branch token %q", filepath.Base(testCase.path), forbidden)
			}
		}
	}
}
