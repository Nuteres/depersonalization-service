package anonymizer

import (
	"regexp"
	"testing"
)

func TestTokenSession_GenerateToken_FormattingAndDLPImmunity(t *testing.T) {
	ts := NewTokenSession("test-salt")

	fio1 := ts.GenerateToken(TokenFIO, "Иванов Иван Иванович")
	fio2 := ts.GenerateToken(TokenFIO, "Иванов Иван Иванович")
	fio3 := ts.GenerateToken(TokenFIO, "Петров Петр Петрович")

	if fio1 != "[ФИО_0001]" {
		t.Fatalf("expected [ФИО_0001], got %s", fio1)
	}
	if fio2 != fio1 {
		t.Fatalf("expected deterministic token for identical original value, got %s != %s", fio2, fio1)
	}
	if fio3 != "[ФИО_0002]" {
		t.Fatalf("expected [ФИО_0002], got %s", fio3)
	}

	pass := ts.GenerateToken(TokenPassport, "4508 123456")
	if pass != "[Паспорт_0001]" {
		t.Fatalf("expected [Паспорт_0001], got %s", pass)
	}

	// Verify passport regex does NOT match token
	rePassport := regexp.MustCompile(`^\d{2}\s?\d{2}\s?\d{6}$`)
	if rePassport.MatchString(pass) {
		t.Fatalf("token %s must not match passport regex", pass)
	}

	// Verify phone regex does NOT match token
	phone := ts.GenerateToken(TokenPhone, "+7 916 123-45-67")
	rePhone := regexp.MustCompile(`^(\+7|8)\s?\(?9\d{2}\)?\s?\d{3}-?\d{2}-?\d{2}$`)
	if rePhone.MatchString(phone) {
		t.Fatalf("token %s must not match phone regex", phone)
	}

	if !IsBracketedToken(fio1) || !IsBracketedToken(pass) {
		t.Fatalf("expected IsBracketedToken to be true for tokens")
	}
	if IsBracketedToken("обычный текст") || IsBracketedToken("[Случайный текст с пробелом]") {
		t.Fatalf("expected IsBracketedToken to be false for non-tokens")
	}
}
