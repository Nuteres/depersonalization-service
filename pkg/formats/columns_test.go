package formats

import (
	"strings"
	"testing"

	"dep_service/internal/domain"
	"dep_service/pkg/anonymizer"
)

func TestDetectColumnType_SeparateNames(t *testing.T) {
	tests := []struct {
		header   string
		expected ColumnType
	}{
		{"Фамилия", ColLastName},
		{"фамилия сотрудника", ColLastName},
		{"Имя", ColFirstName},
		{"имя_клиента", ColFirstName},
		{"Отчество", ColPatronymic},
		{"отчество", ColPatronymic},
		{"ФИО", ColFIO},
		{"фио", ColFIO},
		{"Полное имя", ColFIO},
		{"Логин", ColEmail},
		{"login", ColEmail},
		{"Почта", ColEmail},
		{"Email", ColEmail},
		{"Телефон", ColPhone},
	}

	for _, tt := range tests {
		got := DetectColumnType(tt.header)
		if got != tt.expected {
			t.Errorf("DetectColumnType(%q) = %v; want %v", tt.header, got, tt.expected)
		}
	}
}

func TestDetectColumnTypeWithSamples_EmailLogin(t *testing.T) {
	// Custom header "Логин(вторичныйключ)" with email sample values
	header := "Логин(вторичныйключ)"
	samples := []string{
		"samed_samedov_01@list.ru",
		"lilw776@gmail.com",
		"vladakudryash97@mail.ru",
	}

	got := DetectColumnTypeWithSamples(header, samples)
	if got != ColEmail {
		t.Fatalf("expected ColEmail for header %q with email samples, got %v", header, got)
	}
}

func TestAnonymizeCell_SeparateNamesSingleWord(t *testing.T) {
	anon := anonymizer.New("salt-test-names")

	// Last Name
	surVal := AnonymizeCell(anon, ColLastName, "Самедов", 30)
	if len(strings.Fields(surVal)) != 1 {
		t.Errorf("expected single-word replacement for ColLastName, got %q", surVal)
	}

	// First Name
	firstVal := AnonymizeCell(anon, ColFirstName, "Самед", 30)
	if len(strings.Fields(firstVal)) != 1 {
		t.Errorf("expected single-word replacement for ColFirstName, got %q", firstVal)
	}

	// Patronymic
	patrVal := AnonymizeCell(anon, ColPatronymic, "Намиг Оглы", 30)
	if len(strings.Fields(patrVal)) != 1 {
		t.Errorf("expected single-word replacement for ColPatronymic, got %q", patrVal)
	}

	// Full FIO
	fioVal := AnonymizeCell(anon, ColFIO, "Самедов Самед Намиг Оглы", 30)
	if len(strings.Fields(fioVal)) != 3 {
		t.Errorf("expected three-word replacement for ColFIO, got %q", fioVal)
	}

	// Email
	emailVal := AnonymizeCell(anon, ColEmail, "samed_samedov_01@list.ru", 30)
	if !strings.Contains(emailVal, "@list.ru") {
		t.Errorf("expected preserved domain in synthetic email, got %q", emailVal)
	}

	// Address
	addrVal := AnonymizeCell(anon, ColAddress, "610040, Кировская обл., г. Киров, ул. Торфяная, д. 11, корп. 1, кв. 284", 30)
	if addrVal == "610040, Кировская обл., г. Киров, ул. Торфяная, д. 11, корп. 1, кв. 284" || !strings.Contains(addrVal, "д.") {
		t.Errorf("expected anonymized address with house, got %q", addrVal)
	}

	// Grade
	gradeVal := AnonymizeCell(anon, ColGrade, "5", 30)
	if gradeVal == "5" || gradeVal == "" {
		t.Errorf("expected shifted grade, got %q", gradeVal)
	}

	// Employee ID
	empVal := AnonymizeCell(anon, ColEmployeeID, "746647", 30)
	if empVal == "746647" || len(empVal) != 6 {
		t.Errorf("expected transformed employee ID with same length 6, got %q", empVal)
	}
}

func TestDetectColumnType_ExclusionsAndNewTypes(t *testing.T) {
	tests := []struct {
		header   string
		expected ColumnType
	}{
		{"Имя штатной единицы", ColUnknown},
		{"Имя штатной единицы руководителя", ColUnknown},
		{"Структурное подразделение - системное имя", ColUnknown},
		{"Наименование должности", ColUnknown},
		{"Код должности", ColUnknown},
		{"Таб. №", ColEmployeeID},
		{"Табельный номер", ColEmployeeID},
		{"Грейд", ColGrade},
		{"Грейд (новый)", ColGrade},
		{"Адрес регистрации", ColAddress},
		{"Адрес расположения на назначении", ColAddress},
		{"Город (Адрес расположения на назначении)", ColAddress},
		{"Регион (Адрес расположения на назначении)", ColAddress},
		{"Ф.И.О.", ColFIO},
		{"ФИО руководителя", ColFIO},
	}

	for _, tt := range tests {
		got := DetectColumnType(tt.header)
		if got != tt.expected {
			t.Errorf("DetectColumnType(%q) = %v; want %v", tt.header, got, tt.expected)
		}
	}
}

func TestDetectColumnTypeWithSamples_AddressAndDisqualification(t *testing.T) {
	// 1. Column with generic header but address samples
	addrSamples := []string{
		"610040, Кировская обл., г. Киров, ул. Торфяная, д. 11, корп. 1, кв. 284",
		"610045, Кировская обл., г. Киров, ул. Маршала И.С.Конева, д. 13, кв. 272",
	}
	gotAddr := DetectColumnTypeWithSamples("Местонахождение", addrSamples)
	if gotAddr != ColAddress {
		t.Errorf("expected ColAddress for address samples, got %v", gotAddr)
	}

	// 2. Disqualify False Name if sample has hierarchy ID / organizational markers
	falseNameSamples := []string{
		"Руководитель направления.143777.1",
		"Руководитель группы.196494.1",
	}
	gotFalseName := DetectColumnTypeWithSamples("Имя", falseNameSamples)
	if gotFalseName != ColUnknown {
		t.Errorf("expected ColUnknown for false name samples, got %v", gotFalseName)
	}
}

func TestDetectColumnTypeWithSamples_AuthorRecordAndDWH(t *testing.T) {
	// 1. "Автор записи" with corporate emails and index row
	authorSamples := []string{
		"21",
		"MIKHEEVA.EKATERINA@RT.RU",
		"EKATERINA.AFANASEVA@RT.RU",
		"MIKHEEVA.EKATERINA@RT.RU",
		"NADEZHDA.MOISEEVA",
		"YULIA.ZHURAVLEVA@RT.RU",
		"MIKHEEVA.EKATERINA@RT.RU",
	}
	gotAuthor := DetectColumnTypeWithSamples("Автор записи", authorSamples)
	if gotAuthor != ColEmail {
		t.Errorf("expected ColEmail for 'Автор записи' with emails, got %v", gotAuthor)
	}

	// 2. Arbitrary SQL / DWH technical column names with structured PII
	dwhEmail := DetectColumnTypeWithSamples("usr_account_alias_v1", []string{
		"admin@corp.example.ru",
		"user.dev@test.org",
		"support@service.ru",
	})
	if dwhEmail != ColEmail {
		t.Errorf("expected ColEmail for DWH column, got %v", dwhEmail)
	}

	dwhPhone := DetectColumnTypeWithSamples("fld_val_99", []string{
		"+7 999 123-45-67",
		"8 (926) 000-11-22",
		"+79031112233",
	})
	if dwhPhone != ColPhone {
		t.Errorf("expected ColPhone for DWH column, got %v", dwhPhone)
	}

	dwhSNILS := DetectColumnTypeWithSamples("col_doc_num", []string{
		"112-233-445 95",
		"001-001-001 30",
		"123-456-789 01",
	})
	if dwhSNILS != ColSNILS {
		t.Errorf("expected ColSNILS for DWH column, got %v", dwhSNILS)
	}

	dwhFIO := DetectColumnTypeWithSamples("Инициатор (куратор) ГПХ", []string{
		"Зайцева Анастасия Ивановна",
		"Волков Роман Михайлович",
		"Соловьева Мария Романовна",
	})
	if dwhFIO != ColFIO {
		t.Errorf("expected ColFIO for 'Инициатор (куратор) ГПХ', got %v", dwhFIO)
	}
}

func TestProcessCell_ModeDelete(t *testing.T) {
	anon := anonymizer.New("test-salt")
	val := "Иванов Иван Иванович"

	replaced := ProcessCell(anon, ColFIO, val, domain.ModeReplace, 30)
	if replaced == val || replaced == DeletionMarker {
		t.Fatalf("expected synthetic replacement, got %q", replaced)
	}

	deleted := ProcessCell(anon, ColFIO, val, domain.ModeDelete, 30)
	if deleted != DeletionMarker {
		t.Fatalf("expected %q, got %q", DeletionMarker, deleted)
	}

	empty := ProcessCell(anon, ColFIO, "", domain.ModeDelete, 30)
	if empty != "" {
		t.Fatalf("expected empty string for empty cell, got %q", empty)
	}

	unknown := ProcessCell(anon, ColUnknown, "Simple Text", domain.ModeDelete, 30)
	if unknown != "Simple Text" {
		t.Fatalf("expected untouched text for ColUnknown, got %q", unknown)
	}
}


