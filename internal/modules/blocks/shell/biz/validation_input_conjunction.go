package shell

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"go/ast"
	"go/token"
	"regexp"
	"strconv"
	"strings"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const shellGoConjunctiveEntrypointObservationProtocolRef = "shell/go-test-json/conjunctive-package-behavior"
const shellConjunctiveInputContentContractRef = "shell/go-test-input/conjunctive-entrypoint/v1"
const shellConjunctiveReviewScopeContractRef = "shell/go-test-input/conjunctive-package-review/v1"
const shellConjunctiveReviewScopeRequirements = "The supplied files are the complete current requirement input package, including every required native check, all helpers/fixtures and present root module/workspace definitions. Shell executes every exact original and supplemental native check in this package conjunctively. A missing, failed, skipped or incomplete member cannot satisfy the requirement. " + shellIsolatedReviewScopeRequirements
const shellConjunctiveInputContentRequirements = "Create exactly targetRef as one Go test file declaring every listed nativeCheckRef with signature func(t *testing.T), once each. All members belong to the same immutable requirement package; do not omit or alias any member. Each source obligation check must assert its own exact criterion and declaration, not substitute the shared CWI criterion or another obligation. Named same-package helpers, types, constants and data-only variables are allowed; no init, TestMain, methods, unlisted Test functions, global initializer calls or closures, nested modules, cgo or imports of other validation packages. Keep every helper and fixture within this package and the complete review within 16 files and 64 KiB. Exercise actual workspace implementation in-process or through a freshly Go-built executable inheriting owner instrumentation. Missing dependencies, skips, source markers, test-local substitutes and instrumentation overrides cannot supply proof. Package execution is not full-project build/test proof or an OS sandbox; original full-suite criteria require explicit execution and assertions. Creation, readiness and consistency review grant no execution or acceptance credit."

var shellConjunctiveNativeIdentity = regexp.MustCompile(`^TestWorkOrchCWI_([A-F0-9]{64})_Check_[A-Z0-9_]+_ConjunctiveInput$`)

func shellConjunctiveRequirementIdentity(requirement runtimecontract.RuntimeValidationProofRequirement) string {
	body, _ := json.Marshal(requirement.Normalized())
	digest := sha256.Sum256(append([]byte("shell-conjunctive-requirement/v1\x00"), body...))
	return hex.EncodeToString(digest[:])
}

func shellConjunctiveNativePackage(native string) string {
	match := shellConjunctiveNativeIdentity.FindStringSubmatch(strings.TrimSuffix(native, "_Behavior"))
	if len(match) != 2 {
		return ""
	}
	return "workorch_validation/conjunctive/" + strings.ToLower(match[1])
}

// 完整 package 允许同一 requirement 的多个检查，不允许额外 Test 或执行期初始化。
func shellConjunctiveInputDeclarations(file *ast.File, checks []runtimecontract.RuntimeValidationCheckAttestationBinding) bool {
	allowed := make(map[string]bool)
	for _, check := range checks {
		for _, native := range check.NativeCheckRefs() {
			allowed[native] = true
		}
	}
	for _, item := range file.Imports {
		value, err := strconv.Unquote(item.Path.Value)
		if err != nil || value == "C" || strings.HasPrefix(value, ".") || strings.Contains("/"+value+"/", "/workorch_validation/") {
			return false
		}
	}
	for _, declaration := range file.Decls {
		switch item := declaration.(type) {
		case *ast.GenDecl:
			switch item.Tok {
			case token.IMPORT, token.TYPE, token.CONST:
				continue
			case token.VAR:
				pure := true
				ast.Inspect(item, func(node ast.Node) bool {
					switch node.(type) {
					case *ast.CallExpr, *ast.FuncLit:
						pure = false
					}
					return pure
				})
				if pure {
					continue
				}
			}
		case *ast.FuncDecl:
			if item.Recv == nil && item.Name.Name != "init" && item.Name.Name != "TestMain" &&
				(!strings.HasPrefix(item.Name.Name, "Test") || allowed[item.Name.Name]) {
				continue
			}
		}
		return false
	}
	return true
}
