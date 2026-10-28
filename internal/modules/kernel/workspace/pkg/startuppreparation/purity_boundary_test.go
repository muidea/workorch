package startuppreparation

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// This is a source-boundary regression, not a mathematical transitive purity
// proof. Behavioral tests independently verify detached values and no owner
// writes, and the application read tests install only the state query owner.
func TestPreparationPureCompilationBoundary(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("source location unavailable")
	}
	dir := filepath.Dir(source)
	root := filepath.Clean(filepath.Join(dir, "../../../../../.."))
	paths, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	paths = append(paths, filepath.Join(root, "internal/modules/application/workorchd/biz/appservice/activationcore/prepared_bootstrap.go"), filepath.Join(root, "internal/modules/application/workorchd/biz/appservice/activationcore/runtime_bootstrap.go"))
	paths = append(paths, filepath.Join(root, "internal/modules/application/workorchd/biz/appservice/activationports/prepared_execution_policy.go"))
	paths = append(paths, filepath.Join(root, "internal/modules/application/workorchd/biz/appservice/activationports/prepared_bootstrap.go"))
	configurationPaths, err := filepath.Glob(filepath.Join(root, "internal/pkg/definitionconfiguration/*.go"))
	if err != nil {
		t.Fatal(err)
	}
	paths = append(paths, configurationPaths...)
	allowed := map[string]bool{
		"github.com/muidea/workorch/internal/pkg/definitionconfiguration": true,
		"github.com/muidea/workorch/internal/pkg/runtimeprofiledecode":    true,
		"github.com/muidea/workorch/internal/pkg/workorcheventpayload":    true,
		"crypto/sha256": true, "encoding/hex": true, "encoding/json": true, "fmt": true, "reflect": true, "slices": true, "sort": true, "strings": true, "unicode/utf8": true,
		"github.com/muidea/workorch/internal/pkg/common":                                                  true,
		"github.com/muidea/workorch/internal/pkg/workorchruntimecontract":                                 true,
		"github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/contracts":                      true,
		"github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models":                         true,
		"github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/definitionadmission":            true,
		"github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/startuppreparation":             true,
		"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/activationcore": true,
	}
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		unit, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, spec := range unit.Imports {
			name, err := strconv.Unquote(spec.Path.Value)
			if err != nil || !allowed[name] {
				t.Errorf("pure compilation file %s imports %q", filepath.Base(path), spec.Path.Value)
			}
		}
		ast.Inspect(unit, func(node ast.Node) bool {
			switch n := node.(type) {
			case *ast.GoStmt:
				t.Errorf("pure file %s starts a goroutine", filepath.Base(path))
			case *ast.Ident:
				switch n.Name {
				case "InitialFacts", "EnsureManagedConfigs", "EnsureWorkspaceDefinitionAdmission", "DriveRuntime", "SendEvent", "PostEvent", "EventHub", "ReadFile", "WriteFile", "RefreshCatalog", "InitializeRuntime":
					t.Errorf("pure file %s uses forbidden boundary %s", filepath.Base(path), n.Name)
				}
			}
			return true
		})
	}
}
