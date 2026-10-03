package anonymizer_test

import (
	"strings"
	"testing"

	"dep_service/pkg/anonymizer"
)

func TestGenerateAddress(t *testing.T) {
	anon := anonymizer.New("salt-address-test")

	orig1 := "610000, Российская Федерация, Кировская обл., г. Киров, ул. Московская, д. 20"
	synth1 := anon.GenerateAddress(orig1)

	if synth1 == orig1 {
		t.Fatalf("expected address to be anonymized, got identical: %s", synth1)
	}
	if !strings.Contains(synth1, "Киров") {
		t.Errorf("expected preserved regional city in address: %s", synth1)
	}
	if strings.Contains(synth1, "ул. Московская, д. 20") {
		t.Errorf("expected original street and house to be anonymized, got: %s", synth1)
	}

	// Determinism
	synth1Again := anon.GenerateAddress(orig1)
	if synth1Again != synth1 {
		t.Errorf("expected deterministic address output, got %s vs %s", synth1, synth1Again)
	}

	// Registration with apartment
	orig2 := "610040, Кировская обл., г. Киров, ул. Торфяная, д. 11, корп. 1, кв. 284"
	synth2 := anon.GenerateAddress(orig2)
	if !strings.Contains(synth2, "кв.") {
		t.Errorf("expected synthetic apartment in result: %s", synth2)
	}
	if strings.Contains(synth2, "кв. 284") {
		t.Errorf("expected original apartment to be replaced: %s", synth2)
	}
}

func TestGenerateGrade(t *testing.T) {
	anon := anonymizer.New("salt-grade-test")

	// Numeric grade
	orig := "5"
	synth := anon.GenerateGrade(orig)
	if synth == orig {
		t.Fatalf("expected grade to be anonymized, got %s", synth)
	}
	// Determinism
	if anon.GenerateGrade(orig) != synth {
		t.Errorf("expected deterministic grade")
	}

	// Prefixed grade
	origPrefix := "Grade 7"
	synthPrefix := anon.GenerateGrade(origPrefix)
	if synthPrefix == origPrefix {
		t.Errorf("expected grade prefix to be anonymized: %s", synthPrefix)
	}
}

func TestGenerateEmployeeID(t *testing.T) {
	anon := anonymizer.New("salt-empid-test")

	orig := "746647"
	synth := anon.GenerateEmployeeID(orig)
	if synth == orig {
		t.Fatalf("expected employee ID to be modified, got %s", synth)
	}
	if len(synth) != len(orig) {
		t.Errorf("expected same length %d, got %d", len(orig), len(synth))
	}

	// With separator
	origSep := "659359-5"
	synthSep := anon.GenerateEmployeeID(origSep)
	if synthSep == origSep {
		t.Fatalf("expected employee ID to be modified, got %s", synthSep)
	}
	if len(synthSep) != len(origSep) || synthSep[6] != '-' {
		t.Errorf("expected separator '-' preserved at index 6, got %s", synthSep)
	}
}
