package runtime

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/eventcontract"
)

func TestRuntimeInternalFactsHaveCompleteOwnerPrivateClassification(t *testing.T) {
	kinds, dispatched := runtimeInternalEventSourceFacts(t)
	seenValues := map[RuntimeInternalEventKind]string{}
	for name, kind := range kinds {
		if previous, exists := seenValues[kind]; exists {
			t.Fatalf("runtime internal fact value %q is shared by %s and %s", kind, previous, name)
		}
		seenValues[kind] = name

		metadata, ok := runtimeInternalFactMetadata(kind)
		if !ok {
			t.Fatalf("runtime internal fact %s (%q) has no owner-private classification", name, kind)
		}
		if metadata.Layer != eventcontract.LayerOrchestratorPrivateFact ||
			metadata.StateMachine == "" || metadata.StateMachine == eventcontract.StateMachineNone ||
			metadata.EffectClass == "" || metadata.SourcePolicy == "" ||
			metadata.ScopePolicy != eventcontract.ScopePolicyWorkspaceRun ||
			metadata.IdentityPolicy == "" ||
			metadata.TransitionTarget != eventcontract.TransitionTargetReceiverOwnerPrivate {
			t.Fatalf("runtime internal fact %s has incomplete classification: %+v", name, metadata)
		}
		if !dispatched[name] {
			t.Fatalf("runtime internal fact %s is not handled by ApplyInternalEvent", name)
		}
	}
	if len(kinds) != len(dispatched) {
		t.Fatalf("dispatcher/internal fact count mismatch: kinds=%d dispatched=%d", len(kinds), len(dispatched))
	}
}

// runtimeInternalEventSourceFacts derives completeness from the owner-private
// source itself. The test intentionally keeps no manually maintained global
// topic/type index.
func runtimeInternalEventSourceFacts(t *testing.T) (map[string]RuntimeInternalEventKind, map[string]bool) {
	t.Helper()
	fileSet := token.NewFileSet()
	packages, err := parser.ParseDir(fileSet, ".", func(info os.FileInfo) bool {
		return strings.HasPrefix(info.Name(), "internal_event") && strings.HasSuffix(info.Name(), ".go") && !strings.HasSuffix(info.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatalf("parse runtime internal fact sources: %v", err)
	}
	parsed := packages["runtime"]
	if parsed == nil || parsed.Files["internal_event.go"] == nil {
		t.Fatal("runtime internal event dispatcher source is missing")
	}

	kinds := map[string]RuntimeInternalEventKind{}
	for _, file := range parsed.Files {
		ast.Inspect(file, func(node ast.Node) bool {
			declaration, ok := node.(*ast.GenDecl)
			if !ok || declaration.Tok != token.CONST {
				return true
			}
			for _, spec := range declaration.Specs {
				value, ok := spec.(*ast.ValueSpec)
				if !ok || len(value.Names) != 1 || len(value.Values) != 1 {
					continue
				}
				typeName, typed := value.Type.(*ast.Ident)
				literal, literalOK := value.Values[0].(*ast.BasicLit)
				if !typed || typeName.Name != "RuntimeInternalEventKind" || !literalOK || literal.Kind != token.STRING {
					continue
				}
				decoded, decodeErr := strconv.Unquote(literal.Value)
				if decodeErr != nil {
					t.Fatalf("decode runtime internal fact %s: %v", value.Names[0].Name, decodeErr)
				}
				kinds[value.Names[0].Name] = RuntimeInternalEventKind(decoded)
			}
			return false
		})
	}
	if len(kinds) == 0 {
		t.Fatal("runtime internal fact vocabulary is empty")
	}

	dispatched := map[string]bool{}
	ast.Inspect(parsed.Files["internal_event.go"], func(node ast.Node) bool {
		clause, ok := node.(*ast.CaseClause)
		if !ok {
			return true
		}
		for _, expression := range clause.List {
			identifier, identifierOK := expression.(*ast.Ident)
			if identifierOK {
				if _, known := kinds[identifier.Name]; known {
					dispatched[identifier.Name] = true
				}
			}
		}
		return true
	})
	return kinds, dispatched
}
