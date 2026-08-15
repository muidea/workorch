package workspaceviewcore

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
)

func TestStoreSplitKeepsOneStateAndRevisionAuthority(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read workspaceviewcore: %v", err)
	}
	storeTypes := 0
	mutexFields := 0
	entryRevisionAuthorities := 0
	globalRevisionAuthorities := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "store") || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		file, parseErr := parser.ParseFile(token.NewFileSet(), entry.Name(), nil, 0)
		if parseErr != nil {
			t.Fatalf("parse %s: %v", entry.Name(), parseErr)
		}
		for _, declaration := range file.Decls {
			switch value := declaration.(type) {
			case *ast.GenDecl:
				for _, specification := range value.Specs {
					typeSpec, ok := specification.(*ast.TypeSpec)
					if !ok || typeSpec.Name.Name != "Store" {
						continue
					}
					storeTypes++
					structure, ok := typeSpec.Type.(*ast.StructType)
					if !ok {
						t.Fatal("Store is not a struct")
					}
					for _, field := range structure.Fields.List {
						selector, ok := field.Type.(*ast.SelectorExpr)
						if ok && selector.Sel.Name == "RWMutex" {
							mutexFields++
						}
					}
				}
			case *ast.FuncDecl:
				switch value.Name.Name {
				case "advanceEntryRevision":
					entryRevisionAuthorities++
				case "advanceGlobalRevision":
					globalRevisionAuthorities++
				}
			}
		}
	}
	if storeTypes != 1 || mutexFields != 1 {
		t.Fatalf("Store authority declarations = types:%d mutexes:%d, want 1/1", storeTypes, mutexFields)
	}
	if entryRevisionAuthorities != 1 || globalRevisionAuthorities != 1 {
		t.Fatalf("revision authority declarations = entry:%d global:%d, want 1/1", entryRevisionAuthorities, globalRevisionAuthorities)
	}
}
