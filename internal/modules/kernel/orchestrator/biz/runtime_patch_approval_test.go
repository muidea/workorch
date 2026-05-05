package orchestrator

import (
	"testing"

	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
)

func TestPatchApprovalReasonsRequireApprovalForLargeLockfileChange(t *testing.T) {
	content := ""
	for i := 0; i < 60; i++ {
		content += "github.com/example/mod v1.0.0 h1:hash\n"
	}
	reasons := patchApprovalReasons(runtimePatchRequest{
		PatchType: "file_write",
		TargetRef: "go.sum",
		Content:   content,
	})
	if !containsString(reasons, "lockfile_large_change") {
		t.Fatalf("reasons = %+v, want lockfile_large_change", reasons)
	}
}

func TestPatchApprovalReasonsAllowSmallLockfileChange(t *testing.T) {
	reasons := patchApprovalReasons(runtimePatchRequest{
		PatchType: "file_write",
		TargetRef: "go.sum",
		Content:   "github.com/example/mod v1.0.0 h1:hash\n",
	})
	if containsString(reasons, "lockfile_large_change") || containsString(reasons, "large_change") {
		t.Fatalf("reasons = %+v, want no lockfile or large-change approval reason", reasons)
	}
}

func TestPatchRevertApprovalReasonsRequireApprovalForSensitiveCreatedFile(t *testing.T) {
	reasons := patchRevertApprovalReasons(&runmodel.Patch{
		TargetRef:       ".env",
		OriginalExists:  false,
		OriginalContent: "",
	})
	if !containsString(reasons, "sensitive_path") || !containsString(reasons, "delete_created_file") {
		t.Fatalf("reasons = %+v, want sensitive path and created-file deletion reasons", reasons)
	}
}

func containsString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
