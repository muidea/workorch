package shell

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	shellevents "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func projectReconciliationBatchFixture(t *testing.T) shellevents.QueryRuntimeCWIReconciliationBatchCommand {
	t.Helper()
	r, _ := projectExecutionFixture(t, false)
	recipe := isolatedInputRecipe()
	recipe.AdapterRef = shellProjectAdapterForObservation(shellProjectConjunctionProtocolRef)
	recipe.InputReviewProtocolRef = runtimecontract.RuntimeValidationInputReviewProtocol
	recipe.TimeoutMillis = 60000
	c := shellevents.QueryRuntimeCWIReconciliationBatchCommand{Request: runtimecontract.RuntimeCWIReconciliationRequest{
		RequestID: "fresh-group", WorkspaceID: r.WorkspaceID, RunID: r.RunID, Generation: 1,
		Scope: runtimecontract.RuntimeCWIReconciliationScopeCurrentRun, DefinitionRevision: "definition-group",
		WorkspaceBaselineRevision: "tree-before", WorkspaceBaselineRef: "workspace-tree/tree-before"}}
	for i := 0; i < 2; i++ {
		requirement := shellValidationProofRequirementForTest(t, recipe.ID, common.CoverageWorkItemID(fmt.Sprintf("group-cwi-%d", i)))
		checks, ok := shellValidationCheckAttestations(recipe, requirement)
		if !ok {
			t.Fatal("missing native declaration")
		}
		target := shellValidationNativeCheckTargetRef(requirement, checks...)
		writeNativeStructureSource(t, r.WorkspaceDeliverableDirectory, target, fmt.Sprintf("package checks\nimport (\"testing\"; delivery \"fixture/delivery\")\nfunc %s(t *testing.T){if delivery.Value()!=4{t.Fatal(\"wrong\")}}\n", checks[0].NativeCheckRef))
		binding := runtimecontract.RuntimeCWIReconciliationBindingRef{CoverageWorkItemID: common.CoverageWorkItemID(fmt.Sprintf("cwi-%d", i)), BindingID: fmt.Sprintf("binding-%d", i), BindingHash: fmt.Sprintf("source-binding-%d", i),
			OwnerModuleIDs: []string{ModuleID}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation}, AllowedOperationKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession}, TargetRefs: []string{target}, ValidationRecipeIDs: []string{recipe.ID}, RequiredCheckRefs: requirement.RequiredCheckRefs, ValidationCheckAttestations: checks}
		c.Request.Bindings = append(c.Request.Bindings, binding)
		c.Members = append(c.Members, runtimecontract.RuntimeCWIOwnerReconciliationRequest{WorkspaceID: r.WorkspaceID, RunID: r.RunID, RequestID: c.Request.RequestID, Generation: 1, DefinitionRevision: c.Request.DefinitionRevision,
			OwnerModuleID: ModuleID, Binding: binding, WorkspaceDeliverableDirectory: r.WorkspaceDeliverableDirectory, WorkspaceInternalDirectory: r.WorkspaceInternalDirectory,
			WorkspaceBaselineRevision: c.Request.WorkspaceBaselineRevision, WorkspaceBaselineRef: c.Request.WorkspaceBaselineRef, ValidationRecipes: []runtimecontract.RuntimeValidationRecipe{recipe},
			ValidationScopeTargetRefs: []string{"go.mod", "delivery/value.go", target}, ValidationSourceReceiptHash: fmt.Sprintf("accepted-receipt-%d", i)})
	}
	contract := runtimecontract.RuntimeCWIReconciliationContract{SchemaVersion: runtimecontract.RuntimeCWIReconciliationContractSchemaCurrent, DefinitionRevision: c.Request.DefinitionRevision, Bindings: c.Request.Bindings}
	c.Request.BindingSetHash = runtimecontract.StableRuntimeCWIReconciliationContractHash(contract)
	for i := range c.Members {
		c.Members[i].BindingSetHash = c.Request.BindingSetHash
	}
	if err := c.ValidateForOwner(ModuleID); err != nil {
		t.Fatal(err)
	}
	return c
}

func recordProjectCommands(t *testing.T) string {
	t.Helper()
	goPath, err := exec.LookPath("go")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	counter := filepath.Join(root, "commands")
	// Fixed test-owned paths, no business configuration or external process mock.
	body := fmt.Sprintf("#!/bin/sh\nprintf '%%s\\n' \"$*\" >> '%s'\nexec '%s' \"$@\"\n", counter, goPath)
	if err := os.WriteFile(filepath.Join(root, "go"), []byte(body), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", root+string(os.PathListSeparator)+os.Getenv("PATH"))
	return counter
}

func queryProjectBatch(t *testing.T, c shellevents.QueryRuntimeCWIReconciliationBatchCommand) shellevents.QueryRuntimeCWIReconciliationBatchResult {
	t.Helper()
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	owner := &Biz{Base: basebiz.New(ModuleID, hub, nil), runtimeAdapter: NewRuntimeAdapter()}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { owner.Teardown(ctx) })
	r := hub.Send(magicevent.NewEventWithContext(shellevents.TopicQueryRuntimeCWIReconciliationBatch, runtimecontract.RuntimeModuleOrchestrator, ModuleID, magicevent.NewHeader(), ctx, c))
	if r == nil {
		t.Fatal("nil owner result")
	}
	data, err := r.Get()
	if err != nil {
		t.Fatal(err)
	}
	value, ok := data.(shellevents.QueryRuntimeCWIReconciliationBatchResult)
	if !ok {
		t.Fatalf("wrong owner result %T", data)
	}
	if err := c.ValidateResult(value); err != nil {
		t.Fatal(err)
	}
	return value
}

func TestProjectReconciliationSharesFreshStagesAndKeepsIndependentBehavior(t *testing.T) {
	c := projectReconciliationBatchFixture(t)
	counter := recordProjectCommands(t)
	value := queryProjectBatch(t, c)
	for _, o := range value.Observations {
		if o.Status != runtimecontract.RuntimeCWIReconciliationObservationSatisfied || o.ValidationExecution == nil {
			t.Fatalf("missing exact shared proof: %+v", o)
		}
	}
	a, b := value.Observations[0].ValidationExecution, value.Observations[1].ValidationExecution
	if !reflect.DeepEqual(a.Validation.ExecutionConjunction.Stages[:3], b.Validation.ExecutionConjunction.Stages[:3]) {
		t.Fatal("shared facts replaced original time/output")
	}
	if reflect.DeepEqual(a.Validation.ExecutionConjunction.Stages[3], b.Validation.ExecutionConjunction.Stages[3]) {
		t.Fatal("CWI behavior was borrowed")
	}
	commands, err := os.ReadFile(counter)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(commands)), "\n")
	for _, stage := range []string{"build ./...", "test -json -count=1 ./...", "test -race -json -count=1 ./..."} {
		if n := countProjectCommand(lines, stage); n != 1 {
			t.Fatalf("physical shared stage %q executed %d times: %s", stage, n, commands)
		}
	}
	if len(lines) != 5 {
		t.Fatalf("want 3 shared + 2 exact physical stages, got %s", commands)
	}
	for _, mutation := range []string{"missing-consumer", "borrow-native", "prefix-time", "source-receipt", "scope", "foreign-run", "behavior-as-shared", "unknown-group"} {
		t.Run(mutation, func(t *testing.T) {
			x := shellevents.QueryRuntimeCWIReconciliationBatchResult{}
			for _, o := range value.Observations {
				x.Observations = append(x.Observations, o.Clone())
			}
			u := x.Observations[1].ValidationExecution
			switch mutation {
			case "missing-consumer":
				x.Observations = x.Observations[:1]
			case "borrow-native":
				u.Validation.PassedChecks = slices.Clone(a.Validation.PassedChecks)
			case "prefix-time":
				u.Validation.ExecutionConjunction.Stages[0].StartedAt = u.Validation.ExecutionConjunction.Stages[0].FinishedAt
			case "source-receipt":
				u.SourceReceiptHash = "other-accepted-receipt"
			case "scope":
				u.SourceScopeHash = "other-scope"
			case "foreign-run":
				x.Observations[1].RunID = "other-run"
			case "behavior-as-shared":
				u.SharedStageCount = 4
			case "unknown-group":
				u.GroupID = "unknown"
			}
			if c.ValidateResult(x) == nil {
				t.Fatal("invalid execution fan-out admitted")
			}
		})
	}
}

func TestProjectReconciliationInapplicableGroupExecutesOriginalMethods(t *testing.T) {
	c := projectReconciliationBatchFixture(t)
	c.Members[1].ValidationRecipes[0].TimeoutMillis = 61000
	counter := recordProjectCommands(t)
	value := queryProjectBatch(t, c)
	for _, o := range value.Observations {
		if o.Status != runtimecontract.RuntimeCWIReconciliationObservationSatisfied || o.ValidationExecution != nil {
			t.Fatalf("inapplicable group blocked original method: %+v", o)
		}
	}
	commands, _ := os.ReadFile(counter)
	if n := countProjectCommand(strings.Split(strings.TrimSpace(string(commands)), "\n"), "build ./..."); n != 2 {
		t.Fatalf("original independent builds = %d", n)
	}
}

func TestProjectReconciliationDoesNotRetryFailureOrBorrowBehavior(t *testing.T) {
	for _, kind := range []string{"project-failure", "behavior-omission"} {
		t.Run(kind, func(t *testing.T) {
			c := projectReconciliationBatchFixture(t)
			r := c.Members[1]
			if kind == "project-failure" {
				writeNativeStructureSource(t, r.WorkspaceDeliverableDirectory, "old_test.go", "package fixture\nimport \"testing\"\nfunc TestFailure(t *testing.T){t.Fatal(\"原始失败不改判\")}\n")
			} else {
				native := r.Binding.ValidationCheckAttestations[0].NativeCheckRef
				writeNativeStructureSource(t, r.WorkspaceDeliverableDirectory, r.Binding.TargetRefs[0], fmt.Sprintf("package checks\nimport \"testing\"\nfunc %s(t *testing.T){if 2+2!=4{t.Fatal(\"wrong\")}}\n", native))
			}
			counter := recordProjectCommands(t)
			value := queryProjectBatch(t, c)
			for _, o := range value.Observations {
				if o.Status == runtimecontract.RuntimeCWIReconciliationObservationSatisfied || o.ValidationExecution != nil || len(o.CheckRefs) != 0 {
					t.Fatal("incomplete group gained credit")
				}
			}
			commands, _ := os.ReadFile(counter)
			if n := countProjectCommand(strings.Split(strings.TrimSpace(string(commands)), "\n"), "build ./..."); n != 1 {
				t.Fatalf("failure caused retry: %s", commands)
			}
		})
	}
}

func countProjectCommand(lines []string, stage string) int {
	n := 0
	for _, line := range lines {
		if line == stage {
			n++
		}
	}
	return n
}

func TestProjectReconciliationRawReferencesReadOriginalScopedBytes(t *testing.T) {
	c := projectReconciliationBatchFixture(t)
	value := queryProjectBatch(t, c)
	store, err := openShellProjectRawStore(c.Members[0].WorkspaceInternalDirectory)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	for _, o := range value.Observations {
		if o.ValidationExecution == nil {
			t.Fatal("no execution use")
		}
		u := o.ValidationExecution
		for i, s := range u.Validation.ExecutionConjunction.Stages {
			scope := shellProjectRawScope{WorkspaceID: c.Request.WorkspaceID, RunID: c.Request.RunID, ExecutionKey: "reconciliation/" + u.GroupID, StageID: s.Identity.ID}
			if i >= u.SharedStageCount {
				scope.ExecutionKey += "/" + o.BindingID
			}
			for _, stream := range []string{"stdout", "stderr"} {
				scope.Stream = stream
				identity := s.Stdout
				if stream == "stderr" {
					identity = s.Stderr
				}
				raw, err := store.Read(context.Background(), scope, identity.Storage.Reference)
				if err != nil {
					t.Fatal(err)
				}
				actual := shellProjectOutputIdentity(raw)
				expected := identity.Clone()
				expected.Storage = nil
				if actual != expected {
					t.Fatal("saved original bytes differ from persisted identity")
				}
				wrong := scope
				wrong.RunID = "another-run"
				if _, err := store.Read(context.Background(), wrong, identity.Storage.Reference); err == nil {
					t.Fatal("raw reference crossed owner scope")
				}
			}
		}
	}
}

func TestProjectReconciliationInvalidatesChangedCompleteLiveMemberSet(t *testing.T) {
	for _, kind := range []string{"add", "delete", "replace"} {
		t.Run(kind, func(t *testing.T) {
			c := projectReconciliationBatchFixture(t)
			live := filepath.Join(c.Members[0].WorkspaceDeliverableDirectory, "adjacent.txt")
			if err := os.WriteFile(live, []byte("original\r\n"), 0600); err != nil {
				t.Fatal(err)
			}
			goPath, err := exec.LookPath("go")
			if err != nil {
				t.Fatal(err)
			}
			root := t.TempDir()
			change := ""
			switch kind {
			case "add":
				change = fmt.Sprintf("printf new > '%s.new'", live)
			case "delete":
				change = fmt.Sprintf("rm '%s'", live)
			case "replace":
				change = fmt.Sprintf("printf changed > '%s'", live)
			}
			script := fmt.Sprintf("#!/bin/sh\nif [ \"$1\" = build ]; then %s; fi\nexec '%s' \"$@\"\n", change, goPath)
			if err := os.WriteFile(filepath.Join(root, "go"), []byte(script), 0700); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", root+string(os.PathListSeparator)+os.Getenv("PATH"))
			value := queryProjectBatch(t, c)
			for _, o := range value.Observations {
				if o.Status == runtimecontract.RuntimeCWIReconciliationObservationSatisfied || o.ValidationExecution != nil || o.OriginalDiagnostic == nil || o.OriginalDiagnostic.OriginalCode != "validation_project_source_changed" {
					t.Fatalf("changed source/member set retained shared proof: %+v", o)
				}
			}
		})
	}
}

func TestProjectReconciliationBatchRejectsChangedSealedMembers(t *testing.T) {
	for _, kind := range []string{"missing", "duplicate", "cross-workspace", "cross-run", "generation", "binding-set", "baseline", "binding"} {
		t.Run(kind, func(t *testing.T) {
			c := projectReconciliationBatchFixture(t)
			switch kind {
			case "missing":
				c.Members = c.Members[:1]
			case "duplicate":
				c.Members[1] = c.Members[0]
			case "cross-workspace":
				c.Members[1].WorkspaceID = "other"
			case "cross-run":
				c.Members[1].RunID = "other"
			case "generation":
				c.Members[1].Generation++
			case "binding-set":
				c.Members[1].BindingSetHash = "other"
			case "baseline":
				c.Members[1].WorkspaceBaselineRevision = "other"
			case "binding":
				c.Members[1].Binding = c.Members[1].Binding.Clone()
				c.Members[1].Binding.RequiredCheckRefs = []string{"other"}
			}
			if c.ValidateForOwner(ModuleID) == nil {
				t.Fatal("unsealed members admitted")
			}
		})
	}
}

func TestProjectReconciliationPreflightChecksEveryConsumerBeforeProcesses(t *testing.T) {
	c := projectReconciliationBatchFixture(t)
	if err := os.Remove(filepath.Join(c.Members[1].WorkspaceDeliverableDirectory, c.Members[1].Binding.TargetRefs[0])); err != nil {
		t.Fatal(err)
	}
	counter := recordProjectCommands(t)
	value := queryProjectBatch(t, c)
	if _, err := os.Stat(counter); !os.IsNotExist(err) {
		t.Fatal("incomplete consumer started a process")
	}
	for _, o := range value.Observations {
		if o.Status == runtimecontract.RuntimeCWIReconciliationObservationSatisfied {
			t.Fatal("missing input gained credit")
		}
	}
}

func TestProjectReconciliationBatchHonorsEarlierParentDeadline(t *testing.T) {
	c := projectReconciliationBatchFixture(t)
	requests, ok := shellProjectBatchRequests(c)
	if !ok {
		t.Fatal("batch not eligible")
	}
	counter := recordProjectCommands(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	observations := NewRuntimeAdapter().executeShellProjectBatch(ctx, c, requests)
	if len(observations) != len(c.Members) {
		t.Fatal("cancelled batch lost consumers")
	}
	for _, o := range observations {
		if o.Status == runtimecontract.RuntimeCWIReconciliationObservationSatisfied || o.ValidationExecution != nil || o.OriginalDiagnostic == nil || !strings.Contains(o.OriginalDiagnostic.OriginalMessage, context.Canceled.Error()) {
			t.Fatalf("cancelled request gained proof: %+v", o)
		}
	}
	if _, err := os.Stat(counter); !os.IsNotExist(err) {
		t.Fatal("cancelled batch launched a process")
	}
}

func TestProjectReconciliationBatchIsolatesConcurrentWorkspaces(t *testing.T) {
	for _, id := range []common.WorkspaceID{"workspace-first", "workspace-second"} {
		t.Run(string(id), func(t *testing.T) {
			t.Parallel()
			c := projectReconciliationBatchFixture(t)
			c.Request.WorkspaceID = id
			for i := range c.Members {
				c.Members[i].WorkspaceID = id
			}
			value := queryProjectBatch(t, c)
			if len(value.Observations) != 2 {
				t.Fatal("missing consumers")
			}
			for _, o := range value.Observations {
				if o.WorkspaceID != id || o.ValidationExecution == nil || o.Status != runtimecontract.RuntimeCWIReconciliationObservationSatisfied {
					t.Fatal("workspace proof crossed concurrent query")
				}
			}
			other := c.Request.Clone()
			other.WorkspaceID = "foreign-workspace"
			if runtimecontract.ValidateRuntimeValidationExecutionUses(other, value.Observations) == nil {
				t.Fatal("group crossed workspace")
			}
		})
	}
}
