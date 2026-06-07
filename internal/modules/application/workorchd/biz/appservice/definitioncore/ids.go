package definitioncore

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

func GenerateBusinessID(prefix string) string {
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err == nil {
		return prefix + "-" + hex.EncodeToString(raw[:])
	}
	return fmt.Sprintf("%s-%d", prefix, time.Now().UTC().UnixNano())
}
