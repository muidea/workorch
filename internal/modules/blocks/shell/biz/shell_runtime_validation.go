package shell

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
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
func shellValidationSurfaces(declared, staticallyReady, ready []runtimemodel.RuntimeValidationRecipe, binding runtimemodel.RuntimeWorkspaceManifestBinding, catalogObservedRevision string) []runtimemodel.RuntimeValidationSurface {
	binding = binding.Normalized()
	catalogObservedRevision = strings.TrimSpace(catalogObservedRevision)
	staticIDs := shellValidationRecipeIDs(staticallyReady)
	readyIDs := shellValidationRecipeIDs(ready)
	ret := make([]runtimemodel.RuntimeValidationSurface, 0, len(declared))
	for _, recipe := range runtimemodel.NormalizeRuntimeValidationRecipes(declared) {
		schemaHash := shellValidationRecipeSchemaHash(recipe)
		surface := runtimemodel.RuntimeValidationSurface{
			ID: recipe.ID, OwnerModuleID: ModuleID, OperationKind: runtimemodel.RuntimeBlockKindManagedValidationSession,
			CheckRefs: []string{shellValidationRecipeProofCheckRef(recipe.ID)}, ProofApplicability: runtimemodel.RuntimeValidationProofDirect, EffectScope: runtimemodel.RuntimeEffectEphemeralSession,
			IdempotencyClass: runtimemodel.RuntimeOperationIdempotencyAtMostOnce,
			InputSchemaHash:  schemaHash,
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

// shellProspectiveTargetSatisfactionConstraint publishes only predicates for
// which one target satisfying every conjunct is sufficient to make the exact
// activation-sealed recipe applicable to the resulting manifest. Recipes
// whose missing package roots require multiple incomparable targets remain a
// non-actionable prerequisite; Shell never publishes an implicit any-of.
func shellProspectiveTargetSatisfactionConstraint(recipe runtimemodel.RuntimeValidationRecipe, currentTargetRefs []string, catalogObservedRevision string) (runtimemodel.RuntimeProspectiveTargetSatisfactionConstraint, bool) {
	recipe = recipe.Normalized()
	catalogObservedRevision = strings.TrimSpace(catalogObservedRevision)
	isGoTest, packageRoots, ok := shellGoTestManifestRequirements(recipe)
	if !ok || !isGoTest || catalogObservedRevision == "" {
		return runtimemodel.RuntimeProspectiveTargetSatisfactionConstraint{}, false
	}
	currentTargetRefs = normalizeShellStrings(currentTargetRefs)
	predicates := make([]runtimemodel.RuntimeProspectiveTargetPredicate, 0, 6)
	if !shellManifestHasApplicableGoTestTarget(currentTargetRefs) {
		predicates = append(predicates, runtimemodel.RuntimeProspectiveTargetPredicate{
			Kind: runtimemodel.RuntimeProspectiveTargetPredicatePathSuffix, Value: "_test.go",
		})
		for _, segment := range shellManifestExcludedPathSegments() {
			predicates = append(predicates, runtimemodel.RuntimeProspectiveTargetPredicate{
				Kind: runtimemodel.RuntimeProspectiveTargetPredicatePathSegmentNotEqual, Value: segment,
			})
		}
	}
	missingRoots := make([]string, 0, len(packageRoots))
	for _, root := range packageRoots {
		if !shellManifestContainsPackageRoot(currentTargetRefs, root) {
			missingRoots = append(missingRoots, root)
		}
	}
	if prefix, expressible := shellSingleProspectivePackagePrefix(missingRoots); !expressible {
		return runtimemodel.RuntimeProspectiveTargetSatisfactionConstraint{}, false
	} else if prefix != "" {
		predicates = append(predicates, runtimemodel.RuntimeProspectiveTargetPredicate{
			Kind: runtimemodel.RuntimeProspectiveTargetPredicatePathPrefix, Value: prefix,
		})
	}
	if len(predicates) == 0 {
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
		AllOf:                              predicates,
	}.Normalized()
	if err := value.Validate(); err != nil {
		return runtimemodel.RuntimeProspectiveTargetSatisfactionConstraint{}, false
	}
	return value, true
}

// shellValidationRecipeProofCheckRef is the exact owner-published aggregate
// check for one recipe execution with real, non-conflicting work. Tool-native
// checks remain in the observation for audit, while semantic binding consumes
// this bounded identity instead of incorrectly treating RecipeID as a check.
func shellValidationRecipeProofCheckRef(recipeID string) string {
	recipeID = strings.TrimSpace(recipeID)
	if recipeID == "" {
		return ""
	}
	digest := sha256.Sum256([]byte("shell-managed-validation-check\x00" + recipeID))
	return "shell-validation-check-" + hex.EncodeToString(digest[:])[:20]
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

func shellManifestContainsPackageRoot(targetRefs []string, root string) bool {
	prefix := strings.Trim(strings.TrimSpace(root), "/") + "/"
	for _, ref := range normalizeShellStrings(targetRefs) {
		if strings.HasPrefix(filepath.ToSlash(filepath.Clean(ref)), prefix) {
			return true
		}
	}
	return false
}

func shellSingleProspectivePackagePrefix(roots []string) (string, bool) {
	roots = normalizeShellStrings(roots)
	if len(roots) == 0 {
		return "", true
	}
	longest := roots[0]
	for _, root := range roots[1:] {
		if len(root) > len(longest) {
			longest = root
		}
	}
	for _, root := range roots {
		if longest != root && !strings.HasPrefix(longest, strings.TrimSuffix(root, "/")+"/") {
			return "", false
		}
	}
	return strings.Trim(longest, "/") + "/", true
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
