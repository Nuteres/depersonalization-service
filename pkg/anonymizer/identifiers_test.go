package anonymizer

import (
	"dep_service/pkg/detector"
	"testing"
)

func TestGenerateValidSNILS(t *testing.T) {
	anon := New("salt-152fz")
	generated := anon.GenerateSNILS("112-233-445 95")
	if !detector.ValidateSNILS(generated) {
		t.Fatalf("generated SNILS %s failed modulo 101 validation", generated)
	}

	// Determinism test
	generated2 := anon.GenerateSNILS("112-233-445 95")
	if generated != generated2 {
		t.Fatalf("expected deterministic generation: %s != %s", generated, generated2)
	}

	// Different inputs produce different outputs
	other := anon.GenerateSNILS("999-888-777 00")
	if generated == other {
		t.Fatalf("expected different outputs for different inputs: %s == %s", generated, other)
	}
}

func TestGenerateValidINN12(t *testing.T) {
	anon := New("salt-152fz")
	generated := anon.GenerateINN12("500100732259")
	if !detector.ValidateINN12(generated) {
		t.Fatalf("generated INN %s failed Russian FNS checksums", generated)
	}

	// Determinism test
	generated2 := anon.GenerateINN12("500100732259")
	if generated != generated2 {
		t.Fatalf("expected deterministic generation: %s != %s", generated, generated2)
	}

	// Different inputs produce different outputs
	other := anon.GenerateINN12("770110732200")
	if generated == other {
		t.Fatalf("expected different outputs for different inputs: %s == %s", generated, other)
	}
}
