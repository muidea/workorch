package workorchruntime

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
)

const (
	repositoryDepsImport           = "github.com/muidea/workorch/internal/pkg/workorchdeps"
	repositoryImplementationImport = "github.com/muidea/workorch/internal/initiators/workorchruntime"
	repositoryCommonImport         = "github.com/muidea/workorch/internal/initiators/workorchruntime/pkg/common"
	repositoryInterfacesImport     = "github.com/muidea/workorch/internal/pkg/repository"
	frameworkInitiatorImport       = "github.com/muidea/magicCommon/framework/plugin/initiator"
)

// The workorchdeps consumer argument is a runtime authorization fence. Keep
// the static callsite scan as a second, compile-time-visible owner boundary so
// production code cannot accidentally request a sibling repository family.
func TestRepositoryConsumersUseTheirOwnerRepositoryGroups(t *testing.T) {
	repositoryRoot := repositoryBoundaryRoot(t)
	wantResolvers := map[string]string{
		"internal/modules/blocks/approval":     "ResolveApprovalRepositories",
		"internal/modules/blocks/decision":     "ResolveDecisionRepositories",
		"internal/modules/blocks/eventstream":  "ResolveEventStreamRepositories",
		"internal/modules/blocks/mcpregistry":  "ResolveMCPRepositories",
		"internal/modules/blocks/memory":       "ResolveMemoryRepositories",
		"internal/modules/blocks/result":       "ResolveResultRepositories",
		"internal/modules/blocks/run":          "ResolveRunRepositories",
		"internal/modules/blocks/skill":        "ResolveSkillRepositories",
		"internal/modules/blocks/soul":         "ResolveSoulRepositories",
		"internal/modules/kernel/workspace":    "ResolveWorkspaceRepositories",
		"internal/modules/kernel/governance":   "ResolveGovernanceRepositories",
		"internal/modules/kernel/orchestrator": "ResolveOrchestratorRepositories",
	}
	seen := make(map[string]int, len(wantResolvers))

	for _, sourceRoot := range []string{"application", "internal"} {
		root := filepath.Join(repositoryRoot, sourceRoot)
		err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			file, aliases, err := parseRepositoryBoundaryFile(path)
			if err != nil {
				return err
			}
			if len(aliases) == 0 {
				return nil
			}
			relativePath, err := filepath.Rel(repositoryRoot, path)
			if err != nil {
				return err
			}
			relativePath = filepath.ToSlash(relativePath)
			ast.Inspect(file, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				selector, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				qualifier, ok := selector.X.(*ast.Ident)
				if !ok || !aliases[qualifier.Name] {
					return true
				}
				ownerRoot, expectedResolver := repositoryOwnerResolver(relativePath, wantResolvers)
				if ownerRoot == "" {
					t.Errorf("production caller %s uses workorchdeps.%s outside a repository owner", relativePath, selector.Sel.Name)
					return true
				}
				if selector.Sel.Name != expectedResolver {
					t.Errorf("repository owner %s uses workorchdeps.%s, want %s", ownerRoot, selector.Sel.Name, expectedResolver)
				}
				if len(call.Args) != 1 || !isIdentifier(call.Args[0], "ModuleID") {
					t.Errorf("repository owner %s must call %s with ModuleID", ownerRoot, expectedResolver)
				}
				seen[ownerRoot]++
				return true
			})
			return nil
		})
		if err != nil {
			t.Fatalf("scan production repository consumers under %s: %v", root, err)
		}
	}

	owners := make([]string, 0, len(wantResolvers))
	for ownerRoot := range wantResolvers {
		owners = append(owners, ownerRoot)
	}
	sort.Strings(owners)
	for _, ownerRoot := range owners {
		if seen[ownerRoot] != 1 {
			t.Errorf("repository owner %s resolver call count = %d, want 1", ownerRoot, seen[ownerRoot])
		}
	}
}

// Keep provider discovery centralized in workorchdeps. The implementation
// package is enabled only by the application entry, while production owners
// consume typed groups through the facade rather than retrieving the
// Initiator or importing its common contract directly.
func TestRepositoryProviderAccessIsCentralizedInWorkorchdeps(t *testing.T) {
	repositoryRoot := repositoryBoundaryRoot(t)
	const (
		applicationEntry = "application/workorch/main.go"
		providerFacade   = "internal/pkg/workorchdeps/repositories.go"
		initiatorRoot    = "internal/initiators/workorchruntime/"
	)

	for _, sourceRoot := range []string{"application", "internal"} {
		root := filepath.Join(repositoryRoot, sourceRoot)
		err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			file, _, err := parseRepositoryBoundaryFile(path)
			if err != nil {
				return err
			}
			imports, err := repositoryBoundaryImports(file)
			if err != nil {
				return err
			}
			relativePath, err := filepath.Rel(repositoryRoot, path)
			if err != nil {
				return err
			}
			relativePath = filepath.ToSlash(relativePath)

			for _, imported := range imports {
				switch imported.path {
				case repositoryImplementationImport:
					if relativePath != applicationEntry || imported.alias != "_" {
						t.Errorf("production file %s imports repository implementation directly; only %s may side-effect import it", relativePath, applicationEntry)
					}
				case repositoryCommonImport:
					if relativePath != providerFacade && !strings.HasPrefix(relativePath, initiatorRoot) {
						t.Errorf("production file %s imports repository provider contract directly; use workorchdeps", relativePath)
					}
				}
			}
			assertNoDirectInitiatorLookup(t, relativePath, providerFacade, file, imports)
			return nil
		})
		if err != nil {
			t.Fatalf("scan production repository provider access under %s: %v", root, err)
		}
	}
}

// Keep the Initiator's public surface limited to typed repository groups. The
// framework-provided EventHub and BackgroundRoutine remain Setup parameters;
// they are not exported capabilities or active interaction channels here.
func TestRepositoryInitiatorExportSurfaceStaysRepositoryOnly(t *testing.T) {
	repositoryRoot := repositoryBoundaryRoot(t)
	initiatorRoot := filepath.Join(repositoryRoot, "internal", "initiators", "workorchruntime")
	allowedExports := map[string]map[string]bool{
		"internal/initiators/workorchruntime": {
			"RepositoryInitiator": true,
			"New":                 true,
			"ID":                  true,
			"Weight":              true,
			"Setup":               true,
			"Run":                 true,
			"Teardown":            true,
			"RepositoryProvider":  true,
		},
		"internal/initiators/workorchruntime/pkg/common": {
			"InitiatorID":             true,
			"WorkspaceRepositories":   true,
			"DecisionRepositories":    true,
			"ApprovalRepositories":    true,
			"EventStreamRepositories": true,
			"GovernanceRepositories":  true,
			"MCPRepositories":         true,
			"MemoryRepositories":      true,
			"ResultRepositories":      true,
			"RunRepositories":         true,
			"SkillRepositories":       true,
			"SoulRepositories":        true,
			"RepositoryProvider":      true,
			"Provider":                true,
		},
	}

	err := filepath.WalkDir(initiatorRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, _, err := parseRepositoryBoundaryFile(path)
		if err != nil {
			return err
		}
		imports, err := repositoryBoundaryImports(file)
		if err != nil {
			return err
		}
		relativeDir, err := filepath.Rel(repositoryRoot, filepath.Dir(path))
		if err != nil {
			return err
		}
		packagePath := filepath.ToSlash(relativeDir)
		allowed, knownPackage := allowedExports[packagePath]
		if !knownPackage {
			t.Errorf("repository Initiator gained unreviewed production package %s", packagePath)
			return nil
		}
		for _, imported := range file.Imports {
			importPath, err := strconv.Unquote(imported.Path.Value)
			if err != nil {
				return err
			}
			if forbiddenRepositoryInitiatorImport(importPath) {
				t.Errorf("repository Initiator package %s imports forbidden runtime or owner dependency %q", packagePath, importPath)
			}
		}
		for _, declaration := range file.Decls {
			for _, name := range exportedDeclarationNames(declaration) {
				if !allowed[name] {
					t.Errorf("repository Initiator package %s exports unreviewed symbol %s", packagePath, name)
				}
			}
		}
		assertRepositoryContractShape(t, packagePath, path, file, imports)
		if packagePath == "internal/initiators/workorchruntime" {
			assertNoInitiatorEventOperations(t, path, file)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan repository Initiator production surface: %v", err)
	}
}

type repositoryBoundaryImport struct {
	alias string
	path  string
}

func repositoryBoundaryImports(file *ast.File) ([]repositoryBoundaryImport, error) {
	imports := make([]repositoryBoundaryImport, 0, len(file.Imports))
	for _, imported := range file.Imports {
		importPath, err := strconv.Unquote(imported.Path.Value)
		if err != nil {
			return nil, err
		}
		alias := filepath.Base(importPath)
		if imported.Name != nil {
			alias = imported.Name.Name
		}
		imports = append(imports, repositoryBoundaryImport{alias: alias, path: importPath})
	}
	return imports, nil
}

func assertNoDirectInitiatorLookup(t *testing.T, relativePath string, providerFacade string, file *ast.File, imports []repositoryBoundaryImport) {
	t.Helper()
	namedAliases := map[string]bool{}
	dotImported := false
	for _, imported := range imports {
		if imported.path != frameworkInitiatorImport {
			continue
		}
		if imported.alias == "." {
			dotImported = true
			continue
		}
		namedAliases[imported.alias] = true
	}
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		directLookup := false
		switch function := call.Fun.(type) {
		case *ast.SelectorExpr:
			qualifier, ok := function.X.(*ast.Ident)
			directLookup = ok && namedAliases[qualifier.Name] && function.Sel.Name == "GetEntity"
		case *ast.Ident:
			directLookup = dotImported && function.Name == "GetEntity"
		}
		if directLookup && relativePath != providerFacade {
			t.Errorf("production file %s retrieves an Initiator directly; repository discovery belongs in %s", relativePath, providerFacade)
		}
		return true
	})
}

func assertRepositoryContractShape(t *testing.T, packagePath string, path string, file *ast.File, imports []repositoryBoundaryImport) {
	t.Helper()
	for _, declaration := range file.Decls {
		general, ok := declaration.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, specification := range general.Specs {
			typeSpec, ok := specification.(*ast.TypeSpec)
			if !ok {
				continue
			}
			switch packagePath {
			case "internal/initiators/workorchruntime":
				switch typeSpec.Name.Name {
				case "RepositoryInitiator":
					assertRepositoryInitiatorStorage(t, path, typeSpec)
				case "repositorySet":
					assertRepositoryStructFields(t, path, typeSpec, imports, nil)
				}
			case "internal/initiators/workorchruntime/pkg/common":
				switch typeSpec.Name.Name {
				case "RepositoryProvider":
					assertRepositoryInterface(t, path, typeSpec, map[string]string{
						"Workspace":    "WorkspaceRepositories",
						"Decision":     "DecisionRepositories",
						"Approval":     "ApprovalRepositories",
						"EventStream":  "EventStreamRepositories",
						"Governance":   "GovernanceRepositories",
						"MCP":          "MCPRepositories",
						"Memory":       "MemoryRepositories",
						"Orchestrator": "OrchestratorEvidenceOwnerRepositories",
						"Result":       "ResultRepositories",
						"Run":          "RunRepositories",
						"Skill":        "SkillRepositories",
						"Soul":         "SoulRepositories",
					})
				case "Provider":
					assertRepositoryInterface(t, path, typeSpec, map[string]string{
						"RepositoryProvider": "RepositoryProvider",
					})
				default:
					if ast.IsExported(typeSpec.Name.Name) {
						expectedFields, known := repositoryGroupFieldContracts()[typeSpec.Name.Name]
						if !known {
							t.Errorf("repository contract %s in %s has no reviewed field contract", typeSpec.Name.Name, filepath.Base(path))
							continue
						}
						assertRepositoryStructFields(t, path, typeSpec, imports, expectedFields)
					}
				}
			}
		}
	}
}

func assertRepositoryInitiatorStorage(t *testing.T, path string, typeSpec *ast.TypeSpec) {
	t.Helper()
	structure, ok := typeSpec.Type.(*ast.StructType)
	if !ok {
		t.Errorf("repository Initiator %s must remain a struct", filepath.Base(path))
		return
	}
	if len(structure.Fields.List) != 1 {
		t.Errorf("repository Initiator %s field count = %d, want one private repository reference", filepath.Base(path), len(structure.Fields.List))
		return
	}
	field := structure.Fields.List[0]
	if len(field.Names) != 1 || field.Names[0].Name != "repositories" || !isPointerToIdentifier(field.Type, "repositorySet") {
		t.Errorf("repository Initiator %s may retain only private *repositorySet storage", filepath.Base(path))
	}
}

func assertRepositoryStructFields(t *testing.T, path string, typeSpec *ast.TypeSpec, imports []repositoryBoundaryImport, expected map[string]string) {
	t.Helper()
	structure, ok := typeSpec.Type.(*ast.StructType)
	if !ok {
		t.Errorf("repository contract %s in %s must remain a struct", typeSpec.Name.Name, filepath.Base(path))
		return
	}
	seen := make(map[string]int, len(expected))
	for _, field := range structure.Fields.List {
		if len(field.Names) == 0 {
			t.Errorf("repository contract %s in %s must not embed fields", typeSpec.Name.Name, filepath.Base(path))
			continue
		}
		repositoryType := repositoryInterfaceTypeName(field.Type, imports)
		if repositoryType == "" {
			t.Errorf("repository contract %s field in %s must use an internal/pkg/repository interface", typeSpec.Name.Name, filepath.Base(path))
		}
		for _, name := range field.Names {
			if !ast.IsExported(name.Name) {
				t.Errorf("repository contract %s field %s in %s must remain an explicit exported repository", typeSpec.Name.Name, name.Name, filepath.Base(path))
			}
			if expected == nil {
				continue
			}
			wantType, known := expected[name.Name]
			if !known {
				t.Errorf("repository contract %s in %s exposes unreviewed field %s", typeSpec.Name.Name, filepath.Base(path), name.Name)
				continue
			}
			if repositoryType != wantType {
				t.Errorf("repository contract %s.%s in %s uses %s, want %s", typeSpec.Name.Name, name.Name, filepath.Base(path), repositoryType, wantType)
			}
			seen[name.Name]++
		}
	}
	for fieldName := range expected {
		if seen[fieldName] != 1 {
			t.Errorf("repository contract %s.%s declaration count = %d, want 1", typeSpec.Name.Name, fieldName, seen[fieldName])
		}
	}
}

func repositoryGroupFieldContracts() map[string]map[string]string {
	return map[string]map[string]string{
		"WorkspaceRepositories": {
			"Owner": "WorkspaceOwnerRepositories",
		},
		"DecisionRepositories": {
			"Owner": "DecisionOwnerRepositories",
		},
		"ApprovalRepositories": {
			"Owner": "ApprovalOwnerRepositories",
		},
		"EventStreamRepositories": {
			"Owner": "EventStreamOwnerRepositories",
		},
		"GovernanceRepositories": {
			"AutoGovernanceTraces": "AutoGovernanceTraceRepository",
		},
		"MCPRepositories": {
			"Registry": "MCPRegistryRepository",
		},
		"MemoryRepositories": {
			"Owner": "MemoryOwnerRepositories",
		},
		"ResultRepositories": {
			"Owner": "ResultOwnerRepositories",
		},
		"RunRepositories": {
			"Runs":        "RunRepository",
			"Steps":       "StepRepository",
			"Checkpoints": "RuntimeCheckpointRepository",
			"Owner":       "RunOwnerRepositories",
		},
		"SkillRepositories": {
			"Skills": "SkillRepository",
		},
		"SoulRepositories": {
			"Souls": "SoulRepository",
		},
	}
}

func assertRepositoryInterface(t *testing.T, path string, typeSpec *ast.TypeSpec, expected map[string]string) {
	t.Helper()
	contract, ok := typeSpec.Type.(*ast.InterfaceType)
	if !ok {
		t.Errorf("repository contract %s in %s must remain an interface", typeSpec.Name.Name, filepath.Base(path))
		return
	}
	seen := make(map[string]int, len(expected))
	for _, field := range contract.Methods.List {
		if len(field.Names) != 1 {
			t.Errorf("repository contract %s in %s must not embed interfaces", typeSpec.Name.Name, filepath.Base(path))
			continue
		}
		methodName := field.Names[0].Name
		wantResult, known := expected[methodName]
		if !known {
			t.Errorf("repository contract %s in %s exposes unreviewed method %s", typeSpec.Name.Name, filepath.Base(path), methodName)
			continue
		}
		method, ok := field.Type.(*ast.FuncType)
		if !ok || fieldListLength(method.Params) != 0 || fieldListLength(method.Results) != 1 || !fieldListReturnsIdentifier(method.Results, wantResult) {
			t.Errorf("repository contract %s.%s in %s must take no arguments and return %s", typeSpec.Name.Name, methodName, filepath.Base(path), wantResult)
		}
		seen[methodName]++
	}
	for methodName := range expected {
		if seen[methodName] != 1 {
			t.Errorf("repository contract %s.%s declaration count = %d, want 1", typeSpec.Name.Name, methodName, seen[methodName])
		}
	}
}

func isPointerToIdentifier(expression ast.Expr, name string) bool {
	pointer, ok := expression.(*ast.StarExpr)
	if !ok {
		return false
	}
	return isIdentifier(pointer.X, name)
}

func repositoryInterfaceTypeName(expression ast.Expr, imports []repositoryBoundaryImport) string {
	selector, ok := expression.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	qualifier, ok := selector.X.(*ast.Ident)
	if !ok {
		return ""
	}
	for _, imported := range imports {
		if imported.alias == qualifier.Name && imported.path == repositoryInterfacesImport {
			return selector.Sel.Name
		}
	}
	return ""
}

func fieldListLength(fields *ast.FieldList) int {
	if fields == nil {
		return 0
	}
	count := 0
	for _, field := range fields.List {
		if len(field.Names) == 0 {
			count++
			continue
		}
		count += len(field.Names)
	}
	return count
}

func fieldListReturnsIdentifier(fields *ast.FieldList, name string) bool {
	if fields == nil || len(fields.List) != 1 || len(fields.List[0].Names) > 1 {
		return false
	}
	if isIdentifier(fields.List[0].Type, name) {
		return true
	}
	selector, ok := fields.List[0].Type.(*ast.SelectorExpr)
	return ok && selector.Sel.Name == name
}

func parseRepositoryBoundaryFile(path string) (*ast.File, map[string]bool, error) {
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		return nil, nil, err
	}
	aliases := map[string]bool{}
	for _, imported := range file.Imports {
		importPath, err := strconv.Unquote(imported.Path.Value)
		if err != nil {
			return nil, nil, err
		}
		if importPath != repositoryDepsImport {
			continue
		}
		alias := filepath.Base(importPath)
		if imported.Name != nil {
			alias = imported.Name.Name
		}
		if alias == "." || alias == "_" {
			return nil, nil, &repositoryBoundaryError{path: path, message: "workorchdeps must use a named import"}
		}
		aliases[alias] = true
	}
	return file, aliases, nil
}

func repositoryOwnerResolver(path string, resolvers map[string]string) (string, string) {
	for ownerRoot, resolver := range resolvers {
		if path == ownerRoot || strings.HasPrefix(path, ownerRoot+"/") {
			return ownerRoot, resolver
		}
	}
	return "", ""
}

func isIdentifier(expression ast.Expr, name string) bool {
	identifier, ok := expression.(*ast.Ident)
	return ok && identifier.Name == name
}

func exportedDeclarationNames(declaration ast.Decl) []string {
	switch item := declaration.(type) {
	case *ast.FuncDecl:
		if item.Recv != nil && !exportedReceiver(item.Recv) {
			return nil
		}
		if ast.IsExported(item.Name.Name) {
			return []string{item.Name.Name}
		}
	case *ast.GenDecl:
		var names []string
		for _, specification := range item.Specs {
			switch spec := specification.(type) {
			case *ast.TypeSpec:
				if ast.IsExported(spec.Name.Name) {
					names = append(names, spec.Name.Name)
				}
			case *ast.ValueSpec:
				for _, name := range spec.Names {
					if ast.IsExported(name.Name) {
						names = append(names, name.Name)
					}
				}
			}
		}
		return names
	}
	return nil
}

func exportedReceiver(receiver *ast.FieldList) bool {
	if receiver == nil || len(receiver.List) == 0 {
		return true
	}
	receiverType := receiver.List[0].Type
	if pointer, ok := receiverType.(*ast.StarExpr); ok {
		receiverType = pointer.X
	}
	identifier, ok := receiverType.(*ast.Ident)
	return ok && ast.IsExported(identifier.Name)
}

func forbiddenRepositoryInitiatorImport(importPath string) bool {
	return strings.Contains(importPath, "/internal/modules/") ||
		strings.Contains(importPath, "/internal/services/") ||
		strings.Contains(importPath, "/internal/modules/base/biz") ||
		strings.Contains(importPath, "/pkg/events") ||
		strings.HasSuffix(importPath, "/internal/pkg/workorchruntimecontract")
}

func assertNoInitiatorEventOperations(t *testing.T, path string, file *ast.File) {
	t.Helper()
	forbiddenCalls := map[string]bool{
		"Send":        true,
		"Post":        true,
		"Subscribe":   true,
		"Unsubscribe": true,
	}
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if ok && forbiddenCalls[selector.Sel.Name] {
			t.Errorf("repository Initiator %s performs forbidden EventHub operation %s", filepath.Base(path), selector.Sel.Name)
		}
		return true
	})
}

func repositoryBoundaryRoot(t *testing.T) string {
	t.Helper()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve repository boundary test path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", "..", ".."))
}

type repositoryBoundaryError struct {
	path    string
	message string
}

func (e *repositoryBoundaryError) Error() string {
	return e.path + ": " + e.message
}
