package activationports

import (
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

// Verification only: the shape guard and the hand-written typed decoder must
// evolve together. Merely adding a name to an allowlist must not silently
// claim a producer/consumer exists. This index is never used by production.
func TestRuntimeProfileShapeMatchesTypedDecoderFields(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "runtime_activation.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var decoder *ast.FuncDecl
	for _, declaration := range file.Decls {
		if f, ok := declaration.(*ast.FuncDecl); ok && f.Name.Name == "runtimeExecutionContract" {
			decoder = f
		}
	}
	if decoder == nil {
		t.Fatal("typed decoder not found")
	}
	children := map[string]string{}
	for _, kind := range []string{"profile", "recipe", "evidence"} {
		for key, child := range runtimeProfileFields(kind).arrays {
			children[key] = child
		}
	}
	seen := map[string]bool{}
	ast.Inspect(decoder.Body, func(node ast.Node) bool {
		loop, ok := node.(*ast.RangeStmt)
		if !ok {
			return true
		}
		kind, variable := "", ""
		if id, ok := loop.X.(*ast.Ident); ok && id.Name == "profileIDs" {
			kind, variable = "profile", "profile"
		} else if call, ok := loop.X.(*ast.CallExpr); ok && len(call.Args) == 1 {
			if fn, ok := call.Fun.(*ast.Ident); ok && fn.Name == "runtimeMapSlice" {
				_, key := profileDecoderIndex(call.Args[0])
				kind = children[key]
				if id, ok := loop.Value.(*ast.Ident); ok {
					variable = id.Name
				}
			}
		}
		if kind == "" {
			return true
		}
		actual := map[string]string{}
		ast.Inspect(loop.Body, func(node ast.Node) bool {
			if nested, ok := node.(*ast.RangeStmt); ok {
				if call, ok := nested.X.(*ast.CallExpr); ok && len(call.Args) == 1 {
					owner, key := profileDecoderIndex(call.Args[0])
					if owner == variable && key != "" {
						actual[key] = "objects"
					}
				}
				return false
			}
			if call, ok := node.(*ast.CallExpr); ok && len(call.Args) == 1 {
				if fn, ok := call.Fun.(*ast.Ident); ok {
					if fn.Name == "runtimeValidationFailureRepairConvergence" {
						if arg, ok := call.Args[0].(*ast.Ident); ok && arg.Name == variable {
							actual["validationFailureRepairConvergence"] = "repair"
						}
					}
					if fn.Name == "runtimeImplementationWorkPolicy" {
						if arg, ok := call.Args[0].(*ast.Ident); ok && arg.Name == variable {
							actual["implementationWorkPolicy"] = "implementationWork"
						}
					}
					owner, key := profileDecoderIndex(call.Args[0])
					if owner == variable && key != "" {
						converter := map[string]string{"runtimeString": "string", "runtimeBool": "boolean", "runtimeStrings": "strings", "runtimeInt": "integer"}[fn.Name]
						actual[key] = converter
					}
				}
			}
			return true
		})
		fields := runtimeProfileFields(kind)
		expected := map[string]string{}
		for valueType, names := range map[string]string{"string": fields.text, "boolean": fields.boolean, "strings": fields.stringList, "integer": fields.integer} {
			for _, name := range strings.Fields(names) {
				expected[name] = valueType
			}
		}
		for name := range fields.arrays {
			expected[name] = "objects"
		}
		if kind == "evidence" {
			expected["validationFailureRepairConvergence"] = "repair"
			expected["implementationWorkPolicy"] = "implementationWork"
		}
		if !reflect.DeepEqual(actual, expected) {
			t.Errorf("%s shape/decoder mismatch: decoded=%v checked=%v", kind, actual, expected)
		}
		seen[kind] = true
		return true
	})
	if len(seen) != 11 {
		t.Fatalf("incomplete decoder coverage: %v", seen)
	}
}

func profileDecoderIndex(node ast.Node) (string, string) {
	index, ok := node.(*ast.IndexExpr)
	if !ok {
		return "", ""
	}
	owner, ok := index.X.(*ast.Ident)
	if !ok {
		return "", ""
	}
	key, ok := index.Index.(*ast.BasicLit)
	if !ok || key.Kind != token.STRING {
		return "", ""
	}
	value, err := strconv.Unquote(key.Value)
	if err != nil {
		return "", ""
	}
	return owner.Name, value
}
