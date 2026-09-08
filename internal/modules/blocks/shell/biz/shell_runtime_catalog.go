package shell

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// AcquireRuntimeOperationAdmissibility reports the Shell owner's bounded
// command-parameter envelope. It only inspects the workspace directory; no
// command is started and no managed-validation resource is allocated.
func (a *RuntimeAdapter) AcquireRuntimeOperationAdmissibility(workspaceDirectory, prerequisiteID string, operationKinds []string) (runtimemodel.RuntimeOperationAdmissibility, error) {
	directory, err := filepath.Abs(strings.TrimSpace(workspaceDirectory))
	if err != nil || directory == "" {
		return runtimemodel.RuntimeOperationAdmissibility{}, common.ErrInvalidArgument
	}
	info, err := os.Stat(directory)
	if err != nil || !info.IsDir() {
		if err == nil {
			err = common.ErrInvalidArgument
		}
		return runtimemodel.RuntimeOperationAdmissibility{}, err
	}
	inventory := runtimemodel.RuntimeOperationAdmissibility{
		PrerequisiteID:    prerequisiteID,
		OwnerModuleID:     runtimemodel.RuntimeModuleShell,
		OperationKinds:    append([]string(nil), operationKinds...),
		OperationVariants: shellAdmissibilityOperationVariants(operationKinds),
		OperationPolicies: shellOperationPolicyFacts(operationKinds),
		ConstraintRefs:    []string{"workspace-working-directory", "executable-on-path", "bounded-timeout", "validation-target-must-exist"},
		// Directory mtime changes whenever a workspace child is created. It is
		// neither Shell policy nor a validation-input revision, and using it here
		// turns a preceding File effect into a false candidate failure. Shell's
		// inventory is instead bound to its own stable execution policy.
		ObservedRevision: a.runtimeAdmissibilityPolicyRevision(),
	}
	for _, kind := range operationKinds {
		if runtimemodel.CanonicalRuntimeBlockKind(kind) == runtimemodel.RuntimeBlockKindManagedValidationSession {
			inventory.SelectorRefs = []string{shellValidationTargetSelector}
			break
		}
	}
	inventory.SnapshotID = runtimemodel.StableRuntimeOperationAdmissibilitySnapshotID(inventory)
	if err := inventory.Validate(); err != nil {
		return runtimemodel.RuntimeOperationAdmissibility{}, err
	}
	return inventory, nil
}

func shellOperationPolicyFacts(operationKinds []string) []runtimemodel.RuntimeOperationPolicyFact {
	ret := make([]runtimemodel.RuntimeOperationPolicyFact, 0, 2)
	for _, raw := range operationKinds {
		switch runtimemodel.CanonicalRuntimeBlockKind(raw) {
		case runtimemodel.StepKindShellCommand:
			// Arbitrary Shell commands cannot honestly claim effect=none. The
			// public shell/command surface is therefore durable and at-most-once;
			// non-mutating proof uses the separately managed validation surface.
			value := true
			ret = append(ret, runtimemodel.RuntimeOperationPolicyFact{
				ID:            "shell/command/state-changing/true/v2",
				OwnerModuleID: runtimemodel.RuntimeModuleShell,
				OperationKind: runtimemodel.StepKindShellCommand,
				ParameterMatches: []runtimemodel.RuntimeOperationParameterMatch{{
					Name: "stateChanging", Boolean: &value,
				}},
				EffectScope: runtimemodel.RuntimeEffectWorkspaceDurable, IdempotencyClass: runtimemodel.RuntimeOperationIdempotencyAtMostOnce,
			})
		case runtimemodel.RuntimeBlockKindManagedValidationSession:
			ret = append(ret, runtimemodel.RuntimeOperationPolicyFact{
				ID: "shell/managed-validation/isolated-ephemeral/v2", OwnerModuleID: runtimemodel.RuntimeModuleShell,
				OperationKind: runtimemodel.RuntimeBlockKindManagedValidationSession,
				EffectScope:   runtimemodel.RuntimeEffectEphemeralSession, IdempotencyClass: runtimemodel.RuntimeOperationIdempotencyAtMostOnce,
			})
		}
	}
	return runtimemodel.NormalizeRuntimeOperationPolicyFacts(ret)
}

func shellAdmissibilityOperationVariants(operationKinds []string) []runtimemodel.RuntimeOperationAdmissibilityVariant {
	ret := make([]runtimemodel.RuntimeOperationAdmissibilityVariant, 0, len(operationKinds))
	for _, raw := range operationKinds {
		kind := runtimemodel.CanonicalRuntimeBlockKind(raw)
		switch kind {
		case runtimemodel.StepKindShellCommand:
			// Only shell/command needs the owner-authored existing-scope fact
			// formerly carried by the shared descriptor. Managed validation keeps
			// its bounded TargetRefs/selector catalog: publishing an existing-scope
			// variant there would widen semantic target authority before the proof
			// has selected a concrete validation target.
			ret = append(ret, runtimemodel.RuntimeOperationAdmissibilityVariant{
				Kind: kind,
				AllowedTargetPolicies: []runtimemodel.RuntimeOperationTargetPolicy{
					runtimemodel.RuntimeOperationTargetPolicyExactTarget,
					runtimemodel.RuntimeOperationTargetPolicyExistingOwnerScope,
				},
			})
		}
	}
	return ret
}

func (a *RuntimeAdapter) runtimeAdmissibilityPolicyRevision() string {
	keys := make([]string, 0, len(a.envAllowlist))
	for key := range a.envAllowlist {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	h := sha256.New()
	_, _ = fmt.Fprintf(h, "contract=shell-policy-v3;timeout=%s;input-contract=%s;input-requirements=%s;", a.timeout, shellSupplementContentContractRef, shellSupplementContentRequirements)
	_, _ = fmt.Fprintf(h, "entrypoint-contract=%s;entrypoint-requirements=%s;protocol=%s;process-files=%d;", shellEntrypointContentContractRef, shellEntrypointContentRequirements, shellGoEntrypointObservationProtocolRef, maxShellProcessCoverageFiles)
	_, _ = fmt.Fprintf(h, "failure-details=%s;failure-requirements=%s;", runtimemodel.RuntimeValidationFailureDetailProtocolV2, shellAssertionFeedbackRequirements)
	for _, key := range keys {
		_, _ = fmt.Fprintf(h, "env=%s;", key)
	}
	return "shell-policy-" + hex.EncodeToString(h.Sum(nil))[:20]
}

func (a *RuntimeAdapter) runtimeAdmissibilityObservation(workspaceDirectory, prerequisiteID string, inventory runtimemodel.RuntimeOperationAdmissibility, targetRefs []string) runtimemodel.RuntimePrerequisiteObservation {
	return a.runtimeAdmissibilityObservationWithManifest(workspaceDirectory, prerequisiteID, inventory, targetRefs, runtimemodel.RuntimeWorkspaceManifestBinding{})
}

func (a *RuntimeAdapter) runtimeAdmissibilityObservationWithManifest(workspaceDirectory, prerequisiteID string, inventory runtimemodel.RuntimeOperationAdmissibility, targetRefs []string, binding runtimemodel.RuntimeWorkspaceManifestBinding) runtimemodel.RuntimePrerequisiteObservation {
	dependencies := []runtimemodel.RuntimeFreshnessDependency{{
		Domain: runtimemodel.RuntimeResourceOwnerCatalog, OwnerModuleID: runtimemodel.RuntimeModuleShell, SubjectRef: "shell_runtime_policy", Revision: inventory.ObservedRevision,
	}}
	binding = binding.Normalized()
	if !binding.Empty() {
		dependencies = append(dependencies, runtimemodel.RuntimeFreshnessDependency{
			Domain: runtimemodel.RuntimeResourceWorkspaceTree, OwnerModuleID: runtimemodel.RuntimeModuleFile,
			SubjectRef: "workspace_deliverable", Revision: binding.Manifest.Revision,
		})
	}
	for _, targetRef := range targetRefs {
		revision, ok := shellWorkspaceTargetRevision(workspaceDirectory, targetRef)
		if !ok {
			// Absence is itself an observed target revision. Keeping it allows a
			// File create receipt to invalidate this Shell observation before a
			// later validation treats the new target as if it had been acquired.
			revision = "missing"
		}
		dependencies = append(dependencies, runtimemodel.RuntimeFreshnessDependency{
			Domain: runtimemodel.RuntimeResourceWorkspaceTarget, OwnerModuleID: runtimemodel.RuntimeModuleFile, SubjectRef: strings.TrimSpace(targetRef), Revision: revision,
		})
	}
	observation := runtimemodel.RuntimePrerequisiteObservation{
		PrerequisiteID: prerequisiteID, Kind: runtimemodel.PrerequisiteKindOperationAdmissibility, OwnerModuleID: runtimemodel.RuntimeModuleShell,
		ContentHash: inventory.SnapshotID, Dependencies: dependencies,
	}
	observation.ObservationID = runtimemodel.StableRuntimePrerequisiteObservationID(observation)
	return observation
}

func shellWorkspaceTargetRevision(workspaceDirectory, targetRef string) (string, bool) {
	workspaceDirectory = strings.TrimSpace(workspaceDirectory)
	targetRef = strings.TrimSpace(targetRef)
	if workspaceDirectory == "" || targetRef == "" || filepath.IsAbs(targetRef) {
		return "", false
	}
	root, err := filepath.Abs(workspaceDirectory)
	if err != nil {
		return "", false
	}
	target := filepath.Join(root, filepath.Clean(targetRef))
	if target != root && !strings.HasPrefix(target, root+string(filepath.Separator)) {
		return "", false
	}
	data, err := os.ReadFile(target)
	if err != nil {
		return "", false
	}
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), true
}
