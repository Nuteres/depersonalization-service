package detector

import "testing"

func TestSNILSValidation(t *testing.T) {
	// Valid SNILS with correct modulo 101 checksum
	validSNILS := "112-233-445 95"
	if !ValidateSNILS(validSNILS) {
		t.Fatalf("expected %s to be valid", validSNILS)
	}
	invalidSNILS := "112-233-445 00"
	if ValidateSNILS(invalidSNILS) {
		t.Fatalf("expected %s to be invalid", invalidSNILS)
	}

	// Edge cases
	if ValidateSNILS("") {
		t.Fatalf("expected empty string to be invalid SNILS")
	}
	if ValidateSNILS("12345") {
		t.Fatalf("expected short string to be invalid SNILS")
	}
	// SNILS <= 001001998 is valid without checksum check
	if !ValidateSNILS("001-001-998 00") {
		t.Fatalf("expected SNILS <= 001001998 to be valid")
	}
}

func TestINN12Validation(t *testing.T) {
	// Valid 12-digit Russian personal INN with official weights
	validINN := "500100732259"
	if !ValidateINN12(validINN) {
		t.Fatalf("expected %s to be valid INN-12", validINN)
	}
	invalidINN := "500100732250"
	if ValidateINN12(invalidINN) {
		t.Fatalf("expected %s to be invalid INN-12", invalidINN)
	}

	// Edge cases
	if ValidateINN12("") {
		t.Fatalf("expected empty string to be invalid INN-12")
	}
	if ValidateINN12("50010073225") {
		t.Fatalf("expected 11-digit string to be invalid INN-12")
	}
	if ValidateINN12("5001007322590") {
		t.Fatalf("expected 13-digit string to be invalid INN-12")
	}
}

func TestCardValidation(t *testing.T) {
	// The number from user document that falsely triggered bank card
	userAppId := "4612525897100570"
	if ValidateCardNumber(userAppId) {
		t.Fatalf("expected user app-id %s to be INVALID as a bank card", userAppId)
	}
	if ValidateCardLuhn(userAppId) {
		t.Fatalf("expected user app-id %s to fail Luhn checksum", userAppId)
	}

	// Valid card numbers
	validVisa := "4532015112830366"
	if !ValidateCardNumber(validVisa) {
		t.Fatalf("expected valid Visa %s to pass", validVisa)
	}
	validMir := "2200 0000 0000 0004" // 2200000000000004: Luhn sum 10 -> valid
	if !ValidateCardNumber(validMir) {
		t.Fatalf("expected valid Mir %s to pass", validMir)
	}

	// Invalid prefix or short length
	if ValidateCardNumber("1234567890123456") {
		t.Fatalf("expected unknown prefix to be invalid card")
	}
	if ValidateCardNumber("4532") {
		t.Fatalf("expected short number to be invalid card")
	}
}

