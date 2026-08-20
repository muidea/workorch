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
// additionally redacts secret-like keys and imposes a deterministic size/depth
// ceiling before repository persistence and real-time EventHub/SSE fan-out.
func SanitizeAuditPayload(input workorcheventpayload.Payload) workorcheventpayload.Payload {
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
		if auditSecretKey(name) {
			ret = append(ret, workorcheventpayload.StringField(name, "[redacted]"))
			continue
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

func auditSecretKey(key string) bool {
	normalized := strings.NewReplacer("_", "", "-", "", ".", "").Replace(strings.ToLower(strings.TrimSpace(key)))
	if normalized == "token" || strings.HasSuffix(normalized, "token") {
		return true
	}
	for _, marker := range []string{"password", "passwd", "secret", "credential", "authorization", "accesstoken", "refreshtoken", "apitoken", "apikey", "privatekey", "cookie"} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}
