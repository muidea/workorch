package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// Frozen pre-change original-value encoder, including its detached input.
func referenceRuntimeFeedbackHashBeforeReadOnlyEncoding(feedback RuntimeFeedback) string {
	feedback = feedback.CloneOriginal()
	h := sha256.New()
	writeHashString(h, string(feedback.Domain))
	writeHashString(h, string(feedback.StableCategory))
	writeHashString(h, string(feedback.WorkspaceID))
	writeHashString(h, string(feedback.RunID))
	writeHashString(h, feedback.AuthoritySnapshotID)
	writeHashString(h, feedback.DispatchID)
	if feedback.OriginalDiagnostic == nil {
		writeHashString(h, "original-diagnostic:nil")
	} else {
		diagnostic := feedback.OriginalDiagnostic
		writeHashString(h, diagnostic.SchemaVersion)
		writeHashString(h, diagnostic.OwnerModuleID)
		writeHashString(h, diagnostic.OriginalClass)
		writeHashString(h, diagnostic.OriginalCode)
		writeHashString(h, diagnostic.OriginalMessage)
		writeHashString(h, diagnostic.Signature)
		writeHashString(h, diagnostic.OperationID)
		writeHashStrings(h, diagnostic.TargetRefs)
		writeHashString(h, diagnostic.ExpectedRevision)
		writeHashString(h, diagnostic.ObservedRevision)
		writeRuntimeFieldViolationsHash(h, diagnostic.FieldViolations)
		writeRuntimeSemanticContractViolationsHash(h, diagnostic.SemanticViolations)
		writeHashString(h, diagnostic.DetailsRef)
		writeHashString(h, diagnostic.DetailsHash)
		writeHashString(h, fmt.Sprintf("%d", diagnostic.DetailsBytes))
		writeHashString(h, diagnostic.ContentType)
		writeHashString(h, diagnostic.OmittedReason)
		writeHashString(h, fmt.Sprintf("%t", diagnostic.Truncated))
		writeHashString(h, fmt.Sprintf("%t", diagnostic.Redacted))
		writeRuntimeOriginalDiagnosticTypedFactsHash(h, diagnostic)
	}
	return "runtime-feedback-" + hex.EncodeToString(h.Sum(nil))
}
