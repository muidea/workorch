package contextregistryports

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/fileobservation"
	"github.com/muidea/workorch/internal/pkg/skillhubpreparation"
)

func preparationInstructionFact(f SkillHubPreparationInstruction) skillhubpreparation.Instruction {
	return skillhubpreparation.Instruction{SkillID: f.SkillID, Path: f.Path, Raw: bytes.Clone(f.Raw), RawSHA256: f.RawSHA256, ObservedSize: f.ObservedSize, ReadBytes: f.ReadBytes, Complete: f.Complete, FailureCode: f.FailureCode}
}

func observePreparationSkillReferences(ctx context.Context, root *os.Root, instructions []SkillHubPreparationInstruction) *skillhubpreparation.References {
	r := &skillhubpreparation.References{Version: skillhubpreparation.ReferencesVersion, StartedAt: time.Now().UTC()}
	defer func() { r.CompletedAt = time.Now().UTC() }()
	fail := func(code string, err error) *skillhubpreparation.References { r.PreserveFailure(code, err); return r }
	facts := make([]skillhubpreparation.Instruction, len(instructions))
	for i, f := range instructions {
		facts[i] = preparationInstructionFact(f)
	}
	plan, code := skillhubpreparation.ReferencePlan(root.Name(), facts)
	if code != "" {
		return fail(code, common.ErrConflict)
	}
	r.First, r.Second = []skillhubpreparation.Reference{}, []skillhubpreparation.Reference{}
	for round := range 2 {
		total := 0
		for i, p := range plan {
			if err := ctx.Err(); err != nil {
				return fail("cancelled", err)
			}
			file, err := readPreparationSkillReference(ctx, root, p, min(SkillHubPreparationInstructionLimit, skillhubpreparation.ReferenceBytesLimit-total))
			ref := skillhubpreparation.Reference{Ref: p.Ref, File: preparationInstructionFact(file)}
			total += len(file.Raw)
			if round == 0 {
				r.First = append(r.First, ref)
			} else {
				r.Second = append(r.Second, ref)
			}
			if err != nil {
				if round == 0 {
					return fail("reference_unavailable", err)
				}
				return fail("reference_reobservation_failed", err)
			}
			if round == 1 && (file.ObservedSize != r.First[i].File.ObservedSize || !bytes.Equal(file.Raw, r.First[i].File.Raw)) {
				return fail("reference_drift", common.ErrConflict)
			}
		}
	}
	return r
}

func readPreparationSkillReference(ctx context.Context, installedRoot *os.Root, p skillhubpreparation.ReferenceRequirement, limit int) (SkillHubPreparationInstruction, error) {
	missing := SkillHubPreparationInstruction{SkillID: p.SkillID, Path: p.Path, RawSHA256: fileobservation.Digest(nil), FailureCode: "reference_root_unavailable"}
	if _, err := preparationInstructionInfo(installedRoot, filepath.Join(filepath.FromSlash(p.SkillID), "SKILL.md")); err != nil {
		return missing, err
	}
	// Pin the individual skill directory, not the whole catalog: even a raced
	// internal symlink cannot escape to a sibling skill's files.
	before, err := installedRoot.Lstat(filepath.FromSlash(p.SkillID))
	if err != nil {
		return missing, err
	}
	if !before.IsDir() || before.Mode()&os.ModeSymlink != 0 {
		return missing, common.ErrConflict
	}
	root, err := installedRoot.OpenRoot(filepath.FromSlash(p.SkillID))
	if err != nil {
		return missing, err
	}
	defer root.Close()
	opened, err := root.Stat(".")
	current, currentErr := installedRoot.Lstat(filepath.FromSlash(p.SkillID))
	if err != nil || currentErr != nil || !os.SameFile(before, opened) || !os.SameFile(before, current) || current.Mode()&os.ModeSymlink != 0 {
		return missing, common.ErrConflict
	}
	f, readErr := readPreparationSkillBytes(ctx, root, filepath.FromSlash(p.Ref), p.SkillID, limit)
	after, err := installedRoot.Lstat(filepath.FromSlash(p.SkillID))
	if err != nil || !os.SameFile(before, after) || after.Mode()&os.ModeSymlink != 0 {
		f.FailureCode = "reference_root_drift"
		return f, common.ErrConflict
	}
	return f, readErr
}
