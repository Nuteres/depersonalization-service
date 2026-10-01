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

// parseODTXML parses an ODT XML file (content.xml or styles.xml) and extracts paragraphs (<text:p> and <text:h>)
// along with their inner character data nodes and byte offsets.
func parseODTXML(data []byte) ([]odtParagraph, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	var paras []odtParagraph
	var curPara *odtParagraph
	inParaDepth := 0

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
			if (t.Name.Local == "p" || t.Name.Local == "h") && (t.Name.Space == "urn:oasis:names:tc:opendocument:xmlns:text:1.0" || strings.HasPrefix(t.Name.Space, "http") || t.Name.Space == "" || t.Name.Space == "text") {
				if inParaDepth == 0 {
					curPara = &odtParagraph{}
				}
				inParaDepth++
			} else if inParaDepth > 0 {
				inParaDepth++
			}

		case xml.EndElement:
			if inParaDepth > 0 {
				inParaDepth--
				if inParaDepth == 0 && curPara != nil {
					paras = append(paras, *curPara)
					curPara = nil
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

	return paras, nil
}

// Inspect reads an ODT document from r and returns paragraph count, detected entity counts, and preview snippets.
func (h *ODTHandler) Inspect(r io.ReaderAt, size int64) (int, []EntityCount, []DocxTextSnippet, error) {
	if r == nil {
		return 0, nil, nil, errors.New("reader cannot be nil")
	}
	if size <= 0 {
		return 0, nil, nil, errors.New("invalid file size")
	}

	zr, err := zip.NewReader(r, size)
	if err != nil {
		return 0, nil, nil, fmt.Errorf("failed to open odt zip: %w", err)
	}

	totalParagraphs := 0
	entityTypeCounts := make(map[string]int)
	var snippets []DocxTextSnippet
	seenSnippets := make(map[string]bool)

	for _, f := range zr.File {
		if !isODTTextXML(f.Name) {
			continue
		}

		rc, err := f.Open()
		if err != nil {
			return 0, nil, nil, fmt.Errorf("failed to open %s: %w", f.Name, err)
		}
		data, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			return 0, nil, nil, fmt.Errorf("failed to read %s: %w", f.Name, err)
		}

		paras, err := parseODTXML(data)
		if err != nil {
			return 0, nil, nil, fmt.Errorf("failed to parse %s: %w", f.Name, err)
		}

		totalParagraphs += len(paras)

		for _, p := range paras {
			if len(p.nodes) == 0 {
				continue
			}

			var paraTextBuf strings.Builder
			for _, n := range p.nodes {
				paraTextBuf.WriteString(n.text)
			}
			paraText := paraTextBuf.String()

			matches := findDocxSensitiveMatches(paraText)
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
		}
	}

	var entities []EntityCount
	for k, v := range entityTypeCounts {
		entities = append(entities, EntityCount{Type: k, Count: v})
	}

	return totalParagraphs, entities, snippets, nil
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

	origName := "document.odt"
	if options != nil {
		if s, ok := options["original_file_name"].(string); ok && s != "" {
			origName = s
		}
	}
	dm := depmap.New(origName)

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

		paras, err := parseODTXML(data)
		if err != nil {
			return nil, fmt.Errorf("failed to parse %s: %w", f.Name, err)
		}

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
				synthVal := generateSynthetic(h.Anon, m.entityType, origVal)
				dm.Put(synthVal, origVal)

				spans = append(spans, entityRuneSpan{
					runeStart: rStart,
					runeEnd:   rEnd,
					synthetic: synthVal,
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
