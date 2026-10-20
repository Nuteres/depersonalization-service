package formats

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/xuri/excelize/v2"
)

// ExcelMergeRange represents a merged cell range and its string value for hierarchical header resolution.
type ExcelMergeRange struct {
	StartCol int
	StartRow int
	EndCol   int
	EndRow   int
	Value    string
}

// ExtractMergeRanges extracts merge cell coordinates and their top-left values from an Excel worksheet.
func ExtractMergeRanges(xl *excelize.File, sheetName string) []ExcelMergeRange {
	if xl == nil {
		return nil
	}
	rawMerges, err := xl.GetMergeCells(sheetName)
	if err != nil || len(rawMerges) == 0 {
		return nil
	}
	ranges := make([]ExcelMergeRange, 0, len(rawMerges))
	for _, m := range rawMerges {
		sCol, sRow, err1 := excelize.CellNameToCoordinates(m.GetStartAxis())
		eCol, eRow, err2 := excelize.CellNameToCoordinates(m.GetEndAxis())
		if err1 != nil || err2 != nil {
			continue
		}
		val, _ := xl.GetCellValue(sheetName, m.GetStartAxis())
		ranges = append(ranges, ExcelMergeRange{
			StartCol: sCol,
			StartRow: sRow,
			EndCol:   eCol,
			EndRow:   eRow,
			Value:    strings.TrimSpace(val),
		})
	}
	return ranges
}

// BuildHierarchicalHeaders constructs unified column headers for multi-level/hierarchical
// table headers (spanning headerStartRow to headerEndRow), taking into account merged cells
// or multi-tier category rows above the primary leaf header row.
func BuildHierarchicalHeaders(rows [][]string, leafHeaderIdx int, merges []ExcelMergeRange) []string {
	if len(rows) == 0 {
		return nil
	}
	if leafHeaderIdx < 0 || leafHeaderIdx >= len(rows) {
		leafHeaderIdx = 0
	}

	leafRow := rows[leafHeaderIdx]
	numCols := len(leafRow)
	for r := 0; r <= leafHeaderIdx; r++ {
		if len(rows[r]) > numCols {
			numCols = len(rows[r])
		}
	}
	if numCols == 0 {
		return nil
	}

	// If leafHeaderIdx is 0 and no merges exist, return standard row 0 headers
	if leafHeaderIdx == 0 && len(merges) == 0 {
		res := make([]string, len(leafRow))
		copy(res, leafRow)
		return res
	}

	// Determine headerStartIdx by scanning upward from leafHeaderIdx - 1
	headerStartIdx := leafHeaderIdx
	for r := leafHeaderIdx - 1; r >= 0; r-- {
		// If row is completely empty, stop (blank row separates metadata from header)
		if countNonEmpty(rows[r]) == 0 {
			break
		}

		// Check if row r contains category merges that group columns (not a full-width title banner)
		hasCategoryMerge := false
		r1Based := r + 1
		for _, m := range merges {
			if m.StartRow <= r1Based && r1Based <= m.EndRow && m.EndCol > m.StartCol && m.Value != "" {
				// If the merge spans the entire or almost entire table (>= 80% cols and numCols >= 4), it's a report title banner
				span := m.EndCol - m.StartCol + 1
				if numCols >= 4 && span >= int(float64(numCols)*0.8) {
					continue
				}
				hasCategoryMerge = true
				break
			}
		}

		// A row is part of header block if it has column category merges,
		// or if it has multiple non-empty cells that don't look like document preamble titles
		nonEmpty := countNonEmpty(rows[r])
		if hasCategoryMerge || (nonEmpty >= 2 && !isPreambleRow(rows[r])) {
			headerStartIdx = r
		} else {
			// Single non-empty cell or preamble title row terminates header scanning upward
			break
		}
	}

	// Build a coordinate lookup for merges: key "col:row" (1-based)
	// Intersect merge ranges with header bounds [headerStartIdx+1, leafHeaderIdx+1] and [1, numCols]
	// to avoid materializing huge non-header ranges in memory.
	mergeMap := make(map[string]string)
	minHRow1 := headerStartIdx + 1
	maxHRow1 := leafHeaderIdx + 1
	for _, m := range merges {
		if m.Value == "" {
			continue
		}
		startR := m.StartRow
		if startR < minHRow1 {
			startR = minHRow1
		}
		endR := m.EndRow
		if endR > maxHRow1 {
			endR = maxHRow1
		}
		startC := m.StartCol
		if startC < 1 {
			startC = 1
		}
		endC := m.EndCol
		if endC > numCols {
			endC = numCols
		}
		if startR > endR || startC > endC {
			continue
		}

		for r := startR; r <= endR; r++ {
			for c := startC; c <= endC; c++ {
				key := fmt.Sprintf("%d:%d", c, r)
				mergeMap[key] = m.Value
			}
		}
	}

	combined := make([]string, numCols)
	for c := 0; c < numCols; c++ {
		c1Based := c + 1
		var parts []string
		for r := headerStartIdx; r <= leafHeaderIdx; r++ {
			r1Based := r + 1
			key := fmt.Sprintf("%d:%d", c1Based, r1Based)
			val := ""
			if mv, ok := mergeMap[key]; ok && mv != "" {
				val = mv
			} else if r < len(rows) && c < len(rows[r]) {
				val = strings.TrimSpace(rows[r][c])
			}

			if val != "" {
				// Deduplicate consecutive identical segments
				if len(parts) == 0 || parts[len(parts)-1] != val {
					parts = append(parts, val)
				}
			}
		}

		if len(parts) == 0 {
			if c < len(leafRow) && strings.TrimSpace(leafRow[c]) != "" {
				combined[c] = strings.TrimSpace(leafRow[c])
			} else {
				combined[c] = fmt.Sprintf("Колонка %d", c+1)
			}
		} else {
			combined[c] = strings.Join(parts, " / ")
		}
	}

	// Disambiguate identical paths (e.g. repeated leaf names under identical parents)
	seen := make(map[string]int, len(combined))
	for i, name := range combined {
		seen[name]++
		if count := seen[name]; count > 1 {
			combined[i] = fmt.Sprintf("%s (%d)", name, count)
		}
	}

	return combined
}

func isPreambleRow(row []string) bool {
	joined := strings.ToLower(strings.Join(row, " "))
	preambleMarkers := []string{"отчёт", "отчет", "реестр", "ведомость", "выгрузка", "сформирован", "период:", "организация:", "структурное подразделение"}
	for _, m := range preambleMarkers {
		if strings.Contains(joined, m) && countNonEmpty(row) <= 3 {
			return true
		}
	}
	return false
}

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
			"участник", "кандидат", "респондент", "пользователь", "mail", "время",
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
