package activationports

import (
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"testing"

	"github.com/muidea/workorch/internal/pkg/runtimeprofiledecode"
)

// 保留调用端接线检查；完整字段/转换类型合同在 shared decoder 包逐对象验证。
func TestRuntimeProfileShapeMatchesTypedDecoderFields(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "../../../../../../pkg/runtimeprojection/projection.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var decoder *ast.FuncDecl
	for _, declaration := range file.Decls {
		if function, ok := declaration.(*ast.FuncDecl); ok && function.Name.Name == "RuntimeExecutionContract" {
			decoder = function
		}
	}
	if decoder == nil {
		t.Fatal("activation execution contract decoder 不存在")
	}
	wired := false
	ast.Inspect(decoder.Body, func(node ast.Node) bool {
		assignment, ok := node.(*ast.AssignStmt)
		if !ok || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 {
			return true
		}
		field, ok := assignment.Lhs[0].(*ast.SelectorExpr)
		if !ok || field.Sel.Name != "ProfileContracts" {
			return true
		}
		call, ok := assignment.Rhs[0].(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		owner, ok := selector.X.(*ast.Ident)
		wired = ok && owner.Name == "runtimeprofiledecode" && selector.Sel.Name == "Decode"
		return true
	})
	if !wired {
		t.Fatal("activation 未使用共享 Profile decoder")
	}
	input := map[string]any{"profileContracts": map[string]any{"p": map[string]any{
		"acceptanceDimensions": []any{map[string]any{"id": "d", "required": true}},
		"evidenceRequirements": []any{map[string]any{"id": "e", "scope": "dodItem", "requiredEvidenceKinds": []any{"validation"}}},
	}}}
	if err := validateRuntimeProfileInputShape(input); err != nil {
		t.Fatal(err)
	}
	if got, want := runtimeExecutionContract(input).ProfileContracts, runtimeprofiledecode.Decode(input["profileContracts"]); !reflect.DeepEqual(got, want) {
		t.Fatalf("activation/shared decoder 产物不一致: got=%+v want=%+v", got, want)
	}
}
