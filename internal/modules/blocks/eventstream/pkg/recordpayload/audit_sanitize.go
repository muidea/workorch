package recordpayload

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"unicode/utf8"

	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

const (
	MaxAuditPayloadBytes       = 16 * 1024
	maxAuditPayloadBudget      = 12 * 1024
	maxAuditPayloadFields      = 32
	maxAuditPayloadArrayItems  = 32
	maxAuditPayloadStringBytes = 512
	maxAuditPayloadDepth       = 4
)

// SanitizeAuditPayload is the final Eventstream ingress guard. Typed payload
// encoders should already omit authority material and credentials; this guard
// explicitly omits credential-bearing payloads and imposes a deterministic
// size/depth ceiling before persistence and EventHub/SSE fan-out. It never masks
// arbitrary business values or labels a rewritten payload as the original.
func SanitizeAuditPayload(input workorcheventpayload.Payload) workorcheventpayload.Payload {
	if auditPayloadContainsCredential(input.Fields, 0) {
		return workorcheventpayload.New(
			workorcheventpayload.StringField("payloadSha256", PayloadSHA256(input)),
			workorcheventpayload.IntField("payloadBytes", int64(len(input.CanonicalString()))),
			workorcheventpayload.StringField("auditPayloadOmittedReason", "credential_fields_outside_audit_contract"),
		)
	}
	budget := maxAuditPayloadBudget
	truncated := false
	fields := sanitizeAuditFields(input.Fields, 0, &budget, &truncated)
	if truncated {
		fields = append(fields, workorcheventpayload.BoolField("auditPayloadTruncated", true))
	}
	ret := workorcheventpayload.New(fields...)
	encoded, err := json.Marshal(ret)
	if err == nil && len(encoded) <= MaxAuditPayloadBytes {
		return ret
	}
	return workorcheventpayload.New(
		workorcheventpayload.StringField("payloadSha256", PayloadSHA256(input)),
		workorcheventpayload.IntField("payloadBytes", int64(len(input.CanonicalString()))),
		workorcheventpayload.BoolField("auditPayloadTruncated", true),
	)
}

func PayloadSHA256(payload workorcheventpayload.Payload) string {
	return SHA256String(payload.CanonicalString())
}

func SHA256String(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func sanitizeAuditFields(input []workorcheventpayload.Field, depth int, budget *int, truncated *bool) []workorcheventpayload.Field {
	if depth > maxAuditPayloadDepth || *budget <= 0 {
		*truncated = true
		return nil
	}
	limit := len(input)
	if limit > maxAuditPayloadFields {
		limit = maxAuditPayloadFields
		*truncated = true
	}
	ret := make([]workorcheventpayload.Field, 0, limit)
	for _, field := range input[:limit] {
		name := strings.TrimSpace(field.Name)
		if name == "" {
			*truncated = true
			continue
		}
		*budget -= len(name) + 8
		if *budget <= 0 {
			*truncated = true
			break
		}
		value, ok := sanitizeAuditValue(field.Value, depth, budget, truncated)
		if !ok {
			break
		}
		ret = append(ret, workorcheventpayload.Field{Name: name, Value: value})
	}
	return ret
}

func sanitizeAuditValue(input workorcheventpayload.Value, depth int, budget *int, truncated *bool) (workorcheventpayload.Value, bool) {
	value := input
	switch input.Kind {
	case workorcheventpayload.ValueString:
		value.String = truncateAuditString(input.String, budget, truncated)
	case workorcheventpayload.ValueObject:
		if depth >= maxAuditPayloadDepth {
			*truncated = true
			return workorcheventpayload.Value{Kind: workorcheventpayload.ValueObject}, true
		}
		value.Object = sanitizeAuditFields(input.Object, depth+1, budget, truncated)
	case workorcheventpayload.ValueArray:
		if depth >= maxAuditPayloadDepth {
			*truncated = true
			return workorcheventpayload.Value{Kind: workorcheventpayload.ValueArray}, true
		}
		limit := len(input.Array)
		if limit > maxAuditPayloadArrayItems {
			limit = maxAuditPayloadArrayItems
			*truncated = true
		}
		value.Array = make([]workorcheventpayload.Value, 0, limit)
		for _, item := range input.Array[:limit] {
			next, ok := sanitizeAuditValue(item, depth+1, budget, truncated)
			if !ok {
				break
			}
			value.Array = append(value.Array, next)
		}
	default:
		*budget -= 16
	}
	if *budget <= 0 {
		*truncated = true
		return value, false
	}
	return value, true
}

func truncateAuditString(input string, budget *int, truncated *bool) string {
	limit := maxAuditPayloadStringBytes
	if *budget < limit {
		limit = *budget
	}
	if limit < 0 {
		limit = 0
	}
	if len(input) <= limit {
		*budget -= len(input)
		return input
	}
	*truncated = true
	if limit <= 3 {
		*budget = 0
		return ""
	}
	limit -= 3
	for limit > 0 && !utf8.ValidString(input[:limit]) {
		limit--
	}
	*budget -= limit + 3
	return input[:limit] + "..."
}

// Only exact credential fields are excluded. Business names such as
// tokenBudget, secretCount and credentialRef are ordinary audit metadata.
func auditPayloadContainsCredential(fields []workorcheventpayload.Field, depth int) bool {
	if depth > maxAuditPayloadDepth {
		return false
	}
	for _, field := range fields[:min(len(fields), maxAuditPayloadFields)] {
		key := strings.NewReplacer("_", "", "-", "", ".", "", " ", "").Replace(strings.ToLower(field.Name))
		switch key {
		case "apikey", "apitoken", "accesstoken", "refreshtoken", "token", "password", "passwd", "secret", "clientsecret", "privatekey", "credential", "credentials", "authorization", "cookie", "setcookie":
			if field.Value.Kind == workorcheventpayload.ValueString && field.Value.String != "" ||
				field.Value.Kind == workorcheventpayload.ValueObject && len(field.Value.Object) > 0 ||
				field.Value.Kind == workorcheventpayload.ValueArray && len(field.Value.Array) > 0 {
				return true
			}
		}
		if auditValueContainsCredential(field.Value, depth) {
			return true
		}
	}
	return false
}

func auditValueContainsCredential(value workorcheventpayload.Value, depth int) bool {
	if depth >= maxAuditPayloadDepth {
		return false
	}
	if value.Kind == workorcheventpayload.ValueObject && auditPayloadContainsCredential(value.Object, depth+1) {
		return true
	}
	for _, item := range value.Array[:min(len(value.Array), maxAuditPayloadArrayItems)] {
		if auditValueContainsCredential(item, depth+1) {
			return true
		}
	}
	return false
}
