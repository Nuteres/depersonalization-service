package formats

import (
	"strings"
	"unicode"
)

// FindHeaderRow examines sampled leading rows from a spreadsheet or tabular dataset
// and identifies the index of the primary table header row (0-based) along with its column headers.
// If the table begins with preamble metadata (report title, filters, dates, empty rows),
// FindHeaderRow skips those preamble rows and locates the row containing actual column headers.
func FindHeaderRow(rows [][]string) (int, []string) {
	if len(rows) == 0 {
		return 0, nil
	}
	if len(rows) == 1 {
		return 0, rows[0]
	}

	bestIdx := 0
	maxScore := -1
	var bestHeaders []string

	limit := len(rows)
	if limit > 40 {
		limit = 40
	}

	for i := 0; i < limit; i++ {
		row := rows[i]
		nonEmpty := 0
		textCells := 0
		for _, cell := range row {
			t := strings.TrimSpace(strings.TrimPrefix(cell, "\ufeff"))
			if t != "" {
				nonEmpty++
				if !isPureNumericOrDate(t) {
					textCells++
				}
			}
		}

		if nonEmpty < 2 {
			continue
		}

		// Look ahead to next non-empty row
		nextNonEmpty := 0
		for nextIdx := i + 1; nextIdx < len(rows) && nextIdx <= i+3; nextIdx++ {
			c := countNonEmpty(rows[nextIdx])
			if c > 0 {
				nextNonEmpty = c
				break
			}
		}

		score := nonEmpty*2 + textCells*3
		if nextNonEmpty >= 2 {
			score += 15
		}

		// Bonus for standard table header keywords
		joined := strings.ToLower(strings.Join(row, " "))
		keywords := []string{
			"фио", "сотрудник", "организация", "должность", "дата", "номер", "почта", "статус",
			"курс", "клиент", "паспорт", "снилс", "инн", "телефон", "карта", "id", "name",
			"код", "наименование", "сумма", "результат", "подразделение",
		}
		for _, kw := range keywords {
			if strings.Contains(joined, kw) {
				score += 6
			}
		}

		if score > maxScore {
			maxScore = score
			bestIdx = i
			bestHeaders = row
		}
	}

	if maxScore < 0 {
		return 0, rows[0]
	}
	return bestIdx, bestHeaders
}

func countNonEmpty(row []string) int {
	c := 0
	for _, cell := range row {
		if strings.TrimSpace(cell) != "" {
			c++
		}
	}
	return c
}

func isPureNumericOrDate(s string) bool {
	digits := 0
	total := 0
	for _, r := range s {
		if unicode.IsDigit(r) {
			digits++
		}
		if !unicode.IsSpace(r) {
			total++
		}
	}
	return total > 0 && float64(digits)/float64(total) > 0.6
}
