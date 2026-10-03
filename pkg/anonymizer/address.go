package anonymizer

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var (
	rePostalCode = regexp.MustCompile(`\b\d{6}\b`)

	SyntheticStreets = []string{
		"ул. Лесная", "ул. Садовая", "ул. Центральная", "ул. Советская",
		"ул. Молодежная", "ул. Школьная", "ул. Новая", "ул. Полевая",
		"ул. Мира", "ул. Гагарина", "ул. Набережная", "ул. Солнечная",
		"ул. Цветочная", "ул. Победы", "ул. Зеленая", "ул. Березовая",
		"ул. Октябрьская", "ул. Комсомольская", "ул. Строителей", "пр-кт Мира",
		"ул. Пушкина", "ул. Чехова", "ул. Маяковского", "ул. Парковая",
	}

	SyntheticCities = []string{
		"г. Москва", "г. Санкт-Петербург", "г. Новосибирск", "г. Екатеринбург",
		"г. Казань", "г. Нижний Новгород", "г. Самара", "г. Омск",
		"г. Челябинск", "г. Ростов-на-Дону", "г. Уфа", "г. Волгоград",
		"г. Пермь", "г. Красноярск", "г. Воронеж", "г. Саратов",
		"г. Краснодар", "г. Тюмень", "г. Ижевск", "г. Барнаул",
		"г. Киров", "г. Ярославль", "г. Хабаровск", "г. Оренбург",
	}

	SyntheticRegions = []string{
		"Московская область", "Ленинградская область", "Новосибирская область",
		"Свердловская область", "Нижегородская область", "Самарская область",
		"Республика Татарстан", "Краснодарский край", "Пермский край",
		"Воронежская область", "Ростовская область", "Челябинская область",
	}
)

// GenerateAddress генерирует детерминированный синтетический адрес на русском языке,
// сохраняя при наличии региональный контекст (город, субъект РФ), но маскируя персональную часть (улица, дом, квартира).
func (a *Anonymizer) GenerateAddress(original string) string {
	trimmed := strings.TrimSpace(original)
	if trimmed == "" {
		return original
	}

	seed := a.DeriveSeed(trimmed)
	streetIdx := int(seed % uint64(len(SyntheticStreets)))
	synthStreet := SyntheticStreets[streetIdx]
	synthHouse := 1 + int((seed/10)%120)
	synthApt := 1 + int((seed/100)%250)
	synthPostal := fmt.Sprintf("%06d", 100000+int((seed/1000)%900000))

	lowerTrimmed := strings.ToLower(trimmed)
	hasApt := strings.Contains(lowerTrimmed, "кв.") || strings.Contains(lowerTrimmed, "кв ") || strings.Contains(lowerTrimmed, "квартир")
	hasOriginalPostal := rePostalCode.MatchString(trimmed)

	hasStreetOrHouse := strings.Contains(lowerTrimmed, "ул.") || strings.Contains(lowerTrimmed, "улица") ||
		strings.Contains(lowerTrimmed, "пр-кт") || strings.Contains(lowerTrimmed, "проспект") ||
		strings.Contains(lowerTrimmed, "пер.") || strings.Contains(lowerTrimmed, "переулок") ||
		strings.Contains(lowerTrimmed, "ш.") || strings.Contains(lowerTrimmed, "шоссе") ||
		strings.Contains(lowerTrimmed, "б-р") || strings.Contains(lowerTrimmed, "бульвар") ||
		strings.Contains(lowerTrimmed, "наб.") || strings.Contains(lowerTrimmed, "набережная") ||
		strings.Contains(lowerTrimmed, "д.") || strings.Contains(lowerTrimmed, "дом") ||
		strings.Contains(lowerTrimmed, "корп") || strings.Contains(lowerTrimmed, "кв.") ||
		strings.Contains(lowerTrimmed, "квартира") || strings.Contains(lowerTrimmed, "стр.") ||
		strings.Contains(lowerTrimmed, "литер")

	// If it's only a region
	if !hasStreetOrHouse && !hasOriginalPostal && (strings.Contains(lowerTrimmed, "обл") || strings.Contains(lowerTrimmed, "край") ||
		strings.Contains(lowerTrimmed, "респ") || strings.Contains(lowerTrimmed, "округ")) {
		regIdx := int(seed % uint64(len(SyntheticRegions)))
		if strings.HasSuffix(lowerTrimmed, " обл") {
			return strings.TrimSuffix(SyntheticRegions[regIdx], " область") + " обл"
		}
		return SyntheticRegions[regIdx]
	}

	// If it's only a city
	if !hasStreetOrHouse && !hasOriginalPostal && (strings.HasPrefix(lowerTrimmed, "г.") || strings.HasPrefix(lowerTrimmed, "г ") ||
		strings.HasPrefix(lowerTrimmed, "город ") || strings.HasSuffix(lowerTrimmed, " г.") ||
		strings.HasSuffix(lowerTrimmed, " г")) {
		cityIdx := int(seed % uint64(len(SyntheticCities)))
		synthCity := SyntheticCities[cityIdx]
		if strings.HasPrefix(lowerTrimmed, "г. ") {
			return synthCity
		} else if strings.HasPrefix(lowerTrimmed, "г.") {
			return "г." + strings.TrimPrefix(synthCity, "г. ")
		}
		return synthCity
	}

	parts := strings.Split(trimmed, ",")
	if len(parts) >= 2 {
		// Complex address with multiple comma-separated tokens (e.g. postal, country, region, city, street, house, apt)
		var preserved []string

		for _, p := range parts {
			cleanPart := strings.TrimSpace(p)
			lower := strings.ToLower(cleanPart)

			// Check if this part is postal code, street, house, corp, apt
			if rePostalCode.MatchString(cleanPart) {
				continue
			}
			if strings.Contains(lower, "ул.") || strings.Contains(lower, "улица") ||
				strings.Contains(lower, "пр-кт") || strings.Contains(lower, "проспект") ||
				strings.Contains(lower, "пер.") || strings.Contains(lower, "переулок") ||
				strings.Contains(lower, "ш.") || strings.Contains(lower, "шоссе") ||
				strings.Contains(lower, "б-р") || strings.Contains(lower, "бульвар") ||
				strings.Contains(lower, "наб.") || strings.Contains(lower, "набережная") ||
				strings.Contains(lower, "д.") || strings.Contains(lower, "дом") ||
				strings.Contains(lower, "корп") || strings.Contains(lower, "кв.") ||
				strings.Contains(lower, "квартира") || strings.Contains(lower, "стр.") ||
				strings.Contains(lower, "литер") {
				continue
			}

			// Retain country, region, district, city
			if cleanPart != "" {
				preserved = append(preserved, cleanPart)
			}
		}

		var res []string
		if hasOriginalPostal {
			res = append(res, synthPostal)
		}
		res = append(res, preserved...)
		res = append(res, synthStreet, fmt.Sprintf("д. %d", synthHouse))
		if hasApt {
			res = append(res, fmt.Sprintf("кв. %d", synthApt))
		}

		return strings.Join(res, ", ")
	}

	// Single-line or unstructured address: generate realistic full address
	cityIdx := int(seed % uint64(len(SyntheticCities)))
	synthCity := SyntheticCities[cityIdx]

	if hasApt {
		return fmt.Sprintf("%s, %s, д. %d, кв. %d", synthCity, synthStreet, synthHouse, synthApt)
	}
	return fmt.Sprintf("%s, %s, д. %d", synthCity, synthStreet, synthHouse)
}

// GenerateGrade генерирует детерминированное синтетическое значение грейда/разряда.
// Для числовых значений применяется безопасное смещение (+-1 или +-2), для строковых - псевдоним.
func (a *Anonymizer) GenerateGrade(original string) string {
	trimmed := strings.TrimSpace(original)
	if trimmed == "" {
		return original
	}

	seed := a.DeriveSeed(trimmed)

	// If numeric grade (e.g. "5", "12")
	if num, err := strconv.Atoi(trimmed); err == nil {
		deltas := []int{1, -1, 2, -2}
		delta := deltas[seed%uint64(len(deltas))]
		newGrade := num + delta
		if newGrade < 1 {
			newGrade = 1 + int(seed%3)
		}
		return strconv.Itoa(newGrade)
	}

	// If prefixed with G/Grade/Грейд (e.g. "G5", "Grade 4", "Грейд 3")
	rePrefix := regexp.MustCompile(`(?i)^(grade|грейд|g|l)[\s\-_]*(\d+)$`)
	if match := rePrefix.FindStringSubmatch(trimmed); len(match) == 3 {
		prefix := match[1]
		if num, err := strconv.Atoi(match[2]); err == nil {
			deltas := []int{1, -1, 2, -2}
			delta := deltas[seed%uint64(len(deltas))]
			newGrade := num + delta
			if newGrade < 1 {
				newGrade = 1 + int(seed%3)
			}
			return fmt.Sprintf("%s %d", prefix, newGrade)
		}
	}

	// Fallback deterministic string
	return fmt.Sprintf("Грейд %d", 1+int(seed%15))
}

// GenerateEmployeeID генерирует детерминированный синтетический табельный номер или персональный идентификатор,
// сохраняя точную длину и структуру исходного значения (разделители, дефисы).
func (a *Anonymizer) GenerateEmployeeID(original string) string {
	trimmed := strings.TrimSpace(original)
	if trimmed == "" {
		return original
	}

	seed := a.DeriveSeed(trimmed)
	offset := 1 + int(seed%9) // 1..9, guarantees every digit changes

	var b strings.Builder
	b.Grow(len(trimmed))

	for _, r := range trimmed {
		if r >= '0' && r <= '9' {
			origD := int(r - '0')
			newD := (origD + offset) % 10
			b.WriteByte(byte('0' + newD))
		} else {
			b.WriteRune(r)
		}
	}

	return b.String()
}
