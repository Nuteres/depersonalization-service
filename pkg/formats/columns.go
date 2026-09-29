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
// Russian column keywords: фио, снилс, инн, паспорт, телефон, карта, дата.
func DetectColumnType(header string) ColumnType {
	h := strings.ToLower(strings.TrimSpace(header))
	if h == "" {
		return ColUnknown
	}

	// 1. SNILS
	if strings.Contains(h, "снилс") || strings.Contains(h, "snils") {
		return ColSNILS
	}

	// 2. INN
	if strings.Contains(h, "инн") || strings.Contains(h, "inn") {
		return ColINN
	}

	// 3. Passport
	if strings.Contains(h, "паспорт") || strings.Contains(h, "passport") ||
		strings.Contains(h, "серия и номер") || strings.Contains(h, "серия номер") ||
		strings.Contains(h, "дул") {
		return ColPassport
	}

	// 4. Bank Card
	if strings.Contains(h, "карт") || strings.Contains(h, "card") || strings.Contains(h, "pan") {
		return ColCard
	}

	// 5. Phone
	if strings.Contains(h, "телефон") || strings.Contains(h, "phone") ||
		strings.Contains(h, "тел.") || h == "тел" ||
		strings.Contains(h, "моб") || strings.Contains(h, "сотов") {
		return ColPhone
	}

	// 6. Date (checked before FIO to avoid classifying 'дата рождения клиента' as FIO)
	if strings.Contains(h, "дата") || strings.Contains(h, "date") ||
		strings.Contains(h, "рожден") || strings.Contains(h, "рожд") ||
		strings.Contains(h, "д/р") {
		return ColDate
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

	for i, h := range headers {
		if colMapByName != nil {
			if typeStr, ok := colMapByName[h]; ok {
				colTypes[i] = ParseColumnType(typeStr)
				continue
			}
			if typeStr, ok := colMapByName[strings.ToLower(strings.TrimSpace(h))]; ok {
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
