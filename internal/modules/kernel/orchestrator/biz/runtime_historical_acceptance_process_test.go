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

const crossCWIProcessCutExit = 73

// Real processes with fresh Hub/owners; the admitted startup and provider are
// explicit private fixtures, not public authoring or live-model verification.
func TestDriveRuntimeCrossCWIHistoricalAcceptanceProcessRecovery(t *testing.T) {
	runHistoricalAcceptanceProcessRecovery(t)
}

func TestDriveRuntimeProfileDeliverableHistoricalAcceptanceProcessRecovery(t *testing.T) {
	t.Setenv("WORKORCH_CROSS_CWI_PROFILE_SOURCE", "1")
	runHistoricalAcceptanceProcessRecovery(t)
}

func TestDriveRuntimeProfileDeliverableFourRoundProcessRecovery(t *testing.T) {
	t.Setenv("WORKORCH_CROSS_CWI_PROFILE_SOURCE", "1")
	t.Setenv("WORKORCH_CROSS_CWI_FOUR_ROUNDS", "1")
	runHistoricalAcceptanceProcessRecovery(t)
}

func runHistoricalAcceptanceProcessRecovery(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workorch-cross-cwi-process-original")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	runCrossCWIProcess(t, root, "seed", crossCWIProcessCutExit)
	// Read the cut through a distinct repository path: file repository caches
	// must not hide the second child's later writes to the original path.
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
		t.Fatalf("read original behavior CAS: %v", err)
	}
	before := first.RuntimeState.Clone()
	if err := before.Validate(); err != nil || before.ActiveDispatch != nil || before.TerminalMaterialSeal != nil {
		t.Fatalf("behavior cut is not a valid deferred preimage: %v", err)
	}
	var historical runtimecontract.RuntimeCoverageItemFact
	for _, row := range before.Facts.Coverage {
		if row.ID == "cwi-file" {
			historical = row
		}
	}
	if historical.HistoricalBehavior == nil || len(historical.HistoricalAcceptanceProofs) != 0 ||
		historical.State == runtimecontract.RuntimeCoverageItemStateSatisfied || historical.CWIConvergenceState == runtimecontract.RuntimeCWIConvergenceStateSettled || len(historical.EvidenceRefs) != 0 {
		t.Fatal("historical behavior prematurely granted ordinary acceptance credit")
	}
	if content, err := os.ReadFile(filepath.Join(root, "workspace", "main.go")); err != nil || string(content) != "package main\n" {
		t.Fatalf("original create effect was not durable: %q %v", content, err)
	}
	runCrossCWIProcess(t, root, "resume", 0)
	second, err := filerepository.NewRunRepository(filepath.Join(root, "runs.json")).GetByWorkspaceID(context.Background(), "ws-v2", "run-v2")
	if err != nil || second.RuntimeState == nil || second.State != common.RunStateCompleted {
		t.Fatalf("fresh process did not complete original Run: %v", err)
	}
	after := second.RuntimeState
	if os.Getenv("WORKORCH_CROSS_CWI_FOUR_ROUNDS") == "1" && len(after.ImplementationWork.Rounds) != 4 {
		t.Fatal("fresh process omitted real working rounds")
	}
	if err := after.Validate(); err != nil || after.TerminalMaterialSeal == nil {
		t.Fatalf("fresh process terminal material is invalid: %v", err)
	}
	if !reflect.DeepEqual(first.RuntimeBootstrap, second.RuntimeBootstrap) || !reflect.DeepEqual(before.Definition, after.Definition) ||
		!reflect.DeepEqual(before.Facts.DefinitionSeed, after.Facts.DefinitionSeed) || !reflect.DeepEqual(before.Facts.Activation, after.Facts.Activation) {
		t.Fatal("fresh process replaced original startup identity")
	}
	for _, receipt := range before.OperationReceipts {
		current, found := runtimecontract.RuntimeOperationReceiptForExecutionKeyFence(after.OperationReceipts, receipt.ExecutionKey, receipt.Fence)
		if !found || !runtimecontract.SameRuntimeOperationReceipt(receipt, current) {
			t.Fatal("fresh process rewrote original owner receipt")
		}
	}
	for _, dispatch := range before.CompletedDispatches {
		matches := 0
		for _, current := range after.CompletedDispatches {
			if current.ID == dispatch.ID && reflect.DeepEqual(dispatch, current) {
				matches++
			}
		}
		if matches != 1 {
			t.Fatal("fresh process lost or duplicated original completed transaction")
		}
	}
	for _, allocation := range before.ExecutionBudget.Allocations {
		matches := 0
		for _, current := range after.ExecutionBudget.Allocations {
			if current.AllocationID == allocation.AllocationID && reflect.DeepEqual(current, allocation) {
				matches++
			}
		}
		if matches != 1 {
			t.Fatal("fresh process replaced original allocation")
		}
	}
	for _, consumption := range before.ExecutionBudget.Consumptions {
		matches := 0
		for _, current := range after.ExecutionBudget.Consumptions {
			if current.ConsumptionID == consumption.ConsumptionID && reflect.DeepEqual(current, consumption) {
				matches++
			}
		}
		if matches != 1 {
			t.Fatal("fresh process lost or duplicated original consumption")
		}
	}
	for _, row := range after.Facts.Coverage {
		if row.ID == historical.ID && (!reflect.DeepEqual(row.HistoricalBehavior, historical.HistoricalBehavior) ||
			len(row.HistoricalAcceptanceProofs) != 1 || row.State != runtimecontract.RuntimeCoverageItemStateSatisfied) {
			t.Fatal("ordinary consumer replaced behavior or did not settle it exactly once")
		}
	}
	if len(after.OperationRecoveryAuthorizations) != len(before.OperationRecoveryAuthorizations) || after.Facts.Recovery.OperationRecoveryAttempts != before.Facts.Recovery.OperationRecoveryAttempts {
		t.Fatal("fresh process consumed unnecessary physical recovery")
	}
}

func runCrossCWIProcess(t *testing.T, root, stage string, expectedExit int) {
	t.Helper()
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, binary, "-test.run=^TestCrossCWIHistoricalAcceptanceProcessChild$", "-test.timeout=110s", "-test.v")
	command.Env = append(os.Environ(), "WORKORCH_CROSS_CWI_TEST_STAGE="+stage, "WORKORCH_CROSS_CWI_TEST_ROOT="+root)
	output, err := command.CombinedOutput()
	if writeErr := os.WriteFile(filepath.Join(root, stage+".log"), output, 0o600); writeErr != nil {
		t.Fatal(writeErr)
	}
	exit := 0
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("process failed: %v\n%s", err, output)
		}
		exit = exitErr.ExitCode()
	}
	if ctx.Err() != nil || exit != expectedExit {
		t.Fatalf("process %s exit=%d want=%d: %v\n%s", stage, exit, expectedExit, ctx.Err(), output)
	}
}

func TestCrossCWIHistoricalAcceptanceProcessChild(t *testing.T) {
	if os.Getenv("WORKORCH_CROSS_CWI_TEST_STAGE") == "" {
		return // Only the parent explicitly enables the isolated process fixture.
	}
	if os.Getenv("WORKORCH_CROSS_CWI_FOUR_ROUNDS") == "1" {
		runRuntimeFileBackedFreshFileShellGitAcceptance(t, "after_behavior", true, true)
	} else {
		runRuntimeFileBackedFreshFileShellGitAcceptance(t, "after_behavior", os.Getenv("WORKORCH_CROSS_CWI_PROFILE_SOURCE") == "1")
	}
}
