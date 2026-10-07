package formats

import (
	"math/rand"

	"dep_service/pkg/anonymizer"
)

// isShuffleRows checks whether row shuffling is requested in options.
func isShuffleRows(options map[string]interface{}) bool {
	if options == nil {
		return false
	}
	if v, ok := options["shuffle_rows"].(bool); ok {
		return v
	}
	if v, ok := options["shuffleRows"].(bool); ok {
		return v
	}
	return false
}

// ShuffleRows shuffles rows in place using the Fisher-Yates algorithm
// seeded deterministically by the anonymizer's salt (e.g. DeriveSeed("shuffle")).
func ShuffleRows[T any](anon *anonymizer.Anonymizer, rows []T) {
	if len(rows) <= 1 || anon == nil {
		return
	}
	seed := int64(anon.DeriveSeed("shuffle") & 0x7fffffffffffffff)
	rng := rand.New(rand.NewSource(seed))
	rng.Shuffle(len(rows), func(i, j int) {
		rows[i], rows[j] = rows[j], rows[i]
	})
}
