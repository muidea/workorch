package orchestrator

import (
	"regexp"
	"strings"
	"testing"

	shellcommon "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/common"
	rc "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// Real File/Shell/Run/Result/Workspace owners: nested native subprocesses cross
// conflict maintenance, changed implementation failures, fresh all-of and
// assessment, then retain receipts across owner reconstruction/response loss.
func TestDriveRuntimeNativeSourcesSequentialAcceptanceAndRecovery(t *testing.T) {
	options := validationInputReviewMainlineOptions{Adapter: shellcommon.ValidationRecipeAdapterProjectMultiFailureDiagnostic, ValidationCount: 1, ProtocolRef: rc.RuntimeValidationInputReviewProtocol, DynamicProjectObservations: true, SequentialProjectFailures: true, NativeProjectSources: true}
	baseline := runValidationInputReviewMainline(t, "input_revision_after_failure", "", options)
	for _, cut := range []string{"after_project_receipt", "after_result_persistence"} {
		t.Run(cut, func(t *testing.T) {
			if calls := runValidationInputReviewMainline(t, "input_revision_after_failure", cut, options); calls != baseline {
				t.Fatalf("native recovery repeated provider request: %d != %d", calls, baseline)
			}
		})
	}
}

func nativeSourceChannelMainlineInput(t *testing.T, source string) string {
	t.Helper()
	var imports []string
	if !strings.Contains(source, `"os"`) {
		imports = append(imports, `"os"`)
	}
	if !strings.Contains(source, `"os/exec"`) {
		imports = append(imports, `"os/exec"`)
	}
	source = strings.Replace(source, "import (", "import ("+strings.Join(imports, ";")+";", 1)
	pattern := regexp.MustCompile(`(func Test[[:alnum:]_]+\(t \*testing.T\) \{)`)
	if !pattern.MatchString(source) {
		t.Fatal("native fixture has no real test entry")
	}
	source = pattern.ReplaceAllString(source, "$1\n if managedNativeSource(t){return}")
	return source + `
func managedNativeSource(t *testing.T)bool{
 t.Helper()
 if os.Getenv("WORKORCH_NATIVE_SOURCE_CHILD")==t.Name(){return false}
 t.Parallel()
 runner:=os.Getenv("WORKORCH_NATIVE_SOURCE_RUNNER")
 if runner==""{t.Fatal("owner source instrumentation missing")}
 if err:=exec.Command(runner,t.Name()).Run();err!=nil{t.Fatal("owner native execution failed",err)}
 return true
}
`
}

func assertNativeSourceChannelMainlineFacts(t *testing.T, state rc.RuntimeStateRecord) {
	t.Helper()
	children, failed, accepted := 0, 0, 0
	for _, receipt := range state.OperationReceipts {
		if receipt.Evidence.Validation == nil || receipt.Evidence.Validation.ExecutionConjunction == nil {
			continue
		}
		c := receipt.Evidence.Validation.ExecutionConjunction
		if c.Validate() != nil {
			t.Fatal("native execution persistence changed conjunction")
		}
		count := 0
		for _, stage := range c.Stages {
			if stage.NativeSourceFailure != "" {
				t.Fatal("native source failed in acceptance mainline", stage.NativeSourceFailure)
			}
			for _, child := range stage.NativeExecutions {
				count++
				children++
				if child.Validate() != nil || !child.TerminalComplete || child.TimedOut || child.CleanupFailed || child.Stdout.Storage == nil || child.Stderr.Storage == nil || child.Stdout.Storage.Reference == "" || child.Stderr.Storage.Reference == "" {
					t.Fatal("native raw identity/storage lost at receipt")
				}
			}
		}
		if len(c.Stages) > 1 && count == 0 {
			t.Fatal("actual native observations missing from stage")
		}
		if c.Complete {
			accepted++
			if receipt.Outcome != rc.RuntimeOperationOutcomeAccepted {
				t.Fatal("fresh all-of was not accepted")
			}
		} else {
			failed++
		}
	}
	if children == 0 || failed != 4 || accepted != 1 {
		t.Fatalf("native sequence incomplete: children=%d failed=%d accepted=%d", children, failed, accepted)
	}
}
