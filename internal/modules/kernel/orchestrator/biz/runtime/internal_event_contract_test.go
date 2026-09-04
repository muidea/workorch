package runtime

import (
	"context"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeInternalFactsHaveCompleteOwnerPrivateClassification(t *testing.T) {
	kinds, dispatched, sourcePayloadClasses := runtimeInternalEventSourceFacts(t)
	seenValues := map[RuntimeInternalEventKind]string{}
	for name, kind := range kinds {
		if previous, exists := seenValues[kind]; exists {
			t.Fatalf("runtime internal fact value %q is shared by %s and %s", kind, previous, name)
		}
		seenValues[kind] = name

		metadata, ok := runtimeInternalFactMetadata(kind)
		if !ok {
			t.Fatalf("runtime internal fact %s (%q) has no owner-private classification", name, kind)
		}
		if metadata.Layer != eventcontract.LayerOrchestratorPrivateFact ||
			metadata.StateMachine == "" || metadata.StateMachine == eventcontract.StateMachineNone ||
			metadata.ReducerOwner != "orchestrator" || metadata.MachineRole != eventcontract.StateMachineRoleOwnerInternalReducer ||
			metadata.EffectClass == "" || metadata.SourcePolicy == "" ||
			metadata.ScopePolicy != eventcontract.ScopePolicyWorkspaceRun ||
			metadata.IdentityPolicy == "" ||
			metadata.TransitionTarget != eventcontract.TransitionTargetReceiverOwnerPrivate {
			t.Fatalf("runtime internal fact %s has incomplete classification: %+v", name, metadata)
		}
		if !dispatched[name] {
			t.Fatalf("runtime internal fact %s is not handled by ApplyInternalEvent", name)
		}
		payloadClass, payloadClassified := runtimeInternalFactPayloadClass(kind)
		if !payloadClassified {
			t.Fatalf("runtime internal fact %s has no concrete payload classification", name)
		}
		if payloadClass != sourcePayloadClasses[name] {
			t.Fatalf("runtime internal fact %s payload class = %s, want source class %s", name, payloadClass, sourcePayloadClasses[name])
		}
	}
	if len(kinds) != len(dispatched) || len(kinds) != len(sourcePayloadClasses) {
		t.Fatalf("dispatcher/internal fact/payload count mismatch: kinds=%d dispatched=%d payloads=%d", len(kinds), len(dispatched), len(sourcePayloadClasses))
	}
}

// TestRuntimeInternalFactsStayWithinStateMachineFences makes the SM-1~SM-4
// ownership split executable. Orchestrator never owns an SM-1 transition; its
// activation input and authority facts reduce through SM-2, candidate/dispatch
// transactions through SM-3, and delivery/recovery through SM-4. Bounded
// transaction-shaped result facts keep their exact operation/request identity
// while declaring the machine that owns the coordinated transition: operation
// evidence/stale/governance facts belong to SM-2, and provider request recovery
// belongs to SM-4.
func TestRuntimeInternalFactsStayWithinStateMachineFences(t *testing.T) {
	kinds, _, _ := runtimeInternalEventSourceFacts(t)
	for name, kind := range kinds {
		payloadClass, ok := runtimeInternalFactPayloadClass(kind)
		if !ok {
			t.Fatalf("runtime internal fact %s has no payload class", name)
		}
		metadata, ok := runtimeInternalFactMetadata(kind)
		if !ok {
			t.Fatalf("runtime internal fact %s has no state-machine metadata", name)
		}
		if metadata.StateMachine == eventcontract.StateMachineSM1 {
			t.Fatalf("Orchestrator internal fact %s attempts to own an SM-1 transition", name)
		}
		if metadata.ReducerOwner != eventcontract.ReducerOwnerForStateMachine(metadata.StateMachine) || metadata.MachineRole != eventcontract.StateMachineRoleOwnerInternalReducer {
			t.Fatalf("runtime internal fact %s bypasses canonical reducer ownership: %+v", name, metadata)
		}

		want := map[runtimeInternalEventPayloadClass]eventcontract.StateMachine{
			runtimeInternalEventPayloadActivation:  eventcontract.StateMachineSM2,
			runtimeInternalEventPayloadAuthority:   eventcontract.StateMachineSM2,
			runtimeInternalEventPayloadTransaction: eventcontract.StateMachineSM3,
			runtimeInternalEventPayloadDelivery:    eventcontract.StateMachineSM4,
		}[payloadClass]
		switch kind {
		case RuntimeInternalEventOperationEvidenceObserved,
			RuntimeInternalEventOperationPrerequisiteStaleObserved,
			RuntimeInternalEventOperationGovernanceRequiredObserved:
			want = eventcontract.StateMachineSM2
		case RuntimeInternalEventProviderRequestRecoveryCoordinated:
			want = eventcontract.StateMachineSM4
		}
		if metadata.StateMachine != want {
			t.Fatalf("runtime internal fact %s (%s payload) is classified as %s, want %s", name, payloadClass, metadata.StateMachine, want)
		}
	}
}

func TestRuntimeInternalEventDispatcherRoutesEveryKindToExactlyOnePayloadFamily(t *testing.T) {
	kinds, _, _ := runtimeInternalEventSourceFacts(t)
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, "internal_event.go", nil, 0)
	if err != nil {
		t.Fatalf("parse runtime internal event dispatcher: %v", err)
	}

	var dispatcher *ast.FuncDecl
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Name.Name != "ApplyInternalEvent" {
			continue
		}
		dispatcher = function
		break
	}
	if dispatcher == nil {
		t.Fatal("RuntimeController.ApplyInternalEvent is missing")
	}

	var gate *ast.SwitchStmt
	ast.Inspect(dispatcher.Body, func(node ast.Node) bool {
		statement, ok := node.(*ast.SwitchStmt)
		if !ok {
			return true
		}
		gate = statement
		return false
	})
	if gate == nil {
		t.Fatal("ApplyInternalEvent has no exhaustive kind gate")
	}

	wantRoute := map[runtimeInternalEventPayloadClass]string{
		runtimeInternalEventPayloadActivation:  "applyActivationInternalEvent",
		runtimeInternalEventPayloadAuthority:   "applyAuthorityInternalEvent",
		runtimeInternalEventPayloadTransaction: "applyTransactionInternalEvent",
		runtimeInternalEventPayloadDelivery:    "applyDeliveryInternalEvent",
	}
	counts := map[string]int{}
	defaultFailClosed := false
	for _, statement := range gate.Body.List {
		clause, ok := statement.(*ast.CaseClause)
		if !ok {
			continue
		}
		route := runtimeInternalEventClauseRoute(clause)
		if len(clause.List) == 0 {
			defaultFailClosed = route == "unsupportedRuntimeInternalEvent"
			continue
		}
		for _, expression := range clause.List {
			identifier, ok := expression.(*ast.Ident)
			if !ok {
				t.Fatalf("dispatcher case uses non-static kind expression %T", expression)
			}
			kind, known := kinds[identifier.Name]
			if !known {
				t.Fatalf("dispatcher handles undeclared kind %s", identifier.Name)
			}
			payloadClass, classified := runtimeInternalFactPayloadClass(kind)
			if !classified {
				t.Fatalf("dispatcher kind %s has no payload class", identifier.Name)
			}
			if route != wantRoute[payloadClass] {
				t.Fatalf("dispatcher kind %s routes through %q, want %q", identifier.Name, route, wantRoute[payloadClass])
			}
			counts[identifier.Name]++
		}
	}
	if !defaultFailClosed {
		t.Fatal("ApplyInternalEvent default route does not fail closed")
	}
	for name := range kinds {
		if counts[name] != 1 {
			t.Fatalf("dispatcher kind %s route count = %d, want exactly 1", name, counts[name])
		}
	}
}

func runtimeInternalEventClauseRoute(clause *ast.CaseClause) string {
	for _, statement := range clause.Body {
		returnStatement, ok := statement.(*ast.ReturnStmt)
		if !ok || len(returnStatement.Results) == 0 {
			continue
		}
		call, ok := returnStatement.Results[0].(*ast.CallExpr)
		if !ok {
			continue
		}
		switch function := call.Fun.(type) {
		case *ast.Ident:
			return function.Name
		case *ast.SelectorExpr:
			return function.Sel.Name
		}
	}
	return ""
}

func TestRuntimeInternalEventEnvelopeContainsOnlyScopeAndConcretePayloads(t *testing.T) {
	typeOfEvent := reflect.TypeOf(RuntimeInternalEvent{})
	expected := []struct {
		name   string
		typeOf reflect.Type
	}{
		{name: "Kind", typeOf: reflect.TypeOf(RuntimeInternalEventKind(""))},
		{name: "Scope", typeOf: reflect.TypeOf(RuntimeInternalEventScope{})},
		{name: "Activation", typeOf: reflect.TypeOf((*RuntimeInternalActivationPayload)(nil))},
		{name: "Authority", typeOf: reflect.TypeOf((*RuntimeInternalAuthorityPayload)(nil))},
		{name: "Transaction", typeOf: reflect.TypeOf((*RuntimeInternalTransactionPayload)(nil))},
		{name: "Delivery", typeOf: reflect.TypeOf((*RuntimeInternalDeliveryPayload)(nil))},
	}
	if typeOfEvent.NumField() != len(expected) {
		t.Fatalf("RuntimeInternalEvent fields = %d, want %d narrow envelope fields", typeOfEvent.NumField(), len(expected))
	}
	for index, field := range expected {
		actual := typeOfEvent.Field(index)
		if actual.Name != field.name || actual.Type != field.typeOf {
			t.Fatalf("RuntimeInternalEvent field[%d] = %s %s, want %s %s", index, actual.Name, actual.Type, field.name, field.typeOf)
		}
	}
}

type internalEventEnvelopeRepository struct {
	loads int
	saves int
}

func (r *internalEventEnvelopeRepository) Load(context.Context, common.WorkspaceID, common.RunID) (RuntimeAggregate, error) {
	r.loads++
	return RuntimeAggregate{}, common.ErrNotFound
}

func (r *internalEventEnvelopeRepository) Save(context.Context, RuntimeAggregate, uint64) error {
	r.saves++
	return nil
}

func TestRuntimeInternalEventEnvelopeRejectsMalformedPayloadBeforeLoadOrCAS(t *testing.T) {
	repository := &internalEventEnvelopeRepository{}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	scope := RuntimeInternalEventScope{WorkspaceID: "ws-envelope", RunID: "run-envelope"}
	tests := []struct {
		name  string
		event RuntimeInternalEvent
	}{
		{
			name:  "missing payload",
			event: RuntimeInternalEvent{Kind: RuntimeInternalEventReconcileAuthority, Scope: scope},
		},
		{
			name: "wrong payload",
			event: RuntimeInternalEvent{
				Kind:        RuntimeInternalEventReconcileAuthority,
				Scope:       scope,
				Transaction: &RuntimeInternalTransactionPayload{},
			},
		},
		{
			name: "multiple payloads",
			event: RuntimeInternalEvent{
				Kind:        RuntimeInternalEventReconcileAuthority,
				Scope:       scope,
				Authority:   &RuntimeInternalAuthorityPayload{},
				Transaction: &RuntimeInternalTransactionPayload{},
			},
		},
		{
			name: "missing scope",
			event: RuntimeInternalEvent{
				Kind:      RuntimeInternalEventReconcileAuthority,
				Authority: &RuntimeInternalAuthorityPayload{},
			},
		},
		{
			name: "unknown metadata",
			event: RuntimeInternalEvent{
				Kind:      RuntimeInternalEventKind("unknown"),
				Scope:     scope,
				Authority: &RuntimeInternalAuthorityPayload{},
			},
		},
		{
			name: "operation recovery budget missing operation identity",
			event: RuntimeInternalEvent{
				Kind: RuntimeInternalEventRecoveryBudgetConsumed, Scope: scope,
				Delivery: &RuntimeInternalDeliveryPayload{RecoveryBudgetKind: runtimecontract.RuntimeRecoveryBudgetOperationRecovery},
			},
		},
		{
			name: "split CAS reload budget event",
			event: RuntimeInternalEvent{
				Kind: RuntimeInternalEventRecoveryBudgetConsumed, Scope: scope,
				Delivery: &RuntimeInternalDeliveryPayload{RecoveryBudgetKind: runtimecontract.RuntimeRecoveryBudgetCASReload},
			},
		},
		{
			name: "split CAS reload budget event also carries operation identity",
			event: RuntimeInternalEvent{
				Kind: RuntimeInternalEventRecoveryBudgetConsumed, Scope: scope,
				Delivery: &RuntimeInternalDeliveryPayload{RecoveryBudgetKind: runtimecontract.RuntimeRecoveryBudgetCASReload, RecoveryOperationID: "op-recovery"},
			},
		},
		{
			name: "operation recovery claim missing authorization identity",
			event: RuntimeInternalEvent{
				Kind: RuntimeInternalEventOperationRecoveryClaimed, Scope: scope,
				Delivery: &RuntimeInternalDeliveryPayload{RecoveryOperationID: "op-recovery", LeaseNow: "2026-08-30T00:00:00Z"},
			},
		},
		{
			name: "operation recovery claim carries budget kind",
			event: RuntimeInternalEvent{
				Kind: RuntimeInternalEventOperationRecoveryClaimed, Scope: scope,
				Delivery: &RuntimeInternalDeliveryPayload{RecoveryOperationID: "op-recovery", RecoveryAuthorizationID: "auth-recovery", RecoveryBudgetKind: runtimecontract.RuntimeRecoveryBudgetOperationRecovery, LeaseNow: "2026-08-30T00:00:00Z"},
			},
		},
		{
			name: "receipt without exact owner source",
			event: RuntimeInternalEvent{
				Kind:  RuntimeInternalEventOperationReceiptObserved,
				Scope: scope,
				Transaction: &RuntimeInternalTransactionPayload{
					OperationReceipt: &runtimecontract.RuntimeOperationReceipt{},
				},
			},
		},
		{
			name: "receipt source on unrelated transaction",
			event: RuntimeInternalEvent{
				Kind:  RuntimeInternalEventCandidateRequestSubmitted,
				Scope: scope,
				Transaction: &RuntimeInternalTransactionPayload{
					OperationReceiptOwnerSource: runtimecontract.RuntimeModuleFile,
				},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, applyErr := controller.ApplyInternalEvent(context.Background(), test.event)
			if !errors.Is(applyErr, common.ErrInvalidArgument) {
				t.Fatalf("ApplyInternalEvent error = %v, want invalid argument", applyErr)
			}
			if repository.loads != 0 || repository.saves != 0 {
				t.Fatalf("malformed envelope reached persistence: loads=%d saves=%d", repository.loads, repository.saves)
			}
		})
	}
}

func TestRuntimeInternalEventEnvelopeAcceptsDispatchAndCompleteRecoveryLedgerFacts(t *testing.T) {
	scope := RuntimeInternalEventScope{WorkspaceID: "ws-envelope", RunID: "run-envelope"}
	for _, event := range []RuntimeInternalEvent{
		{Kind: RuntimeInternalEventDispatchExecutionRequested, Scope: scope, Transaction: &RuntimeInternalTransactionPayload{}},
		{Kind: RuntimeInternalEventRecoveryBudgetConsumed, Scope: scope, Delivery: &RuntimeInternalDeliveryPayload{
			RecoveryBudgetKind: runtimecontract.RuntimeRecoveryBudgetOperationRecovery, RecoveryOperationID: "op-recovery", LeaseNow: "2026-08-30T00:00:00Z",
		}},
		{Kind: RuntimeInternalEventOperationRecoveryClaimed, Scope: scope, Delivery: &RuntimeInternalDeliveryPayload{
			RecoveryOperationID: "op-recovery", RecoveryAuthorizationID: "auth-recovery", LeaseNow: "2026-08-30T00:00:00Z",
		}},
		{Kind: RuntimeInternalEventOperationRecoveryCallReturned, Scope: scope, Delivery: &RuntimeInternalDeliveryPayload{
			RecoveryOperationID: "op-recovery", RecoveryAuthorizationID: "auth-recovery", RecoveryClaimID: "claim-recovery",
		}},
	} {
		if err := event.validateEnvelope(); err != nil {
			t.Fatalf("valid dispatch recovery envelope rejected: %v", err)
		}
	}
}

// runtimeInternalEventSourceFacts derives completeness from the owner-private
// source itself. The test intentionally keeps no manually maintained global
// topic/type index.
func runtimeInternalEventSourceFacts(t *testing.T) (map[string]RuntimeInternalEventKind, map[string]bool, map[string]runtimeInternalEventPayloadClass) {
	t.Helper()
	fileSet := token.NewFileSet()
	packages, err := parser.ParseDir(fileSet, ".", func(info os.FileInfo) bool {
		return strings.HasPrefix(info.Name(), "internal_event") && strings.HasSuffix(info.Name(), ".go") && !strings.HasSuffix(info.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatalf("parse runtime internal fact sources: %v", err)
	}
	parsed := packages["runtime"]
	if parsed == nil || parsed.Files["internal_event.go"] == nil {
		t.Fatal("runtime internal event dispatcher source is missing")
	}

	kinds := map[string]RuntimeInternalEventKind{}
	payloadClasses := map[string]runtimeInternalEventPayloadClass{}
	for filename, file := range parsed.Files {
		sourceClass := runtimeInternalEventSourcePayloadClass(filepath.Base(filename))
		ast.Inspect(file, func(node ast.Node) bool {
			declaration, ok := node.(*ast.GenDecl)
			if !ok || declaration.Tok != token.CONST {
				return true
			}
			for _, spec := range declaration.Specs {
				value, ok := spec.(*ast.ValueSpec)
				if !ok || len(value.Names) != 1 || len(value.Values) != 1 {
					continue
				}
				typeName, typed := value.Type.(*ast.Ident)
				literal, literalOK := value.Values[0].(*ast.BasicLit)
				if !typed || typeName.Name != "RuntimeInternalEventKind" || !literalOK || literal.Kind != token.STRING {
					continue
				}
				decoded, decodeErr := strconv.Unquote(literal.Value)
				if decodeErr != nil {
					t.Fatalf("decode runtime internal fact %s: %v", value.Names[0].Name, decodeErr)
				}
				kinds[value.Names[0].Name] = RuntimeInternalEventKind(decoded)
				if sourceClass == "" {
					t.Fatalf("runtime internal fact %s is not declared in a concrete payload source", value.Names[0].Name)
				}
				payloadClasses[value.Names[0].Name] = sourceClass
			}
			return false
		})
	}
	if len(kinds) == 0 {
		t.Fatal("runtime internal fact vocabulary is empty")
	}

	dispatched := map[string]bool{}
	ast.Inspect(parsed.Files["internal_event.go"], func(node ast.Node) bool {
		clause, ok := node.(*ast.CaseClause)
		if !ok {
			return true
		}
		for _, expression := range clause.List {
			identifier, identifierOK := expression.(*ast.Ident)
			if identifierOK {
				if _, known := kinds[identifier.Name]; known {
					dispatched[identifier.Name] = true
				}
			}
		}
		return true
	})
	return kinds, dispatched, payloadClasses
}

func runtimeInternalEventSourcePayloadClass(filename string) runtimeInternalEventPayloadClass {
	switch filename {
	case "internal_event_activation.go":
		return runtimeInternalEventPayloadActivation
	case "internal_event_authority.go":
		return runtimeInternalEventPayloadAuthority
	case "internal_event_transaction.go":
		return runtimeInternalEventPayloadTransaction
	case "internal_event_delivery.go":
		return runtimeInternalEventPayloadDelivery
	default:
		return ""
	}
}
