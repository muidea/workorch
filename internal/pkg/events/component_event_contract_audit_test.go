package events

import (
	"fmt"
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

	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	approvalevents "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/events"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	eventstreamevents "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/events"
	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
	memoryevents "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/events"
	resultevents "github.com/muidea/workorch/internal/modules/blocks/result/pkg/events"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	governanceevents "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/events"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
)

// TestOwnerEventMessageAliasesDoNotGrow is a test-only zero-debt guard.
// Production code must not use this scan as a topic/type registry.
func TestOwnerEventMessageAliasesDoNotGrow(t *testing.T) {
	want := []string{}
	sort.Strings(want)
	got := externalOwnerEventAliases(t)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("external owner EventHub alias debt changed; migrate aliases in the receiving owner pkg/events and shrink the allowlist\nwant:\n%s\ngot:\n%s", strings.Join(want, "\n"), strings.Join(got, "\n"))
	}
}

// TestBusinessEventTopicsStayOwnerLocal prevents shared packages from
// becoming a second topic registry and prevents owner topics from forwarding
// a constant defined elsewhere.
func TestBusinessEventTopicsStayOwnerLocal(t *testing.T) {
	repositoryRoot := auditRepositoryRoot(t)
	checkTopics := func(root string, ownerEvents bool) {
		err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			if ownerEvents && (filepath.Base(filepath.Dir(path)) != "events" || filepath.Base(filepath.Dir(filepath.Dir(path))) != "pkg") {
				return nil
			}
			file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
			if err != nil {
				return err
			}
			for _, decl := range file.Decls {
				generic, ok := decl.(*ast.GenDecl)
				if !ok || generic.Tok != token.CONST {
					continue
				}
				for _, spec := range generic.Specs {
					valueSpec, ok := spec.(*ast.ValueSpec)
					if !ok {
						continue
					}
					for index, name := range valueSpec.Names {
						if !strings.HasPrefix(name.Name, "Topic") {
							continue
						}
						if !ownerEvents {
							t.Errorf("shared package defines business topic %s at %s", name.Name, path)
							continue
						}
						if index >= len(valueSpec.Values) {
							t.Errorf("owner topic %s at %s is not directly defined", name.Name, path)
							continue
						}
						literal, ok := valueSpec.Values[index].(*ast.BasicLit)
						if !ok || literal.Kind != token.STRING {
							t.Errorf("owner topic %s at %s forwards or derives an external definition", name.Name, path)
							continue
						}
						topic, err := strconv.Unquote(literal.Value)
						if err != nil {
							t.Errorf("owner topic %s at %s is not a valid string literal: %v", name.Name, path, err)
							continue
						}
						if eventHubPathHasWildcardSegment(topic) {
							t.Errorf("owner topic %s at %s uses wildcard Event.ID %q", name.Name, path, topic)
						}
					}
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("scan business topics under %s: %v", root, err)
		}
	}
	checkTopics(filepath.Join(repositoryRoot, "internal", "pkg"), false)
	checkTopics(filepath.Join(repositoryRoot, "internal", "modules"), true)
}

// TestProductionEventAndObserverIdentitiesStayConcrete keeps destination
// match patterns out of the two identity surfaces that EventHub uses for
// subscription de-duplication and publication routing. It inspects the Go AST
// and resolved constants so formatting and call-site line layout are irrelevant.
func TestProductionEventAndObserverIdentitiesStayConcrete(t *testing.T) {
	repositoryRoot := auditRepositoryRoot(t)
	constants := auditSourceConstants(t, repositoryRoot)
	internalRoot := filepath.Join(repositoryRoot, "internal")
	err := filepath.WalkDir(internalRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, pkgPath, imports, err := auditParseSourceFile(repositoryRoot, path)
		if err != nil {
			return err
		}
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok || len(call.Args) == 0 {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			qualifier, qualified := selector.X.(*ast.Ident)
			importPath := ""
			if qualified {
				importPath = imports[qualifier.Name]
			}
			switch {
			case importPath == "github.com/muidea/magicCommon/event" && (selector.Sel.Name == "NewEvent" || selector.Sel.Name == "NewEventWithContext"):
				auditConcreteEventHubIdentity(t, call.Args[0], pkgPath, imports, constants, path, "Event.ID")
			case importPath == "github.com/muidea/magicCommon/event" && (selector.Sel.Name == "NewSimpleObserver" || selector.Sel.Name == "NewSimpleObserverWithMatchID"):
				auditConcreteEventHubIdentity(t, call.Args[0], pkgPath, imports, constants, path, "Observer.ID")
			case importPath == "github.com/muidea/workorch/internal/modules/base/biz" && selector.Sel.Name == "New":
				auditConcreteEventHubIdentity(t, call.Args[0], pkgPath, imports, constants, path, "Base Observer.ID")
			case selector.Sel.Name == "SubscribeMatchedFunc":
				auditConcreteEventHubIdentity(t, call.Args[0], pkgPath, imports, constants, path, "matched Observer.ID")
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("scan production EventHub identities: %v", err)
	}
}

func auditConcreteEventHubIdentity(t *testing.T, expr ast.Expr, pkgPath string, imports map[string]string, constants map[string]auditSourceConst, sourcePath string, identityKind string) {
	t.Helper()
	value, resolved := auditStringExpr(expr, pkgPath, imports, constants, map[string]bool{})
	if resolved && eventHubPathHasWildcardSegment(value) {
		t.Errorf("production %s in %s uses wildcard identity %q", identityKind, sourcePath, value)
	}
}

func eventHubPathHasWildcardSegment(value string) bool {
	for _, segment := range strings.Split(strings.TrimSpace(value), "/") {
		switch segment {
		case "#", "+", ":id":
			return true
		}
	}
	return false
}

// TestOwnerEventPackagesDoNotImportOtherOwners keeps Command/Data/Result
// graphs local to the receiving owner. Cross-owner mapping belongs in Biz or
// an adapter package, never in either owner's pkg/events.
func TestOwnerEventPackagesDoNotImportOtherOwners(t *testing.T) {
	repositoryRoot := auditRepositoryRoot(t)
	modulesRoot := filepath.Join(repositoryRoot, "internal", "modules")
	err := filepath.WalkDir(modulesRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") || filepath.Base(filepath.Dir(path)) != "events" || filepath.Base(filepath.Dir(filepath.Dir(path))) != "pkg" {
			return nil
		}
		file, _, imports, err := auditParseSourceFile(repositoryRoot, path)
		if err != nil {
			return err
		}
		ownerDirectory := filepath.Dir(filepath.Dir(filepath.Dir(path)))
		relativeOwner, err := filepath.Rel(repositoryRoot, ownerDirectory)
		if err != nil {
			return err
		}
		ownerImportRoot := "github.com/muidea/workorch/" + filepath.ToSlash(relativeOwner)
		for _, importPath := range imports {
			if strings.Contains(importPath, "/internal/modules/") && importPath != ownerImportRoot && !strings.HasPrefix(importPath, ownerImportRoot+"/") {
				t.Errorf("owner event package %s imports another owner %s", filepath.Dir(path), importPath)
			}
		}
		_ = file
		return nil
	})
	if err != nil {
		t.Fatalf("scan owner event imports: %v", err)
	}
}

// TestOwnerModelsDoNotAliasSharedBusinessContracts prevents owner DTOs from
// being physically defined in internal/pkg and re-exported through aliases.
func TestOwnerModelsDoNotAliasSharedBusinessContracts(t *testing.T) {
	repositoryRoot := auditRepositoryRoot(t)
	modulesRoot := filepath.Join(repositoryRoot, "internal", "modules")
	err := filepath.WalkDir(modulesRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") || filepath.Base(filepath.Dir(path)) != "models" || filepath.Base(filepath.Dir(filepath.Dir(path))) != "pkg" {
			return nil
		}
		file, _, imports, err := auditParseSourceFile(repositoryRoot, path)
		if err != nil {
			return err
		}
		for _, decl := range file.Decls {
			generic, ok := decl.(*ast.GenDecl)
			if !ok || generic.Tok != token.TYPE {
				continue
			}
			for _, spec := range generic.Specs {
				typeSpec, ok := spec.(*ast.TypeSpec)
				if !ok || !typeSpec.Assign.IsValid() {
					continue
				}
				selector, ok := typeSpec.Type.(*ast.SelectorExpr)
				if !ok {
					continue
				}
				qualifier, ok := selector.X.(*ast.Ident)
				if ok && strings.Contains(imports[qualifier.Name], "/internal/pkg/") {
					t.Errorf("owner model alias %s at %s points to shared business DTO %s.%s", typeSpec.Name.Name, path, imports[qualifier.Name], selector.Sel.Name)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan owner model aliases: %v", err)
	}
}

// TestSharedEventsPackageStaysOwnerNeutral prevents the EventHub lane helper
// package from becoming a second business payload contract/index. Audit wire
// payload belongs to blocks/eventstream/pkg/recordpayload; owner commands and
// notifications keep concrete Data/Result in the receiving owner.
func TestSharedEventsPackageStaysOwnerNeutral(t *testing.T) {
	repositoryRoot := auditRepositoryRoot(t)
	sharedEventsRoot := filepath.Join(repositoryRoot, "internal", "pkg", "events")
	err := filepath.WalkDir(sharedEventsRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		for _, decl := range file.Decls {
			switch item := decl.(type) {
			case *ast.GenDecl:
				if item.Tok != token.TYPE {
					continue
				}
				for _, spec := range item.Specs {
					typeSpec, ok := spec.(*ast.TypeSpec)
					if ok && strings.HasSuffix(typeSpec.Name.Name, "PayloadData") {
						t.Errorf("shared EventHub helper defines business payload type %s at %s", typeSpec.Name.Name, path)
					}
				}
			case *ast.FuncDecl:
				name := item.Name.Name
				if strings.HasPrefix(name, "Decode") && strings.HasSuffix(name, "EventPayload") {
					t.Errorf("shared EventHub helper defines business payload decoder %s at %s", name, path)
				}
				if name == "EventPayload" && item.Recv != nil {
					t.Errorf("shared EventHub helper defines business payload encoder method at %s", path)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan shared EventHub helper package: %v", err)
	}
}

func externalOwnerEventAliases(t *testing.T) []string {
	t.Helper()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve audit test path")
	}
	repositoryRoot := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", "..", ".."))
	modulesRoot := filepath.Join(repositoryRoot, "internal", "modules")
	ret := []string{}
	err := filepath.WalkDir(modulesRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") || filepath.Base(filepath.Dir(path)) != "events" || filepath.Base(filepath.Dir(filepath.Dir(path))) != "pkg" {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		imports := map[string]string{}
		for _, item := range file.Imports {
			importPath, err := strconv.Unquote(item.Path.Value)
			if err != nil {
				return err
			}
			name := filepath.Base(importPath)
			if item.Name != nil {
				name = item.Name.Name
			}
			imports[name] = importPath
		}
		for _, decl := range file.Decls {
			generic, ok := decl.(*ast.GenDecl)
			if !ok || generic.Tok != token.TYPE {
				continue
			}
			for _, spec := range generic.Specs {
				typeSpec, ok := spec.(*ast.TypeSpec)
				if !ok || !typeSpec.Assign.IsValid() {
					continue
				}
				selector, ok := typeSpec.Type.(*ast.SelectorExpr)
				if !ok {
					continue
				}
				qualifier, ok := selector.X.(*ast.Ident)
				if !ok {
					continue
				}
				importPath := imports[qualifier.Name]
				if !strings.Contains(importPath, "/internal/modules/") || (!strings.HasSuffix(importPath, "/pkg/contracts") && !strings.HasSuffix(importPath, "/pkg/events")) {
					continue
				}
				relative, err := filepath.Rel(repositoryRoot, path)
				if err != nil {
					return err
				}
				ret = append(ret, fmt.Sprintf("%s:%s=%s.%s", filepath.ToSlash(relative), typeSpec.Name.Name, importPath, selector.Sel.Name))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan owner event aliases: %v", err)
	}
	sort.Strings(ret)
	return ret
}

type auditSourceType struct {
	expr    ast.Expr
	pkgPath string
	imports map[string]string
	alias   bool
}

type auditSourceConst struct {
	expr    ast.Expr
	pkgPath string
	imports map[string]string
}

type auditMessageRoot struct {
	expr    ast.Expr
	pkgPath string
	imports map[string]string
	path    string
}

// TestOwnerEventMessageGraphsAreConcrete is a test-only source graph audit.
// It deliberately avoids a production reflect/type registry.
func TestOwnerEventMessageGraphsAreConcrete(t *testing.T) {
	repositoryRoot := auditRepositoryRoot(t)
	types, roots := auditMessageSourceGraph(t, repositoryRoot)
	for _, root := range roots {
		auditConcreteMessageExpr(t, root.expr, root.pkgPath, root.imports, root.path, types, map[string]bool{})
	}
}

// TestOwnerEventMessageRootsAreDeclaredTypes prevents distinct topics from
// sharing a compatibility/type-alias identity. Supporting owner-neutral value
// aliases may appear inside a message graph, but every Command/Data/Result
// root named by MessageContracts must be declared concretely by its owner.
func TestOwnerEventMessageRootsAreDeclaredTypes(t *testing.T) {
	repositoryRoot := auditRepositoryRoot(t)
	types, roots := auditMessageSourceGraph(t, repositoryRoot)
	for _, root := range roots {
		key := auditRootTypeKey(root.expr, root.pkgPath, root.imports)
		if key == "" {
			t.Fatalf("EventHub message root is not a named owner type path=%s", root.path)
		}
		source, ok := types[key]
		if !ok {
			t.Fatalf("EventHub message root is not declared in repository path=%s type=%s", root.path, key)
		}
		if source.alias {
			t.Fatalf("EventHub message root uses type alias path=%s type=%s", root.path, key)
		}
	}
}

func auditRootTypeKey(expr ast.Expr, pkgPath string, imports map[string]string) string {
	switch value := expr.(type) {
	case *ast.Ident:
		return pkgPath + "." + value.Name
	case *ast.SelectorExpr:
		qualifier, ok := value.X.(*ast.Ident)
		if !ok {
			return ""
		}
		return imports[qualifier.Name] + "." + value.Sel.Name
	default:
		return ""
	}
}

// TestEventHubPostSitesRemainNotificationOnly is a zero-growth guard for the
// production EventHub async surface. The listed functions have been audited to
// construct only the concrete notification topics below. A new Post site must
// first prove that it publishes an immutable fact and does not require an ack.
func TestEventHubPostSitesRemainNotificationOnly(t *testing.T) {
	repositoryRoot := auditRepositoryRoot(t)
	wantSites := map[string]int{
		"internal/modules/application/workorchd/biz/owner.go:publishServiceStopped":                    1,
		"internal/modules/application/workorchd/biz/owner.go:publishWorkspaceProjectionChanged":        1,
		"internal/modules/blocks/approval/biz/approval_notification.go:publishApprovalChanged":         1,
		"internal/modules/blocks/decision/biz/decision_notification.go:publishTicketCreated":           1,
		"internal/modules/blocks/decision/biz/decision_notification.go:recordAndPublishResolvedTicket": 1,
		"internal/modules/blocks/eventstream/biz/event_stream_service.go:publishRecordAppended":        1,
		"internal/modules/blocks/llm/biz/owner.go:publishLLMUsageRecorded":                             1,
		"internal/modules/blocks/memory/biz/memory_notification.go:publishMemoryChanged":               1,
		"internal/modules/blocks/memory/biz/memory_notification.go:publishMemoryRecallRecorded":        1,
		"internal/modules/blocks/result/biz/owner.go:publishResultChanged":                             1,
		"internal/modules/blocks/run/biz/lifecycle_handler.go:publishContinuationRequested":            1,
		"internal/modules/blocks/run/biz/lifecycle_handler.go:publishLifecycleTransition":              2,
		"internal/modules/blocks/run/biz/owner.go:publishRuntimeCommandsReady":                         1,
		"internal/modules/blocks/run/biz/run_service.go:PublishRunState":                               1,
		"internal/modules/kernel/workspace/biz/goal_dod_manager.go:publishDoDEvent":                    1,
		"internal/modules/kernel/workspace/biz/goal_dod_manager.go:publishGoalEvent":                   1,
		"internal/modules/kernel/workspace/biz/goal_dod_manager.go:publishWorkspaceEvent":              1,
		"internal/modules/kernel/workspace/biz/goal_dod_manager.go:publishWorkspaceDeleted":            1,
		"internal/modules/kernel/workspace/biz/owner.go:publishWorkspaceUpdated":                       1,
		"internal/modules/kernel/governance/biz/governance_support.go:publishGovernanceEvent":          1,
		"internal/modules/kernel/orchestrator/biz/runtime_observability.go:observeRuntimeDrive":        1,
	}
	gotSites := auditEventHubPostSites(t, repositoryRoot)
	if !reflectStringIntMapEqual(gotSites, wantSites) {
		t.Fatalf("production EventHub Post sites changed; async delivery is notification-only\nwant=%v\ngot=%v", wantSites, gotSites)
	}

	contracts, _ := testContractCatalog(t)
	postTopics := []string{
		workorchdevents.TopicWorkorchDServiceStopped, workorchdevents.TopicWorkspaceProjectionChanged,
		approvalevents.TopicApprovalResolved,
		decisionevents.TopicTicketCreated, decisionevents.TopicTicketResolved,
		eventstreamevents.TopicEventStreamRecordAppended,
		llmevents.TopicLLMUsageRecorded,
		memoryevents.TopicMemoryCreated, memoryevents.TopicMemoryUpdated, memoryevents.TopicMemoryDeleted, memoryevents.TopicMemoryRecallRecorded,
		resultevents.TopicResultCreated, resultevents.TopicResultAccepted, resultevents.TopicResultRejected, resultevents.TopicResultSuperseded,
		runevents.TopicRunCreated, runevents.TopicRunStarted, runevents.TopicRunBlockedOnDecision, runevents.TopicRunBlockedOnApproval,
		runevents.TopicRunCompleted, runevents.TopicRunFailed, runevents.TopicRunCancelled, runevents.TopicRunSuperseded, runevents.TopicRunStateChanged,
		runevents.TopicRunContinuationRequested, runevents.TopicRunRuntimeCommandsReady,
		workspaceevents.TopicWorkspaceCreated, workspaceevents.TopicWorkspaceStateChanged, workspaceevents.TopicWorkspaceDeleted,
		workspaceevents.TopicGoalDefined, workspaceevents.TopicDoDDefined,
		governanceevents.TopicGovernanceAutoEvaluated, governanceevents.TopicGovernanceAutoResolved,
		governanceevents.TopicGovernanceAutoEscalated, governanceevents.TopicGovernanceAutoBlocked,
		orchestratorevents.TopicOrchestratorRuntimeDriveObserved,
	}
	postTopicSet := map[string]bool{}
	for _, topic := range postTopics {
		postTopicSet[topic] = true
		contract, ok := contracts[topic]
		if !ok {
			t.Fatalf("Post notification topic %q has no contract", topic)
		}
		if contract.CommandClass != eventcontract.CommandClassNotify || len(contract.ReceiverOwners) != 0 {
			t.Fatalf("Post topic %q is not a broadcast notification: %+v", topic, contract)
		}
	}
	for topic, contract := range contracts {
		if contract.CommandClass == eventcontract.CommandClassNotify && !postTopicSet[topic] {
			t.Errorf("notification topic %q has no audited production Post publisher", topic)
		}
	}
}

// TestNotificationSubscribersIgnoreSynchronousResult proves that Post-backed
// notification observers do not acknowledge, reject, or otherwise depend on
// the Result argument that magicCommon leaves nil for asynchronous delivery.
func TestNotificationSubscribersIgnoreSynchronousResult(t *testing.T) {
	repositoryRoot := auditRepositoryRoot(t)
	contracts, _ := testContractCatalog(t)
	constants := auditSourceConstants(t, repositoryRoot)
	modulesRoot := filepath.Join(repositoryRoot, "internal", "modules")
	err := filepath.WalkDir(modulesRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, pkgPath, imports, err := auditParseSourceFile(repositoryRoot, path)
		if err != nil {
			return err
		}
		functions := map[string]*ast.FuncDecl{}
		for _, decl := range file.Decls {
			if function, ok := decl.(*ast.FuncDecl); ok {
				functions[function.Name.Name] = function
			}
		}
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok || len(call.Args) < 4 {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || selector.Sel.Name != "SubscribeMatchedFunc" {
				return true
			}
			topic, topicOK := auditStringExpr(call.Args[2], pkgPath, imports, constants, map[string]bool{})
			callback := call.Args[3]
			if topicOK {
				contract, exists := contracts[topic]
				if !exists || contract.CommandClass != eventcontract.CommandClassNotify {
					return true
				}
			} else {
				// Dynamic matched subscriptions are allowed only for inline
				// notification callbacks whose Result is explicitly ignored.
				if _, ok := callback.(*ast.FuncLit); !ok {
					return true
				}
			}
			var functionType *ast.FuncType
			var body *ast.BlockStmt
			switch value := callback.(type) {
			case *ast.FuncLit:
				functionType, body = value.Type, value.Body
			case *ast.SelectorExpr:
				if function := functions[value.Sel.Name]; function != nil {
					functionType, body = function.Type, function.Body
				} else if function := auditPackageFunction(filepath.Dir(path), value.Sel.Name); function != nil {
					functionType, body = function.Type, function.Body
				}
			case *ast.Ident:
				if function := functions[value.Name]; function != nil {
					functionType, body = function.Type, function.Body
				} else if function := auditPackageFunction(filepath.Dir(path), value.Name); function != nil {
					functionType, body = function.Type, function.Body
				}
			}
			if !auditObserverResultIgnored(functionType, body) {
				t.Errorf("notification subscriber uses synchronous Result file=%s topic=%s", path, topic)
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("scan notification subscribers: %v", err)
	}
}

// TestModuleEventHubSendResultsAreConsumed rejects fire-and-forget Send use in
// production running units. A synchronous command must either decode its
// concrete Result or deliberately propagate it to a typed decoder.
func TestModuleEventHubSendResultsAreConsumed(t *testing.T) {
	repositoryRoot := auditRepositoryRoot(t)
	modulesRoot := filepath.Join(repositoryRoot, "internal", "modules")
	err := filepath.WalkDir(modulesRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		ast.Inspect(file, func(node ast.Node) bool {
			statement, ok := node.(*ast.ExprStmt)
			if !ok {
				return true
			}
			call, ok := statement.X.(*ast.CallExpr)
			if !ok {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if ok && (selector.Sel.Name == "Send" || selector.Sel.Name == "SendEvent") {
				t.Errorf("EventHub Send result ignored at %s", fset.Position(call.Pos()))
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("scan EventHub Send consumers: %v", err)
	}
}

func auditPackageFunction(directory, name string) *ast.FuncDecl {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), filepath.Join(directory, entry.Name()), nil, 0)
		if err != nil {
			continue
		}
		for _, decl := range file.Decls {
			function, ok := decl.(*ast.FuncDecl)
			if ok && function.Name.Name == name {
				return function
			}
		}
	}
	return nil
}

func auditObserverResultIgnored(functionType *ast.FuncType, body *ast.BlockStmt) bool {
	if functionType == nil || functionType.Params == nil || len(functionType.Params.List) < 2 || body == nil {
		return false
	}
	resultParam := functionType.Params.List[1]
	if len(resultParam.Names) != 1 || resultParam.Names[0].Name != "_" {
		return false
	}
	clean := true
	ast.Inspect(body, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if ok && selector.Sel.Name == "Set" {
			clean = false
		}
		return true
	})
	return clean
}

// TestOwnerEventCommandsHaveProductionSenderAndReceiver prevents a command
// contract from surviving only because its owner still declares it. Ordinary
// sender evidence must exist outside the receiving owner; the two Runtime SoT
// owner-internal canonical entries are explicit and may not grow. Receiver
// evidence must exist in the owner. The scan is test-only and never becomes a
// production topic registry.
func TestOwnerEventCommandsHaveProductionSenderAndReceiver(t *testing.T) {
	repositoryRoot := auditRepositoryRoot(t)
	contracts, messages := testContractCatalog(t)
	constants := auditSourceConstants(t, repositoryRoot)
	topicBySymbol := map[string]string{}
	for symbol := range constants {
		if topic, ok := auditStringConstant(symbol, constants, map[string]bool{}); ok {
			if _, exists := contracts[topic]; exists {
				topicBySymbol[symbol] = topic
			}
		}
	}
	uses := auditTopicSymbolUses(t, repositoryRoot, topicBySymbol)
	ownerInternalCanonicalEntries := map[string]bool{
		orchestratorevents.TopicOrchestratorDriveRuntime: true,
	}
	for topic, contract := range contracts {
		message := messages[topic]
		ownerRoot := strings.TrimSuffix(message.DefinitionPackage, "/pkg/events")
		var sender, receiver bool
		for pkgPath := range uses[topic] {
			if pkgPath == ownerRoot || strings.HasPrefix(pkgPath, ownerRoot+"/") {
				receiver = true
			} else {
				sender = true
			}
		}
		if contract.CommandClass == eventcontract.CommandClassNotify {
			if !sender {
				t.Errorf("EventHub notification topic %q lacks a production subscriber outside owner=%s uses=%v", topic, ownerRoot, uses[topic])
			}
			continue
		}
		if ownerInternalCanonicalEntries[topic] && receiver {
			sender = true
		}
		if !sender || !receiver {
			t.Errorf("EventHub command topic %q lacks production closure sender=%t receiver=%t owner=%s uses=%v", topic, sender, receiver, ownerRoot, uses[topic])
		}
	}
}

func auditSourceConstants(t *testing.T, repositoryRoot string) map[string]auditSourceConst {
	t.Helper()
	ret := map[string]auditSourceConst{}
	internalRoot := filepath.Join(repositoryRoot, "internal")
	err := filepath.WalkDir(internalRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, pkgPath, imports, err := auditParseSourceFile(repositoryRoot, path)
		if err != nil {
			return err
		}
		for _, decl := range file.Decls {
			generic, ok := decl.(*ast.GenDecl)
			if !ok || generic.Tok != token.CONST {
				continue
			}
			var inherited []ast.Expr
			for _, spec := range generic.Specs {
				valueSpec, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				values := valueSpec.Values
				if len(values) == 0 {
					values = inherited
				} else {
					inherited = values
				}
				for idx, name := range valueSpec.Names {
					if len(values) == 0 {
						continue
					}
					expr := values[min(idx, len(values)-1)]
					ret[pkgPath+"."+name.Name] = auditSourceConst{expr: expr, pkgPath: pkgPath, imports: imports}
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan source constants: %v", err)
	}
	return ret
}

func auditStringConstant(symbol string, constants map[string]auditSourceConst, seen map[string]bool) (string, bool) {
	if seen[symbol] {
		return "", false
	}
	seen[symbol] = true
	source, ok := constants[symbol]
	if !ok {
		return "", false
	}
	return auditStringExpr(source.expr, source.pkgPath, source.imports, constants, seen)
}

func auditStringExpr(expr ast.Expr, pkgPath string, imports map[string]string, constants map[string]auditSourceConst, seen map[string]bool) (string, bool) {
	switch value := expr.(type) {
	case *ast.BasicLit:
		if value.Kind != token.STRING {
			return "", false
		}
		decoded, err := strconv.Unquote(value.Value)
		return decoded, err == nil
	case *ast.Ident:
		return auditStringConstant(pkgPath+"."+value.Name, constants, seen)
	case *ast.SelectorExpr:
		qualifier, ok := value.X.(*ast.Ident)
		if !ok {
			return "", false
		}
		return auditStringConstant(imports[qualifier.Name]+"."+value.Sel.Name, constants, seen)
	case *ast.BinaryExpr:
		if value.Op != token.ADD {
			return "", false
		}
		left, leftOK := auditStringExpr(value.X, pkgPath, imports, constants, seen)
		right, rightOK := auditStringExpr(value.Y, pkgPath, imports, constants, seen)
		return left + right, leftOK && rightOK
	case *ast.ParenExpr:
		return auditStringExpr(value.X, pkgPath, imports, constants, seen)
	default:
		return "", false
	}
}

func auditTopicSymbolUses(t *testing.T, repositoryRoot string, topicBySymbol map[string]string) map[string]map[string]bool {
	t.Helper()
	ret := map[string]map[string]bool{}
	internalRoot := filepath.Join(repositoryRoot, "internal")
	err := filepath.WalkDir(internalRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") || filepath.Base(filepath.Dir(path)) == "events" && filepath.Base(filepath.Dir(filepath.Dir(path))) == "pkg" {
			return nil
		}
		file, pkgPath, imports, err := auditParseSourceFile(repositoryRoot, path)
		if err != nil {
			return err
		}
		ast.Inspect(file, func(node ast.Node) bool {
			selector, ok := node.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			qualifier, ok := selector.X.(*ast.Ident)
			if !ok {
				return true
			}
			topic := topicBySymbol[imports[qualifier.Name]+"."+selector.Sel.Name]
			if topic == "" {
				return true
			}
			if ret[topic] == nil {
				ret[topic] = map[string]bool{}
			}
			ret[topic][pkgPath] = true
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("scan topic symbol uses: %v", err)
	}
	return ret
}

func auditParseSourceFile(repositoryRoot, path string) (*ast.File, string, map[string]string, error) {
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		return nil, "", nil, err
	}
	relDir, err := filepath.Rel(repositoryRoot, filepath.Dir(path))
	if err != nil {
		return nil, "", nil, err
	}
	pkgPath := "github.com/muidea/workorch/" + filepath.ToSlash(relDir)
	imports := map[string]string{}
	for _, item := range file.Imports {
		importPath, err := strconv.Unquote(item.Path.Value)
		if err != nil {
			return nil, "", nil, err
		}
		name := filepath.Base(importPath)
		if item.Name != nil {
			name = item.Name.Name
		}
		imports[name] = importPath
	}
	return file, pkgPath, imports, nil
}

func auditEventHubPostSites(t *testing.T, repositoryRoot string) map[string]int {
	t.Helper()
	ret := map[string]int{}
	modulesRoot := filepath.Join(repositoryRoot, "internal", "modules")
	err := filepath.WalkDir(modulesRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		if filepath.ToSlash(path) == filepath.ToSlash(filepath.Join(modulesRoot, "base", "biz", "base.go")) {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(repositoryRoot, path)
		if err != nil {
			return err
		}
		for _, decl := range file.Decls {
			function, ok := decl.(*ast.FuncDecl)
			if !ok || function.Body == nil {
				continue
			}
			ast.Inspect(function.Body, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				selector, ok := call.Fun.(*ast.SelectorExpr)
				indirectPost := ok && ((filepath.Base(path) == "run_service.go" && selector.Sel.Name == "post") ||
					(filepath.Base(path) == "goal_dod_manager.go" && selector.Sel.Name == "publish"))
				if ok && (selector.Sel.Name == "PostEvent" || selector.Sel.Name == "Post" || indirectPost) {
					ret[filepath.ToSlash(relative)+":"+function.Name.Name]++
				}
				return true
			})
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan EventHub Post sites: %v", err)
	}
	return ret
}

func reflectStringIntMapEqual(left, right map[string]int) bool {
	if len(left) != len(right) {
		return false
	}
	for key, value := range left {
		if right[key] != value {
			return false
		}
	}
	return true
}

func auditRepositoryRoot(t *testing.T) string {
	t.Helper()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve audit test path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", "..", ".."))
}

func auditMessageSourceGraph(t *testing.T, repositoryRoot string) (map[string]auditSourceType, []auditMessageRoot) {
	t.Helper()
	types := map[string]auditSourceType{}
	roots := []auditMessageRoot{}
	internalRoot := filepath.Join(repositoryRoot, "internal")
	err := filepath.WalkDir(internalRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		relDir, err := filepath.Rel(repositoryRoot, filepath.Dir(path))
		if err != nil {
			return err
		}
		pkgPath := "github.com/muidea/workorch/" + filepath.ToSlash(relDir)
		imports := map[string]string{}
		for _, item := range file.Imports {
			importPath, err := strconv.Unquote(item.Path.Value)
			if err != nil {
				return err
			}
			name := filepath.Base(importPath)
			if item.Name != nil {
				name = item.Name.Name
			}
			imports[name] = importPath
		}
		for _, decl := range file.Decls {
			generic, ok := decl.(*ast.GenDecl)
			if !ok || generic.Tok != token.TYPE {
				continue
			}
			for _, spec := range generic.Specs {
				typeSpec, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				types[pkgPath+"."+typeSpec.Name.Name] = auditSourceType{expr: typeSpec.Type, pkgPath: pkgPath, imports: imports, alias: typeSpec.Assign.IsValid()}
			}
		}
		if filepath.Base(filepath.Dir(path)) != "events" || filepath.Base(filepath.Dir(filepath.Dir(path))) != "pkg" {
			return nil
		}
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			var fun ast.Expr
			var args []ast.Expr
			switch indexed := call.Fun.(type) {
			case *ast.IndexExpr:
				fun, args = indexed.X, []ast.Expr{indexed.Index}
			case *ast.IndexListExpr:
				fun, args = indexed.X, indexed.Indices
			default:
				return true
			}
			selector, ok := fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			switch selector.Sel.Name {
			case "CommandTopic", "FacadeTopic", "DataTopic":
				for _, arg := range args {
					roots = append(roots, auditMessageRoot{expr: arg, pkgPath: pkgPath, imports: imports, path: pkgPath + "." + selector.Sel.Name})
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("scan EventHub message graph: %v", err)
	}
	if len(roots) == 0 {
		t.Fatal("EventHub message graph audit found no generic contract roots")
	}
	return types, roots
}

func auditConcreteMessageExpr(t *testing.T, expr ast.Expr, pkgPath string, imports map[string]string, path string, types map[string]auditSourceType, seen map[string]bool) {
	t.Helper()
	switch value := expr.(type) {
	case *ast.Ident:
		if value.Name == "any" || value.Name == "interface{}" {
			t.Fatalf("forbidden EventHub any path=%s", path)
		}
		key := pkgPath + "." + value.Name
		if source, ok := types[key]; ok {
			if seen[key] {
				return
			}
			seen[key] = true
			auditConcreteMessageExpr(t, source.expr, source.pkgPath, source.imports, path+"->"+key, types, seen)
		}
	case *ast.SelectorExpr:
		qualifier, ok := value.X.(*ast.Ident)
		if !ok {
			return
		}
		importPath := imports[qualifier.Name]
		if importPath == "github.com/muidea/workorch/internal/pkg/workorcheventpayload" && value.Sel.Name == "Payload" {
			return
		}
		if forbiddenEventResourceType(importPath, value.Sel.Name) {
			t.Fatalf("forbidden EventHub resource path=%s type=%s.%s", path, importPath, value.Sel.Name)
		}
		key := importPath + "." + value.Sel.Name
		if source, ok := types[key]; ok {
			if seen[key] {
				return
			}
			seen[key] = true
			auditConcreteMessageExpr(t, source.expr, source.pkgPath, source.imports, path+"->"+key, types, seen)
		}
	case *ast.StarExpr:
		auditConcreteMessageExpr(t, value.X, pkgPath, imports, path+"*", types, seen)
	case *ast.ArrayType:
		auditConcreteMessageExpr(t, value.Elt, pkgPath, imports, path+"[]", types, seen)
	case *ast.StructType:
		for _, field := range value.Fields.List {
			name := "field"
			if len(field.Names) > 0 {
				name = field.Names[0].Name
			}
			auditConcreteMessageExpr(t, field.Type, pkgPath, imports, path+"."+name, types, seen)
		}
	case *ast.MapType:
		t.Fatalf("forbidden EventHub map path=%s", path)
	case *ast.InterfaceType:
		t.Fatalf("forbidden EventHub interface path=%s", path)
	case *ast.ChanType:
		t.Fatalf("forbidden EventHub channel path=%s", path)
	case *ast.FuncType:
		t.Fatalf("forbidden EventHub function path=%s", path)
	case *ast.ParenExpr:
		auditConcreteMessageExpr(t, value.X, pkgPath, imports, path, types, seen)
	case *ast.IndexExpr:
		auditConcreteMessageExpr(t, value.X, pkgPath, imports, path, types, seen)
		auditConcreteMessageExpr(t, value.Index, pkgPath, imports, path, types, seen)
	case *ast.IndexListExpr:
		auditConcreteMessageExpr(t, value.X, pkgPath, imports, path, types, seen)
		for _, arg := range value.Indices {
			auditConcreteMessageExpr(t, arg, pkgPath, imports, path, types, seen)
		}
	}
}

func forbiddenEventResourceType(importPath, typeName string) bool {
	switch importPath {
	case "io", "database/sql", "net/http":
		return true
	}
	for _, suffix := range []string{"Repository", "Registry", "Recorder"} {
		if strings.HasSuffix(typeName, suffix) {
			return true
		}
	}
	return false
}
