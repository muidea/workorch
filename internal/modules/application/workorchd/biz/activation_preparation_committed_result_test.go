package biz

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/preparationcompiler"
	filerepository "github.com/muidea/workorch/internal/pkg/repository/file"
	management "github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
	"github.com/muidea/workorch/internal/pkg/workorchrepo"
	rc "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// Runs inside the full mock-provider preparation workflow, after the real owner
// has committed all ten checks. Only this isolated synthetic archive is edited
// to represent a completion produced before a deployment-default change.
func testCommittedPreparationAcrossBudgetDefaults(t *testing.T, storage string, original *workspace.Workspace, baseURL, path string, intent management.PreparationCheckRequest) {
	t.Helper()
	ctx := context.Background()
	a := original.StartupPreparation.Attempts[0]
	current, err := preparationcompiler.CompileOriginalPreparedBootstrap(original, a)
	if err != nil || current.Compilation == nil {
		t.Fatal("current compilation", err)
	}
	old := current.Compilation.Bootstrap.Clone()
	for i := range old.ExecutionBudgetPolicy.Dimensions {
		d := &old.ExecutionBudgetPolicy.Dimensions[i]
		if d.Dimension == rc.RuntimeBudgetDimensionProviderInputToken || d.Dimension == rc.RuntimeBudgetDimensionProviderOutputToken {
			d.MaxIncrement = 131072
		}
	}
	old.ExecutionBudgetPolicy.PolicyHash = rc.StableRuntimeExecutionBudgetPolicyHash(old.ExecutionBudgetPolicy)
	if rc.ValidateRuntimeBootstrapRaw(old) != nil {
		t.Fatal("invalid synthetic earlier policy")
	}
	raw, err := json.Marshal(old)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(raw)
	record := original.StartupPreparation.Clone()
	c := &record.Completions[0]
	c.BootstrapHash = "prepared-bootstrap-sha256:" + hex.EncodeToString(digest[:])
	c.CompletionID = c.Hash()
	if c.BootstrapHash == current.Compilation.BootstrapHash {
		t.Fatal("fixture did not change deployment defaults")
	}
	file := filepath.Join(storage, "internal", "workspaces", string(original.ID), "workspace.json")
	originalBytes, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.WriteFile(file, originalBytes, 0600); err != nil {
			t.Error(err)
		}
		filerepository.EvictMapFileCache(file)
	})
	var rows map[string]map[string]json.RawMessage
	if err := json.Unmarshal(originalBytes, &rows); err != nil {
		t.Fatal(err)
	}
	rows[string(original.ID)]["StartupPreparation"], err = json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	historicalBytes, err := json.Marshal(rows)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, historicalBytes, 0600); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		// A new repository object can share the process's immutable file-data
		// cache. Evict only this synthetic path to exercise a true cold reload.
		filerepository.EvictMapFileCache(file)
		reopened := workorchrepo.NewWorkspaceScopedFileRepositorySet(storage).Workspaces
		w, err := reopened.Get(ctx, original.ID)
		if err != nil {
			t.Fatal("new deployment rejected original result", err)
		}
		storedRecord, err := json.Marshal(w.StartupPreparation)
		if err != nil || !bytes.Equal(storedRecord, rows[string(original.ID)]["StartupPreparation"]) {
			t.Fatal("loaded formal record changed", err)
		}
		if _, err := reopened.List(ctx); err != nil {
			t.Fatal("list", err)
		}
		for _, action := range []string{"", "/check", "/recover"} {
			method := http.MethodPost
			if action == "" {
				method = http.MethodGet
			}
			view := preparationHTTPWorkflowRequest(t, baseURL, method, path+action, intent)
			if view.Preparation.Record.Completions[0].BootstrapHash != c.BootstrapHash || view.Preparation.Record.Completions[0].CompletionID != c.CompletionID {
				t.Fatal("consumer replaced the formal result")
			}
			if action != "" && (view.Compilation == nil || view.Compilation.BootstrapHash != c.BootstrapHash) {
				t.Fatal("recovery recompiled historical judgment")
			}
		}
		after, err := os.ReadFile(file)
		if err != nil || !bytes.Equal(after, historicalBytes) {
			t.Fatal("read/recovery modified original owner bytes", err)
		}
		bad := *w
		r := record.Clone()
		bad.StartupPreparation = &r
		r.Completions[0].BootstrapHash = current.Compilation.BootstrapHash
		if preparationcompiler.ValidatePreparedPersistence(&bad) == nil {
			t.Fatal("changed result without its identity was accepted")
		}
		// Even self-consistent replacement is forbidden after commitment.
		r.Completions[0].CompletionID = r.Completions[0].Hash()
		if reopened.CompareAndSwap(ctx, &bad, w.Revision) == nil {
			t.Fatal("CAS rewrote original completion")
		}
		if reopened.CompareAndSwap(ctx, w, w.Revision-1) == nil {
			t.Fatal("stale CAS accepted")
		}
	}
}
