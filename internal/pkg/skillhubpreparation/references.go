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

const ReferencesVersion = "workorch-skillhub-reference-observation/v1"
const ReferenceCountLimit = 256
const ReferenceBytesLimit = 1 << 20

type ReferenceRequirement struct {
	SkillID, Ref, Path string
}

type Reference struct {
	Ref  string
	File Instruction
}

// Complete original local file reads, not context, a freshness lease or PASS.
type References struct {
	Version                          string
	StartedAt, CompletedAt           time.Time
	First, Second                    []Reference
	FailureCode                      string
	ErrorText                        []byte
	ErrorBytes                       uint64
	ErrorSHA256, ErrorNotSavedReason string
}

func (r References) Clone() References {
	r.ErrorText = bytes.Clone(r.ErrorText)
	clone := func(in []Reference) []Reference {
		if in == nil {
			return nil
		}
		out := append([]Reference{}, in...)
		for i := range out {
			out[i].File.Raw = bytes.Clone(out[i].File.Raw)
		}
		return out
	}
	r.First, r.Second = clone(r.First), clone(r.Second)
	return r
}

// Derive solely from the actual installed instruction, relative to that skill's
// own directory. No repo/source fallback, sibling skill or Workspace root guess.
func ReferencePlan(root string, instructions []Instruction) ([]ReferenceRequirement, string) {
	if !filepath.IsAbs(root) || filepath.Clean(root) != root || root == string(filepath.Separator) {
		return nil, "reference_source_invalid"
	}
	plan := []ReferenceRequirement{}
	for _, instruction := range instructions {
		id := instruction.SkillID
		if !filepath.IsLocal(id) || filepath.ToSlash(filepath.Clean(id)) != id || id == "." || strings.ContainsAny(id, "\\:\x00\r\n") || instruction.Path != filepath.Join(root, filepath.FromSlash(id), "SKILL.md") || !instruction.Complete || instruction.FailureCode != "" {
			return nil, "reference_source_invalid"
		}
		metadata, err := ParsePreparationSkillHubMetadata(instruction.Raw)
		if err != nil {
			return nil, "reference_metadata_invalid"
		}
		seen := map[string]bool{}
		for _, ref := range metadata.ReferenceRefs {
			if !utf8.ValidString(ref) || len(ref) > 4096 || !filepath.IsLocal(ref) || filepath.ToSlash(filepath.Clean(ref)) != ref || ref == "." || strings.ContainsAny(ref, "\\:\x00\r\n?#") || seen[ref] {
				return nil, "reference_path_invalid"
			}
			seen[ref] = true
			plan = append(plan, ReferenceRequirement{SkillID: id, Ref: ref, Path: filepath.Join(root, filepath.FromSlash(id), filepath.FromSlash(ref))})
			if len(plan) > ReferenceCountLimit {
				return nil, "reference_limit"
			}
		}
	}
	return plan, ""
}

func (r *References) PreserveFailure(code string, err error) {
	r.FailureCode = code
	if err == nil {
		return
	}
	raw := []byte(err.Error())
	if len(raw) == 0 {
		return
	}
	r.ErrorBytes, r.ErrorSHA256 = uint64(len(raw)), fileobservation.Digest(raw)
	if len(raw) > 64<<10 {
		r.ErrorNotSavedReason = "error_exceeds_storage_limit"
		return
	}
	r.ErrorText = raw
}

func (r References) Validate(installed Installed) error {
	if r.Version != ReferencesVersion || r.StartedAt.Before(installed.StartedAt) || r.StartedAt.IsZero() || r.CompletedAt.Before(r.StartedAt) || r.CompletedAt.After(installed.CompletedAt) || len(r.First) > ReferenceCountLimit || len(r.Second) > len(r.First) {
		return common.ErrConflict
	}
	switch r.FailureCode {
	case "", "reference_source_invalid", "reference_metadata_invalid", "reference_path_invalid", "reference_limit", "reference_unavailable", "reference_reobservation_failed", "reference_drift", "reference_root_drift", "cancelled":
	default:
		return common.ErrConflict
	}
	plan, code := ReferencePlan(installed.Root, installed.First)
	if code != "" {
		if r.FailureCode != code || len(r.First) != 0 || len(r.Second) != 0 {
			return common.ErrConflict
		}
	} else {
		if len(r.First) > len(plan) {
			return common.ErrConflict
		}
		for _, files := range [][]Reference{r.First, r.Second} {
			total := 0
			for i, ref := range files {
				f, p := ref.File, plan[i]
				total += len(f.Raw)
				if f.SkillID != p.SkillID || ref.Ref != p.Ref || f.Path != p.Path || len(f.Raw) > InstructionLimit || total > ReferenceBytesLimit || f.ObservedSize < 0 || f.ReadBytes < uint64(len(f.Raw)) || f.RawSHA256 != fileobservation.Digest(f.Raw) || len(f.FailureCode) > 512 || !utf8.ValidString(f.FailureCode) || f.Complete && (f.ReadBytes != uint64(len(f.Raw)) || f.ObservedSize != int64(len(f.Raw))) {
					return common.ErrConflict
				}
			}
		}
	}
	if r.ErrorBytes == 0 {
		if len(r.ErrorText) != 0 || r.ErrorSHA256 != "" || r.ErrorNotSavedReason != "" {
			return common.ErrConflict
		}
	} else if r.ErrorNotSavedReason == "" {
		if len(r.ErrorText) > 64<<10 || uint64(len(r.ErrorText)) != r.ErrorBytes || fileobservation.Digest(r.ErrorText) != r.ErrorSHA256 {
			return common.ErrConflict
		}
	} else if r.ErrorNotSavedReason != "error_exceeds_storage_limit" || len(r.ErrorText) != 0 || r.ErrorBytes <= 64<<10 || len(r.ErrorSHA256) != 64 || strings.Trim(r.ErrorSHA256, "0123456789abcdef") != "" {
		return common.ErrConflict
	}
	return nil
}

func (r References) CompleteFor(installed Installed) bool {
	plan, code := ReferencePlan(installed.Root, installed.First)
	if r.Validate(installed) != nil || code != "" || r.FailureCode != "" || r.ErrorBytes != 0 || r.First == nil || r.Second == nil || len(r.First) != len(plan) || len(r.Second) != len(plan) {
		return false
	}
	for i, a := range r.First {
		b := r.Second[i]
		if !a.File.Complete || !b.File.Complete || a.File.FailureCode != "" || b.File.FailureCode != "" || a.File.ObservedSize != b.File.ObservedSize || !bytes.Equal(a.File.Raw, b.File.Raw) {
			return false
		}
	}
	return true
}
