package file

import (
	"path/filepath"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	fileevents "github.com/muidea/workorch/internal/modules/blocks/file/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/fileobservation"
	"github.com/muidea/workorch/internal/pkg/workorchmanagedconfig"
)

// Explicit, convergent infrastructure creation only. Existing bytes are never
// overwritten. Completion is not resource readiness or Runtime proof.
func (m *Biz) handleInitializePreparationResources(ev magicevent.Event, result magicevent.Result) {
	if result == nil {
		return
	}
	if ev == nil || ev.Source() != "blocks/workspace" || ev.ID() != fileevents.TopicInitializePreparationResources {
		setPrerequisiteError(result, common.ErrInvalidArgument)
		return
	}
	c, ok := ev.Data().(fileevents.InitializePreparationResourcesCommand)
	if !ok || c.Validate() != nil {
		setPrerequisiteError(result, common.ErrInvalidArgument)
		return
	}
	ctx, err := common.BindRuntimeWorkspaceContext(ev.Context(), c.WorkspaceID)
	if setPrerequisiteError(result, err) || setPrerequisiteError(result, ctx.Err()) {
		return
	}
	m.preparationInitializationMu.Lock()
	defer m.preparationInitializationMu.Unlock()
	rootErr := validatePreparationInitializationRoots(c.InternalDirectory, c.DeliverableDirectory)
	r := fileevents.InitializePreparationResourcesResult{Version: fileevents.PreparationInitializationVersion, Command: c.Clone(), StartedAt: time.Now().UTC(), RootDiagnostic: initializationDiagnostic(rootErr)}
	remainingBefore, remainingAfter := fileobservation.ResourceTotalBytes, fileobservation.ResourceTotalBytes
	for _, d := range c.Declarations {
		if setPrerequisiteError(result, ctx.Err()) {
			return
		}
		root := c.InternalDirectory
		if d.Scope == workorchmanagedconfig.ScopeSystem {
			root = workorchmanagedconfig.SystemDirectory(root)
		}
		before := observePreparationFile(root, d.Path, remainingBefore)
		remainingBefore -= before.ContentBytes
		// Only missing resources may be created. Failed reads of existing resources
		// (including invalid/symlink/hardlink targets) are never repaired or replaced.
		raw := d.DefaultsJSON
		writeErr := rootErr
		if rootErr == nil {
			writeErr = createPreparationConfig(root, d.Path, raw)
		}
		after := observePreparationFile(root, d.Path, remainingAfter)
		remainingAfter -= after.ContentBytes
		reason := "managed_config_observation_incomplete"
		if after.Complete {
			reason = workorchmanagedconfig.PreparationJSONReason(d.Policy, d.DefaultsJSON, after.Content)
		}
		r.Files = append(r.Files, fileevents.InitializedPreparationConfig{Before: before, After: after, Reason: reason, WriteDiagnostic: initializationDiagnostic(writeErr)})
	}
	skillRoot := filepath.Join(c.DeliverableDirectory, ".agents", "skills")
	if rootErr == nil {
		r.SkillWriteDiagnostic = initializationDiagnostic(createPreparationSkillDirectory(c.DeliverableDirectory))
	}
	r.ProjectSkills = observePreparationDirectory(skillRoot)
	r.CompletedAt = time.Now().UTC()
	if setPrerequisiteError(result, r.ValidateFor(c)) {
		return
	}
	result.Set(r, nil)
}

func initializationDiagnostic(err error) fileevents.InitializationDiagnostic {
	if err == nil {
		return fileevents.InitializationDiagnostic{}
	}
	raw := []byte(err.Error())
	d := fileevents.InitializationDiagnostic{Raw: raw, Bytes: len(raw), SHA256: fileobservation.Digest(raw)}
	if len(raw) > fileobservation.DiagnosticLimit {
		d.Raw = nil
		d.NotSavedReason = "diagnostic_exceeds_byte_boundary"
	}
	return d
}
