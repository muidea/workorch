// Command stateownercheck verifies the repository's exhaustive, verification-only
// inventory of state-like string types. It is intentionally outside production
// wiring: Runtime reducers must never consume this inventory as transition data.
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
	"strings"
)

type declaration struct {
	File       string
	PackageDir string
	Name       string
}

type inventoryEntry struct {
	Declaration    declaration
	Classification string
	OwnerRoot      string
	TransitionIDs  []string
	Line           int
}

type transitionOwner struct {
	Classification string
	OwnerRoot      string
	Line           int
}

var transitionClassificationsRequiringStateType = map[string]bool{
	"formal_runtime_machine":       true,
	"resource_state_machine":       true,
	"derived_projection_lifecycle": true,
}

var stateLikeSuffixes = []string{"State", "Status", "Phase", "Disposition", "Outcome", "Mode", "Lifecycle", "Stage"}

var classifications = map[string]bool{
	"authoritative_state_machine": true,
	"owner_private_state":         true,
	"immutable_outcome":           true,
	"pure_disposition":            true,
	"derived_projection":          true,
	"ephemeral_technical":         true,
	"invariant_value":             true,
}

func main() {
	root := flag.String("root", ".", "repository root")
	inventoryPath := flag.String("inventory", "scripts/state-model-owners.manifest", "state model inventory")
	transitionPath := flag.String("transitions", "scripts/state-transition-owners.manifest", "transition owner inventory")
	flag.Parse()

	if err := verify(*root, *inventoryPath, *transitionPath); err != nil {
		fmt.Fprintf(os.Stderr, "boundary contract violation: %v\n", err)
		os.Exit(1)
	}
}

func verify(root, inventoryPath, transitionPath string) error {
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
	inventory, err := readInventory(filepath.Join(root, filepath.FromSlash(inventoryPath)))
	if err != nil {
		return err
	}

	for key, entry := range inventory {
		if _, found := declarations[key]; !found {
			return fmt.Errorf("state-model inventory row %d references a missing declaration: %s", entry.Line, key)
		}
		if !classifications[entry.Classification] {
			return fmt.Errorf("state-model inventory row %d has unknown classification %q", entry.Line, entry.Classification)
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
	return nil
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
					if !ok || typeSpec.Assign.IsValid() || !isStateLikeName(typeSpec.Name.Name) {
						continue
					}
					underlying, ok := typeSpec.Type.(*ast.Ident)
					if !ok || underlying.Name != "string" {
						continue
					}
					item := declaration{File: relative, PackageDir: packageDir, Name: typeSpec.Name.Name}
					key := item.File + ":" + item.Name
					if _, duplicate := ret[key]; duplicate {
						return fmt.Errorf("duplicate state-like declaration %s", key)
					}
					ret[key] = item
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
		if len(parts) != 6 {
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
		ret[id] = transitionOwner{Classification: classification, OwnerRoot: filepath.ToSlash(owner), Line: line}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return ret, nil
}
