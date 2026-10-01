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

	"dep_service/pkg/depmap"
)

// RestoreODT reads an anonymized ODT document from r, replaces synthetic values with their original
// values according to dm, writes the restored document to w, and returns the count of restored entities.
func RestoreODT(ctx context.Context, r io.ReaderAt, size int64, w io.Writer, dm *depmap.DepMap) (int, error) {
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
		return 0, fmt.Errorf("failed to open odt zip: %w", err)
	}

	// Collect synthetic-to-original mappings
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

		if !isODTTextXML(f.Name) {
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

		paras, err := parseODTXML(data)
		if err != nil {
			return restoredCount, fmt.Errorf("failed to parse %s: %w", f.Name, err)
		}

		var replacements []textReplacement

		for _, p := range paras {
			for _, n := range p.nodes {
				newText, count := replaceTokensSinglePass(n.text, pairs)
				if count > 0 {
					restoredCount += count

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

	// mimetype must be first uncompressed
	for _, f := range zr.File {
		if f.Name == "mimetype" {
			h := &zip.FileHeader{
				Name:   "mimetype",
				Method: zip.Store,
			}
			fw, err := zw.CreateHeader(h)
			if err != nil {
				_ = zw.Close()
				return restoredCount, fmt.Errorf("failed to create mimetype header: %w", err)
			}
			rc, err := f.Open()
			if err != nil {
				_ = zw.Close()
				return restoredCount, fmt.Errorf("failed to open mimetype: %w", err)
			}
			_, err = io.Copy(fw, rc)
			_ = rc.Close()
			if err != nil {
				_ = zw.Close()
				return restoredCount, fmt.Errorf("failed to copy mimetype: %w", err)
			}
			break
		}
	}

	for _, f := range zr.File {
		select {
		case <-ctx.Done():
			_ = zw.Close()
			return restoredCount, ctx.Err()
		default:
		}

		if f.Name == "mimetype" {
			continue
		}

		if updatedData, ok := updatedXMLs[f.Name]; ok {
			h := &zip.FileHeader{
				Name:   f.Name,
				Method: zip.Deflate,
			}
			fw, err := zw.CreateHeader(h)
			if err != nil {
				_ = zw.Close()
				return restoredCount, fmt.Errorf("failed to create zip entry %s: %w", f.Name, err)
			}
			if _, err := fw.Write(updatedData); err != nil {
				_ = zw.Close()
				return restoredCount, fmt.Errorf("failed to write zip entry %s: %w", f.Name, err)
			}
		} else {
			if err := zw.Copy(f); err != nil {
				_ = zw.Close()
				return restoredCount, fmt.Errorf("failed to copy untouched zip entry %s: %w", f.Name, err)
			}
		}
	}

	if err := zw.Close(); err != nil {
		return restoredCount, fmt.Errorf("failed to close restored odt zip writer: %w", err)
	}

	return restoredCount, nil
}
