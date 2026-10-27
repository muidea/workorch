package skillhubpreparation

import (
	"bytes"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/fileobservation"
)

const InstalledVersion = "workorch-skillhub-installed-facts/v1"
const InstructionLimit = 64 << 10
const InstructionsLimit = 8 << 20

// Actual independent reads, never a caller-provided reobserved/PASS flag.
type Instruction struct {
	SkillID, Path string
	Raw           []byte
	RawSHA256     string
	ObservedSize  int64
	ReadBytes     uint64
	Complete      bool
	FailureCode   string
}
type Installed struct {
	Version, Root                    string
	First, Second                    []Instruction
	StartedAt, CompletedAt           time.Time
	FailureCode                      string
	ErrorText                        []byte
	ErrorBytes                       uint64
	ErrorSHA256, ErrorNotSavedReason string
	References                       *References `json:",omitempty"`
}

func (f Installed) Clone() Installed {
	if f.References != nil {
		r := f.References.Clone()
		f.References = &r
	}
	f.ErrorText = bytes.Clone(f.ErrorText)
	copyFiles := func(in []Instruction) []Instruction {
		if in == nil {
			return nil
		}
		out := make([]Instruction, len(in))
		copy(out, in)
		for i := range out {
			out[i].Raw = bytes.Clone(out[i].Raw)
		}
		return out
	}
	f.First = copyFiles(f.First)
	f.Second = copyFiles(f.Second)
	return f
}
func InstructionPath(root string, item StatusItem, global bool) (string, error) {
	id := item.SkillID
	if id == "" || id == "." || len(id) > 256 || !utf8.ValidString(id) || strings.TrimSpace(id) != id || strings.ContainsAny(id, "\\:\x00\r\n") || !filepath.IsLocal(id) || filepath.ToSlash(filepath.Clean(id)) != id {
		return "", common.ErrConflict
	}
	path := filepath.Join(root, filepath.FromSlash(id), "SKILL.md")
	target := filepath.Dir(path)
	if global {
		if item.Agent != "codex" || item.TargetPath != target && item.TargetPath != path {
			return "", common.ErrConflict
		}
	} else if item.LocalPath != path || item.TargetPath != "" && item.TargetPath != target && item.TargetPath != path {
		return "", common.ErrConflict
	}
	return path, nil
}

// Validate preserves partial failure facts. CompleteFor separately proves the
// all-items/double-read/metadata condition; neither confers resource readiness.
func (f Installed) Validate(status Status, directory string, global bool) error {
	if f.Version != InstalledVersion || f.StartedAt.IsZero() || f.CompletedAt.Before(f.StartedAt) || len(f.First) > 1024 || len(f.Second) > len(f.First) || len(f.FailureCode) > 512 || !utf8.ValidString(f.FailureCode) {
		return common.ErrConflict
	}
	if f.Root != "" && (!filepath.IsAbs(f.Root) || filepath.Clean(f.Root) != f.Root || f.Root == string(filepath.Separator) || !utf8.ValidString(f.Root) || strings.ContainsRune(f.Root, 0) || !global && f.Root != filepath.Join(directory, ".agents", "skills")) {
		return common.ErrConflict
	}
	items := make([]StatusItem, 0, len(status.Items))
	for _, item := range status.Items {
		if !global || item.Agent == "codex" {
			items = append(items, item)
		}
	}
	if len(f.First) > len(items) || f.Root == "" && (len(f.First) > 0 || len(f.Second) > 0 || f.FailureCode == "") {
		return common.ErrConflict
	}
	for _, files := range [][]Instruction{f.First, f.Second} {
		total := 0
		for i, file := range files {
			path, err := InstructionPath(f.Root, items[i], global)
			if err != nil || file.SkillID != items[i].SkillID || file.Path != path || file.ObservedSize < 0 || len(file.Raw) > InstructionLimit || file.ReadBytes < uint64(len(file.Raw)) || file.RawSHA256 != fileobservation.Digest(file.Raw) || len(file.FailureCode) > 512 || !utf8.ValidString(file.FailureCode) {
				return common.ErrConflict
			}
			total += len(file.Raw)
			if total > InstructionsLimit {
				return common.ErrConflict
			}
			if file.Complete && (file.ReadBytes != uint64(len(file.Raw)) || file.ObservedSize != int64(len(file.Raw))) {
				return common.ErrConflict
			}
		}
	}
	if f.ErrorBytes == 0 {
		if len(f.ErrorText) != 0 || f.ErrorSHA256 != "" || f.ErrorNotSavedReason != "" {
			return common.ErrConflict
		}
	} else if f.ErrorNotSavedReason == "" {
		if len(f.ErrorText) > 64<<10 || uint64(len(f.ErrorText)) != f.ErrorBytes || fileobservation.Digest(f.ErrorText) != f.ErrorSHA256 {
			return common.ErrConflict
		}
	} else if f.ErrorNotSavedReason != "error_exceeds_storage_limit" || len(f.ErrorText) != 0 || f.ErrorBytes <= 64<<10 || len(f.ErrorSHA256) != 64 || strings.Trim(f.ErrorSHA256, "0123456789abcdef") != "" {
		return common.ErrConflict
	}
	if f.References != nil && f.References.Validate(f) != nil {
		return common.ErrConflict
	}
	return nil
}
func (f Installed) CompleteFor(status Status, directory string, global bool) bool {
	if f.Validate(status, directory, global) != nil || f.FailureCode != "" || f.ErrorBytes != 0 || f.Root == "" || f.First == nil || f.Second == nil {
		return false
	}
	expected := 0
	for _, item := range status.Items {
		if !global || item.Agent == "codex" {
			expected++
		}
	}
	if len(f.First) != expected || len(f.Second) != expected {
		return false
	}
	for i, a := range f.First {
		b := f.Second[i]
		if !a.Complete || !b.Complete || a.FailureCode != "" || b.FailureCode != "" || a.ObservedSize != b.ObservedSize || !bytes.Equal(a.Raw, b.Raw) {
			return false
		}
		if _, err := ParsePreparationSkillHubMetadata(a.Raw); err != nil {
			return false
		}
	}
	return true
}
