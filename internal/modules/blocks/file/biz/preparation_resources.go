package file

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	fileevents "github.com/muidea/workorch/internal/modules/blocks/file/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/fileobservation"
)

func (m *Biz) handleObservePreparationResources(ev magicevent.Event, result magicevent.Result) {
	if result == nil {
		return
	}
	if ev == nil || ev.Source() != "blocks/workspace" || ev.ID() != fileevents.TopicObservePreparationResources {
		setPrerequisiteError(result, common.ErrInvalidArgument)
		return
	}
	c, ok := ev.Data().(fileevents.ObservePreparationResourcesCommand)
	if !ok || c.Validate() != nil {
		setPrerequisiteError(result, common.ErrInvalidArgument)
		return
	}
	ctx, err := common.BindRuntimeWorkspaceContext(ev.Context(), c.WorkspaceID)
	if setPrerequisiteError(result, err) || setPrerequisiteError(result, ctx.Err()) {
		return
	}
	r := fileevents.ObservePreparationResourcesResult{Version: fileevents.PreparationResourcesVersion, Command: c.Clone(), StartedAt: time.Now().UTC()}
	remaining := fileobservation.ResourceTotalBytes
	for _, ref := range c.ResourceRefs {
		if setPrerequisiteError(result, ctx.Err()) {
			return
		}
		item := observePreparationResource(c.DeliverableDirectory, ref, remaining)
		remaining -= item.ContentBytes
		r.Resources = append(r.Resources, item)
	}
	r.CompletedAt = time.Now().UTC()
	if setPrerequisiteError(result, ctx.Err()) || setPrerequisiteError(result, r.ValidateFor(c)) {
		return
	}
	result.Set(r, nil)
}

func failedPreparationResource(ref string, err error) fileobservation.Resource {
	raw := []byte(err.Error())
	r := fileobservation.Resource{Ref: ref, ContentSHA256: fileobservation.Digest(nil), Diagnostic: raw, DiagnosticBytes: len(raw), DiagnosticSHA256: fileobservation.Digest(raw)}
	if len(raw) > fileobservation.DiagnosticLimit {
		r.Diagnostic, r.DiagnosticNotSavedReason = nil, "diagnostic_exceeds_byte_boundary"
	}
	return r
}

func observePreparationResource(root, ref string, remaining int) fileobservation.Resource {
	fail := func(err error) fileobservation.Resource { return failedPreparationResource(ref, err) }
	// Preserve the existing authoring path authorization boundary. Unsupported
	// URI/absolute/traversal/sensitive refs are explicit failures, never guessed.
	if strings.Contains(ref, ":") || !runtimeFileDeliverableTargetEligible(ref) {
		return fail(common.ErrInvalidArgument)
	}
	for _, segment := range strings.Split(ref, "/") {
		if editApprovalRequired(segment) || strings.HasPrefix(segment, ".") {
			return fail(common.ErrInvalidArgument)
		}
	}
	_, _, cleanRef, err := guardedWorkspacePath(root, ref, true)
	if err != nil {
		return fail(err)
	}
	if filepath.ToSlash(cleanRef) != ref {
		return fail(common.ErrInvalidArgument)
	}
	return observePreparationFile(root, ref, remaining)
}

// The caller supplies the independently authorized canonical target. Both
// observation contracts retain the same confined, pinned, bounded file read.
func observePreparationFile(root, ref string, remaining int) fileobservation.Resource {
	fail := func(err error) fileobservation.Resource { return failedPreparationResource(ref, err) }
	f, err := openAuthoringResource(root, ref)
	if err != nil {
		return fail(err)
	}
	defer f.Close()
	before, err := f.Stat()
	if err != nil {
		return fail(err)
	}
	limit := min(remaining, fileobservation.ResourceMaxBytes)
	if before.Size() > int64(limit) {
		return fail(fmt.Errorf("resource content not saved: complete file exceeds observation byte boundary"))
	}
	raw, err := io.ReadAll(io.LimitReader(f, int64(limit)+1))
	if err != nil {
		return fail(err)
	}
	if len(raw) > limit || int64(len(raw)) != before.Size() {
		return fail(fmt.Errorf("resource content not saved: incomplete or oversized read"))
	}
	after, err := f.Stat()
	if err != nil {
		return fail(err)
	}
	if !os.SameFile(before, after) || !sameAuthoringFileVersion(before, after) {
		return fail(common.ErrConflict)
	}
	pathInfo, err := statAuthoringResource(root, ref)
	if err != nil {
		return fail(err)
	}
	if !os.SameFile(before, pathInfo) || !sameAuthoringFileVersion(before, pathInfo) {
		return fail(common.ErrConflict)
	}
	r := fileobservation.Resource{Ref: ref, Complete: true, Content: raw, ContentBytes: len(raw), ContentSHA256: fileobservation.Digest(raw), DiagnosticSHA256: fileobservation.Digest(nil)}
	if err := preparationResourceIdentity(before, &r); err != nil {
		return fail(err)
	}
	return r
}
