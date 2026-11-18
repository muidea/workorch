package recordpayload

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
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

var auditCredentialKeyReplacer = strings.NewReplacer("_", "", "-", "", ".", "", " ", "")

// IsSanitizedAuditPayload checks whether input already satisfies the exact
// SanitizeAuditPayload contract without cloning or JSON-marshaling clean, bounded
// payloads, falling back to full SanitizeAuditPayload comparison on any edge case.
func IsSanitizedAuditPayload(input workorcheventpayload.Payload) bool {
	if len(input.Fields) == 0 {
		return true
	}
	if auditPayloadContainsCredential(input.Fields, 0) {
		return false
	}
	if auditPayloadAlreadyCleanWithinBudget(input) {
		return true
	}
	return SanitizeAuditPayload(input).Equal(input)
}

func auditPayloadAlreadyCleanWithinBudget(input workorcheventpayload.Payload) bool {
	usedBudget := 0
	nodes := 0
	if !auditFieldsAlreadyCleanWithinBudget(input.Fields, 0, &usedBudget, &nodes) {
		return false
	}
	return nodes <= 64 && usedBudget <= 2048
}

func auditFieldsAlreadyCleanWithinBudget(fields []workorcheventpayload.Field, depth int, usedBudget, nodes *int) bool {
	if depth > maxAuditPayloadDepth || len(fields) > maxAuditPayloadFields {
		return false
	}
	prevName := ""
	for i, field := range fields {
		if field.Name == "" || strings.TrimSpace(field.Name) != field.Name {
			return false
		}
		if i > 0 && field.Name <= prevName {
			return false
		}
		prevName = field.Name
		*nodes++
		*usedBudget += len(field.Name) + 8
		if *nodes > 64 || *usedBudget > 2048 {
			return false
		}
		if !auditValueAlreadyCleanWithinBudget(field.Value, depth, usedBudget, nodes) {
			return false
		}
	}
	return true
}

func auditValueAlreadyCleanWithinBudget(value workorcheventpayload.Value, depth int, usedBudget, nodes *int) bool {
	switch value.Kind {
	case workorcheventpayload.ValueString:
		if len(value.String) > maxAuditPayloadStringBytes {
			return false
		}
		*usedBudget += len(value.String)
	case workorcheventpayload.ValueObject:
		if depth >= maxAuditPayloadDepth {
			return false
		}
		if !auditFieldsAlreadyCleanWithinBudget(value.Object, depth+1, usedBudget, nodes) {
			return false
		}
	case workorcheventpayload.ValueArray:
		if depth >= maxAuditPayloadDepth || len(value.Array) > maxAuditPayloadArrayItems {
			return false
		}
		for _, item := range value.Array {
			*nodes++
			if *nodes > 64 {
				return false
			}
			if !auditValueAlreadyCleanWithinBudget(item, depth+1, usedBudget, nodes) {
				return false
			}
		}
	case workorcheventpayload.ValueNull, workorcheventpayload.ValueBool, workorcheventpayload.ValueInt, workorcheventpayload.ValueUint:
		*usedBudget += 16
	case workorcheventpayload.ValueFloat:
		if math.IsNaN(value.Float) || math.IsInf(value.Float, 0) {
			return false
		}
		*usedBudget += 16
	default:
		return false
	}
	return *usedBudget <= 2048
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
		if isAuditCredentialFieldName(field.Name) {
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

func isAuditCredentialFieldName(name string) bool {
	var buf [16]byte
	n := 0
	for i := 0; i < len(name); i++ {
		b := name[i]
		if b >= utf8.RuneSelf {
			switch auditCredentialKeyReplacer.Replace(strings.ToLower(name)) {
			case "apikey", "apitoken", "accesstoken", "refreshtoken", "token", "password", "passwd", "secret", "clientsecret", "privatekey", "credential", "credentials", "authorization", "cookie", "setcookie":
				return true
			default:
				return false
			}
		}
		if b == '_' || b == '-' || b == '.' || b == ' ' {
			continue
		}
		if b >= 'A' && b <= 'Z' {
			b += 'a' - 'A'
		}
		if n >= 13 {
			return false
		}
		buf[n] = b
		n++
	}
	switch string(buf[:n]) {
	case "apikey", "apitoken", "accesstoken", "refreshtoken", "token", "password", "passwd", "secret", "clientsecret", "privatekey", "credential", "credentials", "authorization", "cookie", "setcookie":
		return true
	default:
		return false
	}
}
