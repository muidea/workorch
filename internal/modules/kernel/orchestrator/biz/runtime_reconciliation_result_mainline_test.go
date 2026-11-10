package orchestrator

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// The producer is the real ordinary validation in the suspension mainline.
// These queries consume that committed result without acquiring a new method,
// running a process, granting credit or changing the original receipt.
func runtimeAssertReconciliationFormalResultReads(t *testing.T, ctx context.Context, hub magicevent.Hub, run *runmodel.Run, a runtime.RuntimeAggregate, directory string) {
	t.Helper()
	request := runtimeReconciliationRequestForRun(run, a, "formal-result-read-test", 0)
	var member runtimecontract.RuntimeCWIOwnerReconciliationRequest
	for _, binding := range request.Bindings {
		if binding.CoverageWorkItemID != "cwi-1" {
			continue
		}
		source, hash, err := runtime.ReconciliationValidationResultSource(a, binding)
		if err != nil || source == nil || hash == "" {
			t.Fatalf("missing real formal validation source: %v", err)
		}
		member = runtimecontract.RuntimeCWIOwnerReconciliationRequest{
			WorkspaceID: request.WorkspaceID, RunID: request.RunID, RequestID: request.RequestID,
			Generation: request.Generation, DefinitionRevision: request.DefinitionRevision,
			WorkspaceDeliverableDirectory: directory, WorkspaceInternalDirectory: filepath.Join(filepath.Dir(directory), "internal"),
			BindingSetHash: request.BindingSetHash, OwnerModuleID: runtimecontract.RuntimeModuleShell, Binding: binding,
			WorkspaceBaselineRevision: request.WorkspaceBaselineRevision, WorkspaceBaselineRef: request.WorkspaceBaselineRef,
			ValidationSource: source, ValidationSourceReceiptHash: hash,
		}
		break
	}
	if member.ValidationSource == nil {
		t.Fatal("missing current acceptance binding")
	}
	for _, scenario := range []string{"other-binding", "missing-receipt", "rejected-receipt"} {
		copy := a.CloneRaw()
		binding := member.Binding.Clone()
		switch scenario {
		case "other-binding":
			binding.BindingHash = "another-binding"
		case "missing-receipt":
			copy.OperationReceipts = nil
		case "rejected-receipt":
			for i := range copy.OperationReceipts {
				if runtimecontract.StableRuntimeOperationReceiptHash(copy.OperationReceipts[i]) == member.ValidationSourceReceiptHash {
					copy.OperationReceipts[i].Outcome = runtimecontract.RuntimeOperationOutcomeRejected
				}
			}
		}
		source, _, err := runtime.ReconciliationValidationResultSource(copy, binding)
		if source != nil || scenario == "missing-receipt" && err == nil {
			t.Fatalf("accepted %s result source: %v", scenario, err)
		}
	}
	marker := filepath.Join(t.TempDir(), "query-executed-go")
	if err := os.WriteFile(filepath.Join(filepath.Dir(marker), "go"), []byte("#!/bin/sh\nprintf executed > '"+marker+"'\nexit 99\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", filepath.Dir(marker)+string(os.PathListSeparator)+os.Getenv("PATH"))
	for range 2 {
		o, err := queryRuntimeCWIReconciliationOwner(ctx, hub, member.OwnerModuleID, member, request)
		if err != nil || o.Status != runtimecontract.RuntimeCWIReconciliationObservationSatisfied || o.ValidationExecution != nil {
			t.Fatalf("formal result query: %v status=%s", err, o.Status)
		}
	}
	for _, scenario := range []string{"receipt", "workspace", "fence", "evidence_contract"} {
		wrong := member
		source := *member.ValidationSource
		wrong.ValidationSource = &source
		switch scenario {
		case "receipt":
			wrong.ValidationSourceReceiptHash = "different-receipt"
		case "workspace":
			source.WorkspaceID = "another-workspace"
		case "fence":
			source.Execution.Fence++
		case "evidence_contract":
			source.EvidencePolicies = nil
		}
		if _, err := queryRuntimeCWIReconciliationOwner(ctx, hub, wrong.OwnerModuleID, wrong, request); err == nil {
			t.Fatalf("accepted wrong %s source", scenario)
		}
	}
	path := filepath.Join(directory, "subject.go")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, info.Mode().Perm()^0040); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, info.Mode().Perm()) })
	o, err := queryRuntimeCWIReconciliationOwner(ctx, hub, member.OwnerModuleID, member, request)
	if err != nil || o.Status != runtimecontract.RuntimeCWIReconciliationObservationUnverifiable || o.OriginalDiagnostic == nil || o.OriginalDiagnostic.OriginalCode != "validation_dependencies_changed" {
		t.Fatalf("changed selector did not invalidate applicability: %v status=%s", err, o.Status)
	}
	if err := os.Rename(path, path+".absent"); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Rename(path+".absent", path) }()
	o, err = queryRuntimeCWIReconciliationOwner(ctx, hub, member.OwnerModuleID, member, request)
	if err != nil || o.Status != runtimecontract.RuntimeCWIReconciliationObservationUnverifiable || o.OriginalDiagnostic == nil || o.OriginalDiagnostic.OriginalCode != "validation_dependencies_changed" {
		t.Fatalf("missing selector did not invalidate applicability: %v", err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("formal result query executed process: %v", err)
	}
}
