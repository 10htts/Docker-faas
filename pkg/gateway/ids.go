package gateway

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

func generateCallID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err == nil {
		return hex.EncodeToString(buf)
	}
	return fmt.Sprintf("call-%d", time.Now().UnixNano())
}
