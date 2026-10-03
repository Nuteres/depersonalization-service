package domain

import (
	"dep_service/pkg/formats"
)

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
	TextSnippets           []formats.DocxTextSnippet      `json:"textSnippets,omitempty"`
	ParagraphPreviews      []formats.TextParagraphPreview `json:"paragraphPreviews,omitempty"`
	PDFTextSnippet         string                         `json:"pdfTextSnippet,omitempty"`
	PDFRedactedSnippet     string                         `json:"pdfRedactedSnippet,omitempty"`
}
