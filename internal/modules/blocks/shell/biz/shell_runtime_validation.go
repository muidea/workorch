package shell

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	modulecommon "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/common"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const (
	shellValidationStaticUnavailableReason       = "owner_static_execution_unavailable"
	shellValidationApplicableInputRequiredReason = "applicable_workspace_input_required"
	shellGoTestObservationProtocolRef            = "shell/go-test-json/v1"
)

// PreflightRuntimeValidationRecipes returns the deterministic subset of
// activation-declared recipes that this Shell owner can execute under its
// current static environment and command policy. It deliberately does not
// start a process, allocate a managed session, or inspect mutable validation
// targets: those checks remain part of the sealed-operation feasibility probe
// immediately before dispatch. A stale/missing executable therefore cannot be
// exposed to the provider as a selectable recipe.
func (a *RuntimeAdapter) PreflightRuntimeValidationRecipes(workspaceDirectory string, recipes []runtimemodel.RuntimeValidationRecipe) ([]runtimemodel.RuntimeValidationRecipe, error) {
	if a == nil {
		return nil, common.ErrInvalidArgument
	}
	directory, err := filepath.Abs(strings.TrimSpace(workspaceDirectory))
	if err != nil || directory == "" {
		return nil, common.ErrInvalidArgument
	}
	info, err := os.Stat(directory)
	if err != nil || !info.IsDir() {
		if err == nil {
			err = common.ErrInvalidArgument
		}
		return nil, err
	}
	confirmed := make([]runtimemodel.RuntimeValidationRecipe, 0, len(recipes))
	for _, recipe := range runtimemodel.NormalizeRuntimeValidationRecipes(recipes) {
		command, args, specErr := shellCommandSpec(recipe.Command, recipe.Args)
		if specErr != nil {
			continue
		}
		execCommand, execArgs, _ := normalizeShellExecutionCommand(command, args)
		if _, lookErr := exec.LookPath(execCommand); lookErr != nil {
			continue
		}
		if policyErr := validateShellCommandPolicy(execCommand, execArgs, directory, true); policyErr != nil {
			continue
		}
		if _, timeoutErr := shellCommandTimeout(recipe.TimeoutMillis, a.timeout); timeoutErr != nil {
			continue
		}
		confirmed = append(confirmed, recipe)
	}
	return runtimemodel.NormalizeRuntimeValidationRecipes(confirmed), nil
}

// PreflightRuntimeValidationRecipesForManifest narrows the static Shell
// execution envelope to recipes that are applicable to the current File-owner
// workspace manifest.  It never starts a process or acquires another owner's
// resource.  The caller must bind the same manifest revision into the returned
// admissibility observation so a later File effect retires this selection.
func (a *RuntimeAdapter) PreflightRuntimeValidationRecipesForManifest(workspaceDirectory string, recipes []runtimemodel.RuntimeValidationRecipe, manifest runtimemodel.WorkspaceManifest) ([]runtimemodel.RuntimeValidationRecipe, error) {
	manifest.Revision = strings.TrimSpace(manifest.Revision)
	manifest.TargetRefs = normalizeShellStrings(manifest.TargetRefs)
	if manifest.Revision == "" {
		return nil, common.ErrInvalidArgument
	}
	confirmed, err := a.PreflightRuntimeValidationRecipes(workspaceDirectory, recipes)
	if err != nil {
		return nil, err
	}
	return runtimeValidationRecipesApplicableToManifest(confirmed, manifest.TargetRefs), nil
}

func runtimeValidationRecipesApplicableToManifest(recipes []runtimemodel.RuntimeValidationRecipe, targetRefs []string) []runtimemodel.RuntimeValidationRecipe {
	applicable := make([]runtimemodel.RuntimeValidationRecipe, 0, len(recipes))
	for _, recipe := range runtimemodel.NormalizeRuntimeValidationRecipes(recipes) {
		if runtimeValidationRecipeApplicableToManifest(recipe, targetRefs) {
			applicable = append(applicable, recipe)
		}
	}
	return runtimemodel.NormalizeRuntimeValidationRecipes(applicable)
}

// shellValidationSurfaces preserves every activation-declared recipe as one
// Shell-owner readiness fact while keeping the executable recipe inventory
// ready-only. It owns no Runtime transition and performs no additional I/O.
func shellValidationSurfaces(declared, staticallyReady, ready []runtimemodel.RuntimeValidationRecipe, requirements []runtimemodel.RuntimeValidationProofRequirement, binding runtimemodel.RuntimeWorkspaceManifestBinding, catalogObservedRevision string, workspaceDirectories ...string) []runtimemodel.RuntimeValidationSurface {
	binding = binding.Normalized()
	catalogObservedRevision = strings.TrimSpace(catalogObservedRevision)
	workspaceDirectory := ""
	if len(workspaceDirectories) != 0 {
		workspaceDirectory = strings.TrimSpace(workspaceDirectories[0])
	}
	staticIDs := shellValidationRecipeIDs(staticallyReady)
	readyIDs := shellValidationRecipeIDs(ready)
	ret := make([]runtimemodel.RuntimeValidationSurface, 0, len(declared))
	for _, recipe := range runtimemodel.NormalizeRuntimeValidationRecipes(declared) {
		schemaHash := shellValidationRecipeSchemaHash(recipe)
		bindings := make([]runtimemodel.RuntimeValidationProofRequirementBinding, 0)
		checks := make([]string, 0)
		preparations := make([]runtimemodel.RuntimeValidationProofPreparation, 0)
		compatibleRequirements := 0
		if staticIDs[recipe.ID] {
			for _, requirement := range runtimemodel.NormalizeRuntimeValidationProofRequirements(requirements) {
				if requirement.Validate() != nil ||
					(requirement.ValidationRecipeID != "" && requirement.ValidationRecipeID != recipe.ID) {
					continue
				}
				compatibleRequirements++
				attestations, observable := shellValidationCheckAttestations(recipe, requirement)
				if !observable {
					continue
				}
				preparation, observableNow, preparable := shellValidationProofPreparation(workspaceDirectory, binding.Manifest, catalogObservedRevision, readyIDs[recipe.ID], recipe, requirement, attestations)
				if !observableNow {
					if preparable {
						preparations = append(preparations, preparation)
					}
					continue
				}
				if !readyIDs[recipe.ID] {
					continue
				}
				ownerBinding, err := runtimemodel.RuntimeValidationProofRequirementBindingForRecipe(requirement, recipe.ID, attestations...)
				if err != nil {
					continue
				}
				bindings = append(bindings, ownerBinding)
				checks = append(checks, requirement.RequiredCheckRefs...)
			}
		}
		surface := runtimemodel.RuntimeValidationSurface{
			ID: recipe.ID, OwnerModuleID: ModuleID, OperationKind: runtimemodel.RuntimeBlockKindManagedValidationSession,
			CheckRefs: normalizeShellStrings(checks), ProofRequirementBindings: bindings, ProofRequirementPreparations: preparations,
			ProofApplicability: runtimemodel.RuntimeValidationProofNotDeclared, EffectScope: runtimemodel.RuntimeEffectEphemeralSession,
			IdempotencyClass: runtimemodel.RuntimeOperationIdempotencyAtMostOnce,
			InputSchemaHash:  schemaHash,
		}
		if len(bindings) != 0 {
			surface.ProofApplicability = runtimemodel.RuntimeValidationProofDirect
		} else if len(preparations) != 0 {
			surface.ProofApplicability = runtimemodel.RuntimeValidationProofDirect
		} else if compatibleRequirements != 0 {
			surface.ProofApplicability = runtimemodel.RuntimeValidationProofAuxiliary
		}
		switch {
		case readyIDs[recipe.ID]:
			surface.Readiness = runtimemodel.RuntimeValidationSurfaceReady
			surface.InputBindingRef = "shell-validation-input/" + schemaHash + "/" + binding.Manifest.Revision
		case staticIDs[recipe.ID]:
			surface.Readiness = runtimemodel.RuntimeValidationSurfacePrerequisiteRequired
			surface.ReadinessReason = shellValidationApplicableInputRequiredReason
			surface.PrerequisiteRef = "shell-validation-prerequisite/" + schemaHash + "/applicable-workspace-input"
			if constraint, ok := shellProspectiveTargetSatisfactionConstraint(recipe, binding.Manifest.TargetRefs, catalogObservedRevision); ok {
				surface.ProspectiveSatisfactionContractRef = modulecommon.ProspectiveSatisfactionContractRef
				surface.ProspectiveTargetSatisfactionConstraint = &constraint
			}
		default:
			surface.Readiness = runtimemodel.RuntimeValidationSurfaceUnavailable
			surface.ReadinessReason = shellValidationStaticUnavailableReason
		}
		ret = append(ret, surface)
	}
	return runtimemodel.NormalizeRuntimeValidationSurfaces(ret)
}

// shellValidationCheckAttestations maps Definition-owned checks to exact Go
// test identities emitted by the owner-native verbose test protocol. Recipes
// without a structured native observation protocol remain executable owner
// tools, but cannot become direct per-CWI proof surfaces.
func shellValidationCheckAttestations(recipe runtimemodel.RuntimeValidationRecipe, requirement runtimemodel.RuntimeValidationProofRequirement) ([]runtimemodel.RuntimeValidationCheckAttestationBinding, bool) {
	command, args, err := shellCommandSpec(recipe.Command, recipe.Args)
	if err != nil {
		return nil, false
	}
	command, args, _ = normalizeShellExecutionCommand(command, args)
	if filepath.Base(strings.TrimSpace(command)) != "go" || len(args) == 0 || strings.TrimSpace(args[0]) != "test" {
		return nil, false
	}
	for index := 1; index < len(args); index++ {
		arg := strings.TrimSpace(args[index])
		if arg == "-run" || arg == "--run" || strings.HasPrefix(arg, "-run=") || strings.HasPrefix(arg, "--run=") {
			return nil, false
		}
	}
	ret := make([]runtimemodel.RuntimeValidationCheckAttestationBinding, 0, len(requirement.RequiredCheckRefs))
	for _, required := range requirement.Normalized().RequiredCheckRefs {
		suffix := strings.TrimPrefix(required, "workorch-cwi-check-")
		if suffix == required || suffix == "" {
			return nil, false
		}
		ret = append(ret, runtimemodel.RuntimeValidationCheckAttestationBinding{
			RequiredCheckRef: required, ObservationProtocolRef: shellGoTestObservationProtocolRef,
			NativeCheckRef: "TestWorkOrchCWI_" + strings.ToUpper(suffix),
		})
	}
	return runtimemodel.NormalizeRuntimeValidationCheckAttestationBindings(ret), len(ret) != 0
}

func shellValidationNativeCheckTargetRef(requirement runtimemodel.RuntimeValidationProofRequirement) string {
	requirement = requirement.Normalized()
	digest := sha256.Sum256([]byte("workorch-shell-native-check-target/v1\x00" + requirement.Hash))
	return "workorch_validation/cwi_" + hex.EncodeToString(digest[:10]) + "_test.go"
}

func shellValidationProofPreparation(workspaceDirectory string, manifest runtimemodel.WorkspaceManifest, catalogObservedRevision string, recipeReady bool, recipe runtimemodel.RuntimeValidationRecipe, requirement runtimemodel.RuntimeValidationProofRequirement, attestations []runtimemodel.RuntimeValidationCheckAttestationBinding) (runtimemodel.RuntimeValidationProofPreparation, bool, bool) {
	nativeChecksDeclared := shellNativeValidationChecksDeclared(workspaceDirectory, manifest.TargetRefs, attestations)
	if nativeChecksDeclared && recipeReady {
		return runtimemodel.RuntimeValidationProofPreparation{}, true, true
	}
	targetRef := ""
	if !nativeChecksDeclared {
		targetRef = shellValidationNativeCheckTargetRef(requirement)
	}
	if targetRef != "" && shellStringIncluded(manifest.TargetRefs, targetRef) {
		digest := sha256.Sum256([]byte("workorch-shell-native-check-repair-target/v1\x00" + requirement.Hash + "\x00" + manifest.Revision))
		base := strings.TrimSuffix(targetRef, "_test.go")
		for index := 0; index < 8; index++ {
			candidate := fmt.Sprintf("%s_repair_%s_%d_test.go", base, hex.EncodeToString(digest[:4]), index+1)
			if !shellStringIncluded(manifest.TargetRefs, candidate) {
				targetRef = candidate
				break
			}
		}
	}
	requiredTargets := []string(nil)
	if targetRef != "" {
		requiredTargets = []string{targetRef}
	}
	constraint, ok := shellProspectiveTargetSatisfactionConstraint(recipe, manifest.TargetRefs, catalogObservedRevision, requiredTargets...)
	if !ok {
		return runtimemodel.RuntimeValidationProofPreparation{}, false, false
	}
	preparation := runtimemodel.RuntimeValidationProofPreparation{
		RequirementHash: requirement.Hash, DefinitionRevision: requirement.DefinitionRevision,
		CoverageWorkItemID: requirement.CoverageWorkItemID, DeclarationRef: requirement.DeclarationRef,
		ValidationRecipeID: recipe.ID, TargetRef: targetRef,
		CheckAttestations:                       runtimemodel.CloneRuntimeValidationCheckAttestationBindings(attestations),
		ProspectiveTargetSatisfactionConstraint: constraint,
	}.Normalized()
	if preparation.Validate() != nil {
		return runtimemodel.RuntimeValidationProofPreparation{}, false, false
	}
	return preparation, false, true
}

func shellNativeValidationChecksDeclared(workspaceDirectory string, targetRefs []string, attestations []runtimemodel.RuntimeValidationCheckAttestationBinding) bool {
	root, err := filepath.Abs(strings.TrimSpace(workspaceDirectory))
	if err != nil || root == "" || len(attestations) == 0 {
		return false
	}
	expected := make(map[string]struct{}, len(attestations))
	for _, attestation := range attestations {
		attestation = attestation.Normalized()
		if attestation.ObservationProtocolRef != shellGoTestObservationProtocolRef || attestation.NativeCheckRef == "" {
			return false
		}
		expected[attestation.NativeCheckRef] = struct{}{}
	}
	for _, targetRef := range normalizeShellStrings(targetRefs) {
		if !strings.HasSuffix(targetRef, "_test.go") {
			continue
		}
		path := filepath.Join(root, filepath.FromSlash(targetRef))
		relative, relErr := filepath.Rel(root, path)
		if relErr != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			continue
		}
		file, parseErr := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if parseErr != nil {
			continue
		}
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Recv != nil || !shellGoTestFunctionHasAssertion(function) {
				continue
			}
			delete(expected, function.Name.Name)
		}
	}
	return len(expected) == 0
}

func shellGoTestFunctionHasAssertion(function *ast.FuncDecl) bool {
	if function == nil || function.Type == nil || function.Type.Params == nil || len(function.Type.Params.List) != 1 || function.Type.Results != nil || function.Body == nil {
		return false
	}
	parameter := function.Type.Params.List[0]
	if len(parameter.Names) != 1 {
		return false
	}
	pointer, ok := parameter.Type.(*ast.StarExpr)
	if !ok {
		return false
	}
	selector, ok := pointer.X.(*ast.SelectorExpr)
	if !ok || selector.Sel == nil || selector.Sel.Name != "T" {
		return false
	}
	parameterName := parameter.Names[0].Name
	hasAssertion := false
	ast.Inspect(function.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selected, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || selected.Sel == nil {
			return true
		}
		receiver, ok := selected.X.(*ast.Ident)
		if !ok || receiver.Name != parameterName {
			return true
		}
		switch selected.Sel.Name {
		case "Error", "Errorf", "Fatal", "Fatalf", "Fail", "FailNow":
			hasAssertion = true
			return false
		default:
			return true
		}
	})
	return hasAssertion
}

func instrumentManagedValidationNativeChecks(request *RuntimeCommandRequest) error {
	if request == nil || len(request.ValidationCheckAttestations) == 0 {
		return nil
	}
	requiredRefs := make([]string, 0, len(request.ValidationCheckAttestations))
	nativeChecks := make([]string, 0, len(request.ValidationCheckAttestations))
	for _, raw := range request.ValidationCheckAttestations {
		attestation := raw.Normalized()
		if attestation.ObservationProtocolRef != shellGoTestObservationProtocolRef ||
			!strings.HasPrefix(attestation.NativeCheckRef, "TestWorkOrchCWI_") {
			return fmt.Errorf("managed validation has unsupported native check attestation")
		}
		requiredRefs = append(requiredRefs, attestation.RequiredCheckRef)
		nativeChecks = append(nativeChecks, attestation.NativeCheckRef)
	}
	if err := runtimemodel.ValidateRuntimeValidationCheckAttestationBindings(request.ValidationCheckAttestations, requiredRefs); err != nil {
		return err
	}
	command, args, err := shellCommandSpec(request.Command, request.Args)
	if err != nil {
		return err
	}
	command, args, _ = normalizeShellExecutionCommand(command, args)
	if filepath.Base(strings.TrimSpace(command)) != "go" || len(args) == 0 || strings.TrimSpace(args[0]) != "test" {
		return fmt.Errorf("managed validation native check attestation requires go test")
	}
	for index := 1; index < len(args); index++ {
		arg := strings.TrimSpace(args[index])
		if arg == "-run" || arg == "--run" || strings.HasPrefix(arg, "-run=") || strings.HasPrefix(arg, "--run=") {
			return fmt.Errorf("managed validation recipe already constrains native checks")
		}
	}
	instrumented := make([]string, 0, len(args)+5)
	instrumented = append(instrumented, args[0], "-json", "-count=1", "-run", "^("+strings.Join(normalizeShellStrings(nativeChecks), "|")+")$")
	instrumented = append(instrumented, args[1:]...)
	request.Command = command
	request.Args = instrumented
	request.ValidationCheckAttestations = runtimemodel.CloneRuntimeValidationCheckAttestationBindings(request.ValidationCheckAttestations)
	return nil
}

// shellProspectiveTargetSatisfactionConstraint publishes one mandatory clause
// for each missing input of the exact activation-sealed recipe. A bounded set
// may cover several clauses, but every clause is required; Shell never
// publishes an alternative aggregation.
func shellProspectiveTargetSatisfactionConstraint(recipe runtimemodel.RuntimeValidationRecipe, currentTargetRefs []string, catalogObservedRevision string, requiredNativeTargetRefs ...string) (runtimemodel.RuntimeProspectiveTargetSatisfactionConstraint, bool) {
	recipe = recipe.Normalized()
	catalogObservedRevision = strings.TrimSpace(catalogObservedRevision)
	isGoTest, packageRoots, ok := shellGoTestManifestRequirements(recipe)
	if !ok || !isGoTest || catalogObservedRevision == "" {
		return runtimemodel.RuntimeProspectiveTargetSatisfactionConstraint{}, false
	}
	currentTargetRefs = normalizeShellStrings(currentTargetRefs)
	clauses := make([]runtimemodel.RuntimeProspectiveTargetClause, 0, 2+len(packageRoots)+len(requiredNativeTargetRefs))
	for _, targetRef := range normalizeShellStrings(requiredNativeTargetRefs) {
		if shellStringIncluded(currentTargetRefs, targetRef) {
			continue
		}
		clauses = append(clauses, runtimemodel.RuntimeProspectiveTargetClause{
			ID: "native-check-target/" + targetRef,
			AllOf: []runtimemodel.RuntimeProspectiveTargetPredicate{{
				Kind: runtimemodel.RuntimeProspectiveTargetPredicatePathExact, Value: targetRef,
			}},
		})
	}
	if !shellManifestHasGoModule(currentTargetRefs) {
		clauses = append(clauses, runtimemodel.RuntimeProspectiveTargetClause{
			ID: "go-module-root",
			AllOf: []runtimemodel.RuntimeProspectiveTargetPredicate{{
				Kind: runtimemodel.RuntimeProspectiveTargetPredicatePathExact, Value: "go.mod",
			}},
		})
	}
	if !shellManifestHasApplicableGoTestTarget(currentTargetRefs) {
		predicates := []runtimemodel.RuntimeProspectiveTargetPredicate{{
			Kind: runtimemodel.RuntimeProspectiveTargetPredicatePathSuffix, Value: "_test.go",
		}}
		for _, segment := range shellManifestExcludedPathSegments() {
			predicates = append(predicates, runtimemodel.RuntimeProspectiveTargetPredicate{
				Kind: runtimemodel.RuntimeProspectiveTargetPredicatePathSegmentNotEqual, Value: segment,
			})
		}
		clauses = append(clauses, runtimemodel.RuntimeProspectiveTargetClause{ID: "go-test-input", AllOf: predicates})
	}
	for _, root := range packageRoots {
		if !shellManifestContainsPackageRoot(currentTargetRefs, root) {
			clauses = append(clauses, runtimemodel.RuntimeProspectiveTargetClause{
				ID: "go-package-root:" + root,
				AllOf: []runtimemodel.RuntimeProspectiveTargetPredicate{{
					Kind: runtimemodel.RuntimeProspectiveTargetPredicatePathPrefix, Value: root,
				}},
			})
		}
	}
	if len(clauses) == 0 || len(clauses) > runtimemodel.RuntimeProspectiveTargetRefsMaxCount {
		return runtimemodel.RuntimeProspectiveTargetSatisfactionConstraint{}, false
	}
	value := runtimemodel.RuntimeProspectiveTargetSatisfactionConstraint{
		SchemaVersion:                      runtimemodel.RuntimeProspectiveTargetSatisfactionConstraintSchemaCurrent,
		OwnerModuleID:                      ModuleID,
		ProspectiveSatisfactionContractRef: modulecommon.ProspectiveSatisfactionContractRef,
		ValidationSurfaceID:                recipe.ID,
		ValidationRecipeID:                 recipe.ID,
		InputSchemaHash:                    shellValidationRecipeSchemaHash(recipe),
		CatalogObservedRevision:            catalogObservedRevision,
		AllOf:                              clauses,
	}.Normalized()
	if err := value.Validate(); err != nil {
		return runtimemodel.RuntimeProspectiveTargetSatisfactionConstraint{}, false
	}
	return value, true
}

func shellValidationRecipeIDs(recipes []runtimemodel.RuntimeValidationRecipe) map[string]bool {
	ret := make(map[string]bool, len(recipes))
	for _, recipe := range runtimemodel.NormalizeRuntimeValidationRecipes(recipes) {
		ret[recipe.ID] = true
	}
	return ret
}

func shellValidationRecipeSchemaHash(recipe runtimemodel.RuntimeValidationRecipe) string {
	recipe = recipe.Normalized()
	digest := sha256.New()
	_, _ = fmt.Fprintf(digest, "%s\x00%s\x00%d\x00", recipe.ID, recipe.Command, recipe.TimeoutMillis)
	for _, arg := range recipe.Args {
		_, _ = fmt.Fprintf(digest, "%s\x00", arg)
	}
	return "shell-validation-recipe-" + hex.EncodeToString(digest.Sum(nil))[:20]
}

func runtimeValidationRecipeApplicableToManifest(recipe runtimemodel.RuntimeValidationRecipe, targetRefs []string) bool {
	isGoTest, packageRoots, ok := shellGoTestManifestRequirements(recipe)
	if !ok {
		return false
	}
	if !isGoTest {
		return true
	}
	allRefs := normalizeShellStrings(targetRefs)
	if !shellManifestHasGoModule(allRefs) {
		return false
	}
	if !shellManifestHasApplicableGoTestTarget(allRefs) {
		return false
	}
	for _, root := range packageRoots {
		if !shellManifestContainsPackageRoot(allRefs, root) {
			return false
		}
	}
	return true
}

func shellGoTestManifestRequirements(recipe runtimemodel.RuntimeValidationRecipe) (bool, []string, bool) {
	command, args, err := shellCommandSpec(recipe.Command, recipe.Args)
	if err != nil {
		return false, nil, false
	}
	command, args, _ = normalizeShellExecutionCommand(command, args)
	if filepath.Base(strings.TrimSpace(command)) != "go" || len(args) == 0 || strings.TrimSpace(args[0]) != "test" {
		return false, nil, true
	}
	roots := make([]string, 0)
	for idx := 1; idx < len(args); idx++ {
		arg := strings.TrimSpace(args[idx])
		if arg == "-run" || arg == "--run" {
			idx++
			continue
		}
		if arg == "" || strings.HasPrefix(arg, "-") || (arg != "." && !strings.HasPrefix(arg, "./")) {
			continue
		}
		root := strings.TrimPrefix(arg, "./")
		if wildcard := strings.Index(root, "..."); wildcard >= 0 {
			root = strings.TrimSuffix(root[:wildcard], "/")
		}
		root = strings.Trim(strings.TrimSpace(root), "/")
		if root != "" && root != "." {
			roots = append(roots, root)
		}
	}
	return true, normalizeShellStrings(roots), true
}

func shellManifestHasApplicableGoTestTarget(targetRefs []string) bool {
	for _, ref := range normalizeShellStrings(targetRefs) {
		clean := filepath.ToSlash(filepath.Clean(ref))
		if clean != "." && !shellManifestPathExcluded(clean) && strings.HasSuffix(pathBase(clean), "_test.go") {
			return true
		}
	}
	return false
}

func shellManifestHasGoModule(targetRefs []string) bool {
	for _, ref := range normalizeShellStrings(targetRefs) {
		if filepath.ToSlash(filepath.Clean(ref)) == "go.mod" {
			return true
		}
	}
	return false
}

func shellManifestContainsPackageRoot(targetRefs []string, root string) bool {
	prefix := strings.Trim(strings.TrimSpace(root), "/") + "/"
	for _, ref := range normalizeShellStrings(targetRefs) {
		if strings.HasPrefix(filepath.ToSlash(filepath.Clean(ref)), prefix) {
			return true
		}
	}
	return false
}

func shellManifestPathExcluded(ref string) bool {
	for _, part := range strings.Split(filepath.ToSlash(ref), "/") {
		for _, excluded := range shellManifestExcludedPathSegments() {
			if part == excluded {
				return true
			}
		}
	}
	return false
}

func shellManifestExcludedPathSegments() []string {
	return []string{".git", ".workorch-data", "node_modules", "vendor"}
}

func pathBase(ref string) string {
	if idx := strings.LastIndex(ref, "/"); idx >= 0 {
		return ref[idx+1:]
	}
	return ref
}

// PreflightRuntimeSemanticValidationRecipe performs the owner-local checks
// required before semantic feasibility may be reported as feasible. The
// target refs are part of the sealed proposal identity and must be passed to
// this preflight; otherwise a workspace that happens to contain an unrelated
// test file could be reported feasible while the later operation preflight
// rejects the exact target. The ordinary admissibility preflight remains
// available for the final sealed operation, but semantic planning must not be
// the first phase to discover that a validation target is absent.
func (a *RuntimeAdapter) PreflightRuntimeSemanticValidationRecipe(workspaceDirectory string, recipe runtimemodel.RuntimeValidationRecipe, targetRefs []string) error {
	confirmed, err := a.PreflightRuntimeValidationRecipes(workspaceDirectory, []runtimemodel.RuntimeValidationRecipe{recipe})
	if err != nil {
		return err
	}
	if len(confirmed) != 1 || confirmed[0].ID != recipe.ID {
		return fmt.Errorf("validation recipe %q is unavailable", recipe.ID)
	}
	command, args, specErr := shellCommandSpec(recipe.Command, recipe.Args)
	if specErr != nil {
		return specErr
	}
	request := RuntimeCommandRequest{
		WorkspaceDeliverableDirectory: workspaceDirectory,
		ValidationRecipeID:            recipe.ID,
		TargetRefs:                    append([]string(nil), targetRefs...),
		ValidationEvidence: runtimemodel.RuntimeValidationEvidence{
			Mode: "runtime_behavior", ObservationType: "semantic_feasibility",
		},
		ManagedValidation: runtimemodel.RuntimeManagedValidation{
			RuntimeManaged: true, Kind: runtimemodel.RuntimeBlockKindManagedValidationSession,
			OriginalNodeKind: runtimemodel.RuntimeBlockKindManagedValidationSession,
		},
	}
	return validateShellValidationPreflight(workspaceDirectory, command, args, request)
}

func validateShellValidationPreflight(deliverableDirectory string, command string, args []string, request RuntimeCommandRequest) error {
	if !shellRequestLooksLikeValidation(request, command, args) {
		return nil
	}
	if err := validateDeclaredValidationTargets(deliverableDirectory, request.TargetRefs); err != nil {
		return err
	}
	base := filepath.Base(strings.TrimSpace(command))
	switch base {
	case "go":
		return validateGoTestValidationPreflight(deliverableDirectory, args)
	default:
		return nil
	}
}

func validateDeclaredValidationTargets(deliverableDirectory string, targetRefs []string) error {
	if err := validateDeclaredValidationTargetScope(deliverableDirectory, targetRefs); err != nil {
		return err
	}
	root, err := filepath.Abs(strings.TrimSpace(deliverableDirectory))
	if err != nil || root == "" {
		return common.ErrInvalidArgument
	}
	for _, targetRef := range normalizeShellStrings(targetRefs) {
		target := filepath.Join(root, filepath.FromSlash(targetRef))
		if _, statErr := os.Stat(target); statErr != nil {
			return fmt.Errorf("%w: validation target %q does not exist", common.ErrInvalidArgument, targetRef)
		}
	}
	return nil
}

// validateDeclaredValidationTargetScope checks only the owner boundary. A
// target may legitimately be absent while File work that creates it is still
// pending; the Shell prerequisite observation records that absence and is
// invalidated by the later File receipt. Feasibility calls the stricter
// validateDeclaredValidationTargets immediately before semantic seal.
func validateDeclaredValidationTargetScope(deliverableDirectory string, targetRefs []string) error {
	root, err := filepath.Abs(strings.TrimSpace(deliverableDirectory))
	if err != nil || root == "" {
		return common.ErrInvalidArgument
	}
	for _, targetRef := range normalizeShellStrings(targetRefs) {
		if filepath.IsAbs(targetRef) {
			return fmt.Errorf("%w: validation target %q must be workspace-relative", common.ErrInvalidArgument, targetRef)
		}
		target := filepath.Join(root, filepath.FromSlash(targetRef))
		rel, relErr := filepath.Rel(root, target)
		if relErr != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return fmt.Errorf("%w: validation target %q escapes workspace", common.ErrInvalidArgument, targetRef)
		}
	}
	return nil
}

func shellRequestLooksLikeValidation(request RuntimeCommandRequest, command string, args []string) bool {
	if !runtimeValidationEvidenceEmpty(request.ValidationEvidence) {
		return true
	}
	// Heuristic for validation-shaped go test invocations used as verification evidence.
	if filepath.Base(strings.TrimSpace(command)) == "go" && len(args) > 0 && strings.TrimSpace(args[0]) == "test" {
		return true
	}
	return false
}

func validateGoTestValidationPreflight(deliverableDirectory string, args []string) error {
	if len(args) == 0 || strings.TrimSpace(args[0]) != "test" {
		return nil
	}
	hasGoModule, err := workspaceContainsGoModule(deliverableDirectory)
	if err != nil {
		return err
	}
	if !hasGoModule {
		return fmt.Errorf("%w: validation preflight failed: root go.mod not found for isolated go test workspace", common.ErrInvalidArgument)
	}
	if err := validateGoTestPackageTargets(deliverableDirectory, args); err != nil {
		return err
	}
	runPattern := goTestRunPattern(args)
	hasTestFiles, err := workspaceContainsGoTestFiles(deliverableDirectory)
	if err != nil {
		return err
	}
	if !hasTestFiles {
		if runPattern != "" {
			return fmt.Errorf("%w: validation preflight failed: no *_test.go files found for go test -run %q; create a matching test first or choose an executable validation command", common.ErrInvalidArgument, runPattern)
		}
		return fmt.Errorf("%w: validation preflight failed: no *_test.go files found for go test; create tests first or choose an executable validation command", common.ErrInvalidArgument)
	}
	return nil
}

func workspaceContainsGoModule(root string) (bool, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return false, common.ErrInvalidArgument
	}
	info, err := os.Stat(filepath.Join(root, "go.mod"))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return info.Mode().IsRegular(), nil
}

func validateGoTestPackageTargets(deliverableDirectory string, args []string) error {
	for idx := 1; idx < len(args); idx++ {
		arg := strings.TrimSpace(args[idx])
		if arg == "" {
			continue
		}
		if arg == "-run" || arg == "--run" {
			idx++
			continue
		}
		if strings.HasPrefix(arg, "-") {
			continue
		}
		// Go package patterns not rooted in the workspace are resolved by the Go
		// toolchain. Workspace-relative targets are deterministic and can be
		// rejected before process start when they do not exist.
		if arg != "." && !strings.HasPrefix(arg, "./") {
			continue
		}
		if arg == "./..." {
			continue
		}
		rel := strings.TrimPrefix(arg, "./")
		// Go's recursive package pattern addresses the directory before the
		// trailing /..., not a literal child named "...". Preflight only proves
		// that workspace-relative roots exist; the Go toolchain remains the
		// authority for package discovery below that root.
		if wildcard := strings.Index(rel, "..."); wildcard >= 0 {
			rel = strings.TrimSuffix(rel[:wildcard], "/")
			if rel == "" {
				continue
			}
		}
		path := filepath.Join(deliverableDirectory, filepath.FromSlash(rel))
		info, err := os.Stat(path)
		if err != nil || !info.IsDir() {
			if err == nil {
				err = common.ErrNotFound
			}
			return fmt.Errorf("%w: validation preflight failed: go test package target %q does not exist", err, arg)
		}
	}
	return nil
}

func goTestRunPattern(args []string) string {
	for idx := 0; idx < len(args); idx++ {
		arg := strings.TrimSpace(args[idx])
		switch {
		case arg == "-run" || arg == "--run":
			if idx+1 < len(args) {
				return strings.TrimSpace(args[idx+1])
			}
		case strings.HasPrefix(arg, "-run="):
			return strings.TrimSpace(strings.TrimPrefix(arg, "-run="))
		case strings.HasPrefix(arg, "--run="):
			return strings.TrimSpace(strings.TrimPrefix(arg, "--run="))
		}
	}
	return ""
}

func workspaceContainsGoTestFiles(root string) (bool, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return false, common.ErrInvalidArgument
	}
	found := false
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			// Permission or transient path errors should not hard-fail preflight broadly.
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		name := d.Name()
		if d.IsDir() {
			if name == ".git" || name == "vendor" || name == "node_modules" || name == ".workorch-data" {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(name, "_test.go") {
			found = true
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil && !errors.Is(err, filepath.SkipAll) {
		return false, err
	}
	return found, nil
}

func runtimeValidationEvidenceEmpty(value runtimemodel.RuntimeValidationEvidence) bool {
	return strings.TrimSpace(value.Mode) == "" &&
		strings.TrimSpace(value.ObservationType) == "" &&
		strings.TrimSpace(value.Observation) == "" &&
		strings.TrimSpace(value.DiagnosticClass) == "" &&
		strings.TrimSpace(value.FailureClass) == "" &&
		strings.TrimSpace(value.ExpectedFailureClass) == "" &&
		strings.TrimSpace(value.RecommendedEvidenceTargetType) == "" &&
		len(value.TargetRefs) == 0 && len(value.CoverageTargets) == 0 && len(value.ObservedClaims) == 0
}
