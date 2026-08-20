package shell

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// shellWorkspaceFileRevisions is an owner-local, bounded observation of the
// authority-relevant workspace tree plus .git control state and symlink
// identity. Generated directories excluded by the File-owner manifest are not
// authority targets. Shell may not silently mutate repository metadata, and
// managed validation uses this view to prove that its owner-isolated execution
// did not escape back into the authoritative workspace.
func shellWorkspaceFileRevisions(deliverableDirectory string, required bool) (map[string]string, error) {
	if !required {
		return nil, nil
	}
	root, err := filepath.Abs(strings.TrimSpace(deliverableDirectory))
	if err != nil || root == "" {
		return nil, common.ErrInvalidArgument
	}
	values := map[string]string{}
	var count int
	var totalBytes int64
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil || rel == "." || filepath.IsAbs(rel) || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return common.ErrInvalidArgument
		}
		if entry.IsDir() {
			if shellWorkspaceObservationPathExcluded(filepath.ToSlash(rel)) {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			target, linkErr := os.Readlink(path)
			if linkErr != nil {
				return linkErr
			}
			hash := sha256.Sum256([]byte("symlink\x00" + target))
			values[filepath.ToSlash(rel)] = "symlink:" + hex.EncodeToString(hash[:])
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("workspace observation cannot safely address special entry %q", filepath.ToSlash(rel))
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			return infoErr
		}
		count++
		totalBytes += info.Size()
		if count > maxObservedWorkspaceFiles || totalBytes > maxObservedWorkspaceBytes {
			return fmt.Errorf("workspace observation exceeds bounded file or byte limit")
		}
		input, openErr := os.Open(path)
		if openErr != nil {
			return openErr
		}
		hash := sha256.New()
		_, _ = fmt.Fprintf(hash, "mode=%o\x00", info.Mode().Perm())
		_, readErr := io.Copy(hash, input)
		closeErr := input.Close()
		if readErr != nil {
			return readErr
		}
		if closeErr != nil {
			return closeErr
		}
		values[filepath.ToSlash(rel)] = hex.EncodeToString(hash.Sum(nil))
		return nil
	})
	if err != nil {
		return nil, err
	}
	return values, nil
}

// shellWorkspaceObservationPathExcluded mirrors the File-owner manifest's
// non-authoritative generated-directory exclusions. The Shell snapshot still
// includes .git control state: arbitrary shell/command must not hide a Git
// metadata mutation behind a declared deliverable target.
func shellWorkspaceObservationPathExcluded(ref string) bool {
	for _, part := range strings.Split(filepath.ToSlash(ref), "/") {
		switch part {
		case "node_modules", "vendor", "dist", "build", ".workorch-data", ".workorch-workspaces":
			return true
		}
	}
	return false
}

// validateShellWorkspaceSymlinkScope rejects links that resolve outside the
// deliverable root before an arbitrary durable Shell command starts. Internal
// links remain observable through their canonical in-workspace target; an
// external link would otherwise let a sealed target mutate host state that no
// workspace receipt can represent.
func validateShellWorkspaceSymlinkScope(deliverableDirectory string) error {
	root, err := filepath.Abs(strings.TrimSpace(deliverableDirectory))
	if err != nil || root == "" {
		return common.ErrInvalidArgument
	}
	return filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root || entry.Type()&os.ModeSymlink == 0 {
			return nil
		}
		resolved, err := filepath.EvalSymlinks(path)
		if err != nil {
			return fmt.Errorf("%w: Shell workspace symlink %q cannot be resolved", common.ErrInvalidArgument, path)
		}
		resolved, err = filepath.Abs(resolved)
		if err != nil || (resolved != root && !strings.HasPrefix(resolved, root+string(filepath.Separator))) {
			return fmt.Errorf("%w: Shell workspace symlink %q escapes the deliverable root", common.ErrInvalidArgument, path)
		}
		return nil
	})
}

func shellWorkspaceTargetRevisions(deliverableDirectory string, request RuntimeCommandRequest) map[string]string {
	if !request.StateChanging {
		return nil
	}
	root, err := filepath.Abs(strings.TrimSpace(deliverableDirectory))
	if err != nil || root == "" {
		return nil
	}
	refs := append([]string{request.TargetRef}, request.TargetRefs...)
	ret := make(map[string]string, len(refs))
	for _, ref := range refs {
		ref = strings.TrimSpace(ref)
		if ref == "" || filepath.IsAbs(ref) {
			continue
		}
		target := filepath.Join(root, filepath.Clean(ref))
		if target != root && !strings.HasPrefix(target, root+string(filepath.Separator)) {
			continue
		}
		data, readErr := os.ReadFile(target)
		if readErr != nil {
			if os.IsNotExist(readErr) {
				ret[ref] = "missing"
			}
			continue
		}
		hash := sha256.Sum256(data)
		ret[ref] = hex.EncodeToString(hash[:])
	}
	return ret
}

func shellWorkspaceTargetChanges(before map[string]string, after map[string]string) []RuntimeWorkspaceTargetChange {
	seen := make(map[string]struct{}, len(before)+len(after))
	for ref := range before {
		seen[ref] = struct{}{}
	}
	for ref := range after {
		seen[ref] = struct{}{}
	}
	refs := make([]string, 0, len(seen))
	for ref := range seen {
		refs = append(refs, ref)
	}
	sort.Strings(refs)
	ret := make([]RuntimeWorkspaceTargetChange, 0, len(refs))
	for _, ref := range refs {
		beforeRevision, afterRevision := before[ref], after[ref]
		if beforeRevision == "" {
			beforeRevision = "missing"
		}
		if afterRevision == "" {
			afterRevision = "missing"
		}
		ret = append(ret, RuntimeWorkspaceTargetChange{TargetRef: ref, BeforeRevision: beforeRevision, AfterRevision: afterRevision})
	}
	return ret
}

func shellWorkspaceTargetChangesChanged(values []RuntimeWorkspaceTargetChange) bool {
	for _, value := range values {
		if strings.TrimSpace(value.TargetRef) != "" && value.BeforeRevision != value.AfterRevision {
			return true
		}
	}
	return false
}

type shellWorkspaceChangeFingerprint struct {
	Available bool
	Value     string
}

func workspaceStateChangeFingerprint(ctx context.Context, deliverableDirectory string, required bool) shellWorkspaceChangeFingerprint {
	if !required {
		return shellWorkspaceChangeFingerprint{}
	}
	dir := strings.TrimSpace(deliverableDirectory)
	if dir == "" {
		return shellWorkspaceChangeFingerprint{}
	}
	gitDir := filepath.Join(dir, ".git")
	if info, err := os.Stat(gitDir); err != nil || !info.IsDir() {
		return shellWorkspaceChangeFingerprint{}
	}
	snapshotCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	head := strings.TrimSpace(gitCommandOutput(snapshotCtx, dir, "rev-parse", "--verify", "HEAD"))
	status := strings.TrimSpace(gitCommandOutput(snapshotCtx, dir, "status", "--porcelain=v1", "--untracked-files=all"))
	return shellWorkspaceChangeFingerprint{
		Available: true,
		Value:     "head=" + head + "\nstatus=" + status,
	}
}

func gitCommandOutput(ctx context.Context, dir string, args ...string) string {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return ""
	}
	return string(out)
}

func attachManagedValidationResources(output *RuntimeCommandResult, request RuntimeCommandRequest, pid int, startedAt time.Time, finishedAt time.Time, processStatus string) {
	if output == nil || !shellInputHasValidationResourceLifecycle(request) {
		return
	}
	managedInput := shellInputHasManagedValidation(request)
	resources := make([]runtimemodel.RuntimeValidationResource, 0, 2)
	if pid > 0 {
		resources = append(resources, runtimemodel.RuntimeValidationResource{
			ID:          shellManagedValidationResourceID(request, "process"),
			WorkspaceID: string(request.WorkspaceID), RunID: string(request.RunID), StepID: string(request.StepID),
			Kind: "process", PID: pid, Status: processStatus,
			CleanupPolicy: "process_group_on_timeout_or_completion",
			CreatedAt:     startedAt.Format(time.RFC3339Nano), ReleasedAt: finishedAt.Format(time.RFC3339Nano),
		})
	}
	if scratch := strings.TrimSpace(request.ValidationScratchDirectory); scratch != "" {
		autoCleanup := managedValidationScratchAutoCleanupEnabled(request)
		cleanupPolicy := "remove_step_scoped_validation_scratch_on_completion"
		status := "released"
		if !autoCleanup {
			cleanupPolicy = "retain_validation_scratch_until_run_terminal"
			status = "retained"
		}
		record := runtimemodel.RuntimeValidationResource{
			ID:          shellManagedValidationResourceID(request, "scratch_dir"),
			WorkspaceID: string(request.WorkspaceID), RunID: string(request.RunID), StepID: string(request.StepID),
			Kind: "scratch_dir", Path: scratch, Status: status, CleanupPolicy: cleanupPolicy,
			CreatedAt: startedAt.Format(time.RFC3339Nano), ReleasedAt: finishedAt.Format(time.RFC3339Nano),
		}
		if cleanupErr := cleanupManagedValidationScratch(request); cleanupErr != nil {
			record.Status = "cleanup_failed"
			record.CleanupError = cleanupErr.Error()
		}
		resources = append(resources, record)
	}
	if len(resources) == 0 {
		return
	}
	output.ValidationResources = resources
	released := managedValidationResourcesReleased(resources)
	if managedInput {
		output.ManagedValidation.ResourceCount = len(resources)
		output.ManagedValidation.ResourcesReleased = released
	}
	output.RuntimeDiagnostics.ValidationResourceLifecycle = runtimemodel.RuntimeValidationResourceLifecycle{
		ResourceCount: len(resources), Released: released,
	}
}

func shellInputHasValidationResourceLifecycle(request RuntimeCommandRequest) bool {
	return shellInputHasManagedValidation(request) || strings.TrimSpace(request.ValidationScratchDirectory) != ""
}

func shellInputHasManagedValidation(request RuntimeCommandRequest) bool {
	managed := request.ManagedValidation
	return managed.RuntimeManaged ||
		strings.TrimSpace(managed.Kind) == runtimemodel.RuntimeBlockKindManagedValidationSession ||
		strings.TrimSpace(managed.OriginalNodeKind) == runtimemodel.RuntimeBlockKindManagedValidationSession
}

func shellManagedValidationResourceID(request RuntimeCommandRequest, kind string) string {
	return strings.Join([]string{string(request.WorkspaceID), string(request.RunID), string(request.StepID), strings.TrimSpace(kind)}, ":")
}

func managedValidationResourcesReleased(resources []runtimemodel.RuntimeValidationResource) bool {
	for _, resource := range resources {
		switch strings.TrimSpace(resource.Status) {
		case "released", "timeout":
		default:
			return false
		}
	}
	return true
}

func cleanupManagedValidationScratch(request RuntimeCommandRequest) error {
	if !managedValidationScratchAutoCleanupEnabled(request) {
		return nil
	}
	scratch := strings.TrimSpace(request.ValidationScratchDirectory)
	if scratch == "" {
		return nil
	}
	scratchAbs, err := filepath.Abs(scratch)
	if err != nil {
		return err
	}
	deliverableAbs, err := filepath.Abs(strings.TrimSpace(request.WorkspaceDeliverableDirectory))
	if err != nil {
		return err
	}
	if scratchAbs == deliverableAbs || strings.HasPrefix(scratchAbs, deliverableAbs+string(os.PathSeparator)) {
		return fmt.Errorf("%w: managed validation scratch cleanup must not target workspace deliverable directory", common.ErrInvalidArgument)
	}
	tempRoot, err := filepath.Abs(os.TempDir())
	if err != nil {
		return err
	}
	if scratchAbs == tempRoot || !strings.HasPrefix(scratchAbs, tempRoot+string(os.PathSeparator)) {
		return fmt.Errorf("%w: managed validation scratch cleanup must stay under temporary directory", common.ErrInvalidArgument)
	}
	if filepath.Base(scratchAbs) == "" || scratchAbs == string(os.PathSeparator) {
		return fmt.Errorf("%w: managed validation scratch cleanup target is invalid", common.ErrInvalidArgument)
	}
	return os.RemoveAll(scratchAbs)
}

func managedValidationScratchAutoCleanupEnabled(request RuntimeCommandRequest) bool {
	if request.ManagedValidation.AutoCleanupSet {
		return request.ManagedValidation.AutoCleanup
	}
	if request.ValidationScratchAutoCleanupSet {
		return request.ValidationScratchAutoCleanup
	}
	return strings.TrimSpace(request.ValidationScratchDirectory) != ""
}
