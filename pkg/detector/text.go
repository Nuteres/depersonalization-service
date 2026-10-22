package detector

import (
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// TextMatch represents an identified sensitive personal data entity in unstructured text.
type TextMatch struct {
	Start      int        // 0-based byte offset in string
	End        int        // 0-based byte offset in string
	Value      string     // Matched and trimmed string value
	EntityType EntityType // Entity category
}

// Canonical regular expressions for Russian PII in unstructured text.
var (
	ReSNILSFormatted    = regexp.MustCompile(`(?:^|[^\p{L}\p{N}_])(\d{3}[-\s]\d{3}[-\s]\d{3}[-\s]\d{2})`)
	ReSNILS11Digits     = regexp.MustCompile(`(?:^|[^\p{L}\p{N}_])(\d{11})`)
	ReSNILSPrefixed     = regexp.MustCompile(`(?i)(?:СНИЛС|SNILS)[:\s№]*(\d{3}[-\s]\d{3}[-\s]\d{3}[-\s]\d{2}|\d{11})`)

	RePassport4_6       = regexp.MustCompile(`(?:^|[^\p{L}\p{N}_])(\d{4}[\s\-]+(?:№|N|no\.)?\s*\d{6})`)
	RePassport2_2_6     = regexp.MustCompile(`(?:^|[^\p{L}\p{N}_])(\d{2}\s+\d{2}\s*(?:№|N|no\.)?\s*\d{6})`)
	RePassportPrefixed  = regexp.MustCompile(`(?i)(?:паспорт(?:а| гражданина рф)?(?:\s*(?:серия|номер|№|N|серии|номеру|серия\s+и\s+номер))*|серия\s+и\s+номер\s+паспорта)[:\s]*([0-9\s№Nno\.\-]{9,24})`)
	RePassportDeptCode  = regexp.MustCompile(`(?i)(?:код\s+подразделения|код\s+подр\.?|код\s+подр)[:\s№]*(\d{3}[\s\-]\d{3})`)

	ReINN12             = regexp.MustCompile(`(?:^|[^\p{L}\p{N}_])(\d{12})`)
	ReINNPrefixed       = regexp.MustCompile(`(?i)(?:ИНН|INN)[:\s№]*\b(\d{10,12})\b`)

	RePhone             = regexp.MustCompile(`(?:^|[^\p{L}\p{N}_])((?:\+7|8)[\s\-\(]*\d{3}[\s\-\)]*\d{3}[\s\-]*\d{2}[\s\-]*\d{2})`)
	RePhonePrefixed     = regexp.MustCompile(`(?i)(?:тел(?:ефон)?|моб\.?|сот\.?|доп\.?\s*тел\.?)[:\s]*((?:\+7|8)?[0-9\s\-\(\)]{10,18})`)

	ReCard16            = regexp.MustCompile(`(?:^|[^\p{L}\p{N}_])((?:\d{4}[\s\-]?){4})`)
	ReBankAccount       = regexp.MustCompile(`(?i)(?:р/с|р/сч|расчетный\s+счет|расчётный\s+счёт|номер\s+счета|лицевой\s+счет|л/с|к/с|к/сч|корр(?:еспондентский)?\.?\s+счет|счет|счёт)[:\s№]*([0-9]{20})`)
	ReAccountStandalone = regexp.MustCompile(`(?:^|[^\p{L}\p{N}_])((?:40[5-8]|30[1-2]|42[3-6])\d{17})`)

	ReEmail             = regexp.MustCompile(`(?:^|[^\p{L}\p{N}_])([a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,})`)

	ReFIOPatronymic     = regexp.MustCompile(`(?:^|[^\p{L}\p{N}_])([А-ЯЁ][а-яё]+(?:-[А-ЯЁ][а-яё]+)?\s+[А-ЯЁ][а-яё]+\s+[А-ЯЁ][а-яё]+(?:овича|евича|ича|овны|евны|ичны|овичу|евичу|ичу|овне|евне|ичне|овичем|евичем|ичем|овной|евной|ичной|овиче|евиче|иче|ович|евич|ич|овна|евна|ична|оглы|кызы|уулу|инична))`)
	ReFIOPrefixed       = regexp.MustCompile(`(?i:(?:сотрудник(?:а|у|ом|е)?|клиент(?:а|у|ом|е)?|пациент(?:а|у|ом|е)?|фио|работник(?:а|у|ом|е)?|физлиц(?:о|а|у|ом|е)|подписал|сдал|принял|ип|арендодател(?:ь|я|ю|ем|е)|арендатор(?:а|у|ом|е)?|(?:гражданин(?:а|у|ом|е)?|гражданк(?:а|и|е|у|ой))(?:\s+(?:российской\s+федерации|рф))?|предпринимател(?:ь|я|ю|ем|е)|директор(?:а|у|ом|е)?|гендиректор(?:а|у|ом|е)?|руководител(?:ь|я|ю|ем|е)|представител(?:ь|я|ю|ем|е))[:\s]+)([А-ЯЁ][а-яё]+(?:\s+[А-ЯЁ][а-яё]+){1,2})`)
	ReFIOInitials1      = regexp.MustCompile(`(?:^|[^\p{L}\p{N}_])([А-ЯЁ][а-яё]+(?:\s+[А-ЯЁ]\.\s*[А-ЯЁ]\.|\s+[А-ЯЁ]\.))`)
	ReFIOInitials2      = regexp.MustCompile(`(?:^|[^\p{L}\p{N}_])((?:[А-ЯЁ]\.\s*[А-ЯЁ]\.|\s*[А-ЯЁ]\.)\s+[А-ЯЁ][а-яё]+)`)

	ReDateDMY           = regexp.MustCompile(`(?:^|[^\p{L}\p{N}_])(\d{2}\.\d{2}\.\d{4})`)
	ReDateYMD           = regexp.MustCompile(`(?:^|[^\p{L}\p{N}_])(\d{4}-\d{2}-\d{2})`)

	ReAddressPrefixed   = regexp.MustCompile(`(?i:(?:зарегистрирован(?:ный|ная|о|ы)?(?:\s+по\s+адресу)?|проживающ(?:ий|ая|ее|ие)?(?:\s+по\s+адресу)?|адрес\s+(?:регистрации|проживания|прописки|местонахождения|объекта|помещения|рег\.?|прож\.?)|фактический\s+адрес|юридический\s+адрес|место\s+(?:жительства|нахождения|регистрации)|по\s+адресу|адрес)[:\s]+)([^\n\r\t;]{12,180})`)
	ReAddressPostal     = regexp.MustCompile(`(?:^|[^\p{L}\p{N}_])(\d{6},\s*(?:г\.|гор\.|город|обл\.|область|край|респ\.|республика)[^\n\r\t;]{10,180})`)
)

// IsBoundaryAfter checks whether the character immediately following end is a non-word boundary.
func IsBoundaryAfter(text string, end int) bool {
	if end >= len(text) {
		return true
	}
	rNext, _ := utf8.DecodeRuneInString(text[end:])
	return !unicode.IsLetter(rNext) && !unicode.IsDigit(rNext) && rNext != '_'
}

// TrimAddress strips common non-address suffixes and punctuation from an extracted address candidate.
func TrimAddress(s string) string {
	s = strings.TrimSpace(s)
	stopMarkers := []string{
		", ИНН", ", инн", ", ОГРН", ", огрн", ", СНИЛС", ", снилс",
		", тел", ", Тел", ", email", ", Email", ", E-mail", ", e-mail",
		", паспорт", ", Паспорт", ", дата", ", Дата", ", код", ", Код",
		", именуем", ", Именуем", ", р/с", ", Р/с", ", счет", ", Счет",
		", проживающ", ", Проживающ", ", зарегистрирован", ", Зарегистрирован",
		", адрес", ", Адрес", ", место", ", Место",
		". Дата", ". дата", ". Срок", ". срок", ". Оплата", ". оплата",
		". Начало", ". начало", ". Должность", ". должность",
		". Телефон", ". телефон", ". Тел", ". тел", ". Email", ". email",
		". ИНН", ". инн", ". СНИЛС", ". снилс", ". Паспорт", ". паспорт",
		". Режим", ". режим", ". Работник", ". работник",
		". Работодатель", ". работодатель", ". Условия", ". условия",
		"; ИНН", "; СНИЛС", "; Тел", "; Email", "; Паспорт",
	}
	for _, marker := range stopMarkers {
		if idx := strings.Index(s, marker); idx != -1 {
			s = strings.TrimSpace(s[:idx])
		}
	}

	sentenceBreakRe := regexp.MustCompile(`\.\s+([А-ЯЁ][а-яё]+)`)
	if loc := sentenceBreakRe.FindStringSubmatchIndex(s); len(loc) >= 4 {
		word := s[loc[2]:loc[3]]
		wordUpper := strings.ToUpper(word)
		if !nonFIOKeywords[wordUpper] && !strings.HasPrefix(wordUpper, "УЛ") && !strings.HasPrefix(wordUpper, "ПРОСП") {
			s = strings.TrimSpace(s[:loc[0]])
		}
	}

	return strings.TrimRight(s, " ,.-—\t")
}

// IsAddressValid verifies whether a candidate address contains typical street or housing keywords.
func IsAddressValid(s string) bool {
	s = TrimAddress(s)
	if len(s) < 10 {
		return false
	}
	lower := strings.ToLower(s)
	markers := []string{"ул.", "улица", "пр-кт", "проспект", "пер.", "переулок", "д.", "дом", "кв.", "квартира", "корп.", "стр.", "шоссе", "б-р", "офис", "наб.", "набережная", "помещение"}
	for _, m := range markers {
		if strings.Contains(lower, m) {
			return true
		}
	}
	return false
}

var addressAbbreviations = map[string]bool{
	"Г": true, "С": true, "Д": true, "П": true, "Р": true, "В": true, "Т": true, "Н": true,
	"Б": true, "М": true, "УЛ": true, "КВ": true, "СТ": true, "ИМ": true, "ОБЛ": true,
	"ПЕР": true, "ПР": true, "ПЛ": true, "НАБ": true, "Ш": true, "СТР": true, "ЭТ": true,
	"ОФ": true, "РИС": true, "ТАБЛ": true, "СМ": true, "СР": true, "РУБ": true, "КОП": true,
	"ТЫС": true, "МЛН": true, "МЛРД": true, "ТЕЛ": true, "ФАКС": true, "ЭКЗ": true, "ПРИЛ": true,
}

var nonFIOKeywords = map[string]bool{
	"МОСКВА": true, "САНКТ-ПЕТЕРБУРГ": true, "ПЕТЕРБУРГ": true, "ПИТЕР": true, "НОВОСИБИРСК": true,
	"ЕКАТЕРИНБУРГ": true, "КАЗАНЬ": true, "НИЖНИЙ НОВГОРОД": true, "ЧЕЛЯБИНСК": true, "САМАРА": true,
	"ОМСК": true, "РОСТОВ-НА-ДОНУ": true, "РОСТОВ": true, "УФА": true, "КРАСНОЯРСК": true,
	"ВОРОНЕЖ": true, "ПЕРМЬ": true, "ВОЛГОГРАД": true, "КРАСНОДАР": true, "САРАТОВ": true,
	"ТЮМЕНЬ": true, "ТОЛЬЯТТИ": true, "ИЖЕВСК": true, "БАРНАУЛ": true, "УЛЬЯНОВСК": true,
	"ИРКУТСК": true, "ХАБАРОВСК": true, "ЯРОСЛАВЛЬ": true, "ВЛАДИВОСТОК": true, "МАХАЧКАЛА": true,
	"ТОМСК": true, "ОРЕНБУРГ": true, "КЕМЕРОВО": true, "НОВОКУЗНЕЦК": true, "РЯЗАНЬ": true,
	"АСТРАХАНЬ": true, "ПЕНЗА": true, "ЛИПЕЦК": true, "КИРОВ": true, "ЧЕБОКСАРЫ": true,
	"ТУЛА": true, "КАЛИНИНГРАД": true, "БАЛАШИХА": true, "КУРСК": true, "СЕВАСТОПОЛЬ": true,
	"УЛАН-УДЭ": true, "СОЧИ": true, "СТАВРОПОЛЬ": true, "ТВЕРЬ": true, "МАГНИТОГОРСК": true,
	"ИВАНОВО": true, "БРЯНСК": true, "СУРГУТ": true, "ВЛАДИМИР": true, "ЧИТА": true,
	"АРХАНГЕЛЬСК": true, "КАЛУГА": true, "СМОЛЕНСК": true, "ВОЛЖСКИЙ": true, "ЯКУТСК": true,
	"САРАНСК": true, "ЧЕРЕПОВЕЦ": true, "КУРГАН": true, "ВОЛОГДА": true, "ОРЁЛ": true, "ОРЕЛ": true,
	"ПОДОЛЬСК": true, "ГРОЗНЫЙ": true, "ВЛАДИКАВКАЗ": true, "ТАМБОВ": true, "МУРМАНСК": true,
	"ПЕТРОЗАВОДСК": true, "НИЖНЕВАРТОВСК": true, "КОСТРОМА": true, "СТЕРЛИТАМАК": true, "НОВОРОССИЙСК": true,
	"ЙОШКАР-ОЛА": true, "ХИМКИ": true, "ТАГАНРОГ": true, "КОМСОМОЛЬСК-НА-АМУРЕ": true, "СЫКТЫВКАР": true,
	"НАЛЬЧИК": true, "НИЖНЕКАМСК": true, "ШАХТЫ": true, "ДЗЕРЖИНСК": true, "БРАТСК": true,
	"ОРСК": true, "АНГАРСК": true, "ЭНГЕЛЬС": true, "БЛАГОВЕЩЕНСК": true, "СТАРЫЙ ОСКОЛ": true,
	"ВЕЛИКИЙ НОВГОРОД": true, "КОРОЛЁВ": true, "КОРОЛЕВ": true, "ПСКОВ": true, "МЫТИЩИ": true,
	"БИЙСК": true, "ЛЮБЕРЦЫ": true, "ПРОКОПЬЕВСК": true, "ЮЖНО-САХАЛИНСК": true, "АРМАВИР": true,
	"РЫБИНСК": true, "БАЛАКОВО": true, "АБАКАН": true, "СЕВЕРОДВИНСК": true, "ПЕТРОПАВЛОВСК-КАМЧАТСКИЙ": true,
	"НОРИЛЬСК": true, "УССУРИЙСК": true, "ВОЛГОДОНСК": true, "СЫЗРАНЬ": true, "КАМЕНСК-УРАЛЬСКИЙ": true,
	"ЗЛАТОУСТ": true, "НОВОЧЕРКАССК": true, "КРАСНОГОРСК": true, "ЭЛЕКТРОСТАЛЬ": true, "АЛЬМЕТЬЕВСК": true,
	"САЛАВАТ": true, "МИАСС": true, "КЕРЧЬ": true, "НАХОДКА": true, "КОПЕЙСК": true,
	"ПЯТИГОРСК": true, "РУБЦОВСК": true, "БЕРЕЗНИКИ": true, "КОЛОМНА": true, "МАЙКОП": true,
	"ОДИНЦОВО": true, "ХАСАВЮРТ": true, "КОВРОВ": true, "КИСЛОВОДСК": true, "НЕФТЕКАМСК": true,
	"НЕФТЕЮГАНСК": true, "НОВОЧЕБОКСАРСК": true, "СЕРПУХОВ": true, "ЩЁЛКОВО": true, "ЩЕЛКОВО": true,
	"НОВОМОСКОВСК": true, "БАТАЙСК": true, "ПЕРВОУРАЛЬСК": true, "ДОМОДЕДОВО": true, "ДЕРБЕНТ": true,
	"ЧЕРКЕССК": true, "ОРЕХОВО-ЗУЕВО": true, "НЕВИННОМЫССК": true, "ДИМИТРОВГРАД": true, "НАЗРАНЬ": true,
	"КЫЗЫЛ": true, "ОКТЯБРЬСКИЙ": true, "ОБНИНСК": true, "КАСПИЙСК": true, "НОВЫЙ УРЕНГОЙ": true,
	"РАМЕНСКОЕ": true, "МУРОМ": true, "НОВОШАХТИНСК": true, "ЖУКОВСКИЙ": true, "СЕВЕРСК": true,
	"ЕССЕНТУКИ": true, "НОЯБРЬСК": true, "АРТЁМ": true, "АРТЕМ": true, "ЕВПАТОРИЯ": true,
	"АЧИНСК": true, "ЕЛЕЦ": true, "СЕРГИЕВ ПОСАД": true, "АРЗАМАС": true, "ДОЛГОПРУДНЫЙ": true,
	"ЭЛИСТА": true, "БЕРДСК": true, "НОВОКУЙБЫШЕВСК": true, "ПУШКИНО": true, "ЖЕЛЕЗНОГОРСК": true,
	"НОГИНСК": true, "ЛЕНИНГРАДСКАЯ": true, "МОСКОВСКАЯ": true, "РОССИЯ": true, "ФЕДЕРАЦИЯ": true,
	"ТАБЛИЦА": true, "ПРИЛОЖЕНИЕ": true, "ПРИКАЗ": true, "ДОГОВОР": true, "АКТ": true,
	"СЧЕТ": true, "СЧЁТ": true, "УСЛУГА": true, "УСЛУГИ": true, "ТАРИФ": true, "ТАРИФЫ": true,
	"УТВЕРЖДЕНО": true, "БЛОКИРОВКА": true, "ПЕРИОД": true, "СТОИМОСТЬ": true, "ТЕРРИТОРИЯ": true,
	"НАИМЕНОВАНИЕ": true, "МОРАТОРИЙ": true, "АБОНЕНТ": true, "ОБЛАСТЬ": true, "РАЙОН": true,
	"КРАЙ": true, "ГОРОД": true, "СЕЛО": true, "ДЕРЕВНЯ": true, "ПОСЕЛОК": true, "ПОСЁЛОК": true,
	"УЛИЦА": true, "ПРОСПЕКТ": true, "ПЕРЕУЛОК": true, "ПЛОЩАДЬ": true, "ШОССЕ": true, "СТРОЕНИЕ": true,
}

var fioStopWords = map[string]bool{
	"дает": true, "обязуется": true, "обязан": true, "вправе": true, "имеет": true,
	"согласен": true, "поручает": true, "принимает": true, "согласие": true,
	"обработку": true, "компании": true, "договора": true, "настоящий": true,
	"трудовой": true, "договор": true, "общество": true, "ответственностью": true,
	"стороны": true, "сторона": true, "работодатель": true, "работник": true,
	"предмет": true, "оплата": true, "режим": true, "срок": true, "действие": true,
	"защита": true, "персональных": true, "данных": true, "конфиденциальность": true,
	"адреса": true, "реквизиты": true, "подписи": true, "отметка": true, "получении": true,
	"экземпляра": true, "генеральный": true, "директор": true, "руководитель": true,
	"ведущего": true, "разработчика": true, "программного": true, "обеспечения": true,
	"условия": true, "рабочего": true, "времени": true, "отдыха": true, "гарантии": true,
}

// IsFIOValid verifies whether full FIO words are valid capitalized Cyrillic names without common document stop words.
func IsFIOValid(s string) bool {
	parts := strings.Fields(s)
	if len(parts) < 2 || len(parts) > 3 {
		return false
	}
	for _, p := range parts {
		if fioStopWords[strings.ToLower(p)] {
			return false
		}
		subparts := strings.Split(p, "-")
		for _, sp := range subparts {
			runes := []rune(sp)
			if len(runes) < 2 {
				return false
			}
			if !unicode.IsUpper(runes[0]) {
				return false
			}
			for _, r := range runes[1:] {
				if !unicode.IsLower(r) {
					return false
				}
			}
			if fioStopWords[strings.ToLower(sp)] {
				return false
			}
		}
	}
	return true
}

// IsFIOInitialsValid verifies surname and initials formatting while filtering abbreviations and city names.
func IsFIOInitialsValid(match string) bool {
	clean := strings.TrimSpace(match)
	parts := strings.Fields(clean)
	if len(parts) < 2 {
		return false
	}

	var surnamePart string
	var initialPart string

	if strings.Contains(parts[0], ".") {
		initialPart = parts[0]
		if len(parts) > 2 && strings.Contains(parts[1], ".") {
			initialPart += parts[1]
			surnamePart = parts[2]
		} else {
			surnamePart = parts[1]
		}
	} else {
		surnamePart = parts[0]
		initialPart = strings.Join(parts[1:], "")
	}

	surnameUpper := strings.ToUpper(strings.Trim(surnamePart, ".,:;!?\"'()[]{} \t"))
	if nonFIOKeywords[surnameUpper] || addressAbbreviations[surnameUpper] {
		return false
	}

	dotCount := strings.Count(initialPart, ".")
	if dotCount == 1 {
		initialLetters := strings.ToUpper(strings.Trim(initialPart, ".,:;!?\"'()[]{} \t"))
		if addressAbbreviations[initialLetters] {
			return false
		}
		runes := []rune(surnameUpper)
		if len(runes) < 3 {
			return false
		}
		hasSurnameSuffix := false
		for _, suffix := range []string{"ОВ", "ОВА", "ЕВ", "ЕВА", "ИН", "ИНА", "ЫН", "ЫНА", "СКИЙ", "СКАЯ", "ЦКИЙ", "ЦКАЯ", "НЫХ", "НИХ", "ИХ", "ИЧ", "ЮК", "УК", "АК", "ЯК", "КО"} {
			if strings.HasSuffix(surnameUpper, suffix) {
				hasSurnameSuffix = true
				break
			}
		}
		if !hasSurnameSuffix {
			return false
		}
	}

	return true
}

// FindTextMatches searches text for all recognized Russian personal data entities,
// sorting matches and filtering overlapping intervals.
func FindTextMatches(text string) []TextMatch {
	return FindTextMatchesWithOptions(text, nil)
}

// FindTextMatchesWithOptions searches text with support for disabling specific EntityType categories.
func FindTextMatchesWithOptions(text string, disabled map[string]bool) []TextMatch {
	var matches []TextMatch

	addRegexMatches := func(re *regexp.Regexp, submatchIdx int, entityType EntityType, validateFunc func(string) bool) {
		if disabled != nil && disabled[string(entityType)] {
			return
		}
		indexes := re.FindAllStringSubmatchIndex(text, -1)
		for _, loc := range indexes {
			var start, end int
			if submatchIdx*2+1 < len(loc) && loc[submatchIdx*2] >= 0 {
				start = loc[submatchIdx*2]
				end = loc[submatchIdx*2+1]
			} else {
				start = loc[0]
				end = loc[1]
			}

			val := text[start:end]
			trimmed := strings.TrimSpace(val)
			if trimmed == "" {
				continue
			}
			leadTrim := strings.Index(val, trimmed)
			start += leadTrim
			end = start + len(trimmed)
			val = trimmed

			if entityType == EntityAddress {
				val = TrimAddress(val)
				end = start + len(val)
			}

			if !IsBoundaryAfter(text, end) {
				continue
			}
			if validateFunc == nil || validateFunc(val) {
				matches = append(matches, TextMatch{
					Start:      start,
					End:        end,
					Value:      val,
					EntityType: entityType,
				})
			}
		}
	}

	// 1. Bank card & 20-digit Account
	addRegexMatches(ReCard16, 1, EntityCard, func(s string) bool {
		return ValidateCardNumber(s)
	})
	addRegexMatches(ReBankAccount, 1, EntityCard, func(s string) bool {
		return len(CleanDigits(s)) == 20
	})
	addRegexMatches(ReAccountStandalone, 1, EntityCard, func(s string) bool {
		return len(CleanDigits(s)) == 20
	})

	// 2. SNILS
	addRegexMatches(ReSNILSFormatted, 1, EntitySNILS, func(s string) bool {
		return len(CleanDigits(s)) == 11
	})
	addRegexMatches(ReSNILS11Digits, 1, EntitySNILS, func(s string) bool {
		return ValidateSNILS(s)
	})
	addRegexMatches(ReSNILSPrefixed, 1, EntitySNILS, func(s string) bool {
		return len(CleanDigits(s)) == 11
	})

	// 3. Email
	addRegexMatches(ReEmail, 1, EntityEmail, nil)

	// 4. Phone
	addRegexMatches(RePhone, 1, EntityPhone, func(s string) bool {
		d := CleanDigits(s)
		return len(d) >= 10 && len(d) <= 11
	})
	addRegexMatches(RePhonePrefixed, 1, EntityPhone, func(s string) bool {
		d := CleanDigits(s)
		return len(d) >= 10 && len(d) <= 11
	})

	// 5. INN
	addRegexMatches(ReINN12, 1, EntityINN, func(s string) bool {
		return ValidateINN12(s)
	})
	addRegexMatches(ReINNPrefixed, 1, EntityINN, func(s string) bool {
		d := CleanDigits(s)
		return len(d) == 10 || len(d) == 12
	})

	// 6. Passport & Dept Code
	addRegexMatches(RePassport4_6, 1, EntityPassport, func(s string) bool {
		return len(CleanDigits(s)) == 10
	})
	addRegexMatches(RePassport2_2_6, 1, EntityPassport, func(s string) bool {
		return len(CleanDigits(s)) == 10
	})
	addRegexMatches(RePassportPrefixed, 1, EntityPassport, func(s string) bool {
		return len(CleanDigits(s)) == 10
	})
	addRegexMatches(RePassportDeptCode, 1, EntityPassport, func(s string) bool {
		return len(CleanDigits(s)) == 6
	})

	// 7. Address
	addRegexMatches(ReAddressPostal, 1, EntityAddress, IsAddressValid)
	addRegexMatches(ReAddressPrefixed, 1, EntityAddress, IsAddressValid)

	// 8. FIO (Full names, prefixes, initials)
	addRegexMatches(ReFIOPatronymic, 1, EntityFIO, IsFIOValid)
	addRegexMatches(ReFIOPrefixed, 1, EntityFIO, func(s string) bool {
		parts := strings.Fields(s)
		return len(parts) >= 2 && IsFIOValid(s)
	})
	addRegexMatches(ReFIOInitials1, 1, EntityFIO, IsFIOInitialsValid)
	addRegexMatches(ReFIOInitials2, 1, EntityFIO, IsFIOInitialsValid)

	// 9. Dates
	addRegexMatches(ReDateDMY, 1, EntityDate, func(s string) bool {
		_, err := time.Parse("02.01.2006", s)
		return err == nil
	})
	addRegexMatches(ReDateYMD, 1, EntityDate, func(s string) bool {
		_, err := time.Parse("2006-01-02", s)
		return err == nil
	})

	if len(matches) <= 1 {
		return matches
	}

	// Filter overlapping matches: prefer earlier start, then longer match
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].Start == matches[j].Start {
			return matches[i].End > matches[j].End
		}
		return matches[i].Start < matches[j].Start
	})

	var deduped []TextMatch
	lastEnd := -1
	for _, m := range matches {
		if m.Start >= lastEnd {
			deduped = append(deduped, m)
			lastEnd = m.End
		}
	}

	return deduped
}
