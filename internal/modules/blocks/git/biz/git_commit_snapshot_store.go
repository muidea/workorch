package git

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"unicode"
	"unicode/utf8"

	gitcommon "github.com/muidea/workorch/internal/modules/blocks/git/pkg/common"
	gitevents "github.com/muidea/workorch/internal/modules/blocks/git/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const gitCommitSnapshotProtocol = gitevents.RuntimeCommitSnapshotProtocol
const gitCommitSnapshotMaxRecordBytes = 2*gitSnapshotMaxBytes + 1024*1024

// Commit-time observation, not terminal currentness or an acceptance result.
// Raw immutable bytes stay in the Git-owned restricted store. Only the content
// address and actual object dependencies cross the existing receipt boundary.
type gitCommitSnapshotScope struct {
	WorkspaceID        common.WorkspaceID
	RunID              common.RunID
	DispatchID         string
	OperationID        string
	DefinitionRevision string
	ExecutionKey       string
	Fence              uint64
	OperationHash      string
	AuthorityHash      string
}

type gitCommitSnapshotRecord struct {
	ProtocolRef string
	Scope       gitCommitSnapshotScope
	Head        string
	RootTree    string
	Objects     []gitSnapshotObject
}

func gitCommitSnapshotScopeForExecution(context runtimecontract.RuntimeOperationExecutionContext) gitCommitSnapshotScope {
	return gitCommitSnapshotScope{WorkspaceID: context.WorkspaceID, RunID: context.RunID,
		DispatchID: context.DispatchID, OperationID: context.OperationID, DefinitionRevision: context.Transaction.DefinitionRevision,
		ExecutionKey: context.Execution.ExecutionKey, Fence: context.Execution.Fence,
		OperationHash: context.Execution.OperationHash, AuthorityHash: context.Execution.AuthorityHash}
}

func (scope gitCommitSnapshotScope) Validate() error {
	for _, value := range []string{string(scope.WorkspaceID), string(scope.RunID), scope.DispatchID, scope.OperationID,
		scope.DefinitionRevision, scope.ExecutionKey, scope.OperationHash, scope.AuthorityHash} {
		if value == "" || strings.TrimSpace(value) != value || len(value) > 2048 || !utf8.ValidString(value) || strings.IndexFunc(value, unicode.IsControl) >= 0 {
			return common.ErrInvalidArgument
		}
	}
	if scope.Fence == 0 {
		return common.ErrInvalidArgument
	}
	return nil
}

// Validate independently walks the archived graph, requiring every reachable
// member and rejecting duplicate/extra objects. Rehashing JSON cannot conceal
// a missing blob, a substituted body or an incomplete recursive tree.
func (record gitCommitSnapshotRecord) Validate(scope gitCommitSnapshotScope) error {
	if scope.Validate() != nil || record.Scope != scope || record.ProtocolRef != gitCommitSnapshotProtocol || !gitDependencyObjectID(record.Head) ||
		len(record.Objects) == 0 || len(record.Objects) > gitSnapshotMaxObjects {
		return common.ErrConflict
	}
	objects := map[string]gitSnapshotObject{}
	size := 0
	for _, object := range record.Objects {
		if _, duplicate := objects[object.ID]; duplicate || len(object.ID) != len(record.Head) {
			return common.ErrConflict
		}
		if err := validateGitSnapshotObject(object); err != nil {
			return err
		}
		objects[object.ID] = object
		size += len(object.Body)
	}
	if size > gitSnapshotMaxBytes {
		return common.ErrConflict
	}
	visited := map[string]string{}
	heights := map[string]int{}
	var visit func(string, string, int) error
	visit = func(kind, id string, depth int) error {
		if depth > gitSnapshotMaxDepth {
			return common.ErrConflict
		}
		if previous, found := visited[id]; found {
			if previous != kind || depth+heights[id] > gitSnapshotMaxDepth {
				return common.ErrConflict
			}
			return nil
		}
		object, found := objects[id]
		if !found || object.Kind != kind {
			return common.ErrConflict
		}
		visited[id] = kind
		switch kind {
		case "commit":
			tree, err := gitSnapshotRootTree(object.Body, len(record.Head))
			if err != nil || tree != record.RootTree {
				return common.ErrConflict
			}
			if err := visit("tree", tree, depth+1); err != nil {
				return err
			}
			heights[id] = heights[tree] + 1
		case "tree":
			members, err := parseGitSnapshotTree(object.Body, len(record.Head)/2)
			if err != nil {
				return err
			}
			for _, member := range members {
				if err := visit(member.Kind, member.ID, depth+1); err != nil {
					return err
				}
				if height := heights[member.ID] + 1; height > heights[id] {
					heights[id] = height
				}
			}
		}
		return nil
	}
	if err := visit("commit", record.Head, 0); err != nil {
		return err
	}
	if len(visited) != len(objects) {
		return common.ErrConflict
	}
	return nil
}

func (a *RuntimeAdapter) commitOwnerEvidence(ctx context.Context, scope runtimecontract.RuntimeOperationExecutionContext, output RuntimeOperationResult) (runtimecontract.RuntimeOperationEvidence, error) {
	evidence := output.Evidence
	if output.Operation != gitcommon.OperationCommit || output.Status != "completed" || output.ZeroWork {
		return evidence, nil
	}
	if err := validateGitCommitDependencyScope(scope); err != nil {
		return evidence, err
	}
	if output.RepoRoot == "" || !gitDependencyObjectID(output.CommitHash) {
		return evidence, fmt.Errorf("Git commit snapshot requires actual repository and commit identity")
	}
	// Enclose the graph read and its two HEAD observations in one bounded
	// owner call. HEAD is sampled, never manufactured from the output label.
	timeout := a.timeout
	if timeout <= 0 {
		timeout = DefaultCommandTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	head, err := a.gitOutput(ctx, output.RepoRoot, a.timeout, "--no-replace-objects", "rev-parse", "--verify", "HEAD")
	if err != nil {
		return evidence, fmt.Errorf("Git commit snapshot HEAD observation: %w", err)
	}
	if strings.TrimSuffix(head, "\n") != output.CommitHash {
		return evidence, fmt.Errorf("Git commit snapshot HEAD is not the actual committed object")
	}
	objects, err := a.readCommitObjectGraph(ctx, output.RepoRoot, output.CommitHash)
	if err != nil {
		return evidence, err
	}
	dependencies, err := gitCommitObjectDependencies(scope.Transaction.DefinitionRevision, string(scope.WorkspaceID), objects)
	if err != nil {
		return evidence, err
	}
	headAfter, err := a.gitOutput(ctx, output.RepoRoot, a.timeout, "--no-replace-objects", "rev-parse", "--verify", "HEAD")
	if err != nil {
		return evidence, fmt.Errorf("Git commit snapshot final HEAD observation: %w", err)
	}
	if headAfter != head {
		return evidence, fmt.Errorf("Git commit snapshot HEAD changed during capture")
	}
	evidence.OwnerRevisionSet = dependencies
	if scope.WorkspaceInternalDirectory == "" {
		// Adapter-only callers have no owner archive location. Dependencies
		// still prove the actual reads, but no raw snapshot address is claimed.
		return evidence, nil
	}
	tree, err := gitSnapshotRootTree(objects[0].Body, len(output.CommitHash))
	if err != nil {
		return evidence, err
	}
	record := gitCommitSnapshotRecord{ProtocolRef: gitCommitSnapshotProtocol, Scope: gitCommitSnapshotScopeForExecution(scope), Head: output.CommitHash, RootTree: tree, Objects: objects}
	address, err := storeGitCommitSnapshot(ctx, scope.WorkspaceInternalDirectory, record)
	if err != nil {
		return evidence, err
	}
	evidence.ContentHash, evidence.ContentAddress = address.Digest, &address
	evidence.OwnerSnapshot = &runtimecontract.RuntimeOwnerSnapshotIdentity{ProtocolRef: runtimecontract.RuntimeTerminalOwnerSnapshotProtocol, SnapshotID: record.Head, RootSnapshotID: record.RootTree}
	evidence.EvidenceRefs = append(append([]string(nil), evidence.EvidenceRefs...), address.Ref)
	return evidence, nil
}

func openGitCommitSnapshotDirectory(internalDirectory string, create bool) (*os.Root, error) {
	if !filepath.IsAbs(internalDirectory) || filepath.Clean(internalDirectory) != internalDirectory {
		return nil, common.ErrInvalidArgument
	}
	info, err := os.Lstat(internalDirectory)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, common.ErrInvalidArgument
	}
	root, err := os.OpenRoot(internalDirectory)
	if err != nil {
		return nil, err
	}
	opened, err := root.Stat(".")
	if err != nil || !os.SameFile(info, opened) {
		_ = root.Close()
		return nil, common.ErrConflict
	}
	for _, name := range []string{"git", "commit-object-snapshots"} {
		if create {
			if err := root.Mkdir(name, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
				_ = root.Close()
				return nil, err
			}
			if err := syncGitSnapshotDirectory(root); err != nil {
				_ = root.Close()
				return nil, err
			}
		}
		info, err := root.Lstat(name)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 {
			_ = root.Close()
			return nil, common.ErrConflict
		}
		next, err := root.OpenRoot(name)
		_ = root.Close()
		if err != nil {
			return nil, err
		}
		opened, err := next.Stat(".")
		if err != nil || !os.SameFile(info, opened) || !opened.IsDir() || opened.Mode().Perm()&0o077 != 0 {
			_ = next.Close()
			return nil, common.ErrConflict
		}
		root = next
	}
	return root, nil
}

func syncGitSnapshotDirectory(root *os.Root) error {
	file, err := root.Open(".")
	if err != nil {
		return err
	}
	err = file.Sync()
	closeErr := file.Close()
	if err != nil {
		return err
	}
	return closeErr
}

func readGitSnapshotBytes(root *os.Root, name string) ([]byte, error) {
	info, err := root.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 || info.Size() > gitCommitSnapshotMaxRecordBytes {
		return nil, common.ErrConflict
	}
	file, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) || !opened.Mode().IsRegular() || opened.Mode().Perm()&0o077 != 0 || opened.Size() > gitCommitSnapshotMaxRecordBytes {
		return nil, common.ErrConflict
	}
	body, err := io.ReadAll(io.LimitReader(file, gitCommitSnapshotMaxRecordBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > gitCommitSnapshotMaxRecordBytes {
		return nil, fmt.Errorf("Git snapshot record exceeds complete storage limit %d", gitCommitSnapshotMaxRecordBytes)
	}
	return body, nil
}

func gitCommitSnapshotFilename(scope gitCommitSnapshotScope) string {
	body, _ := json.Marshal(scope)
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:]) + ".json"
}

func storeGitCommitSnapshot(ctx context.Context, directory string, record gitCommitSnapshotRecord) (runtimecontract.RuntimeEvidenceContentAddress, error) {
	if err := ctx.Err(); err != nil {
		return runtimecontract.RuntimeEvidenceContentAddress{}, err
	}
	if err := record.Validate(record.Scope); err != nil {
		return runtimecontract.RuntimeEvidenceContentAddress{}, err
	}
	body, err := json.Marshal(record)
	if err != nil || len(body) > gitCommitSnapshotMaxRecordBytes {
		return runtimecontract.RuntimeEvidenceContentAddress{}, common.ErrInvalidArgument
	}
	digest := sha256.Sum256(body)
	identity := hex.EncodeToString(digest[:])
	address := runtimecontract.RuntimeEvidenceContentAddress{Algorithm: runtimecontract.RuntimeEvidenceDigestSHA256, Digest: identity, Ref: gitCommitSnapshotProtocol + "/" + identity}
	root, err := openGitCommitSnapshotDirectory(directory, true)
	if err != nil {
		return runtimecontract.RuntimeEvidenceContentAddress{}, err
	}
	defer root.Close()
	// One exact execution/fence has one immutable archive. The content
	// address binds its body; a retry cannot publish a second snapshot for
	// the same execution under a different digest.
	name := gitCommitSnapshotFilename(record.Scope)
	if existing, err := readGitSnapshotBytes(root, name); err == nil {
		if !bytes.Equal(existing, body) {
			return runtimecontract.RuntimeEvidenceContentAddress{}, common.ErrConflict
		}
		if err := syncGitSnapshotDirectory(root); err != nil {
			return runtimecontract.RuntimeEvidenceContentAddress{}, err
		}
		if err := ctx.Err(); err != nil {
			return runtimecontract.RuntimeEvidenceContentAddress{}, err
		}
		return address, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return runtimecontract.RuntimeEvidenceContentAddress{}, err
	}
	temporary := ".snapshot-" + rand.Text()
	file, err := root.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return runtimecontract.RuntimeEvidenceContentAddress{}, err
	}
	defer func() { _ = root.Remove(temporary) }()
	n, writeErr := file.Write(body)
	if writeErr == nil && n != len(body) {
		writeErr = io.ErrShortWrite
	}
	if writeErr == nil {
		writeErr = file.Sync()
	}
	closeErr := file.Close()
	if writeErr != nil {
		return runtimecontract.RuntimeEvidenceContentAddress{}, writeErr
	}
	if closeErr != nil {
		return runtimecontract.RuntimeEvidenceContentAddress{}, closeErr
	}
	if err := ctx.Err(); err != nil {
		return runtimecontract.RuntimeEvidenceContentAddress{}, err
	}
	if err := root.Link(temporary, name); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return runtimecontract.RuntimeEvidenceContentAddress{}, err
		}
		existing, readErr := readGitSnapshotBytes(root, name)
		if readErr != nil || !bytes.Equal(existing, body) {
			return runtimecontract.RuntimeEvidenceContentAddress{}, common.ErrConflict
		}
	}
	if err := root.Remove(temporary); err != nil {
		return runtimecontract.RuntimeEvidenceContentAddress{}, err
	}
	if err := syncGitSnapshotDirectory(root); err != nil {
		return runtimecontract.RuntimeEvidenceContentAddress{}, err
	}
	if err := ctx.Err(); err != nil {
		return runtimecontract.RuntimeEvidenceContentAddress{}, err
	}
	return address, nil
}

func readGitCommitSnapshot(ctx context.Context, directory string, scope gitCommitSnapshotScope, address runtimecontract.RuntimeEvidenceContentAddress) (gitCommitSnapshotRecord, error) {
	if ctx.Err() != nil {
		return gitCommitSnapshotRecord{}, ctx.Err()
	}
	if scope.Validate() != nil || address.Validate() != nil || !reflect.DeepEqual(address, address.Normalized()) || address.Ref != gitCommitSnapshotProtocol+"/"+address.Digest {
		return gitCommitSnapshotRecord{}, common.ErrInvalidArgument
	}
	root, err := openGitCommitSnapshotDirectory(directory, false)
	if err != nil {
		return gitCommitSnapshotRecord{}, err
	}
	defer root.Close()
	body, err := readGitSnapshotBytes(root, gitCommitSnapshotFilename(scope))
	if err != nil {
		return gitCommitSnapshotRecord{}, err
	}
	digest := sha256.Sum256(body)
	if hex.EncodeToString(digest[:]) != address.Digest {
		return gitCommitSnapshotRecord{}, common.ErrConflict
	}
	var record gitCommitSnapshotRecord
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&record); err != nil {
		return gitCommitSnapshotRecord{}, err
	}
	canonical, err := json.Marshal(record)
	if err != nil || !bytes.Equal(body, canonical) {
		return gitCommitSnapshotRecord{}, common.ErrConflict
	}
	if err := record.Validate(scope); err != nil {
		return gitCommitSnapshotRecord{}, err
	}
	if err := ctx.Err(); err != nil {
		return gitCommitSnapshotRecord{}, err
	}
	return record, nil
}
