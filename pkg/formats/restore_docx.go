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

	"dep_service/pkg/depmap"
)

type synthOrigPair struct {
	synth string
	orig  string
}

// RestoreDOCX reads an anonymized DOCX document from r, replaces synthetic values with their original
// values according to dm, writes the restored document to w, and returns the count of restored entities.
func RestoreDOCX(ctx context.Context, r io.ReaderAt, size int64, w io.Writer, dm *depmap.DepMap) (int, error) {
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
		return 0, fmt.Errorf("failed to open docx zip: %w", err)
	}

	// Collect synthetic-to-original mappings from Mapping and Entities
	mapping := make(map[string]string)
	if dm.Mapping != nil {
		for k, v := range dm.Mapping {
			if k != "" {
				mapping[k] = v
			}
		}
	}
	if dm.Entities != nil {
		for k, v := range dm.Entities {
			if k != "" {
				mapping[k] = v
			}
		}
	}

	var pairs []synthOrigPair
	for k, v := range mapping {
		pairs = append(pairs, synthOrigPair{synth: k, orig: v})
	}

	// Sort synthetic values by length descending, then alphabetically for determinism
	sort.Slice(pairs, func(i, j int) bool {
		if len(pairs[i].synth) == len(pairs[j].synth) {
			return pairs[i].synth < pairs[j].synth
		}
		return len(pairs[i].synth) > len(pairs[j].synth)
	})

	restoredCount := 0
	updatedXMLs := make(map[string][]byte)

	for _, f := range zr.File {
		select {
		case <-ctx.Done():
			return restoredCount, ctx.Err()
		default:
		}

		if !isDocumentXML(f.Name) {
			continue
		}

		rc, err := f.Open()
		if err != nil {
			return restoredCount, fmt.Errorf("failed to open %s: %w", f.Name, err)
		}
		data, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			return restoredCount, fmt.Errorf("failed to read %s: %w", f.Name, err)
		}

		paras, err := parseDocxXML(data)
		if err != nil {
			return restoredCount, fmt.Errorf("failed to parse %s: %w", f.Name, err)
		}

		var replacements []textReplacement

		for _, p := range paras {
			for _, n := range p.nodes {
				curText := n.text
				nodeModified := false

				for _, p := range pairs {
					if strings.Contains(curText, p.synth) {
						c := strings.Count(curText, p.synth)
						curText = strings.ReplaceAll(curText, p.synth, p.orig)
						restoredCount += c
						nodeModified = true
					}
				}

				if nodeModified {
					var replBytes []byte
					if curText == "" {
						replBytes = []byte("<w:t></w:t>")
					} else {
						var escBuf bytes.Buffer
						_ = xml.EscapeText(&escBuf, []byte(curText))
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
			return restoredCount, ctx.Err()
		default:
		}

		if updatedData, ok := updatedXMLs[f.Name]; ok {
			h := &zip.FileHeader{
				Name:   f.Name,
				Method: zip.Deflate,
			}
			fw, err := zw.CreateHeader(h)
			if err != nil {
				return restoredCount, fmt.Errorf("failed to create zip entry %s: %w", f.Name, err)
			}
			if _, err := fw.Write(updatedData); err != nil {
				return restoredCount, fmt.Errorf("failed to write zip entry %s: %w", f.Name, err)
			}
		} else {
			fw, err := zw.CreateHeader(&f.FileHeader)
			if err != nil {
				return restoredCount, fmt.Errorf("failed to create untouched zip entry %s: %w", f.Name, err)
			}
			rc, err := f.Open()
			if err != nil {
				return restoredCount, fmt.Errorf("failed to open untouched zip entry %s: %w", f.Name, err)
			}
			_, err = io.Copy(fw, rc)
			_ = rc.Close()
			if err != nil {
				return restoredCount, fmt.Errorf("failed to copy untouched zip entry %s: %w", f.Name, err)
			}
		}
	}

	if err := zw.Close(); err != nil {
		return restoredCount, fmt.Errorf("failed to close restored docx zip writer: %w", err)
	}

	return restoredCount, nil
}
