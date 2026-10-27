package biz

import (
	"context"
	"path/filepath"
	"time"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/contextregistryports"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/fileobservation"
	"github.com/muidea/workorch/internal/pkg/skillhubpreparation"
)

// Only the first accepted continuation calls this adapter. Recovery never
// repeats status: its external service/save effects cannot be inferred absent.
func (b *Biz) observeReservedPreparationSkillHub(ctx context.Context, plan workspace.WorkspacePreparationSkillHubEffects) (*workspace.PreparationSkillHubStatusFacts, *workspace.PreparationSkillHubStatusFacts) {
	notStarted := func(scope, directory, code string, err error) *workspace.PreparationSkillHubStatusFacts {
		now := time.Now().UTC()
		f := workspace.PreparationSkillHubStatusFacts{Version: "workorch-skillhub-preparation-observation/v2", EffectContract: "skillhub-status-effects/v1", Scope: scope, Directory: directory, StartedAt: now, CompletedAt: now, FailureCode: code}
		preservePreparationSkillHubError(&f, err)
		return &f
	}
	stop := func(code string, err error) (*workspace.PreparationSkillHubStatusFacts, *workspace.PreparationSkillHubStatusFacts) {
		return notStarted("global", "", code, err), notStarted("workspace", plan.ProjectDirectory, code, err)
	}
	if plan.Validate() != nil || !plan.CompletedAt.IsZero() {
		return stop("invalid_plan_not_started", common.ErrConflict)
	}
	if !filepath.IsAbs(plan.ProjectDirectory) || filepath.Clean(plan.ProjectDirectory) != plan.ProjectDirectory {
		return stop("directory_invalid_not_started", common.ErrConflict)
	}
	if b.preparationSkillHubRunner == nil {
		return stop("adapter_unavailable_not_started", common.ErrConflict)
	}
	before, err := b.QueryExactConfirmedStartupSnapshot(ctx, workspaceevents.QueryExactConfirmedStartupSnapshotCommand{WorkspaceID: plan.WorkspaceID, ExpectedWorkspaceRevision: plan.ExpectedWorkspaceRevision + 1})
	if err != nil {
		return stop("seal_unavailable_not_started", err)
	}
	s := before.Snapshot
	binding := plan.Binding
	if s.AuthoringRef != binding.AuthoringRef || s.Candidate.CandidateID != binding.CandidateID || s.Candidate.CandidateRevision != binding.CandidateRevision || s.Candidate.CandidateHash != binding.CandidateHash || s.StartupInput.DefinitionRevision != binding.DefinitionRevision || s.StartupInput.StartupInputHash != binding.StartupInputHash || s.AuthoringConfigurationHash != binding.AuthoringConfigurationHash || s.StartupInput.ResolvedProjectWorkspaceDirectory != plan.ProjectDirectory {
		return stop("binding_changed_not_started", common.ErrConflict)
	}
	observe := func(dir string, global bool) (*workspace.PreparationSkillHubStatusFacts, error) {
		o, err := contextregistryports.ObserveSkillHubPreparation(ctx, b.preparationSkillHubRunner, plan.WorkspaceID, dir, global)
		f := workspace.PreparationSkillHubStatusFacts{Version: o.Version, EffectContract: o.EffectContract, Scope: o.Scope, Directory: o.Directory, StartedAt: o.StartedAt, CompletedAt: o.CompletedAt, RawOutput: o.RawOutput, RawStderr: o.RawStderr, RawSHA256: o.RawSHA256, StderrSHA256: o.StderrSHA256, TotalOutputBytes: o.TotalOutputBytes, TotalStderrBytes: o.TotalStderrBytes, OutputComplete: o.OutputComplete, StderrComplete: o.StderrComplete, CommandStarted: o.CommandStarted, ExitCodeKnown: o.ExitCodeKnown, ExitCode: o.ExitCode, FailureCode: o.FailureCode}
		preservePreparationSkillHubError(&f, err)
		if err == nil && o.CommandStarted && o.ExitCodeKnown && o.ExitCode == 0 {
			installed, readErr := contextregistryports.ObserveSkillHubInstalledFromCapture(ctx, o)
			facts := skillhubpreparation.Installed{Version: skillhubpreparation.InstalledVersion, Root: installed.InstalledRoot, StartedAt: installed.StartedAt, CompletedAt: installed.CompletedAt, FailureCode: installed.FailureCode}
			if installed.References != nil {
				r := installed.References.Clone()
				facts.References = &r
			}
			mapFiles := func(in []contextregistryports.SkillHubPreparationInstruction) []skillhubpreparation.Instruction {
				if in == nil {
					return nil
				}
				out := make([]skillhubpreparation.Instruction, len(in))
				for i, file := range in {
					out[i] = skillhubpreparation.Instruction{SkillID: file.SkillID, Path: file.Path, Raw: file.Raw, RawSHA256: file.RawSHA256, ObservedSize: file.ObservedSize, ReadBytes: file.ReadBytes, Complete: file.Complete, FailureCode: file.FailureCode}
				}
				return out
			}
			facts.First = mapFiles(installed.Instructions)
			facts.Second = mapFiles(installed.ReobservedInstructions)
			if readErr == nil {
				for _, file := range facts.First {
					if ctxErr := ctx.Err(); ctxErr != nil {
						facts.FailureCode, readErr = "cancelled", ctxErr
						break
					}
					if _, parseErr := skillhubpreparation.ParsePreparationSkillHubMetadata(file.Raw); parseErr != nil {
						facts.FailureCode = "invalid_metadata"
						readErr = parseErr
						break
					}
				}
				if readErr == nil && ctx.Err() != nil {
					facts.FailureCode, readErr = "cancelled", ctx.Err()
				}
			}
			var diagnostic workspace.PreparationSkillHubStatusFacts
			preservePreparationSkillHubError(&diagnostic, readErr)
			facts.ErrorText, facts.ErrorBytes, facts.ErrorSHA256, facts.ErrorNotSavedReason = diagnostic.ErrorText, diagnostic.ErrorBytes, diagnostic.ErrorSHA256, diagnostic.ErrorNotSavedReason
			f.Installed = &facts
		}
		return &f, err
	}
	global, err := observe("", true)
	if err != nil {
		return global, notStarted("workspace", plan.ProjectDirectory, "preceding_status_failed_not_started", nil)
	}
	project, _ := observe(plan.ProjectDirectory, false)
	return global, project
}

func preservePreparationSkillHubError(f *workspace.PreparationSkillHubStatusFacts, err error) {
	if err == nil {
		return
	}
	raw := []byte(err.Error())
	if len(raw) == 0 {
		return
	}
	f.ErrorBytes = uint64(len(raw))
	f.ErrorSHA256 = fileobservation.Digest(raw)
	if len(raw) > workspace.PreparationSkillHubErrorLimit {
		f.ErrorNotSavedReason = "error_exceeds_storage_limit"
		return
	}
	f.ErrorText = raw
}
