package store

import (
	"crypto/sha256"
	"encoding/hex"
)

// TokenHash devuelve el hash SHA-256 en hex del token en claro (para persistir y buscar en clients).
func TokenHash(plain string) string {
	h := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(h[:])
}
