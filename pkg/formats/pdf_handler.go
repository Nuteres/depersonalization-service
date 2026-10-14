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

	"dep_service/internal/domain"
	"dep_service/pkg/anonymizer"
	"dep_service/pkg/detector"
)

// BlackoutMarker is the replacement string injected into redacted PDF content streams.
const BlackoutMarker = "[ ПДн СКРЫТО ]"

func getPDFMarker(mode domain.ProcessingMode) string {
	if mode == domain.ModeDelete {
		return " " + DeletionMarker + " "
	}
	return BlackoutMarker
}

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
	reFIOPatronymic    = regexp.MustCompile(`(?:^|[^\p{L}\p{N}_])([А-ЯЁ][а-яё]+(?:-[А-ЯЁ][а-яё]+)?\s+[А-ЯЁ][а-яё]+\s+[А-ЯЁ][а-яё]+(?:овича|евича|ича|овны|евны|ичны|овичу|евичу|ичу|овне|евне|ичне|овичем|евичем|ичем|овной|евной|ичной|овиче|евиче|иче|ович|евич|ич|овна|евна|ична|оглы|кызы|уулу|инична))`)
	reFIOPrefixed      = regexp.MustCompile(`(?i:(?:сотрудник|клиент|пациент|фио|работник|физлицо|подписал|сдал|принял|директор|гендиректор|руководитель|представитель)[:\s]+)([А-ЯЁ][а-яё]+(?:\s+[А-ЯЁ][а-яё]+){1,2})`)
	reFIOInitials1     = regexp.MustCompile(`(?:^|[^\p{L}\p{N}_])([А-ЯЁ][а-яё]+(?:\s+[А-ЯЁ]\.\s*[А-ЯЁ]\.|\s+[А-ЯЁ]\.))`)
	reFIOInitials2     = regexp.MustCompile(`(?:^|[^\p{L}\p{N}_])((?:[А-ЯЁ]\.\s*[А-ЯЁ]\.|\s*[А-ЯЁ]\.)\s+[А-ЯЁ][а-яё]+)`)
	reSubtypeForm      = regexp.MustCompile(`/Subtype\s*/Form\b`)
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

	mode := getProcessingMode(options)
	customTerms := extractCustomTerms(options)
	disabledEntities := extractDisabledEntities(options)

	// Parse objects from the PDF
	objects, rootRef, err := parsePDF(pdfBytes)
	if err != nil {
		return fmt.Errorf("failed to parse PDF objects: %w", err)
	}

	objMap := make(map[int]*pdfObject, len(objects))
	for i := range objects {
		objMap[objects[i].ID] = &objects[i]
	}
	fontCMaps, defaultCMap := extractFontCMaps(objMap)
	redactionBoxes := extractRedactionBoxes(options)

	// Collect page-to-content stream mapping for coordinate redaction
	pageTree := getPDFPageTree(pdfBytes, objMap, objects)
	boxesByStream := make(map[int][]domain.PDFRedactionBox)
	if len(redactionBoxes) > 0 {
		boxesByPage := make(map[int][]domain.PDFRedactionBox)
		for _, b := range redactionBoxes {
			p := b.Page
			if p <= 0 {
				p = 1
			}
			boxesByPage[p] = append(boxesByPage[p], b)
		}
		for _, p := range pageTree {
			if pBoxes, ok := boxesByPage[p.Num]; ok && len(pBoxes) > 0 && len(p.ContentRefs) > 0 {
				targetID := p.ContentRefs[len(p.ContentRefs)-1]
				boxesByStream[targetID] = append(boxesByStream[targetID], pBoxes...)
			}
		}
		if len(boxesByStream) == 0 {
			for i := range objects {
				if isContentStream(objects[i].DictHeader) && objects[i].HasStream && len(objects[i].StreamData) > 0 {
					boxesByStream[objects[i].ID] = redactionBoxes
					break
				}
			}
		}
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
		redactedStream, modified := redactContentStream(decompressed, customTerms, disabledEntities, mode, fontCMaps, defaultCMap)

		// If there are redaction boxes assigned to this stream, append vector blackout rectangles
		if boxes, ok := boxesByStream[obj.ID]; ok && len(boxes) > 0 {
			var rectBuf bytes.Buffer
			rectBuf.WriteString("\n")
			for _, b := range boxes {
				fmt.Fprintf(&rectBuf, "q 0 0 0 rg %.3f %.3f %.3f %.3f re f Q\n", b.X, b.Y, b.Width, b.Height)
			}
			redactedStream = append(redactedStream, rectBuf.Bytes()...)
			modified = true
		}

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

type pdfPageTreeEntry struct {
	Num         int
	ContentRefs []int
}

func getPDFPageTree(pdfBytes []byte, objMap map[int]*pdfObject, objects []pdfObject) []pdfPageTreeEntry {
	var pages []pdfPageTreeEntry

	rePagesRef := regexp.MustCompile(`/Pages\s+(\d+)\s+0\s+R`)
	reKids := regexp.MustCompile(`(?s)/Kids\s*\[(.*?)\]`)
	reRef := regexp.MustCompile(`(\d+)\s+0\s+R`)
	reContents := regexp.MustCompile(`/Contents\s+(\d+)\s+0\s+R`)
	reContentsArr := regexp.MustCompile(`(?s)/Contents\s*\[(.*?)\]`)
	reResRef := regexp.MustCompile(`/Resources\s+(\d+)\s+0\s+R`)
	reXObjectDict := regexp.MustCompile(`(?s)/XObject\s*<<([^>]*)>>`)

	rootID := 0
	reRoot := regexp.MustCompile(`/Root\s+(\d+)\s+0\s+R`)
	if rm := reRoot.FindSubmatch(pdfBytes); len(rm) > 1 {
		rootID, _ = strconv.Atoi(string(rm[1]))
	}

	pagesRootID := 0
	if rootObj, ok := objMap[rootID]; ok {
		if pm := rePagesRef.FindStringSubmatch(string(rootObj.DictHeader)); len(pm) > 1 {
			pagesRootID, _ = strconv.Atoi(pm[1])
		}
	}

	visitedPages := make(map[int]bool)
	var collectPages func(nodeID int)
	collectPages = func(nodeID int) {
		if visitedPages[nodeID] {
			return
		}
		visitedPages[nodeID] = true
		node, ok := objMap[nodeID]
		if !ok {
			return
		}
		dictStr := string(node.DictHeader)
		isPage := strings.Contains(dictStr, "/Type /Page\b") || strings.Contains(dictStr, "/Type/Page\b") ||
			(!strings.Contains(dictStr, "/Type /Pages") && !strings.Contains(dictStr, "/Type/Pages") && strings.Contains(dictStr, "/Contents"))

		if isPage {
			var cRefs []int
			if cmArr := reContentsArr.FindStringSubmatch(dictStr); len(cmArr) > 1 {
				for _, r := range reRef.FindAllStringSubmatch(cmArr[1], -1) {
					cID, _ := strconv.Atoi(r[1])
					cRefs = append(cRefs, cID)
				}
			} else if cm := reContents.FindStringSubmatch(dictStr); len(cm) > 1 {
				cID, _ := strconv.Atoi(cm[1])
				cRefs = append(cRefs, cID)
			}

			resDict := dictStr
			if rrm := reResRef.FindStringSubmatch(dictStr); len(rrm) > 1 {
				rID, _ := strconv.Atoi(rrm[1])
				if rObj, ok := objMap[rID]; ok {
					resDict += " " + string(rObj.DictHeader)
				}
			}
			if xm := reXObjectDict.FindStringSubmatch(resDict); len(xm) > 1 {
				for _, xr := range reRef.FindAllStringSubmatch(xm[1], -1) {
					xID, _ := strconv.Atoi(xr[1])
					if xObj, ok := objMap[xID]; ok {
						xDict := string(xObj.DictHeader)
						if reSubtypeForm.MatchString(xDict) || bytes.Contains(xObj.StreamData, []byte("BT")) {
							cRefs = append(cRefs, xID)
						}
					}
				}
			}

			pages = append(pages, pdfPageTreeEntry{Num: len(pages) + 1, ContentRefs: cRefs})
			return
		}

		if km := reKids.FindStringSubmatch(dictStr); len(km) > 1 {
			for _, r := range reRef.FindAllStringSubmatch(km[1], -1) {
				kidID, _ := strconv.Atoi(r[1])
				collectPages(kidID)
			}
		}
	}

	if pagesRootID > 0 {
		collectPages(pagesRootID)
	}

	if len(pages) == 0 {
		var fallbackRefs []int
		for _, obj := range objects {
			if isContentStream(obj.DictHeader) && obj.HasStream && len(obj.StreamData) > 0 {
				fallbackRefs = append(fallbackRefs, obj.ID)
			}
		}
		if len(fallbackRefs) > 0 {
			pages = append(pages, pdfPageTreeEntry{Num: 1, ContentRefs: fallbackRefs})
		}
	}

	return pages
}

func extractRedactionBoxes(options map[string]interface{}) []domain.PDFRedactionBox {
	if options == nil {
		return nil
	}
	if boxes, ok := options["redaction_boxes"].([]domain.PDFRedactionBox); ok {
		return boxes
	}
	if rawList, ok := options["redaction_boxes"].([]interface{}); ok {
		var boxes []domain.PDFRedactionBox
		for _, item := range rawList {
			if b, ok := item.(domain.PDFRedactionBox); ok {
				boxes = append(boxes, b)
			} else if m, ok := item.(map[string]interface{}); ok {
				var box domain.PDFRedactionBox
				if p, ok := m["page"].(float64); ok {
					box.Page = int(p)
				} else if p, ok := m["page"].(int); ok {
					box.Page = p
				}
				if x, ok := m["x"].(float64); ok {
					box.X = x
				}
				if y, ok := m["y"].(float64); ok {
					box.Y = y
				}
				if w, ok := m["width"].(float64); ok {
					box.Width = w
				}
				if h, ok := m["height"].(float64); ok {
					box.Height = h
				}
				boxes = append(boxes, box)
			}
		}
		return boxes
	}
	return nil
}

func extractDisabledEntities(options map[string]interface{}) map[string]bool {
	if options == nil {
		return nil
	}
	res := make(map[string]bool)
	if list, ok := options["disabled_entities"].([]string); ok {
		for _, e := range list {
			res[strings.ToUpper(strings.TrimSpace(e))] = true
		}
	} else if rawList, ok := options["disabled_entities"].([]interface{}); ok {
		for _, item := range rawList {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				res[strings.ToUpper(strings.TrimSpace(s))] = true
			}
		}
	}
	return res
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

type charPos struct {
	elemIdx   int
	byteStart int
	byteEnd   int
	x         float64
	width     float64
}

// redactContentStream scans a PDF content stream, scrubs sensitive text,
// replaces it with BlackoutMarker or DeletionMarker, and appends blackout rectangle operators.
func redactContentStream(data []byte, customTerms []string, disabledEntities map[string]bool, mode domain.ProcessingMode, fontCMaps map[string]*pdfCMap, defaultCMap *pdfCMap) ([]byte, bool) {
	// Skip streams that contain no text operators and no custom search terms
	hasTextOps := bytes.Contains(data, []byte("BT")) || bytes.Contains(data, []byte("Tj")) || bytes.Contains(data, []byte("TJ"))
	if !hasTextOps && len(customTerms) == 0 {
		return data, false
	}

	var out bytes.Buffer
	modified := false

	// Text state tracking
	inText := false
	curFont := ""
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

			op, _ := peekNextOperator(data, i)
			if op == "Tj" || op == "'" || op == "\"" {
				if op == "'" || op == "\"" {
					lineStartY -= leading
					currentX = lineStartX
					currentY = lineStartY
				}

				newRaw, boxes, endX, didRedact := processLiteralTextString(rawStr, decoded, currentFontSize, currentX, currentY, customTerms, disabledEntities, mode)
				if didRedact {
					modified = true
					out.Write(newRaw)
					pendingBoxes = append(pendingBoxes, boxes...)
				} else {
					out.Write(rawStr)
				}
				currentX = endX
			} else {
				out.Write(rawStr)
			}
			continue
		}

		// 2. Hex string: < ... >
		if data[i] == '<' && (i+1 < len(data) && data[i+1] != '<') {
			rawHex, _, nextPos := parseHexString(data, i)
			i = nextPos

			op, _ := peekNextOperator(data, i)
			if op == "Tj" || op == "'" || op == "\"" {
				if op == "'" || op == "\"" {
					lineStartY -= leading
					currentX = lineStartX
					currentY = lineStartY
				}

				activeCMap := fontCMaps[curFont]
				if activeCMap == nil {
					activeCMap = defaultCMap
				}

				newHex, boxes, endX, didRedact := processHexTextStringWithCMap(rawHex, currentFontSize, currentX, currentY, customTerms, disabledEntities, mode, activeCMap)
				if didRedact {
					modified = true
					out.Write(newHex)
					pendingBoxes = append(pendingBoxes, boxes...)
				} else {
					out.Write(rawHex)
				}
				currentX = endX
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
				activeCMap := fontCMaps[curFont]
				if activeCMap == nil {
					activeCMap = defaultCMap
				}

				newArr, boxes, endX, didRedact := processTJArrayWithCMap(rawArr, currentFontSize, currentX, currentY, customTerms, disabledEntities, mode, activeCMap)
				if didRedact {
					modified = true
					out.Write(newArr)
					pendingBoxes = append(pendingBoxes, boxes...)
				} else {
					out.Write(rawArr)
				}
				currentX = endX
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
			fs := extractPreviousFontSize(out.Bytes())
			if fs > 0 {
				currentFontSize = fs
			}
			fn := extractPreviousFontName(out.Bytes())
			if fn != "" {
				curFont = fn
			}
			out.WriteString("Tf")
		case "Tm":
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
	scrubbedRes, extraModified := scrubRemainingSensitiveText(res, customTerms, disabledEntities, mode)
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

func processLiteralTextString(rawStr, decoded []byte, fontSize, curX, curY float64, customTerms []string, disabledEntities map[string]bool, mode domain.ProcessingMode) ([]byte, []BoundingBox, float64, bool) {
	text := decodeText(decoded)
	charWidth := fontSize * 0.52
	runeCount := len([]rune(text))
	endX := curX + float64(runeCount)*charWidth

	matches := findSensitiveMatches(text, customTerms, disabledEntities)
	if len(matches) == 0 {
		return rawStr, nil, endX, false
	}

	sort.Slice(matches, func(i, j int) bool {
		return matches[i].start > matches[j].start
	})

	var boxes []BoundingBox
	redactedText := text
	marker := getPDFMarker(mode)

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
		redactedText = redactedText[:m.start] + marker + redactedText[m.end:]
	}

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

	return buf.Bytes(), boxes, endX, true
}

func processHexTextStringWithCMap(rawHex []byte, fontSize, curX, curY float64, customTerms []string, disabledEntities map[string]bool, mode domain.ProcessingMode, cmap *pdfCMap) ([]byte, []BoundingBox, float64, bool) {
	if len(rawHex) < 2 || rawHex[0] != '<' {
		return rawHex, nil, curX, false
	}
	inner := string(rawHex[1 : len(rawHex)-1])
	step := 4
	if cmap != nil && cmap.codeBytes > 0 {
		step = cmap.codeBytes * 2
	}
	spaceHex := "0003"
	if cmap != nil {
		spaceHex = cmap.getSpaceHex()
	}

	charWidth := fontSize * 0.52
	arrX := curX
	var decodedText strings.Builder
	var charPositions []charPos

	for k := 0; k+step <= len(inner); k += step {
		hexChunk := inner[k : k+step]
		decodedChar := ""
		if cmap != nil {
			decodedChar = cmap.decodeHex(hexChunk)
		} else {
			decodedChar = decodeHexUTF16(hexChunk)
		}
		if decodedChar == "" {
			decodedChar = " "
		}
		for _, r := range decodedChar {
			charPositions = append(charPositions, charPos{
				elemIdx:   0,
				byteStart: 1 + k,
				byteEnd:   1 + k + step,
				x:         arrX,
				width:     charWidth,
			})
			decodedText.WriteRune(r)
			arrX += charWidth
		}
	}

	fullStr := decodedText.String()
	matches := findSensitiveMatches(fullStr, customTerms, disabledEntities)
	if len(matches) == 0 {
		return rawHex, nil, arrX, false
	}

	type runeSpan struct {
		rStart int
		rEnd   int
	}
	var spans []runeSpan
	for _, m := range matches {
		rStart := len([]rune(fullStr[:m.start]))
		rEnd := len([]rune(fullStr[:m.end]))
		if rEnd > rStart && rEnd <= len(charPositions) {
			spans = append(spans, runeSpan{rStart: rStart, rEnd: rEnd})
		}
	}

	newHex := make([]byte, len(rawHex))
	copy(newHex, rawHex)

	var boxes []BoundingBox
	for _, sp := range spans {
		xStart := charPositions[sp.rStart].x
		lastCP := charPositions[sp.rEnd-1]
		xEnd := lastCP.x + lastCP.width
		bw := xEnd - xStart
		if bw < fontSize*1.2 {
			bw = fontSize * 1.2
		}
		by := curY - (fontSize * 0.25)
		bh := fontSize * 1.25

		boxes = append(boxes, BoundingBox{X: xStart, Y: by, W: bw, H: bh})

		for r := sp.rStart; r < sp.rEnd; r++ {
			cp := charPositions[r]
			if cp.byteEnd <= len(newHex)-1 {
				copy(newHex[cp.byteStart:cp.byteEnd], []byte(spaceHex))
			}
		}
	}

	return newHex, boxes, arrX, true
}

func processTJArrayWithCMap(rawArr []byte, fontSize, curX, curY float64, customTerms []string, disabledEntities map[string]bool, mode domain.ProcessingMode, cmap *pdfCMap) ([]byte, []BoundingBox, float64, bool) {
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

	var decodedText strings.Builder
	var charPositions []charPos
	arrX := curX
	charWidth := fontSize * 0.52

	step := 4
	if cmap != nil && cmap.codeBytes > 0 {
		step = cmap.codeBytes * 2
	}
	spaceHex := "0003"
	if cmap != nil {
		spaceHex = cmap.getSpaceHex()
	}

	for elemIdx, elem := range elements {
		if len(elem) == 0 {
			continue
		}
		if elem[0] == '<' {
			inner := string(elem[1 : len(elem)-1])
			for k := 0; k+step <= len(inner); k += step {
				hexChunk := inner[k : k+step]
				decodedChar := ""
				if cmap != nil {
					decodedChar = cmap.decodeHex(hexChunk)
				} else {
					decodedChar = decodeHexUTF16(hexChunk)
				}
				if decodedChar == "" {
					decodedChar = " "
				}
				for _, r := range decodedChar {
					charPositions = append(charPositions, charPos{
						elemIdx:   elemIdx,
						byteStart: 1 + k,
						byteEnd:   1 + k + step,
						x:         arrX,
						width:     charWidth,
					})
					decodedText.WriteRune(r)
					arrX += charWidth
				}
			}
		} else if elem[0] == '(' {
			_, decBytes, _ := parseLiteralString(elem, 0)
			litText := decodeText(decBytes)
			for byteOff, r := range litText {
				charPositions = append(charPositions, charPos{
					elemIdx:   elemIdx,
					byteStart: 1 + byteOff,
					byteEnd:   1 + byteOff + utf8.RuneLen(r),
					x:         arrX,
					width:     charWidth,
				})
				decodedText.WriteRune(r)
				arrX += charWidth
			}
		} else {
			if n, err := strconv.ParseFloat(string(elem), 64); err == nil {
				if n < -200 && decodedText.Len() > 0 && !strings.HasSuffix(decodedText.String(), " ") {
					charPositions = append(charPositions, charPos{
						elemIdx: -1,
						x:       arrX,
						width:   fontSize * 0.25,
					})
					decodedText.WriteString(" ")
					arrX += fontSize * 0.25
				}
				arrX -= (n / 1000.0) * fontSize
			}
		}
	}

	fullStr := decodedText.String()
	matches := findSensitiveMatches(fullStr, customTerms, disabledEntities)
	if len(matches) == 0 {
		return rawArr, nil, arrX, false
	}

	type runeSpan struct {
		rStart int
		rEnd   int
	}
	var spans []runeSpan
	for _, m := range matches {
		rStart := len([]rune(fullStr[:m.start]))
		rEnd := len([]rune(fullStr[:m.end]))
		if rEnd > rStart && rEnd <= len(charPositions) {
			spans = append(spans, runeSpan{rStart: rStart, rEnd: rEnd})
		}
	}

	clonedElements := make([][]byte, len(elements))
	for idx, e := range elements {
		clonedElements[idx] = make([]byte, len(e))
		copy(clonedElements[idx], e)
	}

	var boxes []BoundingBox
	for _, sp := range spans {
		xStart := charPositions[sp.rStart].x
		lastCP := charPositions[sp.rEnd-1]
		xEnd := lastCP.x + lastCP.width
		bw := xEnd - xStart
		if bw < fontSize*1.2 {
			bw = fontSize * 1.2
		}
		by := curY - (fontSize * 0.25)
		bh := fontSize * 1.25

		boxes = append(boxes, BoundingBox{X: xStart, Y: by, W: bw, H: bh})

		for r := sp.rStart; r < sp.rEnd; r++ {
			cp := charPositions[r]
			if cp.elemIdx >= 0 && cp.elemIdx < len(clonedElements) {
				targetElem := clonedElements[cp.elemIdx]
				if len(targetElem) > 0 && targetElem[0] == '<' {
					if cp.byteEnd <= len(targetElem)-1 {
						copy(targetElem[cp.byteStart:cp.byteEnd], []byte(spaceHex))
					}
				} else if len(targetElem) > 0 && targetElem[0] == '(' {
					for b := cp.byteStart; b < cp.byteEnd && b < len(targetElem)-1; b++ {
						targetElem[b] = ' '
					}
				}
			}
		}
	}

	var buf bytes.Buffer
	buf.WriteByte('[')
	for idx, e := range clonedElements {
		if idx > 0 {
			buf.WriteByte(' ')
		}
		buf.Write(e)
	}
	buf.WriteByte(']')

	return buf.Bytes(), boxes, arrX, true
}

func isBoundaryAfter(text string, end int) bool {
	if end >= len(text) {
		return true
	}
	rNext, _ := utf8.DecodeRuneInString(text[end:])
	return !unicode.IsLetter(rNext) && !unicode.IsDigit(rNext) && rNext != '_'
}

// findSensitiveMatches locates all PII entities and custom terms in text.
func findSensitiveMatches(text string, customTerms []string, disabledEntities map[string]bool) []textMatch {
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
	if !disabledEntities["SNILS"] {
		addRegexMatches(reSNILSFormatted, 1, nil)
		addRegexMatches(reSNILS11Digits, 1, func(s string) bool {
			return detector.ValidateSNILS(s)
		})
	}

	// 2. Passport RF
	if !disabledEntities["PASSPORT"] {
		addRegexMatches(rePassport4_6, 1, nil)
		addRegexMatches(rePassport2_2_6, 1, nil)
		addRegexMatches(rePassportPrefixed, 1, nil)
	}

	// 3. INN
	if !disabledEntities["INN"] {
		addRegexMatches(reINN12, 1, func(s string) bool {
			return detector.ValidateINN12(s)
		})
		addRegexMatches(reINNPrefixed, 1, nil)
	}

	// 4. Phone
	if !disabledEntities["PHONE"] {
		addRegexMatches(rePhone, 1, nil)
		addRegexMatches(rePhonePrefixed, 1, nil)
	}

	// 5. Card MIR / Luhn
	if !disabledEntities["BANK_CARD"] && !disabledEntities["CARD"] {
		addRegexMatches(reCard16, 1, nil)
	}

	// 6. FIO
	if !disabledEntities["FIO"] {
		addRegexMatches(reFIOPatronymic, 1, isFIOValid)
		addRegexMatches(reFIOPrefixed, 1, isFIOValid)
		addRegexMatches(reFIOInitials1, 1, nil)
		addRegexMatches(reFIOInitials2, 1, nil)
	}

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

func isFIOValid(s string) bool {
	parts := strings.Fields(s)
	if len(parts) < 2 || len(parts) > 3 {
		return false
	}
	stopWords := map[string]bool{
		"дает": true, "обязуется": true, "обязан": true, "вправе": true, "имеет": true,
		"согласен": true, "поручает": true, "принимает": true, "согласие": true,
		"обработку": true, "компании": true, "договора": true, "настоящий": true,
		"трудовой": true, "договор": true, "общество": true, "ответственностью": true,
		"стороны": true, "сторона": true, "работодатель": true, "работник": true,
		"предмет": true, "оплата": true, "режим": true, "срок": true, "действие": true,
		"защита": true, "персональных": true, "данных": true, "конфиденциальность": true,
		"адреса": true, "реквизиты": true, "подписи": true, "отметка": true, "получении": true,
		"экземпляра": true, "генеральный": true, "директор": true, "руководитель": true,
		"ведущего": true, "разработчика": true, "программного": true, "обеспечения": true,
		"условия": true, "рабочего": true, "времени": true, "отдыха": true, "гарантии": true,
	}
	for _, p := range parts {
		if stopWords[strings.ToLower(p)] {
			return false
		}
		subparts := strings.Split(p, "-")
		for _, sp := range subparts {
			runes := []rune(sp)
			if len(runes) < 2 {
				return false
			}
			if !unicode.IsUpper(runes[0]) {
				return false
			}
			for _, r := range runes[1:] {
				if !unicode.IsLower(r) {
					return false
				}
			}
			if stopWords[strings.ToLower(sp)] {
				return false
			}
		}
	}
	return true
}

// scrubRemainingSensitiveText performs defense-in-depth replacement of any
// lingering sensitive strings across the decompressed stream.
func scrubRemainingSensitiveText(stream []byte, customTerms []string, disabledEntities map[string]bool, mode domain.ProcessingMode) ([]byte, bool) {
	text := string(stream)
	matches := findSensitiveMatches(text, customTerms, disabledEntities)
	if len(matches) == 0 {
		return stream, false
	}

	sort.Slice(matches, func(i, j int) bool {
		return matches[i].start > matches[j].start
	})

	res := text
	marker := getPDFMarker(mode)
	for _, m := range matches {
		res = res[:m.start] + marker + res[m.end:]
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
	if bytes.Contains(dictHeader, []byte("/FontFile")) || bytes.Contains(dictHeader, []byte("/Length1")) {
		return false
	}
	if bytes.Contains(dictHeader, []byte("/Type /CMap")) || bytes.Contains(dictHeader, []byte("/Type/CMap")) {
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

func extractPreviousFontName(buf []byte) string {
	tokens := getLastTokens(buf, 2)
	if len(tokens) >= 2 {
		return strings.TrimPrefix(string(tokens[1]), "/")
	}
	return ""
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

type pdfCMap struct {
	mapping   map[uint32]string
	codeBytes int
}

func parseCMapStream(data []byte) *pdfCMap {
	cmap := &pdfCMap{
		mapping:   make(map[uint32]string),
		codeBytes: 2,
	}

	reCodeSpace := regexp.MustCompile(`<([0-9A-Fa-f]+)>\s*<([0-9A-Fa-f]+)>`)
	csIdx := bytes.Index(data, []byte("begincodespacerange"))
	if csIdx != -1 {
		csEnd := bytes.Index(data[csIdx:], []byte("endcodespacerange"))
		if csEnd != -1 {
			csBlock := data[csIdx : csIdx+csEnd]
			if m := reCodeSpace.FindSubmatch(csBlock); len(m) > 1 {
				cmap.codeBytes = len(m[1]) / 2
				if cmap.codeBytes <= 0 {
					cmap.codeBytes = 2
				}
			}
		}
	}

	reBfCharSection := regexp.MustCompile(`(?s)\d+\s+beginbfchar\s+(.*?)\s+endbfchar`)
	for _, m := range reBfCharSection.FindAllSubmatch(data, -1) {
		lines := strings.Split(string(m[1]), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			rePairs := regexp.MustCompile(`<([0-9A-Fa-f]+)>\s*<([0-9A-Fa-f]+)>`)
			for _, p := range rePairs.FindAllStringSubmatch(line, -1) {
				srcHex := p[1]
				dstHex := p[2]
				srcVal, err := strconv.ParseUint(srcHex, 16, 32)
				if err == nil {
					cmap.mapping[uint32(srcVal)] = decodeHexUTF16(dstHex)
				}
			}
		}
	}

	reBfRangeSection := regexp.MustCompile(`(?s)\d+\s+beginbfrange\s+(.*?)\s+endbfrange`)
	for _, m := range reBfRangeSection.FindAllSubmatch(data, -1) {
		lines := strings.Split(string(m[1]), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			if strings.Contains(line, "[") {
				reArr := regexp.MustCompile(`<([0-9A-Fa-f]+)>\s*<([0-9A-Fa-f]+)>\s*\[(.*?)\]`)
				if arrMatch := reArr.FindStringSubmatch(line); len(arrMatch) > 3 {
					startVal, err1 := strconv.ParseUint(arrMatch[1], 16, 32)
					endVal, err2 := strconv.ParseUint(arrMatch[2], 16, 32)
					if err1 == nil && err2 == nil {
						reHex := regexp.MustCompile(`<([0-9A-Fa-f]+)>`)
						dstMatches := reHex.FindAllStringSubmatch(arrMatch[3], -1)
						for i, dm := range dstMatches {
							srcCode := uint32(startVal) + uint32(i)
							if srcCode <= uint32(endVal) {
								cmap.mapping[srcCode] = decodeHexUTF16(dm[1])
							}
						}
					}
				}
				continue
			}

			reRange := regexp.MustCompile(`<([0-9A-Fa-f]+)>\s*<([0-9A-Fa-f]+)>\s*<([0-9A-Fa-f]+)>`)
			if rMatch := reRange.FindStringSubmatch(line); len(rMatch) > 3 {
				startVal, err1 := strconv.ParseUint(rMatch[1], 16, 32)
				endVal, err2 := strconv.ParseUint(rMatch[2], 16, 32)
				dstStartVal, err3 := strconv.ParseUint(rMatch[3], 16, 32)
				if err1 == nil && err2 == nil && err3 == nil {
					count := int(endVal - startVal)
					for i := 0; i <= count; i++ {
						srcCode := uint32(startVal) + uint32(i)
						dstCode := uint32(dstStartVal) + uint32(i)
						cmap.mapping[srcCode] = string(utf16.Decode([]uint16{uint16(dstCode)}))
					}
				}
			}
		}
	}

	return cmap
}

func decodeHexUTF16(hexStr string) string {
	if len(hexStr)%2 != 0 {
		hexStr = "0" + hexStr
	}
	raw, err := hex.DecodeString(hexStr)
	if err != nil || len(raw) == 0 {
		return ""
	}
	if len(raw)%2 != 0 {
		raw = append([]byte{0}, raw...)
	}
	u16s := make([]uint16, len(raw)/2)
	for i := 0; i < len(u16s); i++ {
		u16s[i] = binary.BigEndian.Uint16(raw[i*2 : i*2+2])
	}
	return string(utf16.Decode(u16s))
}

func (cm *pdfCMap) decodeHex(hexStr string) string {
	if cm == nil || len(cm.mapping) == 0 {
		return decodeHexUTF16(hexStr)
	}
	var sb strings.Builder
	step := cm.codeBytes * 2
	if step <= 0 {
		step = 4
	}
	for i := 0; i+step <= len(hexStr); i += step {
		chunk := hexStr[i : i+step]
		code, err := strconv.ParseUint(chunk, 16, 32)
		if err == nil {
			if s, ok := cm.mapping[uint32(code)]; ok {
				sb.WriteString(s)
			} else {
				if code == 3 || code == 32 {
					sb.WriteString(" ")
				}
			}
		}
	}
	return sb.String()
}

func (cm *pdfCMap) getSpaceHex() string {
	if cm != nil {
		for code, s := range cm.mapping {
			if s == " " {
				step := cm.codeBytes * 2
				if step <= 0 {
					step = 4
				}
				return fmt.Sprintf("%0*X", step, code)
			}
		}
		step := cm.codeBytes * 2
		if step <= 0 {
			step = 4
		}
		return fmt.Sprintf("%0*X", step, 3)
	}
	return "0003"
}

func extractFontCMaps(objMap map[int]*pdfObject) (map[string]*pdfCMap, *pdfCMap) {
	cmaps := make(map[int]*pdfCMap)
	for id, obj := range objMap {
		if !obj.HasStream || len(obj.StreamData) == 0 {
			continue
		}
		streamData := obj.StreamData
		if obj.Compressed {
			zr, err := zlib.NewReader(bytes.NewReader(streamData))
			if err == nil {
				decomp, readErr := io.ReadAll(zr)
				_ = zr.Close()
				if readErr == nil {
					streamData = decomp
				}
			}
		}
		if bytes.Contains(streamData, []byte("begincmap")) {
			cmaps[id] = parseCMapStream(streamData)
		}
	}

	fontCMaps := make(map[string]*pdfCMap)
	reFontInRes := regexp.MustCompile(`/([A-Za-z0-9_-]+)\s+(\d+)\s+0\s+R`)
	reFontDict := regexp.MustCompile(`(?s)/Font\s*<<([^>]*)>>`)
	reToUnicode := regexp.MustCompile(`/ToUnicode\s+(\d+)\s+0\s+R`)

	for _, obj := range objMap {
		if fm := reFontDict.FindStringSubmatch(string(obj.DictHeader)); len(fm) > 1 {
			for _, item := range reFontInRes.FindAllStringSubmatch(fm[1], -1) {
				fontName := item[1]
				fontObjID, _ := strconv.Atoi(item[2])
				if fObj, ok := objMap[fontObjID]; ok {
					if tuMatch := reToUnicode.FindStringSubmatch(string(fObj.DictHeader)); len(tuMatch) > 1 {
						tuID, _ := strconv.Atoi(tuMatch[1])
						if cm, ok := cmaps[tuID]; ok {
							fontCMaps[fontName] = cm
						}
					}
				}
			}
		}
	}

	var defaultCMap *pdfCMap
	for _, cm := range cmaps {
		defaultCMap = cm
		break
	}

	return fontCMaps, defaultCMap
}

func unescapePDFString(s string) string {
	s = strings.ReplaceAll(s, `\n`, "\n")
	s = strings.ReplaceAll(s, `\r`, "")
	s = strings.ReplaceAll(s, `\t`, " ")
	s = strings.ReplaceAll(s, `\(`, "(")
	s = strings.ReplaceAll(s, `\)`, ")")
	s = strings.ReplaceAll(s, `\\`, "\\")
	return s
}

func decodePDFString(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
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
	hasHighBytes := false
	for _, b := range raw {
		if b >= 0x80 {
			hasHighBytes = true
			break
		}
	}
	if hasHighBytes {
		if cp1251, err := charmap.Windows1251.NewDecoder().Bytes(raw); err == nil && utf8.Valid(cp1251) {
			return string(cp1251)
		}
	}
	return string(raw)
}

// ExtractPDFRawTextAndPageCount parses the PDF object structure, determines page count,
// and extracts all text content from content streams with CMap decoding.
func ExtractPDFRawTextAndPageCount(pdfBytes []byte) (int, string, error) {
	if !bytes.HasPrefix(pdfBytes, []byte("%PDF-")) {
		return 0, "", errors.New("invalid PDF: missing %PDF- header")
	}

	objects, _, err := parsePDF(pdfBytes)
	if err != nil {
		return 0, "", err
	}

	objMap := make(map[int]*pdfObject, len(objects))
	for i := range objects {
		objMap[objects[i].ID] = &objects[i]
	}

	pageTree := getPDFPageTree(pdfBytes, objMap, objects)
	pageCount := len(pageTree)
	if pageCount == 0 {
		pageCount = 1
	}

	fontCMaps, defaultCMap := extractFontCMaps(objMap)

	var sb strings.Builder
	for _, obj := range objects {
		if !obj.HasStream || len(obj.StreamData) == 0 {
			continue
		}
		if !isContentStream(obj.DictHeader) {
			continue
		}

		streamData := obj.StreamData
		if obj.Compressed {
			zr, err := zlib.NewReader(bytes.NewReader(obj.StreamData))
			if err == nil {
				decomp, readErr := io.ReadAll(zr)
				_ = zr.Close()
				if readErr == nil {
					streamData = decomp
				}
			}
		}

		currentFont := ""
		i := 0
		n := len(streamData)
		for i < n {
			b := streamData[i]
			if b == '/' {
				end := i + 1
				for end < n && !isWhitespace(streamData[end]) && streamData[end] != '/' && streamData[end] != '[' && streamData[end] != '(' {
					end++
				}
				name := string(streamData[i+1 : end])
				look := end
				for look < n && (isWhitespace(streamData[look]) || (streamData[look] >= '0' && streamData[look] <= '9') || streamData[look] == '.') {
					look++
				}
				if look+2 <= n && streamData[look] == 'T' && streamData[look+1] == 'f' {
					currentFont = name
				}
				i = end
				continue
			}

			if b == '(' {
				_, decoded, next := parseLiteralString(streamData, i)
				txt := decodeText(decoded)
				if txt != "" {
					sb.WriteString(txt)
					sb.WriteString(" ")
				}
				i = next
				continue
			}

			if b == '<' {
				if i+1 < n && streamData[i+1] == '<' {
					i += 2
					continue
				}
				rawHex, decoded, next := parseHexString(streamData, i)
				activeCMap := defaultCMap
				if cm, ok := fontCMaps[currentFont]; ok && cm != nil {
					activeCMap = cm
				}
				if activeCMap != nil && len(rawHex) >= 2 {
					hexStr := string(rawHex[1 : len(rawHex)-1])
					txt := activeCMap.decodeHex(hexStr)
					if txt != "" {
						sb.WriteString(txt)
						sb.WriteString(" ")
					}
				} else {
					txt := decodePDFString(decoded)
					if txt != "" {
						sb.WriteString(txt)
						sb.WriteString(" ")
					}
				}
				i = next
				continue
			}

			if b == '[' {
				rawArr, next := parseArray(streamData, i)
				activeCMap := defaultCMap
				if cm, ok := fontCMaps[currentFont]; ok && cm != nil {
					activeCMap = cm
				}
				arrIdx := 0
				arrLen := len(rawArr)
				for arrIdx < arrLen {
					ab := rawArr[arrIdx]
					if ab == '(' {
						_, d, nxt := parseLiteralString(rawArr, arrIdx)
						txt := decodeText(d)
						if txt != "" {
							sb.WriteString(txt)
						}
						arrIdx = nxt
					} else if ab == '<' {
						if arrIdx+1 < arrLen && rawArr[arrIdx+1] == '<' {
							arrIdx += 2
							continue
						}
						rHex, d, nxt := parseHexString(rawArr, arrIdx)
						if activeCMap != nil && len(rHex) >= 2 {
							sb.WriteString(activeCMap.decodeHex(string(rHex[1 : len(rHex)-1])))
						} else {
							sb.WriteString(decodePDFString(d))
						}
						arrIdx = nxt
					} else {
						arrIdx++
					}
				}
				sb.WriteString(" ")
				i = next
				continue
			}

			i++
		}
		sb.WriteString("\n")
	}

	return pageCount, strings.TrimSpace(sb.String()), nil
}

