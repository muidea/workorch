package runtime

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strconv"
	"strings"
	"testing"
)

// This inventory comes from production producers, not another manually copied
// list of matrix rows. New evaluator Code constants and literal candidate
// failures in either the Runtime owner or its protocol adapter must have an
// explicit owner disposition. It is verification-only, never a runtime registry.
func TestCandidateRejectionProducersHaveOwnerDisposition(t *testing.T) {
	t.Parallel()
	checked := make(map[string]bool)
	check := func(code, source string) {
		t.Helper()
		disposition, known := CandidateRejectionCodeDisposition(code)
		if !known || disposition == "" {
			t.Errorf("production candidate rejection %q at %s has no SM-3 disposition", code, source)
		}
		checked[code] = true
	}
	for _, directory := range []string{".", "..", "../internal/candidateadmission"} {
		fileSet := token.NewFileSet()
		packages, err := parser.ParseDir(fileSet, directory, func(info os.FileInfo) bool {
			return strings.HasSuffix(info.Name(), ".go") && !strings.HasSuffix(info.Name(), "_test.go")
		}, 0)
		if err != nil || len(packages) != 1 {
			t.Fatalf("read production rejection producers in %s: %v", directory, err)
		}
		for _, pkg := range packages {
			for _, file := range pkg.Files {
				for _, declaration := range file.Decls {
					switch value := declaration.(type) {
					case *ast.GenDecl:
						if value.Tok != token.CONST || pkg.Name != "candidateadmission" {
							continue
						}
						for _, specification := range value.Specs {
							spec := specification.(*ast.ValueSpec)
							for index, name := range spec.Names {
								if strings.HasPrefix(name.Name, "Code") {
									if index >= len(spec.Values) {
										t.Fatalf("candidate rejection vocabulary %s must declare its code explicitly", name.Name)
									}
									code, ok := candidateRejectionLiteralCode(spec.Values[index])
									if !ok {
										t.Fatalf("candidate rejection vocabulary %s is no longer a closed string constant", name.Name)
									}
									check(code, fileSet.Position(name.Pos()).String())
								}
							}
						}
					case *ast.FuncDecl:
						if value.Body == nil {
							continue
						}
						// This is terminal cleanup of an already-sealed SM-2/SM-4
						// fact, not a complete-document correction producer. It
						// cannot reopen an attempt or route through this matrix.
						if value.Name.Name == "closePendingCandidateAttemptsForTerminal" {
							continue
						}
						producesFailure := false
						ast.Inspect(value.Body, func(node ast.Node) bool {
							if call, ok := node.(*ast.CallExpr); ok && len(call.Args) != 0 {
								if name, ok := call.Fun.(*ast.Ident); ok {
									switch name.Name {
									case "contractFailure", "diagnosticFailure", "allowedTargetsFailure", "rejectedCandidateAdmission":
										producesFailure = true
										if code, literalCode := candidateRejectionLiteralCode(call.Args[0]); literalCode {
											check(code, fileSet.Position(call.Pos()).String())
										}
									}
								}
							}
							literal, ok := node.(*ast.CompositeLit)
							if !ok {
								return true
							}
							typeName := ""
							switch typed := literal.Type.(type) {
							case *ast.SelectorExpr:
								typeName = typed.Sel.Name
							case *ast.Ident:
								if pkg.Name == "candidateadmission" && typed.Name == "Finding" {
									typeName = "RuntimeCandidateFailure"
								}
							}
							if typeName != "RuntimeCandidateFailure" {
								return true
							}
							producesFailure = true
							for _, element := range literal.Elts {
								field, ok := element.(*ast.KeyValueExpr)
								if ok && field.Key.(*ast.Ident).Name == "Code" {
									if code, literalCode := candidateRejectionLiteralCode(field.Value); literalCode {
										check(code, fileSet.Position(field.Pos()).String())
									} else if !candidateRejectionAuditedCodeProjection(value.Name.Name, field.Value) {
										t.Errorf("unaudited dynamic candidate code in %s", fileSet.Position(field.Pos()))
									}
								}
							}
							return true
						})
						if !producesFailure {
							continue
						}
						ast.Inspect(value.Body, func(node ast.Node) bool {
							assignment, ok := node.(*ast.AssignStmt)
							if !ok || len(assignment.Lhs) != len(assignment.Rhs) {
								return true
							}
							for index, lhs := range assignment.Lhs {
								if name, ok := lhs.(*ast.Ident); ok && name.Name == "code" {
									if code, literalCode := candidateRejectionLiteralCode(assignment.Rhs[index]); literalCode {
										check(code, fileSet.Position(lhs.Pos()).String())
									}
								}
								field, ok := lhs.(*ast.SelectorExpr)
								if !ok || field.Sel.Name != "Code" {
									continue
								}
								if code, literalCode := candidateRejectionLiteralCode(assignment.Rhs[index]); literalCode {
									check(code, fileSet.Position(lhs.Pos()).String())
								} else if value.Name.Name != "runtimeProviderRequestFailure" {
									t.Errorf("unaudited dynamic candidate code assignment in %s", fileSet.Position(lhs.Pos()))
								}
							}
							return true
						})
					}
				}
			}
		}
	}
	if !checked[candidateAdmissionCodeRepairClosureMismatch] || !checked["candidate_operation_value_rule_nonconformant"] || !checked["operation_feasibility_probe_invalid"] {
		t.Fatal("producer scan lost the evaluator, decoder or owner-probe lane")
	}
}

func candidateRejectionLiteralCode(expression ast.Expr) (string, bool) {
	literal, ok := expression.(*ast.BasicLit)
	if !ok || literal.Kind != token.STRING {
		return "", false
	}
	value, err := strconv.Unquote(literal.Value)
	return value, err == nil
}

func candidateRejectionAuditedCodeProjection(function string, expression ast.Expr) bool {
	// The pure evaluator's constants are exhaustively scanned above. These
	// adapters preserve that vocabulary; they never choose a disposition.
	if selector, ok := expression.(*ast.SelectorExpr); ok {
		if qualifier, ok := selector.X.(*ast.Ident); ok && qualifier.Name == "candidateadmission" && strings.HasPrefix(selector.Sel.Name, "Code") {
			return true
		}
	}
	if name, ok := expression.(*ast.Ident); ok && strings.HasPrefix(name.Name, "Code") {
		return true // the candidateadmission Finding vocabulary above
	}
	if function != "contractFailure" && function != "rejectedCandidateAdmission" {
		return false
	}
	call, ok := expression.(*ast.CallExpr)
	if !ok || len(call.Args) != 1 {
		return false
	}
	selector, ok := call.Fun.(*ast.SelectorExpr)
	argument, argumentOK := call.Args[0].(*ast.Ident)
	if !ok || !argumentOK || argument.Name != "code" || selector.Sel.Name != "TrimSpace" {
		return false
	}
	qualifier, ok := selector.X.(*ast.Ident)
	return ok && qualifier.Name == "strings"
}
