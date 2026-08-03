package events

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestEventLaneCallersNameReceivingOwnerInventory keeps lane selection local
// and auditable. Production callers may consume one receiving owner's
// Contracts(), but must not rebuild a cross-owner topic inventory.
func TestEventLaneCallersNameReceivingOwnerInventory(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve current test file")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", "..", ".."))
	fset := token.NewFileSet()
	err := filepath.WalkDir(filepath.Join(root, "internal"), func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil {
				continue
			}
			eventOwners := eventVariableOwners(function.Body)
			ast.Inspect(function.Body, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				if isAppendOfOwnerContracts(call) {
					t.Errorf("%s: production code aggregates owner Contracts() inventories", fset.Position(call.Pos()))
				}
				contractsArg, topicArg, matched := laneBindingArguments(call)
				if !matched {
					return true
				}
				contractOwner := contractsOwner(contractsArg)
				topicOwner := topicExpressionOwner(topicArg, eventOwners)
				if contractOwner == "" {
					t.Errorf("%s: lane binding does not explicitly pass one owner Contracts() inventory", fset.Position(call.Pos()))
					return true
				}
				if topicOwner != "" && contractOwner != topicOwner {
					t.Errorf("%s: lane contract owner %s does not match topic receiver owner %s", fset.Position(call.Pos()), contractOwner, topicOwner)
				}
				return true
			})
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func eventVariableOwners(body *ast.BlockStmt) map[string]string {
	owners := make(map[string]string)
	ast.Inspect(body, func(node ast.Node) bool {
		assignment, ok := node.(*ast.AssignStmt)
		if !ok || len(assignment.Lhs) != len(assignment.Rhs) {
			return true
		}
		for index, rhs := range assignment.Rhs {
			call, ok := rhs.(*ast.CallExpr)
			if !ok || calledFunctionName(call.Fun) != "NewEventWithContext" || len(call.Args) == 0 {
				continue
			}
			name, ok := assignment.Lhs[index].(*ast.Ident)
			if !ok {
				continue
			}
			if owner := topicExpressionOwner(call.Args[0], nil); owner != "" {
				owners[name.Name] = owner
			}
		}
		return true
	})
	return owners
}

func laneBindingArguments(call *ast.CallExpr) (ast.Expr, ast.Expr, bool) {
	name := calledFunctionName(call.Fun)
	switch name {
	case "EventHubLaneKeyFor":
		if len(call.Args) >= 2 {
			return call.Args[0], call.Args[1], true
		}
	case "BindLane", "bindOwnerLane", "bindOwnerCommandLane", "bindGovernanceCommandLane":
		if len(call.Args) >= 3 {
			return call.Args[1], call.Args[2], true
		}
	}
	return nil, nil, false
}

func calledFunctionName(expr ast.Expr) string {
	switch value := expr.(type) {
	case *ast.Ident:
		return value.Name
	case *ast.SelectorExpr:
		return value.Sel.Name
	default:
		return ""
	}
}

func contractsOwner(expr ast.Expr) string {
	if name, ok := expr.(*ast.Ident); ok && name.Name == "ownerContracts" {
		return name.Name
	}
	call, ok := expr.(*ast.CallExpr)
	if !ok || len(call.Args) != 0 {
		return ""
	}
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != "Contracts" {
		return ""
	}
	owner, _ := selector.X.(*ast.Ident)
	if owner == nil {
		return ""
	}
	return owner.Name
}

func topicExpressionOwner(expr ast.Expr, eventOwners map[string]string) string {
	switch value := expr.(type) {
	case *ast.SelectorExpr:
		owner, ok := value.X.(*ast.Ident)
		if ok && strings.HasPrefix(value.Sel.Name, "Topic") {
			return owner.Name
		}
	case *ast.CallExpr:
		selector, ok := value.Fun.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != "ID" {
			return ""
		}
		name, ok := selector.X.(*ast.Ident)
		if ok {
			return eventOwners[name.Name]
		}
	}
	return ""
}

func isAppendOfOwnerContracts(call *ast.CallExpr) bool {
	if calledFunctionName(call.Fun) != "append" {
		return false
	}
	for _, argument := range call.Args {
		if contractsOwner(argument) != "" {
			return true
		}
	}
	return false
}
