package git

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	gitcommon "github.com/muidea/workorch/internal/modules/blocks/git/pkg/common"
)

var runtimeGitBenchmarkResult RuntimeOperationResult

func BenchmarkRuntimeAdapterLargeDirtyRepositoryStatus(b *testing.B) {
	if _, err := exec.LookPath("git"); err != nil {
		b.Skip("git binary not available")
	}
	repo := b.TempDir()
	for _, args := range [][]string{
		{"init", "-q"},
		{"config", "user.email", "workorch@example.test"},
		{"config", "user.name", "WorkOrch Test"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		if output, err := cmd.CombinedOutput(); err != nil {
			b.Fatalf("git %v: %v\n%s", args, err, output)
		}
	}
	for idx := range 900 {
		path := filepath.Join(repo, "dirty", strings.Repeat("x", 64), fmt.Sprintf("file-%04d.txt", idx))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			b.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("dirty\n"), 0o644); err != nil {
			b.Fatal(err)
		}
	}
	adapter := NewRuntimeAdapter(false)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		result, err := adapter.ExecuteOperation(context.Background(), RuntimeOperationRequest{
			Kind: gitcommon.StepKindRead, WorkspaceDeliverableDirectory: repo, Operation: gitcommon.OperationStatus,
		})
		if err != nil || !result.Truncated || len(result.ChangedFiles) > maxChangedFiles || len(result.Evidence.EvidenceRefs) > maxGitEvidenceRefs {
			b.Fatalf("large dirty repository result=%+v err=%v", result, err)
		}
		runtimeGitBenchmarkResult = result
	}
}
