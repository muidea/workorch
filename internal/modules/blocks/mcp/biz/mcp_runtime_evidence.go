package mcp

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	mcpevents "github.com/muidea/workorch/internal/modules/blocks/mcp/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const (
	mcpEvidenceSchemaVersion = "workorch.mcp_evidence.v1"
	maxMCPResponseBytes      = 1 << 20
	maxMCPStreamEvents       = 128
	maxMCPEvidenceBytes      = 2 << 20
	maxMCPEffectChanges      = 16
)

type mcpEvidenceEnvelope struct {
	SchemaVersion    string             `json:"schemaVersion"`
	OperationKind    string             `json:"operationKind"`
	CapabilityID     string             `json:"capabilityId"`
	RegistryRevision string             `json:"registryRevision"`
	Response         MCPResponsePayload `json:"response"`
	StreamEvents     []MCPStreamEvent   `json:"streamEvents,omitempty"`
}

type mcpEffectPayload struct {
	EffectScope     string                    `json:"effectScope"`
	Changed         bool                      `json:"changed"`
	BeforeHash      string                    `json:"beforeHash"`
	AfterHash       string                    `json:"afterHash"`
	ResourceChanges []mcpEffectResourceChange `json:"resourceChanges"`
}

type mcpEffectResourceChange struct {
	SubjectRef     string `json:"subjectRef"`
	BeforeRevision string `json:"beforeRevision"`
	AfterRevision  string `json:"afterRevision"`
}

func prepareMCPEvidenceStore(internalDirectory string) error {
	directory, err := mcpEvidenceDirectory(internalDirectory)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create MCP evidence directory: %w", err)
	}
	info, err := os.Lstat(directory)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("MCP evidence directory is not a regular directory")
	}
	return nil
}

func mcpEvidenceDirectory(internalDirectory string) (string, error) {
	internalDirectory = strings.TrimSpace(internalDirectory)
	if internalDirectory == "" {
		return "", fmt.Errorf("%w: MCP evidence store requires workspace internal directory", common.ErrInvalidArgument)
	}
	return filepath.Join(filepath.Clean(internalDirectory), "mcp", "evidence"), nil
}

func buildMCPOperationEvidence(internalDirectory string, operationKind string, authorization mcpevents.RuntimeExecutionAuthorization, result RuntimeOperationResult) (runtimecontract.RuntimeOperationEvidence, error) {
	if err := prepareMCPEvidenceStore(internalDirectory); err != nil {
		return runtimecontract.RuntimeOperationEvidence{}, err
	}
	if len(result.Response.JSON)+len(result.Response.Text) > maxMCPResponseBytes || len(result.StreamEvents) > maxMCPStreamEvents {
		return runtimecontract.RuntimeOperationEvidence{}, fmt.Errorf("MCP response exceeds owner evidence bounds")
	}
	envelope := mcpEvidenceEnvelope{
		SchemaVersion: mcpEvidenceSchemaVersion, OperationKind: runtimecontract.CanonicalRuntimeBlockKind(operationKind),
		CapabilityID: string(authorization.Capability.ID), RegistryRevision: strings.TrimSpace(authorization.RegistryRevision),
		Response:     MCPResponsePayload{JSON: append(json.RawMessage(nil), result.Response.JSON...), Text: result.Response.Text},
		StreamEvents: cloneMCPStreamEvents(result.StreamEvents),
	}
	raw, err := json.Marshal(envelope)
	if err != nil {
		return runtimecontract.RuntimeOperationEvidence{}, fmt.Errorf("marshal MCP evidence: %w", err)
	}
	if len(raw) > maxMCPEvidenceBytes {
		return runtimecontract.RuntimeOperationEvidence{}, fmt.Errorf("MCP normalized evidence exceeds owner byte bound")
	}
	sum := sha256.Sum256(raw)
	hash := hex.EncodeToString(sum[:])
	directory, _ := mcpEvidenceDirectory(internalDirectory)
	path := filepath.Join(directory, hash+".json")
	if err := persistMCPEvidenceFile(path, raw); err != nil {
		return runtimecontract.RuntimeOperationEvidence{}, err
	}
	refs := []string{
		"workorch://mcp/evidence/" + hash,
		"workorch://mcp/capability/" + url.PathEscape(string(authorization.Capability.ID)) + "/" + url.PathEscape(authorization.RegistryRevision),
	}
	sort.Strings(refs)
	summary := fmt.Sprintf("MCP %s owner evidence captured; capability=%s responseBytes=%d streamEvents=%d", runtimecontract.CanonicalRuntimeBlockKind(operationKind), authorization.Capability.ID, len(result.Response.JSON)+len(result.Response.Text), len(result.StreamEvents))
	return runtimecontract.RuntimeOperationEvidence{
		Summary: summary, ContentHash: hash,
		ContentAddress: &runtimecontract.RuntimeEvidenceContentAddress{
			Algorithm: runtimecontract.RuntimeEvidenceDigestSHA256,
			Digest:    hash,
			Ref:       "workorch://mcp/evidence/" + hash,
		},
		EvidenceRefs: refs,
		EvidenceTags: []string{"mcp", runtimecontract.CanonicalRuntimeBlockKind(operationKind), "owner_normalized"},
	}, nil
}

func persistMCPEvidenceFile(path string, raw []byte) error {
	if existing, err := readBoundedMCPEvidence(path); err == nil {
		if !bytes.Equal(existing, raw) {
			return fmt.Errorf("%w: MCP evidence hash collision", common.ErrConflict)
		}
		if err := os.Chmod(path, 0o600); err != nil {
			return fmt.Errorf("secure existing MCP evidence: %w", err)
		}
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	directory := filepath.Dir(path)
	temporary, err := os.CreateTemp(directory, ".mcp-evidence-*")
	if err != nil {
		return fmt.Errorf("create MCP evidence temporary file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(raw); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		if existing, readErr := readBoundedMCPEvidence(path); readErr == nil && bytes.Equal(existing, raw) {
			return nil
		}
		return fmt.Errorf("commit MCP evidence: %w", err)
	}
	return nil
}

func readBoundedMCPEvidence(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("MCP evidence path is not a regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, maxMCPEvidenceBytes+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > maxMCPEvidenceBytes {
		return nil, fmt.Errorf("stored MCP evidence exceeds owner byte bound")
	}
	return raw, nil
}

func cloneMCPStreamEvents(values []MCPStreamEvent) []MCPStreamEvent {
	ret := make([]MCPStreamEvent, 0, len(values))
	for _, value := range values {
		ret = append(ret, MCPStreamEvent{
			Method: value.Method, Params: append(json.RawMessage(nil), value.Params...),
			Payload: MCPResponsePayload{JSON: append(json.RawMessage(nil), value.Payload.JSON...), Text: value.Payload.Text},
		})
	}
	return ret
}

func mcpReceiptEffectObservation(result RuntimeOperationResult, authorization mcpevents.RuntimeExecutionAuthorization) (scope runtimecontract.RuntimeEffectScope, changed bool, beforeHash, afterHash string, changes []runtimecontract.RuntimeResourceChange, err error) {
	declared := runtimecontract.NormalizeRuntimeEffectScope(authorization.Policy.EffectScope)
	if !runtimecontract.RuntimeEffectIsDurable(declared) {
		if payload, found, parseErr := parseMCPEffectPayload(result.Response); parseErr != nil {
			return runtimecontract.RuntimeEffectUnknown, false, "", "", nil, parseErr
		} else if found {
			observed := runtimecontract.NormalizeRuntimeEffectScope(runtimecontract.RuntimeEffectScope(payload.EffectScope))
			before, after := strings.TrimSpace(payload.BeforeHash), strings.TrimSpace(payload.AfterHash)
			if observed != declared || payload.Changed || len(payload.ResourceChanges) != 0 || (before != "" || after != "") && (before == "" || before != after || !validMCPRevisionToken(before)) {
				return runtimecontract.RuntimeEffectUnknown, false, "", "", nil, fmt.Errorf("MCP response claimed an invalid effect observation for a non-durable capability")
			}
		}
		return declared, false, "", "", nil, nil
	}
	payload, found, parseErr := parseMCPEffectPayload(result.Response)
	if parseErr != nil {
		return runtimecontract.RuntimeEffectUnknown, false, "", "", nil, parseErr
	}
	if !found {
		return runtimecontract.RuntimeEffectUnknown, false, "", "", nil, fmt.Errorf("durable MCP operation did not return workorchEffect observation")
	}
	observed := runtimecontract.NormalizeRuntimeEffectScope(runtimecontract.RuntimeEffectScope(payload.EffectScope))
	if observed != declared {
		return runtimecontract.RuntimeEffectUnknown, false, "", "", nil, fmt.Errorf("MCP effect observation does not match sealed capability effect")
	}
	beforeHash, afterHash = strings.TrimSpace(payload.BeforeHash), strings.TrimSpace(payload.AfterHash)
	if !validMCPRevisionToken(beforeHash) || !validMCPRevisionToken(afterHash) {
		return runtimecontract.RuntimeEffectUnknown, false, "", "", nil, fmt.Errorf("MCP effect observation has invalid bounded revisions")
	}
	if !payload.Changed {
		if beforeHash == "" || afterHash == "" || beforeHash != afterHash || len(payload.ResourceChanges) != 0 {
			return runtimecontract.RuntimeEffectUnknown, false, "", "", nil, fmt.Errorf("MCP no-op effect observation requires one unchanged revision")
		}
		return runtimecontract.RuntimeEffectNone, false, beforeHash, afterHash, nil, nil
	}
	if beforeHash == "" || afterHash == "" || beforeHash == afterHash || len(payload.ResourceChanges) == 0 || len(payload.ResourceChanges) > maxMCPEffectChanges {
		return runtimecontract.RuntimeEffectUnknown, false, "", "", nil, fmt.Errorf("MCP durable effect observation requires bounded changed revisions")
	}
	for _, item := range payload.ResourceChanges {
		change := runtimecontract.RuntimeResourceChange{
			Domain: runtimecontract.RuntimeResourceRemoteResource, OwnerModuleID: ModuleID,
			SubjectRef: strings.TrimSpace(item.SubjectRef), BeforeRevision: strings.TrimSpace(item.BeforeRevision), AfterRevision: strings.TrimSpace(item.AfterRevision),
		}.Normalized()
		if !validMCPResourceSubjectRef(change.SubjectRef) || change.BeforeRevision == change.AfterRevision ||
			!validMCPRevisionToken(change.BeforeRevision) || !validMCPRevisionToken(change.AfterRevision) {
			return runtimecontract.RuntimeEffectUnknown, false, "", "", nil, fmt.Errorf("MCP resource change has invalid bounded identity or revisions")
		}
		if err := change.Validate(); err != nil {
			return runtimecontract.RuntimeEffectUnknown, false, "", "", nil, err
		}
		changes = append(changes, change)
	}
	return declared, true, beforeHash, afterHash, runtimecontract.NormalizeRuntimeResourceChanges(changes), nil
}

func validMCPResourceSubjectRef(value string) bool {
	value = strings.TrimSpace(value)
	lower := strings.ToLower(value)
	if value == "" || len(value) > 512 || strings.Contains(value, "://") || strings.Contains(value, "\\") ||
		strings.ContainsAny(value, "\r\n\t") || strings.Contains(lower, "credential") || strings.Contains(lower, "password") ||
		strings.Contains(lower, "privatekey") || strings.Contains(lower, "secret") || strings.Contains(lower, "token") {
		return false
	}
	for _, char := range value {
		if char < 0x21 || char == 0x7f {
			return false
		}
	}
	return true
}

func validMCPRevisionToken(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 256 {
		return false
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || strings.ContainsRune("._:-", char) {
			continue
		}
		return false
	}
	return true
}

func parseMCPEffectPayload(response MCPResponsePayload) (mcpEffectPayload, bool, error) {
	if len(response.JSON) == 0 {
		return mcpEffectPayload{}, false, nil
	}
	var envelope struct {
		WorkOrchEffect *mcpEffectPayload `json:"workorchEffect"`
	}
	if err := json.Unmarshal(response.JSON, &envelope); err != nil {
		return mcpEffectPayload{}, false, fmt.Errorf("invalid MCP effect observation")
	}
	if envelope.WorkOrchEffect == nil {
		return mcpEffectPayload{}, false, nil
	}
	return *envelope.WorkOrchEffect, true, nil
}
