package formats

// SheetInfo represents metadata and detected column headers for an individual sheet in a workbook.
type SheetInfo struct {
	Index    int      `json:"index"`
	Name     string   `json:"name"`
	Columns  []string `json:"columns"`
	Headers  []string `json:"headers,omitempty"`
	RowCount int      `json:"rowCount"`
}
