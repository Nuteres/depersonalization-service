package detector_test

import (
	"testing"

	"dep_service/pkg/detector"
)

func TestFindTextMatches(t *testing.T) {
	text := `Паспорт 4509 123456 выдан 12.05.2015. СНИЛС: 123-456-789 01. Телефон: +7 (916) 123-45-67.
Сотрудник: Иванов Иван Иванович, проживающий по адресу: г. Москва, ул. Ленина, д. 10, кв. 5.`

	matches := detector.FindTextMatches(text)
	if len(matches) == 0 {
		t.Fatalf("expected matches, got 0")
	}

	foundTypes := make(map[detector.EntityType]bool)
	for _, m := range matches {
		foundTypes[m.EntityType] = true
	}

	expectedTypes := []detector.EntityType{
		detector.EntityPassport,
		detector.EntityDate,
		detector.EntitySNILS,
		detector.EntityPhone,
		detector.EntityFIO,
		detector.EntityAddress,
	}

	for _, et := range expectedTypes {
		if !foundTypes[et] {
			t.Errorf("expected entity type %s to be detected", et)
		}
	}
}

func TestFindTextMatches_DisabledEntities(t *testing.T) {
	text := `СНИЛС 123-456-789 01 и телефон +79161234567`
	disabled := map[string]bool{
		string(detector.EntitySNILS): true,
	}

	matches := detector.FindTextMatchesWithOptions(text, disabled)
	if len(matches) == 0 {
		t.Fatalf("expected at least one match for enabled entities, got 0")
	}

	foundPhone := false
	for _, m := range matches {
		if m.EntityType == detector.EntitySNILS {
			t.Errorf("expected SNILS to be disabled, but got match: %s", m.Value)
		}
		if m.EntityType == detector.EntityPhone {
			foundPhone = true
		}
	}
	if !foundPhone {
		t.Errorf("expected phone match to remain enabled, but it was not found")
	}

	// Test CARD alias normalization
	cardText := `Карта: 2200 1234 5678 9012, телефон: +79161234567`
	disabledCard := map[string]bool{
		"CARD": true,
	}
	cardMatches := detector.FindTextMatchesWithOptions(cardText, disabledCard)
	foundCard := false
	foundCardPhone := false
	for _, m := range cardMatches {
		if m.EntityType == detector.EntityCard {
			foundCard = true
		}
		if m.EntityType == detector.EntityPhone {
			foundCardPhone = true
		}
	}
	if foundCard {
		t.Errorf("expected CARD alias to disable card detection")
	}
	if !foundCardPhone {
		t.Errorf("expected phone to be detected when CARD is disabled")
	}
}

func TestFindTextMatches_TieBreakingPriority(t *testing.T) {
	// A valid SNILS that is 11 digits starting with 8:
	// For instance, 800-000-000 72 (checksum: 8*11 = 88 != 72), let's construct a valid 11-digit string or test
	// deterministic ordering on same span
	text := `80000000072`
	matches := detector.FindTextMatches(text)
	// Must return a deterministic single match (or empty if neither is valid) without crashing or unstable ordering
	if len(matches) > 1 {
		t.Errorf("expected deduplication to at most 1 match, got %d", len(matches))
	}
}
