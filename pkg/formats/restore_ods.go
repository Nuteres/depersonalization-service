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

	for _, sheet := range doc.sheets {
		select {
		case <-ctx.Done():
			return restoredCount, ctx.Err()
		default:
		}

		for _, row := range sheet.rows {
			for _, cell := range row.cells {
				val := cell.fullText
				if len(val) == 0 {
					continue
				}

				orig, ok := dm.LookupOriginal(val)
				if !ok {
					trimmed := strings.TrimSpace(val)
					trimmed = strings.TrimPrefix(trimmed, "\ufeff")
					if trimmed != val {
						orig, ok = dm.LookupOriginal(trimmed)
					}
				}

				if ok {
					restoredCount++

					if len(cell.paragraphs) > 0 {
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

					// If cell start tag contained office:string-value, update attribute
					if len(cell.tagBytes) > 0 {
						tagStr := string(cell.tagBytes)
						if strings.Contains(tagStr, "office:string-value") {
							oldAttr := extractAttrValue(cell.tagBytes, "office:string-value")
							if oldAttr != "" {
								newTag := updateAttrInTag(cell.tagBytes, "office:string-value", oldAttr, orig)
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
	}

	restoredContent := applyReplacements(contentBytes, replacements)

	if err := repackODSZip(zr, w, restoredContent); err != nil {
		return restoredCount, fmt.Errorf("failed to repack restored ods zip: %w", err)
	}

	return restoredCount, nil
}
