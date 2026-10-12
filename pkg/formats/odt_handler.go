package formats

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"dep_service/internal/domain"
	"dep_service/pkg/anonymizer"
	"dep_service/pkg/depmap"
)

// ODTHandler handles inspection and anonymization of OpenDocument Text (.odt) documents.
type ODTHandler struct {
	Anon *anonymizer.Anonymizer
}

// NewODTHandler creates a new ODTHandler with the given Anonymizer.
func NewODTHandler(anon *anonymizer.Anonymizer) *ODTHandler {
	return &ODTHandler{Anon: anon}
}

// SupportedExtensions returns file extensions supported by this handler.
func (h *ODTHandler) SupportedExtensions() []string {
	return []string{".odt"}
}

// isODTTextXML returns true for XML files in an ODT package containing text content.
func isODTTextXML(name string) bool {
	return name == "content.xml" || name == "styles.xml"
}

type odtTextNode struct {
	startOffset int64
	endOffset   int64
	text        string
}

type odtParagraph struct {
	nodes []odtTextNode
}

type odtCell struct {
	paragraphs []odtParagraph
}

type odtRow struct {
	cells []odtCell
}

type odtTable struct {
	rows []odtRow
}

type odtBlockType int

const (
	odtBlockTypeParagraph odtBlockType = iota
	odtBlockTypeTable
)

type odtBlock struct {
	blockType odtBlockType
	paragraph odtParagraph
	table     odtTable
}

// parseODTXML parses an ODT XML file (content.xml or styles.xml) and extracts document blocks
// (paragraphs and tables) along with their inner character data nodes and byte offsets.
func parseODTXML(data []byte) ([]odtBlock, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	var blocks []odtBlock
	var curPara *odtParagraph
	inParaDepth := 0

	tblDepth := 0
	trDepth := 0
	tcDepth := 0

	var curTable odtTable
	var curRow odtRow
	var curCell odtCell

	for {
		startOffset := dec.InputOffset()
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("xml decode error: %w", err)
		}
		endOffset := dec.InputOffset()

		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local == "table" {
				if tblDepth == 0 {
					curTable = odtTable{}
				}
				tblDepth++
			} else if t.Name.Local == "table-row" {
				if tblDepth > 0 && trDepth == 0 {
					curRow = odtRow{}
				}
				trDepth++
			} else if t.Name.Local == "table-cell" {
				if tblDepth > 0 && trDepth > 0 && tcDepth == 0 {
					curCell = odtCell{}
				}
				tcDepth++
			} else if (t.Name.Local == "p" || t.Name.Local == "h") && (t.Name.Space == "urn:oasis:names:tc:opendocument:xmlns:text:1.0" || strings.HasPrefix(t.Name.Space, "http") || t.Name.Space == "" || t.Name.Space == "text") {
				if inParaDepth == 0 {
					curPara = &odtParagraph{}
				}
				inParaDepth++
			} else if inParaDepth > 0 && (t.Name.Local == "line-break" || t.Name.Local == "tab") {
				textVal := "\n"
				if t.Name.Local == "tab" {
					textVal = "\t"
				}
				if curPara != nil {
					curPara.nodes = append(curPara.nodes, odtTextNode{
						startOffset: startOffset,
						endOffset:   endOffset,
						text:        textVal,
					})
				}
			} else if inParaDepth > 0 {
				inParaDepth++
			}

		case xml.EndElement:
			if inParaDepth > 0 {
				inParaDepth--
				if inParaDepth == 0 && curPara != nil {
					p := *curPara
					if tblDepth > 0 && tcDepth > 0 {
						curCell.paragraphs = append(curCell.paragraphs, p)
					} else {
						blocks = append(blocks, odtBlock{
							blockType: odtBlockTypeParagraph,
							paragraph: p,
						})
					}
					curPara = nil
				}
			} else if t.Name.Local == "table-cell" {
				tcDepth--
				if tcDepth == 0 && tblDepth > 0 && trDepth > 0 {
					curRow.cells = append(curRow.cells, curCell)
					curCell = odtCell{}
				}
			} else if t.Name.Local == "table-row" {
				trDepth--
				if trDepth == 0 && tblDepth > 0 {
					curTable.rows = append(curTable.rows, curRow)
					curRow = odtRow{}
				}
			} else if t.Name.Local == "table" {
				tblDepth--
				if tblDepth == 0 {
					blocks = append(blocks, odtBlock{
						blockType: odtBlockTypeTable,
						table:     curTable,
					})
					curTable = odtTable{}
				}
			}

		case xml.CharData:
			if inParaDepth > 0 && curPara != nil {
				textVal := string(t)
				curPara.nodes = append(curPara.nodes, odtTextNode{
					startOffset: startOffset,
					endOffset:   endOffset,
					text:        textVal,
				})
			}
		}
	}

	return blocks, nil
}

func extractODTParagraphsFromBlocks(blocks []odtBlock) []odtParagraph {
	var paras []odtParagraph
	for _, b := range blocks {
		if b.blockType == odtBlockTypeParagraph {
			paras = append(paras, b.paragraph)
		} else if b.blockType == odtBlockTypeTable {
			for _, row := range b.table.rows {
				for _, cell := range row.cells {
					paras = append(paras, cell.paragraphs...)
				}
			}
		}
	}
	return paras
}

// Inspect reads an ODT document from r and returns paragraph count, detected entity counts, preview snippets, paragraph previews, and full document blocks.
func (h *ODTHandler) Inspect(r io.ReaderAt, size int64) (int, []EntityCount, []DocxTextSnippet, []TextParagraphPreview, []TextDocumentBlock, error) {
	if r == nil {
		return 0, nil, nil, nil, nil, errors.New("reader cannot be nil")
	}
	if size <= 0 {
		return 0, nil, nil, nil, nil, errors.New("invalid file size")
	}

	zr, err := zip.NewReader(r, size)
	if err != nil {
		return 0, nil, nil, nil, nil, fmt.Errorf("failed to open odt zip: %w", err)
	}

	totalParagraphs := 0
	entityTypeCounts := make(map[string]int)
	var snippets []DocxTextSnippet
	seenSnippets := make(map[string]bool)
	var paragraphPreviews []TextParagraphPreview
	var documentBlocks []TextDocumentBlock

	for _, f := range zr.File {
		if !isODTTextXML(f.Name) {
			continue
		}

		rc, err := f.Open()
		if err != nil {
			return 0, nil, nil, nil, nil, fmt.Errorf("failed to open %s: %w", f.Name, err)
		}
		data, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			return 0, nil, nil, nil, nil, fmt.Errorf("failed to read %s: %w", f.Name, err)
		}

		blocks, err := parseODTXML(data)
		if err != nil {
			return 0, nil, nil, nil, nil, fmt.Errorf("failed to parse %s: %w", f.Name, err)
		}

		paras := extractODTParagraphsFromBlocks(blocks)
		totalParagraphs += len(paras)

		isMainBody := (f.Name == "content.xml")

		for _, b := range blocks {
			if b.blockType == odtBlockTypeParagraph {
				if len(b.paragraph.nodes) == 0 {
					continue
				}

				var paraTextBuf strings.Builder
				for _, n := range b.paragraph.nodes {
					paraTextBuf.WriteString(n.text)
				}
				pStr := paraTextBuf.String()
				trimmedP := strings.TrimSpace(pStr)
				if trimmedP == "" {
					continue
				}

				matches := findDocxSensitiveMatches(pStr)
				for _, m := range matches {
					entityTypeCounts[m.entityType]++

					if len(snippets) < 20 && !seenSnippets[m.value] {
						seenSnippets[m.value] = true
						synth := m.value
						if h.Anon != nil {
							synth = generateSynthetic(h.Anon, m.entityType, m.value)
						}
						snippets = append(snippets, DocxTextSnippet{
							Original:   m.value,
							Anonymized: synth,
							Type:       m.entityType,
						})
					}
				}

				if isMainBody {
					segments := buildParagraphSegments(pStr, matches, h.Anon)

					paragraphPreviews = append(paragraphPreviews, TextParagraphPreview{
						Index:    len(paragraphPreviews) + 1,
						Segments: segments,
					})

					documentBlocks = append(documentBlocks, TextDocumentBlock{
						Type:     "paragraph",
						Index:    len(documentBlocks) + 1,
						Segments: segments,
					})
				}
			} else if b.blockType == odtBlockTypeTable {
				var rowPreviews []TextTableRowPreview
				for _, r := range b.table.rows {
					var cellPreviews []TextTableCellPreview
					for _, c := range r.cells {
						var cellText strings.Builder
						for pIdx, cp := range c.paragraphs {
							if pIdx > 0 {
								cellText.WriteString("\n")
							}
							for _, n := range cp.nodes {
								cellText.WriteString(n.text)
							}
						}
						cStr := cellText.String()
						matches := findDocxSensitiveMatches(cStr)
						for _, m := range matches {
							entityTypeCounts[m.entityType]++

							if len(snippets) < 20 && !seenSnippets[m.value] {
								seenSnippets[m.value] = true
								synth := m.value
								if h.Anon != nil {
									synth = generateSynthetic(h.Anon, m.entityType, m.value)
								}
								snippets = append(snippets, DocxTextSnippet{
									Original:   m.value,
									Anonymized: synth,
									Type:       m.entityType,
								})
							}
						}

						if isMainBody {
							var segments []TextParagraphSegment
							if strings.TrimSpace(cStr) == "" {
								segments = []TextParagraphSegment{{Text: cStr, IsEntity: false}}
							} else {
								segments = buildParagraphSegments(cStr, matches, h.Anon)
								paragraphPreviews = append(paragraphPreviews, TextParagraphPreview{
									Index:    len(paragraphPreviews) + 1,
									Segments: segments,
								})
							}

							cellPreviews = append(cellPreviews, TextTableCellPreview{
								Segments: segments,
							})
						}
					}
					if isMainBody {
						rowPreviews = append(rowPreviews, TextTableRowPreview{
							Cells: cellPreviews,
						})
					}
				}

				if isMainBody {
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

	var entities []EntityCount
	for k, v := range entityTypeCounts {
		entities = append(entities, EntityCount{Type: k, Count: v})
	}

	return totalParagraphs, entities, snippets, paragraphPreviews, documentBlocks, nil
}

// Process reads an ODT document from r, anonymizes detected sensitive personal data,
// writes the updated document to w, and returns the generated depmap container.
func (h *ODTHandler) Process(ctx context.Context, r io.ReaderAt, size int64, w io.Writer, options map[string]interface{}) (*depmap.DepMap, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	if h.Anon == nil {
		return nil, errors.New("anonymizer cannot be nil")
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

	zr, err := zip.NewReader(r, size)
	if err != nil {
		return nil, fmt.Errorf("failed to open odt zip: %w", err)
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

	origName := "document.odt"
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

		if !isODTTextXML(f.Name) {
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

		blocks, err := parseODTXML(data)
		if err != nil {
			return nil, fmt.Errorf("failed to parse %s: %w", f.Name, err)
		}

		paras := extractODTParagraphsFromBlocks(blocks)
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
					var escBuf bytes.Buffer
					_ = xml.EscapeText(&escBuf, []byte(newText))

					replacements = append(replacements, textReplacement{
						start:   int(n.startOffset),
						end:     int(n.endOffset),
						newText: escBuf.Bytes(),
					})
				}
			}
		}

		newXML := applyReplacements(data, replacements)
		updatedXMLs[f.Name] = newXML
	}

	// Repack output zip
	zw := zip.NewWriter(w)

	// In ODF specification, mimetype must be the first file and stored uncompressed
	for _, f := range zr.File {
		if f.Name == "mimetype" {
			h := &zip.FileHeader{
				Name:   "mimetype",
				Method: zip.Store,
			}
			fw, err := zw.CreateHeader(h)
			if err != nil {
				_ = zw.Close()
				return nil, fmt.Errorf("failed to create mimetype header: %w", err)
			}
			rc, err := f.Open()
			if err != nil {
				_ = zw.Close()
				return nil, fmt.Errorf("failed to open mimetype: %w", err)
			}
			_, err = io.Copy(fw, rc)
			_ = rc.Close()
			if err != nil {
				_ = zw.Close()
				return nil, fmt.Errorf("failed to copy mimetype: %w", err)
			}
			break
		}
	}

	for _, f := range zr.File {
		select {
		case <-ctx.Done():
			_ = zw.Close()
			return nil, ctx.Err()
		default:
		}

		if f.Name == "mimetype" {
			continue // Already written
		}

		if updatedData, ok := updatedXMLs[f.Name]; ok {
			h := &zip.FileHeader{
				Name:   f.Name,
				Method: zip.Deflate,
			}
			fw, err := zw.CreateHeader(h)
			if err != nil {
				_ = zw.Close()
				return nil, fmt.Errorf("failed to create zip entry %s: %w", f.Name, err)
			}
			if _, err := fw.Write(updatedData); err != nil {
				_ = zw.Close()
				return nil, fmt.Errorf("failed to write zip entry %s: %w", f.Name, err)
			}
		} else {
			if err := zw.Copy(f); err != nil {
				_ = zw.Close()
				return nil, fmt.Errorf("failed to copy untouched zip entry %s: %w", f.Name, err)
			}
		}
	}

	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("failed to close odt zip writer: %w", err)
	}

	return dm, nil
}
