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
	"time"

	"dep_service/pkg/depmap"
)

// RestoreODS reads an anonymized ODS document from r, replaces synthetic values with their originals
// according to dm, and writes the restored ODS document to w.
// Returns the count of restored cells.
func RestoreODS(ctx context.Context, r io.ReaderAt, size int64, w io.Writer, dm *depmap.DepMap) (int, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	default:
	}

	if dm == nil {
		return 0, errors.New("depmap cannot be nil")
	}
	if r == nil {
		return 0, errors.New("reader cannot be nil")
	}
	if w == nil {
		return 0, errors.New("writer cannot be nil")
	}
	if size <= 0 {
		return 0, errors.New("invalid file size")
	}

	zr, err := zip.NewReader(r, size)
	if err != nil {
		return 0, fmt.Errorf("failed to open ods zip: %w", err)
	}

	contentBytes, err := readZipEntry(zr, "content.xml")
	if err != nil {
		return 0, err
	}

	doc, err := parseODSDoc(contentBytes)
	if err != nil {
		return 0, fmt.Errorf("failed to parse content.xml: %w", err)
	}

	var replacements []textReplacement
	restoredCount := 0
	hasCellMappings := len(dm.CellMappings) > 0

	for _, sheet := range doc.sheets {
		select {
		case <-ctx.Done():
			return restoredCount, ctx.Err()
		default:
		}

		var headers []string
		if hasCellMappings {
			sampleLimit := len(sheet.rows)
			if sampleLimit > 40 {
				sampleLimit = 40
			}
			sampleRows := make([][]string, sampleLimit)
			for rIdx := 0; rIdx < sampleLimit; rIdx++ {
				sampleRows[rIdx] = rowToStrings(sheet.rows[rIdx])
			}
			if len(sampleRows) > 0 {
				_, headers = FindHeaderRow(sampleRows)
			}
		}

		for rIdx, row := range sheet.rows {
			physicalRow := rIdx + 1
			for _, cell := range row.cells {
				val := cell.fullText
				if len(val) == 0 {
					continue
				}

				orig, ok := RestoreCellString(val, dm)
				if !ok && hasCellMappings {
					trimmed := strings.TrimSpace(strings.TrimPrefix(val, "\ufeff"))
					if trimmed == DeletionMarker {
						var headerName string
						if cell.colIdx < len(headers) {
							headerName = strings.TrimSpace(headers[cell.colIdx])
						}
						if o, found := dm.GetCell(sheet.name, headerName, physicalRow); found {
							orig = o
							ok = true
						}
					}
				}

				dateRestored := false
				var origDateAttr string

				// Check if office:date-value is on the cell tag and can be restored
				if len(cell.tagBytes) > 0 && strings.Contains(string(cell.tagBytes), "office:date-value") {
					curDateAttr := extractAttrValue(cell.tagBytes, "office:date-value")
					if curDateAttr != "" {
						if origDate, found := dm.LookupOriginal(curDateAttr); found {
							origDateAttr = origDate
							dateRestored = true
						}
					}
				}

				if ok || dateRestored {
					restoredCount++

					if ok && len(cell.paragraphs) > 0 {
						p := cell.paragraphs[0]
						var buf bytes.Buffer
						xml.EscapeText(&buf, []byte(orig))
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

					// Update tag attributes if present
					if len(cell.tagBytes) > 0 {
						newTag := cell.tagBytes
						tagModified := false

						tagStr := string(cell.tagBytes)
						if ok && strings.Contains(tagStr, "office:string-value") {
							oldAttr := extractAttrValue(cell.tagBytes, "office:string-value")
							if oldAttr != "" {
								newTag = updateAttrInTag(newTag, "office:string-value", oldAttr, orig)
								tagModified = true
							}
						}

						if strings.Contains(tagStr, "office:date-value") {
							curDateAttr := extractAttrValue(cell.tagBytes, "office:date-value")
							if curDateAttr != "" {
								targetDate := origDateAttr
								if targetDate == "" && ok {
									// Fallback: if orig date text was restored, try parsing to ISO format
									for _, layout := range []string{"02.01.2006", "02/01/2006", "02-01-2006", "2006-01-02"} {
										if t, err := time.Parse(layout, orig); err == nil {
											targetDate = t.Format("2006-01-02")
											break
										}
									}
								}
								if targetDate != "" && targetDate != curDateAttr {
									newTag = updateAttrInTag(newTag, "office:date-value", curDateAttr, targetDate)
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
	}

	restoredContent := applyReplacements(contentBytes, replacements)

	if err := repackODSZip(zr, w, restoredContent); err != nil {
		return restoredCount, fmt.Errorf("failed to repack restored ods zip: %w", err)
	}

	return restoredCount, nil
}
