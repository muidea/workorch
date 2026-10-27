package contextregistryports

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistrycore"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/fileobservation"
	"github.com/muidea/workorch/internal/pkg/skillhubpreparation"
)

const SkillHubPreparationInstructionLimit = 64 * 1024
const SkillHubPreparationInstructionsLimit = 8 * 1024 * 1024

// These are restricted diagnostic bytes, not selected Skill context or a
// currentness permit. Status directory hashes are opaque: they are not hashes
// of SKILL.md alone and must never be compared to RawSHA256 as equivalent.
type SkillHubPreparationInstruction struct {
	SkillID      string
	Path         string
	Raw          []byte
	RawSHA256    string
	ObservedSize int64
	ReadBytes    uint64
	Complete     bool
	Reobserved   bool
	FailureCode  string
}

type SkillHubPreparationInstructions struct {
	Inventory              SkillHubPreparationObservation
	InstalledRoot          string
	Instructions           []SkillHubPreparationInstruction
	ReobservedInstructions []SkillHubPreparationInstruction
	StartedAt              time.Time
	CompletedAt            time.Time
	FailureCode            string
	InstructionsReobserved bool
	OtherAgentItemCount    int
	FailedReobservation    *SkillHubPreparationInstruction
	References             *skillhubpreparation.References
}

// ObserveSkillHubPreparationInstructions first obtains the inventory itself;
// callers cannot replace it with an "already verified" flag. Status retains
// SkillHub's existing effects; only subsequent installed-file reads are read-only.
// Global instructions are Codex-only; the whole multi-agent inventory is kept.
func ObserveSkillHubPreparationInstructions(ctx context.Context, runner SkillHubPreparationRunner, workspaceID common.WorkspaceID, directory string, global bool) (SkillHubPreparationInstructions, error) {
	value := SkillHubPreparationInstructions{StartedAt: time.Now().UTC()}
	fail := func(code string, err error) (SkillHubPreparationInstructions, error) {
		value.FailureCode = code
		value.CompletedAt = time.Now().UTC()
		return value, err
	}
	if ctx == nil {
		return fail("invalid_request", common.ErrInvalidArgument)
	}
	bounded, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	var err error
	value.Inventory, err = ObserveSkillHubPreparation(bounded, runner, workspaceID, directory, global)
	if err != nil {
		return fail("inventory_unavailable", err)
	}
	return ObserveSkillHubInstalledFromCapture(bounded, value.Inventory)
}

// Reparse exact captured raw status. No runner, command or caller success flag
// is accepted; this path can only read installed files, never repeat status.
func ObserveSkillHubInstalledFromCapture(ctx context.Context, capture SkillHubPreparationObservation) (SkillHubPreparationInstructions, error) {
	value := SkillHubPreparationInstructions{StartedAt: time.Now().UTC(), Inventory: capture}
	fail := func(code string, err error) (SkillHubPreparationInstructions, error) {
		value.FailureCode = code
		value.CompletedAt = time.Now().UTC()
		return value, err
	}
	global := capture.Scope == "global"
	directory := capture.Directory
	if capture.StartedAt.IsZero() || capture.CompletedAt.Before(capture.StartedAt) || capture.CompletedAt.After(value.StartedAt) {
		return fail("inventory_unavailable", common.ErrConflict)
	}
	if ctx == nil || capture.WorkspaceID == "" || capture.Version != "workorch-skillhub-preparation-observation/v2" || capture.EffectContract != "skillhub-status-effects/v1" || capture.Scope != "global" && capture.Scope != "workspace" || global && directory != "" || !global && (!filepath.IsAbs(directory) || filepath.Clean(directory) != directory) || capture.FailureCode != "" || !capture.CommandStarted || !capture.ExitCodeKnown || capture.ExitCode != 0 || !capture.OutputComplete || !capture.StderrComplete || uint64(len(capture.RawOutput)) != capture.TotalOutputBytes || uint64(len(capture.RawStderr)) != capture.TotalStderrBytes || len(capture.RawStderr) > SkillHubPreparationOutputLimit || fileobservation.Digest(capture.RawOutput) != capture.RawSHA256 || fileobservation.Digest(capture.RawStderr) != capture.StderrSHA256 {
		return fail("inventory_unavailable", common.ErrConflict)
	}
	if _, err := skillhubpreparation.ParseStatus(capture.RawOutput, directory, global); err != nil {
		return fail("inventory_unavailable", err)
	}
	// Raw external JSON is the sole inventory source, never capture.Status.
	value.Inventory.Status = contextregistrycore.SkillHubStatus{}
	if err := json.Unmarshal(capture.RawOutput, &value.Inventory.Status); err != nil {
		return fail("inventory_unavailable", err)
	}
	bounded, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	rootPath, err := preparationSkillInstalledRoot(directory, global)
	if err != nil {
		return fail("installed_root_unavailable", err)
	}
	value.InstalledRoot = rootPath
	// Root is derived independently of external status paths. No SourcePath or
	// RepoPath fallback is allowed, including when an installed file is missing.
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return fail("installed_root_unavailable", err)
	}
	defer root.Close()
	rootInfo, err := root.Stat(".")
	if err != nil {
		return fail("installed_root_unavailable", err)
	}
	value.Instructions = make([]SkillHubPreparationInstruction, 0, len(value.Inventory.Status.Items))
	value.ReobservedInstructions = make([]SkillHubPreparationInstruction, 0, len(value.Inventory.Status.Items))
	total := 0
	for _, item := range value.Inventory.Status.Items {
		if err := bounded.Err(); err != nil {
			return fail("cancelled", err)
		}
		if global && item.Agent != "codex" {
			value.OtherAgentItemCount++
			continue
		}
		relative, err := preparationSkillInstructionPath(rootPath, item, global)
		if err != nil {
			return fail("invalid_installed_path", err)
		}
		remaining := min(SkillHubPreparationInstructionLimit, SkillHubPreparationInstructionsLimit-total)
		observed, err := readPreparationSkillInstruction(bounded, root, relative, item.SkillID, remaining)
		value.Instructions = append(value.Instructions, observed)
		if err != nil {
			return fail("instruction_unavailable", err)
		}
		total += len(observed.Raw)
	}
	// Read only explicitly declared local references. The second instruction
	// read below verifies that the source declaration did not drift around them.
	value.References = observePreparationSkillReferences(bounded, root, value.Instructions)
	reobservedTotal := 0
	for i := range value.Instructions {
		previous := &value.Instructions[i]
		relative, err := filepath.Rel(rootPath, previous.Path)
		if err != nil {
			return fail("invalid_installed_path", err)
		}
		current, err := readPreparationSkillInstruction(bounded, root, relative, previous.SkillID, min(SkillHubPreparationInstructionLimit, SkillHubPreparationInstructionsLimit-reobservedTotal))
		value.ReobservedInstructions = append(value.ReobservedInstructions, current)
		reobservedTotal += len(current.Raw)
		if err != nil {
			value.FailedReobservation = &current
			return fail("instruction_reobservation_failed", err)
		}
		if !bytes.Equal(previous.Raw, current.Raw) || previous.ObservedSize != current.ObservedSize {
			value.FailedReobservation = &current
			return fail("instruction_drift", common.ErrConflict)
		}
		previous.Reobserved = true
	}
	// os.Root pins the opened directory. Verify that the configured pathname
	// still designates it; successful reads of a renamed old root are not current.
	currentRoot, err := preparationSkillInstalledRoot(directory, global)
	if err != nil || currentRoot != rootPath {
		return fail("installed_root_drift", common.ErrConflict)
	}
	currentInfo, err := os.Stat(rootPath)
	if err != nil || !os.SameFile(rootInfo, currentInfo) {
		return fail("installed_root_drift", common.ErrConflict)
	}
	if err := bounded.Err(); err != nil {
		return fail("cancelled", err)
	}
	value.InstructionsReobserved = true
	value.CompletedAt = time.Now().UTC()
	return value, nil
}

func preparationSkillInstalledRoot(directory string, global bool) (string, error) {
	root := filepath.Join(directory, ".agents", "skills")
	if global {
		root = os.Getenv("CODEX_SKILLS_DIR")
		if root == "" {
			base := os.Getenv("CODEX_HOME")
			if base == "" {
				home, err := os.UserHomeDir()
				if err != nil {
					return "", err
				}
				base = filepath.Join(home, ".codex")
			}
			root = filepath.Join(base, "skills")
		}
	}
	if !filepath.IsAbs(root) || filepath.Clean(root) != root || root == string(filepath.Separator) || !utf8.ValidString(root) || strings.ContainsRune(root, 0) {
		return "", common.ErrConflict
	}
	info, err := os.Lstat(root)
	if err != nil {
		return "", err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", common.ErrConflict
	}
	return root, nil
}

func preparationSkillInstructionPath(root string, item contextregistrycore.SkillHubStatusItem, global bool) (string, error) {
	id := item.SkillID
	if id == "" || id == "." || len(id) > 256 || !utf8.ValidString(id) || strings.TrimSpace(id) != id || strings.ContainsAny(id, "\\:\x00\r\n") || !filepath.IsLocal(id) || filepath.ToSlash(filepath.Clean(id)) != id {
		return "", common.ErrConflict
	}
	relative := filepath.Join(filepath.FromSlash(id), "SKILL.md")
	path := filepath.Join(root, relative)
	target := filepath.Dir(path)
	if global {
		if item.Agent != "codex" || item.TargetPath != target && item.TargetPath != path {
			return "", common.ErrConflict
		}
	} else if item.LocalPath != path || item.TargetPath != "" && item.TargetPath != target && item.TargetPath != path {
		return "", common.ErrConflict
	}
	return relative, nil
}

func readPreparationSkillInstruction(ctx context.Context, root *os.Root, relative, id string, limit int) (SkillHubPreparationInstruction, error) {
	value, err := readPreparationSkillBytes(ctx, root, relative, id, limit)
	if err == nil && (len(value.Raw) == 0 || !utf8.Valid(value.Raw)) {
		value.FailureCode = "invalid_instruction_content"
		return value, common.ErrConflict
	}
	return value, err
}

// Auxiliary resources may be binary or empty; preserve their bytes without
// applying instruction text validation or interpreting/executing their content.
func readPreparationSkillBytes(ctx context.Context, root *os.Root, relative, id string, limit int) (SkillHubPreparationInstruction, error) {
	value := SkillHubPreparationInstruction{SkillID: id, Path: filepath.Join(root.Name(), relative)}
	finishHash := func() { hash := sha256.Sum256(value.Raw); value.RawSHA256 = hex.EncodeToString(hash[:]) }
	fail := func(code string, err error) (SkillHubPreparationInstruction, error) {
		value.FailureCode = code
		finishHash()
		return value, err
	}
	if ctx == nil || limit < 0 || limit > SkillHubPreparationInstructionLimit {
		return fail("invalid_request", common.ErrInvalidArgument)
	}
	if err := ctx.Err(); err != nil {
		return fail("cancelled", err)
	}
	before, err := preparationInstructionInfo(root, relative)
	if err != nil {
		return fail("invalid_instruction_file", err)
	}
	value.ObservedSize = before.Size()
	if before.Size() > int64(limit) {
		return fail("instruction_too_large", common.ErrConflict)
	}
	f, err := openPreparationInstructionFile(root, relative)
	if err != nil {
		return fail("instruction_open_failed", err)
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !samePreparationInstructionFile(before, opened) {
		return fail("instruction_drift", common.ErrConflict)
	}
	// Read in bounded chunks, retaining original bytes even for invalid UTF-8.
	buffer := make([]byte, 4096)
	for {
		if err := ctx.Err(); err != nil {
			return fail("cancelled", err)
		}
		remaining := limit - len(value.Raw)
		n, readErr := f.Read(buffer[:min(len(buffer), remaining+1)])
		value.ReadBytes += uint64(n)
		value.Raw = append(value.Raw, buffer[:min(n, remaining)]...)
		if n > remaining {
			return fail("instruction_too_large", common.ErrConflict)
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return fail("instruction_read_failed", readErr)
		}
		if n == 0 {
			return fail("instruction_read_incomplete", io.ErrNoProgress)
		}
	}
	current, err := preparationInstructionInfo(root, relative)
	after, statErr := f.Stat()
	if err != nil || statErr != nil || !samePreparationInstructionFile(before, current) || !samePreparationInstructionFile(before, after) || int64(len(value.Raw)) != before.Size() {
		return fail("instruction_drift", common.ErrConflict)
	}
	value.Complete = true
	finishHash()
	return value, nil
}

func preparationInstructionInfo(root *os.Root, relative string) (os.FileInfo, error) {
	if !filepath.IsLocal(relative) {
		return nil, common.ErrConflict
	}
	parts := strings.Split(relative, string(filepath.Separator))
	for i := range parts {
		info, err := root.Lstat(filepath.Join(parts[:i+1]...))
		if err != nil {
			return nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 || i < len(parts)-1 && !info.IsDir() {
			return nil, common.ErrConflict
		}
		if i == len(parts)-1 {
			if !info.Mode().IsRegular() {
				return nil, common.ErrConflict
			}
			return info, nil
		}
	}
	return nil, common.ErrConflict
}

func samePreparationInstructionFile(a, b os.FileInfo) bool {
	return a != nil && b != nil && a.Mode().IsRegular() && b.Mode().IsRegular() && os.SameFile(a, b) && a.Size() == b.Size() && a.Mode() == b.Mode() && a.ModTime() == b.ModTime()
}
