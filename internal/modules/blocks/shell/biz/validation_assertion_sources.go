package shell

import (
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"strconv"
	"strings"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const shellNativeFailureObservationRequirements = " Native case outcomes and raw output bytes remain exact and independent facts. Exclude an ordinary Log/Logf frame only when the sealed source and resolved static argument types prove the direct testing.T call is an observation; unresolved helpers, user formatting and ambiguous call sites remain potential assertions. Never classify by message words or erase raw output. Reproduction compares complete check/case/package/target/body identities (only owner-declared scratch noise may differ); helper decoration location/multiplicity is retained in raw diagnostics rather than an additional assertion identity. Changed values, failed cases or sources, missing/ambiguous frames, per-body/aggregate/count limits and complete serialized feedback capacity remain closed with exact causes."

// Only a direct Log/Logf on a statically identified *testing.T parameter is
// an observation. Helpers, aliases, shadowed receivers and mixed call sites
// remain possible failures. Never classify by diagnostic text. The compiler
// consumes the exact sealed source, not a mutable file or provider assertion.
func shellNativeObservationLines(subject *runtimecontract.RuntimeValidationInputReviewSubject) map[string]map[int]bool {
	result := map[string]map[int]bool{}
	if subject == nil {
		return result
	}
	compilerImports := importer.Default()
	for _, source := range subject.Files {
		if !strings.HasSuffix(source.TargetRef, "_test.go") || source.ContentHash != runtimecontract.RuntimeValidationInputContentHash(source.Content) {
			continue
		}
		positions := token.NewFileSet()
		file, err := parser.ParseFile(positions, source.TargetRef, source.Content, 0)
		if err != nil {
			continue
		}
		// Missing delivery declarations/imports are allowed to leave type holes,
		// never to guess a type. Only fully resolved primitive arguments (and
		// closed standard-library value operations) can exclude a log frame.
		info := &types.Info{Types: map[ast.Expr]types.TypeAndValue{}, Uses: map[*ast.Ident]types.Object{}, Selections: map[*ast.SelectorExpr]*types.Selection{}}
		config := types.Config{Importer: compilerImports, Error: func(error) {}}
		_, _ = config.Check(file.Name.Name, positions, []*ast.File{file}, info)
		alias := ""
		for _, im := range file.Imports {
			value, err := strconv.Unquote(im.Path.Value)
			if err == nil && value == "testing" {
				alias = "testing"
				if im.Name != nil {
					alias = im.Name.Name
				}
			}
		}
		if alias == "" || alias == "." || alias == "_" {
			continue
		}
		receivers := map[*ast.Object]bool{}
		ast.Inspect(file, func(node ast.Node) bool {
			signature, ok := node.(*ast.FuncType)
			if !ok || signature.Params == nil {
				return true
			}
			for _, field := range signature.Params.List {
				pointer, ok := field.Type.(*ast.StarExpr)
				if !ok {
					continue
				}
				selector, ok := pointer.X.(*ast.SelectorExpr)
				if !ok || selector.Sel.Name != "T" {
					continue
				}
				pkg, ok := selector.X.(*ast.Ident)
				if !ok || pkg.Name != alias {
					continue
				}
				for _, name := range field.Names {
					if name.Obj != nil {
						receivers[name.Obj] = true
					}
				}
			}
			return true
		})
		lines, blocked := map[int]bool{}, map[int]bool{}
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			observation := false
			if ok {
				receiver, ok := selector.X.(*ast.Ident)
				observation = ok && receiver.Obj != nil && receivers[receiver.Obj] && (selector.Sel.Name == "Log" || selector.Sel.Name == "Logf")
				for _, arg := range call.Args {
					observation = observation && shellObservationArgument(arg, info)
				}
			}
			start, end := positions.Position(call.Pos()).Line, positions.Position(call.Fun.End()).Line
			for line := start; line <= end; line++ {
				if observation {
					lines[line] = true
				} else {
					blocked[line] = true
				}
			}
			// A verified closed argument tree cannot emit testing failures.
			// Otherwise inspect all nested calls and retain the ambiguous frame.
			return !observation
		})
		for line := range blocked {
			delete(lines, line)
		}
		result[source.TargetRef] = lines
	}
	return result
}

// This deliberately supports a closed subset, rather than guessing purity of
// user helpers or interface formatting. Unsupported expressions remain possible
// assertions. Source type information is local to the sealed input; no code runs.
func shellObservationArgument(expr ast.Expr, info *types.Info) bool {
	typ := info.TypeOf(expr)
	if typ == nil {
		return false
	}
	safeType := func(typ types.Type) bool {
		switch typ := types.Unalias(typ).(type) {
		case *types.Basic:
			return typ.Kind() != types.Invalid && typ.Kind() != types.UnsafePointer && typ.Kind() != types.UntypedNil
		case *types.Slice:
			element, ok := types.Unalias(typ.Elem()).(*types.Basic)
			return ok && (element.Kind() == types.Uint8 || element.Kind() == types.Int32)
		case *types.Named:
			return typ.Obj().Pkg() != nil && typ.Obj().Pkg().Path() == "time" && (typ.Obj().Name() == "Time" || typ.Obj().Name() == "Duration")
		}
		return false
	}
	if !safeType(typ) {
		return false
	}
	switch expr := expr.(type) {
	case *ast.BasicLit, *ast.Ident:
		return true
	case *ast.ParenExpr:
		return shellObservationArgument(expr.X, info)
	case *ast.UnaryExpr:
		return expr.Op != token.ARROW && shellObservationArgument(expr.X, info)
	case *ast.BinaryExpr:
		return shellObservationArgument(expr.X, info) && shellObservationArgument(expr.Y, info)
	case *ast.SelectorExpr:
		// Reading a field/constant of a known primitive type executes no code.
		safe := true
		ast.Inspect(expr.X, func(node ast.Node) bool {
			switch node := node.(type) {
			case *ast.CallExpr, *ast.FuncLit:
				safe = false
			case *ast.UnaryExpr:
				safe = safe && node.Op != token.ARROW
			}
			return safe
		})
		return safe
	case *ast.IndexExpr:
		return shellObservationArgument(expr.X, info) && shellObservationArgument(expr.Index, info)
	case *ast.CallExpr:
		if conversion := info.Types[expr.Fun]; conversion.IsType() {
			// Converting to a builtin primitive cannot invoke user methods.
			if _, ok := types.Unalias(conversion.Type).(*types.Basic); !ok {
				return false
			}
		} else {
			var object types.Object
			switch fun := expr.Fun.(type) {
			case *ast.Ident:
				object = info.Uses[fun]
			case *ast.SelectorExpr:
				object = info.Uses[fun.Sel]
				if selection := info.Selections[fun]; selection != nil && !shellObservationArgument(fun.X, info) {
					return false
				}
			}
			if object == nil {
				return false
			}
			if builtin, ok := object.(*types.Builtin); ok {
				if builtin.Name() != "len" && builtin.Name() != "cap" {
					return false
				}
			} else if object.Pkg() == nil || (object.Pkg().Path() != "time" && object.Pkg().Path() != "runtime" && object.Pkg().Path() != "strings") {
				return false
			}
		}
		for _, arg := range expr.Args {
			if !shellObservationArgument(arg, info) {
				return false
			}
		}
		return true
	}
	return false
}

// Reproduction compares complete source-bound failure identities. Original
// bodies/locations remain unchanged in each receipt. Helper decoration can
// report the same body at multiple call sites; that is not a second assertion.
// ComparisonHash only removes the owner's already declared scratch noise.
func shellSameFailureAssertions(expected, observed []runtimecontract.RuntimeValidationCheckFailure) bool {
	type identity struct {
		Check, Case, Package, Target, Comparison string
		Shape                                    runtimecontract.RuntimeValidationReportedAssertion
	}
	keys := func(values []runtimecontract.RuntimeValidationCheckFailure) map[identity]bool {
		result := map[identity]bool{}
		for _, value := range values {
			digest := value.ComparisonHash
			if digest == "" {
				digest = value.DiagnosticHash
			}
			result[identity{value.CheckRef, value.CaseRef, value.PackageRef, value.TargetRef, digest, value.ReportedAssertion}] = true
		}
		return result
	}
	a, b := keys(expected), keys(observed)
	if len(a) != len(b) {
		return false
	}
	for key := range a {
		if !b[key] {
			return false
		}
	}
	return true
}
