package formats

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"sort"

	"dep_service/internal/domain"
	"dep_service/pkg/anonymizer"
	"dep_service/pkg/depmap"
)

// TXTHandler processes plain text (.txt) files with DLP-safe tokenization or synthetic anonymization.
type TXTHandler struct {
	Anon *anonymizer.Anonymizer
}

// NewTXTHandler creates a new TXTHandler with the given Anonymizer.
func NewTXTHandler(anon *anonymizer.Anonymizer) *TXTHandler {
	return &TXTHandler{Anon: anon}
}

// SupportedExtensions returns extensions handled by TXTHandler.
func (h *TXTHandler) SupportedExtensions() []string {
	return []string{".txt", ".text"}
}

// Process reads plain text line-by-line, detects PII entities, replaces them with tokens or synthetic data, and writes to w.
func (h *TXTHandler) Process(ctx context.Context, r io.Reader, w io.Writer, options map[string]interface{}) error {
	if ctx == nil {
		ctx = context.Background()
	}

	strategy := getProcessingStrategy(options)
	salt := ""
	if options != nil {
		if s, ok := options["salt"].(string); ok {
			salt = s
		}
	}
	ts := anonymizer.NewTokenSession(salt)

	var dm *depmap.DepMap
	if d, ok := options["depmap"].(*depmap.DepMap); ok {
		dm = d
	} else if d, ok := options["depMap"].(*depmap.DepMap); ok {
		dm = d
	}
	if dm != nil {
		dm.Mode = string(strategy)
	}

	scanner := bufio.NewScanner(r)
	writer := bufio.NewWriter(w)
	defer writer.Flush()

	firstLine := true
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line := scanner.Text()
		matches := findDocxSensitiveMatches(line)
		if len(matches) == 0 {
			if !firstLine {
				if _, err := writer.WriteString("\n"); err != nil {
					return err
				}
			}
			if _, err := writer.WriteString(line); err != nil {
				return err
			}
			firstLine = false
			continue
		}

		// Sort matches descending by start to replace without invalidating offsets
		sort.Slice(matches, func(i, j int) bool {
			return matches[i].start > matches[j].start
		})

		newLine := line
		for _, m := range matches {
			if m.start < 0 || m.end > len(newLine) || m.start >= m.end {
				continue
			}
			origVal := newLine[m.start:m.end]
			var replacement string

			if strategy == domain.StrategyDelete {
				replacement = DeletionMarker
			} else if strategy == domain.StrategySemanticTokens || strategy == "" {
				tokenType := entityTypeToTokenType(m.entityType)
				replacement = ts.GenerateToken(tokenType, origVal)
				if dm != nil {
					dm.Put(replacement, origVal)
				}
			} else {
				replacement = generateSynthetic(h.Anon, m.entityType, origVal)
				if dm != nil {
					dm.Put(replacement, origVal)
				}
			}

			newLine = newLine[:m.start] + replacement + newLine[m.end:]
		}

		if !firstLine {
			if _, err := writer.WriteString("\n"); err != nil {
				return err
			}
		}
		if _, err := writer.WriteString(newLine); err != nil {
			return err
		}
		firstLine = false
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading text stream: %w", err)
	}

	return nil
}

// Inspect reads plain text line-by-line, detects PII entities, counts them, and builds structured previews.
func (h *TXTHandler) Inspect(r io.Reader) (int, []EntityCount, []DocxTextSnippet, []TextParagraphPreview, []TextDocumentBlock, error) {
	if r == nil {
		return 0, nil, nil, nil, nil, errors.New("reader cannot be nil")
	}

	ts := anonymizer.NewTokenSession("")
	scanner := bufio.NewScanner(r)
	lineCount := 0
	counts := make(map[string]int)
	var snippets []DocxTextSnippet
	var paragraphPreviews []TextParagraphPreview
	var documentBlocks []TextDocumentBlock

	for scanner.Scan() {
		line := scanner.Text()
		lineCount++

		matches := findDocxSensitiveMatches(line)
		for _, m := range matches {
			counts[m.entityType]++
			if len(snippets) < 50 {
				tokenVal := ts.GenerateToken(EntityTypeToTokenType(m.entityType), m.value)
				snippets = append(snippets, DocxTextSnippet{
					Original:   m.value,
					Anonymized: tokenVal,
					Type:       m.entityType,
				})
			}
		}

		segments := buildParagraphSegments(line, matches, h.Anon, ts)
		preview := TextParagraphPreview{
			Index:    lineCount,
			Segments: segments,
		}
		paragraphPreviews = append(paragraphPreviews, preview)
		documentBlocks = append(documentBlocks, TextDocumentBlock{
			Type:     "paragraph",
			Index:    lineCount,
			Segments: segments,
		})
	}

	if err := scanner.Err(); err != nil {
		return 0, nil, nil, nil, nil, fmt.Errorf("error reading text stream: %w", err)
	}

	var entitiesCounts []EntityCount
	for t, c := range counts {
		entitiesCounts = append(entitiesCounts, EntityCount{
			Type:  t,
			Count: c,
		})
	}
	sort.Slice(entitiesCounts, func(i, j int) bool {
		return entitiesCounts[i].Count > entitiesCounts[j].Count
	})

	return lineCount, entitiesCounts, snippets, paragraphPreviews, documentBlocks, nil
}

