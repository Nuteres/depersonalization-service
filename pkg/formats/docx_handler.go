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
	"strings"
	"unicode/utf8"

	"dep_service/internal/domain"
	"dep_service/pkg/anonymizer"
	"dep_service/pkg/depmap"
	"dep_service/pkg/detector"
)

// EntityCount tracks the number of detected entities of a specific type.
type EntityCount struct {
	Type  string `json:"type"`
	Count int    `json:"count"`
}

// DocxTextSnippet represents a single detected entity snippet.
type DocxTextSnippet = domain.DocxTextSnippet

// TextParagraphSegment represents a piece of text inside a paragraph, indicating if it was anonymized.
type TextParagraphSegment = domain.TextParagraphSegment

// TextTableCellPreview represents a cell inside a table row for Word-like preview.
type TextTableCellPreview = domain.TextTableCellPreview

// TextTableRowPreview represents a table row for Word-like preview.
type TextTableRowPreview = domain.TextTableRowPreview

// TextTablePreview represents a structured table for Word-like preview.
type TextTablePreview = domain.TextTablePreview

// TextDocumentBlock represents either a top-level paragraph or a table in a document.
type TextDocumentBlock = domain.TextDocumentBlock

// TextParagraphPreview holds the segmented text of a paragraph for inline Word-like document preview.
type TextParagraphPreview = domain.TextParagraphPreview

// DOCXHandler handles inspection and anonymization of Microsoft Word (.docx) documents.
type DOCXHandler struct {
	Anon *anonymizer.Anonymizer
}

// NewDOCXHandler creates a new DOCXHandler with the given Anonymizer.
func NewDOCXHandler(anon *anonymizer.Anonymizer) *DOCXHandler {
	return &DOCXHandler{Anon: anon}
}

// SupportedExtensions returns file extensions supported by this handler.
func (h *DOCXHandler) SupportedExtensions() []string {
	return []string{".docx"}
}

type docxMatch struct {
	start      int // byte offset in string
	end        int // byte offset in string
	value      string
	entityType string
}

// findDocxSensitiveMatches locates sensitive entities in paragraph text.
func findDocxSensitiveMatches(text string) []docxMatch {
	tm := detector.FindTextMatches(text)
	if len(tm) == 0 {
		return nil
	}
	res := make([]docxMatch, len(tm))
	for i, m := range tm {
		res[i] = docxMatch{
			start:      m.Start,
			end:        m.End,
			value:      m.Value,
			entityType: string(m.EntityType),
		}
	}
	return res
}

func generateSynthetic(anon *anonymizer.Anonymizer, entityType string, original string) string {
	if anon == nil {
		return fmt.Sprintf("[%s]", entityType)
	}
	switch entityType {
	case string(detector.EntityFIO), "NAME", string(detector.EntityLastName), string(detector.EntityFirstName), string(detector.EntityPatronymic):
		return anon.GenerateFIO(original)
	case string(detector.EntityPassport):
		if len(detector.CleanDigits(original)) == 6 && strings.Contains(original, "-") {
			return anon.GeneratePassportDeptCode(original)
		}
		return anon.GeneratePassport(original)
	case string(detector.EntityPhone):
		return anon.GeneratePhone(original)
	case string(detector.EntitySNILS):
		return anon.GenerateSNILS(original)
	case string(detector.EntityINN):
		return anon.GenerateINN12(original)
	case string(detector.EntityCard), "CARD":
		if len(detector.CleanDigits(original)) == 20 {
			return anon.GenerateBankAccount(original)
		}
		return anon.GenerateCardMIR(original)
	case string(detector.EntityEmail):
		return anon.GenerateEmail(original)
	case string(detector.EntityBirthday), "DATE":
		return anon.PerturbDate(original, 30)
	case string(detector.EntityAddress):
		return anon.GenerateAddress(original)
	default:
		return anon.GenerateFIO(original)
	}
}

func isWordElement(name xml.Name, local string) bool {
	if name.Local != local {
		return false
	}
	return name.Space == "" || strings.Contains(name.Space, "wordprocessingml")
}

func getDocxFileOrder(name string) (order int, isBodyOrNotes bool) {
	switch name {
	case "word/document.xml":
		return 1, true
	case "word/footnotes.xml":
		return 2, true
	case "word/endnotes.xml":
		return 3, true
	case "word/comments.xml":
		return 4, false
	default:
		if strings.HasPrefix(name, "word/header") && strings.HasSuffix(name, ".xml") {
			return 5, false
		}
		if strings.HasPrefix(name, "word/footer") && strings.HasSuffix(name, ".xml") {
			return 6, false
		}
		return 100, false
	}
}

func isDocumentXML(name string) bool {
	if name == "word/document.xml" || name == "word/footnotes.xml" || name == "word/endnotes.xml" || name == "word/comments.xml" {
		return true
	}
	if strings.HasPrefix(name, "word/header") && strings.HasSuffix(name, ".xml") {
		return true
	}
	if strings.HasPrefix(name, "word/footer") && strings.HasSuffix(name, ".xml") {
		return true
	}
	return false
}

type docxTextNode struct {
	startOffset int // byte offset of '<' in <w:t ...>
	endOffset   int // byte offset after '>' in </w:t>
	text        string
	isBold      bool
	isItalic    bool
	isUnderline bool
	isStrike    bool
	isCode      bool
}

type docxParagraph struct {
	nodes           []docxTextNode
	isHeading       bool
	headingLevel    int
	alignment       string // "left", "center", "right", "justify"
	firstLineIndent bool
	isList          bool
	isEmpty         bool
}

func isDocxHeadingText(str string) bool {
	if len(str) == 0 || len(str) > 130 {
		return false
	}
	if len(str) >= 4 && str[0] >= '1' && str[0] <= '9' && str[1] == '.' && str[2] == ' ' {
		if !strings.HasSuffix(str, ".") {
			return true
		}
	}
	upper := strings.ToUpper(str)
	if strings.HasPrefix(upper, "АКТ ") ||
		strings.HasPrefix(upper, "ТРУДОВОЙ ДОГОВОР") ||
		strings.HasPrefix(upper, "СОГЛАСИЕ ") ||
		upper == "ЗАЯВЛЕНИЕ" ||
		strings.HasPrefix(upper, "СВЕДЕНИЯ О ") ||
		strings.HasPrefix(upper, "РЕКВИЗИТЫ ") ||
		strings.HasPrefix(upper, "РАСЧЕТ ЗАЯВЛЯЕМОЙ СУММЫ") ||
		strings.HasPrefix(upper, "СПЕЦИФИКАЦИЯ") {
		return true
	}
	return false
}

type docxCell struct {
	paragraphs []docxParagraph
	alignment  string
}

type docxRow struct {
	cells    []docxCell
	isHeader bool
}

type docxTable struct {
	rows []docxRow
}

type docxBlockType int

const (
	docxBlockTypeParagraph docxBlockType = iota
	docxBlockTypeHeading
	docxBlockTypeTable
)

type docxBlock struct {
	blockType docxBlockType
	paragraph docxParagraph
	table     docxTable
}

func getAttrVal(attrs []xml.Attr, local string) string {
	for _, a := range attrs {
		if a.Name.Local == local {
			return a.Value
		}
	}
	return ""
}

func isPropertyTrue(attrs []xml.Attr) bool {
	val := strings.ToLower(strings.TrimSpace(getAttrVal(attrs, "val")))
	if val == "" {
		return true // presence without val implies true in WordprocessingML, e.g. <w:b/>
	}
	return val != "0" && val != "false" && val != "none" && val != "off"
}

func isUnderlineTrue(attrs []xml.Attr) bool {
	val := strings.ToLower(strings.TrimSpace(getAttrVal(attrs, "val")))
	if val == "" {
		return true
	}
	return val != "none" && val != "0" && val != "false" && val != "off"
}

func getAlignmentVal(attrs []xml.Attr) string {
	val := strings.ToLower(strings.TrimSpace(getAttrVal(attrs, "val")))
	switch val {
	case "center":
		return "center"
	case "right", "end":
		return "right"
	case "both", "distribute", "justify":
		return "justify"
	case "left", "start":
		return "left"
	default:
		return ""
	}
}

func hasFirstLineIndent(attrs []xml.Attr) bool {
	val := strings.TrimSpace(getAttrVal(attrs, "firstLine"))
	return val != "" && val != "0"
}

func parseDocxXML(xmlData []byte) ([]docxBlock, error) {
	dec := xml.NewDecoder(bytes.NewReader(xmlData))
	var blocks []docxBlock
	var curNodes []docxTextNode
	curPIsHeading := false
	curPHeadingLevel := 0
	curPAlign := ""
	curPFirstLineIndent := false
	curPIsList := false

	rDepth := 0
	curRBold := false
	curRItalic := false
	curRUnderline := false
	curRStrike := false
	curRCode := false

	pDepth := 0
	inT := false
	var curTText strings.Builder
	var tStartOffset int

	tblDepth := 0
	trDepth := 0
	tcDepth := 0
	curCellAlign := ""

	var curTable docxTable
	var curRow docxRow
	var curCell docxCell

	var curNoteId string
	inSpecialNote := false

	for {
		offsetBefore := int(dec.InputOffset())
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("xml decode error: %w", err)
		}
		offsetAfter := int(dec.InputOffset())

		switch t := tok.(type) {
		case xml.StartElement:
			if isWordElement(t.Name, "endnote") || isWordElement(t.Name, "footnote") {
				var noteId, noteType string
				for _, attr := range t.Attr {
					if attr.Name.Local == "id" {
						noteId = attr.Value
					} else if attr.Name.Local == "type" {
						noteType = attr.Value
					}
				}
				if noteType == "separator" || noteType == "continuationSeparator" || noteId == "-1" || noteId == "0" {
					inSpecialNote = true
				} else {
					inSpecialNote = false
					curNoteId = noteId
				}
			} else if isWordElement(t.Name, "tbl") {
				if tblDepth == 0 {
					curTable = docxTable{}
				}
				tblDepth++
			} else if isWordElement(t.Name, "tr") {
				if tblDepth > 0 && trDepth == 0 {
					curRow = docxRow{}
				}
				trDepth++
			} else if isWordElement(t.Name, "tblHeader") && trDepth > 0 {
				curRow.isHeader = true
			} else if isWordElement(t.Name, "tc") {
				if tblDepth > 0 && trDepth > 0 && tcDepth == 0 {
					curCell = docxCell{}
					curCellAlign = ""
				}
				tcDepth++
			} else if tcDepth > 0 && pDepth == 0 && isWordElement(t.Name, "jc") {
				if a := getAlignmentVal(t.Attr); a != "" {
					curCellAlign = a
				}
			} else if isWordElement(t.Name, "p") {
				if pDepth == 0 {
					curNodes = nil
					curPIsHeading = false
					curPHeadingLevel = 0
					curPAlign = ""
					curPFirstLineIndent = false
					curPIsList = false
				}
				pDepth++
			} else if pDepth > 0 && isWordElement(t.Name, "pStyle") {
				valLower := strings.ToLower(getAttrVal(t.Attr, "val"))
				if strings.Contains(valLower, "heading") || strings.Contains(valLower, "title") || strings.Contains(valLower, "заголовок") {
					curPIsHeading = true
					if strings.Contains(valLower, "1") {
						curPHeadingLevel = 1
					} else if strings.Contains(valLower, "2") {
						curPHeadingLevel = 2
					} else if strings.Contains(valLower, "3") {
						curPHeadingLevel = 3
					} else {
						curPHeadingLevel = 1
					}
				}
			} else if pDepth > 0 && rDepth == 0 && isWordElement(t.Name, "jc") {
				if a := getAlignmentVal(t.Attr); a != "" {
					curPAlign = a
				}
			} else if pDepth > 0 && rDepth == 0 && isWordElement(t.Name, "ind") {
				if hasFirstLineIndent(t.Attr) {
					curPFirstLineIndent = true
				}
			} else if pDepth > 0 && rDepth == 0 && isWordElement(t.Name, "numPr") {
				curPIsList = true
			} else if pDepth > 0 && isWordElement(t.Name, "r") {
				if rDepth == 0 {
					curRBold = false
					curRItalic = false
					curRUnderline = false
					curRStrike = false
					curRCode = false
				}
				rDepth++
			} else if pDepth > 0 && rDepth > 0 && isWordElement(t.Name, "b") {
				curRBold = isPropertyTrue(t.Attr)
			} else if pDepth > 0 && rDepth > 0 && isWordElement(t.Name, "bCs") {
				curRBold = true
			} else if pDepth > 0 && rDepth > 0 && isWordElement(t.Name, "i") {
				curRItalic = isPropertyTrue(t.Attr)
			} else if pDepth > 0 && rDepth > 0 && isWordElement(t.Name, "iCs") {
				curRItalic = true
			} else if pDepth > 0 && rDepth > 0 && isWordElement(t.Name, "u") {
				curRUnderline = isUnderlineTrue(t.Attr)
			} else if pDepth > 0 && rDepth > 0 && (isWordElement(t.Name, "strike") || isWordElement(t.Name, "dstrike")) {
				curRStrike = isPropertyTrue(t.Attr)
			} else if pDepth > 0 && rDepth > 0 && isWordElement(t.Name, "rFonts") {
				for _, a := range t.Attr {
					if a.Name.Local == "ascii" || a.Name.Local == "hAnsi" || a.Name.Local == "cs" {
						fontLower := strings.ToLower(a.Value)
						if strings.Contains(fontLower, "courier") || strings.Contains(fontLower, "consolas") || strings.Contains(fontLower, "mono") {
							curRCode = true
						}
					}
				}
			} else if pDepth > 0 && isWordElement(t.Name, "t") {
				inT = true
				tStartOffset = offsetBefore
				curTText.Reset()
			} else if pDepth > 0 && (isWordElement(t.Name, "br") || isWordElement(t.Name, "cr")) {
				curNodes = append(curNodes, docxTextNode{
					startOffset: offsetBefore,
					endOffset:   offsetAfter,
					text:        "\n",
					isBold:      curRBold,
					isItalic:    curRItalic,
					isUnderline: curRUnderline,
					isStrike:    curRStrike,
					isCode:      curRCode,
				})
			} else if pDepth > 0 && isWordElement(t.Name, "tab") {
				curNodes = append(curNodes, docxTextNode{
					startOffset: offsetBefore,
					endOffset:   offsetAfter,
					text:        "\t",
					isBold:      curRBold,
					isItalic:    curRItalic,
					isUnderline: curRUnderline,
					isStrike:    curRStrike,
					isCode:      curRCode,
				})
			} else if pDepth > 0 && (isWordElement(t.Name, "endnoteRef") || isWordElement(t.Name, "footnoteRef")) {
				if curNoteId != "" && !inSpecialNote {
					curNodes = append(curNodes, docxTextNode{
						startOffset: offsetBefore,
						endOffset:   offsetAfter,
						text:        curNoteId,
					})
				}
			}
		case xml.CharData:
			if inT {
				curTText.Write(t)
			}
		case xml.EndElement:
			if isWordElement(t.Name, "endnote") || isWordElement(t.Name, "footnote") {
				curNoteId = ""
				inSpecialNote = false
			} else if inT && isWordElement(t.Name, "t") {
				inT = false
				curNodes = append(curNodes, docxTextNode{
					startOffset: tStartOffset,
					endOffset:   offsetAfter,
					text:        curTText.String(),
					isBold:      curRBold,
					isItalic:    curRItalic,
					isUnderline: curRUnderline,
					isStrike:    curRStrike,
					isCode:      curRCode,
				})
			} else if isWordElement(t.Name, "r") {
				rDepth--
				if rDepth <= 0 {
					rDepth = 0
					curRBold = false
					curRItalic = false
					curRUnderline = false
					curRStrike = false
					curRCode = false
				}
			} else if isWordElement(t.Name, "p") {
				pDepth--
				if pDepth == 0 {
					if !inSpecialNote {
						isHeading := curPIsHeading
						var sb strings.Builder
						for _, n := range curNodes {
							sb.WriteString(n.text)
						}
						str := strings.TrimSpace(sb.String())
						if !isHeading && tblDepth == 0 {
							if isDocxHeadingText(str) {
								isHeading = true
								curPHeadingLevel = 1
							}
						}
						p := docxParagraph{
							nodes:           curNodes,
							isHeading:       isHeading,
							headingLevel:    curPHeadingLevel,
							alignment:       curPAlign,
							firstLineIndent: curPFirstLineIndent,
							isList:          curPIsList,
							isEmpty:         len(str) == 0,
						}
						if tblDepth > 0 && tcDepth > 0 {
							curCell.paragraphs = append(curCell.paragraphs, p)
						} else {
							bType := docxBlockTypeParagraph
							if isHeading {
								bType = docxBlockTypeHeading
							}
							blocks = append(blocks, docxBlock{
								blockType: bType,
								paragraph: p,
							})
						}
					}
					curNodes = nil
					curPIsHeading = false
					curPHeadingLevel = 0
					curPAlign = ""
					curPFirstLineIndent = false
					curPIsList = false
				}
			} else if isWordElement(t.Name, "tc") {
				tcDepth--
				if tcDepth == 0 && tblDepth > 0 && trDepth > 0 {
					curCell.alignment = curCellAlign
					curRow.cells = append(curRow.cells, curCell)
					curCell = docxCell{}
					curCellAlign = ""
				}
			} else if isWordElement(t.Name, "tr") {
				trDepth--
				if trDepth == 0 && tblDepth > 0 {
					curTable.rows = append(curTable.rows, curRow)
					curRow = docxRow{}
				}
			} else if isWordElement(t.Name, "tbl") {
				tblDepth--
				if tblDepth == 0 {
					blocks = append(blocks, docxBlock{
						blockType: docxBlockTypeTable,
						table:     curTable,
					})
					curTable = docxTable{}
				}
			}
		}
	}

	return blocks, nil
}

func extractParagraphsFromBlocks(blocks []docxBlock) []docxParagraph {
	var paras []docxParagraph
	for _, b := range blocks {
		if b.blockType == docxBlockTypeParagraph || b.blockType == docxBlockTypeHeading {
			paras = append(paras, b.paragraph)
		} else if b.blockType == docxBlockTypeTable {
			for _, row := range b.table.rows {
				for _, cell := range row.cells {
					paras = append(paras, cell.paragraphs...)
				}
			}
		}
	}
	return paras
}

// buildStyledParagraphSegments creates structured segments preserving formatting styles (bold, italic, etc.)
// and entity replacement tokens.
func buildStyledParagraphSegments(nodes []docxTextNode, matches []docxMatch, anon *anonymizer.Anonymizer, ts *anonymizer.TokenSession) []TextParagraphSegment {
	if len(nodes) == 0 {
		return nil
	}

	type nodeSpan struct {
		node  docxTextNode
		start int
		end   int
	}

	var pStr strings.Builder
	spans := make([]nodeSpan, 0, len(nodes))
	for _, n := range nodes {
		start := pStr.Len()
		pStr.WriteString(n.text)
		end := pStr.Len()
		spans = append(spans, nodeSpan{node: n, start: start, end: end})
	}
	text := pStr.String()

	if len(matches) == 0 {
		var segments []TextParagraphSegment
		for _, sp := range spans {
			if sp.start == sp.end {
				continue
			}
			segments = append(segments, TextParagraphSegment{
				Text:        sp.node.text,
				IsEntity:    false,
				IsBold:      sp.node.isBold,
				IsItalic:    sp.node.isItalic,
				IsUnderline: sp.node.isUnderline,
				IsStrike:    sp.node.isStrike,
				IsCode:      sp.node.isCode,
			})
		}
		return segments
	}

	getStyleAt := func(offset int) (bold, italic, underline, strike, code bool) {
		for _, sp := range spans {
			if offset >= sp.start && offset < sp.end {
				return sp.node.isBold, sp.node.isItalic, sp.node.isUnderline, sp.node.isStrike, sp.node.isCode
			}
		}
		if len(spans) > 0 {
			last := spans[len(spans)-1].node
			return last.isBold, last.isItalic, last.isUnderline, last.isStrike, last.isCode
		}
		return false, false, false, false, false
	}

	emitNonEntity := func(start, end int) []TextParagraphSegment {
		if start >= end || start >= len(text) {
			return nil
		}
		var segs []TextParagraphSegment
		for _, sp := range spans {
			iStart := max(start, sp.start)
			iEnd := min(end, sp.end)
			if iStart < iEnd {
				chunkText := text[iStart:iEnd]
				segs = append(segs, TextParagraphSegment{
					Text:        chunkText,
					IsEntity:    false,
					IsBold:      sp.node.isBold,
					IsItalic:    sp.node.isItalic,
					IsUnderline: sp.node.isUnderline,
					IsStrike:    sp.node.isStrike,
					IsCode:      sp.node.isCode,
				})
			}
		}
		return segs
	}

	var segments []TextParagraphSegment
	lastIdx := 0
	for _, m := range matches {
		if m.start > lastIdx && m.start <= len(text) {
			segments = append(segments, emitNonEntity(lastIdx, m.start)...)
		}
		var repl string
		if ts != nil {
			repl = ts.GenerateToken(EntityTypeToTokenType(m.entityType), m.value)
		} else {
			repl = generateSynthetic(anon, m.entityType, m.value)
		}
		b, it, u, s, c := getStyleAt(m.start)
		segments = append(segments, TextParagraphSegment{
			Text:        repl,
			IsEntity:    true,
			Type:        m.entityType,
			Original:    m.value,
			Anonymized:  repl,
			IsBold:      b,
			IsItalic:    it,
			IsUnderline: u,
			IsStrike:    s,
			IsCode:      c,
		})
		lastIdx = m.end
	}
	if lastIdx < len(text) {
		segments = append(segments, emitNonEntity(lastIdx, len(text))...)
	}

	var merged []TextParagraphSegment
	for _, seg := range segments {
		if len(merged) > 0 && !seg.IsEntity && !merged[len(merged)-1].IsEntity &&
			seg.IsBold == merged[len(merged)-1].IsBold &&
			seg.IsItalic == merged[len(merged)-1].IsItalic &&
			seg.IsUnderline == merged[len(merged)-1].IsUnderline &&
			seg.IsStrike == merged[len(merged)-1].IsStrike &&
			seg.IsCode == merged[len(merged)-1].IsCode {
			merged[len(merged)-1].Text += seg.Text
		} else {
			merged = append(merged, seg)
		}
	}
	return merged
}

func buildParagraphSegments(text string, matches []docxMatch, anon *anonymizer.Anonymizer, ts *anonymizer.TokenSession) []TextParagraphSegment {
	return buildStyledParagraphSegments([]docxTextNode{{text: text}}, matches, anon, ts)
}


func getDisabledEntityTypes(options map[string]interface{}) map[string]bool {
	disabled := make(map[string]bool)
	if options == nil {
		return disabled
	}

	addVal := func(v interface{}) {
		switch val := v.(type) {
		case string:
			s := strings.ToUpper(strings.TrimSpace(val))
			disabled[s] = true
			if s == "CARD" {
				disabled[string(detector.EntityCard)] = true
			} else if s == string(detector.EntityCard) {
				disabled["CARD"] = true
			}
			if s == "DATE" {
				disabled[string(detector.EntityBirthday)] = true
			} else if s == string(detector.EntityBirthday) {
				disabled["DATE"] = true
			}
		case detector.EntityType:
			disabled[strings.ToUpper(string(val))] = true
		}
	}

	addList := func(list interface{}) {
		switch l := list.(type) {
		case []string:
			for _, s := range l {
				addVal(s)
			}
		case []detector.EntityType:
			for _, s := range l {
				addVal(s)
			}
		case []interface{}:
			for _, itm := range l {
				addVal(itm)
			}
		}
	}

	if v, ok := options["disabled_entities"]; ok {
		addList(v)
	}
	if v, ok := options["disabled_entity_types"]; ok {
		addList(v)
	}

	return disabled
}

// Inspect examines paragraphs and tables in the DOCX archive and reports paragraph count, detected entities, snippets, structured paragraph previews, and full document blocks.
func (h *DOCXHandler) Inspect(r io.ReaderAt, size int64) (int, []EntityCount, []DocxTextSnippet, []TextParagraphPreview, []TextDocumentBlock, error) {
	if r == nil {
		return 0, nil, nil, nil, nil, errors.New("reader cannot be nil")
	}
	if size <= 0 {
		return 0, nil, nil, nil, nil, errors.New("invalid file size")
	}

	zr, err := zip.NewReader(r, size)
	if err != nil {
		return 0, nil, nil, nil, nil, fmt.Errorf("failed to open docx zip: %w", err)
	}

	paragraphCount := 0
	counts := make(map[string]int)
	var snippets []DocxTextSnippet
	var paragraphPreviews []TextParagraphPreview
	var documentBlocks []TextDocumentBlock
	ts := anonymizer.NewTokenSession("")

	// Filter and sort XML files in logical document reading order:
	// 1. word/document.xml (main body)
	// 2. word/footnotes.xml (footnotes)
	// 3. word/endnotes.xml (endnotes)
	// 4. word/comments.xml (comments)
	// 5. word/header*.xml & word/footer*.xml (headers & footers)
	var docxFiles []*zip.File
	for _, f := range zr.File {
		if isDocumentXML(f.Name) {
			docxFiles = append(docxFiles, f)
		}
	}
	sort.Slice(docxFiles, func(i, j int) bool {
		oi, _ := getDocxFileOrder(docxFiles[i].Name)
		oj, _ := getDocxFileOrder(docxFiles[j].Name)
		if oi == oj {
			return docxFiles[i].Name < docxFiles[j].Name
		}
		return oi < oj
	})

	for _, f := range docxFiles {
		rc, err := f.Open()
		if err != nil {
			return 0, nil, nil, nil, nil, fmt.Errorf("failed to open %s: %w", f.Name, err)
		}
		data, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			return 0, nil, nil, nil, nil, fmt.Errorf("failed to read %s: %w", f.Name, err)
		}

		blocks, err := parseDocxXML(data)
		if err != nil {
			return 0, nil, nil, nil, nil, fmt.Errorf("failed to parse %s: %w", f.Name, err)
		}

		paras := extractParagraphsFromBlocks(blocks)
		paragraphCount += len(paras)

		_, isBodyOrNotes := getDocxFileOrder(f.Name)

		for _, b := range blocks {
			if b.blockType == docxBlockTypeParagraph || b.blockType == docxBlockTypeHeading {
				var paraText strings.Builder
				for _, n := range b.paragraph.nodes {
					paraText.WriteString(n.text)
				}
				pStr := paraText.String()
				trimmedP := strings.TrimSpace(pStr)
				if trimmedP == "" {
					if isBodyOrNotes {
						documentBlocks = append(documentBlocks, TextDocumentBlock{
							Type:    "paragraph",
							Index:   len(documentBlocks) + 1,
							IsEmpty: true,
						})
					}
					continue
				}

				matches := findDocxSensitiveMatches(pStr)
				for _, m := range matches {
					counts[m.entityType]++
					tok := ts.GenerateToken(EntityTypeToTokenType(m.entityType), m.value)
					snippets = append(snippets, DocxTextSnippet{
						Original:   m.value,
						Anonymized: tok,
						Type:       m.entityType,
					})
				}

				if isBodyOrNotes {
					segments := buildStyledParagraphSegments(b.paragraph.nodes, matches, h.Anon, ts)

					paragraphPreviews = append(paragraphPreviews, TextParagraphPreview{
						Index:    len(paragraphPreviews) + 1,
						Segments: segments,
					})

					blockTypeStr := "paragraph"
					if b.blockType == docxBlockTypeHeading {
						blockTypeStr = "heading"
					}
					documentBlocks = append(documentBlocks, TextDocumentBlock{
						Type:            blockTypeStr,
						Index:           len(documentBlocks) + 1,
						Segments:        segments,
						Alignment:       b.paragraph.alignment,
						HeadingLevel:    b.paragraph.headingLevel,
						FirstLineIndent: b.paragraph.firstLineIndent,
						IsList:          b.paragraph.isList,
						IsEmpty:         false,
					})
				}
			} else if b.blockType == docxBlockTypeTable {
				var rowPreviews []TextTableRowPreview
				for _, r := range b.table.rows {
					var cellPreviews []TextTableCellPreview
					for _, c := range r.cells {
						var cellText strings.Builder
						var allCellNodes []docxTextNode
						for pIdx, cp := range c.paragraphs {
							if pIdx > 0 {
								cellText.WriteString("\n")
								allCellNodes = append(allCellNodes, docxTextNode{text: "\n"})
							}
							for _, n := range cp.nodes {
								cellText.WriteString(n.text)
								allCellNodes = append(allCellNodes, n)
							}
						}
						cStr := cellText.String()
						matches := findDocxSensitiveMatches(cStr)
						for _, m := range matches {
							counts[m.entityType]++
							tok := ts.GenerateToken(EntityTypeToTokenType(m.entityType), m.value)
							snippets = append(snippets, DocxTextSnippet{
								Original:   m.value,
								Anonymized: tok,
								Type:       m.entityType,
							})
						}

						if isBodyOrNotes {
							var segments []TextParagraphSegment
							if strings.TrimSpace(cStr) == "" {
								segments = buildStyledParagraphSegments(allCellNodes, nil, h.Anon, ts)
							} else {
								segments = buildStyledParagraphSegments(allCellNodes, matches, h.Anon, ts)
								paragraphPreviews = append(paragraphPreviews, TextParagraphPreview{
									Index:    len(paragraphPreviews) + 1,
									Segments: segments,
								})
							}

							cellAlign := c.alignment
							if cellAlign == "" {
								for _, cp := range c.paragraphs {
									if cp.alignment != "" {
										cellAlign = cp.alignment
										break
									}
								}
							}

							cellPreviews = append(cellPreviews, TextTableCellPreview{
								Segments:  segments,
								Alignment: cellAlign,
							})
						}
					}
					if isBodyOrNotes {
						rowPreviews = append(rowPreviews, TextTableRowPreview{
							IsHeader: r.isHeader,
							Cells:    cellPreviews,
						})
					}
				}

				if isBodyOrNotes {
					documentBlocks = append(documentBlocks, TextDocumentBlock{
						Type:  "table",
						Index: len(documentBlocks) + 1,
						Table: &TextTablePreview{
							Rows: rowPreviews,
						},
					})
				}
			}
		}
	}

	// Produce deterministic entities slice
	typeOrder := []string{
		string(detector.EntityFIO),
		string(detector.EntityPassport),
		string(detector.EntityPhone),
		string(detector.EntitySNILS),
		string(detector.EntityINN),
		string(detector.EntityCard),
		string(detector.EntityEmail),
		string(detector.EntityBirthday),
		string(detector.EntityAddress),
	}

	var entities []EntityCount
	for _, t := range typeOrder {
		if c, ok := counts[t]; ok && c > 0 {
			entities = append(entities, EntityCount{Type: t, Count: c})
			delete(counts, t)
		}
	}
	var remainingTypes []string
	for k := range counts {
		remainingTypes = append(remainingTypes, k)
	}
	sort.Strings(remainingTypes)
	for _, k := range remainingTypes {
		entities = append(entities, EntityCount{Type: k, Count: counts[k]})
	}

	return paragraphCount, entities, snippets, paragraphPreviews, documentBlocks, nil
}

// Process reads a DOCX document from r, anonymizes detected entities across paragraphs and runs,
// writes the output archive to w, and returns the populated DepMap.
func (h *DOCXHandler) Process(ctx context.Context, r io.ReaderAt, size int64, w io.Writer, options map[string]interface{}) (*depmap.DepMap, error) {
	if h.Anon == nil {
		return nil, errors.New("anonymizer is not initialized")
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
		return nil, fmt.Errorf("failed to open docx zip: %w", err)
	}

	disabledTypes := getDisabledEntityTypes(options)
	strategy := getProcessingStrategy(options)
	salt := ""
	if options != nil {
		if s, ok := options["salt"].(string); ok {
			salt = s
		}
	}
	ts := anonymizer.NewTokenSession(salt)

	origName := "document.docx"
	if options != nil {
		if s, ok := options["original_file_name"].(string); ok && s != "" {
			origName = s
		}
	}
	var dm *depmap.DepMap
	if d, ok := options["depmap"].(*depmap.DepMap); ok {
		dm = d
	} else if d, ok := options["depMap"].(*depmap.DepMap); ok {
		dm = d
	}
	if dm == nil {
		dm = depmap.New(origName)
	}
	dm.Mode = string(strategy)

	updatedXMLs := make(map[string][]byte)

	for _, f := range zr.File {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		if !isDocumentXML(f.Name) {
			continue
		}

		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("failed to open %s: %w", f.Name, err)
		}
		data, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			return nil, fmt.Errorf("failed to read %s: %w", f.Name, err)
		}

		blocks, err := parseDocxXML(data)
		if err != nil {
			return nil, fmt.Errorf("failed to parse %s: %w", f.Name, err)
		}

		paras := extractParagraphsFromBlocks(blocks)
		var replacements []textReplacement

		for _, p := range paras {
			if len(p.nodes) == 0 {
				continue
			}

			// 1. Build combined paragraph text and rune mapping
			var paraTextBuf strings.Builder
			nodeRuneStarts := make([]int, len(p.nodes))
			nodeRuneEnds := make([]int, len(p.nodes))

			curRuneIdx := 0
			for idx, n := range p.nodes {
				nodeRuneStarts[idx] = curRuneIdx
				rCount := utf8.RuneCountInString(n.text)
				nodeRuneEnds[idx] = curRuneIdx + rCount
				curRuneIdx += rCount
				paraTextBuf.WriteString(n.text)
			}

			paraText := paraTextBuf.String()
			paraRunes := []rune(paraText)
			totalRunes := len(paraRunes)

			// 2. Map byte offsets in paraText to rune offsets
			byteToRune := make([]int, len(paraText)+1)
			rIdx := 0
			for bIdx := range paraText {
				byteToRune[bIdx] = rIdx
				rIdx++
			}
			byteToRune[len(paraText)] = rIdx

			// 3. Find sensitive matches in paragraph text
			matches := findDocxSensitiveMatches(paraText)

			type entityRuneSpan struct {
				runeStart int
				runeEnd   int
				synthetic string
			}

			var spans []entityRuneSpan
			for _, m := range matches {
				if disabledTypes[strings.ToUpper(m.entityType)] {
					continue
				}
				rStart := byteToRune[m.start]
				rEnd := byteToRune[m.end]
				if rStart >= rEnd || rStart >= totalRunes {
					continue
				}
				if rEnd > totalRunes {
					rEnd = totalRunes
				}

				origVal := string(paraRunes[rStart:rEnd])
				var replVal string
				if strategy == domain.StrategyDelete {
					replVal = DeletionMarker
					dm.PutOrdinal(origVal)
				} else if strategy == domain.StrategySemanticTokens {
					tokenType := entityTypeToTokenType(m.entityType)
					replVal = ts.GenerateToken(tokenType, origVal)
					dm.Put(replVal, origVal)
				} else {
					replVal = generateSynthetic(h.Anon, m.entityType, origVal)
					dm.Put(replVal, origVal)
				}

				spans = append(spans, entityRuneSpan{
					runeStart: rStart,
					runeEnd:   rEnd,
					synthetic: replVal,
				})
			}

			if len(spans) == 0 {
				continue
			}

			// 4. Project replacements onto nodes
			for idx, n := range p.nodes {
				nStart := nodeRuneStarts[idx]
				nEnd := nodeRuneEnds[idx]

				var newNodeBuf strings.Builder
				nodeModified := false

				for r := nStart; r < nEnd; r++ {
					spanIdx := -1
					for sIdx, sp := range spans {
						if r >= sp.runeStart && r < sp.runeEnd {
							spanIdx = sIdx
							break
						}
					}

					if spanIdx == -1 {
						newNodeBuf.WriteRune(paraRunes[r])
					} else {
						nodeModified = true
						if r == spans[spanIdx].runeStart {
							newNodeBuf.WriteString(spans[spanIdx].synthetic)
						}
					}
				}

				if nodeModified {
					newText := newNodeBuf.String()
					var replBytes []byte
					if newText == "" {
						replBytes = []byte("<w:t></w:t>")
					} else {
						var escBuf bytes.Buffer
						_ = xml.EscapeText(&escBuf, []byte(newText))
						replBytes = []byte(fmt.Sprintf(`<w:t xml:space="preserve">%s</w:t>`, escBuf.String()))
					}

					replacements = append(replacements, textReplacement{
						start:   n.startOffset,
						end:     n.endOffset,
						newText: replBytes,
					})
				}
			}
		}

		newXML := applyReplacements(data, replacements)
		updatedXMLs[f.Name] = newXML
	}

	// Repack output zip
	zw := zip.NewWriter(w)
	for _, f := range zr.File {
		select {
		case <-ctx.Done():
			_ = zw.Close()
			return nil, ctx.Err()
		default:
		}

		if updatedData, ok := updatedXMLs[f.Name]; ok {
			h := &zip.FileHeader{
				Name:   f.Name,
				Method: zip.Deflate,
			}
			fw, err := zw.CreateHeader(h)
			if err != nil {
				return nil, fmt.Errorf("failed to create zip entry %s: %w", f.Name, err)
			}
			if _, err := fw.Write(updatedData); err != nil {
				return nil, fmt.Errorf("failed to write zip entry %s: %w", f.Name, err)
			}
		} else {
			if err := zw.Copy(f); err != nil {
				return nil, fmt.Errorf("failed to copy untouched zip entry %s: %w", f.Name, err)
			}
		}
	}

	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("failed to close docx zip writer: %w", err)
	}

	return dm, nil
}
