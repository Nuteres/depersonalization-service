package anonymizer

import "testing"

func TestDeterministicSeed(t *testing.T) {
	anon := New("secret-salt-152fz")
	seed1 := anon.DeriveSeed("Иванов Иван Иванович")
	seed2 := anon.DeriveSeed("Иванов Иван Иванович")
	seed3 := anon.DeriveSeed("Петров Петр Петрович")

	if seed1 != seed2 {
		t.Fatalf("expected identical seed for same input, got %d and %d", seed1, seed2)
	}
	if seed1 == seed3 {
		t.Fatalf("expected different seed for different input, got %d", seed1)
	}
}

func TestRandIntRange(t *testing.T) {
	anon := New("salt")
	seed := anon.DeriveSeed("test")
	val := anon.RandInt(seed, 10, 20)
	if val < 10 || val > 20 {
		t.Fatalf("RandInt out of range: %d", val)
	}
}

func TestRandIntEdgeCases(t *testing.T) {
	anon := New("salt")
	seed := anon.DeriveSeed("test")

	// min >= max
	if val := anon.RandInt(seed, 20, 10); val != 20 {
		t.Fatalf("expected min when min > max, got %d", val)
	}
	if val := anon.RandInt(seed, 15, 15); val != 15 {
		t.Fatalf("expected min when min == max, got %d", val)
	}

	// Determinism
	val1 := anon.RandInt(seed, 1, 100)
	val2 := anon.RandInt(seed, 1, 100)
	if val1 != val2 {
		t.Fatalf("expected deterministic RandInt result for same seed, got %d and %d", val1, val2)
	}
}

func TestGenerateSalt(t *testing.T) {
	salt1, err := GenerateSalt(16)
	if err != nil {
		t.Fatalf("unexpected error generating salt: %v", err)
	}
	if len(salt1) != 32 {
		t.Fatalf("expected 32 hex chars for 16 bytes salt, got %d (val: %s)", len(salt1), salt1)
	}

	salt2, err := GenerateSalt(16)
	if err != nil {
		t.Fatalf("unexpected error generating salt: %v", err)
	}
	if salt1 == salt2 {
		t.Fatalf("expected different salts from successive GenerateSalt calls, got identical: %s", salt1)
	}

	// Default length when <= 0
	saltDefault, err := GenerateSalt(0)
	if err != nil {
		t.Fatalf("unexpected error generating salt with default length: %v", err)
	}
	if len(saltDefault) != 32 {
		t.Fatalf("expected 32 hex chars for default length (16 bytes), got %d", len(saltDefault))
	}
}
