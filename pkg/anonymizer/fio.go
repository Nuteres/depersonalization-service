package anonymizer

import (
	"fmt"
	"regexp"
	"strings"
)

type Gender int

const (
	GenderMale Gender = iota
	GenderFemale
)

func DetectGender(fio string) Gender {
	parts := strings.Fields(fio)
	if len(parts) == 0 {
		return GenderMale
	}

	// 1. Check patronymics first (strongest indicator)
	for _, p := range parts {
		lower := strings.ToLower(p)
		if strings.HasSuffix(lower, "овна") || strings.HasSuffix(lower, "евна") ||
			strings.HasSuffix(lower, "ична") || strings.HasSuffix(lower, "инична") ||
			strings.HasSuffix(lower, "кызы") {
			return GenderFemale
		}
		if strings.HasSuffix(lower, "ович") || strings.HasSuffix(lower, "евич") ||
			strings.HasSuffix(lower, "ич") || strings.HasSuffix(lower, "оглы") ||
			strings.HasSuffix(lower, "уулу") {
			return GenderMale
		}
	}

	// 2. Check surnames
	for _, p := range parts {
		lower := strings.ToLower(p)
		if strings.HasSuffix(lower, "ова") || strings.HasSuffix(lower, "ева") ||
			strings.HasSuffix(lower, "ина") || strings.HasSuffix(lower, "ына") ||
			strings.HasSuffix(lower, "ая") || strings.HasSuffix(lower, "яя") ||
			strings.HasSuffix(lower, "ская") || strings.HasSuffix(lower, "цкая") {
			return GenderFemale
		}
		if strings.HasSuffix(lower, "ов") || strings.HasSuffix(lower, "ев") ||
			strings.HasSuffix(lower, "ин") || strings.HasSuffix(lower, "ын") ||
			strings.HasSuffix(lower, "ский") || strings.HasSuffix(lower, "цкий") ||
			strings.HasSuffix(lower, "ий") || strings.HasSuffix(lower, "ый") ||
			strings.HasSuffix(lower, "ой") {
			return GenderMale
		}
	}

	// 3. Check first names
	maleExceptions := map[string]bool{
		"никита": true, "илья": true, "данила": true, "даниил": true,
		"лука": true, "фома": true, "кузьма": true, "савва": true,
		"миша": true, "саша": true, "женя": true, "сережа": true,
	}

	for _, p := range parts {
		lower := strings.ToLower(p)
		for _, fn := range FemaleNames {
			if strings.ToLower(fn) == lower {
				return GenderFemale
			}
		}
		for _, mn := range MaleNames {
			if strings.ToLower(mn) == lower {
				return GenderMale
			}
		}
		if maleExceptions[lower] {
			return GenderMale
		}
		if strings.HasSuffix(lower, "а") || strings.HasSuffix(lower, "я") {
			return GenderFemale
		}
	}

	return GenderMale
}

var MaleSurnames = []string{"Смирнов", "Иванов", "Кузнецов", "Попов", "Соколов", "Лебедев", "Козлов", "Новиков", "Морозов", "Петров", "Волков", "Соловьев", "Васильев", "Зайцев", "Павлов"}
var FemaleSurnames = []string{"Смирнова", "Иванова", "Кузнецова", "Попова", "Соколова", "Лебедева", "Козлова", "Новикова", "Морозова", "Петрова", "Волкова", "Соловьева", "Васильева", "Зайцева", "Павлова"}

var MaleNames = []string{"Александр", "Михаил", "Максим", "Артем", "Даниил", "Иван", "Дмитрий", "Кирилл", "Андрей", "Егор", "Роман", "Сергей", "Никита"}
var FemaleNames = []string{"София", "Анна", "Мария", "Алиса", "Ева", "Виктория", "Полина", "Екатерина", "Ксения", "Дарья", "Анастасия", "Ольга"}

var MalePatronymics = []string{"Александрович", "Михайлович", "Максимович", "Иванович", "Дмитриевич", "Андреевич", "Сергеевич", "Романович", "Павлович"}
var FemalePatronymics = []string{"Александровна", "Михайловна", "Максимовна", "Ивановна", "Дмитриевна", "Андреевна", "Сергеевна", "Романовна", "Павловна"}

var (
	reSurnameInitials = regexp.MustCompile(`^([А-ЯЁ][а-яё]+(?:-[А-ЯЁ][а-яё]+)?)\s+([А-ЯЁ])\.\s*([А-ЯЁ])\.$`)
	reInitialsSurname = regexp.MustCompile(`^([А-ЯЁ])\.\s*([А-ЯЁ])\.\s+([А-ЯЁ][а-яё]+(?:-[А-ЯЁ][а-яё]+)?)$`)
	reSurnameOneInit  = regexp.MustCompile(`^([А-ЯЁ][а-яё]+(?:-[А-ЯЁ][а-яё]+)?)\s+([А-ЯЁ])\.$`)
	reOneInitSurname  = regexp.MustCompile(`^([А-ЯЁ])\.\s+([А-ЯЁ][а-яё]+(?:-[А-ЯЁ][а-яё]+)?)$`)
)

func (a *Anonymizer) GenerateFIO(original string) string {
	trimmed := strings.TrimSpace(original)
	if trimmed == "" {
		return original
	}
	seed := a.DeriveSeed(trimmed)
	gender := DetectGender(trimmed)

	var s, n, p string
	if gender == GenderFemale {
		sIdx := a.RandInt(seed, 0, len(FemaleSurnames)-1)
		nIdx := a.RandInt(seed+1, 0, len(FemaleNames)-1)
		pIdx := a.RandInt(seed+2, 0, len(FemalePatronymics)-1)
		s, n, p = FemaleSurnames[sIdx], FemaleNames[nIdx], FemalePatronymics[pIdx]
	} else {
		sIdx := a.RandInt(seed, 0, len(MaleSurnames)-1)
		nIdx := a.RandInt(seed+1, 0, len(MaleNames)-1)
		pIdx := a.RandInt(seed+2, 0, len(MalePatronymics)-1)
		s, n, p = MaleSurnames[sIdx], MaleNames[nIdx], MalePatronymics[pIdx]
	}

	// 1. Check initials formats
	if reSurnameInitials.MatchString(trimmed) {
		return fmt.Sprintf("%s %c.%c.", s, []rune(n)[0], []rune(p)[0])
	}
	if reInitialsSurname.MatchString(trimmed) {
		return fmt.Sprintf("%c.%c. %s", []rune(n)[0], []rune(p)[0], s)
	}
	if reSurnameOneInit.MatchString(trimmed) {
		return fmt.Sprintf("%s %c.", s, []rune(n)[0])
	}
	if reOneInitSurname.MatchString(trimmed) {
		return fmt.Sprintf("%c. %s", []rune(n)[0], s)
	}

	// 2. Check 2-word names
	fields := strings.Fields(trimmed)
	if len(fields) == 2 {
		firstLower := strings.ToLower(fields[0])
		isFirstNameFirst := false
		for _, fn := range FemaleNames {
			if strings.ToLower(fn) == firstLower {
				isFirstNameFirst = true
				break
			}
		}
		if !isFirstNameFirst {
			for _, mn := range MaleNames {
				if strings.ToLower(mn) == firstLower {
					isFirstNameFirst = true
					break
				}
			}
		}
		if isFirstNameFirst {
			return fmt.Sprintf("%s %s", n, s)
		}
		return fmt.Sprintf("%s %s", s, n)
	}

	return fmt.Sprintf("%s %s %s", s, n, p)
}

// GenerateLastName генерирует только синтетическую фамилию с сохранением пола исходного носителя.
func (a *Anonymizer) GenerateLastName(original string) string {
	trimmed := strings.TrimSpace(original)
	if trimmed == "" {
		return original
	}
	seed := a.DeriveSeed(trimmed)
	gender := DetectGender(trimmed)

	if gender == GenderFemale {
		sIdx := a.RandInt(seed, 0, len(FemaleSurnames)-1)
		return FemaleSurnames[sIdx]
	}
	sIdx := a.RandInt(seed, 0, len(MaleSurnames)-1)
	return MaleSurnames[sIdx]
}

// GenerateFirstName генерирует только синтетическое имя с сохранением пола исходного носителя.
func (a *Anonymizer) GenerateFirstName(original string) string {
	trimmed := strings.TrimSpace(original)
	if trimmed == "" {
		return original
	}
	seed := a.DeriveSeed(trimmed)
	gender := DetectGender(trimmed)

	if gender == GenderFemale {
		nIdx := a.RandInt(seed, 0, len(FemaleNames)-1)
		return FemaleNames[nIdx]
	}
	nIdx := a.RandInt(seed, 0, len(MaleNames)-1)
	return MaleNames[nIdx]
}

// GeneratePatronymic генерирует только синтетическое отчество с сохранением пола исходного носителя.
func (a *Anonymizer) GeneratePatronymic(original string) string {
	trimmed := strings.TrimSpace(original)
	if trimmed == "" {
		return original
	}
	seed := a.DeriveSeed(trimmed)
	gender := DetectGender(trimmed)

	if gender == GenderFemale {
		pIdx := a.RandInt(seed, 0, len(FemalePatronymics)-1)
		return FemalePatronymics[pIdx]
	}
	pIdx := a.RandInt(seed, 0, len(MalePatronymics)-1)
	return MalePatronymics[pIdx]
}
