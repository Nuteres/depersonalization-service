package formats

import (
	"regexp"
	"strings"
	"unicode"

	"dep_service/internal/domain"
	"dep_service/pkg/anonymizer"
	"dep_service/pkg/detector"
)

var (
	reEmailPattern    = regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
	rePhonePattern    = regexp.MustCompile(`^(?:\+7|8)?[\s\-\(]*\d{3}[\s\-\)]*\d{3}[\s\-]*\d{2}[\s\-]*\d{2}$`)
	reDatePattern     = regexp.MustCompile(`^\d{2}[./-]\d{2}[./-]\d{4}$|^\d{4}[./-]\d{2}[./-]\d{2}$`)
	reSNILSPattern    = regexp.MustCompile(`^\d{3}[-\s]\d{3}[-\s]\d{3}[-\s]\d{2}$|^\d{11}$`)
	rePassportPattern = regexp.MustCompile(`^\d{4}\s+\d{6}$|^\d{10}$`)
	rePostalPattern   = regexp.MustCompile(`\b\d{6}\b`)
	reFIOPattern      = regexp.MustCompile(`^[А-ЯЁ][а-яё]+(?:-[А-ЯЁ][а-яё]+)?\s+[А-ЯЁ][а-яё]+\s+[А-ЯЁ][а-яё]+(?:ович|евич|ич|овна|евна|ична|оглы|кызы)$`)
)

// ColumnType defines the detected or specified PII type for a column.
type ColumnType int

const (
	ColUnknown ColumnType = iota
	ColFIO
	ColSNILS
	ColINN
	ColPassport
	ColPhone
	ColCard
	ColDate
	ColEmail
	ColLastName
	ColFirstName
	ColPatronymic
	ColAddress
	ColGrade
	ColEmployeeID
)

func isNonPerson(h string) bool {
	nonPersonKeywords := []string{
		"штатн", "подразделен", "организац", "позици", "должност",
		"функци", "блок", "филиал", "департамент", "управлен",
		"отдел", "сектор", "группа", "проект", "продукт",
		"систем", "объект", "тип", "вид", "категор", "статус",
		"код", "номер", "id", "файла", "документ", "сервер",
		"папк", "учетн", "обособлен", "узел", "маркер",
		"семейств", "уровен",
	}
	for _, kw := range nonPersonKeywords {
		if strings.Contains(h, kw) {
			return true
		}
	}
	return false
}

// DetectColumnType analyzes a column header string and returns the identified PII type.
func DetectColumnType(header string) ColumnType {
	h := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(header, "\ufeff")))
	if h == "" {
		return ColUnknown
	}

	// 1. Date / Period / Validity term (checked first so that composite headers like
	// "Дата выдачи паспорта", "Срок действия карты", "Период" are classified as dates).
	if strings.Contains(h, "дата") || strings.Contains(h, "date") ||
		strings.Contains(h, "срок") || strings.Contains(h, "период") ||
		strings.Contains(h, "рожден") || strings.Contains(h, "рожд") ||
		strings.Contains(h, "д/р") {
		return ColDate
	}

	// 2. SNILS
	if strings.Contains(h, "снилс") || strings.Contains(h, "snils") {
		return ColSNILS
	}

	// 3. INN
	if strings.Contains(h, "инн") || strings.Contains(h, "inn") {
		return ColINN
	}

	// 4. Passport
	if strings.Contains(h, "паспорт") || strings.Contains(h, "passport") ||
		strings.Contains(h, "серия и номер") || strings.Contains(h, "серия номер") ||
		strings.Contains(h, "дул") {
		return ColPassport
	}

	// 5. Bank Card
	if strings.Contains(h, "карт") || strings.Contains(h, "card") || strings.Contains(h, "pan") {
		return ColCard
	}

	// 6. Phone
	if strings.Contains(h, "телефон") || strings.Contains(h, "phone") ||
		strings.Contains(h, "тел.") || h == "тел" ||
		strings.Contains(h, "моб") || strings.Contains(h, "сотов") {
		return ColPhone
	}

	// 7. Email / Corporate or Personal (or Login)
	if strings.Contains(h, "почт") || strings.Contains(h, "email") ||
		strings.Contains(h, "e-mail") || strings.Contains(h, "mail") ||
		strings.Contains(h, "логин") || strings.Contains(h, "login") {
		return ColEmail
	}

	// 8. Employee ID / Personnel Number (Табельный номер)
	if strings.Contains(h, "табельн") || strings.Contains(h, "таб.") ||
		strings.Contains(h, "таб №") || strings.Contains(h, "таб no") ||
		strings.Contains(h, "таб_номер") || strings.Contains(h, "employee id") ||
		strings.Contains(h, "employee_id") || strings.Contains(h, "emp id") ||
		strings.Contains(h, "персональный номер") || strings.Contains(h, "личный номер") {
		return ColEmployeeID
	}

	// 9. Grade / Qualification level
	if (strings.Contains(h, "грейд") || strings.Contains(h, "grade") ||
		strings.Contains(h, "квалификационный разряд") || strings.Contains(h, "квалификационный уровень") ||
		h == "разряд" || strings.HasPrefix(h, "разряд ") || strings.HasSuffix(h, " разряд")) &&
		!strings.Contains(h, "должност") && !strings.Contains(h, "професси") {
		return ColGrade
	}

	// 10. Address / Location (registration, living address, city, street, region)
	if strings.Contains(h, "адрес") || strings.Contains(h, "address") ||
		strings.Contains(h, "место жительства") || strings.Contains(h, "место регистрации") ||
		strings.Contains(h, "место проживания") || strings.Contains(h, "фактический адрес") ||
		strings.Contains(h, "юридический адрес") || strings.Contains(h, "прописк") ||
		strings.Contains(h, "город") || strings.Contains(h, "населенный пункт") ||
		strings.Contains(h, "улица") || strings.Contains(h, "регион") ||
		(strings.Contains(h, "субъект") && strings.Contains(h, "рф") && !strings.Contains(h, "код")) {
		if !strings.Contains(h, "телефон") && !strings.Contains(h, "тел") &&
			!strings.Contains(h, "email") && !strings.Contains(h, "почт") &&
			!strings.Contains(h, "ip") && !strings.Contains(h, "mac") {
			return ColAddress
		}
	}

	// 11. Specific Name Components (checked before full FIO and roles)
	// Last Name / Surname
	if (strings.Contains(h, "фамили") || strings.Contains(h, "surname") ||
		strings.Contains(h, "last name") || strings.Contains(h, "lastname")) && !isNonPerson(h) {
		return ColLastName
	}

	// Patronymic
	if strings.Contains(h, "отчеств") || strings.Contains(h, "patronymic") ||
		strings.Contains(h, "middle name") {
		return ColPatronymic
	}

	// Full FIO (checked before First Name so "полное имя" or "full name" is classified as ColFIO)
	if strings.Contains(h, "фио") || strings.Contains(h, "ф.и.о") || strings.Contains(h, "fio") {
		return ColFIO
	}
	if (strings.Contains(h, "полное имя") || strings.Contains(h, "full name")) && !isNonPerson(h) {
		return ColFIO
	}

	// First Name
	if (h == "имя" || strings.HasPrefix(h, "имя ") || strings.HasPrefix(h, "имя_") ||
		strings.HasSuffix(h, " имя") || strings.Contains(h, "first name") ||
		strings.Contains(h, "firstname") || strings.Contains(h, "forename")) &&
		!strings.Contains(h, "полное") && !isNonPerson(h) {
		return ColFirstName
	}

	// Role/Person nouns: check they are not job titles, statuses, departments, IDs, or categories
	if strings.Contains(h, "клиент") || strings.Contains(h, "сотрудник") ||
		strings.Contains(h, "пациент") || strings.Contains(h, "работник") ||
		strings.Contains(h, "физлицо") || strings.Contains(h, "физ. лицо") ||
		strings.Contains(h, "учащ") || strings.Contains(h, "пользовател") ||
		strings.Contains(h, "автор") || strings.Contains(h, "создател") ||
		strings.Contains(h, "куратор") || strings.Contains(h, "инициатор") ||
		strings.Contains(h, "владелец") {
		if !isNonPerson(h) || strings.Contains(h, "автор") || strings.Contains(h, "создател") || strings.Contains(h, "куратор") || strings.Contains(h, "инициатор") || strings.Contains(h, "владелец") {
			return ColFIO
		}
	}

	return ColUnknown
}

// ParseColumnType converts string representation to ColumnType.
func ParseColumnType(s string) ColumnType {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "FIO", "NAME":
		return ColFIO
	case "LAST_NAME", "SURNAME":
		return ColLastName
	case "FIRST_NAME":
		return ColFirstName
	case "PATRONYMIC":
		return ColPatronymic
	case "SNILS":
		return ColSNILS
	case "INN":
		return ColINN
	case "PASSPORT":
		return ColPassport
	case "PHONE":
		return ColPhone
	case "BANK_CARD", "CARD":
		return ColCard
	case "BIRTHDAY", "DATE":
		return ColDate
	case "EMAIL", "MAIL":
		return ColEmail
	case "ADDRESS", "ADDR":
		return ColAddress
	case "GRADE":
		return ColGrade
	case "EMPLOYEE_ID", "TABEL", "TAB_NUM":
		return ColEmployeeID
	default:
		return ColUnknown
	}
}

func isSampleObviouslyNotPersonName(s string) bool {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return false
	}
	// Check for path or hierarchy characters
	if strings.ContainsAny(trimmed, `\/|>{}[]`) {
		return true
	}
	lower := strings.ToLower(trimmed)
	orgMarkers := []string{
		"пао", "ооо", "зао", "ао", "филиал", "отдел", "департамент", "управление",
		"группа", "бюро", "служба", "участок", "цех", "дирекция", "сегмент",
		"блок", "сектор", "направление", "проект", "подразделение", "структурн",
		"штатн", "должност", "руководитель", "специалист", "инженер", "менеджер", "директор",
	}
	for _, m := range orgMarkers {
		if strings.Contains(lower, m) {
			return true
		}
	}
	// Check for system IDs with numbers and dots (e.g. "143777.1", "01302.0342")
	if strings.Contains(trimmed, ".") {
		for _, r := range trimmed {
			if unicode.IsDigit(r) {
				return true
			}
		}
	}
	// Check length - Russian first name is rarely > 30 chars; FIO rarely > 60 chars
	if len([]rune(trimmed)) > 60 {
		return true
	}
	return false
}

func isAddressSample(v string) bool {
	lower := strings.ToLower(v)
	markers := []string{"ул.", "улица", "пр-кт", "проспект", "пер.", "переулок", "д.", "дом ", "кв.", "квартира", "корп.", "стр.", "шоссе", "б-р"}
	found := 0
	for _, m := range markers {
		if strings.Contains(lower, m) {
			found++
		}
	}
	if found >= 2 {
		return true
	}
	if found >= 1 && (strings.Contains(lower, "обл") || strings.Contains(lower, "г.") || strings.Contains(lower, "город")) {
		return true
	}
	// If 6-digit postal code, require at least one marker or city/region or federation keyword
	if rePostalPattern.MatchString(v) && (found >= 1 || strings.Contains(lower, "обл") || strings.Contains(lower, "г.") || strings.Contains(lower, "город") || strings.Contains(lower, "россия") || strings.Contains(lower, "федерация")) {
		return true
	}
	return false
}

// DetectColumnTypeWithSamples checks sample cell values (data-first) and header string to classify the column.
func DetectColumnTypeWithSamples(header string, samples []string) ColumnType {
	// 1. Data-First Analysis: inspect data samples for high-confidence structured PII patterns
	// (Email, Phone, SNILS, INN-12, Passport, Address, Date, Russian Full FIO).
	// When strong patterns match the sample data, data takes priority over arbitrary header names
	// (supporting SQL query aliases, DWH columns, custom ERP/CRM headers, etc.).
	if len(samples) > 0 {
		emailCount := 0
		phoneCount := 0
		dateCount := 0
		snilsCount := 0
		passportCount := 0
		innCount := 0
		fioCount := 0
		addressCount := 0
		nonEmpty := 0

		for _, s := range samples {
			v := strings.TrimSpace(s)
			if v == "" {
				continue
			}
			nonEmpty++
			if reEmailPattern.MatchString(v) {
				emailCount++
			}
			if rePhonePattern.MatchString(v) {
				phoneCount++
			}
			if reDatePattern.MatchString(v) {
				dateCount++
			}
			if reSNILSPattern.MatchString(v) || detector.ValidateSNILS(v) {
				snilsCount++
			}
			if rePassportPattern.MatchString(v) {
				passportCount++
			}
			if detector.ValidateINN12(v) {
				innCount++
			}
			if reFIOPattern.MatchString(v) {
				fioCount++
			}
			if isAddressSample(v) {
				addressCount++
			}
		}

		if nonEmpty > 0 {
			if emailCount > 0 && (emailCount*2 >= nonEmpty || emailCount >= 3) {
				return ColEmail
			}
			if phoneCount > 0 && (phoneCount*2 >= nonEmpty || phoneCount >= 3) {
				return ColPhone
			}
			if snilsCount > 0 && (snilsCount*2 >= nonEmpty || snilsCount >= 3) {
				return ColSNILS
			}
			if innCount > 0 && (innCount*2 >= nonEmpty || innCount >= 3) {
				return ColINN
			}
			if passportCount > 0 && (passportCount*2 >= nonEmpty || passportCount >= 3) {
				return ColPassport
			}
			if fioCount > 0 && (fioCount*2 >= nonEmpty || fioCount >= 3) {
				return ColFIO
			}
			if addressCount > 0 && (addressCount*2 >= nonEmpty || addressCount >= 3) {
				return ColAddress
			}
			if dateCount > 0 && (dateCount*2 >= nonEmpty || dateCount >= 3) {
				return ColDate
			}
		}
	}

	// 2. Header-Based Analysis: If data samples didn't match structured patterns (e.g. single-word names,
	// employee IDs, grades, or empty samples), analyze the column header.
	ct := DetectColumnType(header)

	// If header heuristic identified a person name (First/Last/Patronymic/FIO), validate against samples.
	// Columns like "Имя штатной единицы" or "Имя структурного подразделения" contain digits or org words.
	if ct == ColFirstName || ct == ColLastName || ct == ColPatronymic || ct == ColFIO {
		invalidCount := 0
		nonEmpty := 0
		for _, s := range samples {
			v := strings.TrimSpace(s)
			if v == "" {
				continue
			}
			nonEmpty++
			if isSampleObviouslyNotPersonName(v) {
				invalidCount++
			}
		}
		if nonEmpty > 0 && invalidCount > 0 {
			ct = ColUnknown
		}
	}

	return ct
}

// ResolveColumnTypes determines the ColumnType for each column header,
// taking into account any manual overrides passed in options["columns"].
func ResolveColumnTypes(headers []string, options map[string]interface{}) []ColumnType {
	return ResolveColumnTypesWithSamples(headers, nil, options)
}

// ResolveColumnTypesWithSamples determines ColumnTypes considering headers, sample rows, and options overrides.
func ResolveColumnTypesWithSamples(headers []string, sampleRows [][]string, options map[string]interface{}) []ColumnType {
	colTypes := make([]ColumnType, len(headers))

	var colMapByName map[string]string
	if options != nil {
		if m, ok := options["columns"].(map[string]string); ok {
			colMapByName = m
		} else if m, ok := options["columns"].(map[string]interface{}); ok {
			colMapByName = make(map[string]string, len(m))
			for k, v := range m {
				if vs, ok := v.(string); ok {
					colMapByName[k] = vs
				}
			}
		}
	}

	for i, rawH := range headers {
		h := strings.TrimPrefix(rawH, "\ufeff")
		cleanLower := strings.ToLower(strings.TrimSpace(h))
		if colMapByName != nil {
			if typeStr, ok := colMapByName[h]; ok {
				colTypes[i] = ParseColumnType(typeStr)
				continue
			}
			if typeStr, ok := colMapByName[rawH]; ok {
				colTypes[i] = ParseColumnType(typeStr)
				continue
			}
			if typeStr, ok := colMapByName[cleanLower]; ok {
				colTypes[i] = ParseColumnType(typeStr)
				continue
			}
		}

		var samples []string
		if len(sampleRows) > 0 {
			for _, row := range sampleRows {
				if i < len(row) && strings.TrimSpace(row[i]) != "" {
					samples = append(samples, strings.TrimSpace(row[i]))
				}
			}
		}

		colTypes[i] = DetectColumnTypeWithSamples(h, samples)
	}

	return colTypes
}

// AnonymizeCell replaces cell value with deterministic synthetic data according to colType.
func AnonymizeCell(anon *anonymizer.Anonymizer, colType ColumnType, val string, dateRange int) string {
	if strings.TrimSpace(val) == "" || anon == nil {
		return val
	}

	switch colType {
	case ColFIO:
		return anon.GenerateFIO(val)
	case ColLastName:
		return anon.GenerateLastName(val)
	case ColFirstName:
		return anon.GenerateFirstName(val)
	case ColPatronymic:
		return anon.GeneratePatronymic(val)
	case ColSNILS:
		return anon.GenerateSNILS(val)
	case ColINN:
		return anon.GenerateINN12(val)
	case ColPassport:
		return anon.GeneratePassport(val)
	case ColPhone:
		return anon.GeneratePhone(val)
	case ColCard:
		return anon.GenerateCardMIR(val)
	case ColDate:
		return anon.PerturbDate(val, dateRange)
	case ColEmail:
		return anon.GenerateEmail(val)
	case ColAddress:
		return anon.GenerateAddress(val)
	case ColGrade:
		return anon.GenerateGrade(val)
	case ColEmployeeID:
		return anon.GenerateEmployeeID(val)
	default:
		return val
	}
}

// DeletionMarker is the standardized string marker used to replace sensitive personal data in ModeDelete.
const DeletionMarker = "[УДАЛЕНО]"

// ProcessCell processes a cell value according to colType and mode (replace or delete).
func ProcessCell(anon *anonymizer.Anonymizer, colType ColumnType, val string, mode domain.ProcessingMode, dateRange int) string {
	if strings.TrimSpace(val) == "" || colType == ColUnknown {
		return val
	}
	if mode == domain.ModeDelete {
		return DeletionMarker
	}
	return AnonymizeCell(anon, colType, val, dateRange)
}

func getProcessingMode(options map[string]interface{}) domain.ProcessingMode {
	if options == nil {
		return domain.ModeReplace
	}
	if m, ok := options["mode"].(domain.ProcessingMode); ok && m != "" {
		return m
	}
	if s, ok := options["mode"].(string); ok && s != "" {
		return domain.ProcessingMode(s)
	}
	return domain.ModeReplace
}

func getDateRange(options map[string]interface{}) int {
	if options == nil {
		return 30
	}
	if r, ok := options["date_range"].(int); ok && r > 0 {
		return r
	}
	if r, ok := options["days_range"].(int); ok && r > 0 {
		return r
	}
	return 30
}

// BuildDisabledMap creates a set of 1-based disabled column indices from options,
// taking into account sheet-specific overrides in disabled_columns_by_sheet.
func BuildDisabledMap(sheetName string, options map[string]interface{}) map[int]bool {
	disabledMap := make(map[int]bool)
	if options == nil {
		return disabledMap
	}

	hasSheetSpecific := false
	if bySheet, ok := options["disabled_columns_by_sheet"].(map[string][]int); ok && len(bySheet) > 0 {
		if dList, found := bySheet[sheetName]; found {
			hasSheetSpecific = true
			for _, colIdx := range dList {
				disabledMap[colIdx] = true
			}
		}
	} else if bySheetIntf, ok := options["disabled_columns_by_sheet"].(map[string]interface{}); ok && len(bySheetIntf) > 0 {
		if sheetVal, found := bySheetIntf[sheetName]; found {
			hasSheetSpecific = true
			if dList, ok := sheetVal.([]int); ok {
				for _, colIdx := range dList {
					disabledMap[colIdx] = true
				}
			} else if dInterface, ok := sheetVal.([]interface{}); ok {
				for _, itm := range dInterface {
					if n, ok := itm.(int); ok {
						disabledMap[n] = true
					} else if f, ok := itm.(float64); ok {
						disabledMap[int(f)] = true
					}
				}
			}
		}
	}

	if !hasSheetSpecific {
		if dList, ok := options["disabled_columns"].([]int); ok {
			for _, colIdx := range dList {
				disabledMap[colIdx] = true
			}
		} else if dInterface, ok := options["disabled_columns"].([]interface{}); ok {
			for _, itm := range dInterface {
				if n, ok := itm.(int); ok {
					disabledMap[n] = true
				} else if f, ok := itm.(float64); ok {
					disabledMap[int(f)] = true
				}
			}
		}
	}

	return disabledMap
}
