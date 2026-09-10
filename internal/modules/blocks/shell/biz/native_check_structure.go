package shell

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const (
	maxNativeCheckSourceFiles = 256
	maxNativeCheckSourceBytes = 8 << 20
	maxNativeCheckWalkNodes   = 32768
	maxNativeCheckCallDepth   = 64
)

// This inspection establishes only an executable check's structural readiness.
// It is not a semantic grader and cannot establish that a Goal/DoD is satisfied.
// The same bounded inspection feeds catalog readiness and its factual diagnostic.
type shellNativeCheckInspection struct {
	InputTargetRefs []string
	Code            string
	CheckRef        string
	TargetRefs      []string
}

type shellNativeCheckFunction struct {
	decl         *ast.FuncDecl
	targetRef    string
	packageKey   string
	testingAlias string
}

func inspectShellNativeValidationChecks(directory string, targetRefs []string, attestations []runtimecontract.RuntimeValidationCheckAttestationBinding) (inspection shellNativeCheckInspection) {
	root, err := filepath.Abs(strings.TrimSpace(directory))
	if err != nil || strings.TrimSpace(directory) == "" || len(attestations) == 0 {
		return shellNativeCheckInspection{Code: "native_check_input_unavailable"}
	}
	sourceRoot, err := os.OpenRoot(root)
	if err != nil {
		return shellNativeCheckInspection{Code: "native_check_input_unavailable"}
	}
	defer sourceRoot.Close()
	functions := make(map[string][]shellNativeCheckFunction)
	entries := make(map[string][]shellNativeCheckFunction)
	// Preserve every observed native input, including ambiguous/rejected
	// entries. Readiness failure never turns an input into an ordinary writable
	// implementation. These identities do not grant repair authority.
	defer func() {
		for _, check := range shellFlattenNativeCheckAttestations(attestations) {
			for _, entry := range entries[check.NativeCheckRef] {
				inspection.InputTargetRefs = append(inspection.InputTargetRefs, entry.targetRef)
			}
		}
		inspection.InputTargetRefs = normalizeShellStrings(inspection.InputTargetRefs)
	}()
	var invalidSources []string
	fileCount, sourceBytes := 0, int64(0)
	for _, ref := range normalizeShellStrings(targetRefs) {
		if !strings.HasSuffix(ref, ".go") || managedValidationSandboxPathExcluded(ref) {
			continue
		}
		path := filepath.Join(root, filepath.FromSlash(ref))
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil || filepath.IsAbs(ref) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return shellNativeCheckInspection{Code: "native_check_source_out_of_scope"}
		}
		info, statErr := os.Lstat(path)
		if statErr != nil || !info.Mode().IsRegular() {
			return shellNativeCheckInspection{Code: "native_check_source_unavailable", TargetRefs: []string{ref}}
		}
		fileCount++
		if fileCount > maxNativeCheckSourceFiles || info.Size() > maxNativeCheckSourceBytes-sourceBytes {
			return shellNativeCheckInspection{Code: "native_check_analysis_limit"}
		}
		source, openErr := sourceRoot.Open(filepath.FromSlash(ref))
		if openErr != nil {
			return shellNativeCheckInspection{Code: "native_check_source_unavailable", TargetRefs: []string{ref}}
		}
		content, readErr := io.ReadAll(io.LimitReader(source, maxNativeCheckSourceBytes-sourceBytes+1))
		source.Close()
		sourceBytes += int64(len(content))
		if sourceBytes > maxNativeCheckSourceBytes {
			return shellNativeCheckInspection{Code: "native_check_analysis_limit"}
		}
		if readErr != nil {
			return shellNativeCheckInspection{Code: "native_check_source_unavailable", TargetRefs: []string{ref}}
		}
		file, parseErr := parser.ParseFile(token.NewFileSet(), path, content, 0)
		if parseErr != nil {
			// Compilation has its own owner diagnostic. Do not infer a native
			// declaration from a partial AST or export raw parser text.
			invalidSources = append(invalidSources, ref)
			continue
		}
		for _, check := range attestations {
			if shellVersionedNativeCheck(check) && ref == shellVersionedNativeCheckTarget(check.NativeCheckRef) && !shellEntrypointSupplementDeclarations(file, check.NativeCheckRef) {
				return shellNativeCheckInspection{Code: "native_check_versioned_contract_invalid", CheckRef: check.NativeCheckRef, TargetRefs: []string{ref}}
			}
			if check.ObservationProtocolRef == shellGoEntrypointObservationProtocolRef && strings.HasSuffix(ref, "_test.go") && shellNativeRunnerOverride(file) {
				return shellNativeCheckInspection{Code: "native_check_runner_override", CheckRef: check.NativeCheckRef, TargetRefs: []string{ref}}
			}
			if check.SupplementalNativeCheckRef == "" || ref != shellSupplementTargetRef(check.NativeCheckRef) {
				continue
			}
			if check.ObservationProtocolRef == shellGoEntrypointObservationProtocolRef {
				if !shellEntrypointSupplementDeclarations(file, check.SupplementalNativeCheckRef) {
					return shellNativeCheckInspection{Code: "native_check_supplement_contract_invalid", CheckRef: check.SupplementalNativeCheckRef, TargetRefs: []string{ref}}
				}
				continue
			}
			for _, decl := range file.Decls {
				switch item := decl.(type) {
				case *ast.GenDecl:
					if item.Tok == token.IMPORT {
						continue
					}
				case *ast.FuncDecl:
					if item.Recv == nil && item.Name.Name == check.SupplementalNativeCheckRef {
						continue
					}
				}
				return shellNativeCheckInspection{Code: "native_check_supplement_contract_invalid", CheckRef: check.SupplementalNativeCheckRef, TargetRefs: []string{ref}}
			}
		}
		alias := ""
		for _, item := range file.Imports {
			importPath, _ := strconv.Unquote(item.Path.Value)
			if importPath == "testing" {
				alias = "testing"
				if item.Name != nil {
					alias = item.Name.Name
				}
			}
		}
		packageKey := filepath.ToSlash(filepath.Dir(ref)) + "\x00" + file.Name.Name
		for _, declaration := range file.Decls {
			fn, ok := declaration.(*ast.FuncDecl)
			if !ok || fn.Recv != nil || fn.Body == nil {
				continue
			}
			value := shellNativeCheckFunction{fn, ref, packageKey, alias}
			key := packageKey + "\x00" + fn.Name.Name
			functions[key] = append(functions[key], value)
			if strings.HasSuffix(ref, "_test.go") {
				entries[fn.Name.Name] = append(entries[fn.Name.Name], value)
			}
		}
	}
	for _, raw := range shellFlattenNativeCheckAttestations(attestations) {
		if !shellNativeObservationProtocolSupported(raw.ObservationProtocolRef) || raw.NativeCheckRef == "" {
			return shellNativeCheckInspection{Code: "native_check_protocol_unsupported", CheckRef: raw.NativeCheckRef}
		}
		matches := entries[raw.NativeCheckRef]
		if len(matches) == 0 {
			if len(invalidSources) != 0 {
				return shellNativeCheckInspection{Code: "native_check_source_invalid", CheckRef: raw.NativeCheckRef, TargetRefs: invalidSources}
			}
			return shellNativeCheckInspection{Code: "native_check_missing", CheckRef: raw.NativeCheckRef}
		}
		if len(matches) != 1 {
			refs := make([]string, 0, len(matches))
			for _, match := range matches {
				refs = append(refs, match.targetRef)
			}
			return shellNativeCheckInspection{Code: "native_check_ambiguous", CheckRef: raw.NativeCheckRef, TargetRefs: normalizeShellStrings(refs)}
		}
		entry := matches[0]
		if shellVersionedNativeCheck(raw) && entry.targetRef != shellVersionedNativeCheckTarget(raw.NativeCheckRef) {
			return shellNativeCheckInspection{Code: "native_check_versioned_out_of_scope", CheckRef: raw.NativeCheckRef, TargetRefs: []string{entry.targetRef}}
		}
		if (raw.ObservationProtocolRef == shellGoSupplementObservationProtocolRef || raw.ObservationProtocolRef == shellGoEntrypointObservationProtocolRef) && strings.HasSuffix(raw.NativeCheckRef, "_Behavior") && entry.targetRef != shellSupplementTargetRef(strings.TrimSuffix(raw.NativeCheckRef, "_Behavior")) {
			return shellNativeCheckInspection{Code: "native_check_supplement_out_of_scope", CheckRef: raw.NativeCheckRef, TargetRefs: []string{entry.targetRef}}
		}
		parameters := nativeCheckTestingParameters(entry.decl.Type, entry.testingAlias)
		if entry.decl.Type.Results != nil || len(parameters) != 1 || parameters[0] == nil || entry.decl.Type.Params.NumFields() != 1 {
			return shellNativeCheckInspection{Code: "native_check_signature_invalid", CheckRef: raw.NativeCheckRef, TargetRefs: []string{entry.targetRef}}
		}
		walk := shellNativeCheckWalk{functions: functions, visiting: make(map[*ast.FuncDecl]bool)}
		if !walk.function(entry, map[*ast.Object]bool{parameters[0].Obj: true}) || walk.limitExceeded {
			code := "native_check_assertion_unresolved"
			if walk.limitExceeded {
				code = "native_check_analysis_limit"
			}
			return shellNativeCheckInspection{Code: code, CheckRef: raw.NativeCheckRef, TargetRefs: []string{entry.targetRef}}
		}
	}
	return shellNativeCheckInspection{}
}

// Keep positional nil entries: only a real *testing.T passed from a known
// testing object may reach a helper. An ident named "t" is not enough.
func nativeCheckTestingParameters(kind *ast.FuncType, alias string) []*ast.Ident {
	var parameters []*ast.Ident
	if kind == nil || kind.Params == nil {
		return parameters
	}
	for _, field := range kind.Params.List {
		pointer, pointerOK := field.Type.(*ast.StarExpr)
		valid := false
		if pointerOK {
			selector, ok := pointer.X.(*ast.SelectorExpr)
			if ok {
				base, ok := selector.X.(*ast.Ident)
				valid = ok && alias != "" && alias != "." && alias != "_" && base.Name == alias && selector.Sel.Name == "T"
			}
		}
		if len(field.Names) == 0 {
			parameters = append(parameters, nil)
		}
		for _, name := range field.Names {
			if valid {
				parameters = append(parameters, name)
			} else {
				parameters = append(parameters, nil)
			}
		}
	}
	return parameters
}

type shellNativeCheckWalk struct {
	functions     map[string][]shellNativeCheckFunction
	visiting      map[*ast.FuncDecl]bool
	nodes         int
	depth         int
	limitExceeded bool
}

func (w *shellNativeCheckWalk) function(fn shellNativeCheckFunction, objects map[*ast.Object]bool) bool {
	if w.visiting[fn.decl] || len(objects) == 0 {
		return false
	}
	if w.depth == maxNativeCheckCallDepth {
		w.limitExceeded = true
		return false
	}
	w.depth++
	defer func() { w.depth-- }()
	w.visiting[fn.decl] = true
	defer delete(w.visiting, fn.decl)
	return w.body(fn, fn.decl.Body, objects)
}

func (w *shellNativeCheckWalk) body(fn shellNativeCheckFunction, body *ast.BlockStmt, objects map[*ast.Object]bool) bool {
	found := false
	ast.Inspect(body, func(node ast.Node) bool {
		w.nodes++
		if w.nodes > maxNativeCheckWalkNodes {
			w.limitExceeded = true
		}
		if w.limitExceeded || found {
			return false
		}
		if _, literal := node.(*ast.FuncLit); literal {
			// An uninvoked closure is not a reachable assertion. t.Run is
			// handled explicitly below with its own testing parameter.
			return false
		}
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		if selector, ok := call.Fun.(*ast.SelectorExpr); ok {
			if receiver, ok := selector.X.(*ast.Ident); ok && receiver.Obj != nil && objects[receiver.Obj] {
				switch selector.Sel.Name {
				case "Error", "Errorf", "Fatal", "Fatalf", "Fail", "FailNow":
					found = true
				case "Run":
					if len(call.Args) == 2 {
						if child, ok := call.Args[1].(*ast.FuncLit); ok {
							params := nativeCheckTestingParameters(child.Type, fn.testingAlias)
							if len(params) == 1 && params[0] != nil {
								found = w.body(fn, child.Body, map[*ast.Object]bool{params[0].Obj: true})
							}
						}
					}
				}
			}
		}
		if name, ok := call.Fun.(*ast.Ident); ok {
			helpers := w.functions[fn.packageKey+"\x00"+name.Name]
			if len(helpers) == 1 && (name.Obj == nil || name.Obj.Kind == ast.Fun) {
				helper := helpers[0]
				bound := make(map[*ast.Object]bool)
				for index, parameter := range nativeCheckTestingParameters(helper.decl.Type, helper.testingAlias) {
					if parameter == nil || index >= len(call.Args) {
						continue
					}
					if argument, ok := call.Args[index].(*ast.Ident); ok && argument.Obj != nil && objects[argument.Obj] {
						bound[parameter.Obj] = true
					}
				}
				found = found || w.function(helper, bound)
			}
		}
		return !found
	})
	return found
}
