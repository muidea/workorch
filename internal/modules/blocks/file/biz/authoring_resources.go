package file

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	magicevent "github.com/muidea/magicCommon/event"
	fileevents "github.com/muidea/workorch/internal/modules/blocks/file/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	workorchevents "github.com/muidea/workorch/internal/pkg/events"
)

const authoringApplicationSource = "application/workorchd"

func (m *Biz) handleVerifyAuthoringResource(event magicevent.Event, result magicevent.Result) {
	if result == nil {
		return
	}
	if event == nil || event.Source() != workspaceevents.CommandDestination {
		setPrerequisiteError(result, common.ErrInvalidArgument)
		return
	}
	command, ok := event.Data().(fileevents.VerifyAuthoringResourceCommand)
	if !ok || command.Validate() != nil {
		setPrerequisiteError(result, common.ErrInvalidArgument)
		return
	}
	query := fileevents.AcquireAuthoringResourceCommand{WorkspaceID: command.WorkspaceID, AuthoringInputHash: command.AuthoringInputHash, ResourceRef: command.ResourceRef}
	fact, err := m.acquireAuthoringResource(event, query)
	if setPrerequisiteError(result, err) {
		return
	}
	if fact.WorkspaceRevision != command.WorkspaceRevision || fact.ContentHash != command.ContentHash || fact.Content != command.Content {
		setPrerequisiteError(result, common.ErrConflict)
		return
	}
	result.Set(fileevents.VerifyAuthoringResourceResult{WorkspaceID: fact.WorkspaceID, WorkspaceRevision: fact.WorkspaceRevision, AuthoringInputHash: fact.AuthoringInputHash, ResourceRef: fact.ResourceRef, ContentHash: fact.ContentHash}, nil)
}

func (m *Biz) handleAcquireAuthoringResource(event magicevent.Event, result magicevent.Result) {
	if result == nil {
		return
	}
	if event == nil || event.Source() != authoringApplicationSource {
		setPrerequisiteError(result, fmt.Errorf("%w: authoring File query requires the exact Application source", common.ErrInvalidArgument))
		return
	}
	command, ok := event.Data().(fileevents.AcquireAuthoringResourceCommand)
	if !ok || command.Validate() != nil {
		setPrerequisiteError(result, common.ErrInvalidArgument)
		return
	}
	response, err := m.acquireAuthoringResource(event, command)
	if setPrerequisiteError(result, err) {
		return
	}
	result.Set(response, nil)
}

func (m *Biz) acquireAuthoringResource(event magicevent.Event, command fileevents.AcquireAuthoringResourceCommand) (fileevents.AcquireAuthoringResourceResult, error) {
	if command.Validate() != nil || m == nil || m.runtimeAdapter == nil {
		return fileevents.AcquireAuthoringResourceResult{}, common.ErrInvalidArgument
	}
	if !runtimeFileDeliverableTargetEligible(command.ResourceRef) {
		return fileevents.AcquireAuthoringResourceResult{}, common.ErrInvalidArgument
	}
	for _, segment := range strings.Split(command.ResourceRef, "/") {
		if editApprovalRequired(segment) || strings.HasPrefix(segment, ".") {
			return fileevents.AcquireAuthoringResourceResult{}, fmt.Errorf("%w: authoring resource has a sensitive path", common.ErrInvalidArgument)
		}
	}
	scope, err := m.authoringWorkspaceScope(event, command)
	if err != nil {
		return fileevents.AcquireAuthoringResourceResult{}, err
	}
	root := scope.DeliverableDirectory
	_, target, ref, err := guardedWorkspacePath(root, command.ResourceRef, true)
	if err != nil || filepath.ToSlash(ref) != command.ResourceRef {
		return fileevents.AcquireAuthoringResourceResult{}, common.ErrInvalidArgument
	}
	// The owner-private opener rejects symlinks, mount transitions and linked
	// inodes before returning a regular file. The workspace root comes only
	// from the Workspace-owner scope, never from Application's command.
	file, err := openAuthoringResource(root, command.ResourceRef)
	if err != nil {
		return fileevents.AcquireAuthoringResourceResult{}, err
	}
	defer file.Close()
	openedInfo, err := file.Stat()
	if err != nil {
		return fileevents.AcquireAuthoringResourceResult{}, err
	}
	if !openedInfo.Mode().IsRegular() || openedInfo.Size() > fileevents.AuthoringResourceMaxBytes {
		return fileevents.AcquireAuthoringResourceResult{}, fmt.Errorf("%w: authoring resource is not a bounded regular file", common.ErrInvalidArgument)
	}
	content, err := io.ReadAll(io.LimitReader(file, fileevents.AuthoringResourceMaxBytes+1))
	if err != nil {
		return fileevents.AcquireAuthoringResourceResult{}, err
	}
	if len(content) > fileevents.AuthoringResourceMaxBytes || int64(len(content)) != openedInfo.Size() || !utf8.Valid(content) || strings.ContainsRune(string(content), 0) || containsRuntimeRedactionMarker(string(content)) || containsAuthoringSensitiveContent(string(content)) {
		return fileevents.AcquireAuthoringResourceResult{}, fmt.Errorf("%w: authoring resource is incomplete, binary or sensitive", common.ErrInvalidArgument)
	}
	// A changed path/symlink after the read is not an authoritative snapshot.
	_, observed, currentRef, err := guardedWorkspacePath(root, command.ResourceRef, true)
	if err != nil || observed != target || filepath.ToSlash(currentRef) != command.ResourceRef {
		return fileevents.AcquireAuthoringResourceResult{}, fmt.Errorf("%w: authoring resource changed during observation", common.ErrConflict)
	}
	currentInfo, err := file.Stat()
	if err != nil || !os.SameFile(openedInfo, currentInfo) || !sameAuthoringFileVersion(openedInfo, currentInfo) {
		return fileevents.AcquireAuthoringResourceResult{}, fmt.Errorf("%w: authoring resource changed during observation", common.ErrConflict)
	}
	pathInfo, err := statAuthoringResource(root, command.ResourceRef)
	if err != nil || !os.SameFile(openedInfo, pathInfo) || !sameAuthoringFileVersion(openedInfo, pathInfo) {
		return fileevents.AcquireAuthoringResourceResult{}, fmt.Errorf("%w: authoring resource was replaced during observation", common.ErrConflict)
	}
	latest, err := m.authoringWorkspaceScope(event, command)
	if err != nil || latest != scope {
		return fileevents.AcquireAuthoringResourceResult{}, fmt.Errorf("%w: authoring workspace changed during observation", common.ErrConflict)
	}
	sum := sha256.Sum256(content)
	response := fileevents.AcquireAuthoringResourceResult{WorkspaceID: command.WorkspaceID, WorkspaceRevision: scope.WorkspaceRevision, AuthoringInputHash: command.AuthoringInputHash, ResourceRef: command.ResourceRef, Content: string(content), ContentHash: "sha256:" + hex.EncodeToString(sum[:]), Size: int64(len(content))}
	if err := response.ValidateFor(command); err != nil {
		return fileevents.AcquireAuthoringResourceResult{}, err
	}
	return response, nil
}

func (m *Biz) authoringWorkspaceScope(event magicevent.Event, command fileevents.AcquireAuthoringResourceCommand) (workspaceevents.QueryAuthoringResourceScopeResult, error) {
	query := workspaceevents.QueryAuthoringResourceScopeCommand{WorkspaceID: command.WorkspaceID, InputHash: command.AuthoringInputHash, ResourceRef: command.ResourceRef}
	ev := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceAuthoringResourceScopeQuery, ModuleID, workspaceevents.CommandDestination, magicevent.NewHeader(), event.Context(), query)
	ev.BindLaneKey(workorchevents.EventHubLaneKeyFor(workspaceevents.Contracts(), workspaceevents.TopicWorkspaceAuthoringResourceScopeQuery, eventcontract.LaneScope{WorkspaceID: string(command.WorkspaceID)}))
	result := m.SendEvent(ev)
	if result == nil {
		return workspaceevents.QueryAuthoringResourceScopeResult{}, common.ErrConflict
	}
	if err := result.Error(); err != nil {
		return workspaceevents.QueryAuthoringResourceScopeResult{}, err
	}
	value, err := result.Get()
	if err != nil {
		return workspaceevents.QueryAuthoringResourceScopeResult{}, err
	}
	view, ok := value.(workspaceevents.QueryAuthoringResourceScopeResult)
	if !ok || view.ValidateFor(query) != nil {
		return workspaceevents.QueryAuthoringResourceScopeResult{}, common.ErrConflict
	}
	return view, nil
}

// This is deliberately conservative rather than a claim of full secret
// detection: unsupported or recognizable credential-shaped input stays local.
func containsAuthoringSensitiveContent(content string) bool {
	upper := strings.ToUpper(content)
	for _, marker := range []string{"PRIVATE KEY-----", "OPENSSH PRIVATE KEY", "API_KEY=", "API_TOKEN=", "PASSWORD=", "SECRET_KEY=", "AUTHORIZATION: BEARER ", "GHP_", "GHO_", "GHU_", "GHS_", "GHR_"} {
		if strings.Contains(upper, marker) {
			return true
		}
	}
	return false
}
