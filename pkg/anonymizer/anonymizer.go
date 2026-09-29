package anonymizer

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
)

// Anonymizer - генератор детерминированных синтетических данных
type Anonymizer struct {
	salt []byte
}

func New(salt string) *Anonymizer {
	return &Anonymizer{salt: []byte(salt)}
}

// DeriveSeed - получение детерминированного seed для сохранения связей (referential integrity)
func (a *Anonymizer) DeriveSeed(originalValue string) uint64 {
	mac := hmac.New(sha256.New, a.salt)
	mac.Write([]byte(originalValue))
	sum := mac.Sum(nil)
	return binary.BigEndian.Uint64(sum[:8])
}
