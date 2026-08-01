package run

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"testing"
)

// C0 regression gate: recovery inventory is a Run-owned opaque projection.
// Keep this structural test beside the owner rather than relying on a fixture
// whose populated aggregate fields could accidentally make an unauthorized
// read look harmless.
func TestRecoveryInventoryDoesNotInspectOrchestratorState(t *testing.T) {
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve recovery inventory source path")
	}
	ownerFile := filepath.Join(filepath.Dir(sourceFile), "owner.go")
	parsed, err := parser.ParseFile(token.NewFileSet(), ownerFile, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", ownerFile, err)
	}
	var inventory *ast.FuncDecl
	for _, declaration := range parsed.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if ok && function.Name.Name == "buildRuntimeRecoveryInventory" {
			inventory = function
			break
		}
	}
	if inventory == nil {
		t.Fatal("buildRuntimeRecoveryInventory not found")
	}
	forbidden := map[string]struct{}{
		"Decision": {}, "ActiveDispatch": {}, "GovernanceRequests": {}, "GovernanceResolutions": {},
		"DeliveryAcknowledgements": {}, "Recovery": {}, "TerminalDeliveryAttempts": {},
		"BlockingTicketID": {}, "BlockingApprovalID": {},
	}
	var found string
	ast.Inspect(inventory.Body, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if _, prohibited := forbidden[selector.Sel.Name]; prohibited {
			found = selector.Sel.Name
			return false
		}
		return true
	})
	if found != "" {
		t.Fatalf("Run recovery inventory inspects Orchestrator-owned field %q", found)
	}
}
