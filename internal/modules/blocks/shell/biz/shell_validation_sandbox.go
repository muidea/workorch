package shell

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

const managedValidationSandboxDirectory = "workspace"

// prepareManagedValidationSandbox creates an owner-controlled, bounded copy of
// the deliverable tree. Managed validation executes only inside this copy; its
// relative writes are therefore ephemeral and discarded with the session.
// Symlinks are rejected rather than copied so a workspace link cannot redirect
// a supposedly non-durable recipe to a host path.
func prepareManagedValidationSandbox(request *RuntimeCommandRequest, deliverableDirectory string) (string, error) {
	if request == nil || !shellInputHasManagedValidation(*request) {
		return deliverableDirectory, nil
	}
	scratch := strings.TrimSpace(request.ValidationScratchDirectory)
	if scratch == "" {
		created, err := os.MkdirTemp("", "workorch-shell-validation-")
		if err != nil {
			return "", err
		}
		scratch = created
		request.ValidationScratchDirectory = scratch
		request.ValidationScratchAutoCleanup = true
		request.ValidationScratchAutoCleanupSet = true
		request.ManagedValidation.AutoCleanup = true
		request.ManagedValidation.AutoCleanupSet = true
	}
	if err := validateManagedValidationScratchPath(deliverableDirectory, scratch); err != nil {
		return "", err
	}
	root := filepath.Join(scratch, managedValidationSandboxDirectory)
	if err := os.RemoveAll(root); err != nil {
		return "", err
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", err
	}
	inputs, err := copyManagedValidationWorkspace(deliverableDirectory, root)
	if err != nil {
		return "", err
	}
	request.validationCopiedInputs = inputs
	for _, directory := range []string{filepath.Join(scratch, "home"), filepath.Join(scratch, "tmp"), filepath.Join(scratch, "cache")} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			return "", err
		}
	}
	return root, nil
}

func validateManagedValidationScratchPath(deliverableDirectory, scratch string) error {
	scratchAbs, err := filepath.Abs(strings.TrimSpace(scratch))
	if err != nil || scratchAbs == "" {
		return common.ErrInvalidArgument
	}
	deliverableAbs, err := filepath.Abs(strings.TrimSpace(deliverableDirectory))
	if err != nil || deliverableAbs == "" {
		return common.ErrInvalidArgument
	}
	if scratchAbs == deliverableAbs || strings.HasPrefix(scratchAbs, deliverableAbs+string(os.PathSeparator)) {
		return fmt.Errorf("%w: managed validation sandbox must stay outside workspace deliverable directory", common.ErrInvalidArgument)
	}
	tempRoot, err := filepath.Abs(os.TempDir())
	if err != nil || scratchAbs == tempRoot || !strings.HasPrefix(scratchAbs, tempRoot+string(os.PathSeparator)) {
		return fmt.Errorf("%w: managed validation sandbox must stay below the temporary directory", common.ErrInvalidArgument)
	}
	return nil
}

func copyManagedValidationWorkspace(source, destination string) ([]shellValidationCopiedInput, error) {
	source, err := filepath.Abs(strings.TrimSpace(source))
	if err != nil || source == "" {
		return nil, common.ErrInvalidArgument
	}
	inputs := make([]shellValidationCopiedInput, 0)
	var fileCount int
	var totalBytes int64
	err = filepath.WalkDir(source, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == source {
			return nil
		}
		rel, err := filepath.Rel(source, path)
		if err != nil || rel == "." || filepath.IsAbs(rel) || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return common.ErrInvalidArgument
		}
		clean := filepath.ToSlash(filepath.Clean(rel))
		if entry.IsDir() {
			if managedValidationSandboxPathExcluded(clean) {
				return filepath.SkipDir
			}
			return os.MkdirAll(filepath.Join(destination, rel), 0o755)
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("%w: managed validation sandbox rejects workspace symlink %q", common.ErrInvalidArgument, clean)
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("%w: managed validation sandbox rejects special workspace entry %q", common.ErrInvalidArgument, clean)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		fileCount++
		if fileCount > maxObservedWorkspaceFiles || info.Size() > maxObservedWorkspaceBytes-totalBytes {
			return fmt.Errorf("managed validation sandbox exceeds bounded file or byte limit")
		}
		target := filepath.Join(destination, rel)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		input, err := os.Open(path)
		if err != nil {
			return err
		}
		output, createErr := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, info.Mode().Perm()&0o777)
		if createErr != nil {
			_ = input.Close()
			return createErr
		}
		digest := sha256.New()
		remaining := maxObservedWorkspaceBytes - totalBytes
		copied, copyErr := io.Copy(io.MultiWriter(output, digest), io.LimitReader(input, remaining+1))
		closeOutputErr := output.Close()
		closeInputErr := input.Close()
		if copyErr != nil {
			return copyErr
		}
		if copied > remaining {
			return fmt.Errorf("managed validation sandbox exceeds bounded file or byte limit during copy")
		}
		if closeOutputErr != nil {
			return closeOutputErr
		}
		if closeInputErr != nil {
			return closeInputErr
		}
		totalBytes += copied
		inputs = append(inputs, shellValidationCopiedInput{TargetRef: clean, ContentHash: hex.EncodeToString(digest.Sum(nil)), Size: copied, Mode: uint32(info.Mode().Perm() & 0o777)})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return inputs, nil
}

func managedValidationSandboxPathExcluded(ref string) bool {
	for _, part := range strings.Split(filepath.ToSlash(ref), "/") {
		for _, excluded := range managedValidationSandboxExcludedDirectories() {
			if part == excluded {
				return true
			}
		}
	}
	return false
}

func managedValidationSandboxExcludedDirectories() []string {
	return []string{".git", "node_modules", "vendor", "dist", "build", ".workorch-data", ".workorch-workspaces"}
}

func managedValidationEnvironment(base []string, scratch string) []string {
	scratch = strings.TrimSpace(scratch)
	values := make(map[string]string, len(base)+6)
	for _, item := range base {
		key, value, found := strings.Cut(item, "=")
		if found && strings.TrimSpace(key) != "" {
			values[key] = value
		}
	}
	hostHome := strings.TrimSpace(values["HOME"])
	overrides := map[string]string{
		"HOME":           filepath.Join(scratch, "home"),
		"TMPDIR":         filepath.Join(scratch, "tmp"),
		"XDG_CACHE_HOME": filepath.Join(scratch, "cache"),
	}
	// Go's content-addressed build/module caches are owner technical caches, not
	// workspace authority. Reuse their explicit host locations so every managed
	// validation does not cold-compile the toolchain or download dependencies.
	// All recipe-relative writes, HOME, TMPDIR and other XDG caches remain inside
	// the disposable session copy, and at-most-once replay semantics account for
	// these non-authoritative ephemeral cache effects.
	if cache := managedValidationHostCachePath(values["GOCACHE"], hostHome, filepath.Join(".cache", "go-build")); cache != "" {
		overrides["GOCACHE"] = cache
	} else {
		overrides["GOCACHE"] = filepath.Join(scratch, "cache", "go-build")
	}
	if cache := managedValidationHostCachePath(values["GOMODCACHE"], hostHome, filepath.Join("go", "pkg", "mod")); cache != "" {
		overrides["GOMODCACHE"] = cache
	}
	for key, value := range overrides {
		values[key] = value
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	ret := make([]string, 0, len(keys))
	for _, key := range keys {
		ret = append(ret, key+"="+values[key])
	}
	return ret
}

func managedValidationHostCachePath(explicit, hostHome, homeRelative string) string {
	value := strings.TrimSpace(explicit)
	if value == "off" {
		return ""
	}
	if value == "" && strings.TrimSpace(hostHome) != "" {
		value = filepath.Join(hostHome, homeRelative)
	}
	if value == "" || !filepath.IsAbs(value) {
		return ""
	}
	return filepath.Clean(value)
}
