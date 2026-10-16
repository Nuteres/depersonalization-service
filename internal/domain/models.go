package domain



// EntityInfo describes a detected sensitive data column or entity.
type EntityInfo struct {
	ColumnIndex int    `json:"columnIndex"`
	ColumnName  string `json:"columnName"`
	Type        string `json:"type"`
	Description string `json:"description"`
	Action      string `json:"action"`
}

// PreviewRow represents a single row showing values before and after anonymization.
type PreviewRow struct {
	Index  int      `json:"index"`
	Before []string `json:"before"`
	After  []string `json:"after"`
}

// SheetInfo represents metadata, detected entities, and preview rows of an individual sheet in a workbook.
type SheetInfo struct {
	Name        string       `json:"name"`
	Index       int          `json:"index"` // 0-based
	RowCount    int          `json:"rowCount"`
	Headers     []string     `json:"headers"`
	Entities    []EntityInfo `json:"entities"`
	PreviewRows []PreviewRow `json:"previewRows"`
}

// FileInfo holds metadata, detected entities, and preview rows for an inspected file.
type FileInfo struct {
	Path                   string                    `json:"path"`
	Name                   string                    `json:"name"`
	Extension              string                    `json:"extension"`
	FileType               string                    `json:"fileType"` // "table", "text", or "pdf"
	Format                 string                    `json:"format"`   // "xlsx", "xlsb", "ods", "xls", "csv", "docx", "odt", "pdf"
	SupportedExportFormats []string                  `json:"supportedExportFormats,omitempty"`
	Size                   int64                     `json:"size"`
	RowCount               int                       `json:"rowCount"`
	ParagraphCount         int                       `json:"paragraphCount,omitempty"`
	PageCount              int                       `json:"pageCount"`
	Entities               []EntityInfo              `json:"entities"`
	Headers                []string                  `json:"headers"`
	PreviewRows            []PreviewRow              `json:"previewRows"`
	Sheets                 []SheetInfo               `json:"sheets,omitempty"`
	ActiveSheet            string                    `json:"activeSheet,omitempty"`
	TextSnippets           []DocxTextSnippet      `json:"textSnippets,omitempty"`
	ParagraphPreviews      []TextParagraphPreview `json:"paragraphPreviews,omitempty"`
	DocumentBlocks         []TextDocumentBlock    `json:"documentBlocks,omitempty"`
	PDFTextSnippet         string                 `json:"pdfTextSnippet,omitempty"`
	PDFRedactedSnippet     string                 `json:"pdfRedactedSnippet,omitempty"`
}

const (
	// MaxInteractiveFileSize is the maximum file size (100 MB) permitted for in-memory document parsing and inspection.
	// Files exceeding this limit are safely rejected to prevent out-of-memory crashes on user systems.
	MaxInteractiveFileSize = 100 * 1024 * 1024 // 100 MB

	// MaxBase64PreviewFileSize is the maximum file size (100 MB) permitted for base64 encoding over IPC.
	MaxBase64PreviewFileSize = 100 * 1024 * 1024 // 100 MB
)

// DocxTextSnippet represents a single detected entity snippet.
type DocxTextSnippet struct {
	Original   string `json:"original"`
	Anonymized string `json:"anonymized"`
	Type       string `json:"type"`
}

// TextParagraphSegment represents a piece of text inside a paragraph, indicating if it was anonymized.
type TextParagraphSegment struct {
	Text        string `json:"text"`
	IsEntity    bool   `json:"isEntity"`
	Type        string `json:"type,omitempty"`
	Original    string `json:"original,omitempty"`
	Anonymized  string `json:"anonymized,omitempty"`
	IsBold      bool   `json:"isBold,omitempty"`
	IsItalic    bool   `json:"isItalic,omitempty"`
	IsUnderline bool   `json:"isUnderline,omitempty"`
	IsStrike    bool   `json:"isStrike,omitempty"`
	IsCode      bool   `json:"isCode,omitempty"`
}

// TextTableCellPreview represents a cell inside a table row for Word-like preview.
type TextTableCellPreview struct {
	Segments  []TextParagraphSegment `json:"segments"`
	Alignment string                 `json:"alignment,omitempty"`
}

// TextTableRowPreview represents a table row for Word-like preview.
type TextTableRowPreview struct {
	IsHeader bool                   `json:"isHeader,omitempty"`
	Cells    []TextTableCellPreview `json:"cells"`
}

// TextTablePreview represents a structured table for Word-like preview.
type TextTablePreview struct {
	Rows []TextTableRowPreview `json:"rows"`
}

// TextDocumentBlock represents either a top-level paragraph or a table in a document.
type TextDocumentBlock struct {
	Type            string                 `json:"type"` // "paragraph" or "table"
	Index           int                    `json:"index"`
	Segments        []TextParagraphSegment `json:"segments,omitempty"`
	Table           *TextTablePreview      `json:"table,omitempty"`
	Alignment       string                 `json:"alignment,omitempty"` // "left", "center", "right", "justify"
	HeadingLevel    int                    `json:"headingLevel,omitempty"`
	FirstLineIndent bool                   `json:"firstLineIndent,omitempty"`
	IsList          bool                   `json:"isList,omitempty"`
	IsEmpty         bool                   `json:"isEmpty,omitempty"`
}

// TextParagraphPreview holds the segmented text of a paragraph for inline Word-like document preview.
type TextParagraphPreview struct {
	Index    int                    `json:"index"`
	Segments []TextParagraphSegment `json:"segments"`
	Table    *TextTablePreview      `json:"table,omitempty"`
}

