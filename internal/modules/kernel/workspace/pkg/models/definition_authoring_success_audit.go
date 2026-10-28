package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// Success byte audit is deliberately independent from failure Diagnostic. The
// complete saved document is mandatory for owner reload; transport wire can be
// unobserved/partial/over-limit without overriding a known successful document.
type DefinitionAuthoringSuccessByteAudit struct {
	Document DefinitionProviderObservation
	Wire     DefinitionProviderObservation
}

func (a DefinitionAuthoringSuccessByteAudit) Clone() DefinitionAuthoringSuccessByteAudit {
	a.Document = a.Document.Clone()
	a.Wire = a.Wire.Clone()
	return a
}
func (a DefinitionAuthoringSuccessByteAudit) Validate() error {
	if a.Document.Validate() != nil || !a.Document.Observed || !a.Document.Complete || !a.Document.Saved || a.Document.ObservedBytes == 0 || a.Wire.Validate() != nil {
		return fmt.Errorf("authoring success audit requires complete saved bounded document and valid wire observation")
	}
	return nil
}
func (a DefinitionAuthoringSuccessByteAudit) Metadata() DefinitionAuthoringSuccessByteAudit {
	a.Document.Content = nil
	a.Wire.Content = nil
	return a
}
func (a DefinitionAuthoringSuccessByteAudit) ValidateMetadata() error {
	if a.Document.ValidateMetadata() != nil || !a.Document.Observed || !a.Document.Complete || !a.Document.Saved || a.Document.ObservedBytes == 0 || a.Wire.ValidateMetadata() != nil {
		return fmt.Errorf("authoring success audit metadata invalid")
	}
	return nil
}
func (o DefinitionProviderObservation) ValidateMetadata() error {
	if o.Content != nil {
		return fmt.Errorf("provider observation metadata contains raw bytes")
	}
	if !o.Observed {
		if o.ObservedBytes != 0 || o.SHA256 != "" || o.Complete || o.Saved || o.Reason == "" {
			return fmt.Errorf("unobserved metadata conflicting facts")
		}
		return nil
	}
	hash, err := hex.DecodeString(o.SHA256)
	if err != nil || len(hash) != sha256.Size || hex.EncodeToString(hash) != o.SHA256 {
		return fmt.Errorf("provider observation metadata hash invalid")
	}
	if !o.Complete && o.Reason == "" {
		return fmt.Errorf("partial metadata lacks reason")
	}
	if o.Saved {
		if o.ObservedBytes > DefinitionAuthoringDiagnosticDocumentLimit {
			return fmt.Errorf("saved observation metadata exceeds limit")
		}
	} else if o.ObservedBytes <= DefinitionAuthoringDiagnosticDocumentLimit || o.Reason != "observation_storage_limit" {
		return fmt.Errorf("observation metadata storage reason invalid")
	}
	return nil
}
func (d DefinitionAuthoringDiagnostic) Metadata() DefinitionAuthoringDiagnostic {
	d = d.Clone()
	d.ParseError = ""
	d.Payload = nil
	if d.ProviderDocument != nil {
		o := *d.ProviderDocument
		o.Content = nil
		d.ProviderDocument = &o
	}
	if d.ProviderWire != nil {
		o := *d.ProviderWire
		o.Content = nil
		d.ProviderWire = &o
	}
	if d.ProviderCause != nil {
		o := *d.ProviderCause
		o.Content = nil
		d.ProviderCause = &o
	}
	return d
}
func (d DefinitionAuthoringDiagnostic) ValidateMetadata() error {
	if d.ParseError != "" || d.Payload != nil {
		return fmt.Errorf("diagnostic metadata contains raw bytes")
	}
	validHash := func(s string) bool {
		hash, err := hex.DecodeString(s)
		return err == nil && len(hash) == sha256.Size && hex.EncodeToString(hash) == s
	}
	if d.ParseErrorBytes == 0 || !validHash(d.ParseErrorSHA256) || !validHash(d.PayloadSHA256) {
		return fmt.Errorf("diagnostic metadata identity invalid")
	}
	if d.ParseErrorSaved != (d.ParseErrorBytes <= DefinitionAuthoringDiagnosticErrorLimit) || d.PayloadSaved != (d.PayloadBytes <= DefinitionAuthoringDiagnosticDocumentLimit) {
		return fmt.Errorf("diagnostic metadata limit mismatch")
	}
	if d.ParseErrorSaved && d.PayloadSaved {
		if d.NotSavedReason != "" {
			return fmt.Errorf("diagnostic metadata saved reason invalid")
		}
	} else if d.NotSavedReason != definitionAuthoringDiagnosticNotSavedReason {
		return fmt.Errorf("diagnostic metadata unsaved reason invalid")
	}
	if d.ProviderCode != "" || d.ProviderDocument != nil || d.ProviderWire != nil || d.ProviderCause != nil {
		if len(d.ProviderCode) > 128 || d.ProviderDocument == nil || d.ProviderWire == nil || d.ProviderDocument.ValidateMetadata() != nil || d.ProviderWire.ValidateMetadata() != nil || d.ProviderCause != nil && (d.ProviderCode == "" || d.ProviderCause.ValidateMetadata() != nil || !d.ProviderCause.Observed || !d.ProviderCause.Complete) {
			return fmt.Errorf("provider diagnostic metadata invalid")
		}
		if d.ProviderCode == "" && (!d.ProviderDocument.Observed || !d.ProviderDocument.Complete || !d.ProviderDocument.Saved || d.ProviderDocument.ObservedBytes != d.PayloadBytes || d.ProviderDocument.SHA256 != d.PayloadSHA256) {
			return fmt.Errorf("parser diagnostic metadata document differs from payload")
		}
	}
	return nil
}
func (r DefinitionAuthoringSpecificationRequest) Metadata() DefinitionAuthoringSpecificationRequest {
	r = r.Clone()
	if r.ReviewAudit != nil {
		a := r.ReviewAudit.Metadata()
		r.ReviewAudit = &a
	}
	if r.SuccessAudit != nil {
		a := r.SuccessAudit.Metadata()
		r.SuccessAudit = &a
	}
	if r.Diagnostic != nil {
		d := r.Diagnostic.Metadata()
		r.Diagnostic = &d
	}
	return r
}
