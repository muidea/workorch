package orchestrator

// Local prerequisite mapping is internal to the owner Biz.

import (
	cryptorand "crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const validationScratchRevisionFile = ".workorch-runtime-revision"

func ResolveFailureTargetPrerequisite(facts runtimecontract.RuntimeFacts, prerequisiteID string) (runtimecontract.RuntimeFacts, error) {
	prerequisite, err := pendingPrerequisite(facts, prerequisiteID, runtimecontract.PrerequisiteKindFailureTarget)
	if err != nil {
		return runtimecontract.RuntimeFacts{}, err
	}
	if facts.Failure == nil || strings.TrimSpace(facts.Failure.Signature) == "" || len(facts.Failure.TargetRefs) == 0 {
		return runtimecontract.RuntimeFacts{}, fmt.Errorf("failure target prerequisite %q has no typed failure targets", prerequisite.ID)
	}
	return runtime.CommitPrerequisiteResult(facts, runtime.PrerequisiteResult{
		PrerequisiteID: prerequisite.ID,
		FailureTarget: &runtimecontract.FailureTarget{
			Signature:  facts.Failure.Signature,
			TargetRefs: facts.Failure.TargetRefs,
		},
	})
}

// AcquireValidationScratchPrerequisite derives a process-independent scratch
// reference below the Workspace owner's persisted internal directory. Scratch
// is a local SM-2 prerequisite, never a process-global temp directory or a
// deliverable artifact; using a workspace-scoped root makes the committed
// reference meaningful after a process restart and keeps test/runtime roots
// isolated.
func AcquireValidationScratchPrerequisite(facts runtimecontract.RuntimeFacts, workspaceInternalDirectory string, prerequisiteID string) (runtimecontract.RuntimeFacts, error) {
	prerequisite, err := pendingPrerequisite(facts, prerequisiteID, runtimecontract.PrerequisiteKindValidationScratch)
	if err != nil {
		return runtimecontract.RuntimeFacts{}, err
	}
	if facts.WorkspaceID == "" || facts.RunID == "" {
		return runtimecontract.RuntimeFacts{}, common.ErrInvalidArgument
	}
	root := strings.TrimSpace(workspaceInternalDirectory)
	if root == "" {
		return runtimecontract.RuntimeFacts{}, fmt.Errorf("validation scratch prerequisite %q requires workspace internal directory", prerequisite.ID)
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return runtimecontract.RuntimeFacts{}, err
	}
	identity, err := validationScratchScopedIdentity(facts, prerequisite)
	if err != nil {
		return runtimecontract.RuntimeFacts{}, err
	}
	// Keep the durable location and the freshness subject derived from the
	// same persisted prerequisite scope. A global scratch therefore survives a
	// focus advance, while a focus scratch cannot be reused by another focus.
	reference := filepath.Join(root, filepath.FromSlash(identity))
	revision, err := ensureValidationScratchRevision(reference)
	if err != nil {
		return runtimecontract.RuntimeFacts{}, err
	}
	scratch := &runtimecontract.ValidationScratch{
		Reference: reference,
		Revision:  revision,
	}
	return runtime.CommitPrerequisiteResult(facts, runtime.PrerequisiteResult{
		PrerequisiteID:    prerequisite.ID,
		ValidationScratch: scratch,
		Observation:       validationScratchObservation(facts, prerequisite, *scratch),
	})
}

// ensureValidationScratchRevision gives each durable directory incarnation a
// local opaque revision. The path alone cannot distinguish a directory that
// was deleted and recreated after its prerequisite observation was retired;
// the marker makes that stale/reacquire boundary explicit without exporting
// filesystem behavior into the contract package.
func ensureValidationScratchRevision(reference string) (string, error) {
	if err := os.MkdirAll(reference, 0o755); err != nil {
		return "", err
	}
	marker := filepath.Join(reference, validationScratchRevisionFile)
	revision, err := readValidationScratchRevision(marker)
	if err == nil {
		return revision, nil
	}
	if !os.IsNotExist(err) {
		return "", err
	}
	entropy := make([]byte, 16)
	if _, err := cryptorand.Read(entropy); err != nil {
		return "", err
	}
	revision = "scratch-" + hex.EncodeToString(entropy)
	file, err := os.OpenFile(marker, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if os.IsExist(err) {
			concurrent, readErr := readValidationScratchRevision(marker)
			if readErr != nil {
				return "", readErr
			}
			return concurrent, nil
		}
		return "", err
	}
	if _, err := file.WriteString(revision + "\n"); err != nil {
		_ = file.Close()
		return "", err
	}
	if err := file.Close(); err != nil {
		return "", err
	}
	return revision, nil
}

func readValidationScratchRevision(marker string) (string, error) {
	file, err := os.Open(marker)
	if err != nil {
		return "", err
	}
	defer file.Close()
	contents, err := io.ReadAll(io.LimitReader(file, 257))
	if err != nil {
		return "", err
	}
	if len(contents) > 256 {
		return "", fmt.Errorf("validation scratch revision marker %q exceeds 256 bytes", marker)
	}
	revision := strings.TrimSpace(string(contents))
	if revision == "" {
		return "", fmt.Errorf("validation scratch revision marker %q is empty", marker)
	}
	return revision, nil
}

// validationScratchObservation binds a local scratch reference to one stable,
// scoped resource identity. The contract package only stores this DTO; the
// filesystem probe and any ensuing state transition remain in Orchestrator.
func validationScratchObservation(facts runtimecontract.RuntimeFacts, prerequisite runtimecontract.RuntimePrerequisite, scratch runtimecontract.ValidationScratch) *runtimecontract.RuntimePrerequisiteObservation {
	identity, err := validationScratchScopedIdentity(facts, prerequisite)
	if err != nil {
		return nil
	}
	content := strings.TrimSpace(scratch.Reference) + "\x00" + strings.TrimSpace(scratch.Revision)
	sum := sha256.Sum256([]byte(content))
	observation := runtimecontract.RuntimePrerequisiteObservation{
		PrerequisiteID: prerequisite.ID,
		Kind:           runtimecontract.PrerequisiteKindValidationScratch,
		OwnerModuleID:  runtimecontract.RuntimeModuleOrchestrator,
		ContentHash:    hex.EncodeToString(sum[:]),
		Dependencies: []runtimecontract.RuntimeFreshnessDependency{{
			Domain:        runtimecontract.RuntimeResourceLocalResource,
			OwnerModuleID: runtimecontract.RuntimeModuleOrchestrator,
			SubjectRef:    identity,
			Revision:      scratch.Revision,
		}},
	}
	observation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(observation)
	return &observation
}

func validationScratchScopedIdentity(facts runtimecontract.RuntimeFacts, prerequisite runtimecontract.RuntimePrerequisite) (string, error) {
	if facts.WorkspaceID == "" || facts.RunID == "" {
		return "", common.ErrInvalidArgument
	}
	parts := []string{"validation-scratch", safePrerequisitePathPart(string(facts.WorkspaceID)), safePrerequisitePathPart(string(facts.RunID))}
	switch prerequisite.Scope {
	case runtimecontract.RuntimePrerequisiteScopeGlobal:
		parts = append(parts, "global")
	case runtimecontract.RuntimePrerequisiteScopeFocus:
		if prerequisite.FocusID == "" || prerequisite.FocusID != facts.Focus.ID {
			return "", fmt.Errorf("validation scratch prerequisite %q is not bound to current focus", prerequisite.ID)
		}
		parts = append(parts, "focus", safePrerequisitePathPart(prerequisite.FocusID))
	default:
		return "", fmt.Errorf("validation scratch prerequisite %q has unsupported scope %q", prerequisite.ID, prerequisite.Scope)
	}
	return filepath.ToSlash(filepath.Join(parts...)), nil
}

// InspectValidationScratchPrerequisite is a read-only SM-2 probe. A missing,
// non-directory or mode-read-only committed scratch cannot be passed to a
// candidate; the caller must retire its exact persisted observation through
// the regular prerequisite stale transition before reacquiring it.
func InspectValidationScratchPrerequisite(facts runtimecontract.RuntimeFacts) (*runtimecontract.RuntimePrerequisiteStale, error) {
	facts = facts.Normalized()
	scratch := facts.Resources.ValidationScratch
	if scratch == nil || strings.TrimSpace(scratch.Reference) == "" {
		return nil, nil
	}
	var prerequisite runtimecontract.RuntimePrerequisite
	for _, value := range facts.Prerequisites.Satisfied {
		if value.Kind == runtimecontract.PrerequisiteKindValidationScratch {
			prerequisite = value
			break
		}
	}
	if prerequisite.ID == "" {
		return nil, fmt.Errorf("validation scratch has no satisfied prerequisite")
	}
	observation, found := runtimecontract.RuntimePrerequisiteObservationForID(facts.Resources.PrerequisiteObservations, prerequisite.ID)
	if !found {
		return nil, fmt.Errorf("validation scratch prerequisite %q has no freshness observation", prerequisite.ID)
	}
	identity, identityErr := validationScratchScopedIdentity(facts, prerequisite)
	if identityErr != nil {
		return nil, identityErr
	}
	var dependency runtimecontract.RuntimeFreshnessDependency
	for _, value := range observation.Dependencies {
		if value.Domain == runtimecontract.RuntimeResourceLocalResource && value.OwnerModuleID == runtimecontract.RuntimeModuleOrchestrator && value.SubjectRef == identity {
			dependency = value
			break
		}
	}
	if dependency.Revision == "" {
		return nil, fmt.Errorf("validation scratch prerequisite %q has no local resource dependency", prerequisite.ID)
	}
	status := ""
	info, err := os.Stat(scratch.Reference)
	switch {
	case err != nil && os.IsNotExist(err):
		status = "missing"
	case err != nil:
		status = "unavailable"
	case !info.IsDir():
		status = "not-directory"
	case info.Mode().Perm()&0o222 == 0:
		status = "not-writable"
	}
	if status == "" {
		revision, markerErr := readValidationScratchRevision(filepath.Join(scratch.Reference, validationScratchRevisionFile))
		switch {
		case markerErr != nil && os.IsNotExist(markerErr):
			status = "revision-missing"
		case markerErr != nil:
			status = "revision-unavailable"
		case revision != strings.TrimSpace(scratch.Revision):
			status = "revision-mismatch"
		default:
			return nil, nil
		}
	}
	sum := sha256.Sum256([]byte(strings.TrimSpace(scratch.Reference) + "\x00" + status))
	return &runtimecontract.RuntimePrerequisiteStale{
		PrerequisiteID: prerequisite.ID, ObservationID: observation.ObservationID,
		Domain: dependency.Domain, OwnerModuleID: dependency.OwnerModuleID, SubjectRef: dependency.SubjectRef,
		ExpectedRevision: dependency.Revision, ObservedRevision: "scratch-" + status + "-" + hex.EncodeToString(sum[:])[:16],
		Reason: "validation scratch " + status,
	}, nil
}

func pendingPrerequisite(facts runtimecontract.RuntimeFacts, id string, kind runtimecontract.PrerequisiteKind) (runtimecontract.RuntimePrerequisite, error) {
	id = strings.TrimSpace(id)
	for _, item := range facts.Normalized().Prerequisites.Pending {
		if item.ID == id && item.Kind == kind {
			return item, nil
		}
	}
	return runtimecontract.RuntimePrerequisite{}, fmt.Errorf("pending prerequisite %q of kind %s was not found", id, kind)
}

func safePrerequisitePathPart(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	var ret strings.Builder
	for _, char := range value {
		switch {
		case char >= 'a' && char <= 'z', char >= 'A' && char <= 'Z', char >= '0' && char <= '9', char == '-', char == '_', char == '.':
			ret.WriteRune(char)
		default:
			ret.WriteByte('_')
		}
	}
	normalized := strings.Trim(ret.String(), "._-")
	if normalized == "" {
		return "unknown"
	}
	return normalized
}
