// Package protocol defines the MCP owner's shared stdio negotiation rules.
package protocol

import (
	"encoding/json"
	"errors"
)

const PreferredVersion = "2025-11-25"

var ErrInvalidInitialization = errors.New("invalid MCP initialization result")
var ErrUnsupportedVersion = errors.New("unsupported MCP protocol version")

// SupportsVersion is an exact allowlist for the operations implemented by this
// client. A server may select an older supported version; dates are not ranges.
func SupportsVersion(version string) bool {
	return version == "2024-11-05" || version == PreferredVersion
}

// NegotiatedVersion reads the server's choice without rewriting its result.
// Callers retain the original bytes at their owner-local storage boundary.
func NegotiatedVersion(result json.RawMessage) (string, error) {
	var initialization struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	if json.Unmarshal(result, &initialization) != nil || initialization.ProtocolVersion == "" {
		return "", ErrInvalidInitialization
	}
	if !SupportsVersion(initialization.ProtocolVersion) {
		return "", ErrUnsupportedVersion
	}
	return initialization.ProtocolVersion, nil
}
