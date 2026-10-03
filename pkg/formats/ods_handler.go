package formats

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"dep_service/pkg/anonymizer"
	"dep_service/pkg/depmap"

	"github.com/xuri/excelize/v2"
)

// ODSSheetInfo is a type alias of SheetInfo for backward compatibility.
type ODSSheetInfo = SheetInfo

// ODSHandler implements processing and inspection for OpenDocument Spreadsheet (.ods) files.
type ODSHandler struct {
	Anon *anonymizer.Anonymizer
}

// NewODSHandler creates a new ODSHandler with the given Anonymizer.
func NewODSHandler(anon *anonymizer.Anonymizer) *ODSHandler {
	return &ODSHandler{Anon: anon}
}

// SupportedExtensions returns file extensions supported by this handler.
func (h *ODSHandler) SupportedExtensions() []string {
	return []string{".ods"}
}

// Inspect examines the sheets in the ODS document, detecting headers and row counts.
func (h *ODSHandler) Inspect(r io.ReaderAt, size int64) ([]ODSSheetInfo, error) {
	if r == nil {
		return nil, errors.New("reader cannot be nil")
	}
	if size <= 0 {
		return nil, errors.New("invalid file size")
	}

	zr, err := zip.NewReader(r, size)
	if err != nil {
		return nil, fmt.Errorf("failed to open ods zip: %w", err)
	}

	contentBytes, err := readZipEntry(zr, "content.xml")
	if err != nil {
		return nil, err
	}

	doc, err := parseODSDoc(contentBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse content.xml: %w", err)
	}

	var sheets []ODSSheetInfo
	for i, s := range doc.sheets {
		// Convert leading rows (up to 40) into sample string rows
		sampleLimit := len(s.rows)
		if sampleLimit > 40 {
			sampleLimit = 40
		}
		sampleRows := make([][]string, sampleLimit)
		for rIdx := 0; rIdx < sampleLimit; rIdx++ {
			sampleRows[rIdx] = rowToStrings(s.rows[rIdx])
		}

		_, headers := FindHeaderRow(sampleRows)
		sheets = append(sheets, ODSSheetInfo{
			Index:    i,
			Name:     s.name,
			Columns:  headers,
			Headers:  headers,
			RowCount: len(s.rows),
		})
	}

	return sheets, nil
}

// InspectRows returns sheet metadata and up to sampleLimit rows per sheet.
func (h *ODSHandler) InspectRows(r io.ReaderAt, size int64, sampleLimit int) ([]ODSSheetInfo, map[string][][]string, error) {
	if r == nil {
		return nil, nil, errors.New("reader cannot be nil")
	}
	if size <= 0 {
		return nil, nil, errors.New("invalid file size")
	}

	zr, err := zip.NewReader(r, size)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open ods zip: %w", err)
	}

	contentBytes, err := readZipEntry(zr, "content.xml")
	if err != nil {
		return nil, nil, err
	}

	doc, err := parseODSDoc(contentBytes)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse content.xml: %w", err)
	}

	if sampleLimit <= 0 {
		sampleLimit = 40
	}

	var sheets []ODSSheetInfo
	samples := make(map[string][][]string)

	for i, s := range doc.sheets {
		limit := len(s.rows)
		if limit > sampleLimit {
			limit = sampleLimit
		}
		sampleRows := make([][]string, limit)
		for rIdx := 0; rIdx < limit; rIdx++ {
			sampleRows[rIdx] = rowToStrings(s.rows[rIdx])
		}
		samples[s.name] = sampleRows

		_, headers := FindHeaderRow(sampleRows)
		sheets = append(sheets, ODSSheetInfo{
			Index:    i,
			Name:     s.name,
			Columns:  headers,
			Headers:  headers,
			RowCount: len(s.rows),
		})
	}

	return sheets, samples, nil
}

// Process reads sheets and rows from r, anonymizes detected PII columns,
// writes the output ODS archive to w, and returns the populated DepMap.
func (h *ODSHandler) Process(ctx context.Context, r io.ReaderAt, size int64, w io.Writer, options map[string]interface{}) (*depmap.DepMap, error) {
	if h.Anon == nil {
		return nil, fmt.Errorf("anonymizer is not initialized")
	}
	if r == nil {
		return nil, errors.New("reader cannot be nil")
	}
	if w == nil {
		return nil, errors.New("writer cannot be nil")
	}
	if size <= 0 {
		return nil, errors.New("invalid file size")
	}

	if ctx == nil {
		ctx = context.Background()
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	zr, err := zip.NewReader(r, size)
	if err != nil {
		return nil, fmt.Errorf("failed to open source ods zip: %w", err)
	}

	contentBytes, err := readZipEntry(zr, "content.xml")
	if err != nil {
		return nil, err
	}

	doc, err := parseODSDoc(contentBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse content.xml: %w", err)
	}

	origFileName := "document.ods"
	if options != nil {
		if fn, ok := options["filename"].(string); ok && fn != "" {
			origFileName = fn
		} else if fn, ok := options["file_name"].(string); ok && fn != "" {
			origFileName = fn
		}
	}
	dm := depmap.New(origFileName)

	var targetSheet string
	if options != nil {
		if s, ok := options["sheet"].(string); ok {
			targetSheet = s
		}
	}

	dateRange := getDateRange(options)

	var progressFn func(int)
	if options != nil {
		if cb, ok := options["progress_callback"].(func(int)); ok {
			progressFn = cb
		}
	}

	var replacements []textReplacement
	processedRows := 0

	for _, sheet := range doc.sheets {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		if targetSheet != "" && sheet.name != targetSheet {
			continue
		}

		// Sample leading rows to find the header row
		sampleLimit := len(sheet.rows)
		if sampleLimit > 40 {
			sampleLimit = 40
		}
		sampleRows := make([][]string, sampleLimit)
		for rIdx := 0; rIdx < sampleLimit; rIdx++ {
			sampleRows[rIdx] = rowToStrings(sheet.rows[rIdx])
		}

		headerRowIndex, headers := FindHeaderRow(sampleRows)
		colTypes := ResolveColumnTypesWithSamples(headers, sampleRows[headerRowIndex+1:], options)

		// Build map of disabled columns (1-based index)
		disabledMap := make(map[int]bool)
		hasSheetSpecific := false
		if options != nil {
			if bySheet, ok := options["disabled_columns_by_sheet"].(map[string][]int); ok && len(bySheet) > 0 {
				if dList, found := bySheet[sheet.name]; found {
					hasSheetSpecific = true
					for _, colIdx := range dList {
						disabledMap[colIdx] = true
					}
				}
			} else if bySheetIntf, ok := options["disabled_columns_by_sheet"].(map[string]interface{}); ok && len(bySheetIntf) > 0 {
				if sheetVal, found := bySheetIntf[sheet.name]; found {
					hasSheetSpecific = true
					if dList, ok := sheetVal.([]int); ok {
						for _, colIdx := range dList {
							disabledMap[colIdx] = true
						}
					} else if dInterface, ok := sheetVal.([]interface{}); ok {
						for _, itm := range dInterface {
							if n, ok := itm.(int); ok {
								disabledMap[n] = true
							} else if f, ok := itm.(float64); ok {
								disabledMap[int(f)] = true
							}
						}
					}
				}
			}

			if !hasSheetSpecific {
				if dList, ok := options["disabled_columns"].([]int); ok {
					for _, colIdx := range dList {
						disabledMap[colIdx] = true
					}
				} else if dInterface, ok := options["disabled_columns"].([]interface{}); ok {
					for _, itm := range dInterface {
						if n, ok := itm.(int); ok {
							disabledMap[n] = true
						} else if f, ok := itm.(float64); ok {
							disabledMap[int(f)] = true
						}
					}
				}
			}
		}

		for rIdx, row := range sheet.rows {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
			}

			processedRows++
			if progressFn != nil && processedRows%50 == 0 {
				progressFn(processedRows)
			}

			// Skip preamble rows and header row
			if rIdx <= headerRowIndex {
				continue
			}

			// Data row
			for _, cell := range row.cells {
				col1Based := cell.colIdx + 1
				if cell.colIdx >= len(colTypes) || disabledMap[col1Based] {
					continue
				}

				colType := colTypes[cell.colIdx]
				val := cell.fullText
				if strings.TrimSpace(val) == "" {
					continue
				}

				synthetic := AnonymizeCell(h.Anon, colType, val, dateRange)
				if synthetic == val {
					continue
				}

				dm.Put(synthetic, val)

				// Queue replacement for paragraphs
				if len(cell.paragraphs) > 0 {
					p := cell.paragraphs[0]
					var buf bytes.Buffer
					xml.EscapeText(&buf, []byte(synthetic))
					replacements = append(replacements, textReplacement{
						start:   p.startOffset,
						end:     p.endOffset,
						newText: buf.Bytes(),
					})
					for _, extraP := range cell.paragraphs[1:] {
						replacements = append(replacements, textReplacement{
							start:   extraP.startOffset,
							end:     extraP.endOffset,
							newText: nil,
						})
					}
				}

				// If cell start tag contained office:string-value or office:date-value, update attributes
				if len(cell.tagBytes) > 0 {
					newTag := cell.tagBytes
					tagModified := false
					tagStr := string(cell.tagBytes)

					if strings.Contains(tagStr, "office:string-value") {
						oldAttr := extractAttrValue(cell.tagBytes, "office:string-value")
						if oldAttr != "" {
							newTag = updateAttrInTag(newTag, "office:string-value", oldAttr, synthetic)
							tagModified = true
						}
					}

					if strings.Contains(tagStr, "office:date-value") {
						oldDateAttr := extractAttrValue(cell.tagBytes, "office:date-value")
						if oldDateAttr != "" {
							synthDateAttr := h.Anon.PerturbDate(oldDateAttr, dateRange)
							if synthDateAttr != oldDateAttr {
								dm.Put(synthDateAttr, oldDateAttr)
								newTag = updateAttrInTag(newTag, "office:date-value", oldDateAttr, synthDateAttr)
								tagModified = true
							}
						}
					}

					if tagModified {
						replacements = append(replacements, textReplacement{
							start:   cell.startOffset,
							end:     cell.startOffset + len(cell.tagBytes),
							newText: newTag,
						})
					}
				}
			}
		}
	}

	// Apply replacements to content.xml
	newContent := applyReplacements(contentBytes, replacements)

	// Repack output zip
	if err := repackODSZip(zr, w, newContent); err != nil {
		return nil, fmt.Errorf("failed to repack ods zip: %w", err)
	}

	return dm, nil
}

// --- Internal Helper Types & Functions ---

type textReplacement struct {
	start   int
	end     int
	newText []byte
}

type odsTextP struct {
	startOffset int
	endOffset   int
	text        string
}

type odsCell struct {
	colIdx      int
	startOffset int
	endOffset   int
	tagBytes    []byte
	paragraphs  []odsTextP
	fullText    string
}

type odsRow struct {
	rowIdx int
	cells  []odsCell
}

type odsSheet struct {
	name string
	rows []odsRow
}

type odsDoc struct {
	sheets []odsSheet
}

func parseODSDoc(xmlData []byte) (*odsDoc, error) {
	dec := xml.NewDecoder(bytes.NewReader(xmlData))
	doc := &odsDoc{}

	var curSheet *odsSheet
	var curRow *odsRow
	var curCell *odsCell
	var pTextBuf bytes.Buffer
	var pStartOffset int
	pDepth := 0
	cellColIdx := 0

	for {
		offsetBefore := dec.InputOffset()
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("xml decode error: %w", err)
		}
		offsetAfter := dec.InputOffset()

		switch t := tok.(type) {
		case xml.StartElement:
			local := t.Name.Local
			isTable := local == "table" && (t.Name.Space == "" || strings.Contains(t.Name.Space, "table"))
			isRow := local == "table-row" && (t.Name.Space == "" || strings.Contains(t.Name.Space, "table"))
			isCell := (local == "table-cell" || local == "covered-table-cell") && (t.Name.Space == "" || strings.Contains(t.Name.Space, "table"))
			isP := local == "p" && (t.Name.Space == "" || strings.Contains(t.Name.Space, "text"))

			if isTable {
				tableName := ""
				for _, a := range t.Attr {
					if a.Name.Local == "name" {
						tableName = a.Value
						break
					}
				}
				if tableName == "" {
					tableName = fmt.Sprintf("Sheet%d", len(doc.sheets)+1)
				}
				doc.sheets = append(doc.sheets, odsSheet{name: tableName})
				curSheet = &doc.sheets[len(doc.sheets)-1]
			} else if isRow && curSheet != nil {
				curSheet.rows = append(curSheet.rows, odsRow{rowIdx: len(curSheet.rows)})
				curRow = &curSheet.rows[len(curSheet.rows)-1]
				cellColIdx = 0
			} else if isCell && curRow != nil {
				startTagStart := int(offsetBefore)
				startTagEnd := int(offsetAfter)
				var tagBytes []byte
				if startTagStart >= 0 && startTagEnd <= len(xmlData) && startTagStart <= startTagEnd {
					tagBytes = xmlData[startTagStart:startTagEnd]
				}

				curCell = &odsCell{
					colIdx:      cellColIdx,
					startOffset: startTagStart,
					endOffset:   startTagEnd,
					tagBytes:    tagBytes,
				}
			} else if isP && curCell != nil {
				if pDepth == 0 {
					pStartOffset = int(offsetAfter)
					pTextBuf.Reset()
				}
				pDepth++
			} else if curCell != nil && pDepth > 0 {
				pDepth++
			}

		case xml.CharData:
			if curCell != nil && pDepth > 0 {
				pTextBuf.Write(t)
			}

		case xml.EndElement:
			local := t.Name.Local
			isTable := local == "table" && (t.Name.Space == "" || strings.Contains(t.Name.Space, "table"))
			isRow := local == "table-row" && (t.Name.Space == "" || strings.Contains(t.Name.Space, "table"))
			isCell := (local == "table-cell" || local == "covered-table-cell") && (t.Name.Space == "" || strings.Contains(t.Name.Space, "table"))
			isP := local == "p" && (t.Name.Space == "" || strings.Contains(t.Name.Space, "text"))

			if isP && curCell != nil {
				pDepth--
				if pDepth == 0 {
					pEndOffset := int(offsetBefore)
					pText := pTextBuf.String()
					curCell.paragraphs = append(curCell.paragraphs, odsTextP{
						startOffset: pStartOffset,
						endOffset:   pEndOffset,
						text:        pText,
					})
					if curCell.fullText == "" {
						curCell.fullText = pText
					} else {
						curCell.fullText += "\n" + pText
					}
					pTextBuf.Reset()
				}
			} else if curCell != nil && pDepth > 0 {
				pDepth--
			} else if isCell && curRow != nil && curCell != nil {
				curCell.endOffset = int(offsetAfter)
				if curCell.fullText == "" && len(curCell.paragraphs) == 0 {
					if sv := extractAttrValue(curCell.tagBytes, "office:string-value"); sv != "" {
						curCell.fullText = sv
					} else if dv := extractAttrValue(curCell.tagBytes, "office:date-value"); dv != "" {
						curCell.fullText = dv
					} else if v := extractAttrValue(curCell.tagBytes, "office:value"); v != "" {
						curCell.fullText = v
					}
				}
				repeat := extractColumnsRepeated(curCell.tagBytes)
				curRow.cells = append(curRow.cells, *curCell)
				cellColIdx += repeat
				curCell = nil
			} else if isRow {
				curRow = nil
			} else if isTable {
				curSheet = nil
			}
		}
	}

	return doc, nil
}

func rowToStrings(row odsRow) []string {
	if len(row.cells) == 0 {
		return nil
	}
	maxCol := 0
	for _, c := range row.cells {
		if c.colIdx > maxCol {
			maxCol = c.colIdx
		}
	}
	res := make([]string, maxCol+1)
	for _, c := range row.cells {
		if c.colIdx < len(res) {
			res[c.colIdx] = c.fullText
		}
	}
	return res
}

func readZipEntry(zr *zip.Reader, name string) ([]byte, error) {
	for _, f := range zr.File {
		if f.Name == name {
			rc, err := f.Open()
			if err != nil {
				return nil, fmt.Errorf("failed to open %s: %w", name, err)
			}
			defer rc.Close()
			return io.ReadAll(rc)
		}
	}
	return nil, fmt.Errorf("%s not found in ods archive", name)
}

func repackODSZip(zr *zip.Reader, w io.Writer, updatedContent []byte) error {
	zw := zip.NewWriter(w)

	// 1. Write mimetype first uncompressed (zip.Store)
	var mimeContent []byte
	var mimeHeader *zip.FileHeader
	for _, f := range zr.File {
		if f.Name == "mimetype" {
			rc, err := f.Open()
			if err == nil {
				mimeContent, _ = io.ReadAll(rc)
				rc.Close()
			}
			mimeHeader = &f.FileHeader
			break
		}
	}
	if len(mimeContent) == 0 {
		mimeContent = []byte("application/vnd.oasis.opendocument.spreadsheet")
	}

	h := &zip.FileHeader{
		Name:   "mimetype",
		Method: zip.Store,
	}
	if mimeHeader != nil && mimeHeader.Comment != "" {
		h.Comment = mimeHeader.Comment
	}
	mw, err := zw.CreateHeader(h)
	if err != nil {
		return fmt.Errorf("failed to create mimetype in zip: %w", err)
	}
	if _, err := mw.Write(mimeContent); err != nil {
		return fmt.Errorf("failed to write mimetype: %w", err)
	}

	// 2. Write content.xml (zip.Deflate)
	cw, err := zw.CreateHeader(&zip.FileHeader{
		Name:   "content.xml",
		Method: zip.Deflate,
	})
	if err != nil {
		return fmt.Errorf("failed to create content.xml in zip: %w", err)
	}
	if _, err := cw.Write(updatedContent); err != nil {
		return fmt.Errorf("failed to write content.xml: %w", err)
	}

	// 3. Write all other entries intact
	for _, f := range zr.File {
		if f.Name == "mimetype" || f.Name == "content.xml" {
			continue
		}

		fw, err := zw.CreateHeader(&zip.FileHeader{
			Name:   f.Name,
			Method: f.Method,
		})
		if err != nil {
			return fmt.Errorf("failed to create entry %s: %w", f.Name, err)
		}
		rc, err := f.Open()
		if err != nil {
			return fmt.Errorf("failed to open entry %s: %w", f.Name, err)
		}
		_, err = io.Copy(fw, rc)
		rc.Close()
		if err != nil {
			return fmt.Errorf("failed to copy entry %s: %w", f.Name, err)
		}
	}

	return zw.Close()
}

func applyReplacements(src []byte, reps []textReplacement) []byte {
	if len(reps) == 0 {
		return src
	}

	// Sort by start offset ascending
	sort.Slice(reps, func(i, j int) bool {
		return reps[i].start < reps[j].start
	})

	var out bytes.Buffer
	out.Grow(len(src))

	lastOffset := 0
	for _, r := range reps {
		if r.start < lastOffset {
			continue
		}
		if r.start > len(src) || r.end > len(src) || r.start > r.end {
			continue
		}
		out.Write(src[lastOffset:r.start])
		if len(r.newText) > 0 {
			out.Write(r.newText)
		}
		lastOffset = r.end
	}
	if lastOffset < len(src) {
		out.Write(src[lastOffset:])
	}

	return out.Bytes()
}

func extractColumnsRepeated(tagBytes []byte) int {
	s := string(tagBytes)
	idx := strings.Index(s, "number-columns-repeated=")
	if idx == -1 {
		return 1
	}
	rest := s[idx+len("number-columns-repeated="):]
	if len(rest) == 0 {
		return 1
	}
	quote := rest[0]
	if quote != '"' && quote != '\'' {
		return 1
	}
	rest = rest[1:]
	endIdx := strings.IndexByte(rest, quote)
	if endIdx == -1 {
		return 1
	}
	if n, err := strconv.Atoi(rest[:endIdx]); err == nil && n > 1 {
		return n
	}
	return 1
}

func extractAttrValue(tagBytes []byte, attrName string) string {
	s := string(tagBytes)
	idx := strings.Index(s, attrName+"=")
	if idx == -1 {
		return ""
	}
	rest := s[idx+len(attrName+"="):]
	if len(rest) == 0 {
		return ""
	}
	quote := rest[0]
	if quote != '"' && quote != '\'' {
		return ""
	}
	rest = rest[1:]
	endIdx := strings.IndexByte(rest, quote)
	if endIdx == -1 {
		return ""
	}
	return rest[:endIdx]
}

func xmlEscapeAttr(s string) string {
	var buf bytes.Buffer
	for _, r := range s {
		switch r {
		case '&':
			buf.WriteString("&amp;")
		case '"':
			buf.WriteString("&quot;")
		case '\'':
			buf.WriteString("&apos;")
		case '<':
			buf.WriteString("&lt;")
		case '>':
			buf.WriteString("&gt;")
		default:
			buf.WriteRune(r)
		}
	}
	return buf.String()
}

func updateAttrInTag(tag []byte, attrName, oldVal, newVal string) []byte {
	escapedOld := xmlEscapeAttr(oldVal)
	escapedNew := xmlEscapeAttr(newVal)

	// Try double quotes: attr="escapedOld" -> attr="escapedNew"
	needleDbl := fmt.Sprintf("%s=\"%s\"", attrName, escapedOld)
	replacementDbl := fmt.Sprintf("%s=\"%s\"", attrName, escapedNew)
	res := bytes.Replace(tag, []byte(needleDbl), []byte(replacementDbl), 1)
	if !bytes.Equal(res, tag) {
		return res
	}

	// Try unescaped old in double quotes
	needleDblRaw := fmt.Sprintf("%s=\"%s\"", attrName, oldVal)
	res = bytes.Replace(tag, []byte(needleDblRaw), []byte(replacementDbl), 1)
	if !bytes.Equal(res, tag) {
		return res
	}

	// Try single quotes: attr='escapedOld' -> attr="escapedNew"
	needleSgl := fmt.Sprintf("%s='%s'", attrName, escapedOld)
	res = bytes.Replace(tag, []byte(needleSgl), []byte(replacementDbl), 1)
	if !bytes.Equal(res, tag) {
		return res
	}

	// Try unescaped old in single quotes
	needleSglRaw := fmt.Sprintf("%s='%s'", attrName, oldVal)
	res = bytes.Replace(tag, []byte(needleSglRaw), []byte(replacementDbl), 1)
	return res
}

// ProcessToXLSX reads sheets from an ODS archive at r/size, anonymizes sensitive PII cells,
// and streams the result into an Excel (.xlsx) file written to w, returning a populated DepMap.
func (h *ODSHandler) ProcessToXLSX(ctx context.Context, r io.ReaderAt, size int64, w io.Writer, options map[string]interface{}) (*depmap.DepMap, error) {
	if h.Anon == nil {
		return nil, fmt.Errorf("anonymizer is not initialized")
	}
	if r == nil {
		return nil, errors.New("reader cannot be nil")
	}
	if w == nil {
		return nil, errors.New("writer cannot be nil")
	}
	if size <= 0 {
		return nil, errors.New("invalid file size")
	}

	if ctx == nil {
		ctx = context.Background()
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	zr, err := zip.NewReader(r, size)
	if err != nil {
		return nil, fmt.Errorf("failed to open source ods zip: %w", err)
	}

	contentBytes, err := readZipEntry(zr, "content.xml")
	if err != nil {
		return nil, err
	}

	doc, err := parseODSDoc(contentBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse content.xml: %w", err)
	}

	origFileName := "document.ods"
	if options != nil {
		if fn, ok := options["filename"].(string); ok && fn != "" {
			origFileName = fn
		} else if fn, ok := options["file_name"].(string); ok && fn != "" {
			origFileName = fn
		}
	}
	dm := depmap.New(origFileName)

	var targetSheet string
	if options != nil {
		if s, ok := options["sheet"].(string); ok {
			targetSheet = s
		}
	}

	dateRange := getDateRange(options)

	var progressFn func(int)
	if options != nil {
		if cb, ok := options["progress_callback"].(func(int)); ok {
			progressFn = cb
		}
	}

	outFile := excelize.NewFile()
	defer outFile.Close()

	totalProcessedRows := 0

	for sheetIdx, sheet := range doc.sheets {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		if targetSheet != "" && sheet.name != targetSheet {
			continue
		}

		sheetName := sheet.name
		if sheetIdx == 0 {
			outFile.SetSheetName("Sheet1", sheetName)
		} else {
			outFile.NewSheet(sheetName)
		}

		// Sample leading rows to find the header row
		sampleLimit := len(sheet.rows)
		if sampleLimit > 40 {
			sampleLimit = 40
		}
		sampleRows := make([][]string, sampleLimit)
		for rIdx := 0; rIdx < sampleLimit; rIdx++ {
			sampleRows[rIdx] = rowToStrings(sheet.rows[rIdx])
		}

		headerRowIndex, headers := FindHeaderRow(sampleRows)
		colTypes := ResolveColumnTypesWithSamples(headers, sampleRows[headerRowIndex+1:], options)

		// Build map of disabled columns (1-based index)
		disabledMap := make(map[int]bool)
		hasSheetSpecific := false
		if options != nil {
			if bySheet, ok := options["disabled_columns_by_sheet"].(map[string][]int); ok && len(bySheet) > 0 {
				if dList, found := bySheet[sheet.name]; found {
					hasSheetSpecific = true
					for _, colIdx := range dList {
						disabledMap[colIdx] = true
					}
				}
			}
			if !hasSheetSpecific {
				if dList, ok := options["disabled_columns"].([]int); ok {
					for _, colIdx := range dList {
						disabledMap[colIdx] = true
					}
				}
			}
		}

		sw, err := outFile.NewStreamWriter(sheetName)
		if err != nil {
			return nil, fmt.Errorf("failed to create stream writer for sheet %s: %w", sheetName, err)
		}

		for rIdx, row := range sheet.rows {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
			}

			rowValues := rowToStrings(row)
			rowCells := make([]interface{}, len(rowValues))

			for cIdx, val := range rowValues {
				if rIdx <= headerRowIndex {
					rowCells[cIdx] = val
					continue
				}

				col1Based := cIdx + 1
				if disabledMap[col1Based] {
					rowCells[cIdx] = val
					continue
				}

				colType := ColUnknown
				if cIdx < len(colTypes) {
					colType = colTypes[cIdx]
				}

				if colType == ColUnknown {
					rowCells[cIdx] = val
					continue
				}

				anonymized := AnonymizeCell(h.Anon, colType, val, dateRange)
				if anonymized != val && strings.TrimSpace(val) != "" {
					dm.Put(anonymized, val)
				}
				rowCells[cIdx] = anonymized
			}

			cellCoord, err := excelize.CoordinatesToCellName(1, rIdx+1)
			if err != nil {
				return nil, fmt.Errorf("invalid coordinate at row %d: %w", rIdx+1, err)
			}
			if err := sw.SetRow(cellCoord, rowCells); err != nil {
				return nil, fmt.Errorf("failed to stream row %d: %w", rIdx+1, err)
			}

			totalProcessedRows++
			if progressFn != nil && totalProcessedRows%100 == 0 {
				progressFn(totalProcessedRows)
			}
		}

		if err := sw.Flush(); err != nil {
			return nil, fmt.Errorf("failed to flush sheet %s: %w", sheetName, err)
		}
	}

	if progressFn != nil {
		progressFn(totalProcessedRows)
	}

	if err := outFile.Write(w); err != nil {
		return nil, fmt.Errorf("failed to write xlsx output: %w", err)
	}

	return dm, nil
}

