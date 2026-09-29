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
