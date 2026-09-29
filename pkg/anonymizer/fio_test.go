package anonymizer

import "testing"

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
