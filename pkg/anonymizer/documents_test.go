package anonymizer

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestGeneratePassport(t *testing.T) {
	anon := New("salt-documents")
	orig := "4510 123456"
	passport := anon.GeneratePassport(orig)

	// Format check: 4 digits series, space, 6 digits number
	matched, err := regexp.MatchString(`^\d{4} \d{6}$`, passport)
	if err != nil || !matched {
		t.Fatalf("expected passport format 'XXXX XXXXXX', got '%s'", passport)
	}

	parts := strings.Split(passport, " ")
	region, err := strconv.Atoi(parts[0][:2])
	if err != nil || region < 1 || region > 99 {
		t.Fatalf("expected valid region 1..99 in series, got %s", parts[0])
	}

	// Determinism checks
	anonSame := New("salt-documents")
	if anonSame.GeneratePassport(orig) != passport {
		t.Fatalf("expected identical passport for same salt and input")
	}

	anonDiff := New("different-salt")
	if anonDiff.GeneratePassport(orig) == passport {
		t.Fatalf("expected different passport for different salt")
	}

	if anon.GeneratePassport("7702 654321") == passport {
		t.Fatalf("expected different passport for different original")
	}
}

func TestGeneratePhone(t *testing.T) {
	anon := New("salt-phone")
	orig := "+7 999 123-45-67"
	phone := anon.GeneratePhone(orig)

	// Format check: +7 9xx xxx-xx-xx
	matched, err := regexp.MatchString(`^\+7 9\d{2} \d{3}-\d{2}-\d{2}$`, phone)
	if err != nil || !matched {
		t.Fatalf("expected phone format '+7 9xx xxx-xx-xx', got '%s'", phone)
	}

	// Determinism checks
	anonSame := New("salt-phone")
	if anonSame.GeneratePhone(orig) != phone {
		t.Fatalf("expected identical phone for same salt and input")
	}

	anonDiff := New("other-salt")
	if anonDiff.GeneratePhone(orig) == phone {
		t.Fatalf("expected different phone for different salt")
	}

	if anon.GeneratePhone("+7 (916) 000-00-00") == phone {
		t.Fatalf("expected different phone for different input")
	}
}

func TestPerturbDate(t *testing.T) {
	anon := New("salt-dates")

	testCases := []struct {
		original string
		layout   string
		rangeDays int
	}{
		{"1990-05-15", "2006-01-02", 30},
		{"15.05.1985", "02.01.2006", 45},
		{"01/12/2000", "02/01/2006", 15},
		{"2021-08-20 14:30:00", "2006-01-02 15:04:05", 10},
	}

	for _, tc := range testCases {
		perturbed := anon.PerturbDate(tc.original, tc.rangeDays)
		if perturbed == tc.original {
			t.Fatalf("expected perturbed date to differ from original: %s", tc.original)
		}

		tOrig, err := time.Parse(tc.layout, tc.original)
		if err != nil {
			t.Fatalf("failed to parse original date %s with layout %s: %v", tc.original, tc.layout, err)
		}

		tPerturbed, err := time.Parse(tc.layout, perturbed)
		if err != nil {
			t.Fatalf("failed to parse perturbed date %s with layout %s: %v", perturbed, tc.layout, err)
		}

		diffHours := tPerturbed.Sub(tOrig).Hours()
		diffDays := int(diffHours / 24)
		if diffDays < -tc.rangeDays || diffDays > tc.rangeDays {
			t.Fatalf("perturbed date shift %d days outside allowed range [-%d, %d]", diffDays, tc.rangeDays, tc.rangeDays)
		}
	}

	// Determinism check
	if anon.PerturbDate("1995-10-10", 30) != anon.PerturbDate("1995-10-10", 30) {
		t.Fatalf("expected deterministic perturbation for same input")
	}

	anonDiff := New("other-salt-dates")
	if anonDiff.PerturbDate("1995-10-10", 30) == anon.PerturbDate("1995-10-10", 30) {
		t.Fatalf("expected different perturbation with different salt")
	}

	// Edge cases
	if empty := anon.PerturbDate("", 10); empty != "" {
		t.Fatalf("expected empty string for empty input, got %s", empty)
	}

	invalid := "not-a-date"
	if res := anon.PerturbDate(invalid, 10); res != invalid {
		t.Fatalf("expected invalid date to return original, got %s", res)
	}

	zeroRange := "1990-05-15"
	if res := anon.PerturbDate(zeroRange, 0); res != zeroRange {
		t.Fatalf("expected zero range to return original date, got %s", res)
	}
}

func TestBatchDocuments(t *testing.T) {
	anon := New("salt-batch-docs")
	for i := 0; i < 50; i++ {
		orig := fmt.Sprintf("doc-%d", i)
		passport := anon.GeneratePassport(orig)
		phone := anon.GeneratePhone(orig)

		matchedPass, _ := regexp.MatchString(`^\d{4} \d{6}$`, passport)
		if !matchedPass {
			t.Fatalf("invalid passport format on iteration %d: %s", i, passport)
		}

		matchedPhone, _ := regexp.MatchString(`^\+7 9\d{2} \d{3}-\d{2}-\d{2}$`, phone)
		if !matchedPhone {
			t.Fatalf("invalid phone format on iteration %d: %s", i, phone)
		}
	}
}
