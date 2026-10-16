package orchestrator

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/muidea/workorch/internal/pkg/common"
	filerepository "github.com/muidea/workorch/internal/pkg/repository/file"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestDriveRuntimeCWISuspensionProcessRecovery(t *testing.T) {
	for _, cut := range []string{"after_suspend", "after_resume"} {
		t.Run(cut, func(t *testing.T) {
			root := t.TempDir()
			runCWISuspensionProcess(t, root, "seed", cut, crossCWIProcessCutExit)
			// Separate path avoids an in-process repository cache masking the
			// second child's changes to the original durable path.
			body, err := os.ReadFile(filepath.Join(root, "runs.json"))
			if err != nil {
				t.Fatal(err)
			}
			cutPath := filepath.Join(root, "cut-runs.json")
			if err := os.WriteFile(cutPath, body, 0o600); err != nil {
				t.Fatal(err)
			}
			first, err := filerepository.NewRunRepository(cutPath).GetByWorkspaceID(context.Background(), "ws-v2", "run-v2")
			if err != nil || first.RuntimeState == nil {
				t.Fatalf("read durable process cut: %v", err)
			}
			before := first.RuntimeState.Clone()
			if err := before.Validate(); err != nil || len(before.Facts.CWISuspensions) != 1 || before.ActiveDispatch != nil || before.TerminalMaterialSeal != nil {
				t.Fatalf("invalid suspension process preimage: %v", err)
			}
			pause := before.Facts.CWISuspensions[0]
			if (pause.ResumeBinding != nil) != (cut == "after_resume") {
				t.Fatal("process did not exit at exact requested CAS")
			}
			runCWISuspensionProcess(t, root, "resume", cut, 0)
			second, err := filerepository.NewRunRepository(filepath.Join(root, "runs.json")).GetByWorkspaceID(context.Background(), "ws-v2", "run-v2")
			if err != nil || second.RuntimeState == nil || second.State != common.RunStateCompleted {
				t.Fatalf("fresh process did not complete original Run: %v", err)
			}
			after := second.RuntimeState
			if err := after.Validate(); err != nil || !runtimeTerminalDeliveryFullyAcknowledged(runtimeAggregateFromRecord(*after)) {
				t.Fatalf("fresh process did not complete valid ordered delivery: %v", err)
			}
			if !reflect.DeepEqual(first.RuntimeBootstrap, second.RuntimeBootstrap) || !reflect.DeepEqual(before.Facts.DefinitionSeed, after.Facts.DefinitionSeed) || !reflect.DeepEqual(before.Facts.Activation, after.Facts.Activation) {
				t.Fatal("fresh process changed original sealed startup")
			}
			resumed := after.Facts.CWISuspensions[0].CloneRaw()
			if pause.ResumeBinding == nil {
				resumed.ResumeBinding, resumed.ResumeAssessmentID, resumed.ResumeEvidenceRevision, resumed.ResumeObservationCount = nil, "", "", 0
			}
			if !reflect.DeepEqual(pause, resumed) {
				t.Fatal("fresh process rewrote original pause/resume source")
			}
			for _, receipt := range before.OperationReceipts {
				current, found := runtimecontract.RuntimeOperationReceiptForExecutionKeyFence(after.OperationReceipts, receipt.ExecutionKey, receipt.Fence)
				if !found || !runtimecontract.SameRuntimeOperationReceipt(receipt, current) {
					t.Fatal("fresh process lost or rewrote original receipt")
				}
			}
			for _, allocation := range before.ExecutionBudget.Allocations {
				matches := 0
				for _, current := range after.ExecutionBudget.Allocations {
					if reflect.DeepEqual(current, allocation) {
						matches++
					}
				}
				if matches != 1 {
					t.Fatal("fresh process replaced or refunded original allocation")
				}
			}
			for _, consumption := range before.ExecutionBudget.Consumptions {
				matches := 0
				for _, current := range after.ExecutionBudget.Consumptions {
					if reflect.DeepEqual(current, consumption) {
						matches++
					}
				}
				if matches != 1 {
					t.Fatal("fresh process replaced or refunded original consumption")
				}
			}
			if !reflect.DeepEqual(before.OperationRecoveryAuthorizations, after.OperationRecoveryAuthorizations) {
				t.Fatal("fresh process issued unnecessary physical recovery")
			}
		})
	}
}

func runCWISuspensionProcess(t *testing.T, root, stage, cut string, expectedExit int) {
	t.Helper()
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, binary, "-test.run=^TestCWISuspensionProcessChild$", "-test.timeout=110s", "-test.v")
	command.Env = append(os.Environ(), "WORKORCH_CWI_SUSPENSION_STAGE="+stage, "WORKORCH_CWI_SUSPENSION_ROOT="+root, "WORKORCH_CWI_SUSPENSION_CUT="+cut)
	output, err := command.CombinedOutput()
	if writeErr := os.WriteFile(filepath.Join(root, stage+".log"), output, 0o600); writeErr != nil {
		t.Fatal(writeErr)
	}
	exit := 0
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("child process: %v\n%s", err, output)
		}
		exit = exitErr.ExitCode()
	}
	if ctx.Err() != nil || exit != expectedExit {
		t.Fatalf("process %s exit=%d want=%d: %v\n%s", stage, exit, expectedExit, ctx.Err(), output)
	}
}

func TestCWISuspensionProcessChild(t *testing.T) {
	if os.Getenv("WORKORCH_CWI_SUSPENSION_STAGE") == "" {
		return
	}
	runRuntimeCWISuspensionMainline(t, os.Getenv("WORKORCH_CWI_SUSPENSION_CUT"))
}
