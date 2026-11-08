package shell

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	shellcommon "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/common"
	shellevents "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/events"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestModuleChecksumDiagnosticDoesNotRepairImportingSource(t *testing.T) {
	for _, scenario := range []string{"missing", "existing", "scope-missing", "truncated", "timeout", "different-dependency", "marker"} {
		t.Run(scenario, func(t *testing.T) {
			request := RuntimeCommandRequest{
				ManagedValidation:         runtimecontract.RuntimeManagedValidation{RuntimeManaged: true, Kind: runtimecontract.RuntimeBlockKindManagedValidationSession},
				ValidationScopeTargetRefs: []string{"go.mod", "internal/client.go"},
			}
			output := RuntimeCommandResult{Command: "go", Args: []string{"build", "./..."}, ExitCode: 1, Stage: "project-build",
				Stderr: "internal/client.go:7:2: missing go.sum entry for module providing package example.org/dependency (imported by fixture/internal); to add:\n\tgo get fixture/internal\n"}
			switch scenario {
			case "existing", "marker":
				request.ValidationScopeTargetRefs = append(request.ValidationScopeTargetRefs, "go.sum")
			case "scope-missing":
				request.ValidationScopeTargetRefs = []string{"internal/client.go"}
			case "truncated":
				output.OutputTruncated = true
			case "different-dependency":
				output.Stderr = "no required module provides package example.org/other"
			}
			before := output.Stderr
			attachShellValidationFailureDiagnostic(&output, request, "go", 1, scenario == "timeout", false, errors.New("exit status 1"))
			diagnostic := output.ValidationFailureDiagnostic
			if diagnostic == nil || output.Stderr != before {
				t.Fatal("original diagnostic lost or rewritten")
			}
			if scenario == "missing" && (diagnostic.OwnerCode != shellcommon.ValidationDiagnosticCodeModuleChecksumCreate || len(diagnostic.TargetRefs) != 0) {
				t.Fatalf("creation invented an existing surface: %+v", diagnostic)
			}
			if scenario == "existing" || scenario == "marker" {
				if diagnostic.OwnerCode != shellcommon.ValidationDiagnosticCodeModuleChecksumUpdate || !reflect.DeepEqual(diagnostic.TargetRefs, []string{"go.sum"}) {
					t.Fatalf("checksum update targeted importing source: %+v", diagnostic)
				}
			} else if len(diagnostic.TargetRefs) != 0 {
				t.Fatalf("unattributed failure acquired targets: %+v", diagnostic)
			}
			if scenario == "scope-missing" || scenario == "truncated" || scenario == "timeout" || scenario == "different-dependency" {
				if diagnostic.OwnerCode == shellcommon.ValidationDiagnosticCodeModuleChecksumCreate || diagnostic.OwnerCode == shellcommon.ValidationDiagnosticCodeModuleChecksumUpdate {
					t.Fatal("incomplete/non-checksum input borrowed checksum route")
				}
			}
			if scenario == "marker" {
				output.Stdout = `WORKORCH_FAILURE_TARGET={"schemaVersion":"workorch.failure_target.v1","targetRole":"implementation","diagnosticClass":"validation_dependency_mismatch","targetRefs":["internal/client.go"]}`
			}
			refs, valid, _ := shellFailureTargetCandidateUnion(output, "implementation", request.ValidationScopeTargetRefs)
			if scenario == "marker" && valid {
				t.Fatal("free text widened typed dependency surface")
			}
			observationDiagnostic := diagnostic.Clone()
			observationDiagnostic.TargetRefs = refs
			ctx := runtimecontract.RuntimeOperationExecutionContext{WorkspaceID: "ws", RunID: "run", DispatchID: "dispatch", OperationID: "validate", Transaction: runtimecontract.AuthorityTransaction{AuthoritySnapshotID: "authority"}}
			observation := shellValidationFailureObservation(ctx, shellevents.RuntimeExecutionAuthorization{TargetRole: "implementation", ValidationScopeTargetRefs: request.ValidationScopeTargetRefs}, observationDiagnostic)
			if scenario == "existing" && (observation == nil || !reflect.DeepEqual(observation.CandidateTargetRefs, []string{"go.sum"})) {
				t.Fatal("existing checksum metadata lost File confirmation input")
			}
			if scenario != "existing" && scenario != "marker" && observation != nil {
				t.Fatal("prospective/unknown dependency invented current File observation")
			}
		})
	}
}

// The Go process, stage storage and completion boundary are real; no model or
// network operation is needed to reproduce a read-only missing checksum build.
func TestProjectBuildFailurePublishesTypedDiagnosticAndOriginalStage(t *testing.T) {
	for _, scenario := range []string{"checksum-missing", "checksum-existing", "source-compile"} {
		t.Run(scenario, func(t *testing.T) {
			request, execution := projectExecutionFixture(t, false, shellcommon.ValidationRecipeAdapterProjectMultiFailureDiagnostic)
			if strings.HasPrefix(scenario, "checksum") {
				writeNativeStructureSource(t, request.WorkspaceDeliverableDirectory, "go.mod", "module fixture\ngo 1.26\nrequire example.org/not-fetched v1.0.0\n")
				writeNativeStructureSource(t, request.WorkspaceDeliverableDirectory, "delivery/value.go", "package delivery\nimport _ \"example.org/not-fetched\"\nfunc Value() int {return 4}\n")
				if scenario == "checksum-existing" {
					writeNativeStructureSource(t, request.WorkspaceDeliverableDirectory, "go.sum", "")
					request.ValidationScopeTargetRefs = append(request.ValidationScopeTargetRefs, "go.sum")
				}
			} else {
				writeNativeStructureSource(t, request.WorkspaceDeliverableDirectory, "delivery/value.go", "package delivery\nfunc Value() int {return missingValue}\n")
			}
			result, err := NewRuntimeAdapter().executeShellProjectConjunction(context.Background(), request, execution)
			diagnostic := result.Result.ValidationFailureDiagnostic
			if err == nil || diagnostic == nil || result.Result.FailureCode != "validation_project_stage_failed" || result.Result.Stage != "project-build" || result.Result.ExitCode != 1 || result.Conjunction == nil || result.Conjunction.Complete || len(result.Conjunction.Stages) != 1 {
				t.Fatalf("project build lost typed failure: err=%v result=%+v diagnostic=%+v", err, result.Result, diagnostic)
			}
			wantCode := shellcommon.ValidationDiagnosticCodeModuleChecksumCreate
			if scenario == "checksum-existing" {
				wantCode = shellcommon.ValidationDiagnosticCodeModuleChecksumUpdate
			} else if scenario == "source-compile" {
				wantCode = shellcommon.ValidationDiagnosticCodeSourceCompileFailed
			}
			if diagnostic.OwnerCode != wantCode || diagnostic.Stage != "project-build" || result.Result.Evidence.Validation == nil || result.Result.Evidence.Validation.ProofClass != "" {
				t.Fatalf("failure was untyped or credited: %+v", diagnostic)
			}
			stage := result.Conjunction.Stages[0]
			store, err := openShellProjectRawStore(request.WorkspaceInternalDirectory)
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			raw, err := store.Read(context.Background(), shellProjectRawScope{WorkspaceID: request.WorkspaceID, RunID: request.RunID, ExecutionKey: execution.Execution.ExecutionKey, StageID: "project-build", Stream: "stderr"}, stage.Stderr.Storage.Reference)
			if err != nil || string(raw.Raw) != result.Result.Stderr {
				t.Fatalf("original stderr not preserved: %v", err)
			}
			if _, err := os.Stat(filepath.Join(request.WorkspaceDeliverableDirectory, "go.sum")); scenario == "checksum-missing" && !os.IsNotExist(err) {
				t.Fatal("validation repaired the project instead of reporting failure")
			}
		})
	}
}
