package formats

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"dep_service/internal/domain"
	"dep_service/pkg/anonymizer"
	"dep_service/pkg/depmap"
	"dep_service/pkg/detector"
)

// EntityCount tracks the number of detected entities of a specific type.
type EntityCount struct {
	Type  string `json:"type"`
	Count int    `json:"count"`
}

// DocxTextSnippet represents a single detected entity snippet.
type DocxTextSnippet = domain.DocxTextSnippet

// TextParagraphSegment represents a piece of text inside a paragraph, indicating if it was anonymized.
type TextParagraphSegment = domain.TextParagraphSegment

// TextTableCellPreview represents a cell inside a table row for Word-like preview.
type TextTableCellPreview = domain.TextTableCellPreview

// TextTableRowPreview represents a table row for Word-like preview.
type TextTableRowPreview = domain.TextTableRowPreview

// TextTablePreview represents a structured table for Word-like preview.
type TextTablePreview = domain.TextTablePreview

// TextDocumentBlock represents either a top-level paragraph or a table in a document.
type TextDocumentBlock = domain.TextDocumentBlock

// TextParagraphPreview holds the segmented text of a paragraph for inline Word-like document preview.
type TextParagraphPreview = domain.TextParagraphPreview

// DOCXHandler handles inspection and anonymization of Microsoft Word (.docx) documents.
type DOCXHandler struct {
	Anon *anonymizer.Anonymizer
}

// NewDOCXHandler creates a new DOCXHandler with the given Anonymizer.
func NewDOCXHandler(anon *anonymizer.Anonymizer) *DOCXHandler {
	return &DOCXHandler{Anon: anon}
}

// SupportedExtensions returns file extensions supported by this handler.
func (h *DOCXHandler) SupportedExtensions() []string {
	return []string{".docx"}
}

// Regular expressions for detecting sensitive Russian personal data in Word documents.
var (
	docxSNILSFormatted   = regexp.MustCompile(`(?:^|[^\p{L}\p{N}_])(\d{3}[-\s]\d{3}[-\s]\d{3}[-\s]\d{2})`)
	docxSNILS11Digits    = regexp.MustCompile(`(?:^|[^\p{L}\p{N}_])(\d{11})`)
	docxSNILSPrefixed    = regexp.MustCompile(`(?i)(?:СНИЛС|SNILS)[:\s№]*(\d{3}[-\s]\d{3}[-\s]\d{3}[-\s]\d{2}|\d{11})`)

	docxPassport4_6      = regexp.MustCompile(`(?:^|[^\p{L}\p{N}_])(\d{4}[\s\-]+(?:№|N|no\.)?\s*\d{6})`)
	docxPassport2_2_6    = regexp.MustCompile(`(?:^|[^\p{L}\p{N}_])(\d{2}\s+\d{2}\s*(?:№|N|no\.)?\s*\d{6})`)
	docxPassportPrefixed = regexp.MustCompile(`(?i)(?:паспорт(?:а| гражданина рф)?(?:\s*(?:серия|номер|№|N|серии|номеру|серия\s+и\s+номер))*|серия\s+и\s+номер\s+паспорта)[:\s]*([0-9\s№Nno\.\-]{9,24})`)
	docxPassportDeptCode = regexp.MustCompile(`(?i)(?:код\s+подразделения|код\s+подр\.?|код\s+подр)[:\s№]*(\d{3}[\s\-]\d{3})`)

	docxINN12            = regexp.MustCompile(`(?:^|[^\p{L}\p{N}_])(\d{12})`)
	docxINNPrefixed      = regexp.MustCompile(`(?i)(?:ИНН|INN)[:\s№]*\b(\d{10,12})\b`)

	docxPhone            = regexp.MustCompile(`(?:^|[^\p{L}\p{N}_])((?:\+7|8)[\s\-\(]*\d{3}[\s\-\)]*\d{3}[\s\-]*\d{2}[\s\-]*\d{2})`)
	docxPhonePrefixed    = regexp.MustCompile(`(?i)(?:тел(?:ефон)?|моб\.?|сот\.?|доп\.?\s*тел\.?)[:\s]*((?:\+7|8)?[0-9\s\-\(\)]{10,18})`)

	docxCard16           = regexp.MustCompile(`(?:^|[^\p{L}\p{N}_])((?:\d{4}[\s\-]?){4})`)
	docxBankAccount      = regexp.MustCompile(`(?i)(?:р/с|р/сч|расчетный\s+счет|расчётный\s+счёт|номер\s+счета|лицевой\s+счет|л/с|к/с|к/сч|корр(?:еспондентский)?\.?\s+счет|счет|счёт)[:\s№]*([0-9]{20})`)
	docxAccountStandalone= regexp.MustCompile(`(?:^|[^\p{L}\p{N}_])((?:40[5-8]|30[1-2]|42[3-6])\d{17})`)

	docxEmail            = regexp.MustCompile(`(?:^|[^\p{L}\p{N}_])([a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,})`)

	docxFIOPatronymic    = regexp.MustCompile(`(?:^|[^\p{L}\p{N}_])([А-ЯЁ][а-яё]+(?:-[А-ЯЁ][а-яё]+)?\s+[А-ЯЁ][а-яё]+\s+[А-ЯЁ][а-яё]+(?:ович|евич|ич|овна|евна|ична|оглы|кызы|уулу|инична))`)
	docxFIOPrefixed      = regexp.MustCompile(`(?i:(?:сотрудник|клиент|пациент|фио|работник|физлицо|подписал|сдал|принял|ип|арендодатель|арендатор|(?:гражданин|гражданка)(?:\s+(?:российской\s+федерации|рф))?|предприниматель|директор|гендиректор|руководитель|представитель)[:\s]+)([А-ЯЁ][а-яё]+(?:\s+[А-ЯЁ][а-яё]+){1,2})`)
	docxFIOInitials1     = regexp.MustCompile(`(?:^|[^\p{L}\p{N}_])([А-ЯЁ][а-яё]+(?:\s+[А-ЯЁ]\.\s*[А-ЯЁ]\.|\s+[А-ЯЁ]\.))`)
	docxFIOInitials2     = regexp.MustCompile(`(?:^|[^\p{L}\p{N}_])((?:[А-ЯЁ]\.\s*[А-ЯЁ]\.|\s*[А-ЯЁ]\.)\s+[А-ЯЁ][а-яё]+)`)

	docxDateDMY          = regexp.MustCompile(`(?:^|[^\p{L}\p{N}_])(\d{2}\.\d{2}\.\d{4})`)
	docxDateYMD          = regexp.MustCompile(`(?:^|[^\p{L}\p{N}_])(\d{4}-\d{2}-\d{2})`)

	docxAddressPrefixed  = regexp.MustCompile(`(?i:(?:зарегистрирован(?:ный|ная|о|ы)?(?:\s+по\s+адресу)?|проживающ(?:ий|ая|ее|ие)?(?:\s+по\s+адресу)?|адрес\s+(?:регистрации|проживания|прописки|местонахождения|объекта|помещения|рег\.?|прож\.?)|фактический\s+адрес|юридический\s+адрес|место\s+(?:жительства|нахождения|регистрации)|по\s+адресу|адрес)[:\s]+)([^\n\r\t;]{12,180})`)
	docxAddressPostal    = regexp.MustCompile(`(?:^|[^\p{L}\p{N}_])(\d{6},\s*(?:г\.|гор\.|город|обл\.|область|край|респ\.|республика)[^\n\r\t;]{10,180})`)
)

type docxMatch struct {
	start      int // byte offset in string
	end        int // byte offset in string
	value      string
	entityType string
}

func isDocxBoundaryAfter(text string, end int) bool {
	if end >= len(text) {
		return true
	}
	rNext, _ := utf8.DecodeRuneInString(text[end:])
	return !unicode.IsLetter(rNext) && !unicode.IsDigit(rNext) && rNext != '_'
}

func cleanDigits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func trimDocxAddress(s string) string {
	s = strings.TrimSpace(s)
	stopMarkers := []string{
		", ИНН", ", инн", ", ОГРН", ", огрн", ", СНИЛС", ", снилс",
		", тел", ", Тел", ", email", ", Email", ", E-mail", ", e-mail",
		", паспорт", ", Паспорт", ", дата", ", Дата", ", код", ", Код",
		", именуем", ", Именуем", ", р/с", ", Р/с", ", счет", ", Счет",
		", проживающ", ", Проживающ", ", зарегистрирован", ", Зарегистрирован",
		", адрес", ", Адрес", ", место", ", Место",
	}
	for _, marker := range stopMarkers {
		if idx := strings.Index(s, marker); idx != -1 {
			s = strings.TrimSpace(s[:idx])
		}
	}
	return strings.TrimRight(s, " ,.-—\t")
}

func isDocxAddressValid(s string) bool {
	s = trimDocxAddress(s)
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

var docxAddressAbbreviations = map[string]bool{
	"Г": true, "С": true, "Д": true, "П": true, "Р": true, "В": true, "Т": true, "Н": true,
	"Б": true, "М": true, "УЛ": true, "КВ": true, "СТ": true, "ИМ": true, "ОБЛ": true,
	"ПЕР": true, "ПР": true, "ПЛ": true, "НАБ": true, "Ш": true, "СТР": true, "ЭТ": true,
	"ОФ": true, "РИС": true, "ТАБЛ": true, "СМ": true, "СР": true, "РУБ": true, "КОП": true,
	"ТЫС": true, "МЛН": true, "МЛРД": true, "ТЕЛ": true, "ФАКС": true, "ЭКЗ": true, "ПРИЛ": true,
}

var docxNonFIOKeywords = map[string]bool{
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

func isDocxFIOInitialsValid(match string) bool {
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
	if docxNonFIOKeywords[surnameUpper] || docxAddressAbbreviations[surnameUpper] {
		return false
	}

	dotCount := strings.Count(initialPart, ".")
	if dotCount == 1 {
		// Single initial like "Г. Москва" or "И. Иванов"
		initialLetters := strings.ToUpper(strings.Trim(initialPart, ".,:;!?\"'()[]{} \t"))
		if docxAddressAbbreviations[initialLetters] {
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

// findDocxSensitiveMatches locates sensitive entities in paragraph text.
func findDocxSensitiveMatches(text string) []docxMatch {
	var matches []docxMatch

	addRegexMatches := func(re *regexp.Regexp, submatchIdx int, entityType string, validateFunc func(string) bool) {
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

			if entityType == string(detector.EntityAddress) {
				val = trimDocxAddress(val)
				end = start + len(val)
			}

			if !isDocxBoundaryAfter(text, end) {
				continue
			}
			if validateFunc == nil || validateFunc(val) {
				matches = append(matches, docxMatch{
					start:      start,
					end:        end,
					value:      val,
					entityType: entityType,
				})
			}
		}
	}

	// 1. Bank card & 20-digit Account (Priority 1)
	addRegexMatches(docxCard16, 1, string(detector.EntityCard), func(s string) bool {
		return len(cleanDigits(s)) == 16
	})
	addRegexMatches(docxBankAccount, 1, string(detector.EntityCard), func(s string) bool {
		return len(cleanDigits(s)) == 20
	})
	addRegexMatches(docxAccountStandalone, 1, string(detector.EntityCard), func(s string) bool {
		return len(cleanDigits(s)) == 20
	})

	// 2. SNILS
	addRegexMatches(docxSNILSFormatted, 1, string(detector.EntitySNILS), func(s string) bool {
		return len(cleanDigits(s)) == 11
	})
	addRegexMatches(docxSNILS11Digits, 1, string(detector.EntitySNILS), func(s string) bool {
		return detector.ValidateSNILS(s)
	})
	addRegexMatches(docxSNILSPrefixed, 1, string(detector.EntitySNILS), func(s string) bool {
		return len(cleanDigits(s)) == 11
	})

	// 3. Email
	addRegexMatches(docxEmail, 1, string(detector.EntityEmail), nil)

	// 4. Phone
	addRegexMatches(docxPhone, 1, string(detector.EntityPhone), func(s string) bool {
		d := cleanDigits(s)
		return len(d) >= 10 && len(d) <= 11
	})
	addRegexMatches(docxPhonePrefixed, 1, string(detector.EntityPhone), func(s string) bool {
		d := cleanDigits(s)
		return len(d) >= 10 && len(d) <= 11
	})

	// 5. INN
	addRegexMatches(docxINN12, 1, string(detector.EntityINN), func(s string) bool {
		return detector.ValidateINN12(s)
	})
	addRegexMatches(docxINNPrefixed, 1, string(detector.EntityINN), func(s string) bool {
		d := cleanDigits(s)
		return len(d) == 10 || len(d) == 12
	})

	// 6. Passport & Dept Code
	addRegexMatches(docxPassport4_6, 1, string(detector.EntityPassport), func(s string) bool {
		return len(cleanDigits(s)) == 10
	})
	addRegexMatches(docxPassport2_2_6, 1, string(detector.EntityPassport), func(s string) bool {
		return len(cleanDigits(s)) == 10
	})
	addRegexMatches(docxPassportPrefixed, 1, string(detector.EntityPassport), func(s string) bool {
		return len(cleanDigits(s)) == 10
	})
	addRegexMatches(docxPassportDeptCode, 1, string(detector.EntityPassport), func(s string) bool {
		return len(cleanDigits(s)) == 6
	})

	// 7. Address
	addRegexMatches(docxAddressPostal, 1, string(detector.EntityAddress), isDocxAddressValid)
	addRegexMatches(docxAddressPrefixed, 1, string(detector.EntityAddress), isDocxAddressValid)

	// 8. FIO (Full names, prefixes, initials)
	addRegexMatches(docxFIOPatronymic, 1, string(detector.EntityFIO), nil)
	addRegexMatches(docxFIOPrefixed, 1, string(detector.EntityFIO), func(s string) bool {
		parts := strings.Fields(s)
		return len(parts) >= 2
	})
	addRegexMatches(docxFIOInitials1, 1, string(detector.EntityFIO), isDocxFIOInitialsValid)
	addRegexMatches(docxFIOInitials2, 1, string(detector.EntityFIO), isDocxFIOInitialsValid)

	// 9. Dates (General document dates)
	addRegexMatches(docxDateDMY, 1, string(detector.EntityDate), func(s string) bool {
		_, err := time.Parse("02.01.2006", s)
		return err == nil
	})
	addRegexMatches(docxDateYMD, 1, string(detector.EntityDate), func(s string) bool {
		_, err := time.Parse("2006-01-02", s)
		return err == nil
	})

	if len(matches) <= 1 {
		return matches
	}

	// Filter overlapping matches: prefer earlier start, then longer match
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].start == matches[j].start {
			return matches[i].end > matches[j].end
		}
		return matches[i].start < matches[j].start
	})

	var deduped []docxMatch
	lastEnd := -1
	for _, m := range matches {
		if m.start >= lastEnd {
			deduped = append(deduped, m)
			lastEnd = m.end
		}
	}

	return deduped
}

func generateSynthetic(anon *anonymizer.Anonymizer, entityType string, original string) string {
	if anon == nil {
		return fmt.Sprintf("[%s]", entityType)
	}
	switch entityType {
	case string(detector.EntityFIO), "NAME", string(detector.EntityLastName), string(detector.EntityFirstName), string(detector.EntityPatronymic):
		return anon.GenerateFIO(original)
	case string(detector.EntityPassport):
		if len(cleanDigits(original)) == 6 && strings.Contains(original, "-") {
			return anon.GeneratePassportDeptCode(original)
		}
		return anon.GeneratePassport(original)
	case string(detector.EntityPhone):
		return anon.GeneratePhone(original)
	case string(detector.EntitySNILS):
		return anon.GenerateSNILS(original)
	case string(detector.EntityINN):
		return anon.GenerateINN12(original)
	case string(detector.EntityCard), "CARD":
		if len(cleanDigits(original)) == 20 {
			return anon.GenerateBankAccount(original)
		}
		return anon.GenerateCardMIR(original)
	case string(detector.EntityEmail):
		return anon.GenerateEmail(original)
	case string(detector.EntityBirthday), "DATE":
		return anon.PerturbDate(original, 30)
	case string(detector.EntityAddress):
		return anon.GenerateAddress(original)
	default:
		return anon.GenerateFIO(original)
	}
}

func isWordElement(name xml.Name, local string) bool {
	if name.Local != local {
		return false
	}
	return name.Space == "" || strings.Contains(name.Space, "wordprocessingml")
}

func getDocxFileOrder(name string) (order int, isBodyOrNotes bool) {
	switch name {
	case "word/document.xml":
		return 1, true
	case "word/footnotes.xml":
		return 2, true
	case "word/endnotes.xml":
		return 3, true
	case "word/comments.xml":
		return 4, false
	default:
		if strings.HasPrefix(name, "word/header") && strings.HasSuffix(name, ".xml") {
			return 5, false
		}
		if strings.HasPrefix(name, "word/footer") && strings.HasSuffix(name, ".xml") {
			return 6, false
		}
		return 100, false
	}
}

func isDocumentXML(name string) bool {
	if name == "word/document.xml" || name == "word/footnotes.xml" || name == "word/endnotes.xml" || name == "word/comments.xml" {
		return true
	}
	if strings.HasPrefix(name, "word/header") && strings.HasSuffix(name, ".xml") {
		return true
	}
	if strings.HasPrefix(name, "word/footer") && strings.HasSuffix(name, ".xml") {
		return true
	}
	return false
}

type docxTextNode struct {
	startOffset int // byte offset of '<' in <w:t ...>
	endOffset   int // byte offset after '>' in </w:t>
	text        string
}

type docxParagraph struct {
	nodes []docxTextNode
}

type docxCell struct {
	paragraphs []docxParagraph
}

type docxRow struct {
	cells []docxCell
}

type docxTable struct {
	rows []docxRow
}

type docxBlockType int

const (
	docxBlockTypeParagraph docxBlockType = iota
	docxBlockTypeTable
)

type docxBlock struct {
	blockType docxBlockType
	paragraph docxParagraph
	table     docxTable
}

func parseDocxXML(xmlData []byte) ([]docxBlock, error) {
	dec := xml.NewDecoder(bytes.NewReader(xmlData))
	var blocks []docxBlock
	var curNodes []docxTextNode
	pDepth := 0
	inT := false
	var curTText strings.Builder
	var tStartOffset int

	tblDepth := 0
	trDepth := 0
	tcDepth := 0

	var curTable docxTable
	var curRow docxRow
	var curCell docxCell

	var curNoteId string
	inSpecialNote := false

	for {
		offsetBefore := int(dec.InputOffset())
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("xml decode error: %w", err)
		}
		offsetAfter := int(dec.InputOffset())

		switch t := tok.(type) {
		case xml.StartElement:
			if isWordElement(t.Name, "endnote") || isWordElement(t.Name, "footnote") {
				var noteId, noteType string
				for _, attr := range t.Attr {
					if attr.Name.Local == "id" {
						noteId = attr.Value
					} else if attr.Name.Local == "type" {
						noteType = attr.Value
					}
				}
				if noteType == "separator" || noteType == "continuationSeparator" || noteId == "-1" || noteId == "0" {
					inSpecialNote = true
				} else {
					inSpecialNote = false
					curNoteId = noteId
				}
			} else if isWordElement(t.Name, "tbl") {
				if tblDepth == 0 {
					curTable = docxTable{}
				}
				tblDepth++
			} else if isWordElement(t.Name, "tr") {
				if tblDepth > 0 && trDepth == 0 {
					curRow = docxRow{}
				}
				trDepth++
			} else if isWordElement(t.Name, "tc") {
				if tblDepth > 0 && trDepth > 0 && tcDepth == 0 {
					curCell = docxCell{}
				}
				tcDepth++
			} else if isWordElement(t.Name, "p") {
				if pDepth == 0 {
					curNodes = nil
				}
				pDepth++
			} else if pDepth > 0 && isWordElement(t.Name, "t") {
				inT = true
				tStartOffset = offsetBefore
				curTText.Reset()
			} else if pDepth > 0 && (isWordElement(t.Name, "br") || isWordElement(t.Name, "cr")) {
				curNodes = append(curNodes, docxTextNode{
					startOffset: offsetBefore,
					endOffset:   offsetAfter,
					text:        "\n",
				})
			} else if pDepth > 0 && isWordElement(t.Name, "tab") {
				curNodes = append(curNodes, docxTextNode{
					startOffset: offsetBefore,
					endOffset:   offsetAfter,
					text:        "\t",
				})
			} else if pDepth > 0 && (isWordElement(t.Name, "endnoteRef") || isWordElement(t.Name, "footnoteRef")) {
				if curNoteId != "" && !inSpecialNote {
					curNodes = append(curNodes, docxTextNode{
						startOffset: offsetBefore,
						endOffset:   offsetAfter,
						text:        curNoteId,
					})
				}
			}
		case xml.CharData:
			if inT {
				curTText.Write(t)
			}
		case xml.EndElement:
			if isWordElement(t.Name, "endnote") || isWordElement(t.Name, "footnote") {
				curNoteId = ""
				inSpecialNote = false
			} else if inT && isWordElement(t.Name, "t") {
				inT = false
				curNodes = append(curNodes, docxTextNode{
					startOffset: tStartOffset,
					endOffset:   offsetAfter,
					text:        curTText.String(),
				})
			} else if isWordElement(t.Name, "p") {
				pDepth--
				if pDepth == 0 {
					if !inSpecialNote {
						p := docxParagraph{nodes: curNodes}
						if tblDepth > 0 && tcDepth > 0 {
							curCell.paragraphs = append(curCell.paragraphs, p)
						} else {
							blocks = append(blocks, docxBlock{
								blockType: docxBlockTypeParagraph,
								paragraph: p,
							})
						}
					}
					curNodes = nil
				}
			} else if isWordElement(t.Name, "tc") {
				tcDepth--
				if tcDepth == 0 && tblDepth > 0 && trDepth > 0 {
					curRow.cells = append(curRow.cells, curCell)
					curCell = docxCell{}
				}
			} else if isWordElement(t.Name, "tr") {
				trDepth--
				if trDepth == 0 && tblDepth > 0 {
					curTable.rows = append(curTable.rows, curRow)
					curRow = docxRow{}
				}
			} else if isWordElement(t.Name, "tbl") {
				tblDepth--
				if tblDepth == 0 {
					blocks = append(blocks, docxBlock{
						blockType: docxBlockTypeTable,
						table:     curTable,
					})
					curTable = docxTable{}
				}
			}
		}
	}

	return blocks, nil
}

func extractParagraphsFromBlocks(blocks []docxBlock) []docxParagraph {
	var paras []docxParagraph
	for _, b := range blocks {
		if b.blockType == docxBlockTypeParagraph {
			paras = append(paras, b.paragraph)
		} else if b.blockType == docxBlockTypeTable {
			for _, row := range b.table.rows {
				for _, cell := range row.cells {
					paras = append(paras, cell.paragraphs...)
				}
			}
		}
	}
	return paras
}

func buildParagraphSegments(text string, matches []docxMatch, anon *anonymizer.Anonymizer) []TextParagraphSegment {
	if len(matches) == 0 {
		return []TextParagraphSegment{
			{
				Text:     text,
				IsEntity: false,
			},
		}
	}

	var segments []TextParagraphSegment
	lastIdx := 0
	for _, m := range matches {
		if m.start > lastIdx && m.start <= len(text) {
			segments = append(segments, TextParagraphSegment{
				Text:     text[lastIdx:m.start],
				IsEntity: false,
			})
		}
		synth := generateSynthetic(anon, m.entityType, m.value)
		segments = append(segments, TextParagraphSegment{
			Text:       synth,
			IsEntity:   true,
			Type:       m.entityType,
			Original:   m.value,
			Anonymized: synth,
		})
		lastIdx = m.end
	}
	if lastIdx < len(text) {
		segments = append(segments, TextParagraphSegment{
			Text:     text[lastIdx:],
			IsEntity: false,
		})
	}
	return segments
}

func getDisabledEntityTypes(options map[string]interface{}) map[string]bool {
	disabled := make(map[string]bool)
	if options == nil {
		return disabled
	}

	addVal := func(v interface{}) {
		switch val := v.(type) {
		case string:
			s := strings.ToUpper(strings.TrimSpace(val))
			disabled[s] = true
			if s == "CARD" {
				disabled[string(detector.EntityCard)] = true
			} else if s == string(detector.EntityCard) {
				disabled["CARD"] = true
			}
			if s == "DATE" {
				disabled[string(detector.EntityBirthday)] = true
			} else if s == string(detector.EntityBirthday) {
				disabled["DATE"] = true
			}
		case detector.EntityType:
			disabled[strings.ToUpper(string(val))] = true
		}
	}

	addList := func(list interface{}) {
		switch l := list.(type) {
		case []string:
			for _, s := range l {
				addVal(s)
			}
		case []detector.EntityType:
			for _, s := range l {
				addVal(s)
			}
		case []interface{}:
			for _, itm := range l {
				addVal(itm)
			}
		}
	}

	if v, ok := options["disabled_entities"]; ok {
		addList(v)
	}
	if v, ok := options["disabled_entity_types"]; ok {
		addList(v)
	}

	return disabled
}

// Inspect examines paragraphs and tables in the DOCX archive and reports paragraph count, detected entities, snippets, structured paragraph previews, and full document blocks.
func (h *DOCXHandler) Inspect(r io.ReaderAt, size int64) (int, []EntityCount, []DocxTextSnippet, []TextParagraphPreview, []TextDocumentBlock, error) {
	if r == nil {
		return 0, nil, nil, nil, nil, errors.New("reader cannot be nil")
	}
	if size <= 0 {
		return 0, nil, nil, nil, nil, errors.New("invalid file size")
	}

	zr, err := zip.NewReader(r, size)
	if err != nil {
		return 0, nil, nil, nil, nil, fmt.Errorf("failed to open docx zip: %w", err)
	}

	paragraphCount := 0
	counts := make(map[string]int)
	var snippets []DocxTextSnippet
	var paragraphPreviews []TextParagraphPreview
	var documentBlocks []TextDocumentBlock

	// Filter and sort XML files in logical document reading order:
	// 1. word/document.xml (main body)
	// 2. word/footnotes.xml (footnotes)
	// 3. word/endnotes.xml (endnotes)
	// 4. word/comments.xml (comments)
	// 5. word/header*.xml & word/footer*.xml (headers & footers)
	var docxFiles []*zip.File
	for _, f := range zr.File {
		if isDocumentXML(f.Name) {
			docxFiles = append(docxFiles, f)
		}
	}
	sort.Slice(docxFiles, func(i, j int) bool {
		oi, _ := getDocxFileOrder(docxFiles[i].Name)
		oj, _ := getDocxFileOrder(docxFiles[j].Name)
		if oi == oj {
			return docxFiles[i].Name < docxFiles[j].Name
		}
		return oi < oj
	})

	for _, f := range docxFiles {
		rc, err := f.Open()
		if err != nil {
			return 0, nil, nil, nil, nil, fmt.Errorf("failed to open %s: %w", f.Name, err)
		}
		data, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			return 0, nil, nil, nil, nil, fmt.Errorf("failed to read %s: %w", f.Name, err)
		}

		blocks, err := parseDocxXML(data)
		if err != nil {
			return 0, nil, nil, nil, nil, fmt.Errorf("failed to parse %s: %w", f.Name, err)
		}

		paras := extractParagraphsFromBlocks(blocks)
		paragraphCount += len(paras)

		_, isBodyOrNotes := getDocxFileOrder(f.Name)

		for _, b := range blocks {
			if b.blockType == docxBlockTypeParagraph {
				var paraText strings.Builder
				for _, n := range b.paragraph.nodes {
					paraText.WriteString(n.text)
				}
				pStr := paraText.String()
				trimmedP := strings.TrimSpace(pStr)
				if trimmedP == "" {
					continue
				}

				matches := findDocxSensitiveMatches(pStr)
				for _, m := range matches {
					counts[m.entityType]++
					synth := generateSynthetic(h.Anon, m.entityType, m.value)
					snippets = append(snippets, DocxTextSnippet{
						Original:   m.value,
						Anonymized: synth,
						Type:       m.entityType,
					})
				}

				if isBodyOrNotes {
					segments := buildParagraphSegments(pStr, matches, h.Anon)

					paragraphPreviews = append(paragraphPreviews, TextParagraphPreview{
						Index:    len(paragraphPreviews) + 1,
						Segments: segments,
					})

					documentBlocks = append(documentBlocks, TextDocumentBlock{
						Type:     "paragraph",
						Index:    len(documentBlocks) + 1,
						Segments: segments,
					})
				}
			} else if b.blockType == docxBlockTypeTable {
				var rowPreviews []TextTableRowPreview
				for _, r := range b.table.rows {
					var cellPreviews []TextTableCellPreview
					for _, c := range r.cells {
						var cellText strings.Builder
						for pIdx, cp := range c.paragraphs {
							if pIdx > 0 {
								cellText.WriteString("\n")
							}
							for _, n := range cp.nodes {
								cellText.WriteString(n.text)
							}
						}
						cStr := cellText.String()
						matches := findDocxSensitiveMatches(cStr)
						for _, m := range matches {
							counts[m.entityType]++
							synth := generateSynthetic(h.Anon, m.entityType, m.value)
							snippets = append(snippets, DocxTextSnippet{
								Original:   m.value,
								Anonymized: synth,
								Type:       m.entityType,
							})
						}

						if isBodyOrNotes {
							var segments []TextParagraphSegment
							if strings.TrimSpace(cStr) == "" {
								segments = []TextParagraphSegment{{Text: cStr, IsEntity: false}}
							} else {
								segments = buildParagraphSegments(cStr, matches, h.Anon)
								paragraphPreviews = append(paragraphPreviews, TextParagraphPreview{
									Index:    len(paragraphPreviews) + 1,
									Segments: segments,
								})
							}

							cellPreviews = append(cellPreviews, TextTableCellPreview{
								Segments: segments,
							})
						}
					}
					if isBodyOrNotes {
						rowPreviews = append(rowPreviews, TextTableRowPreview{
							Cells: cellPreviews,
						})
					}
				}

				if isBodyOrNotes {
					documentBlocks = append(documentBlocks, TextDocumentBlock{
						Type:  "table",
						Index: len(documentBlocks) + 1,
						Table: &TextTablePreview{
							Rows: rowPreviews,
						},
					})
				}
			}
		}
	}

	// Produce deterministic entities slice
	typeOrder := []string{
		string(detector.EntityFIO),
		string(detector.EntityPassport),
		string(detector.EntityPhone),
		string(detector.EntitySNILS),
		string(detector.EntityINN),
		string(detector.EntityCard),
		string(detector.EntityEmail),
		string(detector.EntityBirthday),
		string(detector.EntityAddress),
	}

	var entities []EntityCount
	for _, t := range typeOrder {
		if c, ok := counts[t]; ok && c > 0 {
			entities = append(entities, EntityCount{Type: t, Count: c})
			delete(counts, t)
		}
	}
	var remainingTypes []string
	for k := range counts {
		remainingTypes = append(remainingTypes, k)
	}
	sort.Strings(remainingTypes)
	for _, k := range remainingTypes {
		entities = append(entities, EntityCount{Type: k, Count: counts[k]})
	}

	return paragraphCount, entities, snippets, paragraphPreviews, documentBlocks, nil
}

// Process reads a DOCX document from r, anonymizes detected entities across paragraphs and runs,
// writes the output archive to w, and returns the populated DepMap.
func (h *DOCXHandler) Process(ctx context.Context, r io.ReaderAt, size int64, w io.Writer, options map[string]interface{}) (*depmap.DepMap, error) {
	if h.Anon == nil {
		return nil, errors.New("anonymizer is not initialized")
	}
	if r == nil {
		return nil, errors.New("reader cannot be nil")
	}
	if w == nil {
		return nil, errors.New("writer cannot be nil")
	}
	if size <= 0 {
		return nil, errors.New("invalid file size")
	}

	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	zr, err := zip.NewReader(r, size)
	if err != nil {
		return nil, fmt.Errorf("failed to open docx zip: %w", err)
	}

	disabledTypes := getDisabledEntityTypes(options)
	mode := getProcessingMode(options)

	origName := "document.docx"
	if options != nil {
		if s, ok := options["original_file_name"].(string); ok && s != "" {
			origName = s
		}
	}
	var dm *depmap.DepMap
	if d, ok := options["depmap"].(*depmap.DepMap); ok {
		dm = d
	} else if d, ok := options["depMap"].(*depmap.DepMap); ok {
		dm = d
	}
	if dm == nil {
		dm = depmap.New(origName)
	}
	dm.Mode = string(mode)

	updatedXMLs := make(map[string][]byte)

	for _, f := range zr.File {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		if !isDocumentXML(f.Name) {
			continue
		}

		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("failed to open %s: %w", f.Name, err)
		}
		data, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			return nil, fmt.Errorf("failed to read %s: %w", f.Name, err)
		}

		blocks, err := parseDocxXML(data)
		if err != nil {
			return nil, fmt.Errorf("failed to parse %s: %w", f.Name, err)
		}

		paras := extractParagraphsFromBlocks(blocks)
		var replacements []textReplacement

		for _, p := range paras {
			if len(p.nodes) == 0 {
				continue
			}

			// 1. Build combined paragraph text and rune mapping
			var paraTextBuf strings.Builder
			nodeRuneStarts := make([]int, len(p.nodes))
			nodeRuneEnds := make([]int, len(p.nodes))

			curRuneIdx := 0
			for idx, n := range p.nodes {
				nodeRuneStarts[idx] = curRuneIdx
				rCount := utf8.RuneCountInString(n.text)
				nodeRuneEnds[idx] = curRuneIdx + rCount
				curRuneIdx += rCount
				paraTextBuf.WriteString(n.text)
			}

			paraText := paraTextBuf.String()
			paraRunes := []rune(paraText)
			totalRunes := len(paraRunes)

			// 2. Map byte offsets in paraText to rune offsets
			byteToRune := make([]int, len(paraText)+1)
			rIdx := 0
			for bIdx := range paraText {
				byteToRune[bIdx] = rIdx
				rIdx++
			}
			byteToRune[len(paraText)] = rIdx

			// 3. Find sensitive matches in paragraph text
			matches := findDocxSensitiveMatches(paraText)

			type entityRuneSpan struct {
				runeStart int
				runeEnd   int
				synthetic string
			}

			var spans []entityRuneSpan
			for _, m := range matches {
				if disabledTypes[strings.ToUpper(m.entityType)] {
					continue
				}
				rStart := byteToRune[m.start]
				rEnd := byteToRune[m.end]
				if rStart >= rEnd || rStart >= totalRunes {
					continue
				}
				if rEnd > totalRunes {
					rEnd = totalRunes
				}

				origVal := string(paraRunes[rStart:rEnd])
				var replVal string
				if mode == domain.ModeDelete {
					replVal = DeletionMarker
					dm.PutOrdinal(origVal)
				} else {
					replVal = generateSynthetic(h.Anon, m.entityType, origVal)
					dm.Put(replVal, origVal)
				}

				spans = append(spans, entityRuneSpan{
					runeStart: rStart,
					runeEnd:   rEnd,
					synthetic: replVal,
				})
			}

			if len(spans) == 0 {
				continue
			}

			// 4. Project replacements onto nodes
			for idx, n := range p.nodes {
				nStart := nodeRuneStarts[idx]
				nEnd := nodeRuneEnds[idx]

				var newNodeBuf strings.Builder
				nodeModified := false

				for r := nStart; r < nEnd; r++ {
					spanIdx := -1
					for sIdx, sp := range spans {
						if r >= sp.runeStart && r < sp.runeEnd {
							spanIdx = sIdx
							break
						}
					}

					if spanIdx == -1 {
						newNodeBuf.WriteRune(paraRunes[r])
					} else {
						nodeModified = true
						if r == spans[spanIdx].runeStart {
							newNodeBuf.WriteString(spans[spanIdx].synthetic)
						}
					}
				}

				if nodeModified {
					newText := newNodeBuf.String()
					var replBytes []byte
					if newText == "" {
						replBytes = []byte("<w:t></w:t>")
					} else {
						var escBuf bytes.Buffer
						_ = xml.EscapeText(&escBuf, []byte(newText))
						replBytes = []byte(fmt.Sprintf(`<w:t xml:space="preserve">%s</w:t>`, escBuf.String()))
					}

					replacements = append(replacements, textReplacement{
						start:   n.startOffset,
						end:     n.endOffset,
						newText: replBytes,
					})
				}
			}
		}

		newXML := applyReplacements(data, replacements)
		updatedXMLs[f.Name] = newXML
	}

	// Repack output zip
	zw := zip.NewWriter(w)
	for _, f := range zr.File {
		select {
		case <-ctx.Done():
			_ = zw.Close()
			return nil, ctx.Err()
		default:
		}

		if updatedData, ok := updatedXMLs[f.Name]; ok {
			h := &zip.FileHeader{
				Name:   f.Name,
				Method: zip.Deflate,
			}
			fw, err := zw.CreateHeader(h)
			if err != nil {
				return nil, fmt.Errorf("failed to create zip entry %s: %w", f.Name, err)
			}
			if _, err := fw.Write(updatedData); err != nil {
				return nil, fmt.Errorf("failed to write zip entry %s: %w", f.Name, err)
			}
		} else {
			if err := zw.Copy(f); err != nil {
				return nil, fmt.Errorf("failed to copy untouched zip entry %s: %w", f.Name, err)
			}
		}
	}

	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("failed to close docx zip writer: %w", err)
	}

	return dm, nil
}
