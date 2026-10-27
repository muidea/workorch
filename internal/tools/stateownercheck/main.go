// Command stateownercheck verifies the repository's exhaustive, verification-only
// inventory of state-like string types and explicitly audited bare state-like
// fields in owner models. It is intentionally outside production wiring:
// Runtime reducers must never consume this inventory as transition data.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type declaration struct {
	File       string
	PackageDir string
	Name       string
	Kind       declarationKind
}

type declarationKind string

const (
	declarationNamedStateType declarationKind = "named_state_type"
	declarationBareStateField declarationKind = "bare_state_field"
)

type inventoryEntry struct {
	Declaration    declaration
	Classification string
	OwnerRoot      string
	TransitionIDs  []string
	Line           int
}

type transitionOwner struct {
	Classification    string
	OwnerRoot         string
	InternalEventFile string
	DispatcherFile    string
	DispatcherSymbol  string
	InternalEventType string
	TransitionSinks   []string
	Line              int
}

type privilegedSinkReference struct {
	File     string
	Function string
}

var transitionClassificationsRequiringStateType = map[string]bool{
	"formal_runtime_machine":       true,
	"resource_state_machine":       true,
	"derived_projection_lifecycle": true,
}

// uniqueOwnerPrivateTransitionSinks are irreversible or privileged physical
// effects that must have exactly one production call site inside their
// declared owner-private dispatcher. The transition manifest still declares
// the complete reducer/persistence path; this narrower inventory prevents a
// second call site from reusing the same capability as a hidden state-machine
// ingress.
var uniqueOwnerPrivateTransitionSinks = map[string][]string{
	"workspace.deletion":              {"CleanupDeletionMaterial"},
	"run.workspace_retirement":        {"PurgeWorkspaceResources"},
	"orchestrator.workspace_deletion": {"PurgeWorkspaceEvidence"},
	"memory.workspace_deletion":       {"WithWorkspacePurgeFence"},
	"result.workspace_deletion":       {"WithWorkspacePurgeFence"},
	"decision.workspace_deletion":     {"WithWorkspacePurgeFence"},
	"approval.workspace_deletion":     {"WithWorkspacePurgeFence"},
	"eventstream.workspace_deletion":  {"PurgeWorkspaceEventRecords"},
}

// privilegedSinkRepositoryDelegations are the exact storage-layer adapters
// that may forward an already owner-authorized physical operation. They do not
// choose a business transition. Every other production reference must be the
// private dispatcher derived from the transition manifest.
var privilegedSinkRepositoryDelegations = map[string][]privilegedSinkReference{
	"PurgeWorkspaceEventRecords": {
		{File: "internal/pkg/workorchrepo/workspace_scoped_repositories.go", Function: "purgeWorkspaceEventRecordsPhysical"},
	},
	"WithWorkspacePurgeFence": {
		{File: "internal/pkg/workorchrepo/workspace_scoped_repositories.go", Function: "PurgeWorkspaceEvidence"},
		{File: "internal/pkg/repository/memory/orchestrator_workspace_deletion.go", Function: "PurgeWorkspaceEvidence"},
		{File: "internal/pkg/repository/memory/memory.go", Function: "PurgeWorkspaceEventRecords"},
		{File: "internal/pkg/repository/file/file.go", Function: "PurgeWorkspaceEventRecords"},
	},
}

var stateLikeSuffixes = []string{"State", "Status", "Phase", "Disposition", "Outcome", "Mode", "Lifecycle", "Stage"}

// targetStatePrefixes name caller-selected transition outputs that must never
// cross an owner boundary, including non-State vocabularies such as Status,
// Phase, Mode, or Stage.
var targetStatePrefixes = []string{"Next", "Target", "Desired", "Initial", "Transition", "Requested", "Proposed", "New", "Final", "Resulting", "Destination", "Selected", "Wanted"}

var exactToStateFields = map[string]bool{
	"ToState":       true,
	"ToStatus":      true,
	"ToPhase":       true,
	"ToDisposition": true,
	"ToOutcome":     true,
	"ToMode":        true,
	"ToLifecycle":   true,
	"ToStage":       true,
}

// observedStateCarrierTypes records repository-local immutable event facts
// whose plain State field is an observation produced by that fact's owner. It
// is deliberately exact: generic Data/*Data names are not authority-bearing
// evidence and must not become an escape hatch for nested transition intent.
var observedStateCarrierTypes = map[string]bool{
	"internal/modules/blocks/approval/pkg/events:ApprovalChangedData":       true,
	"internal/modules/blocks/eventstream/pkg/events:ApprovalResolutionFact": true,
	"internal/pkg/workorchdeletioncontract:WorkspaceRuntimeQuiescenceToken": true,
	"internal/pkg/workorchruntimecontract:AuthorityTransaction":             true,
	"internal/pkg/workorchruntimecontract:RuntimeCandidateContractManifest": true,
	"internal/pkg/workorchruntimecontract:RuntimeDecision":                  true,
}

var observedStateCarrierEdges = map[string]bool{
	"internal/modules/blocks/run/pkg/events:StoreRuntimeStateRecordCommand|RuntimeState|internal/pkg/workorchruntimecontract:RuntimeStateRecord": true,
}

var observedCommandStateFields = map[string]bool{
	"internal/modules/kernel/orchestrator/pkg/events:RecordApprovalEvidenceCommand|ApprovalState":              true,
	"internal/modules/blocks/memory/pkg/events:UpsertMemoryRecallTraceCommand|Stage":                           true,
	"internal/modules/blocks/memory/pkg/events:UpsertMemoryRecallTraceCommand|Status":                          true,
	"internal/modules/kernel/orchestrator/pkg/events:CommitRuntimeGovernanceApprovalResolutionCommand|Outcome": true,
	"internal/modules/kernel/orchestrator/pkg/events:CommitRuntimeGovernanceTicketResolutionCommand|Outcome":   true,
}

// An observed field in a shared owner model is allowed only for this exact
// command/model/field triple, never as a model-wide grant. The settlement stores
// the original Run retirement observation and cannot choose a retirement phase.
var observedCommandCarrierFields = map[string]bool{
	"internal/modules/kernel/workspace/pkg/events:SettleWorkspacePreparationBlockedCommand|internal/modules/blocks/run/pkg/models:WorkspaceRetirementFence|Phase": true,
}

// observedStateCarrierFields is narrower than a carrier-type grant. Use it
// when only one immutable observation inside a shared DTO is state-like; a
// newly added sibling State field then fails closed until separately audited.
var observedStateCarrierFields = map[string]bool{
	"internal/modules/kernel/workspace/pkg/models:DefinitionSpecificationRequestIdentity|Phase": true,
	// 前置诊断只记录 reserve 前的失败位置，Stage 不选择 owner transition。
	"internal/modules/kernel/workspace/pkg/models:DefinitionAuthoringPreparationDiagnostic|Stage": true,
	// Fixed workflow route observed from the exact Workspace configuration;
	// not a requested preparation/Runtime transition or an owner target state.
	"internal/modules/kernel/workspace/pkg/models:PreparationSoulSelection|Stage": true,
	// LLM-owned cache classification at one observation point, never a requested
	// Workspace transition. Only this field is observed; sibling authority fails.
	"internal/modules/blocks/llm/pkg/models:ProviderModelCatalog|State": true,
	// Specification critiques and declaration matches are immutable authoring
	// facts. Workspace Compile, not either Status field, selects admission.
	"internal/modules/kernel/workspace/pkg/models:GoalSpecificationCompletenessReview|Status": true,
	"internal/modules/kernel/workspace/pkg/models:GoalSpecificationVerification|Status":       true,
	// 当前 mapping 的 Status 只是 provider 验证声明；owner 重编译与准入仍独立裁决。
	"internal/modules/kernel/workspace/pkg/models:DefinitionAuthoringVerificationMapping|Status":            true,
	"internal/modules/blocks/llm/pkg/models:ProviderAdapterCacheHint|Mode":                                  true,
	"internal/modules/blocks/llm/pkg/models:ProviderContextBudget|ProfileMode":                              true,
	"internal/modules/blocks/llm/pkg/models:ProviderContextProfile|ProfileMode":                             true,
	"internal/modules/blocks/llm/pkg/models:ProviderObservedRequestShape|ProviderFacingMode":                true,
	"internal/modules/blocks/mcpregistry/pkg/events:CapabilityQuery|Stage":                                  true,
	"internal/pkg/workorchdefinitioncontract:WorkspaceDefinitionClarificationResolutionBinding|Disposition": true,
	"internal/pkg/workorchdefinitioncontract:WorkspaceDefinitionClarificationTicketBinding|Disposition":     true,
	"internal/pkg/soulbinding:Binding|Stage":                                                                true,
	"internal/pkg/workorchmemorycontract:RecallSelectionEvidence|Status":                                    true,
	"internal/pkg/workorchquerycontract:AutoGovernanceTraceQuery|Status":                                    true,
	"internal/pkg/workorchquerycontract:MemoryRecallTraceQuery|Stage":                                       true,
	"internal/pkg/workorchquerycontract:MemoryRecallTraceQuery|Status":                                      true,
	"internal/pkg/workorchruntimecontract:RuntimeAssessmentFact|ClaimedDisposition":                         true,
	"internal/pkg/workorchruntimecontract:RuntimeAssessmentFact|Disposition":                                true,
	"internal/pkg/workorchruntimecontract:RuntimeBudgetDimensionPolicy|ExhaustionDisposition":               true,
	"internal/pkg/workorchruntimecontract:RuntimeExecutionBudgetPolicy|RestartDisposition":                  true,
	"internal/pkg/workorchruntimecontract:RuntimeCapabilityStateSnapshot|Disposition":                       true,
	"internal/pkg/workorchruntimecontract:RuntimeCandidateContext|RequiredState":                            true,
	"internal/pkg/workorchruntimecontract:RuntimeCapabilityUsagePolicy|UsageMode":                           true,
	"internal/pkg/workorchruntimecontract:RuntimeGovernanceResolution|Outcome":                              true,
	"internal/pkg/workorchruntimecontract:RuntimeMCPCapabilityContext|Status":                               true,
	"internal/pkg/workorchruntimecontract:RuntimeOperationReceipt|Outcome":                                  true,
	"internal/pkg/workorchruntimecontract:RuntimeRepairDirective|RequiredState":                             true,
	"internal/pkg/workorchruntimecontract:RuntimeRequiredOperationReceiptBinding|Outcome":                   true,
	"internal/pkg/workorchruntimecontract:RuntimeStageSoulBinding|Stage":                                    true,
	"internal/pkg/workorchruntimecontract:RuntimeTargetSelection|Disposition":                               true,
	"internal/pkg/workorchruntimecontract:RuntimeTemplateStageGovernance|ApprovalMode":                      true,
	"internal/pkg/workorchruntimecontract:RuntimeTemplateStageGovernance|DecisionMode":                      true,
	"internal/pkg/workorchruntimecontract:RuntimeValidationEvidence|Mode":                                   true,
	"internal/pkg/workorchruntimecontract:TargetSnapshot|ContentState":                                      true,
	"internal/pkg/workorchruntimecontract:ValidationFailureDiagnostic|EvidenceMode":                         true,
	"internal/pkg/workorchruntimecontract:ValidationFailureDiagnostic|Stage":                                true,
}

var classifications = map[string]bool{
	"authoritative_state_machine": true,
	"owner_private_state":         true,
	"immutable_outcome":           true,
	"immutable_trace":             true,
	"pure_disposition":            true,
	"derived_projection":          true,
	"ephemeral_technical":         true,
	"invariant_value":             true,
}

// A bare state-like string field, including a nullable *string, is never
// transition authority. Owner models may retain one only when the manifest
// explicitly records that the value is a completed outcome, a derived
// projection, or immutable trace metadata. Any transition-bearing value must
// instead use a named state type.
var bareStateFieldClassifications = map[string]bool{
	"immutable_outcome":  true,
	"immutable_trace":    true,
	"derived_projection": true,
}

func main() {
	root := flag.String("root", ".", "repository root")
	inventoryPath := flag.String("inventory", "scripts/state-model-owners.manifest", "state model inventory")
	transitionPath := flag.String("transitions", "scripts/state-transition-owners.manifest", "transition owner inventory")
	flag.Parse()

	if err := verifyCanonicalRepository(*root, *inventoryPath, *transitionPath); err != nil {
		fmt.Fprintf(os.Stderr, "boundary contract violation: %v\n", err)
		os.Exit(1)
	}
}

func verify(root, inventoryPath, transitionPath string) error {
	return verifyRepository(root, inventoryPath, transitionPath, false)
}

func verifyCanonicalRepository(root, inventoryPath, transitionPath string) error {
	return verifyRepository(root, inventoryPath, transitionPath, true)
}

func verifyRepository(root, inventoryPath, transitionPath string, requirePrivilegedTransitions bool) error {
	root, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	declarations, typedConstantCounts, err := discoverDeclarations(root)
	if err != nil {
		return err
	}
	transitions, err := readTransitionOwners(filepath.Join(root, filepath.FromSlash(transitionPath)))
	if err != nil {
		return err
	}
	if requirePrivilegedTransitions {
		if err := verifyRequiredPrivilegedTransitions(transitions); err != nil {
			return err
		}
	}
	inventory, err := readInventory(filepath.Join(root, filepath.FromSlash(inventoryPath)))
	if err != nil {
		return err
	}

	for key, entry := range inventory {
		discovered, found := declarations[key]
		if !found {
			return fmt.Errorf("state-model inventory row %d references a missing declaration: %s", entry.Line, key)
		}
		if !classifications[entry.Classification] {
			return fmt.Errorf("state-model inventory row %d has unknown classification %q", entry.Line, entry.Classification)
		}
		if discovered.Kind == declarationBareStateField && !bareStateFieldClassifications[entry.Classification] {
			return fmt.Errorf("state-model inventory row %d classifies bare state-like string field %s as %q; use a named state type for transition authority or classify the field as immutable_outcome, immutable_trace, or derived_projection", entry.Line, key, entry.Classification)
		}
		transitionBearing := entry.Classification == "authoritative_state_machine" || entry.Classification == "owner_private_state"
		if transitionBearing && len(entry.TransitionIDs) == 0 {
			return fmt.Errorf("state-model inventory row %d classifies %s as transition-bearing without a transition owner", entry.Line, key)
		}
		if !transitionBearing && len(entry.TransitionIDs) != 0 {
			return fmt.Errorf("state-model inventory row %d gives non-machine %s a transition owner", entry.Line, key)
		}
		if strings.TrimSpace(entry.OwnerRoot) == "" {
			return fmt.Errorf("state-model inventory row %d omits the owning or observing component for %s", entry.Line, key)
		}
		if entry.OwnerRoot != "-" {
			info, statErr := os.Stat(filepath.Join(root, filepath.FromSlash(entry.OwnerRoot)))
			if statErr != nil || !info.IsDir() {
				return fmt.Errorf("state-model inventory row %d references a missing owner root %q", entry.Line, entry.OwnerRoot)
			}
		}
		if entry.Classification == "invariant_value" {
			constantKey := entry.Declaration.PackageDir + ":" + entry.Declaration.Name
			if typedConstantCounts[constantKey] != 1 {
				return fmt.Errorf("state-model inventory row %d classifies %s as invariant but it has %d explicitly typed constants, want exactly 1", entry.Line, key, typedConstantCounts[constantKey])
			}
		}
		// Multiple transition IDs are legal only as coordination inside the
		// state type's one owner. Each referenced transition must therefore
		// resolve back to the exact same owner root.
		for _, transitionID := range entry.TransitionIDs {
			transition, found := transitions[transitionID]
			if !found {
				return fmt.Errorf("state-model inventory row %d references unknown transition %q", entry.Line, transitionID)
			}
			if entry.OwnerRoot == "-" || transition.OwnerRoot != entry.OwnerRoot {
				return fmt.Errorf("state-model inventory row %d assigns %s to %s but transition %s is owned by %s", entry.Line, key, entry.OwnerRoot, transitionID, transition.OwnerRoot)
			}
		}
	}

	transitionStateTypes := map[string]int{}
	for _, entry := range inventory {
		for _, transitionID := range entry.TransitionIDs {
			transitionStateTypes[transitionID]++
		}
	}
	for transitionID, transition := range transitions {
		if transitionClassificationsRequiringStateType[transition.Classification] && transitionStateTypes[transitionID] == 0 {
			return fmt.Errorf("state-transition owner row %d declares %s %q without a named state type in scripts/state-model-owners.manifest", transition.Line, transition.Classification, transitionID)
		}
	}

	missing := make([]string, 0)
	for key := range declarations {
		if _, found := inventory[key]; !found {
			missing = append(missing, key)
		}
	}
	if len(missing) != 0 {
		sort.Strings(missing)
		return fmt.Errorf("state-like string declarations are absent from scripts/state-model-owners.manifest:\n%s", strings.Join(missing, "\n"))
	}
	if err := verifyTransitionOwnerRoots(root, transitions); err != nil {
		return err
	}
	if err := verifyCrossOwnerCommandContracts(root); err != nil {
		return err
	}
	if err := verifyRepositoryProviderAccess(root); err != nil {
		return err
	}
	return nil
}

type repositoryTypeDeclaration struct {
	Spec       *ast.TypeSpec
	PackageDir string
	File       string
	Imports    map[string]string
}

type repositoryTypeIndex struct {
	ModulePath   string
	Declarations map[string]repositoryTypeDeclaration
}

func verifyCrossOwnerCommandContracts(root string) error {
	index, err := loadRepositoryTypeIndex(root)
	if err != nil {
		return err
	}
	keys := make([]string, 0)
	for key, declaration := range index.Declarations {
		name := declaration.Spec.Name.Name
		if isEventsPackageDir(declaration.PackageDir) && ast.IsExported(name) && strings.HasSuffix(name, "Command") {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	violations := make([]string, 0)
	for _, key := range keys {
		declaration := index.Declarations[key]
		name := declaration.Spec.Name.Name
		field, unresolved := targetStateFieldInRepositoryType(declaration.Spec.Type, declaration, index, key, map[string]bool{key + "|authority": true}, false)
		if unresolved != "" {
			violations = append(violations, fmt.Sprintf("cross-owner command %s in %s references unaudited repository type %s; use explicit intent or immutable fact fields", name, declaration.File, unresolved))
			continue
		}
		if field != "" {
			violations = append(violations, fmt.Sprintf("cross-owner command %s in %s exposes target-state field %s; publish intent or immutable fact and let the owner reducer choose the transition", name, declaration.File, field))
		}
	}
	if len(violations) != 0 {
		return fmt.Errorf("%s", strings.Join(violations, "\n"))
	}
	return nil
}

// verifyRepositoryProviderAccess keeps repository-family authorization at the
// single workorchdeps resolver. The initiator owns the provider implementation,
// but owner/application packages must never obtain it directly and select a
// sibling repository family without the typed consumer check.
func verifyRepositoryProviderAccess(root string) error {
	allowedRoots := []string{
		"internal/initiators/workorchruntime",
		"internal/pkg/workorchdeps",
	}
	violations := make([]string, 0)
	for _, top := range []string{"application", "internal"} {
		base := filepath.Join(root, top)
		err := filepath.WalkDir(base, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				if entry.Name() == "vendor" || entry.Name() == "testdata" {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
				return nil
			}
			relative, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			relative = filepath.ToSlash(relative)
			allowed := false
			for _, allowedRoot := range allowedRoots {
				if relative == allowedRoot || strings.HasPrefix(relative, allowedRoot+"/") {
					allowed = true
					break
				}
			}
			if allowed {
				return nil
			}
			fset := token.NewFileSet()
			parsed, err := parser.ParseFile(fset, path, nil, 0)
			if err != nil {
				return fmt.Errorf("parse repository-provider consumer %s: %w", relative, err)
			}
			ast.Inspect(parsed, func(node ast.Node) bool {
				selector, ok := node.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				if selector.Sel.Name != "RepositoryProvider" {
					return true
				}
				position := fset.Position(selector.Pos())
				violations = append(violations, fmt.Sprintf("%s:%d", relative, position.Line))
				return true
			})
			return nil
		})
		if err != nil {
			return err
		}
	}
	if len(violations) != 0 {
		sort.Strings(violations)
		return fmt.Errorf("production code bypasses typed workorchdeps repository-family authorization through RepositoryProvider(): %s", strings.Join(violations, ", "))
	}
	return nil
}

func loadRepositoryTypeIndex(root string) (repositoryTypeIndex, error) {
	modulePath, err := repositoryModulePath(filepath.Join(root, "go.mod"))
	if err != nil {
		return repositoryTypeIndex{}, err
	}
	index := repositoryTypeIndex{ModulePath: modulePath, Declarations: map[string]repositoryTypeDeclaration{}}
	for _, top := range []string{"application", "internal"} {
		base := filepath.Join(root, top)
		err := filepath.WalkDir(base, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				if entry.Name() == "vendor" || entry.Name() == "testdata" {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
				return nil
			}
			parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
			if err != nil {
				return fmt.Errorf("parse repository Go file %s: %w", path, err)
			}
			relative, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			relative = filepath.ToSlash(relative)
			packageDir := filepath.ToSlash(filepath.Dir(relative))
			imports, forbiddenImports := repositoryLocalImports(parsed, modulePath)
			if isEventsPackageDir(packageDir) && len(forbiddenImports) != 0 {
				return fmt.Errorf("production events file %s uses repository-local dot/blank import %s; event DTO provenance must remain explicit", relative, strings.Join(forbiddenImports, ", "))
			}
			for _, node := range parsed.Decls {
				general, ok := node.(*ast.GenDecl)
				if !ok || general.Tok != token.TYPE {
					continue
				}
				for _, specification := range general.Specs {
					typeSpec, ok := specification.(*ast.TypeSpec)
					if !ok {
						continue
					}
					key := repositoryTypeKey(packageDir, typeSpec.Name.Name)
					if _, duplicate := index.Declarations[key]; duplicate {
						return fmt.Errorf("duplicate repository type declaration %s", key)
					}
					index.Declarations[key] = repositoryTypeDeclaration{Spec: typeSpec, PackageDir: packageDir, File: relative, Imports: imports}
				}
			}
			return nil
		})
		if err != nil {
			return repositoryTypeIndex{}, err
		}
	}
	return index, nil
}

func repositoryModulePath(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read go.mod: %w", err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == "module" && fields[1] != "" {
			return fields[1], nil
		}
	}
	return "", fmt.Errorf("go.mod has no module path")
}

func repositoryLocalImports(parsed *ast.File, modulePath string) (map[string]string, []string) {
	ret := map[string]string{}
	forbidden := make([]string, 0)
	for _, item := range parsed.Imports {
		path, err := strconv.Unquote(item.Path.Value)
		if err != nil || !strings.HasPrefix(path, modulePath+"/") {
			continue
		}
		alias := filepath.Base(path)
		if item.Name != nil {
			alias = item.Name.Name
		}
		if alias == "_" || alias == "." {
			forbidden = append(forbidden, alias+" "+path)
			continue
		}
		if alias == "" {
			continue
		}
		ret[alias] = filepath.ToSlash(strings.TrimPrefix(path, modulePath+"/"))
	}
	sort.Strings(forbidden)
	return ret, forbidden
}

func repositoryTypeKey(packageDir, name string) string {
	return filepath.ToSlash(packageDir) + ":" + name
}

// targetStateFieldInRepositoryType follows local and repository-qualified
// request/intent DTOs. Immutable fact carriers may contain a plain observed
// State, but are still traversed so NextState/TargetState-style authority can
// never be hidden behind Fact, Result, Data, or another benign-looking name.
func targetStateFieldInRepositoryType(expression ast.Expr, context repositoryTypeDeclaration, index repositoryTypeIndex, commandKey string, seen map[string]bool, allowObservedState bool) (field, unresolved string) {
	switch value := expression.(type) {
	case *ast.StructType:
		for _, item := range value.Fields.List {
			childAllowsObservedState := allowObservedState || isObservedStateCarrier(commandKey, item.Names, item.Type, context, index)
			for _, name := range item.Names {
				fieldAllowsObservedState := childAllowsObservedState || isExplicitObservedStateField(commandKey, context, name.Name)
				if isForbiddenCrossOwnerStateField(name.Name, fieldAllowsObservedState) {
					return name.Name, ""
				}
			}
			if field, unresolved := targetStateFieldInRepositoryType(item.Type, context, index, commandKey, seen, childAllowsObservedState); field != "" || unresolved != "" {
				if field != "" {
					label := namedTypeName(item.Type)
					if len(item.Names) != 0 {
						label = item.Names[0].Name
					}
					if label != "" {
						field = label + "." + field
					}
				}
				return field, unresolved
			}
		}
	case *ast.Ident, *ast.SelectorExpr:
		declaration, key, repositoryLocal := resolveRepositoryType(value, context, index)
		if declaration == nil {
			if repositoryLocal {
				return "", key
			}
			return "", ""
		}
		visitKey := key + "|authority"
		if allowObservedState {
			visitKey = key + "|observed"
		}
		if seen[visitKey] {
			return "", ""
		}
		seen[visitKey] = true
		return targetStateFieldInRepositoryType(declaration.Spec.Type, *declaration, index, commandKey, seen, allowObservedState)
	case *ast.StarExpr:
		return targetStateFieldInRepositoryType(value.X, context, index, commandKey, seen, allowObservedState)
	case *ast.ArrayType:
		return targetStateFieldInRepositoryType(value.Elt, context, index, commandKey, seen, allowObservedState)
	case *ast.MapType:
		if field, unresolved := targetStateFieldInRepositoryType(value.Key, context, index, commandKey, seen, allowObservedState); field != "" || unresolved != "" {
			return field, unresolved
		}
		return targetStateFieldInRepositoryType(value.Value, context, index, commandKey, seen, allowObservedState)
	case *ast.IndexExpr:
		if field, unresolved := targetStateFieldInRepositoryType(value.X, context, index, commandKey, seen, allowObservedState); field != "" || unresolved != "" {
			return field, unresolved
		}
		return targetStateFieldInRepositoryType(value.Index, context, index, commandKey, seen, allowObservedState)
	case *ast.IndexListExpr:
		if field, unresolved := targetStateFieldInRepositoryType(value.X, context, index, commandKey, seen, allowObservedState); field != "" || unresolved != "" {
			return field, unresolved
		}
		for _, item := range value.Indices {
			if field, unresolved := targetStateFieldInRepositoryType(item, context, index, commandKey, seen, allowObservedState); field != "" || unresolved != "" {
				return field, unresolved
			}
		}
		return "", ""
	case *ast.ParenExpr:
		return targetStateFieldInRepositoryType(value.X, context, index, commandKey, seen, allowObservedState)
	case *ast.ChanType:
		return targetStateFieldInRepositoryType(value.Value, context, index, commandKey, seen, allowObservedState)
	}
	return "", ""
}

func resolveRepositoryType(expression ast.Expr, context repositoryTypeDeclaration, index repositoryTypeIndex) (*repositoryTypeDeclaration, string, bool) {
	switch value := expression.(type) {
	case *ast.Ident:
		key := repositoryTypeKey(context.PackageDir, value.Name)
		declaration, found := index.Declarations[key]
		if !found {
			return nil, key, false
		}
		return &declaration, key, true
	case *ast.SelectorExpr:
		alias, ok := value.X.(*ast.Ident)
		if !ok {
			return nil, namedTypeName(value), false
		}
		packageDir, local := context.Imports[alias.Name]
		if !local {
			return nil, alias.Name + "." + value.Sel.Name, false
		}
		key := repositoryTypeKey(packageDir, value.Sel.Name)
		declaration, found := index.Declarations[key]
		if !found {
			return nil, key, true
		}
		return &declaration, key, true
	case *ast.StarExpr:
		return resolveRepositoryType(value.X, context, index)
	case *ast.ArrayType:
		return resolveRepositoryType(value.Elt, context, index)
	case *ast.MapType:
		return resolveRepositoryType(value.Value, context, index)
	case *ast.IndexExpr:
		return resolveRepositoryType(value.X, context, index)
	case *ast.IndexListExpr:
		return resolveRepositoryType(value.X, context, index)
	case *ast.ParenExpr:
		return resolveRepositoryType(value.X, context, index)
	default:
		return nil, fmt.Sprintf("%T", expression), false
	}
}

func isObservedStateCarrier(commandKey string, fieldNames []*ast.Ident, expression ast.Expr, context repositoryTypeDeclaration, index repositoryTypeIndex) bool {
	declaration, key, _ := resolveRepositoryType(expression, context, index)
	if declaration == nil {
		return false
	}
	if observedStateCarrierTypes[key] {
		return true
	}
	for _, fieldName := range fieldNames {
		if observedStateCarrierEdges[commandKey+"|"+fieldName.Name+"|"+key] {
			return true
		}
	}
	return false
}

func isExplicitObservedStateField(commandKey string, context repositoryTypeDeclaration, fieldName string) bool {
	if observedCommandStateFields[commandKey+"|"+fieldName] {
		return true
	}
	contextKey := repositoryTypeKey(context.PackageDir, context.Spec.Name.Name)
	return observedStateCarrierFields[contextKey+"|"+fieldName] || observedCommandCarrierFields[commandKey+"|"+contextKey+"|"+fieldName]
}

func namedTypeName(expression ast.Expr) string {
	switch value := expression.(type) {
	case *ast.Ident:
		return value.Name
	case *ast.SelectorExpr:
		return value.Sel.Name
	case *ast.StarExpr:
		return namedTypeName(value.X)
	case *ast.IndexExpr:
		return namedTypeName(value.X)
	case *ast.IndexListExpr:
		return namedTypeName(value.X)
	case *ast.ParenExpr:
		return namedTypeName(value.X)
	default:
		return ""
	}
}

func isEventsPackageDir(packageDir string) bool {
	return strings.Contains("/"+strings.Trim(filepath.ToSlash(packageDir), "/")+"/", "/pkg/events/")
}

func isTargetStateField(name string) bool {
	return isForbiddenCrossOwnerStateField(name, false)
}

func isForbiddenCrossOwnerStateField(name string, observedCarrier bool) bool {
	if !isStateLikeName(name) {
		return false
	}
	if exactToStateFields[name] {
		return true
	}
	for _, prefix := range targetStatePrefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	if observedCarrier {
		return false
	}
	switch name {
	case "CurrentState", "ObservedState", "PreviousState":
		return false
	default:
		return true
	}
}

// verifyTransitionOwnerRoots anchors every declared mutation entrypoint to its
// unique owner. This deliberately verifies manifest paths and the dispatcher's
// AST declaration instead of guessing at arbitrary State field assignments,
// which are also used legitimately by immutable facts and projections.
func verifyRequiredPrivilegedTransitions(transitions map[string]transitionOwner) error {
	missing := make([]string, 0)
	for transitionID := range uniqueOwnerPrivateTransitionSinks {
		if _, found := transitions[transitionID]; !found {
			missing = append(missing, transitionID)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	sort.Strings(missing)
	return fmt.Errorf("state-transition owner inventory omits required privileged transitions: %s", strings.Join(missing, ", "))
}

func verifyTransitionOwnerRoots(root string, transitions map[string]transitionOwner) error {
	identities := make([]string, 0, len(transitions))
	for identity := range transitions {
		identities = append(identities, identity)
	}
	sort.Strings(identities)
	for _, identity := range identities {
		transition := transitions[identity]
		ownerRoot, err := ownerDirectory(root, transition.OwnerRoot)
		if err != nil {
			return fmt.Errorf("state-transition owner row %d declares invalid owner root for %s: %w", transition.Line, identity, err)
		}
		internalEventPath, err := ownerFile(root, ownerRoot, transition.OwnerRoot, transition.InternalEventFile)
		if err != nil {
			return fmt.Errorf("state-transition owner row %d declares invalid internal event file for %s: %w", transition.Line, identity, err)
		}
		dispatcherPath, err := ownerFile(root, ownerRoot, transition.OwnerRoot, transition.DispatcherFile)
		if err != nil {
			return fmt.Errorf("state-transition owner row %d declares invalid dispatcher file for %s: %w", transition.Line, identity, err)
		}
		if err := verifyPrivateDispatcher(root, internalEventPath, dispatcherPath, transition.DispatcherSymbol, transition.InternalEventType, transition.TransitionSinks); err != nil {
			return fmt.Errorf("state-transition owner row %d declares invalid dispatcher for %s: %w", transition.Line, identity, err)
		}
		if uniqueSinks := uniqueOwnerPrivateTransitionSinks[identity]; len(uniqueSinks) != 0 {
			if err := verifyUniqueOwnerPrivateTransitionSinkCalls(root, ownerRoot, dispatcherPath, transition, uniqueSinks); err != nil {
				return fmt.Errorf("state-transition owner row %d violates privileged sink ownership for %s: %w", transition.Line, identity, err)
			}
		}
	}
	return verifyGlobalPrivilegedSinkReferences(root, transitions)
}

func verifyUniqueOwnerPrivateTransitionSinkCalls(root, ownerRoot, dispatcherPath string, transition transitionOwner, sinkMethods []string) error {
	wanted := map[string]bool{}
	for _, method := range sinkMethods {
		declared := false
		for _, sink := range transition.TransitionSinks {
			if selectorTerminalName(sink) == method {
				declared = true
				break
			}
		}
		if !declared {
			return fmt.Errorf("dispatcher %q does not declare privileged sink %q as an exact transition sink", transition.DispatcherSymbol, method)
		}
		wanted[method] = true
	}

	type sinkCall struct {
		location   string
		dispatcher bool
	}
	calls := map[string][]sinkCall{}
	err := filepath.WalkDir(ownerRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == "vendor" || entry.Name() == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		fset := token.NewFileSet()
		parsed, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return fmt.Errorf("parse owner production file %s: %w", path, err)
		}
		for _, node := range parsed.Decls {
			function, ok := node.(*ast.FuncDecl)
			if !ok || function.Body == nil {
				continue
			}
			ast.Inspect(function.Body, func(node ast.Node) bool {
				selector, ok := node.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				method := selector.Sel.Name
				if !wanted[method] {
					return true
				}
				position := fset.Position(selector.Pos())
				relative, relativeErr := filepath.Rel(root, position.Filename)
				if relativeErr != nil {
					relative = position.Filename
				}
				calls[method] = append(calls[method], sinkCall{
					location:   fmt.Sprintf("%s:%d:%s", filepath.ToSlash(relative), position.Line, function.Name.Name),
					dispatcher: filepath.Clean(path) == filepath.Clean(dispatcherPath) && function.Name.Name == transition.DispatcherSymbol,
				})
				return true
			})
		}
		return nil
	})
	if err != nil {
		return err
	}
	for _, method := range sinkMethods {
		methodCalls := calls[method]
		if len(methodCalls) == 1 && methodCalls[0].dispatcher {
			continue
		}
		locations := make([]string, 0, len(methodCalls))
		for _, call := range methodCalls {
			locations = append(locations, call.location)
		}
		sort.Strings(locations)
		return fmt.Errorf("privileged sink %q has %d production references under owner root, want exactly one call in dispatcher %q: %s", method, len(methodCalls), transition.DispatcherSymbol, strings.Join(locations, ", "))
	}
	return nil
}

func verifyGlobalPrivilegedSinkReferences(root string, transitions map[string]transitionOwner) error {
	allowed := map[string]map[string]bool{}
	for transitionID, sinkMethods := range uniqueOwnerPrivateTransitionSinks {
		transition, found := transitions[transitionID]
		if !found {
			continue
		}
		dispatcherFile, err := repositoryRelativeFile(root, transition.DispatcherFile)
		if err != nil {
			return err
		}
		for _, method := range sinkMethods {
			if allowed[method] == nil {
				allowed[method] = map[string]bool{}
			}
			allowed[method][dispatcherFile+"|"+transition.DispatcherSymbol] = true
		}
	}
	if len(allowed) == 0 {
		return nil
	}
	for method, references := range privilegedSinkRepositoryDelegations {
		if allowed[method] == nil {
			continue
		}
		for _, reference := range references {
			allowed[method][filepath.ToSlash(reference.File)+"|"+reference.Function] = true
		}
	}

	violations := make([]string, 0)
	for _, top := range []string{"application", "internal"} {
		base := filepath.Join(root, top)
		err := filepath.WalkDir(base, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				if entry.Name() == "vendor" || entry.Name() == "testdata" {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
				return nil
			}
			relative, err := repositoryRelativeFile(root, path)
			if err != nil {
				return err
			}
			fset := token.NewFileSet()
			parsed, err := parser.ParseFile(fset, path, nil, 0)
			if err != nil {
				return fmt.Errorf("parse privileged-sink production file %s: %w", relative, err)
			}
			ast.Inspect(parsed, func(node ast.Node) bool {
				selector, ok := node.(*ast.SelectorExpr)
				if !ok || allowed[selector.Sel.Name] == nil {
					return true
				}
				function := enclosingProductionFunction(parsed, selector.Pos())
				if allowed[selector.Sel.Name][relative+"|"+function] {
					return true
				}
				position := fset.Position(selector.Pos())
				violations = append(violations, fmt.Sprintf("%s:%d:%s:%s", relative, position.Line, function, selector.Sel.Name))
				return true
			})
			return nil
		})
		if err != nil {
			return err
		}
	}
	if len(violations) != 0 {
		sort.Strings(violations)
		return fmt.Errorf("privileged physical sink is referenced outside its owner-private dispatcher or exact repository delegation: %s", strings.Join(violations, ", "))
	}
	return nil
}

func repositoryRelativeFile(root, path string) (string, error) {
	if !filepath.IsAbs(path) {
		return filepath.ToSlash(filepath.Clean(path)), nil
	}
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return "", err
	}
	return filepath.ToSlash(relative), nil
}

func enclosingProductionFunction(parsed *ast.File, position token.Pos) string {
	for _, declarationNode := range parsed.Decls {
		function, ok := declarationNode.(*ast.FuncDecl)
		if ok && function.Pos() <= position && position <= function.End() {
			return function.Name.Name
		}
	}
	return "<package-scope>"
}

func selectorTerminalName(selector string) string {
	if position := strings.LastIndex(selector, "."); position >= 0 {
		return selector[position+1:]
	}
	return selector
}

func ownerDirectory(root, ownerRoot string) (string, error) {
	if ownerRoot == "" || ownerRoot == "-" || filepath.IsAbs(filepath.FromSlash(ownerRoot)) {
		return "", fmt.Errorf("owner root %q must be a repository-relative directory", ownerRoot)
	}
	ownerPath := filepath.Join(root, filepath.FromSlash(ownerRoot))
	info, err := os.Stat(ownerPath)
	if err != nil {
		return "", fmt.Errorf("owner root %q: %w", ownerRoot, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("owner root %q is not a directory", ownerRoot)
	}
	return ownerPath, nil
}

func ownerFile(root, ownerRoot, ownerName, declared string) (string, error) {
	if declared == "" || filepath.IsAbs(filepath.FromSlash(declared)) {
		return "", fmt.Errorf("path %q must be repository-relative", declared)
	}
	target := filepath.Join(root, filepath.FromSlash(declared))
	relative, err := filepath.Rel(ownerRoot, target)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q is outside owner root %q", declared, ownerName)
	}
	info, err := os.Stat(target)
	if err != nil {
		return "", fmt.Errorf("path %q: %w", declared, err)
	}
	if !info.Mode().IsRegular() || filepath.Ext(target) != ".go" || strings.HasSuffix(target, "_test.go") {
		return "", fmt.Errorf("path %q must be a production Go file", declared)
	}
	return target, nil
}

func verifyPrivateDispatcher(root, internalEventPath, dispatcherPath, symbol, internalEventType string, transitionSinks []string) error {
	if symbol == "" || ast.IsExported(symbol) {
		return fmt.Errorf("dispatcher symbol %q must be owner-private", symbol)
	}
	declarationCount, err := countProductionFunctionDeclarations(filepath.Dir(dispatcherPath), symbol)
	if err != nil {
		return err
	}
	if declarationCount != 1 {
		return fmt.Errorf("dispatcher symbol %q has %d production declarations in its package, want exactly one", symbol, declarationCount)
	}
	internalEventTypes, err := declaredTypeNames(internalEventPath)
	if err != nil {
		return err
	}
	if !internalEventTypes[internalEventType] {
		return fmt.Errorf("internal event type %q is not declared in its manifest file", internalEventType)
	}
	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, dispatcherPath, nil, 0)
	if err != nil {
		return fmt.Errorf("parse dispatcher file: %w", err)
	}
	modulePath, err := repositoryModulePath(filepath.Join(root, "go.mod"))
	if err != nil {
		return err
	}
	eventPackageDir, err := filepath.Rel(root, filepath.Dir(internalEventPath))
	if err != nil {
		return fmt.Errorf("resolve internal event package: %w", err)
	}
	eventImportPath := modulePath + "/" + filepath.ToSlash(eventPackageDir)
	samePackage := filepath.Clean(filepath.Dir(internalEventPath)) == filepath.Clean(filepath.Dir(dispatcherPath))
	for _, declarationNode := range parsed.Decls {
		function, ok := declarationNode.(*ast.FuncDecl)
		if ok && function.Name.Name == symbol {
			return verifyDispatcherConsumesInternalEvent(function, parsed, fset, internalEventType, eventImportPath, samePackage, transitionSinks)
		}
	}
	return fmt.Errorf("dispatcher symbol %q is not declared in its dispatcher file", symbol)
}

func countProductionFunctionDeclarations(directory, symbol string) (int, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return 0, fmt.Errorf("read dispatcher package: %w", err)
	}
	count := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(directory, entry.Name())
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return 0, fmt.Errorf("parse dispatcher package file %s: %w", path, err)
		}
		for _, node := range parsed.Decls {
			function, ok := node.(*ast.FuncDecl)
			if ok && function.Name.Name == symbol {
				count++
			}
		}
	}
	return count, nil
}

func declaredTypeNames(path string) (map[string]bool, error) {
	parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		return nil, fmt.Errorf("parse internal event file: %w", err)
	}
	ret := map[string]bool{}
	for _, declarationNode := range parsed.Decls {
		general, ok := declarationNode.(*ast.GenDecl)
		if !ok || general.Tok != token.TYPE {
			continue
		}
		for _, specification := range general.Specs {
			if typeSpec, ok := specification.(*ast.TypeSpec); ok && !typeSpec.Assign.IsValid() {
				ret[typeSpec.Name.Name] = true
			}
		}
	}
	if len(ret) == 0 {
		return nil, fmt.Errorf("internal event file declares no event type")
	}
	return ret, nil
}

var stateMutationSinkNames = map[string]bool{
	"CAS":            true,
	"Commit":         true,
	"CompareAndSwap": true,
	"Create":         true,
	"Delete":         true,
	"Save":           true,
	"Store":          true,
	"Update":         true,
	"Upsert":         true,
}

func verifyDispatcherConsumesInternalEvent(function *ast.FuncDecl, parsed *ast.File, fset *token.FileSet, internalEventType, eventImportPath string, samePackage bool, transitionSinks []string) error {
	if function.Body == nil || function.Type.Params == nil {
		return fmt.Errorf("dispatcher %q has no body or parameters", function.Name.Name)
	}
	parameterObjects := map[*ast.Object]bool{}
	for _, parameter := range function.Type.Params.List {
		if !matchesExactInternalEventType(parameter.Type, parsed, internalEventType, eventImportPath, samePackage) {
			continue
		}
		for _, name := range parameter.Names {
			if name.Obj != nil {
				parameterObjects[name.Obj] = true
			}
		}
	}
	if len(parameterObjects) == 0 {
		return fmt.Errorf("dispatcher %q does not accept the exact manifest internal event type %s", function.Name.Name, internalEventType)
	}
	allowedSinks := map[string]bool{}
	for _, sink := range transitionSinks {
		if sink == "" {
			return fmt.Errorf("dispatcher %q has an empty transition sink", function.Name.Name)
		}
		allowedSinks[sink] = true
	}
	if len(allowedSinks) == 0 {
		return fmt.Errorf("dispatcher %q declares no exact transition sink", function.Name.Name)
	}

	bindings := dispatcherBindingObjects(function)
	parents := parentNodes(function.Body)
	// Track owner-internal event data by declaration identity rather than by
	// spelling. This follows local normalization, immutable fact copies, and
	// aggregate construction without allowing a nested variable that shadows
	// `event` (or a repository receiver) to satisfy the transition proof.
	eventTainted := cloneObjectSet(parameterObjects)
	propagateObjectProvenance(function.Body, eventTainted, func(expression ast.Expr) bool {
		return expressionReferencesTaint(expression, eventTainted)
	})

	reducerSinks := map[string]bool{}
	for sink := range allowedSinks {
		if !isStateMutationCall(sink) {
			reducerSinks[sink] = true
		}
	}
	// Reducer-derived objects are a separate provenance class. A repository
	// mutation may persist a value produced by an exact private reducer, but a
	// raw internal-event fact cannot bypass that reducer merely because another
	// branch contains one valid sink call.
	sinkDerived := map[*ast.Object]bool{}
	propagateObjectProvenance(function.Body, sinkDerived, func(expression ast.Expr) bool {
		return expressionHasSinkDerivedProvenance(expression, sinkDerived, eventTainted, reducerSinks, bindings, parents)
	})

	seenSinks := map[string]bool{}
	forbiddenMutations := make([]string, 0)
	forbiddenAssignments := make([]string, 0)
	ast.Inspect(function.Body, func(node ast.Node) bool {
		switch value := node.(type) {
		case *ast.CallExpr:
			derived := callReferencesTaint(value, eventTainted) || callReferencesTaint(value, sinkDerived) || callControlledByTaint(value, parents, eventTainted)
			name := callExpressionName(value.Fun)
			declared := allowedSinks[name] && callUsesExactSinkBinding(value, name, bindings)
			if declared && derived {
				seenSinks[name] = true
			}
			if !isStateMutationCall(name) {
				return true
			}
			// Every repository-style mutation in a private dispatcher is closed
			// over the manifest, even when a guard clause or constant payload
			// hides its data/control dependence from local taint analysis.
			if !declared || !derived || (len(reducerSinks) != 0 && !mutationPayloadHasSinkDerivedProvenance(function.Body, value, sinkDerived, eventTainted, reducerSinks, bindings, parents)) {
				position := fset.Position(value.Pos())
				forbiddenMutations = append(forbiddenMutations, fmt.Sprintf("%s:%d %s", filepath.Base(position.Filename), position.Line, name))
			}
		case *ast.AssignStmt:
			for index, left := range value.Lhs {
				selector := assignedStateSelector(left)
				if selector == "" {
					continue
				}
				if !assignmentValueHasProvenance(index, len(value.Lhs), value.Rhs, func(expression ast.Expr) bool {
					return expressionHasSafeSinkDerivedProvenance(function.Body, expression, sinkDerived, eventTainted, reducerSinks, bindings, parents)
				}) {
					position := fset.Position(left.Pos())
					forbiddenAssignments = append(forbiddenAssignments, fmt.Sprintf("%s:%d %s", filepath.Base(position.Filename), position.Line, selector))
				}
			}
		}
		return true
	})
	if len(forbiddenMutations) != 0 {
		sort.Strings(forbiddenMutations)
		return fmt.Errorf("dispatcher %q performs event-derived state mutation outside its exact transition sinks: %s", function.Name.Name, strings.Join(forbiddenMutations, ", "))
	}
	if len(forbiddenAssignments) != 0 {
		sort.Strings(forbiddenAssignments)
		return fmt.Errorf("dispatcher %q assigns owner state without exact reducer-derived provenance: %s", function.Name.Name, strings.Join(forbiddenAssignments, ", "))
	}
	missingSinks := make([]string, 0)
	for sink := range allowedSinks {
		if !seenSinks[sink] {
			missingSinks = append(missingSinks, sink)
		}
	}
	if len(missingSinks) != 0 {
		sort.Strings(missingSinks)
		return fmt.Errorf("dispatcher %q does not consume its declared internal event through exact transition sinks: %s", function.Name.Name, strings.Join(missingSinks, ", "))
	}
	if len(seenSinks) == 0 {
		return fmt.Errorf("dispatcher %q does not consume its declared internal event in a non-empty transition path", function.Name.Name)
	}
	return nil
}

func matchesExactInternalEventType(expression ast.Expr, parsed *ast.File, internalEventType, eventImportPath string, samePackage bool) bool {
	switch value := expression.(type) {
	case *ast.Ident:
		return samePackage && value.Name == internalEventType
	case *ast.SelectorExpr:
		alias, ok := value.X.(*ast.Ident)
		return ok && value.Sel.Name == internalEventType && importPathForAlias(parsed, alias.Name) == eventImportPath
	case *ast.StarExpr:
		return matchesExactInternalEventType(value.X, parsed, internalEventType, eventImportPath, samePackage)
	case *ast.ParenExpr:
		return matchesExactInternalEventType(value.X, parsed, internalEventType, eventImportPath, samePackage)
	default:
		return false
	}
}

func importPathForAlias(parsed *ast.File, alias string) string {
	for _, item := range parsed.Imports {
		path, err := strconv.Unquote(item.Path.Value)
		if err != nil {
			continue
		}
		declaredAlias := filepath.Base(path)
		if item.Name != nil {
			declaredAlias = item.Name.Name
		}
		if declaredAlias == alias {
			return path
		}
	}
	return ""
}

func dispatcherBindingObjects(function *ast.FuncDecl) map[string]*ast.Object {
	ret := map[string]*ast.Object{}
	for _, fields := range []*ast.FieldList{function.Recv, function.Type.Params} {
		if fields == nil {
			continue
		}
		for _, field := range fields.List {
			for _, name := range field.Names {
				if name.Obj != nil {
					ret[name.Name] = name.Obj
				}
			}
		}
	}
	return ret
}

func callUsesExactSinkBinding(call *ast.CallExpr, sink string, bindings map[string]*ast.Object) bool {
	if callExpressionName(call.Fun) != sink {
		return false
	}
	parts := strings.Split(sink, ".")
	root := callRootIdentifier(call.Fun)
	if root == nil || root.Obj == nil {
		return false
	}
	if len(parts) == 1 {
		return root.Name == sink && root.Obj.Kind == ast.Fun
	}
	expected, found := bindings[parts[0]]
	return found && root.Name == parts[0] && root.Obj == expected
}

func callRootIdentifier(expression ast.Expr) *ast.Ident {
	switch value := expression.(type) {
	case *ast.Ident:
		return value
	case *ast.SelectorExpr:
		return callRootIdentifier(value.X)
	case *ast.IndexExpr:
		return callRootIdentifier(value.X)
	case *ast.IndexListExpr:
		return callRootIdentifier(value.X)
	case *ast.ParenExpr:
		return callRootIdentifier(value.X)
	default:
		return nil
	}
}

func cloneObjectSet(input map[*ast.Object]bool) map[*ast.Object]bool {
	ret := make(map[*ast.Object]bool, len(input))
	for object := range input {
		ret[object] = true
	}
	return ret
}

func propagateObjectProvenance(root ast.Node, objects map[*ast.Object]bool, expressionHasProvenance func(ast.Expr) bool) {
	for changed := true; changed; {
		changed = false
		ast.Inspect(root, func(node ast.Node) bool {
			switch value := node.(type) {
			case *ast.AssignStmt:
				for index, left := range value.Lhs {
					name, ok := left.(*ast.Ident)
					if !ok || name.Name == "_" || name.Obj == nil || objects[name.Obj] {
						continue
					}
					if assignmentValueHasProvenance(index, len(value.Lhs), value.Rhs, expressionHasProvenance) {
						objects[name.Obj] = true
						changed = true
					}
				}
			case *ast.ValueSpec:
				for index, name := range value.Names {
					if name.Name == "_" || name.Obj == nil || objects[name.Obj] {
						continue
					}
					if assignmentValueHasProvenance(index, len(value.Names), value.Values, expressionHasProvenance) {
						objects[name.Obj] = true
						changed = true
					}
				}
			case *ast.RangeStmt:
				if !expressionHasProvenance(value.X) {
					break
				}
				for _, target := range []ast.Expr{value.Key, value.Value} {
					name, ok := target.(*ast.Ident)
					if ok && name.Name != "_" && name.Obj != nil && !objects[name.Obj] {
						objects[name.Obj] = true
						changed = true
					}
				}
			}
			return true
		})
	}
}

func assignmentValueHasProvenance(index, leftCount int, values []ast.Expr, expressionHasProvenance func(ast.Expr) bool) bool {
	if len(values) == leftCount && index < len(values) {
		return expressionHasProvenance(values[index])
	}
	for _, value := range values {
		if expressionHasProvenance(value) {
			return true
		}
	}
	return false
}

func expressionHasSinkDerivedProvenance(expression ast.Expr, sinkDerived, eventTainted map[*ast.Object]bool, reducerSinks map[string]bool, bindings map[string]*ast.Object, parents map[ast.Node]ast.Node) bool {
	if expressionReferencesTaint(expression, sinkDerived) {
		return true
	}
	found := false
	ast.Inspect(expression, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return !found
		}
		name := callExpressionName(call.Fun)
		if reducerSinks[name] && callUsesExactSinkBinding(call, name, bindings) &&
			(callReferencesTaint(call, eventTainted) || callReferencesTaint(call, sinkDerived) || callControlledByTaint(call, parents, eventTainted)) {
			found = true
			return false
		}
		return !found
	})
	return found
}

func mutationPayloadHasSinkDerivedProvenance(root ast.Node, call *ast.CallExpr, sinkDerived, eventTainted map[*ast.Object]bool, reducerSinks map[string]bool, bindings map[string]*ast.Object, parents map[ast.Node]ast.Node) bool {
	for _, payload := range mutationPayloadExpressions(call) {
		if expressionHasSafeSinkDerivedProvenance(root, payload, sinkDerived, eventTainted, reducerSinks, bindings, parents) {
			return true
		}
	}
	return false
}

func expressionHasSafeSinkDerivedProvenance(root ast.Node, expression ast.Expr, sinkDerived, eventTainted map[*ast.Object]bool, reducerSinks map[string]bool, bindings map[string]*ast.Object, parents map[ast.Node]ast.Node) bool {
	if !expressionHasSinkDerivedProvenance(expression, sinkDerived, eventTainted, reducerSinks, bindings, parents) {
		return false
	}
	rawEvent := make(map[*ast.Object]bool, len(eventTainted))
	for object := range eventTainted {
		if !sinkDerived[object] {
			rawEvent[object] = true
		}
	}
	if expressionReferencesUnsanitizedEvent(expression, rawEvent, reducerSinks, bindings) {
		return false
	}
	for _, object := range referencedObjects(expression, sinkDerived) {
		if !sinkDerivedObjectAssignmentsAreSafe(root, object, sinkDerived, rawEvent, reducerSinks, bindings, parents, map[*ast.Object]bool{}) {
			return false
		}
	}
	return true
}

func expressionReferencesUnsanitizedEvent(expression ast.Expr, rawEvent map[*ast.Object]bool, reducerSinks map[string]bool, bindings map[string]*ast.Object) bool {
	if expression == nil {
		return false
	}
	found := false
	ast.Inspect(expression, func(node ast.Node) bool {
		if found {
			return false
		}
		if call, ok := node.(*ast.CallExpr); ok {
			name := callExpressionName(call.Fun)
			if reducerSinks[name] && callUsesExactSinkBinding(call, name, bindings) {
				return false
			}
		}
		identifier, ok := node.(*ast.Ident)
		if ok && identifier.Obj != nil && rawEvent[identifier.Obj] {
			found = true
			return false
		}
		return true
	})
	return found
}

func referencedObjects(expression ast.Expr, selected map[*ast.Object]bool) []*ast.Object {
	seen := map[*ast.Object]bool{}
	ret := make([]*ast.Object, 0)
	ast.Inspect(expression, func(node ast.Node) bool {
		identifier, ok := node.(*ast.Ident)
		if !ok || identifier.Obj == nil || !selected[identifier.Obj] || seen[identifier.Obj] {
			return true
		}
		seen[identifier.Obj] = true
		ret = append(ret, identifier.Obj)
		return true
	})
	return ret
}

func sinkDerivedObjectAssignmentsAreSafe(root ast.Node, object *ast.Object, sinkDerived, rawEvent map[*ast.Object]bool, reducerSinks map[string]bool, bindings map[string]*ast.Object, parents map[ast.Node]ast.Node, checking map[*ast.Object]bool) bool {
	if object == nil || checking[object] {
		return true
	}
	checking[object] = true
	defer delete(checking, object)
	foundAssignment := false
	safe := true
	checkExpression := func(expression ast.Expr) {
		foundAssignment = true
		if !expressionHasSinkDerivedProvenance(expression, sinkDerived, rawEvent, reducerSinks, bindings, parents) ||
			expressionReferencesUnsanitizedEvent(expression, rawEvent, reducerSinks, bindings) {
			safe = false
			return
		}
		for _, dependency := range referencedObjects(expression, sinkDerived) {
			if dependency != object && !sinkDerivedObjectAssignmentsAreSafe(root, dependency, sinkDerived, rawEvent, reducerSinks, bindings, parents, checking) {
				safe = false
				return
			}
		}
	}
	ast.Inspect(root, func(node ast.Node) bool {
		if !safe {
			return false
		}
		switch value := node.(type) {
		case *ast.AssignStmt:
			for index, left := range value.Lhs {
				identifier, ok := left.(*ast.Ident)
				if !ok || identifier.Obj != object {
					continue
				}
				for _, expression := range assignmentExpressions(index, len(value.Lhs), value.Rhs) {
					checkExpression(expression)
				}
			}
		case *ast.ValueSpec:
			for index, identifier := range value.Names {
				if identifier.Obj != object {
					continue
				}
				for _, expression := range assignmentExpressions(index, len(value.Names), value.Values) {
					checkExpression(expression)
				}
			}
		case *ast.RangeStmt:
			for _, target := range []ast.Expr{value.Key, value.Value} {
				identifier, ok := target.(*ast.Ident)
				if ok && identifier.Obj == object {
					checkExpression(value.X)
				}
			}
		}
		return safe
	})
	return safe && foundAssignment
}

func assignmentExpressions(index, leftCount int, values []ast.Expr) []ast.Expr {
	if len(values) == leftCount && index < len(values) {
		return values[index : index+1]
	}
	return values
}

func mutationPayloadExpressions(call *ast.CallExpr) []ast.Expr {
	if len(call.Args) == 0 {
		return nil
	}
	parts := strings.Split(callExpressionName(call.Fun), ".")
	method := parts[len(parts)-1]
	if (method == "CAS" || method == "CompareAndSwap") && len(call.Args) >= 2 {
		return call.Args[len(call.Args)-2 : len(call.Args)-1]
	}
	return call.Args[len(call.Args)-1:]
}

func assignedStateSelector(expression ast.Expr) string {
	selector, ok := expression.(*ast.SelectorExpr)
	if !ok || !isStateLikeSelectorName(selector.Sel.Name) {
		return ""
	}
	return callExpressionName(selector)
}

func isStateLikeSelectorName(name string) bool {
	lower := strings.ToLower(name)
	for _, suffix := range stateLikeSuffixes {
		if strings.HasSuffix(lower, strings.ToLower(suffix)) {
			return true
		}
	}
	return false
}

func expressionReferencesTaint(expression ast.Expr, tainted map[*ast.Object]bool) bool {
	if expression == nil {
		return false
	}
	found := false
	ast.Inspect(expression, func(node ast.Node) bool {
		identifier, ok := node.(*ast.Ident)
		if ok && identifier.Obj != nil && tainted[identifier.Obj] {
			found = true
			return false
		}
		return !found
	})
	return found
}

func callReferencesTaint(call *ast.CallExpr, tainted map[*ast.Object]bool) bool {
	for _, argument := range call.Args {
		if expressionReferencesTaint(argument, tainted) {
			return true
		}
	}
	if selector, ok := call.Fun.(*ast.SelectorExpr); ok {
		return expressionReferencesTaint(selector.X, tainted)
	}
	return false
}

func parentNodes(root ast.Node) map[ast.Node]ast.Node {
	ret := map[ast.Node]ast.Node{}
	stack := make([]ast.Node, 0)
	ast.Inspect(root, func(node ast.Node) bool {
		if node == nil {
			stack = stack[:len(stack)-1]
			return false
		}
		if len(stack) != 0 {
			ret[node] = stack[len(stack)-1]
		}
		stack = append(stack, node)
		return true
	})
	return ret
}

func callControlledByTaint(call *ast.CallExpr, parents map[ast.Node]ast.Node, tainted map[*ast.Object]bool) bool {
	var child ast.Node = call
	for parent := parents[child]; parent != nil; child, parent = parent, parents[parent] {
		switch control := parent.(type) {
		case *ast.IfStmt:
			if child != control.Init && child != control.Cond && expressionReferencesTaint(control.Cond, tainted) {
				return true
			}
		case *ast.SwitchStmt:
			if child != control.Init && child != control.Tag && expressionReferencesTaint(control.Tag, tainted) {
				return true
			}
		case *ast.TypeSwitchStmt:
			if child != control.Init && child != control.Assign && nodeReferencesTaint(control.Assign, tainted) {
				return true
			}
		case *ast.ForStmt:
			if child != control.Init && child != control.Cond && expressionReferencesTaint(control.Cond, tainted) {
				return true
			}
		case *ast.RangeStmt:
			if child != control.X && expressionReferencesTaint(control.X, tainted) {
				return true
			}
		case *ast.CaseClause:
			if !nodeIsOneOf(child, control.List) && expressionsReferenceTaint(control.List, tainted) {
				return true
			}
		case *ast.CommClause:
			if child != control.Comm && nodeReferencesTaint(control.Comm, tainted) {
				return true
			}
		}
	}
	return false
}

func nodeReferencesTaint(node ast.Node, tainted map[*ast.Object]bool) bool {
	if node == nil {
		return false
	}
	found := false
	ast.Inspect(node, func(item ast.Node) bool {
		identifier, ok := item.(*ast.Ident)
		if ok && identifier.Obj != nil && tainted[identifier.Obj] {
			found = true
			return false
		}
		return !found
	})
	return found
}

func expressionsReferenceTaint(expressions []ast.Expr, tainted map[*ast.Object]bool) bool {
	for _, expression := range expressions {
		if expressionReferencesTaint(expression, tainted) {
			return true
		}
	}
	return false
}

func nodeIsOneOf(node ast.Node, expressions []ast.Expr) bool {
	for _, expression := range expressions {
		if node == expression {
			return true
		}
	}
	return false
}

func isStateMutationCall(name string) bool {
	parts := strings.Split(name, ".")
	return len(parts) != 0 && stateMutationSinkNames[parts[len(parts)-1]]
}

func callExpressionName(expression ast.Expr) string {
	switch value := expression.(type) {
	case *ast.Ident:
		return value.Name
	case *ast.SelectorExpr:
		prefix := callExpressionName(value.X)
		if prefix == "" {
			return value.Sel.Name
		}
		return prefix + "." + value.Sel.Name
	case *ast.IndexExpr:
		return callExpressionName(value.X)
	case *ast.IndexListExpr:
		return callExpressionName(value.X)
	case *ast.ParenExpr:
		return callExpressionName(value.X)
	default:
		return ""
	}
}

func discoverDeclarations(root string) (map[string]declaration, map[string]int, error) {
	ret := map[string]declaration{}
	typedConstantCounts := map[string]int{}
	for _, top := range []string{"application", "internal"} {
		base := filepath.Join(root, top)
		err := filepath.WalkDir(base, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				if entry.Name() == "vendor" || entry.Name() == "testdata" {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
				return nil
			}
			parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
			if err != nil {
				return fmt.Errorf("parse %s: %w", path, err)
			}
			relative, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			relative = filepath.ToSlash(relative)
			packageDir := filepath.ToSlash(filepath.Dir(relative))
			for _, declarationNode := range parsed.Decls {
				general, ok := declarationNode.(*ast.GenDecl)
				if !ok {
					continue
				}
				if general.Tok == token.CONST {
					for _, specification := range general.Specs {
						valueSpec, ok := specification.(*ast.ValueSpec)
						if !ok {
							continue
						}
						typeName, ok := valueSpec.Type.(*ast.Ident)
						if !ok {
							continue
						}
						typedConstantCounts[packageDir+":"+typeName.Name] += len(valueSpec.Names)
					}
					continue
				}
				if general.Tok != token.TYPE {
					continue
				}
				for _, specification := range general.Specs {
					typeSpec, ok := specification.(*ast.TypeSpec)
					if !ok || typeSpec.Assign.IsValid() {
						continue
					}
					if underlying, ok := typeSpec.Type.(*ast.Ident); ok && underlying.Name == "string" && isStateLikeName(typeSpec.Name.Name) {
						item := declaration{File: relative, PackageDir: packageDir, Name: typeSpec.Name.Name, Kind: declarationNamedStateType}
						key := item.File + ":" + item.Name
						if _, duplicate := ret[key]; duplicate {
							return fmt.Errorf("duplicate state-like declaration %s", key)
						}
						ret[key] = item
					}
					if !isPersistentOwnerModelPackageDir(packageDir) {
						continue
					}
					structure, ok := typeSpec.Type.(*ast.StructType)
					if !ok {
						continue
					}
					if err := discoverBareStateFields(ret, relative, packageDir, typeSpec.Name.Name, structure); err != nil {
						return err
					}
				}
			}
			return nil
		})
		if err != nil {
			return nil, nil, err
		}
	}
	return ret, typedConstantCounts, nil
}

func discoverBareStateFields(declarations map[string]declaration, file, packageDir, parentName string, structure *ast.StructType) error {
	if structure == nil || structure.Fields == nil {
		return nil
	}
	for _, field := range structure.Fields.List {
		for _, name := range field.Names {
			fieldName := parentName + "." + name.Name
			if isStateLikeName(name.Name) && isBareStringExpression(field.Type) {
				item := declaration{File: file, PackageDir: packageDir, Name: fieldName, Kind: declarationBareStateField}
				key := item.File + ":" + item.Name
				if _, duplicate := declarations[key]; duplicate {
					return fmt.Errorf("duplicate state-like declaration %s", key)
				}
				declarations[key] = item
			}
			if nested, ok := field.Type.(*ast.StructType); ok {
				if err := discoverBareStateFields(declarations, file, packageDir, fieldName, nested); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func isBareStringExpression(expression ast.Expr) bool {
	switch value := expression.(type) {
	case *ast.Ident:
		return value.Name == "string"
	case *ast.StarExpr:
		return isBareStringExpression(value.X)
	case *ast.ParenExpr:
		return isBareStringExpression(value.X)
	default:
		return false
	}
}

func isPersistentOwnerModelPackageDir(packageDir string) bool {
	normalized := "/" + strings.Trim(filepath.ToSlash(packageDir), "/") + "/"
	ownerPackage := strings.HasPrefix(normalized, "/internal/modules/blocks/") || strings.HasPrefix(normalized, "/internal/modules/kernel/")
	return ownerPackage && strings.Contains(normalized, "/pkg/models/")
}

func isStateLikeName(name string) bool {
	for _, suffix := range stateLikeSuffixes {
		if strings.HasSuffix(name, suffix) {
			return true
		}
	}
	return false
}

func readInventory(path string) (map[string]inventoryEntry, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("read state-model inventory: %w", err)
	}
	defer file.Close()

	ret := map[string]inventoryEntry{}
	scanner := bufio.NewScanner(file)
	for line := 1; scanner.Scan(); line++ {
		raw := strings.TrimSpace(scanner.Text())
		if raw == "" || strings.HasPrefix(raw, "#") {
			continue
		}
		parts := strings.Split(raw, "|")
		if len(parts) != 5 {
			return nil, fmt.Errorf("malformed state-model inventory row %d", line)
		}
		for index := range parts {
			parts[index] = strings.TrimSpace(parts[index])
		}
		entry := inventoryEntry{
			Declaration: declaration{
				File:       filepath.ToSlash(parts[0]),
				PackageDir: filepath.ToSlash(filepath.Dir(parts[0])),
				Name:       parts[1],
			},
			Classification: parts[2], OwnerRoot: filepath.ToSlash(parts[3]), Line: line,
		}
		if parts[4] != "-" && parts[4] != "" {
			entry.TransitionIDs = strings.Split(parts[4], ",")
			for index := range entry.TransitionIDs {
				entry.TransitionIDs[index] = strings.TrimSpace(entry.TransitionIDs[index])
			}
		}
		key := entry.Declaration.File + ":" + entry.Declaration.Name
		if _, duplicate := ret[key]; duplicate {
			return nil, fmt.Errorf("duplicate state-model inventory declaration at row %d: %s", line, key)
		}
		ret[key] = entry
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return ret, nil
}

func readTransitionOwners(path string) (map[string]transitionOwner, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("read state-transition owner inventory: %w", err)
	}
	defer file.Close()

	ret := map[string]transitionOwner{}
	scanner := bufio.NewScanner(file)
	for line := 1; scanner.Scan(); line++ {
		raw := strings.TrimSpace(scanner.Text())
		if raw == "" || strings.HasPrefix(raw, "#") {
			continue
		}
		parts := strings.Split(raw, "|")
		if len(parts) != 8 {
			return nil, fmt.Errorf("malformed state-transition owner row %d", line)
		}
		id := strings.TrimSpace(parts[0])
		classification := strings.TrimSpace(parts[1])
		owner := strings.TrimSpace(parts[2])
		if id == "" || classification == "" || owner == "" {
			return nil, fmt.Errorf("state-transition owner row %d omits identity, classification or owner", line)
		}
		if _, duplicate := ret[id]; duplicate {
			return nil, fmt.Errorf("duplicate state-transition owner identity %q", id)
		}
		internalEventType := strings.TrimSpace(parts[6])
		if !token.IsIdentifier(internalEventType) {
			return nil, fmt.Errorf("state-transition owner row %d has invalid internal event type %q", line, internalEventType)
		}
		sinks := splitNonEmpty(parts[7])
		if len(sinks) == 0 {
			return nil, fmt.Errorf("state-transition owner row %d omits exact transition sinks", line)
		}
		for _, sink := range sinks {
			if !isSelectorPath(sink) {
				return nil, fmt.Errorf("state-transition owner row %d has invalid transition sink %q", line, sink)
			}
		}
		ret[id] = transitionOwner{
			Classification:    classification,
			OwnerRoot:         filepath.ToSlash(owner),
			InternalEventFile: filepath.ToSlash(strings.TrimSpace(parts[3])),
			DispatcherFile:    filepath.ToSlash(strings.TrimSpace(parts[4])),
			DispatcherSymbol:  strings.TrimSpace(parts[5]),
			InternalEventType: internalEventType,
			TransitionSinks:   sinks,
			Line:              line,
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return ret, nil
}

func splitNonEmpty(raw string) []string {
	ret := make([]string, 0)
	for _, value := range strings.Split(raw, ",") {
		value = strings.TrimSpace(value)
		if value != "" {
			ret = append(ret, value)
		}
	}
	return ret
}

func isSelectorPath(value string) bool {
	parts := strings.Split(value, ".")
	if len(parts) == 0 {
		return false
	}
	for _, part := range parts {
		if !token.IsIdentifier(part) {
			return false
		}
	}
	return true
}
