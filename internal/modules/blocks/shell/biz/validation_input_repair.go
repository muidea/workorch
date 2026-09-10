package shell

import (
	"crypto/sha256"
	"encoding/hex"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const shellInputSymbolRepairContract = "shell/validation-input/unambiguous-data-symbol-case/v1"
const shellInputSymbolRepairCode = "validation_input_symbol_case_mismatch"

var shellInputCompilerLocation = regexp.MustCompile(`^((?:\./)?[[:alnum:]_./-]+\.go):([0-9]+):([0-9]+): (.+)$`)
var shellInputUndefinedSymbol = regexp.MustCompile(`^undefined: ([A-Za-z_][A-Za-z_0-9]*)$`)

// Only the exact bound supplemental input is eligible. Compiler locations are
// facts even when no safe delta can be established; they never fall back into
// ordinary implementation targets. The owner derives a byte-exact postimage,
// not an instruction to edit assertions or a File operation.
func shellValidationInputCompileFact(output RuntimeCommandResult, request RuntimeCommandRequest, command string) ([]runtimecontract.RuntimeValidationInputLocation, *runtimecontract.RuntimeValidationInputRepair) {
	if !shellInputHasManagedValidation(request) || filepath.Base(command) != "go" || output.ExitCode == 0 || output.OutputTruncated ||
		len(request.ValidationCheckAttestations) != 1 {
		return nil, nil
	}
	check := request.ValidationCheckAttestations[0]
	if !shellEntrypointObservationProtocol(check.ObservationProtocolRef) || check.SupplementalNativeCheckRef != shellSupplementCheckRef(check.NativeCheckRef) {
		return nil, nil
	}
	target := shellSupplementTargetRef(check.NativeCheckRef)
	scope := map[string]struct{}{}
	for _, ref := range request.ValidationScopeTargetRefs {
		scope[ref] = struct{}{}
	}
	if _, ok := scope[target]; !ok {
		return nil, nil
	}
	var locations []runtimecontract.RuntimeValidationInputLocation
	seen := map[string]bool{}
	repairable := output.validationSourcesUnchanged
	for _, record := range shellValidationDiagnosticRecords(output, command, runtimecontract.ValidationDiagnosticBuildFailed) {
		for _, line := range strings.Split(record.Output, "\n") {
			match := shellInputCompilerLocation.FindStringSubmatch(strings.TrimSpace(line))
			if len(match) != 5 {
				if shellGoCompilerDiagnosticLine(line) {
					repairable = false
				}
				continue
			}
			ref, ok := shellDiagnosticTargetInScope(match[1], record.Package, record.ImportPath, output.WorkingDirectory, scope)
			if !ok || ref != target {
				repairable = false
				continue
			}
			lineNo, errLine := strconv.Atoi(match[2])
			column, errColumn := strconv.Atoi(match[3])
			if errLine != nil || errColumn != nil || lineNo < 1 || column < 1 {
				repairable = false
				continue
			}
			key := match[2] + ":" + match[3]
			if seen[key] {
				continue
			}
			seen[key] = true
			location := runtimecontract.RuntimeValidationInputLocation{TargetRef: ref, Line: lineNo, Column: column, Code: "input_compile_failed"}
			if symbol := shellInputUndefinedSymbol.FindStringSubmatch(match[4]); len(symbol) == 2 && len(symbol[1]) <= 128 {
				location.Code, location.Symbol = "undefined_identifier", symbol[1]
			} else {
				repairable = false
			}
			locations = append(locations, location)
			if len(locations) > runtimecontract.RuntimeValidationInputDetailLimit {
				return nil, nil
			}
		}
	}
	sort.Slice(locations, func(i, j int) bool {
		if locations[i].Line != locations[j].Line {
			return locations[i].Line < locations[j].Line
		}
		return locations[i].Column < locations[j].Column
	})
	if len(locations) == 0 || !repairable {
		return locations, nil
	}
	// Read only the already-executed isolated copy, with an independent size and
	// regular-file bound. File later confirms the durable preimage independently.
	inputPath := filepath.Join(output.WorkingDirectory, filepath.FromSlash(target))
	info, err := os.Lstat(inputPath)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 256*1024 {
		return locations, nil
	}
	before, err := os.ReadFile(inputPath)
	if err != nil || len(before) > 256*1024 {
		return locations, nil
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, target, before, 0)
	if err != nil {
		return locations, nil
	}
	// No language-aware choice crosses the owner boundary. Only unique same-file
	// package data declarations are candidates, never functions, types, selectors,
	// imports, declarations or assertion/expected-value replacement.
	dataNames := map[string][]string{}
	for _, declaration := range file.Decls {
		general, ok := declaration.(*ast.GenDecl)
		if !ok || (general.Tok != token.CONST && general.Tok != token.VAR) {
			continue
		}
		for _, spec := range general.Specs {
			value, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for _, name := range value.Names {
				dataNames[strings.ToLower(name.Name)] = append(dataNames[strings.ToLower(name.Name)], name.Name)
			}
		}
	}
	unresolved := map[string]*ast.Ident{}
	for _, id := range file.Unresolved {
		pos := fset.PositionFor(id.Pos(), false)
		unresolved[strconv.Itoa(pos.Line)+":"+strconv.Itoa(pos.Column)] = id
	}
	after := append([]byte(nil), before...)
	var corrections []runtimecontract.RuntimeValidationInputSymbolCorrection
	for _, location := range locations {
		id := unresolved[strconv.Itoa(location.Line)+":"+strconv.Itoa(location.Column)]
		if id == nil || id.Name != location.Symbol {
			return locations, nil
		}
		names := dataNames[strings.ToLower(id.Name)]
		if len(names) != 1 || names[0] == id.Name || len(names[0]) != len(id.Name) || !strings.EqualFold(names[0], id.Name) {
			return locations, nil
		}
		global := file.Scope.Lookup(names[0])
		shadowed := false
		ast.Inspect(file, func(node ast.Node) bool {
			if other, ok := node.(*ast.Ident); ok && other.Name == names[0] && other.Obj != nil && other.Obj != global {
				shadowed = true
			}
			return !shadowed
		})
		if shadowed {
			return locations, nil
		}
		start := fset.PositionFor(id.Pos(), false).Offset
		if start < 0 || start+len(id.Name) > len(before) || string(before[start:start+len(id.Name)]) != id.Name {
			return locations, nil
		}
		copy(after[start:start+len(id.Name)], names[0])
		corrections = append(corrections, runtimecontract.RuntimeValidationInputSymbolCorrection{Line: location.Line, Column: location.Column, Before: id.Name, After: names[0]})
	}
	beforeHash, afterHash := sha256.Sum256(before), sha256.Sum256(after)
	repair := &runtimecontract.RuntimeValidationInputRepair{ContractRef: shellInputSymbolRepairContract, RequiredCheckRef: check.RequiredCheckRef,
		ObservationProtocolRef: check.ObservationProtocolRef,
		OriginalNativeCheckRef: check.NativeCheckRef, SupplementalNativeCheckRef: check.SupplementalNativeCheckRef, TargetRef: target,
		BeforeHash: hex.EncodeToString(beforeHash[:]), AfterHash: hex.EncodeToString(afterHash[:]), Corrections: corrections}
	if repair.Validate() != nil {
		return locations, nil
	}
	return locations, repair
}
