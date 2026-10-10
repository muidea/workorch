package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// StableRuntimeEvidenceFactHash returns the exact canonical value identity of
// one Orchestrator-owned EvidenceFact. The hash is a mutable execution-commit
// projection: it binds the fact written by SM-2/SM-3 without becoming part of
// immutable dispatch or operation authority.
func StableRuntimeEvidenceFactHash(fact EvidenceFact) string {
	values := normalizeEvidenceFacts([]EvidenceFact{fact})
	if len(values) != 1 {
		return ""
	}
	fact = values[0]
	h := sha256.New()
	writeHashString(h, "runtime-evidence-fact/v2")
	writeHashString(h, string(fact.ID))
	writeHashString(h, fact.Kind)
	writeHashString(h, fact.FocusID)
	writeHashString(h, fact.SubjectRef)
	writeHashString(h, fact.SourceRef)
	writeHashString(h, fact.AuthoritySnapshotID)
	writeHashString(h, fact.DispatchID)
	writeHashString(h, fact.OperationID)
	writeHashString(h, fact.Revision)
	writeHashString(h, string(fact.FailureClass))
	writeHashString(h, fact.FailureCode)
	writeHashString(h, fact.FailureSignature)
	writeHashString(h, fact.FailureMessage)
	writeHashString(h, fact.Summary)
	writeHashString(h, fact.ContentHash)
	if fact.OwnerRevisionSet != nil {
		writeHashString(h, "owner-read-dependencies/v1")
		writeHashString(h, stableFormalValueHash("runtime-owner-read-dependencies", *fact.OwnerRevisionSet))
	}
	if fact.ContentAddress == nil {
		writeHashString(h, "content-address:nil")
	} else {
		address := fact.ContentAddress.Normalized()
		writeHashString(h, string(address.Algorithm))
		writeHashString(h, address.Digest)
		writeHashString(h, address.Ref)
	}
	if fact.ExitCode == nil {
		writeHashString(h, "exit:nil")
	} else {
		writeHashString(h, fmt.Sprintf("exit:%d", *fact.ExitCode))
	}
	writeHashStrings(h, fact.EvidenceRefs)
	writeHashStrings(h, fact.TargetRefs)
	writeHashStrings(h, fact.BindingRefs)
	coverageIDs := make([]string, 0, len(fact.CoverageWorkItemIDs))
	for _, id := range fact.CoverageWorkItemIDs {
		coverageIDs = append(coverageIDs, string(id))
	}
	writeHashStrings(h, coverageIDs)
	writeHashString(h, string(fact.Quality))
	writeHashString(h, fmt.Sprintf("%t", fact.ZeroWork))
	writeHashString(h, fact.EvidenceReason)
	writeHashStrings(h, fact.EvidenceTags)
	if fact.Validation == nil {
		writeHashString(h, "validation:nil")
	} else {
		observation := fact.Validation.Normalized()
		writeHashString(h, observation.RecipeID)
		writeHashString(h, observation.ObservationProtocolRef)
		writeHashStrings(h, observation.ExecutedChecks)
		writeHashStrings(h, observation.PassedChecks)
		writeHashStrings(h, observation.FailedChecks)
		writeHashStrings(h, observation.SkippedChecks)
		writeHashString(h, observation.OutputPreview)
		writeRuntimeValidationFailureDetailsHash(h, observation)
		if observation.ProofClass != "" {
			writeHashString(h, "validation-proof-class/v1")
			writeHashString(h, string(observation.ProofClass))
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}
