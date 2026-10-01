package formats

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

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
type DocxTextSnippet struct {
	Original   string `json:"original"`
	Anonymized string `json:"anonymized"`
	Type       string `json:"type"`
}

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

// Regular expressions for detecting sensitive Russian personal data in Word documents.
var (
	docxSNILSFormatted   = regexp.MustCompile(`(?:^|[^\p{L}\p{N}_])(\d{3}[-\s]\d{3}[-\s]\d{3}[-\s]\d{2})`)
	docxSNILS11Digits    = regexp.MustCompile(`(?:^|[^\p{L}\p{N}_])(\d{11})`)
	docxPassport4_6      = regexp.MustCompile(`(?:^|[^\p{L}\p{N}_])(\d{4}\s+\d{6})`)
	docxPassport2_2_6    = regexp.MustCompile(`(?:^|[^\p{L}\p{N}_])(\d{2}\s+\d{2}\s+\d{6})`)
	docxPassportPrefixed = regexp.MustCompile(`(?i)(?:паспорт(?:а| гражданина рф)?|серия|номер|№)[:\s]*([0-9\s]{10,13})`)
	docxINN12            = regexp.MustCompile(`(?:^|[^\p{L}\p{N}_])(\d{12})`)
	docxINNPrefixed      = regexp.MustCompile(`(?i)(?:ИНН|INN)[:\s]*\b(\d{10,12})\b`)
	docxPhone            = regexp.MustCompile(`(?:^|[^\p{L}\p{N}_])((?:\+7|8)[\s\-\(]*\d{3}[\s\-\)]*\d{3}[\s\-]*\d{2}[\s\-]*\d{2})`)
	docxPhonePrefixed    = regexp.MustCompile(`(?i)(?:тел(?:ефон)?|моб\.?|сот\.?)[:\s]*((?:\+7|8)?[0-9\s\-\(\)]{10,18})`)
	docxCard16           = regexp.MustCompile(`(?:^|[^\p{L}\p{N}_])((?:\d{4}[\s\-]?){4})`)
	docxEmail            = regexp.MustCompile(`(?:^|[^\p{L}\p{N}_])([a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,})`)
	docxFIOPatronymic    = regexp.MustCompile(`(?:^|[^\p{L}\p{N}_])([А-ЯЁ][а-яё]+(?:-[А-ЯЁ][а-яё]+)?\s+[А-ЯЁ][а-яё]+\s+[А-ЯЁ][а-яё]+(?:ович|евич|ич|овна|евна|ична))`)
	docxFIOPrefixed      = regexp.MustCompile(`(?i)(?:сотрудник|клиент|пациент|фио|работник|физлицо|подписал)[:\s]+([А-ЯЁ][а-яё]+(?:\s+[А-ЯЁ][а-яё]+){1,2})`)
	docxDateDMY          = regexp.MustCompile(`(?:^|[^\p{L}\p{N}_])(\d{2}\.\d{2}\.\d{4})`)
	docxDateYMD          = regexp.MustCompile(`(?:^|[^\p{L}\p{N}_])(\d{4}-\d{2}-\d{2})`)
)

type docxMatch struct {
	start      int // byte offset in string
	end        int // byte offset in string
	value      string
	entityType string
}

func isDocxBoundaryAfter(text string, end int) bool {
	if end >= len(text) {
		return true
	}
	rNext, _ := utf8.DecodeRuneInString(text[end:])
	return !unicode.IsLetter(rNext) && !unicode.IsDigit(rNext) && rNext != '_'
}

func cleanDigits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// findDocxSensitiveMatches locates sensitive entities in paragraph text.
func findDocxSensitiveMatches(text string) []docxMatch {
	var matches []docxMatch

	addRegexMatches := func(re *regexp.Regexp, submatchIdx int, entityType string, validateFunc func(string) bool) {
		indexes := re.FindAllStringSubmatchIndex(text, -1)
		for _, loc := range indexes {
			var start, end int
			if submatchIdx*2+1 < len(loc) && loc[submatchIdx*2] >= 0 {
				start = loc[submatchIdx*2]
				end = loc[submatchIdx*2+1]
			} else {
				start = loc[0]
				end = loc[1]
			}

			val := text[start:end]
			trimmed := strings.TrimSpace(val)
			if trimmed == "" {
				continue
			}
			leadTrim := strings.Index(val, trimmed)
			start += leadTrim
			end = start + len(trimmed)
			val = trimmed

			if !isDocxBoundaryAfter(text, end) {
				continue
			}
			if validateFunc == nil || validateFunc(val) {
				matches = append(matches, docxMatch{
					start:      start,
					end:        end,
					value:      val,
					entityType: entityType,
				})
			}
		}
	}

	// 1. Bank card (16 digits)
	addRegexMatches(docxCard16, 1, string(detector.EntityCard), func(s string) bool {
		return len(cleanDigits(s)) == 16
	})

	// 2. SNILS
	addRegexMatches(docxSNILSFormatted, 1, string(detector.EntitySNILS), func(s string) bool {
		return len(cleanDigits(s)) == 11
	})
	addRegexMatches(docxSNILS11Digits, 1, string(detector.EntitySNILS), func(s string) bool {
		return detector.ValidateSNILS(s)
	})

	// 3. Email
	addRegexMatches(docxEmail, 1, string(detector.EntityEmail), nil)

	// 4. Phone
	addRegexMatches(docxPhone, 1, string(detector.EntityPhone), func(s string) bool {
		d := cleanDigits(s)
		return len(d) >= 10 && len(d) <= 11
	})
	addRegexMatches(docxPhonePrefixed, 1, string(detector.EntityPhone), func(s string) bool {
		d := cleanDigits(s)
		return len(d) >= 10 && len(d) <= 11
	})

	// 5. INN
	addRegexMatches(docxINN12, 1, string(detector.EntityINN), func(s string) bool {
		return detector.ValidateINN12(s)
	})
	addRegexMatches(docxINNPrefixed, 1, string(detector.EntityINN), nil)

	// 6. Passport
	addRegexMatches(docxPassport4_6, 1, string(detector.EntityPassport), func(s string) bool {
		return len(cleanDigits(s)) == 10
	})
	addRegexMatches(docxPassport2_2_6, 1, string(detector.EntityPassport), func(s string) bool {
		return len(cleanDigits(s)) == 10
	})
	addRegexMatches(docxPassportPrefixed, 1, string(detector.EntityPassport), func(s string) bool {
		return len(cleanDigits(s)) == 10
	})

	// 7. FIO
	addRegexMatches(docxFIOPatronymic, 1, string(detector.EntityFIO), nil)
	addRegexMatches(docxFIOPrefixed, 1, string(detector.EntityFIO), func(s string) bool {
		parts := strings.Fields(s)
		return len(parts) >= 2
	})

	// 8. Dates
	addRegexMatches(docxDateDMY, 1, string(detector.EntityBirthday), func(s string) bool {
		_, err := time.Parse("02.01.2006", s)
		return err == nil
	})
	addRegexMatches(docxDateYMD, 1, string(detector.EntityBirthday), func(s string) bool {
		_, err := time.Parse("2006-01-02", s)
		return err == nil
	})

	if len(matches) <= 1 {
		return matches
	}

	// Filter overlapping matches: prefer earlier start, then longer match
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].start == matches[j].start {
			return matches[i].end > matches[j].end
		}
		return matches[i].start < matches[j].start
	})

	var deduped []docxMatch
	lastEnd := -1
	for _, m := range matches {
		if m.start >= lastEnd {
			deduped = append(deduped, m)
			lastEnd = m.end
		}
	}

	return deduped
}

func generateSynthetic(anon *anonymizer.Anonymizer, entityType string, original string) string {
	if anon == nil {
		return fmt.Sprintf("[%s]", entityType)
	}
	switch entityType {
	case string(detector.EntityFIO), "NAME":
		return anon.GenerateFIO(original)
	case string(detector.EntityPassport):
		return anon.GeneratePassport(original)
	case string(detector.EntityPhone):
		return anon.GeneratePhone(original)
	case string(detector.EntitySNILS):
		return anon.GenerateSNILS(original)
	case string(detector.EntityINN):
		return anon.GenerateINN12(original)
	case string(detector.EntityCard), "CARD":
		return anon.GenerateCardMIR(original)
	case string(detector.EntityEmail):
		return anon.GenerateEmail(original)
	case string(detector.EntityBirthday), "DATE":
		return anon.PerturbDate(original, 30)
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

func isDocumentXML(name string) bool {
	if name == "word/document.xml" {
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
}

type docxParagraph struct {
	nodes []docxTextNode
}

func parseDocxXML(xmlData []byte) ([]docxParagraph, error) {
	dec := xml.NewDecoder(bytes.NewReader(xmlData))
	var paragraphs []docxParagraph
	var curNodes []docxTextNode
	pDepth := 0
	inT := false
	var curTText strings.Builder
	var tStartOffset int

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
			if isWordElement(t.Name, "p") {
				if pDepth == 0 {
					curNodes = nil
				}
				pDepth++
			} else if pDepth > 0 && isWordElement(t.Name, "t") {
				inT = true
				tStartOffset = offsetBefore
				curTText.Reset()
			}
		case xml.CharData:
			if inT {
				curTText.Write(t)
			}
		case xml.EndElement:
			if inT && isWordElement(t.Name, "t") {
				inT = false
				curNodes = append(curNodes, docxTextNode{
					startOffset: tStartOffset,
					endOffset:   offsetAfter,
					text:        curTText.String(),
				})
			} else if isWordElement(t.Name, "p") {
				pDepth--
				if pDepth == 0 {
					paragraphs = append(paragraphs, docxParagraph{
						nodes: curNodes,
					})
					curNodes = nil
				}
			}
		}
	}

	return paragraphs, nil
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

// Inspect examines paragraphs in the DOCX archive and reports paragraph count, detected entities and snippets.
func (h *DOCXHandler) Inspect(r io.ReaderAt, size int64) (int, []EntityCount, []DocxTextSnippet, error) {
	if r == nil {
		return 0, nil, nil, errors.New("reader cannot be nil")
	}
	if size <= 0 {
		return 0, nil, nil, errors.New("invalid file size")
	}

	zr, err := zip.NewReader(r, size)
	if err != nil {
		return 0, nil, nil, fmt.Errorf("failed to open docx zip: %w", err)
	}

	paragraphCount := 0
	counts := make(map[string]int)
	var snippets []DocxTextSnippet

	for _, f := range zr.File {
		if !isDocumentXML(f.Name) {
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

		paras, err := parseDocxXML(data)
		if err != nil {
			return 0, nil, nil, fmt.Errorf("failed to parse %s: %w", f.Name, err)
		}

		paragraphCount += len(paras)

		for _, p := range paras {
			if len(p.nodes) == 0 {
				continue
			}

			var paraText strings.Builder
			for _, n := range p.nodes {
				paraText.WriteString(n.text)
			}

			matches := findDocxSensitiveMatches(paraText.String())
			for _, m := range matches {
				counts[m.entityType]++
				synth := generateSynthetic(h.Anon, m.entityType, m.value)
				snippets = append(snippets, DocxTextSnippet{
					Original:   m.value,
					Anonymized: synth,
					Type:       m.entityType,
				})
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

	return paragraphCount, entities, snippets, nil
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

	origName := "document.docx"
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

		paras, err := parseDocxXML(data)
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
			fw, err := zw.CreateHeader(&f.FileHeader)
			if err != nil {
				return nil, fmt.Errorf("failed to create untouched zip entry %s: %w", f.Name, err)
			}
			rc, err := f.Open()
			if err != nil {
				return nil, fmt.Errorf("failed to open untouched zip entry %s: %w", f.Name, err)
			}
			_, err = io.Copy(fw, rc)
			_ = rc.Close()
			if err != nil {
				return nil, fmt.Errorf("failed to copy untouched zip entry %s: %w", f.Name, err)
			}
		}
	}

	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("failed to close docx zip writer: %w", err)
	}

	return dm, nil
}
