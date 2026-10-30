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
	for _, folder := range []string{"internal/pkg/bootstrapcompiler", "internal/pkg/runtimeprojection", "internal/modules/kernel/workspace/pkg/preparationcompiler"} {
		more, err := filepath.Glob(filepath.Join(root, folder, "*.go"))
		if err != nil {
			t.Fatal(err)
		}
		paths = append(paths, more...)
	}
	allowed := map[string]bool{
		"time": true,
		"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract":                   true,
		"github.com/muidea/workorch/internal/pkg/bootstrapcompiler":                            true,
		"github.com/muidea/workorch/internal/pkg/runtimeprojection":                            true,
		"github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/preparationcompiler": true,
		"github.com/muidea/workorch/internal/modules/blocks/soul/pkg/models":                   true,
		"github.com/muidea/workorch/internal/modules/blocks/mcpregistry/pkg/models":            true,
		"github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models":                 true,
		"github.com/muidea/workorch/internal/pkg/workorchskillcontract":                        true,
		"github.com/muidea/workorch/internal/pkg/workorchmemorycontract":                       true,
		"github.com/muidea/workorch/internal/pkg/memoryselection":                              true,
		"github.com/muidea/workorch/internal/pkg/definitionconfiguration":                      true,
		"github.com/muidea/workorch/internal/pkg/runtimeprofiledecode":                         true,
		"github.com/muidea/workorch/internal/pkg/workorcheventpayload":                         true,
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
			// These exact files consume detached owner value types / payload hashing
			// only. Do not grant blanket access to owner packages in all compilers.
			valueImport := strings.Contains(filepath.ToSlash(path), "/pkg/preparationcompiler/") && (filepath.Base(path) == "completion.go" && name == "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models" ||
				filepath.Base(path) == "first_execution.go" && (name == "github.com/muidea/workorch/internal/modules/blocks/file/pkg/preparation" || name == "github.com/muidea/workorch/internal/pkg/fileobservation"))
			if err != nil || !allowed[name] && !valueImport {
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
