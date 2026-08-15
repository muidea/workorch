package readmodelprojection

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestWD5WorkspaceRunContextFacadeImplementationsStayPrivate(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	expected := map[string]bool{
		"activationSnapshot":                       false,
		"canonicalStepKind":                        false,
		"currentRunStatusStep":                     false,
		"diffWorkspaceProjectionSections":          false,
		"mapAcceptanceProjection":                  false,
		"mapConvergenceStatus":                     false,
		"mapRunContextBundle":                      false,
		"mapRunObservability":                      false,
		"mapRunStatus":                             false,
		"mapSteps":                                 false,
		"mapWorkspaceWithRunProjection":            false,
		"projectRunSnapshotCompact":                false,
		"projectRunSnapshotDetail":                 false,
		"projectWorkspaceRuntimeView":              false,
		"projectWorkspaceRuntimeViewWithActivity":  false,
		"projectWorkspaceSnapshotDetail":           false,
		"projectWorkspaceSnapshotSummary":          false,
		"publicRunObservability":                   false,
		"publicRunSnapshot":                        false,
		"publicWorkspaceExecutionActivation":       false,
		"publicWorkspaceSnapshot":                  false,
		"runRecoverySnapshotFromModels":            false,
		"runSnapshotFromModels":                    false,
		"runSnapshotTaskDecompositions":            false,
		"runSnapshotTaskProgress":                  false,
		"selectDisplayRunID":                       false,
		"selectWorkspaceProjectionRun":             false,
		"workspaceContextBundleFromSnapshots":      false,
		"workspaceSnapshotFromModels":              false,
		"workspaceSnapshotTaskProgress":            false,
		"workspaceSnapshotWithRunSnapshot":         false,
		"workspaceTaskDecompositionsFromSnapshots": false,
	}
	dir := filepath.Dir(file)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), filepath.Join(dir, entry.Name()), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, declaration := range parsed.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok {
				continue
			}
			if _, tracked := expected[function.Name.Name]; tracked {
				if ast.IsExported(function.Name.Name) {
					t.Fatalf("facade implementation %s must remain package-private", function.Name.Name)
				}
				expected[function.Name.Name] = true
			}
		}
	}
	for name, found := range expected {
		if !found {
			t.Errorf("package-private facade implementation %s not found", name)
		}
	}
}

func TestWD5ProjectionExportSurfaceIsExplicitlyBounded(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	allowed := map[string]bool{
		"ActivityProjection":          false,
		"ContextProjection":           false,
		"EvidenceProjection":          false,
		"GovernanceProjection":        false,
		"RunProjection":               false,
		"RunSnapshotModelInput":       false,
		"WorkspaceProjection":         false,
		"WorkspaceSnapshotModelInput": false,
	}
	dir := filepath.Dir(file)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	mark := func(name string) {
		if !ast.IsExported(name) {
			return
		}
		if _, ok := allowed[name]; !ok {
			t.Errorf("unexpected exported projection declaration %s", name)
			return
		}
		allowed[name] = true
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), filepath.Join(dir, entry.Name()), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, declaration := range parsed.Decls {
			switch typed := declaration.(type) {
			case *ast.FuncDecl:
				mark(typed.Name.Name)
			case *ast.GenDecl:
				for _, specification := range typed.Specs {
					switch spec := specification.(type) {
					case *ast.TypeSpec:
						mark(spec.Name.Name)
					case *ast.ValueSpec:
						for _, name := range spec.Names {
							mark(name.Name)
						}
					}
				}
			}
		}
	}
	for name, found := range allowed {
		if !found {
			t.Errorf("expected projection export %s not found", name)
		}
	}
}

func TestWD5ProjectionMicroFileConsolidationRemainsBounded(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	entries, err := os.ReadDir(filepath.Dir(file))
	if err != nil {
		t.Fatal(err)
	}
	production := 0
	consolidated := 0
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		if strings.HasPrefix(name, "projection_family_") {
			t.Fatalf("numeric projection family file %q must use a business-semantic name", name)
		}
		production++
		if strings.HasPrefix(name, "projection_") {
			consolidated++
		}
	}
	if production > 150 {
		t.Fatalf("projection production files = %d, want <= 150 after WD5 mechanical consolidation", production)
	}
	if consolidated < 10 {
		t.Fatalf("consolidated projection files = %d, want at least 10", consolidated)
	}
}

func TestWD5ProductionProjectionUsesSemanticFacades(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", "..", "..", "..", ".."))
	forbidden := []string{
		"readmodelprojection.ActivationSnapshot(",
		"readmodelprojection.BuildWorkspaceActivityFeedContent(",
		"readmodelprojection.CommitActivityFeedTransaction(",
		"readmodelprojection.CanonicalStepKind(",
		"readmodelprojection.CurrentRunStatusStep(",
		"readmodelprojection.DiffWorkspaceProjectionSections(",
		"readmodelprojection.MapAcceptanceProjection(",
		"readmodelprojection.MapConvergenceStatus(",
		"readmodelprojection.MapEventRecords(",
		"readmodelprojection.MapMemoryRecallTraces(",
		"readmodelprojection.MapRunContextBundle(",
		"readmodelprojection.MapRunObservability(",
		"readmodelprojection.MapRunStatus(",
		"readmodelprojection.MapSteps(",
		"readmodelprojection.MapWorkspaceWithRunProjection(",
		"readmodelprojection.PageRunArtifacts(",
		"readmodelprojection.PageRunPatches(",
		"readmodelprojection.PageRunResults(",
		"readmodelprojection.PageRunRuntimeCheckpoints(",
		"readmodelprojection.PageRunSteps(",
		"readmodelprojection.PageRunToolCalls(",
		"readmodelprojection.PageRunValidationResults(",
		"readmodelprojection.ProjectRunSnapshotCompact(",
		"readmodelprojection.ProjectRunSnapshotDetail(",
		"readmodelprojection.ProjectWorkspaceRuntimeView(",
		"readmodelprojection.ProjectWorkspaceRuntimeViewWithActivity(",
		"readmodelprojection.ProjectWorkspaceSnapshotDetail(",
		"readmodelprojection.ProjectWorkspaceSnapshotSummary(",
		"readmodelprojection.PublicRunObservability(",
		"readmodelprojection.PublicRunSnapshot(",
		"readmodelprojection.PublicWorkspaceExecutionActivation(",
		"readmodelprojection.PublicWorkspaceSnapshot(",
		"readmodelprojection.RunRecoverySnapshotFromModels(",
		"readmodelprojection.RunSnapshotFromModels(",
		"readmodelprojection.RunSnapshotTaskDecompositions(",
		"readmodelprojection.RunSnapshotTaskProgress(",
		"readmodelprojection.SelectDisplayRunID(",
		"readmodelprojection.SelectWorkspaceProjectionRun(",
		"readmodelprojection.WorkspaceContextBundleFromSnapshots(",
		"readmodelprojection.WorkspaceSnapshotFromModels(",
		"readmodelprojection.WorkspaceSnapshotTaskProgress(",
		"readmodelprojection.WorkspaceSnapshotWithRunSnapshot(",
		"readmodelprojection.WorkspaceTaskDecompositionsFromSnapshots(",
		"readmodelprojection.MapApproval(",
		"readmodelprojection.MapDecision(",
		"readmodelprojection.PageApprovals(",
		"readmodelprojection.PageDecisionTickets(",
		"readmodelprojection.ProjectApprovalView(",
		"readmodelprojection.ProjectDecisionView(",
		"readmodelprojection.HasOpenDecisionBlocker(",
		"readmodelprojection.HasOpenApprovalBlocker(",
		"readmodelprojection.NormalizeWorkspaceGovernanceProjection(",
		"readmodelprojection.WorkspacePromptMode(",
	}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == ".git" || entry.Name() == "node_modules" || strings.Contains(path, string(filepath.Separator)+"readmodelprojection") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, token := range forbidden {
			if strings.Contains(string(data), token) {
				return fmt.Errorf("%s uses projection helper %q directly; use its typed projection facade", path, token)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestWD5SemanticFacadesContainOnlyBoundPureFunctions(t *testing.T) {
	facades := map[string]any{
		"workspace":  WorkspaceProjection(),
		"run":        RunProjection(),
		"governance": GovernanceProjection(),
		"context":    ContextProjection(),
		"evidence":   EvidenceProjection(),
		"activity":   ActivityProjection(),
	}
	for name, facade := range facades {
		value := reflect.ValueOf(facade)
		typeOf := value.Type()
		if value.Kind() != reflect.Struct || value.NumField() == 0 {
			t.Fatalf("%s facade = %T, want non-empty stateless function table", name, facade)
		}
		for index := 0; index < value.NumField(); index++ {
			field := value.Field(index)
			if field.Kind() != reflect.Func {
				t.Fatalf("%s.%s kind = %s, want func-only facade", name, typeOf.Field(index).Name, field.Kind())
			}
			if field.IsNil() {
				t.Fatalf("%s.%s is nil", name, typeOf.Field(index).Name)
			}
		}
	}
}
