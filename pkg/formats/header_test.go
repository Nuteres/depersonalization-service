package formats

import (
	"reflect"
	"testing"
)

func TestFindHeaderRow_StandardTable(t *testing.T) {
	rows := [][]string{
		{"ФИО", "СНИЛС", "Телефон", "Сумма"},
		{"Иванов Иван", "112-233-445 95", "+7 916 111-22-33", "15000"},
		{"Петров Петр", "223-344-556 06", "+7 925 222-33-44", "25000"},
	}

	idx, headers := FindHeaderRow(rows)
	if idx != 0 {
		t.Fatalf("expected header at index 0, got %d", idx)
	}
	expected := []string{"ФИО", "СНИЛС", "Телефон", "Сумма"}
	if !reflect.DeepEqual(headers, expected) {
		t.Fatalf("expected headers %v, got %v", expected, headers)
	}
}

func TestFindHeaderRow_WithPreamble(t *testing.T) {
	rows := [][]string{
		{"Отчет по курсам компании"},
		{"Период: 01.01.2025 - 31.12.2025"},
		{"Подразделение: Все"},
		{},
		{"ID", "ФИО сотрудника", "Рабочая почта", "Дата приема", "Статус"},
		{"101", "Сидорова Анна", "a.sidorova@corp.local", "01.02.2020", "Активен"},
		{"102", "Козлов Михаил", "m.kozlov@corp.local", "15.03.2021", "Активен"},
	}

	idx, headers := FindHeaderRow(rows)
	if idx != 4 {
		t.Fatalf("expected header at index 4, got %d", idx)
	}
	if len(headers) != 5 || headers[1] != "ФИО сотрудника" {
		t.Fatalf("unexpected headers: %v", headers)
	}
}

func TestFindHeaderRow_ParticipantKeyword(t *testing.T) {
	rows := [][]string{
		{"Компания", "Участник", "Email", "Год рождения"},
		{"Ростелеком", "ФИО1", "user@corp.ru", "1990"},
	}

	idx, headers := FindHeaderRow(rows)
	if idx != 0 {
		t.Fatalf("expected header at index 0, got %d", idx)
	}
	if headers[1] != "Участник" {
		t.Fatalf("expected 'Участник', got %q", headers[1])
	}
}

func TestFindHeaderRow_EdgeCases(t *testing.T) {
	// Empty
	idx, headers := FindHeaderRow(nil)
	if idx != 0 || headers != nil {
		t.Fatalf("expected 0, nil for empty rows")
	}

	// Single row
	single := [][]string{{"Колонка1", "Колонка2"}}
	idx, headers = FindHeaderRow(single)
	if idx != 0 || len(headers) != 2 {
		t.Fatalf("expected 0, [Колонка1, Колонка2] for single row")
	}
}

func TestBuildHierarchicalHeaders_MultiRowWithMerges(t *testing.T) {
	rows := [][]string{
		{"", "", "Корпоративные компетенции", "", "Оценка"},
		{"", "", "Блок результат", "Блок команда", "Оценка"},
		{"ФИО", "Email", "Тест 1", "Тест 2", "Итог"},
		{"Иванов И.И.", "ivanov@rt.ru", "5", "4", "4.5"},
	}

	merges := []ExcelMergeRange{
		{StartCol: 3, StartRow: 1, EndCol: 4, EndRow: 1, Value: "Корпоративные компетенции"},
		{StartCol: 1, StartRow: 1, EndCol: 1, EndRow: 2, Value: "ФИО"},
	}

	leafIdx := 2
	headers := BuildHierarchicalHeaders(rows, leafIdx, merges)

	expected := []string{
		"ФИО",
		"Email",
		"Корпоративные компетенции / Блок результат / Тест 1",
		"Корпоративные компетенции / Блок команда / Тест 2",
		"Оценка / Итог",
	}

	if !reflect.DeepEqual(headers, expected) {
		t.Fatalf("expected hierarchical headers:\n%v\ngot:\n%v", expected, headers)
	}
}

func TestBuildHierarchicalHeaders_WithPreambleExcluded(t *testing.T) {
	rows := [][]string{
		{"Отчет по сотрудникам компании"},
		{"Фильтр: отдел разработки"},
		{},
		{"ID", "ФИО", "Email"},
		{"1", "Сидоров С.С.", "sidorov@corp.local"},
	}

	leafIdx := 3
	headers := BuildHierarchicalHeaders(rows, leafIdx, nil)

	expected := []string{"ID", "ФИО", "Email"}
	if !reflect.DeepEqual(headers, expected) {
		t.Fatalf("expected headers %v, got %v", expected, headers)
	}
}

func TestBuildHierarchicalHeaders_DuplicatePathsDisambiguated(t *testing.T) {
	rows := [][]string{
		{"Оценка", "Оценка"},
		{"Балл", "Балл"},
		{"10", "15"},
	}
	merges := []ExcelMergeRange{
		{StartCol: 1, StartRow: 1, EndCol: 2, EndRow: 1, Value: "Оценка"},
	}
	headers := BuildHierarchicalHeaders(rows, 1, merges)
	expected := []string{"Оценка / Балл", "Оценка / Балл (2)"}
	if !reflect.DeepEqual(headers, expected) {
		t.Fatalf("expected disambiguated headers %v, got %v", expected, headers)
	}
}

func TestBuildHierarchicalHeaders_TitleBannerExcluded(t *testing.T) {
	rows := [][]string{
		{"Общий отчет по оценке персонала компании за 2026 год", "", "", ""},
		{"Личные данные", "", "Результаты", ""},
		{"ФИО", "Email", "Тест", "Балл"},
	}
	merges := []ExcelMergeRange{
		{StartCol: 1, StartRow: 1, EndCol: 4, EndRow: 1, Value: "Общий отчет по оценке персонала компании за 2026 год"},
		{StartCol: 1, StartRow: 2, EndCol: 2, EndRow: 2, Value: "Личные данные"},
		{StartCol: 3, StartRow: 2, EndCol: 4, EndRow: 2, Value: "Результаты"},
	}
	headers := BuildHierarchicalHeaders(rows, 2, merges)
	expected := []string{
		"Личные данные / ФИО",
		"Личные данные / Email",
		"Результаты / Тест",
		"Результаты / Балл",
	}
	if !reflect.DeepEqual(headers, expected) {
		t.Fatalf("expected banner excluded %v, got %v", expected, headers)
	}
}
