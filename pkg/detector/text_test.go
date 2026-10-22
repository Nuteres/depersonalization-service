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
	for _, m := range matches {
		if m.EntityType == detector.EntitySNILS {
			t.Errorf("expected SNILS to be disabled, but got match: %s", m.Value)
		}
	}
}
