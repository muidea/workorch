package integration_test

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const implementationWorkFullflowCommitPrefix = "workorch://git/commit/"

// 只检查已由 aggregate 反证的 Git commit receipt；此 helper 不封存 authority。
func implementationWorkFullflowCommitReceiptRef(receipt runtimecontract.RuntimeOperationReceipt, operationID string) (string, error) {
	if operationID == "" || receipt.OperationID != operationID || receipt.ExecutionKey == "" || receipt.Fence == 0 || receipt.Outcome != runtimecontract.RuntimeOperationOutcomeAccepted || receipt.ObservedEffectScope != runtimecontract.RuntimeEffectWorkspaceDurable || !receipt.Changed || receipt.Evidence.ZeroWork || receipt.Evidence.ExitCode == nil || *receipt.Evidence.ExitCode != 0 || !slices.Contains(receipt.Evidence.EvidenceTags, "git-operation:commit") || slices.Contains(receipt.Evidence.EvidenceTags, "git-output-truncated") {
		return "", fmt.Errorf("缺少 exact accepted durable Git commit receipt")
	}
	ref := ""
	for _, candidate := range receipt.Evidence.EvidenceRefs {
		if !strings.HasPrefix(candidate, implementationWorkFullflowCommitPrefix) {
			continue
		}
		if ref != "" || !implementationWorkFullflowCommitIDValid(strings.TrimPrefix(candidate, implementationWorkFullflowCommitPrefix)) {
			return "", fmt.Errorf("commit 引用重复或非完整 canonical object ID")
		}
		ref = candidate
	}
	if ref == "" {
		return "", fmt.Errorf("Git commit receipt 无 commit 引用")
	}
	return ref, nil
}

func implementationWorkFullflowCommitIDValid(value string) bool {
	if (len(value) != 40 && len(value) != 64) || strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

// 只读测试自有交付目录：完整 commit/tree/blob 与当前文件逐 bytes 核对。
// 不执行交付源码、不读取 live 数据、不向 Runtime 写入 Git 观察或验收 credit。
func implementationWorkFullflowVerifyCommitTree(directory, ref string, required map[string]string, reviewed map[string]string) error {
	commit := strings.TrimPrefix(ref, implementationWorkFullflowCommitPrefix)
	if !strings.HasPrefix(ref, implementationWorkFullflowCommitPrefix) || !implementationWorkFullflowCommitIDValid(commit) {
		return fmt.Errorf("非 canonical commit 引用")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	objectType, err := implementationWorkFullflowGitRead(ctx, directory, "cat-file", "-t", commit)
	if err != nil || string(objectType) != "commit\n" {
		return fmt.Errorf("receipt 引用不是存在的 commit: %v", err)
	}
	head, err := implementationWorkFullflowGitRead(ctx, directory, "rev-parse", "--verify", "HEAD")
	if err != nil || string(head) != commit+"\n" {
		return fmt.Errorf("最终 HEAD 与 owner commit receipt 不同: %v", err)
	}
	listing, err := implementationWorkFullflowGitRead(ctx, directory, "ls-tree", "-r", "-z", "--full-tree", commit)
	if err != nil {
		return err
	}
	type member struct{ mode, blob string }
	tree := map[string]member{}
	for _, raw := range bytes.Split(listing, []byte{0}) {
		if len(raw) == 0 {
			continue
		}
		parts := bytes.SplitN(raw, []byte{'\t'}, 2)
		if len(parts) != 2 {
			return fmt.Errorf("非法 tree member")
		}
		fields := strings.Fields(string(parts[0]))
		path := string(parts[1])
		if len(fields) != 3 || (fields[0] != "100644" && fields[0] != "100755") || fields[1] != "blob" || !implementationWorkFullflowCommitIDValid(fields[2]) || !implementationWorkFullflowPathValid(path) {
			return fmt.Errorf("不支持或越界的 tree member %q", path)
		}
		if _, exists := tree[path]; exists || len(tree) >= 64 {
			return fmt.Errorf("tree 重复或超出 64 members")
		}
		tree[path] = member{fields[0], fields[2]}
	}
	for path := range required {
		if _, exists := tree[path]; !exists {
			return fmt.Errorf("commit 缺交付文件 %q", path)
		}
	}
	for path := range reviewed {
		if _, exists := tree[path]; !exists {
			return fmt.Errorf("commit 缺已评审输入 %q", path)
		}
	}
	seen := map[string]bool{}
	err = filepath.WalkDir(directory, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == directory {
			return nil
		}
		rel, err := filepath.Rel(directory, path)
		if err != nil {
			return err
		}
		if rel == ".git" {
			if !entry.IsDir() {
				return fmt.Errorf("测试交付仓库 .git 非目录")
			}
			return filepath.SkipDir
		}
		if entry.IsDir() {
			return nil
		}
		rel = filepath.ToSlash(rel)
		m, exists := tree[rel]
		info, err := entry.Info()
		if err != nil || !exists || !info.Mode().IsRegular() || info.Size() > 1<<20 {
			return fmt.Errorf("未提交或不支持的交付 member %q: %v", rel, err)
		}
		if (info.Mode().Perm()&0111 != 0) != (m.mode == "100755") {
			return fmt.Errorf("tree 与交付 executable mode 不同: %q", rel)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		blob, err := implementationWorkFullflowGitRead(ctx, directory, "cat-file", "blob", m.blob)
		if err != nil || !bytes.Equal(blob, data) {
			return fmt.Errorf("commit blob 与交付 bytes 不同: %q: %v", rel, err)
		}
		if expected, requiredMember := required[rel]; requiredMember && !bytes.Equal(blob, []byte(expected)) {
			return fmt.Errorf("commit 中交付文件不符合最终完整 bytes: %q", rel)
		}
		if hash, reviewedInput := reviewed[rel]; reviewedInput && runtimecontract.RuntimeValidationInputContentHash(string(blob)) != hash {
			return fmt.Errorf("commit 中已评审输入 hash 不同: %q", rel)
		}
		seen[rel] = true
		return nil
	})
	if err != nil {
		return err
	}
	if len(seen) != len(tree) {
		return fmt.Errorf("commit 含不在最终交付目录的 member")
	}
	return nil
}

func implementationWorkFullflowPathValid(path string) bool {
	return path != "" && path != "." && filepath.ToSlash(filepath.Clean(path)) == path && !filepath.IsAbs(path) && path != ".." && !strings.HasPrefix(path, "../") && path != ".git" && !strings.HasPrefix(path, ".git/") && !strings.ContainsAny(path, "\x00\r\n")
}

// 读取原 stdout，不 trim/sanitize；环境只排除 Git 外部仓库/config 注入。
func implementationWorkFullflowGitRead(ctx context.Context, directory string, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, "git", args...)
	command.Dir = directory
	for _, variable := range os.Environ() {
		key, _, _ := strings.Cut(variable, "=")
		if !strings.HasPrefix(key, "GIT_") {
			command.Env = append(command.Env, variable)
		}
	}
	command.Env = append(command.Env, "GIT_CONFIG_SYSTEM=/dev/null", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1", "GIT_TERMINAL_PROMPT=0", "GIT_NO_REPLACE_OBJECTS=1")
	var stdout, stderr implementationWorkFullflowGitBuffer
	command.Stdout, command.Stderr = &stdout, &stderr
	if err := command.Run(); err != nil {
		if stdout.exceeded || stderr.exceeded {
			return nil, fmt.Errorf("只读 Git %v 已知失败: %w；stdout/stderr 超过 1MiB，诊断未完整保存", args, err)
		}
		return nil, fmt.Errorf("只读 Git %v 失败: %w: %s", args, err, stderr.buf.Bytes())
	}
	if stdout.exceeded || stderr.exceeded {
		return nil, fmt.Errorf("Git 读取超过显式 1MiB 边界，结果不作为完整证据")
	}
	return stdout.buf.Bytes(), nil
}

// 持续接收但只保存显式边界内的原 bytes；超界不得提交完整证据。
type implementationWorkFullflowGitBuffer struct {
	buf      bytes.Buffer
	exceeded bool
}

func (b *implementationWorkFullflowGitBuffer) Write(data []byte) (int, error) {
	n := len(data)
	remaining := 1<<20 - b.buf.Len()
	if n > remaining {
		b.exceeded = true
		data = data[:remaining]
	}
	_, err := b.buf.Write(data)
	return n, err
}

// 以下只校准证据断言，不产生 Runtime receipt 或正式 Result。
func TestImplementationWorkCommitEvidenceReadCapacityIsExplicit(t *testing.T) {
	var buffer implementationWorkFullflowGitBuffer
	raw := []byte("  synthetic\r\n\x00\xff\n")
	if n, err := buffer.Write(raw); err != nil || n != len(raw) || !bytes.Equal(buffer.buf.Bytes(), raw) || buffer.exceeded {
		t.Fatal("边界内原 bytes 被改写")
	}
	large := bytes.Repeat([]byte{'z'}, 1<<20)
	if n, err := buffer.Write(large); err != nil || n != len(large) || buffer.buf.Len() != 1<<20 || !buffer.exceeded {
		t.Fatal("超界未明确标记或保存容量无界")
	}
	if n, err := buffer.Write(raw); err != nil || n != len(raw) || buffer.buf.Len() != 1<<20 || !buffer.exceeded {
		t.Fatal("达到上限后仍扩大或停止排空输入")
	}
}

func TestImplementationWorkCommitEvidenceRequiresExactAcceptedReceipt(t *testing.T) {
	exit := int64(0)
	ref := implementationWorkFullflowCommitPrefix + strings.Repeat("a", 40)
	base := runtimecontract.RuntimeOperationReceipt{
		OperationID: "commit", ExecutionKey: "synthetic/exact", Fence: 1,
		Outcome: runtimecontract.RuntimeOperationOutcomeAccepted, Changed: true,
		ObservedEffectScope: runtimecontract.RuntimeEffectWorkspaceDurable,
		Evidence:            runtimecontract.RuntimeOperationEvidence{ExitCode: &exit, EvidenceTags: []string{"git-operation:commit"}, EvidenceRefs: []string{ref}},
	}
	for _, scenario := range []string{"exact", "wrong-operation", "wrong-key", "zero-fence", "rejected", "no-op", "not-durable", "no-change", "zero-work", "nonzero-exit", "missing-exit", "truncated", "tag-substring", "missing-commit", "duplicate", "abbreviated", "invalid-hex", "uppercase"} {
		t.Run(scenario, func(t *testing.T) {
			receipt := base
			receipt.Evidence.EvidenceTags = append([]string(nil), base.Evidence.EvidenceTags...)
			receipt.Evidence.EvidenceRefs = append([]string(nil), base.Evidence.EvidenceRefs...)
			switch scenario {
			case "wrong-operation":
				receipt.OperationID = "other"
			case "wrong-key":
				receipt.ExecutionKey = ""
			case "zero-fence":
				receipt.Fence = 0
			case "rejected":
				receipt.Outcome = runtimecontract.RuntimeOperationOutcomeRejected
			case "no-op":
				receipt.Outcome = runtimecontract.RuntimeOperationOutcomeNoOp
			case "not-durable":
				receipt.ObservedEffectScope = runtimecontract.RuntimeEffectNone
			case "no-change":
				receipt.Changed = false
			case "zero-work":
				receipt.Evidence.ZeroWork = true
			case "nonzero-exit":
				failed := int64(1)
				receipt.Evidence.ExitCode = &failed
			case "missing-exit":
				receipt.Evidence.ExitCode = nil
			case "truncated":
				receipt.Evidence.EvidenceTags = append(receipt.Evidence.EvidenceTags, "git-output-truncated")
			case "tag-substring":
				receipt.Evidence.EvidenceTags = []string{"not-git-operation:commit"}
			case "missing-commit":
				receipt.Evidence.EvidenceRefs = nil
			case "duplicate":
				receipt.Evidence.EvidenceRefs = append(receipt.Evidence.EvidenceRefs, ref)
			case "abbreviated":
				receipt.Evidence.EvidenceRefs = []string{implementationWorkFullflowCommitPrefix + "abcdef0"}
			case "invalid-hex":
				receipt.Evidence.EvidenceRefs = []string{implementationWorkFullflowCommitPrefix + strings.Repeat("z", 40)}
			case "uppercase":
				receipt.Evidence.EvidenceRefs = []string{implementationWorkFullflowCommitPrefix + strings.Repeat("A", 40)}
			}
			got, err := implementationWorkFullflowCommitReceiptRef(receipt, "commit")
			if scenario == "exact" {
				if err != nil || got != ref {
					t.Fatalf("合法 exact receipt 被拒: %s %v", got, err)
				}
			} else if err == nil {
				t.Fatal("缺失或伪造 commit receipt 被接受")
			}
		})
	}
}

func TestImplementationWorkCommitEvidenceReadsActualTreeAndBytes(t *testing.T) {
	for _, scenario := range []string{"exact", "wrong-pin", "missing-member", "changed-bytes", "untracked", "deleted-member", "changed-mode", "wrong-input-hash", "wrong-expected-bytes", "symlink"} {
		t.Run(scenario, func(t *testing.T) {
			root := t.TempDir()
			files := map[string]string{"go.mod": "module synthetic\ngo 1.26\n", "delivery.go": "package synthetic\nfunc Value() int {return 4}\n", "input_test.go": "package synthetic\n", "nested/含 # % 空格/原文.bin": "\x00\xff\r\noriginal bytes\n"}
			for path, body := range files {
				if err := os.MkdirAll(filepath.Dir(filepath.Join(root, path)), 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(root, path), []byte(body), 0644); err != nil {
					t.Fatal(err)
				}
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			for _, args := range [][]string{{"init", "-q"}, {"add", "--", "go.mod", "delivery.go", "input_test.go", "nested"}, {"-c", "user.name=Synthetic", "-c", "user.email=synthetic@example.local", "-c", "core.hooksPath=/dev/null", "commit", "-q", "--no-verify", "-m", "test: synthetic commit evidence"}} {
				if _, err := implementationWorkFullflowGitRead(ctx, root, args...); err != nil {
					t.Fatal(err)
				}
			}
			head, err := implementationWorkFullflowGitRead(ctx, root, "rev-parse", "HEAD")
			if err != nil {
				t.Fatal(err)
			}
			ref := implementationWorkFullflowCommitPrefix + strings.TrimSuffix(string(head), "\n")
			reviewed := map[string]string{"input_test.go": runtimecontract.RuntimeValidationInputContentHash(files["input_test.go"])}
			switch scenario {
			case "wrong-pin":
				ref = implementationWorkFullflowCommitPrefix + strings.Repeat("0", 40)
			case "missing-member":
				files["missing.md"] = "required"
			case "changed-bytes":
				err = os.WriteFile(filepath.Join(root, "delivery.go"), []byte("different\n"), 0644)
			case "untracked":
				err = os.WriteFile(filepath.Join(root, "untracked.txt"), []byte("uncommitted"), 0644)
			case "deleted-member":
				err = os.Remove(filepath.Join(root, "delivery.go"))
			case "changed-mode":
				err = os.Chmod(filepath.Join(root, "delivery.go"), 0755)
			case "symlink":
				err = os.Symlink("delivery.go", filepath.Join(root, "linked.go"))
			case "wrong-expected-bytes":
				files["delivery.go"] = "not the expected implementation\n"
			case "wrong-input-hash":
				reviewed["input_test.go"] = runtimecontract.RuntimeValidationInputContentHash("different")
			}
			if err != nil {
				t.Fatal(err)
			}
			err = implementationWorkFullflowVerifyCommitTree(root, ref, files, reviewed)
			if scenario == "exact" && err != nil {
				t.Fatalf("真实完整 commit tree 被拒: %v", err)
			}
			if scenario != "exact" && err == nil {
				t.Fatal("缺失或漂移 tree/blob 被接受")
			}
		})
	}
}
