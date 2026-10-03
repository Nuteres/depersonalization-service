package anonymizer

import (
	"strings"
	"testing"
)

func TestGenderPreservation(t *testing.T) {
	anon := New("salt-fio")
	maleOriginal := "Иванов Иван Иванович"
	maleGen := anon.GenerateFIO(maleOriginal)
	if DetectGender(maleGen) != GenderMale {
		t.Fatalf("expected male replacement for %s, got %s", maleOriginal, maleGen)
	}

	femaleOriginal := "Смирнова Ольга Алексеевна"
	femaleGen := anon.GenerateFIO(femaleOriginal)
	if DetectGender(femaleGen) != GenderFemale {
		t.Fatalf("expected female replacement for %s, got %s", femaleOriginal, femaleGen)
	}
}

func TestFIODeterminism(t *testing.T) {
	anon1 := New("salt-1")
	anon2 := New("salt-1")
	anonDiff := New("salt-2")

	input := "Петров Петр Петрович"
	gen1 := anon1.GenerateFIO(input)
	gen2 := anon2.GenerateFIO(input)
	genDiff := anonDiff.GenerateFIO(input)

	if gen1 != gen2 {
		t.Fatalf("expected identical output for same salt and input: got %q vs %q", gen1, gen2)
	}
	if gen1 == genDiff {
		t.Fatalf("expected different output for different salts: got %q", gen1)
	}
}

func TestDetectGender(t *testing.T) {
	tests := []struct {
		fio      string
		expected Gender
	}{
		{"Иванов Иван Иванович", GenderMale},
		{"Петров Алексей Сергеевич", GenderMale},
		{"Иванова Анна Ивановна", GenderFemale},
		{"Смирнова Екатерина Алексеевна", GenderFemale},
		{"Кузнецова Мария Ильинична", GenderFemale},
		{"Белая Елена Викторовна", GenderFemale},
	}

	for _, tt := range tests {
		got := DetectGender(tt.fio)
		if got != tt.expected {
			t.Errorf("DetectGender(%q) = %v; want %v", tt.fio, got, tt.expected)
		}
	}
}

func TestSinglePartGenerators(t *testing.T) {
	anon := New("salt-single")

	// Last Name: Male and Female
	maleSurname := anon.GenerateLastName("Самедов")
	if len(strings.Fields(maleSurname)) != 1 {
		t.Fatalf("expected single-word surname, got %q", maleSurname)
	}
	if DetectGender(maleSurname) != GenderMale {
		t.Errorf("expected male surname for 'Самедов', got %q", maleSurname)
	}

	femaleSurname := anon.GenerateLastName("Кудряшова")
	if len(strings.Fields(femaleSurname)) != 1 {
		t.Fatalf("expected single-word surname, got %q", femaleSurname)
	}
	if DetectGender(femaleSurname) != GenderFemale {
		t.Errorf("expected female surname for 'Кудряшова', got %q", femaleSurname)
	}

	// First Name: Male and Female
	maleName := anon.GenerateFirstName("Самед")
	if len(strings.Fields(maleName)) != 1 {
		t.Fatalf("expected single-word first name, got %q", maleName)
	}

	femaleName := anon.GenerateFirstName("Владислава")
	if len(strings.Fields(femaleName)) != 1 {
		t.Fatalf("expected single-word first name, got %q", femaleName)
	}
	if DetectGender(femaleName) != GenderFemale {
		t.Errorf("expected female name for 'Владислава', got %q", femaleName)
	}

	// Patronymic: Male and Female
	malePatr := anon.GeneratePatronymic("Намиг Оглы")
	if len(strings.Fields(malePatr)) != 1 {
		t.Fatalf("expected single-word patronymic, got %q", malePatr)
	}
	if DetectGender(malePatr) != GenderMale {
		t.Errorf("expected male patronymic for 'Намиг Оглы', got %q", malePatr)
	}

	femalePatr := anon.GeneratePatronymic("Витальевна")
	if len(strings.Fields(femalePatr)) != 1 {
		t.Fatalf("expected single-word patronymic, got %q", femalePatr)
	}
	if DetectGender(femalePatr) != GenderFemale {
		t.Errorf("expected female patronymic for 'Витальевна', got %q", femalePatr)
	}

	// Determinism test
	anon2 := New("salt-single")
	if anon.GenerateLastName("Самедов") != anon2.GenerateLastName("Самедов") {
		t.Errorf("expected deterministic surname generation")
	}
}
