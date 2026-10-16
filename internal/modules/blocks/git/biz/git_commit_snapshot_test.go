package git

import (
	"bytes"
	"compress/zlib"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	gitcommon "github.com/muidea/workorch/internal/modules/blocks/git/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func gitSnapshotFixture(t *testing.T) (string, gitCommitSnapshotRecord) {
	t.Helper()
	repo := newTestRepo(t)
	for _, name := range []string{"internal/a/body.bin", "internal/b/duplicate.bin"} {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(repo, name)), 0o700); err != nil {
			t.Fatal(err)
		}
		writeFile(t, filepath.Join(repo, name), string([]byte{0, 255, 128, 10, 13, 0, 42}))
	}
	writeFile(t, filepath.Join(repo, "newline\nfile"), "raw filename\n")
	if err := os.Symlink("internal/a/body.bin", filepath.Join(repo, "link")); err != nil {
		t.Fatal(err)
	}
	runGitForTest(t, repo, "add", ".")
	runGitForTest(t, repo, "commit", "-m", "snapshot")
	head := gitOutputForTest(t, repo, "rev-parse", "HEAD")
	objects, err := NewRuntimeAdapter().readCommitObjectGraph(context.Background(), repo, head)
	if err != nil {
		t.Fatal(err)
	}
	tree := gitOutputForTest(t, repo, "rev-parse", "HEAD^{tree}")
	return repo, gitCommitSnapshotRecord{ProtocolRef: gitCommitSnapshotProtocol,
		Scope: gitCommitSnapshotScope{WorkspaceID: "ws", RunID: "run", DispatchID: "dispatch", OperationID: "commit",
			DefinitionRevision: "definition", ExecutionKey: "exec/ws/run/dispatch/commit", Fence: 1, OperationHash: "operation-hash", AuthorityHash: "authority-hash"},
		Head: head, RootTree: tree, Objects: objects}
}

func TestGitCommitSnapshotCompleteGraphMatchesIndependentGitOracle(t *testing.T) {
	repo, record := gitSnapshotFixture(t)
	if err := record.Validate(record.Scope); err != nil {
		t.Fatal(err)
	}
	oracle := map[string]bool{record.Head: true, record.RootTree: true}
	command := exec.Command("git", "ls-tree", "-r", "-t", "-z", record.Head)
	command.Dir = repo
	body, err := command.Output()
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range bytes.Split(body, []byte{0}) {
		if len(line) == 0 {
			continue
		}
		metadata, _, found := bytes.Cut(line, []byte{'\t'})
		fields := strings.Fields(string(metadata))
		if !found || len(fields) != 3 {
			t.Fatal("invalid independent ls-tree oracle")
		}
		oracle[fields[2]] = true
	}
	if len(oracle) != len(record.Objects) {
		t.Fatalf("reachable graph omissions/extras: %d != %d", len(oracle), len(record.Objects))
	}
	for _, object := range record.Objects {
		if !oracle[object.ID] {
			t.Fatalf("extra snapshot member %s", object.ID)
		}
		command := exec.Command("git", "--no-replace-objects", "cat-file", object.Kind, object.ID)
		command.Dir = repo
		actual, err := command.Output()
		if err != nil || !bytes.Equal(actual, object.Body) {
			t.Fatal("snapshot changed original binary bytes")
		}
	}
}

func TestGitCommitSnapshotArchiveRejectsIncompleteOrRehashedGraph(t *testing.T) {
	_, original := gitSnapshotFixture(t)
	encoded, _ := json.Marshal(original)
	for _, mode := range []string{"missing blob", "missing tree", "duplicate", "extra object", "changed bytes", "wrong head", "wrong root", "wrong scope", "unknown protocol"} {
		t.Run(mode, func(t *testing.T) {
			var record gitCommitSnapshotRecord
			if err := json.Unmarshal(encoded, &record); err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "missing blob", "missing tree":
				kind := "blob"
				if mode == "missing tree" {
					kind = "tree"
				}
				for i, object := range record.Objects {
					if object.Kind == kind {
						record.Objects = append(record.Objects[:i], record.Objects[i+1:]...)
						break
					}
				}
			case "duplicate":
				record.Objects = append(record.Objects, record.Objects[0])
			case "extra object":
				// The parent commit is a valid actual Git object, but is not
				// part of the captured commit's deliverable tree graph.
				repo, _ := gitSnapshotFixture(t)
				id := gitOutputForTest(t, repo, "rev-parse", "HEAD^")
				body, err := NewRuntimeAdapter().readDependencyObject(context.Background(), repo, "commit", id)
				if err != nil {
					t.Fatal(err)
				}
				record.Objects = append(record.Objects, gitSnapshotObject{Kind: "commit", ID: id, Body: []byte(body)})
			case "changed bytes":
				record.Objects[len(record.Objects)-1].Body = []byte("replacement")
			case "wrong head":
				record.Head = record.RootTree
			case "wrong root":
				record.RootTree = strings.Repeat("0", len(record.RootTree))
			case "wrong scope":
				record.Scope.Fence++
			case "unknown protocol":
				record.ProtocolRef = "future"
			}
			if err := record.Validate(original.Scope); err == nil {
				t.Fatal("self-consistent JSON/hash must not grant complete graph proof")
			}
		})
	}
}

func TestGitCommitSnapshotArchivePreservesBinaryBytesAndConcurrentDuplicate(t *testing.T) {
	_, record := gitSnapshotFixture(t)
	directory := t.TempDir()
	ctx := context.Background()
	address, err := storeGitCommitSnapshot(ctx, directory, record)
	if err != nil {
		t.Fatal(err)
	}
	var group sync.WaitGroup
	for i := 0; i < 8; i++ {
		group.Go(func() {
			duplicate, err := storeGitCommitSnapshot(ctx, directory, record)
			if err != nil || duplicate != address {
				t.Errorf("concurrent duplicate: %v", err)
			}
		})
	}
	group.Wait()
	restored, err := readGitCommitSnapshot(ctx, directory, record.Scope, address)
	if err != nil || !reflect.DeepEqual(restored, record) {
		t.Fatalf("binary archive reload drift: %v", err)
	}
	restored.Objects[0].Body[0] ^= 1
	again, err := readGitCommitSnapshot(ctx, directory, record.Scope, address)
	if err != nil || !reflect.DeepEqual(again, record) {
		t.Fatal("returned bytes alias archive")
	}
	files, err := os.ReadDir(filepath.Join(directory, "git", "commit-object-snapshots"))
	if err != nil || len(files) != 1 {
		t.Fatalf("duplicate raw archive records: %v", err)
	}
	if files[0].Name() != gitCommitSnapshotFilename(record.Scope) {
		t.Fatal("wrong archive identity")
	}
	wrong := record.Scope
	wrong.RunID = "other-run"
	if _, err := readGitCommitSnapshot(ctx, directory, wrong, address); err == nil {
		t.Fatal("cross-run raw read")
	}
	path := filepath.Join(directory, "git", "commit-object-snapshots", gitCommitSnapshotFilename(record.Scope))
	if err := os.WriteFile(path, []byte("tampered"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readGitCommitSnapshot(ctx, directory, record.Scope, address); err == nil {
		t.Fatal("raw archive tamper accepted")
	}
	if _, err := storeGitCommitSnapshot(ctx, directory, record); err == nil {
		t.Fatal("archive overwrite silently repaired tamper")
	}
}

func TestGitCommitSnapshotRejectsSymlinkedStoreAndCancellation(t *testing.T) {
	_, record := gitSnapshotFixture(t)
	directory := t.TempDir()
	if err := os.Symlink(t.TempDir(), filepath.Join(directory, "git")); err != nil {
		t.Fatal(err)
	}
	if _, err := storeGitCommitSnapshot(context.Background(), directory, record); err == nil {
		t.Fatal("symlinked owner store accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := storeGitCommitSnapshot(ctx, t.TempDir(), record); err == nil {
		t.Fatal("cancelled raw store succeeded")
	}
}

func TestGitCommitSnapshotCaptureRejectsNoncurrentHead(t *testing.T) {
	repo, record := gitSnapshotFixture(t)
	runGitForTest(t, repo, "commit", "--allow-empty", "-m", "new head")
	output := RuntimeOperationResult{Operation: gitcommon.OperationCommit, Status: "completed", RepoRoot: repo, CommitHash: record.Head}
	scope := runtimecontract.RuntimeOperationExecutionContext{WorkspaceID: record.Scope.WorkspaceID, Transaction: runtimecontract.AuthorityTransaction{DefinitionRevision: record.Scope.DefinitionRevision}}
	if _, err := NewRuntimeAdapter().commitOwnerEvidence(context.Background(), scope, output); err == nil {
		t.Fatal("old commit was labelled current HEAD at capture")
	}
}

func TestGitCommitObjectGraphRejectsSubmoduleAndOversizedBlob(t *testing.T) {
	for _, mode := range []string{"submodule", "oversized blob"} {
		t.Run(mode, func(t *testing.T) {
			repo := newTestRepo(t)
			if mode == "submodule" {
				head := gitOutputForTest(t, repo, "rev-parse", "HEAD")
				runGitForTest(t, repo, "update-index", "--add", "--cacheinfo", "160000,"+head+",submodule")
			} else {
				writeFile(t, filepath.Join(repo, "large"), strings.Repeat("x", maxOutputBytes+1))
				runGitForTest(t, repo, "add", "large")
			}
			runGitForTest(t, repo, "commit", "-m", "unsupported graph")
			head := gitOutputForTest(t, repo, "rev-parse", "HEAD")
			if graph, err := NewRuntimeAdapter().readCommitObjectGraph(context.Background(), repo, head); err == nil || graph != nil {
				t.Fatal("partial graph was presented as complete")
			}
		})
	}
}

func TestGitSnapshotTreeParserRejectsMalformedAndDuplicateMembers(t *testing.T) {
	id := bytes.Repeat([]byte{1}, 20)
	entry := append([]byte("100644 a\x00"), id...)
	for _, body := range [][]byte{[]byte("malformed"), []byte("100644 a\x00short"), append(append([]byte{}, entry...), entry...), append([]byte("100644 ../a\x00"), id...), append([]byte("unknown a\x00"), id...)} {
		if _, err := parseGitSnapshotTree(body, 20); err == nil {
			t.Fatal("invalid raw tree accepted")
		}
	}
}

func TestGitCommitSnapshotSameExecutionCannotPublishDifferentValidRecord(t *testing.T) {
	repo, original := gitSnapshotFixture(t)
	directory := t.TempDir()
	address, err := storeGitCommitSnapshot(context.Background(), directory, original)
	if err != nil {
		t.Fatal(err)
	}
	runGitForTest(t, repo, "commit", "--allow-empty", "-m", "another valid commit")
	changed := original
	changed.Head = gitOutputForTest(t, repo, "rev-parse", "HEAD")
	changed.Objects, err = NewRuntimeAdapter().readCommitObjectGraph(context.Background(), repo, changed.Head)
	if err != nil || changed.Validate(original.Scope) != nil {
		t.Fatalf("second valid graph fixture: %v", err)
	}
	if _, err := storeGitCommitSnapshot(context.Background(), directory, changed); err == nil {
		t.Fatal("same execution/fence published a second valid snapshot")
	}
	restored, err := readGitCommitSnapshot(context.Background(), directory, original.Scope, address)
	if err != nil || !reflect.DeepEqual(original, restored) {
		t.Fatal("conflicting publish changed original snapshot")
	}
	// A distinct exact execution may publish its own immutable observation.
	changed.Scope.Fence++
	if _, err := storeGitCommitSnapshot(context.Background(), directory, changed); err != nil {
		t.Fatal(err)
	}
}

func TestGitCommitSnapshotSHA256BinaryArchive(t *testing.T) {
	repo := t.TempDir()
	runGitForTest(t, repo, "init", "--object-format=sha256")
	runGitForTest(t, repo, "config", "user.email", "workorch@example.test")
	runGitForTest(t, repo, "config", "user.name", "WorkOrch Test")
	writeFile(t, filepath.Join(repo, "raw.bin"), string([]byte{0, 255, 128, 10}))
	runGitForTest(t, repo, "add", ".")
	runGitForTest(t, repo, "commit", "-m", "binary sha256 snapshot")
	_, record := gitSnapshotFixture(t)
	record.Head = gitOutputForTest(t, repo, "rev-parse", "HEAD")
	record.RootTree = gitOutputForTest(t, repo, "rev-parse", "HEAD^{tree}")
	var err error
	record.Objects, err = NewRuntimeAdapter().readCommitObjectGraph(context.Background(), repo, record.Head)
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	address, err := storeGitCommitSnapshot(context.Background(), directory, record)
	if err != nil {
		t.Fatal(err)
	}
	reloaded, err := readGitCommitSnapshot(context.Background(), directory, record.Scope, address)
	if err != nil || !reflect.DeepEqual(record, reloaded) {
		t.Fatalf("SHA256 raw archive changed: %v", err)
	}
	for _, object := range reloaded.Objects {
		if len(object.ID) != 64 {
			t.Fatal("SHA256 identity lost")
		}
	}
}

// Independent fixture writer: Git IDs use the documented object framing, not
// the production validator or dependency compiler. Trees keep original bytes.
func gitSnapshotObjectForTest(kind string, body []byte) gitSnapshotObject {
	hash := sha1.New()
	_, _ = fmt.Fprintf(hash, "%s %d\x00", kind, len(body))
	_, _ = hash.Write(body)
	return gitSnapshotObject{Kind: kind, ID: hex.EncodeToString(hash.Sum(nil)), Body: append([]byte(nil), body...)}
}

func gitSnapshotTreeEntryForTest(name string, object gitSnapshotObject) []byte {
	mode := "100644"
	if object.Kind == "tree" {
		mode = "40000"
	}
	id, _ := hex.DecodeString(object.ID)
	return append([]byte(mode+" "+name+"\x00"), id...)
}

func gitSnapshotRecordForTest(root gitSnapshotObject, objects []gitSnapshotObject) gitCommitSnapshotRecord {
	commit := gitSnapshotObjectForTest("commit", []byte("tree "+root.ID+"\nauthor Test <t@example.test> 1 +0000\ncommitter Test <t@example.test> 1 +0000\n\nsnapshot\n"))
	return gitCommitSnapshotRecord{ProtocolRef: gitCommitSnapshotProtocol,
		Scope: gitCommitSnapshotScope{WorkspaceID: "ws", RunID: "run", DispatchID: "dispatch", OperationID: "commit", DefinitionRevision: "definition",
			ExecutionKey: "execution", Fence: 1, OperationHash: "operation", AuthorityHash: "authority"},
		Head: commit.ID, RootTree: root.ID, Objects: append([]gitSnapshotObject{commit}, objects...)}
}

func gitSnapshotFlatRecordForTest(count, blobBytes, lastBytes int) gitCommitSnapshotRecord {
	objects := []gitSnapshotObject{}
	tree := []byte{}
	for i := 0; i < count; i++ {
		size := blobBytes
		if i == count-1 {
			size = lastBytes
		}
		body := bytes.Repeat([]byte{'x'}, size)
		copy(body, fmt.Sprintf("%08d", i))
		object := gitSnapshotObjectForTest("blob", body)
		objects = append(objects, object)
		tree = append(tree, gitSnapshotTreeEntryForTest(fmt.Sprintf("member-%04d", i), object)...)
	}
	root := gitSnapshotObjectForTest("tree", tree)
	objects = append(objects, root)
	return gitSnapshotRecordForTest(root, objects)
}

func TestGitCommitSnapshotCountAndTotalBytesBoundaries(t *testing.T) {
	for _, count := range []int{gitSnapshotMaxObjects - 2, gitSnapshotMaxObjects - 1} {
		record := gitSnapshotFlatRecordForTest(count, 16, 16)
		err := record.Validate(record.Scope)
		if (err == nil) != (count+2 == gitSnapshotMaxObjects) {
			t.Fatalf("complete object count boundary %d: %v", count+2, err)
		}
	}
	base := gitSnapshotFlatRecordForTest(64, maxOutputBytes, maxOutputBytes)
	overhead := 0
	for _, object := range base.Objects {
		if object.Kind != "blob" {
			overhead += len(object.Body)
		}
	}
	for _, excess := range []int{0, 1} {
		record := gitSnapshotFlatRecordForTest(64, maxOutputBytes, maxOutputBytes-overhead+excess)
		bytes := 0
		for _, object := range record.Objects {
			bytes += len(object.Body)
		}
		if bytes != gitSnapshotMaxBytes+excess {
			t.Fatal("incorrect independent byte boundary fixture")
		}
		err := record.Validate(record.Scope)
		if (err == nil) != (excess == 0) {
			t.Fatalf("complete bytes boundary %d: %v", bytes, err)
		}
		if excess != 0 {
			directory := t.TempDir()
			if _, err := storeGitCommitSnapshot(context.Background(), directory, record); err == nil {
				t.Fatal("over-budget archive was published")
			}
			if _, err := os.Stat(filepath.Join(directory, "git")); !os.IsNotExist(err) {
				t.Fatal("invalid archive created owner storage")
			}
		}
	}
}

func gitSnapshotInstallObjectsForTest(t *testing.T, repo string, record gitCommitSnapshotRecord) {
	t.Helper()
	for _, object := range record.Objects {
		var encoded bytes.Buffer
		writer := zlib.NewWriter(&encoded)
		if _, err := fmt.Fprintf(writer, "%s %d\x00", object.Kind, len(object.Body)); err != nil {
			t.Fatal(err)
		}
		if _, err := writer.Write(object.Body); err != nil {
			t.Fatal(err)
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		directory := filepath.Join(repo, ".git", "objects", object.ID[:2])
		if err := os.MkdirAll(directory, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(directory, object.ID[2:]), encoded.Bytes(), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestGitCommitObjectGraphCaptureCountAndBytesLimitsReturnNoPartialProof(t *testing.T) {
	for _, dimension := range []string{"count", "bytes"} {
		for _, excess := range []int{0, 1} {
			t.Run(fmt.Sprintf("%s-excess-%d", dimension, excess), func(t *testing.T) {
				record := gitSnapshotFlatRecordForTest(gitSnapshotMaxObjects-2+excess, 16, 16)
				if dimension == "bytes" {
					base := gitSnapshotFlatRecordForTest(64, maxOutputBytes, maxOutputBytes)
					overhead := len(base.Objects[0].Body) + len(base.Objects[len(base.Objects)-1].Body)
					record = gitSnapshotFlatRecordForTest(64, maxOutputBytes, maxOutputBytes-overhead+excess)
				}
				repo := newTestRepo(t)
				gitSnapshotInstallObjectsForTest(t, repo, record)
				adapter := NewRuntimeAdapter()
				// Fix the fixture budget explicitly: these tests distinguish
				// storage limits from caller-selected shorter deadlines.
				adapter.timeout = 30 * time.Second
				graph, err := adapter.readCommitObjectGraph(context.Background(), repo, record.Head)
				if excess == 0 {
					if err != nil || len(graph) != len(record.Objects) {
						t.Fatalf("actual capture exact %s limit: %v", dimension, err)
					}
				} else {
					message := "object count limit"
					if dimension == "bytes" {
						message = "object bytes limit"
					}
					if err == nil || !strings.Contains(err.Error(), message) || graph != nil {
						t.Fatalf("actual over-%s capture did not reject complete graph: %v", dimension, err)
					}
				}
			})
		}
	}
}

func TestGitCommitSnapshotSharedTreeDepthChecksEveryReachablePath(t *testing.T) {
	for _, excess := range []int{0, 1} {
		t.Run(fmt.Sprintf("excess-%d", excess), func(t *testing.T) {
			blob := gitSnapshotObjectForTest("blob", []byte("leaf"))
			shared := gitSnapshotObjectForTest("tree", gitSnapshotTreeEntryForTest("leaf", blob))
			objects := []gitSnapshotObject{blob, shared}
			deep := shared
			for i := 0; i < gitSnapshotMaxDepth-3+excess; i++ {
				deep = gitSnapshotObjectForTest("tree", gitSnapshotTreeEntryForTest("nested", deep))
				objects = append(objects, deep)
			}
			// Visit the shared subtree shallowly first, then reach it by a
			// deeper path. A seen-ID-only walk misses the deepest blob.
			body := append(gitSnapshotTreeEntryForTest("a-shallow", shared), gitSnapshotTreeEntryForTest("z-deep", deep)...)
			root := gitSnapshotObjectForTest("tree", body)
			objects = append(objects, root)
			record := gitSnapshotRecordForTest(root, objects)
			if err := record.Validate(record.Scope); (err == nil) != (excess == 0) {
				t.Fatalf("archive depth guard: %v", err)
			}
			repo := newTestRepo(t)
			for _, object := range record.Objects {
				command := exec.Command("git", "hash-object", "-w", "-t", object.Kind, "--stdin")
				command.Dir, command.Stdin = repo, bytes.NewReader(object.Body)
				output, err := command.CombinedOutput()
				if err != nil || strings.TrimSpace(string(output)) != object.ID {
					t.Fatalf("independent Git object fixture: %v %s", err, output)
				}
			}
			graph, err := NewRuntimeAdapter().readCommitObjectGraph(context.Background(), repo, record.Head)
			if (err == nil) != (excess == 0) || (err != nil && graph != nil) {
				t.Fatalf("actual capture depth guard/partial result: %v", err)
			}
		})
	}
}

// Inject cancellation at the archive's explicit publication checkpoints,
// without adding fault switches or test callbacks to the production owner.
type gitSnapshotCheckpointContext struct {
	context.Context
	mu       sync.Mutex
	checks   int
	cancelAt int
	cancel   context.CancelFunc
}

func newGitSnapshotCheckpointContext(checkpoint int) *gitSnapshotCheckpointContext {
	ctx, cancel := context.WithCancel(context.Background())
	return &gitSnapshotCheckpointContext{Context: ctx, cancelAt: checkpoint, cancel: cancel}
}

func (c *gitSnapshotCheckpointContext) Err() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.checks++
	if c.checks >= c.cancelAt {
		c.cancel()
	}
	return c.Context.Err()
}

func TestGitCommitSnapshotPublishInterruptionRecoversExactOriginal(t *testing.T) {
	_, record := gitSnapshotFixture(t)
	for _, checkpoint := range []int{2, 3} {
		t.Run(fmt.Sprintf("checkpoint-%d", checkpoint), func(t *testing.T) {
			directory := t.TempDir()
			ctx := newGitSnapshotCheckpointContext(checkpoint)
			if _, err := storeGitCommitSnapshot(ctx, directory, record); err != context.Canceled {
				t.Fatalf("expected cancellation checkpoint: %v", err)
			}
			files, err := os.ReadDir(filepath.Join(directory, "git", "commit-object-snapshots"))
			if err != nil || len(files) != checkpoint-2 {
				t.Fatalf("publication boundary or temporary cleanup drift: %v %v", files, err)
			}
			// Before publication there is no complete record. After durable
			// publication a lost return must replay the same scope/body.
			address, err := storeGitCommitSnapshot(context.Background(), directory, record)
			if err != nil {
				t.Fatal(err)
			}
			duplicateContext := newGitSnapshotCheckpointContext(2)
			if failedAddress, err := storeGitCommitSnapshot(duplicateContext, directory, record); err != context.Canceled || failedAddress != (runtimecontract.RuntimeEvidenceContentAddress{}) {
				t.Fatal("cancelled duplicate return advertised a successful proof")
			}
			reloaded, err := readGitCommitSnapshot(context.Background(), directory, record.Scope, address)
			if err != nil || !reflect.DeepEqual(reloaded, record) {
				t.Fatalf("snapshot checkpoint recovery changed original: %v", err)
			}
			files, err = os.ReadDir(filepath.Join(directory, "git", "commit-object-snapshots"))
			if err != nil || len(files) != 1 {
				t.Fatal("recovery published duplicate archive or leaked temporary")
			}
		})
	}
}

func TestGitCommitSnapshotArchiveRejectsUnsafeFileAndDirectoryWithoutRepair(t *testing.T) {
	_, record := gitSnapshotFixture(t)
	for _, mode := range []string{"wide directory", "wide file", "symlink file"} {
		t.Run(mode, func(t *testing.T) {
			directory := t.TempDir()
			address, err := storeGitCommitSnapshot(context.Background(), directory, record)
			if err != nil {
				t.Fatal(err)
			}
			ownerDirectory := filepath.Join(directory, "git", "commit-object-snapshots")
			path := filepath.Join(ownerDirectory, gitCommitSnapshotFilename(record.Scope))
			switch mode {
			case "wide directory":
				if err := os.Chmod(ownerDirectory, 0o755); err != nil {
					t.Fatal(err)
				}
			case "wide file":
				if err := os.Chmod(path, 0o644); err != nil {
					t.Fatal(err)
				}
			case "symlink file":
				// Preserve the actual original and put a pointer in its place;
				// even an in-root link cannot grant a raw-read/write boundary.
				if err := os.Rename(path, path+".original"); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(path+".original", path); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := readGitCommitSnapshot(context.Background(), directory, record.Scope, address); err == nil {
				t.Fatal("unsafe owner archive read granted")
			}
			if _, err := storeGitCommitSnapshot(context.Background(), directory, record); err == nil {
				t.Fatal("unsafe owner archive was silently repaired or reused")
			}
		})
	}
}
