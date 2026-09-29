package formats

import (
	"strings"

	"dep_service/pkg/anonymizer"
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
)

// DetectColumnType analyzes a column header string and returns the identified PII type.
// Russian column keywords: фио, снилс, инн, паспорт, телефон, карта, дата/срок/период.
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

	// 7. FIO / Person
	if strings.Contains(h, "фио") || strings.Contains(h, "ф.и.о") || strings.Contains(h, "fio") ||
		strings.Contains(h, "фамили") || strings.Contains(h, "клиент") ||
		strings.Contains(h, "сотрудник") || strings.Contains(h, "пациент") ||
		strings.Contains(h, "работник") || strings.Contains(h, "физлицо") ||
		strings.Contains(h, "физ. лицо") {
		return ColFIO
	}

	return ColUnknown
}

// ParseColumnType converts string representation to ColumnType.
func ParseColumnType(s string) ColumnType {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "FIO", "NAME":
		return ColFIO
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
	default:
		return ColUnknown
	}
}

// ResolveColumnTypes determines the ColumnType for each column header,
// taking into account any manual overrides passed in options["columns"].
// Strips UTF-8 BOM if present so lookups in options["columns"] match cleanly.
func ResolveColumnTypes(headers []string, options map[string]interface{}) []ColumnType {
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
		colTypes[i] = DetectColumnType(h)
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
	default:
		return val
	}
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
