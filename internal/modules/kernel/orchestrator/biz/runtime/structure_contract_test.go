package runtime

import (
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// The lifecycle exception must never grow into a second Runtime mutation,
// read, I/O, reducer, callback or authority surface.
func TestOwnerHistoricalCompilerReleaseHasOnlyPureDiscardBody(t *testing.T) {
	for _, file := range parseRuntimeProductionSources(t) {
		for _, declaration := range file.Decls {
			fn, ok := declaration.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || fn.Name.Name != "ReleaseHistoricalCompilation" {
				continue
			}
			var body strings.Builder
			if err := format.Node(&body, token.NewFileSet(), fn.Body); err != nil {
				t.Fatal(err)
			}
			if body.String() != "{\n\tif c != nil {\n\t\tc.historicalCompiler.Close()\n\t}\n}" {
				t.Fatalf("pure lifecycle discard expanded: %s", body.String())
			}
			return
		}
	}
	t.Fatal("pure historical compiler discard method missing")
}

// TestRuntimeMutationOwnerStructureIsUnique freezes the O0 owner boundary.
// SM-2/SM-3/SM-4 may add private handlers, but they must continue to share one
// aggregate, one mutation dispatcher and one aggregate CAS coordinator.
func TestRuntimeMutationOwnerStructureIsUnique(t *testing.T) {
	t.Parallel()

	files := parseRuntimeProductionSources(t)
	typeCounts := map[string]int{}
	methods := map[string][]string{}
	applyInternalEventCount := 0
	type saveCall struct {
		file     string
		receiver string
		method   string
		callee   string
	}
	var repositorySaveCalls []saveCall

	for filename, file := range files {
		ast.Inspect(file, func(node ast.Node) bool {
			declaration, ok := node.(*ast.GenDecl)
			if !ok || declaration.Tok != token.TYPE {
				return true
			}
			for _, specification := range declaration.Specs {
				if typeSpec, typeOK := specification.(*ast.TypeSpec); typeOK {
					typeCounts[typeSpec.Name.Name]++
				}
			}
			return false
		})

		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Recv == nil || len(function.Recv.List) != 1 {
				continue
			}
			receiver := runtimeReceiverType(function.Recv.List[0].Type)
			if receiver == "" {
				continue
			}
			if ast.IsExported(function.Name.Name) {
				methods[receiver] = append(methods[receiver], function.Name.Name)
			}
			if receiver == "RuntimeController" && function.Name.Name == "ApplyInternalEvent" {
				applyInternalEventCount++
			}
			ast.Inspect(function.Body, func(node ast.Node) bool {
				call, callOK := node.(*ast.CallExpr)
				if !callOK {
					return true
				}
				selector, selectorOK := call.Fun.(*ast.SelectorExpr)
				if selectorOK && (selector.Sel.Name == "Save" || selector.Sel.Name == "SaveValidated") {
					repositorySaveCalls = append(repositorySaveCalls, saveCall{
						file:     filepath.Base(filename),
						receiver: receiver,
						method:   function.Name.Name,
						callee:   selector.Sel.Name,
					})
				}
				return true
			})
		}
	}

	for _, name := range []string{"RuntimeAggregate", "RuntimeController", "RuntimeTransitionCoordinator"} {
		if typeCounts[name] != 1 {
			t.Fatalf("production type %s declarations = %d, want exactly 1", name, typeCounts[name])
		}
	}
	if applyInternalEventCount != 1 {
		t.Fatalf("RuntimeController.ApplyInternalEvent declarations = %d, want exactly 1 mutation dispatcher", applyInternalEventCount)
	}

	assertRuntimeExportedMethods(t, methods, "RuntimeController", []string{
		"ApplyInternalEvent",
		"CompileEvidenceApplicabilityRead",
		"Load",
		"QueryOperationReceipt",
		"ReleaseHistoricalCompilation", // lifecycle-only, statically checked below
	})
	assertRuntimeExportedMethods(t, methods, "RuntimeTransitionCoordinator", []string{"Commit"})

	if len(repositorySaveCalls) != 2 {
		t.Fatalf("production repository Save call sites = %d, want the validated production path and guarded fallback only through RuntimeTransitionCoordinator.Commit: %+v", len(repositorySaveCalls), repositorySaveCalls)
	}
	wantCallees := map[string]bool{"Save": true, "SaveValidated": true}
	for _, call := range repositorySaveCalls {
		if call.file != "transition_coordinator.go" || call.receiver != "RuntimeTransitionCoordinator" || call.method != "Commit" || !wantCallees[call.callee] {
			t.Fatalf("repository Save bypasses the sole CAS coordinator: %+v", call)
		}
		delete(wantCallees, call.callee)
	}
	if len(wantCallees) != 0 {
		t.Fatalf("runtime transition coordinator save paths are incomplete: %+v", wantCallees)
	}
}

// TestRuntimePrivateComponentsShareOneMutationKernel freezes the first O2
// extraction boundary.  The five names are deliberately package-private:
// they are coordination facets of one Orchestrator owner, not new framework
// modules or alternate state owners.  Each facet may depend on the kernel,
// but may not retain its own repository, coordinator, aggregate, or sibling
// component reference.
func TestRuntimePrivateComponentsShareOneMutationKernel(t *testing.T) {
	t.Parallel()

	files := parseRuntimeProductionSources(t)
	componentNames := []string{
		"runtimeMutationKernel",
		"authorityComponent",
		"planningTransactionComponent",
		"candidateDispatchComponent",
		"driveDeliveryComponent",
	}
	declared := map[string]*ast.TypeSpec{}
	for _, file := range files {
		for _, declaration := range file.Decls {
			gen, ok := declaration.(*ast.GenDecl)
			if !ok || gen.Tok != token.TYPE {
				continue
			}
			for _, specification := range gen.Specs {
				typeSpec, ok := specification.(*ast.TypeSpec)
				if ok {
					declared[typeSpec.Name.Name] = typeSpec
				}
			}
		}
	}
	for _, name := range componentNames {
		if declared[name] == nil {
			t.Fatalf("O2 component %s is not declared in the runtime package", name)
		}
	}

	for _, name := range []string{"authorityComponent", "planningTransactionComponent", "candidateDispatchComponent", "driveDeliveryComponent"} {
		structure, ok := declared[name].Type.(*ast.StructType)
		if !ok || structure.Fields == nil || len(structure.Fields.List) != 1 {
			t.Fatalf("%s must contain exactly one embedded mutation kernel", name)
		}
		field := structure.Fields.List[0]
		if len(field.Names) != 0 || runtimeReceiverType(field.Type) != "runtimeMutationKernel" {
			t.Fatalf("%s must embed *runtimeMutationKernel, got %#v", name, field.Type)
		}
	}

	controller, ok := declared["RuntimeController"].Type.(*ast.StructType)
	if !ok || controller.Fields == nil {
		t.Fatal("RuntimeController must remain a struct")
	}
	fields := map[string]ast.Expr{}
	for _, field := range controller.Fields.List {
		if len(field.Names) == 0 {
			if runtimeReceiverType(field.Type) != "runtimeMutationKernel" {
				t.Fatalf("RuntimeController anonymous dependency must be *runtimeMutationKernel, got %#v", field.Type)
			}
			continue
		}
		for _, fieldName := range field.Names {
			fields[fieldName.Name] = field.Type
		}
	}
	for fieldName, componentName := range map[string]string{
		"authority":         "authorityComponent",
		"planning":          "planningTransactionComponent",
		"candidateDispatch": "candidateDispatchComponent",
		"driveDelivery":     "driveDeliveryComponent",
	} {
		if runtimeReceiverType(fields[fieldName]) != componentName {
			t.Fatalf("RuntimeController.%s must be *%s, got %#v", fieldName, componentName, fields[fieldName])
		}
	}

	methodsByReceiver := map[string]map[string]bool{}
	for _, file := range files {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Recv == nil || len(function.Recv.List) != 1 {
				continue
			}
			receiver := runtimeReceiverType(function.Recv.List[0].Type)
			if _, tracked := declared[receiver]; !tracked {
				continue
			}
			if methodsByReceiver[receiver] == nil {
				methodsByReceiver[receiver] = map[string]bool{}
			}
			methodsByReceiver[receiver][function.Name.Name] = true
		}
	}
	for receiver, required := range map[string][]string{
		"authorityComponent":           {"reconcileAuthorityBeforeCandidate", "recordFeasibilityFact", "commitPrerequisite", "commitAssessment", "commitGovernanceResolution", "commitReconciliationBatch", "invalidatePrerequisitesForReadOnlyStale"},
		"planningTransactionComponent": {"beginSemanticPlanningMutation", "commitSemanticPlanningMutation", "requireSemanticContract", "ensureCandidateRequestSessionWithExpected", "markCandidateRequestSubmitted", "coordinateProviderRequestRecovery", "invalidateCandidateRequestSessionForPrerequisiteStale"},
		"candidateDispatchComponent":   {"commitEvidence", "commitOperationReceipt", "admitCandidateProposal", "admitAndSealCandidateAttempt", "beginCandidateAttempt", "claimOperationExecution", "invalidateActiveDispatchForPrerequisiteStale"},
		"driveDeliveryComponent":       {"scheduleDriveLease", "claimDriveLease", "consumeRecoveryBudget", "recordDeliveryAcknowledgement", "ensureReconciliationRequest", "requestTermination"},
	} {
		for _, method := range required {
			if !methodsByReceiver[receiver][method] {
				t.Fatalf("O2 component %s is missing owned handler %s", receiver, method)
			}
		}
	}

	// A component may not invoke the public mutation dispatcher or a sibling
	// component.  Reducer calls remain owner-local and are handled by the
	// component that owns the corresponding state-machine facet.
	for _, file := range files {
		ast.Inspect(file, func(node ast.Node) bool {
			function, ok := node.(*ast.FuncDecl)
			if !ok || function.Recv == nil || len(function.Recv.List) != 1 {
				return true
			}
			receiver := runtimeReceiverType(function.Recv.List[0].Type)
			if receiver != "authorityComponent" && receiver != "planningTransactionComponent" && receiver != "candidateDispatchComponent" && receiver != "driveDeliveryComponent" {
				return true
			}
			ast.Inspect(function.Body, func(child ast.Node) bool {
				selector, ok := child.(*ast.SelectorExpr)
				if !ok || selector.Sel == nil {
					return true
				}
				if identifier, ok := selector.X.(*ast.Ident); ok && identifier.Name == "c" && (selector.Sel.Name == "ApplyInternalEvent" || selector.Sel.Name == "authority" || selector.Sel.Name == "planning" || selector.Sel.Name == "candidateDispatch" || selector.Sel.Name == "driveDelivery") {
					t.Fatalf("%s.%s reaches dispatcher or sibling component through %s", receiver, function.Name.Name, selector.Sel.Name)
				}
				return true
			})
			return false
		})
	}
}

func TestRuntimePrivateComponentsUseTheControllerKernelInstance(t *testing.T) {
	t.Parallel()

	controller, err := NewRuntimeController(&controllerRepository{})
	if err != nil {
		t.Fatalf("NewRuntimeController: %v", err)
	}
	if controller.runtimeMutationKernel == nil {
		t.Fatal("controller has no mutation kernel")
	}
	for name, component := range map[string]*runtimeMutationKernel{
		"authority":          controller.authority.runtimeMutationKernel,
		"planning":           controller.planning.runtimeMutationKernel,
		"candidate dispatch": controller.candidateDispatch.runtimeMutationKernel,
		"drive delivery":     controller.driveDelivery.runtimeMutationKernel,
	} {
		if component != controller.runtimeMutationKernel {
			t.Fatalf("%s component owns a second mutation kernel", name)
		}
	}
}

// TestRuntimeControllerComponentMutationsOnlyEnterThroughInternalEventRoutes
// keeps the owner-local dispatcher as the sole production mutation surface.
// Focused tests may use test-only controller adapters, but production
// RuntimeController methods may reach component mutators only while reducing
// an already-classified RuntimeInternalEvent.
func TestRuntimeControllerComponentMutationsOnlyEnterThroughInternalEventRoutes(t *testing.T) {
	t.Parallel()

	components := map[string]struct{}{
		"authority":         {},
		"planning":          {},
		"candidateDispatch": {},
		"driveDelivery":     {},
	}
	for filename, file := range parseRuntimeProductionSources(t) {
		base := filepath.Base(filename)
		if base == "component_facades.go" {
			t.Fatal("RuntimeController component facade is compiled into production")
		}
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Recv == nil || len(function.Recv.List) != 1 || runtimeReceiverType(function.Recv.List[0].Type) != "RuntimeController" {
				continue
			}
			receiverNames := function.Recv.List[0].Names
			if len(receiverNames) != 1 {
				t.Fatalf("RuntimeController.%s must use one named receiver", function.Name.Name)
			}
			receiverName := receiverNames[0].Name
			ast.Inspect(function.Body, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				method, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				component, ok := method.X.(*ast.SelectorExpr)
				if !ok || component.Sel == nil {
					return true
				}
				receiver, ok := component.X.(*ast.Ident)
				if !ok || receiver.Name != receiverName {
					return true
				}
				if _, tracked := components[component.Sel.Name]; !tracked {
					return true
				}
				if !strings.HasPrefix(base, "internal_event_route_") {
					t.Fatalf("production RuntimeController.%s calls %s.%s from %s; route through ApplyInternalEvent", function.Name.Name, component.Sel.Name, method.Sel.Name, base)
				}
				return true
			})
		}
	}
}

// TestFocusedRulePackagesRemainStateless freezes the O3-O6 extraction rule:
// focused rule packages may project facts, but cannot become a second Runtime
// owner through repository/EventHub/aggregate access.
func TestFocusedRulePackagesRemainStateless(t *testing.T) {
	t.Parallel()
	root := filepath.Join("..", "internal")
	for _, packageName := range []string{"semanticplan", "requestcontent", "candidateadmission", "grounding", "proof", "governancecheck"} {
		directory := filepath.Join(root, packageName)
		entries, err := os.ReadDir(directory)
		if err != nil {
			t.Fatalf("read focused package %s: %v", packageName, err)
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
				continue
			}
			filename := filepath.Join(directory, entry.Name())
			file, err := parser.ParseFile(token.NewFileSet(), filename, nil, 0)
			if err != nil {
				t.Fatalf("parse focused package %s: %v", packageName, err)
			}
			for _, imported := range file.Imports {
				path := strings.Trim(imported.Path.Value, "\"")
				forbidden := []string{"/biz/runtime", "/repository", "magicCommon/event", "magicCommon/task"}
				for _, value := range forbidden {
					if strings.Contains(path, value) {
						t.Fatalf("focused package %s imports forbidden owner/I-O surface %q", packageName, path)
					}
				}
			}
			ast.Inspect(file, func(node ast.Node) bool {
				selector, ok := node.(*ast.SelectorExpr)
				if !ok || selector.Sel == nil {
					return true
				}
				switch selector.Sel.Name {
				case "Save", "Update", "Create", "Publish", "Subscribe", "ApplyInternalEvent":
					t.Fatalf("focused package %s contains state/I-O call %s", packageName, selector.Sel.Name)
				}
				return true
			})
		}
	}
}

// TestCandidateAdmissionUsesOnePureEvaluatorEntry freezes O4's split between
// aggregate projection/owner-local rejection sealing and the one structural
// CandidateEvaluator. Adding another Runtime validator call site would restore
// competing first-failure order and is therefore a boundary violation.
func TestCandidateAdmissionUsesOnePureEvaluatorEntry(t *testing.T) {
	t.Parallel()
	files := parseRuntimeProductionSources(t)
	evaluatorCalls := 0
	for filename, file := range files {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil {
				continue
			}
			ast.Inspect(function.Body, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				selector, ok := call.Fun.(*ast.SelectorExpr)
				if !ok || selector.Sel == nil {
					return true
				}
				if qualifier, ok := selector.X.(*ast.Ident); ok && qualifier.Name == "candidateadmission" && selector.Sel.Name == "Evaluate" {
					evaluatorCalls++
					if function.Name.Name != "EvaluateCandidateAdmissionWithManifest" {
						t.Fatalf("candidate evaluator called from %s:%s", filepath.Base(filename), function.Name.Name)
					}
				}
				if function.Name.Name == "EvaluateCandidateAdmissionWithManifest" {
					switch selector.Sel.Name {
					case "ValidateRuntimeCandidateContractManifestForDecision", "ValidateRuntimeCandidateAgainstManifest", "ValidateWithActivation":
						t.Fatalf("Runtime candidate wrapper retained competing validator %s", selector.Sel.Name)
					}
				}
				return true
			})
		}
	}
	if evaluatorCalls != 1 {
		t.Fatalf("candidateadmission.Evaluate production call sites = %d, want exactly 1", evaluatorCalls)
	}
}

// TestO5ProofAndGroundingComparisonsUseFocusedEntries freezes the O5 split:
// Runtime may project aggregate facts and map findings to owner-local events,
// but exact receipt, reconciliation, repair-surface and validation-grounding
// comparisons have one stateless implementation.
func TestO5ProofAndGroundingComparisonsUseFocusedEntries(t *testing.T) {
	t.Parallel()
	files := parseRuntimeProductionSources(t)
	expected := map[string]int{
		"proof.ReconciliationHistoryContainsExactBatch":            1,
		"proof.CompareReconciliationBinding":                       1,
		"proof.RepairSurfaceConfirmationExists":                    1,
		"proof.RepairSurfaceConfirmationIdentityConflict":          1,
		"proof.ValidationFailureObservationMatchesSealedOperation": 1,
		"proof.MatchCoverageReceipt":                               1,
		"grounding.ValidationFailureTargetsAreGrounded":            1,
	}
	actual := make(map[string]int, len(expected))
	forbiddenContractComparisons := map[string]struct{}{
		"MatchCoverageEvidenceBindingReceipt":                {},
		"StableRuntimeCWIReconciliationObservationSetRef":    {},
		"StableRuntimeRepairSurfaceConfirmationHash":         {},
		"StableRuntimeRepairSurfaceConfirmationIdentityHash": {},
		"SameRuntimeRepairSurfaceConfirmation":               {},
		"SameRuntimeRepairSurfaceConfirmationIdentity":       {},
	}
	for filename, file := range files {
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || selector.Sel == nil {
				return true
			}
			qualifier, ok := selector.X.(*ast.Ident)
			if !ok {
				return true
			}
			key := qualifier.Name + "." + selector.Sel.Name
			if _, tracked := expected[key]; tracked {
				actual[key]++
			}
			if qualifier.Name == "runtimecontract" {
				if _, forbidden := forbiddenContractComparisons[selector.Sel.Name]; forbidden {
					t.Fatalf("Runtime production source %s bypasses focused O5 comparison with runtimecontract.%s", filepath.Base(filename), selector.Sel.Name)
				}
			}
			return true
		})
	}
	for key, count := range expected {
		if actual[key] != count {
			t.Fatalf("focused O5 entry %s production call sites = %d, want %d", key, actual[key], count)
		}
	}
}

// TestO6GovernanceComparisonsUseFocusedEntries freezes the O6 split between
// typed governance facts and the authorityComponent mutation transaction.
// Ticket creation, binding mutation, evidence, resume dispatch and CAS remain
// in Runtime; reusable identity/outcome comparisons use governancecheck while
// the persisted bidirectional execution lineage remains a contract invariant.
func TestO6GovernanceComparisonsUseFocusedEntries(t *testing.T) {
	t.Parallel()
	expected := map[string]int{
		"governancecheck.SameRequestRecord":              1,
		"governancecheck.RequestRecordConflicts":         1,
		"governancecheck.SuspendedBindingMatchesRequest": 1,
		"governancecheck.RequestBindsResolution":         1,
		"governancecheck.BindingMatchesResolution":       1,
		"governancecheck.BindingStateAllowsOutcome":      2,
		"governancecheck.ExternalDependencyMatches":      1,
		"governancecheck.GovernedDispatchMatches":        2,
		"governancecheck.DependencyMatches":              2,
		// 原执行/回执查询及只读来源重编都必须反查同一 persisted governance grant。
		"governancecheck.GrantForExecution":                                   5,
		"runtimecontract.ValidateRuntimeOperationGovernanceExecutionBindings": 1,
		"governancecheck.SameGrant":                                           1,
		"governancecheck.EvaluateOperationSuspension":                         1,
		"governancecheck.DependencyForFacts":                                  1,
		"runtimecontract.ValidateRuntimeGovernanceRequestDispatchLineage":     1,
		"governancecheck.BindingMatchesExecution":                             2,
		"governancecheck.BindingMatchesReceipt":                               1,
		"governancecheck.BindingMatchesRequest":                               1,
		"governancecheck.ResolutionRecordMatchesRequest":                      1,
		"governancecheck.ResolutionRecordMatchesDispatch":                     1,
	}
	actual := make(map[string]int, len(expected))
	readSourceGrantCalls := 0
	recordSuspensionReceiver := ""
	for _, file := range parseRuntimeProductionSources(t) {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if ok && function.Name.Name == "compileRuntimeOperationReadSource" {
				ast.Inspect(function.Body, func(node ast.Node) bool {
					call, ok := node.(*ast.CallExpr)
					if !ok {
						return true
					}
					selector, ok := call.Fun.(*ast.SelectorExpr)
					if !ok {
						return true
					}
					qualifier, ok := selector.X.(*ast.Ident)
					if ok && qualifier.Name == "governancecheck" && selector.Sel.Name == "GrantForExecution" {
						readSourceGrantCalls++
					}
					return true
				})
			}
			if !ok || function.Name.Name != "recordOperationGovernanceRequired" || function.Recv == nil || len(function.Recv.List) != 1 {
				continue
			}
			recordSuspensionReceiver = runtimeReceiverType(function.Recv.List[0].Type)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || selector.Sel == nil {
				return true
			}
			qualifier, ok := selector.X.(*ast.Ident)
			if !ok {
				return true
			}
			key := qualifier.Name + "." + selector.Sel.Name
			if _, tracked := expected[key]; tracked {
				actual[key]++
			}
			if qualifier.Name == "runtimecontract" && selector.Sel.Name == "SameRuntimeOperationGovernanceGrant" {
				t.Fatalf("Runtime production bypasses focused governance grant comparison")
			}
			return true
		})
	}
	for key, count := range expected {
		if actual[key] != count {
			t.Fatalf("focused O6 entry %s production call sites = %d, want %d", key, actual[key], count)
		}
	}
	if readSourceGrantCalls != 1 {
		t.Fatalf("只读来源必须恰好反查一次 persisted governance grant: got=%d", readSourceGrantCalls)
	}
	if recordSuspensionReceiver != "candidateDispatchComponent" {
		t.Fatalf("operation governance receipt closeout receiver = %q, want candidateDispatchComponent", recordSuspensionReceiver)
	}
}

// TestO7SemanticPlanningTransactionHasNoMutationCallback freezes the first O7
// cleanup slice. Semantic handlers may share the exact load/validate/CAS
// template, but every business mutation remains visible at its concrete call
// site; a callback, interface, reflection or map-based transaction would hide
// which private event changes which ledger fields.
func TestO7SemanticPlanningTransactionHasNoMutationCallback(t *testing.T) {
	t.Parallel()
	beginCalls := 0
	commitCalls := 0
	legacyDeclarations := 0
	for filename, file := range parseRuntimeProductionSources(t) {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok {
				continue
			}
			if function.Name.Name == "commitSemanticPlanning" {
				legacyDeclarations++
			}
			if function.Recv != nil && len(function.Recv.List) == 1 && runtimeReceiverType(function.Recv.List[0].Type) == "planningTransactionComponent" {
				for _, field := range function.Type.Params.List {
					if _, callback := field.Type.(*ast.FuncType); callback {
						t.Fatalf("planning transaction method %s in %s accepts a mutation callback", function.Name.Name, filepath.Base(filename))
					}
				}
			}
		}
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			switch selector.Sel.Name {
			case "beginSemanticPlanningMutation":
				beginCalls++
			case "commitSemanticPlanningMutation":
				commitCalls++
			}
			return true
		})
	}
	if legacyDeclarations != 0 {
		t.Fatalf("legacy callback transaction declarations = %d, want 0", legacyDeclarations)
	}
	if beginCalls != 11 || commitCalls != 11 {
		t.Fatalf("explicit semantic planning transaction calls begin=%d commit=%d, want 11/11", beginCalls, commitCalls)
	}
}

// TestO7CandidateRequestAndExecutionFacetsRemainNarrow freezes the second O7
// cleanup slice.  Request-session preparation is an SM-3 planning concern and
// may only return a typed SM-2 finding; candidate dispatch must not terminalize
// that finding itself.  The execution coordinator is kept in its own file so
// the candidate receipt/evidence handlers cannot silently become a second
// RuntimeController mutation entry.
func TestO7CandidateRequestAndExecutionFacetsRemainNarrow(t *testing.T) {
	t.Parallel()
	files := parseRuntimeProductionSources(t)
	ensureReceivers := map[string]bool{}
	terminalizerCount := 0
	controllerExecutionMethods := map[string]bool{}
	for filename, file := range files {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Recv == nil || len(function.Recv.List) != 1 {
				continue
			}
			receiver := runtimeReceiverType(function.Recv.List[0].Type)
			switch function.Name.Name {
			case "ensureCandidateRequestSessionWithExpected":
				ensureReceivers[receiver] = true
			case "terminalizeEquivalentAuthorityNoProgress":
				terminalizerCount++
			}
			if receiver == "candidateDispatchComponent" && function.Name.Name == "executeActiveDispatch" {
				if filepath.Base(filename) != "execution_coordinator.go" {
					t.Fatalf("%s remains in mixed executor file %s", function.Name.Name, filepath.Base(filename))
				}
				controllerExecutionMethods[function.Name.Name] = true
			}
		}
	}
	if !ensureReceivers["planningTransactionComponent"] || len(ensureReceivers) != 1 {
		t.Fatalf("candidate request ensure receiver = %v, want planningTransactionComponent only", ensureReceivers)
	}
	if terminalizerCount != 0 {
		t.Fatalf("equivalent-authority terminalizer declarations = %d, want 0", terminalizerCount)
	}
	for _, name := range []string{"executeActiveDispatch"} {
		if !controllerExecutionMethods[name] {
			t.Fatalf("execution coordinator method %s is missing", name)
		}
	}
}

// TestO7ExecutionHandsRecoveryBudgetToSM4 freezes the cross-machine handoff:
// SM-3 execution may report that a bounded replay allowance is required, but
// it may not mutate the SM-4 ledger itself.  The next owner-private event is
// the only production path that consumes that budget and returns an
// operation-identity fence to the retry.
func TestO7ExecutionHandsRecoveryBudgetToSM4(t *testing.T) {
	t.Parallel()
	files := parseRuntimeProductionSources(t)
	for filename, file := range files {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Recv == nil || len(function.Recv.List) != 1 {
				continue
			}
			if runtimeReceiverType(function.Recv.List[0].Type) != "candidateDispatchComponent" || function.Name.Name != "executeActiveDispatch" {
				continue
			}
			ast.Inspect(function.Body, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				selector, ok := call.Fun.(*ast.SelectorExpr)
				if ok && selector.Sel != nil && selector.Sel.Name == "consumeRecoveryBudget" {
					t.Fatalf("%s mutates SM-4 recovery budget from %s", filepath.Base(filename), function.Name.Name)
				}
				return true
			})
		}
	}

	transactionSource := files[filepath.Join(".", "internal_event_transaction.go")]
	if transactionSource == nil {
		// parseRuntimeProductionSources uses absolute-independent paths, so find
		// the file by basename when the parser's map key differs by Go version.
		for filename, file := range files {
			if filepath.Base(filename) == "internal_event_transaction.go" {
				transactionSource = file
				break
			}
		}
	}
	if transactionSource == nil {
		t.Fatal("internal event transaction source is missing")
	}
	seenOperationID, seenBudgetAttempt := false, false
	ast.Inspect(transactionSource, func(node ast.Node) bool {
		field, ok := node.(*ast.Field)
		if !ok || field.Names == nil {
			return true
		}
		for _, name := range field.Names {
			seenOperationID = seenOperationID || name.Name == "RecoveryOperationID"
			seenBudgetAttempt = seenBudgetAttempt || name.Name == "RecoveryBudgetAttempt"
		}
		return true
	})
	if seenOperationID || seenBudgetAttempt {
		t.Fatalf("process-local transaction recovery authority returned: operation=%v attempt=%v", seenOperationID, seenBudgetAttempt)
	}
	deliverySource := files[filepath.Join(".", "internal_event_delivery.go")]
	if deliverySource == nil {
		for filename, file := range files {
			if filepath.Base(filename) == "internal_event_delivery.go" {
				deliverySource = file
				break
			}
		}
	}
	if deliverySource == nil {
		t.Fatal("internal event delivery source is missing")
	}
	seenAuthorizationID, seenClaimID := false, false
	ast.Inspect(deliverySource, func(node ast.Node) bool {
		field, ok := node.(*ast.Field)
		if !ok || field.Names == nil {
			return true
		}
		for _, name := range field.Names {
			seenOperationID = seenOperationID || name.Name == "RecoveryOperationID"
			seenAuthorizationID = seenAuthorizationID || name.Name == "RecoveryAuthorizationID"
			seenClaimID = seenClaimID || name.Name == "RecoveryClaimID"
		}
		return true
	})
	if !seenOperationID || !seenAuthorizationID || !seenClaimID {
		t.Fatalf("durable recovery-ledger fact identity fields missing: operation=%v authorization=%v claim=%v", seenOperationID, seenAuthorizationID, seenClaimID)
	}
}

// TestO7RuntimeFilesKeepOneFacetPerTransitionFile makes the physical cleanup
// observable.  The package still has one Orchestrator owner, but a future
// change must not put authority, candidate, delivery and execution handlers
// back into a catch-all controller/executor file.
func TestO7RuntimeFilesKeepOneFacetPerTransitionFile(t *testing.T) {
	t.Parallel()
	want := map[string]map[string]bool{
		"controller.go": {
			"RuntimeController.initialize":                   true,
			"RuntimeController.Load":                         true,
			"RuntimeController.ReleaseHistoricalCompilation": true,
		},
		"authority_transition.go": {
			"authorityComponent.reconcileAuthorityBeforeCandidate": true,
			"authorityComponent.recordFeasibilityFact":             true,
			"authorityComponent.recordFailureFact":                 true,
			"authorityComponent.recordDriveFailureFact":            true,
			"authorityComponent.commitFailureFact":                 true,
			"authorityComponent.commitPrerequisite":                true,
		},
		"authority_assessment_transition.go": {"authorityComponent.commitAssessment": true},
		"drive_delivery_transition.go":       {"driveDeliveryComponent.requestTermination": true},
		"execution_coordinator.go": {
			"candidateDispatchComponent.executeActiveDispatch": true,
		},
	}
	files := parseRuntimeProductionSources(t)
	seen := make(map[string]map[string]bool)
	for filename, file := range files {
		base := filepath.Base(filename)
		if want[base] == nil {
			continue
		}
		if seen[base] == nil {
			seen[base] = map[string]bool{}
		}
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Recv == nil || len(function.Recv.List) != 1 {
				continue
			}
			key := runtimeReceiverType(function.Recv.List[0].Type) + "." + function.Name.Name
			seen[base][key] = true
			trackedReceiver := strings.HasSuffix(key, "Component."+function.Name.Name) || strings.HasPrefix(key, "RuntimeController.")
			if trackedReceiver && !want[base][key] {
				t.Fatalf("unexpected transition handler %s in %s", key, base)
			}
		}
	}
	for filename, expected := range want {
		for key := range expected {
			if !seen[filename][key] {
				t.Fatalf("transition handler %s is missing from %s", key, filename)
			}
		}
	}
	for filename, file := range files {
		base := filepath.Base(filename)
		if base != "semantic_planning.go" {
			continue
		}
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if ok {
				if selector, selectorOK := call.Fun.(*ast.SelectorExpr); selectorOK && selector.Sel.Name == "Commit" {
					if receiver, receiverOK := selector.X.(*ast.SelectorExpr); receiverOK && receiver.Sel.Name == "transitions" {
						t.Fatalf("semantic_planning.go retains a direct CAS call; use semantic transaction files")
					}
				}
			}
			if function, functionOK := node.(*ast.FuncDecl); functionOK && function.Recv != nil && len(function.Recv.List) == 1 && runtimeReceiverType(function.Recv.List[0].Type) == "RuntimeController" {
				t.Fatalf("semantic_planning.go contains a RuntimeController transition handler")
			}
			return true
		})
	}
}

func parseRuntimeProductionSources(t *testing.T) map[string]*ast.File {
	t.Helper()
	fileSet := token.NewFileSet()
	packages, err := parser.ParseDir(fileSet, ".", func(info os.FileInfo) bool {
		return strings.HasSuffix(info.Name(), ".go") && !strings.HasSuffix(info.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatalf("parse Runtime production sources: %v", err)
	}
	parsed := packages["runtime"]
	if parsed == nil {
		t.Fatal("Runtime production package source is missing")
	}
	return parsed.Files
}

func runtimeReceiverType(expression ast.Expr) string {
	switch typed := expression.(type) {
	case *ast.Ident:
		return typed.Name
	case *ast.StarExpr:
		return runtimeReceiverType(typed.X)
	default:
		return ""
	}
}

func assertRuntimeExportedMethods(t *testing.T, methods map[string][]string, receiver string, expected []string) {
	t.Helper()
	actual := append([]string(nil), methods[receiver]...)
	sort.Strings(actual)
	sort.Strings(expected)
	if strings.Join(actual, "\x00") != strings.Join(expected, "\x00") {
		t.Fatalf("%s exported methods = %v, want %v", receiver, actual, expected)
	}
}

// Private continuation helpers may be used only by the complete read gates
// above them. This is a lexical composition, never a caller's validated flag.
func TestApplicabilityReadContinuationsStayBehindCompletePublicGates(t *testing.T) {
	want := map[string]map[string]int{
		"compileRuntimeOperationReadSource":          {"CompileRuntimeOperationReadSource": 1, "compileRuntimeEvidenceApplicabilityRequest": 1, "compileRuntimeEvidenceApplicabilityRead": 1},
		"compileRuntimeEvidenceApplicabilityRequest": {"CompileRuntimeEvidenceApplicabilityRequest": 1, "nextRuntimeEvidenceApplicabilityRequest": 1},
		"nextRuntimeEvidenceApplicabilityRequest":    {"NextRuntimeEvidenceApplicabilityRequest": 1, "compileRuntimeEvidenceApplicabilityRead": 1},
		"compileRuntimeEvidenceApplicabilityRead":    {"CompileRuntimeEvidenceApplicabilityRead": 1, "CompileEvidenceApplicabilityRead": 1},
	}
	actual := map[string]map[string]int{}
	gates := map[string]int{}
	ownerCompiler, ownerGate, ownerImage := 0, 0, 0
	for _, file := range parseRuntimeProductionSources(t) {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				if id, ok := call.Fun.(*ast.Ident); ok && want[id.Name] != nil {
					if actual[id.Name] == nil {
						actual[id.Name] = map[string]int{}
					}
					actual[id.Name][fn.Name.Name]++
				}
				if fn.Name.Name == "CompileEvidenceApplicabilityRead" {
					if id, ok := call.Fun.(*ast.Ident); ok {
						switch id.Name {
						case "validate":
							ownerGate++
						case "ownerStateSourceImage":
							ownerImage++
						}
					}
					if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Compile" {
						if field, ok := sel.X.(*ast.SelectorExpr); ok && field.Sel.Name == "historicalCompiler" {
							ownerCompiler++
						}
					}
				}
				if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Validate" {
					if id, ok := sel.X.(*ast.Ident); ok && id.Name == "aggregate" {
						gates[fn.Name.Name]++
					}
				}
				return true
			})
		}
	}
	if !reflect.DeepEqual(want, actual) {
		t.Fatalf("unchecked read continuation callers: %v", actual)
	}
	if ownerCompiler != 1 || ownerGate != 1 || ownerImage != 1 {
		t.Fatal("owner read must enter exactly one complete raw-image historical compiler gate")
	}
	for _, entry := range []string{"CompileRuntimeOperationReadSource", "CompileRuntimeEvidenceApplicabilityRequest", "NextRuntimeEvidenceApplicabilityRequest", "CompileRuntimeEvidenceApplicabilityRead"} {
		if gates[entry] != 1 {
			t.Fatalf("%s must run its complete aggregate gate exactly once", entry)
		}
	}
}
