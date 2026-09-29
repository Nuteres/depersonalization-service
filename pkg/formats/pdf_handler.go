package formats

import (
	"bytes"
	"compress/zlib"
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"

	"dep_service/pkg/anonymizer"
	"dep_service/pkg/detector"
)

// BlackoutMarker is the replacement string injected into redacted PDF content streams.
const BlackoutMarker = "[ ПДн СКРЫТО ]"

// PDFHandler implements FormatHandler for PDF documents, performing physical text scrubbing
// and vector blackout redaction rectangle overlay.
type PDFHandler struct {
	Anon *anonymizer.Anonymizer
}

// NewPDFHandler creates a new PDFHandler with the given Anonymizer.
func NewPDFHandler(anon *anonymizer.Anonymizer) *PDFHandler {
	return &PDFHandler{Anon: anon}
}

// SupportedExtensions returns file extensions supported by this handler.
func (h *PDFHandler) SupportedExtensions() []string {
	return []string{".pdf"}
}

// BoundingBox holds the coordinates of a blackout rectangle in PDF points.
type BoundingBox struct {
	X float64
	Y float64
	W float64
	H float64
}

// pdfObject represents an indirect object in a PDF file.
type pdfObject struct {
	ID         int
	Gen        int
	DictHeader []byte
	StreamData []byte
	HasStream  bool
	Compressed bool // Uses FlateDecode
}

// Regular expressions for detecting sensitive Russian personal data.
var (
	reSNILSFormatted   = regexp.MustCompile(`(?:^|[^\p{L}\p{N}_])(\d{3}[-\s]\d{3}[-\s]\d{3}[-\s]\d{2})`)
	reSNILS11Digits    = regexp.MustCompile(`(?:^|[^\p{L}\p{N}_])(\d{11})`)
	rePassport4_6      = regexp.MustCompile(`(?:^|[^\p{L}\p{N}_])(\d{4}\s+\d{6})`)
	rePassport2_2_6    = regexp.MustCompile(`(?:^|[^\p{L}\p{N}_])(\d{2}\s+\d{2}\s+\d{6})`)
	rePassportPrefixed = regexp.MustCompile(`(?i)(?:паспорт(?:а| гражданина рф)?|серия|номер|№)[:\s]*([0-9\s]{10,13})`)
	reINN12            = regexp.MustCompile(`(?:^|[^\p{L}\p{N}_])(\d{12})`)
	reINNPrefixed      = regexp.MustCompile(`(?i)(?:ИНН|INN)[:\s]*\b(\d{10,12})\b`)
	rePhone            = regexp.MustCompile(`(?:^|[^\p{L}\p{N}_])((?:\+7|8)[\s\-\(]*\d{3}[\s\-\)]*\d{3}[\s\-]*\d{2}[\s\-]*\d{2})`)
	rePhonePrefixed    = regexp.MustCompile(`(?i)(?:тел(?:ефон)?|моб\.?|сот\.?)[:\s]*((?:\+7|8)?[0-9\s\-\(\)]{10,18})`)
	reCard16           = regexp.MustCompile(`(?:^|[^\p{L}\p{N}_])((?:\d{4}[\s\-]?){4})`)
	reFIOPatronymic    = regexp.MustCompile(`(?:^|[^\p{L}\p{N}_])([А-ЯЁ][а-яё]+(?:-[А-ЯЁ][а-яё]+)?\s+[А-ЯЁ][а-яё]+\s+[А-ЯЁ][а-яё]+(?:ович|евич|ич|овна|евна|ична))`)
	reFIOPrefixed      = regexp.MustCompile(`(?i)(?:сотрудник|клиент|пациент|фио|работник|физлицо)[:\s]*([А-ЯЁ][а-яё]+(?:\s+[А-ЯЁ][а-яё]+){1,2})`)
)

// Process reads a PDF from r, scrubs sensitive text entities, draws solid black
// redaction rectangles, and writes the resulting sanitized PDF to w.
func (h *PDFHandler) Process(ctx context.Context, r io.Reader, w io.Writer, options map[string]interface{}) error {
	if h.Anon == nil {
		return errors.New("anonymizer is not initialized")
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Read entire PDF into memory
	pdfBytes, err := io.ReadAll(r)
	if err != nil {
		return fmt.Errorf("failed to read input PDF: %w", err)
	}

	if !bytes.HasPrefix(pdfBytes, []byte("%PDF-")) {
		return errors.New("invalid PDF: missing %PDF- header")
	}

	customTerms := extractCustomTerms(options)

	// Parse objects from the PDF
	objects, rootRef, err := parsePDF(pdfBytes)
	if err != nil {
		return fmt.Errorf("failed to parse PDF objects: %w", err)
	}

	// Process each stream object
	for i := range objects {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		obj := &objects[i]
		if !obj.HasStream || len(obj.StreamData) == 0 {
			continue
		}

		// Skip non-content streams (images, fonts, descriptors, metadata, etc.)
		if !isContentStream(obj.DictHeader) {
			continue
		}

		// Decompress if FlateDecode
		var decompressed []byte
		if obj.Compressed {
			zr, err := zlib.NewReader(bytes.NewReader(obj.StreamData))
			if err == nil {
				decomp, readErr := io.ReadAll(zr)
				_ = zr.Close()
				if readErr == nil {
					decompressed = decomp
				}
			}
			if decompressed == nil {
				// Fallback to raw if zlib read failed
				decompressed = obj.StreamData
				obj.Compressed = false
			}
		} else {
			decompressed = obj.StreamData
		}

		// Redact content stream
		redactedStream, modified := redactContentStream(decompressed, customTerms)
		if modified {
			if obj.Compressed {
				var buf bytes.Buffer
				zw := zlib.NewWriter(&buf)
				_, _ = zw.Write(redactedStream)
				_ = zw.Close()
				obj.StreamData = buf.Bytes()
			} else {
				obj.StreamData = redactedStream
			}

			// Update /Length in DictHeader
			obj.DictHeader = updateStreamLength(obj.DictHeader, len(obj.StreamData))
		}
	}

	// Rebuild and write output PDF
	return writePDF(w, objects, rootRef)
}

func extractCustomTerms(options map[string]interface{}) []string {
	if options == nil {
		return nil
	}
	if terms, ok := options["redact_terms"].([]string); ok {
		return terms
	}
	if rawList, ok := options["redact_terms"].([]interface{}); ok {
		var terms []string
		for _, item := range rawList {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				terms = append(terms, strings.TrimSpace(s))
			}
		}
		return terms
	}
	return nil
}

// redactContentStream scans a PDF content stream, scrubs sensitive text,
// replaces it with BlackoutMarker, and appends blackout rectangle operators.
func redactContentStream(data []byte, customTerms []string) ([]byte, bool) {
	// Skip streams that contain no text operators and no custom search terms
	hasTextOps := bytes.Contains(data, []byte("BT")) || bytes.Contains(data, []byte("Tj")) || bytes.Contains(data, []byte("TJ"))
	if !hasTextOps && len(customTerms) == 0 {
		return data, false
	}

	var out bytes.Buffer
	modified := false

	// Text state tracking
	inText := false
	currentFontSize := 12.0
	currentX := 0.0
	currentY := 0.0
	lineStartX := 0.0
	lineStartY := 0.0
	leading := 14.0

	var pendingBoxes []BoundingBox

	i := 0
	for i < len(data) {
		// 1. Literal string: ( ... )
		if data[i] == '(' {
			rawStr, decoded, nextPos := parseLiteralString(data, i)
			i = nextPos

			// Lookahead for operator (Tj, ', ", etc.)
			op, _ := peekNextOperator(data, i)

			if op == "Tj" || op == "'" || op == "\"" {
				if op == "'" || op == "\"" {
					lineStartY -= leading
					currentX = lineStartX
					currentY = lineStartY
				}

				newRaw, boxes, didRedact := processTextString(decoded, currentFontSize, currentX, currentY, customTerms)
				if didRedact {
					modified = true
					out.Write(newRaw)
					pendingBoxes = append(pendingBoxes, boxes...)
				} else {
					out.Write(rawStr)
				}
				advance := float64(len([]rune(string(decoded)))) * (currentFontSize * 0.52)
				currentX += advance
			} else {
				out.Write(rawStr)
			}
			continue
		}

		// 2. Hex string: < ... >
		if data[i] == '<' && (i+1 < len(data) && data[i+1] != '<') {
			rawHex, decoded, nextPos := parseHexString(data, i)
			i = nextPos

			op, _ := peekNextOperator(data, i)
			if op == "Tj" || op == "'" || op == "\"" {
				newRaw, boxes, didRedact := processTextString(decoded, currentFontSize, currentX, currentY, customTerms)
				if didRedact {
					modified = true
					out.Write(newRaw)
					pendingBoxes = append(pendingBoxes, boxes...)
				} else {
					out.Write(rawHex)
				}
				advance := float64(len([]rune(string(decoded)))) * (currentFontSize * 0.52)
				currentX += advance
			} else {
				out.Write(rawHex)
			}
			continue
		}

		// 3. Array: [ ... ] (handles TJ operator)
		if data[i] == '[' {
			rawArr, nextPos := parseArray(data, i)
			i = nextPos

			op, _ := peekNextOperator(data, i)
			if op == "TJ" {
				newArr, boxes, didRedact := processTJArray(rawArr, currentFontSize, currentX, currentY, customTerms)
				if didRedact {
					modified = true
					out.Write(newArr)
					pendingBoxes = append(pendingBoxes, boxes...)
				} else {
					out.Write(rawArr)
				}
			} else {
				out.Write(rawArr)
			}
			continue
		}

		// 4. Name token: /Name
		if data[i] == '/' {
			start := i
			i++
			for i < len(data) && !isWhitespace(data[i]) && data[i] != '(' && data[i] != '<' && data[i] != '[' && data[i] != '/' && data[i] != ')' && data[i] != '>' && data[i] != ']' {
				i++
			}
			out.Write(data[start:i])
			continue
		}

		// 5. White space
		if isWhitespace(data[i]) {
			out.WriteByte(data[i])
			i++
			continue
		}

		// 6. Read regular word/token
		wordStart := i
		for i < len(data) && !isWhitespace(data[i]) && data[i] != '(' && data[i] != '<' && data[i] != '[' && data[i] != '/' && data[i] != ')' && data[i] != '>' && data[i] != ']' {
			i++
		}
		if wordStart == i {
			out.WriteByte(data[i])
			i++
			continue
		}
		word := string(data[wordStart:i])

		switch word {
		case "BT":
			inText = true
			currentX = 0
			currentY = 0
			lineStartX = 0
			lineStartY = 0
			out.WriteString("BT")
		case "ET":
			inText = false
			out.WriteString("ET")
			// Emit solid black rectangles outside BT ... ET
			if len(pendingBoxes) > 0 {
				out.WriteString("\n")
				for _, b := range pendingBoxes {
					fmt.Fprintf(&out, "q 0 0 0 rg %.2f %.2f %.2f %.2f re f Q\n", b.X, b.Y, b.W, b.H)
				}
				pendingBoxes = nil
			}
		case "Tf":
			// Previous tokens set font size
			fs := extractPreviousFontSize(out.Bytes())
			if fs > 0 {
				currentFontSize = fs
			}
			out.WriteString("Tf")
		case "Tm":
			// Set text matrix: a b c d e f Tm
			x, y := extractPreviousTmCoords(out.Bytes())
			currentX = x
			currentY = y
			lineStartX = x
			lineStartY = y
			out.WriteString("Tm")
		case "Td":
			tx, ty := extractPreviousTdCoords(out.Bytes())
			lineStartX += tx
			lineStartY += ty
			currentX = lineStartX
			currentY = lineStartY
			out.WriteString("Td")
		case "TD":
			tx, ty := extractPreviousTdCoords(out.Bytes())
			leading = -ty
			lineStartX += tx
			lineStartY += ty
			currentX = lineStartX
			currentY = lineStartY
			out.WriteString("TD")
		case "T*":
			lineStartY -= leading
			currentX = lineStartX
			currentY = lineStartY
			out.WriteString("T*")
		default:
			out.WriteString(word)
		}
	}

	_ = inText

	// If there are any pending boxes, emit them
	if len(pendingBoxes) > 0 {
		out.WriteString("\n")
		for _, b := range pendingBoxes {
			fmt.Fprintf(&out, "q 0 0 0 rg %.2f %.2f %.2f %.2f re f Q\n", b.X, b.Y, b.W, b.H)
		}
	}

	res := out.Bytes()

	// Defense-in-depth: scrub any remaining raw occurrences of sensitive strings
	scrubbedRes, extraModified := scrubRemainingSensitiveText(res, customTerms)
	if extraModified {
		modified = true
		res = scrubbedRes
	}

	return res, modified
}

type textMatch struct {
	start int
	end   int
	value string
}

// processTextString searches for sensitive personal data in a text string,
// calculates bounding boxes, and replaces matched text with BlackoutMarker.
func processTextString(decoded []byte, fontSize, curX, curY float64, customTerms []string) ([]byte, []BoundingBox, bool) {
	text := decodeText(decoded)
	matches := findSensitiveMatches(text, customTerms)
	if len(matches) == 0 {
		return nil, nil, false
	}

	// Sort matches right-to-left so indices stay valid during replacements
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].start > matches[j].start
	})

	var boxes []BoundingBox
	redactedText := text
	runes := []rune(text)
	charWidth := fontSize * 0.52

	for _, m := range matches {
		runeStart := len([]rune(text[:m.start]))
		runeLen := len([]rune(m.value))
		if runeLen == 0 {
			continue
		}

		bx := curX + float64(runeStart)*charWidth
		bw := float64(runeLen) * charWidth
		if bw < fontSize*1.2 {
			bw = fontSize * 1.2
		}
		by := curY - (fontSize * 0.25)
		bh := fontSize * 1.25

		boxes = append(boxes, BoundingBox{X: bx, Y: by, W: bw, H: bh})

		// Replace in string
		redactedText = redactedText[:m.start] + BlackoutMarker + redactedText[m.end:]
	}
	_ = runes

	// Format back as PDF literal string: ( ... )
	var buf bytes.Buffer
	buf.WriteByte('(')
	for i := 0; i < len(redactedText); i++ {
		b := redactedText[i]
		if b == '(' || b == ')' || b == '\\' {
			buf.WriteByte('\\')
		}
		buf.WriteByte(b)
	}
	buf.WriteByte(')')

	return buf.Bytes(), boxes, true
}

// processTJArray processes an array operand of a TJ operator.
func processTJArray(rawArr []byte, fontSize, curX, curY float64, customTerms []string) ([]byte, []BoundingBox, bool) {
	// Parse array elements: either strings or numbers
	var elements [][]byte
	i := 1 // skip '['
	for i < len(rawArr) && rawArr[i] != ']' {
		if isWhitespace(rawArr[i]) {
			i++
			continue
		}
		if rawArr[i] == '(' {
			rawStr, _, nextPos := parseLiteralString(rawArr, i)
			elements = append(elements, rawStr)
			i = nextPos
			continue
		}
		if rawArr[i] == '<' && (i+1 < len(rawArr) && rawArr[i+1] != '<') {
			rawHex, _, nextPos := parseHexString(rawArr, i)
			elements = append(elements, rawHex)
			i = nextPos
			continue
		}
		// Number
		start := i
		for i < len(rawArr) && !isWhitespace(rawArr[i]) && rawArr[i] != ']' && rawArr[i] != '(' && rawArr[i] != '<' {
			i++
		}
		elements = append(elements, rawArr[start:i])
	}

	var totalBoxes []BoundingBox
	modified := false
	var newElements [][]byte

	arrX := curX
	charWidth := fontSize * 0.52

	for _, elem := range elements {
		if len(elem) > 0 && elem[0] == '(' {
			_, decoded, _ := parseLiteralString(elem, 0)
			newRaw, boxes, didRedact := processTextString(decoded, fontSize, arrX, curY, customTerms)
			if didRedact {
				modified = true
				newElements = append(newElements, newRaw)
				totalBoxes = append(totalBoxes, boxes...)
			} else {
				newElements = append(newElements, elem)
			}
			arrX += float64(len([]rune(string(decoded)))) * charWidth
		} else if len(elem) > 0 && elem[0] == '<' {
			_, decoded, _ := parseHexString(elem, 0)
			newRaw, boxes, didRedact := processTextString(decoded, fontSize, arrX, curY, customTerms)
			if didRedact {
				modified = true
				newElements = append(newElements, newRaw)
				totalBoxes = append(totalBoxes, boxes...)
			} else {
				newElements = append(newElements, elem)
			}
			arrX += float64(len([]rune(string(decoded)))) * charWidth
		} else {
			// Spacing number
			newElements = append(newElements, elem)
			if n, err := strconv.ParseFloat(string(elem), 64); err == nil {
				arrX -= (n / 1000.0) * fontSize
			}
		}
	}

	if !modified {
		return rawArr, nil, false
	}

	var buf bytes.Buffer
	buf.WriteByte('[')
	for idx, e := range newElements {
		if idx > 0 {
			buf.WriteByte(' ')
		}
		buf.Write(e)
	}
	buf.WriteByte(']')

	return buf.Bytes(), totalBoxes, true
}

func isBoundaryAfter(text string, end int) bool {
	if end >= len(text) {
		return true
	}
	rNext, _ := utf8.DecodeRuneInString(text[end:])
	return !unicode.IsLetter(rNext) && !unicode.IsDigit(rNext) && rNext != '_'
}

// findSensitiveMatches locates all PII entities and custom terms in text.
func findSensitiveMatches(text string, customTerms []string) []textMatch {
	var matches []textMatch

	addRegexMatches := func(re *regexp.Regexp, submatchIdx int, validateFunc func(string) bool) {
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
			if !isBoundaryAfter(text, end) {
				continue
			}
			val := text[start:end]
			if validateFunc == nil || validateFunc(val) {
				matches = append(matches, textMatch{start: start, end: end, value: val})
			}
		}
	}

	// 1. SNILS
	addRegexMatches(reSNILSFormatted, 1, nil)
	addRegexMatches(reSNILS11Digits, 1, func(s string) bool {
		return detector.ValidateSNILS(s)
	})

	// 2. Passport RF
	addRegexMatches(rePassport4_6, 1, nil)
	addRegexMatches(rePassport2_2_6, 1, nil)
	addRegexMatches(rePassportPrefixed, 1, nil)

	// 3. INN
	addRegexMatches(reINN12, 1, func(s string) bool {
		return detector.ValidateINN12(s)
	})
	addRegexMatches(reINNPrefixed, 1, nil)

	// 4. Phone
	addRegexMatches(rePhone, 1, nil)
	addRegexMatches(rePhonePrefixed, 1, nil)

	// 5. Card MIR / Luhn
	addRegexMatches(reCard16, 1, nil)

	// 6. FIO
	addRegexMatches(reFIOPatronymic, 1, nil)
	addRegexMatches(reFIOPrefixed, 1, nil)

	// 7. Custom terms
	for _, term := range customTerms {
		if term == "" {
			continue
		}
		start := 0
		for {
			idx := strings.Index(text[start:], term)
			if idx == -1 {
				break
			}
			mStart := start + idx
			mEnd := mStart + len(term)
			matches = append(matches, textMatch{start: mStart, end: mEnd, value: term})
			start = mEnd
		}
	}

	// Filter overlapping matches: prefer earlier or longer
	if len(matches) <= 1 {
		return matches
	}

	sort.Slice(matches, func(i, j int) bool {
		if matches[i].start == matches[j].start {
			return matches[i].end > matches[j].end
		}
		return matches[i].start < matches[j].start
	})

	var deduped []textMatch
	lastEnd := -1
	for _, m := range matches {
		if m.start >= lastEnd {
			deduped = append(deduped, m)
			lastEnd = m.end
		}
	}

	return deduped
}

// scrubRemainingSensitiveText performs defense-in-depth replacement of any
// lingering sensitive strings across the decompressed stream.
func scrubRemainingSensitiveText(stream []byte, customTerms []string) ([]byte, bool) {
	text := string(stream)
	matches := findSensitiveMatches(text, customTerms)
	if len(matches) == 0 {
		return stream, false
	}

	sort.Slice(matches, func(i, j int) bool {
		return matches[i].start > matches[j].start
	})

	res := text
	for _, m := range matches {
		res = res[:m.start] + BlackoutMarker + res[m.end:]
	}

	return []byte(res), true
}

func decodeText(raw []byte) string {
	// UTF-16BE BOM check
	if len(raw) >= 2 && raw[0] == 0xFE && raw[1] == 0xFF {
		u16 := make([]uint16, (len(raw)-2)/2)
		for j := 0; j < len(u16); j++ {
			u16[j] = binary.BigEndian.Uint16(raw[2+j*2:])
		}
		return string(utf16.Decode(u16))
	}

	if utf8.Valid(raw) {
		return string(raw)
	}

	// Try Windows-1251
	if cp1251, err := charmap.Windows1251.NewDecoder().Bytes(raw); err == nil && utf8.Valid(cp1251) {
		return string(cp1251)
	}

	return string(raw)
}

func parseLiteralString(data []byte, start int) (raw []byte, decoded []byte, nextPos int) {
	depth := 1
	escaped := false
	var dec bytes.Buffer

	for i := start + 1; i < len(data); i++ {
		b := data[i]
		if escaped {
			escaped = false
			switch b {
			case 'n':
				dec.WriteByte('\n')
			case 'r':
				dec.WriteByte('\r')
			case 't':
				dec.WriteByte('\t')
			case 'b':
				dec.WriteByte('\b')
			case 'f':
				dec.WriteByte('\f')
			case '(', ')', '\\':
				dec.WriteByte(b)
			case '0', '1', '2', '3', '4', '5', '6', '7':
				// Octal escape \ooo
				octalVal := int(b - '0')
				if i+1 < len(data) && data[i+1] >= '0' && data[i+1] <= '7' {
					i++
					octalVal = octalVal*8 + int(data[i]-'0')
					if i+1 < len(data) && data[i+1] >= '0' && data[i+1] <= '7' {
						i++
						octalVal = octalVal*8 + int(data[i]-'0')
					}
				}
				dec.WriteByte(byte(octalVal))
			default:
				dec.WriteByte(b)
			}
			continue
		}

		if b == '\\' {
			escaped = true
			continue
		}

		if b == '(' {
			depth++
			dec.WriteByte(b)
		} else if b == ')' {
			depth--
			if depth == 0 {
				return data[start : i+1], dec.Bytes(), i + 1
			}
			dec.WriteByte(b)
		} else {
			dec.WriteByte(b)
		}
	}

	return data[start:], dec.Bytes(), len(data)
}

func parseHexString(data []byte, start int) (raw []byte, decoded []byte, nextPos int) {
	end := bytes.IndexByte(data[start:], '>')
	if end == -1 {
		return data[start:], nil, len(data)
	}
	hexBytes := data[start+1 : start+end]
	var cleaned []byte
	for _, b := range hexBytes {
		if !isWhitespace(b) {
			cleaned = append(cleaned, b)
		}
	}
	if len(cleaned)%2 != 0 {
		cleaned = append(cleaned, '0')
	}

	dec, _ := hex.DecodeString(string(cleaned))
	return data[start : start+end+1], dec, start + end + 1
}

func parseArray(data []byte, start int) (raw []byte, nextPos int) {
	depth := 1
	for i := start + 1; i < len(data); i++ {
		if data[i] == '[' {
			depth++
		} else if data[i] == ']' {
			depth--
			if depth == 0 {
				return data[start : i+1], i + 1
			}
		}
	}
	return data[start:], len(data)
}

func peekNextOperator(data []byte, start int) (string, int) {
	i := start
	for i < len(data) && isWhitespace(data[i]) {
		i++
	}
	if i >= len(data) {
		return "", 0
	}
	wordStart := i
	for i < len(data) && !isWhitespace(data[i]) && data[i] != '(' && data[i] != '<' && data[i] != '[' && data[i] != '/' {
		i++
	}
	return string(data[wordStart:i]), i - start
}

func isWhitespace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\r' || b == '\n' || b == '\f' || b == 0
}

func isContentStream(dictHeader []byte) bool {
	if bytes.Contains(dictHeader, []byte("/Subtype /Image")) || bytes.Contains(dictHeader, []byte("/Subtype/Image")) {
		return false
	}
	if bytes.Contains(dictHeader, []byte("/Type /Font")) || bytes.Contains(dictHeader, []byte("/Type/Font")) {
		return false
	}
	if bytes.Contains(dictHeader, []byte("/Type /FontDescriptor")) || bytes.Contains(dictHeader, []byte("/Type/FontDescriptor")) {
		return false
	}
	if bytes.Contains(dictHeader, []byte("/FontFile")) {
		return false
	}
	if bytes.Contains(dictHeader, []byte("/Type /Metadata")) || bytes.Contains(dictHeader, []byte("/Type/Metadata")) {
		return false
	}
	if bytes.Contains(dictHeader, []byte("/Type /XRef")) || bytes.Contains(dictHeader, []byte("/Type/XRef")) {
		return false
	}
	if bytes.Contains(dictHeader, []byte("/Type /ObjStm")) || bytes.Contains(dictHeader, []byte("/Type/ObjStm")) {
		return false
	}
	return true
}

// getLastTokens scans backwards from the end of buf and returns up to n tokens
// (in reverse order: [last, second-to-last, ...]) in O(1) time without buffer copies.
func getLastTokens(buf []byte, n int) [][]byte {
	tokens := make([][]byte, 0, n)
	i := len(buf) - 1
	for i >= 0 && len(tokens) < n {
		for i >= 0 && isWhitespace(buf[i]) {
			i--
		}
		if i < 0 {
			break
		}
		end := i + 1
		for i >= 0 && !isWhitespace(buf[i]) {
			i--
		}
		tokens = append(tokens, buf[i+1:end])
	}
	return tokens
}

func extractPreviousFontSize(buf []byte) float64 {
	tokens := getLastTokens(buf, 1)
	if len(tokens) >= 1 {
		if fs, err := strconv.ParseFloat(string(tokens[0]), 64); err == nil && fs > 0 {
			return fs
		}
	}
	return 12.0
}

func extractPreviousTmCoords(buf []byte) (float64, float64) {
	tokens := getLastTokens(buf, 2)
	if len(tokens) >= 2 {
		y, errY := strconv.ParseFloat(string(tokens[0]), 64)
		x, errX := strconv.ParseFloat(string(tokens[1]), 64)
		if errX == nil && errY == nil {
			return x, y
		}
	}
	return 0, 0
}

func extractPreviousTdCoords(buf []byte) (float64, float64) {
	tokens := getLastTokens(buf, 2)
	if len(tokens) >= 2 {
		ty, errY := strconv.ParseFloat(string(tokens[0]), 64)
		tx, errX := strconv.ParseFloat(string(tokens[1]), 64)
		if errX == nil && errY == nil {
			return tx, ty
		}
	}
	return 0, 0
}

func updateStreamLength(dictHeader []byte, newLen int) []byte {
	reLen := regexp.MustCompile(`(?s)/Length\s+(\d+|\d+\s+\d+\s+R)`)
	newLenStr := fmt.Sprintf("/Length %d", newLen)
	if reLen.Match(dictHeader) {
		return reLen.ReplaceAll(dictHeader, []byte(newLenStr))
	}
	// If /Length is missing, inject it right before >>
	idx := bytes.LastIndex(dictHeader, []byte(">>"))
	if idx != -1 {
		var b bytes.Buffer
		b.Write(dictHeader[:idx])
		b.WriteString(" " + newLenStr + " ")
		b.Write(dictHeader[idx:])
		return b.Bytes()
	}
	return dictHeader
}

// parsePDF scans indirect objects and trailer from a PDF byte slice.
func parsePDF(data []byte) ([]pdfObject, string, error) {
	var objects []pdfObject
	rootRef := ""

	// Find trailer /Root if present
	trailerIdx := bytes.LastIndex(data, []byte("trailer"))
	if trailerIdx != -1 {
		trailerBlock := string(data[trailerIdx:])
		reRoot := regexp.MustCompile(`/Root\s+(\d+\s+\d+\s+R)`)
		if m := reRoot.FindStringSubmatch(trailerBlock); len(m) > 1 {
			rootRef = m[1]
		}
	}

	reObjHeader := regexp.MustCompile(`(\d+)\s+(\d+)\s+obj\b`)
	locs := reObjHeader.FindAllIndex(data, -1)
	if len(locs) == 0 {
		return nil, "", errors.New("no PDF objects found")
	}

	for i := 0; i < len(locs); i++ {
		start := locs[i][0]
		headerMatch := reObjHeader.FindSubmatch(data[start:locs[i][1]])
		id, _ := strconv.Atoi(string(headerMatch[1]))
		gen, _ := strconv.Atoi(string(headerMatch[2]))

		// Scan end of object
		var objEnd int
		if i+1 < len(locs) {
			objEnd = locs[i+1][0]
		} else {
			objEnd = len(data)
		}

		objBytes := data[start:objEnd]
		endObjIdx := bytes.Index(objBytes, []byte("endobj"))
		if endObjIdx != -1 {
			objBytes = objBytes[:endObjIdx+len("endobj")]
		}

		// Check for stream
		streamTag := []byte("stream")
		sPos := bytes.Index(objBytes, streamTag)

		if sPos == -1 {
			// Non-stream object
			headerPart := objBytes[len(headerMatch[0]):]
			if idx := bytes.Index(headerPart, []byte("endobj")); idx != -1 {
				headerPart = headerPart[:idx]
			}
			objects = append(objects, pdfObject{
				ID:         id,
				Gen:        gen,
				DictHeader: bytes.TrimSpace(headerPart),
				HasStream:  false,
			})
		} else {
			dictPart := objBytes[len(headerMatch[0]):sPos]
			streamStart := sPos + len(streamTag)
			if streamStart < len(objBytes) && objBytes[streamStart] == '\r' {
				streamStart++
			}
			if streamStart < len(objBytes) && objBytes[streamStart] == '\n' {
				streamStart++
			}

			var streamData []byte
			reLen := regexp.MustCompile(`/Length\s+(\d+)`)
			if m := reLen.FindSubmatch(dictPart); len(m) > 1 {
				if length, err := strconv.Atoi(string(m[1])); err == nil && length >= 0 && streamStart+length <= len(objBytes) {
					streamData = objBytes[streamStart : streamStart+length]
				}
			}

			if streamData == nil {
				endstreamTag := []byte("endstream")
				ePos := bytes.Index(objBytes[streamStart:], endstreamTag)
				if ePos != -1 {
					rawEnd := streamStart + ePos
					// PDF spec §7.3.8: strip at most one EOL sequence immediately preceding endstream
					if rawEnd > streamStart && objBytes[rawEnd-1] == '\n' {
						rawEnd--
						if rawEnd > streamStart && objBytes[rawEnd-1] == '\r' {
							rawEnd--
						}
					} else if rawEnd > streamStart && objBytes[rawEnd-1] == '\r' {
						rawEnd--
					}
					streamData = objBytes[streamStart:rawEnd]
				} else {
					streamData = objBytes[streamStart:]
				}
			}

			compressed := bytes.Contains(dictPart, []byte("/FlateDecode"))

			objects = append(objects, pdfObject{
				ID:         id,
				Gen:        gen,
				DictHeader: bytes.TrimSpace(dictPart),
				StreamData: streamData,
				HasStream:  true,
				Compressed: compressed,
			})
		}
	}

	// Fallback for rootRef: search parsed objects for /Type /Catalog
	if rootRef == "" {
		for _, obj := range objects {
			if bytes.Contains(obj.DictHeader, []byte("/Type /Catalog")) || bytes.Contains(obj.DictHeader, []byte("/Type/Catalog")) {
				rootRef = fmt.Sprintf("%d %d R", obj.ID, obj.Gen)
				break
			}
		}
		if rootRef == "" {
			rootRef = "1 0 R"
		}
	}

	return objects, rootRef, nil
}

// writePDF generates a valid PDF conforming to the PDF specification with an
// updated cross-reference table and trailer.
func writePDF(w io.Writer, objects []pdfObject, rootRef string) error {
	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n%\xe2\xe3\xcf\xd3\n")

	// Sort objects by ID
	sort.Slice(objects, func(i, j int) bool {
		return objects[i].ID < objects[j].ID
	})

	maxID := 0
	offsets := make(map[int]int)

	for _, obj := range objects {
		if obj.ID > maxID {
			maxID = obj.ID
		}
		offsets[obj.ID] = buf.Len()

		fmt.Fprintf(&buf, "%d %d obj\n", obj.ID, obj.Gen)
		if len(obj.DictHeader) > 0 {
			buf.Write(obj.DictHeader)
			buf.WriteString("\n")
		}
		if obj.HasStream {
			buf.WriteString("stream\n")
			buf.Write(obj.StreamData)
			buf.WriteString("\nendstream\n")
		}
		buf.WriteString("endobj\n")
	}

	startXref := buf.Len()
	fmt.Fprintf(&buf, "xref\n0 %d\n", maxID+1)
	buf.WriteString("0000000000 65535 f \n")

	for id := 1; id <= maxID; id++ {
		if offset, ok := offsets[id]; ok {
			fmt.Fprintf(&buf, "%010d 00000 n \n", offset)
		} else {
			buf.WriteString("0000000000 65535 f \n")
		}
	}

	fmt.Fprintf(&buf, "trailer\n<< /Size %d /Root %s >>\nstartxref\n%d\n%%%%EOF\n", maxID+1, rootRef, startXref)

	_, err := w.Write(buf.Bytes())
	return err
}
