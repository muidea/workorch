package orchestrator

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	runtimev2 "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtimev2"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func ResolveFailureTargetPrerequisite(facts runtimecontract.RuntimeFacts, prerequisiteID string) (runtimecontract.RuntimeFacts, error) {
	prerequisite, err := pendingPrerequisite(facts, prerequisiteID, runtimecontract.PrerequisiteKindFailureTarget)
	if err != nil {
		return runtimecontract.RuntimeFacts{}, err
	}
	if facts.Failure == nil || len(facts.Failure.TargetRefs) == 0 {
		return runtimecontract.RuntimeFacts{}, fmt.Errorf("failure target prerequisite %q has no typed failure targets", prerequisite.ID)
	}
	return runtimev2.CommitPrerequisiteResult(facts, runtimev2.PrerequisiteResult{
		PrerequisiteID: prerequisite.ID,
		FailureTarget: &runtimecontract.FailureTarget{
			Signature:  facts.Failure.Signature,
			TargetRefs: facts.Failure.TargetRefs,
		},
	})
}

func AcquireValidationScratchPrerequisite(facts runtimecontract.RuntimeFacts, prerequisiteID string) (runtimecontract.RuntimeFacts, error) {
	prerequisite, err := pendingPrerequisite(facts, prerequisiteID, runtimecontract.PrerequisiteKindValidationScratch)
	if err != nil {
		return runtimecontract.RuntimeFacts{}, err
	}
	if facts.WorkspaceID == "" || facts.RunID == "" {
		return runtimecontract.RuntimeFacts{}, common.ErrInvalidArgument
	}
	reference := filepath.Join(os.TempDir(), "workorch-validation-scratch-v2", safePrerequisitePathPart(string(facts.WorkspaceID)), safePrerequisitePathPart(string(facts.RunID)), safePrerequisitePathPart(facts.Focus.ID))
	if err := os.MkdirAll(reference, 0o755); err != nil {
		return runtimecontract.RuntimeFacts{}, err
	}
	sum := sha256.Sum256([]byte(reference))
	return runtimev2.CommitPrerequisiteResult(facts, runtimev2.PrerequisiteResult{
		PrerequisiteID: prerequisite.ID,
		ValidationScratch: &runtimecontract.ValidationScratch{
			Reference: reference,
			Revision:  "scratch-" + hex.EncodeToString(sum[:])[:16],
		},
	})
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
