package anonymizer

import (
	"fmt"
	"strings"
	"time"
)

var recognizedDateLayouts = []string{
	"2006-01-02",
	"02.01.2006",
	"02/01/2006",
	"2006/01/02",
	"2006.01.02",
	"02-01-2006",
	"2006-01-02 15:04:05",
	"02.01.2006 15:04:05",
	"2006-01-02T15:04:05Z07:00",
	"2006-01-02T15:04:05",
}

// GeneratePassport генерирует синтетическую серию (4 цифры: 2 цифры региона + 2 цифры года)
// и номер (6 цифр) паспорта гражданина РФ.
func (a *Anonymizer) GeneratePassport(original string) string {
	seed := a.DeriveSeed(original)
	region := a.RandInt(seed, 1, 99)
	year := a.RandInt(seed+1, 0, 99)
	number := a.RandInt(seed+2, 100000, 999999)

	return fmt.Sprintf("%02d%02d %06d", region, year, number)
}

// GeneratePhone генерирует синтетический номер мобильного телефона в формате РФ (+7 9xx xxx-xx-xx).
func (a *Anonymizer) GeneratePhone(original string) string {
	seed := a.DeriveSeed(original)
	defCode := a.RandInt(seed, 0, 99)
	part1 := a.RandInt(seed+1, 100, 999)
	part2 := a.RandInt(seed+2, 0, 99)
	part3 := a.RandInt(seed+3, 0, 99)

	return fmt.Sprintf("+7 9%02d %03d-%02d-%02d", defCode, part1, part2, part3)
}

// PerturbDate выполняет детерминированное смещение даты в диапазоне ±daysRange дней,
// сохраняя исходный строковый формат даты и реалистичный демографический диапазон.
func (a *Anonymizer) PerturbDate(originalDate string, daysRange int) string {
	trimmed := strings.TrimSpace(originalDate)
	if trimmed == "" || daysRange <= 0 {
		return originalDate
	}

	var parsedTime time.Time
	var matchedLayout string
	for _, layout := range recognizedDateLayouts {
		if t, err := time.Parse(layout, trimmed); err == nil {
			parsedTime = t
			matchedLayout = layout
			break
		}
	}

	if matchedLayout == "" {
		return originalDate
	}

	seed := a.DeriveSeed(originalDate)
	shift := a.RandInt(seed, -daysRange, daysRange)
	if shift == 0 {
		if seed%2 == 0 {
			shift = 1
		} else {
			shift = -1
		}
	}

	perturbed := parsedTime.AddDate(0, 0, shift)
	return perturbed.Format(matchedLayout)
}
