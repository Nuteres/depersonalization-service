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

// RandInt - детерминированная генерация целого числа в диапазоне [min, max]
func (a *Anonymizer) RandInt(seed uint64, min, max int) int {
	if min >= max {
		return min
	}
	// Linear Congruential Generator step
	next := seed*6364136223846793005 + 1442695040888963407
	diff := uint64(max - min + 1)
	return min + int(next%diff)
}
